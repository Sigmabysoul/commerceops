// This file assembles domain services, infrastructure adapters, and HTTP routes into the running application in the application composition layer.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/domain/automation"
	"github.com/commerceops/commerceops/services/api/internal/domain/batch"
	"github.com/commerceops/commerceops/services/api/internal/domain/consignment"
	"github.com/commerceops/commerceops/services/api/internal/domain/core"
	"github.com/commerceops/commerceops/services/api/internal/domain/inventory"
	"github.com/commerceops/commerceops/services/api/internal/domain/marketplace"
	"github.com/commerceops/commerceops/services/api/internal/domain/marketplace/amazon"
	"github.com/commerceops/commerceops/services/api/internal/domain/marketplace/snapdeal"
	"github.com/commerceops/commerceops/services/api/internal/domain/marketplaceaccount"
	"github.com/commerceops/commerceops/services/api/internal/domain/printing"
	"github.com/commerceops/commerceops/services/api/internal/domain/product"
	"github.com/commerceops/commerceops/services/api/internal/domain/reporting"
	returnsdomain "github.com/commerceops/commerceops/services/api/internal/domain/returns"
	"github.com/commerceops/commerceops/services/api/internal/domain/traceability"
	"github.com/commerceops/commerceops/services/api/internal/platform/auth"
	"github.com/commerceops/commerceops/services/api/internal/platform/authorization"
	"github.com/commerceops/commerceops/services/api/internal/platform/config"
	"github.com/commerceops/commerceops/services/api/internal/platform/database"
	"github.com/commerceops/commerceops/services/api/internal/platform/documents/pdf/extractor"
	"github.com/commerceops/commerceops/services/api/internal/platform/documents/pdf/generator"
	"github.com/commerceops/commerceops/services/api/internal/platform/health"
	"github.com/commerceops/commerceops/services/api/internal/platform/httpserver"
	"github.com/commerceops/commerceops/services/api/internal/platform/objectstorage"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/v1/health", health.NewHandler(db, cfg.DatabaseTimeout))
	authService := auth.NewService(db, cfg.SessionLifetime)
	authHTTP := auth.NewHTTPHandler(authService, cfg.SecureCookies, cfg.SessionLifetime)
	authorizer := authorization.NewService(db)
	coreHTTP := core.NewHTTPHandler(core.NewService(db, authorizer))
	productHTTP := product.NewHTTPHandler(product.NewService(db, authorizer))
	marketplaceAccountHTTP := marketplaceaccount.NewHTTPHandler(marketplaceaccount.NewService(db, authorizer))
	inventoryService := inventory.NewService(db, authorizer)
	inventoryHTTP := inventory.NewHTTPHandler(inventoryService)
	reportingHTTP := reporting.NewHTTPHandler(reporting.NewService(db, authorizer))
	returnsHTTP := returnsdomain.NewHTTPHandler(returnsdomain.NewService(db, authorizer, inventoryService))
	consignmentHTTP := consignment.NewHTTPHandler(consignment.NewService(db, authorizer, inventoryService))
	traceabilityHTTP := traceability.NewHTTPHandler(traceability.NewService(db, authorizer))
	storage, err := newObjectStorage(ctx, cfg)
	if err != nil {
		return err
	}
	physicalPrinting := printing.NewService(db, authorizer, storage)
	printingHTTP := printing.NewHTTPHandler(physicalPrinting)
	automationService := automation.NewService(db, authorizer, physicalPrinting)
	automation.NewHTTPHandler(automationService).Register(mux, authHTTP.RequireSession)
	automationCtx, stopAutomation := context.WithCancel(ctx)
	automationDone := make(chan struct{})
	go func() { defer close(automationDone); automationService.Run(automationCtx, logger) }()
	defer func() { stopAutomation(); <-automationDone }()
	printingService := batch.NewPrintingService(db, authorizer, storage, pdfgenerator.NewPoppler()).
		RegisterPrintGenerator("amazon", amazon.PrintGenerationVersion, amazon.NewPrintGenerator()).
		RegisterPrintGenerator("meesho", pdfgenerator.SourcePageGenerationVersion, pdfgenerator.NewSourcePages()).
		RegisterPrintGenerator("snapdeal", snapdeal.PrintGenerationVersion, snapdeal.NewPrintGenerator())
	batchHTTP := batch.NewHTTPHandler(printingService)
	marketplaceService, err := marketplace.NewService(db, authorizer, storage, pdfextractor.NewPoppler())
	if err != nil {
		return err
	}
	marketplaceHTTP := marketplace.NewHTTPHandler(marketplaceService)
	amazonService, err := marketplace.NewAmazonService(db, authorizer, storage, pdfextractor.NewPopplerWithOCR())
	if err != nil {
		return err
	}
	amazonHTTP := marketplace.NewHTTPHandler(amazonService)
	meeshoService, err := marketplace.NewMeeshoService(db, authorizer, storage, pdfextractor.NewPoppler())
	if err != nil {
		return err
	}
	meeshoHTTP := marketplace.NewHTTPHandler(meeshoService)
	myntraService, err := marketplace.NewMyntraService(db, authorizer, storage)
	if err != nil {
		return err
	}
	myntraHTTP := marketplace.NewHTTPHandler(myntraService)
	snapdealService, err := marketplace.NewSnapdealService(db, authorizer, storage, pdfextractor.NewPoppler())
	if err != nil {
		return err
	}
	snapdealHTTP := marketplace.NewHTTPHandler(snapdealService)
	mux.HandleFunc("/api/v1/auth/login", authHTTP.Login)
	mux.Handle("/api/v1/auth/logout", authHTTP.RequireSession(http.HandlerFunc(authHTTP.Logout)))
	mux.Handle("/api/v1/auth/session", authHTTP.RequireSession(http.HandlerFunc(authHTTP.Session)))
	mux.Handle("/api/v1/company", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Company)))
	mux.Handle("/api/v1/employees", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Employees)))
	mux.Handle("/api/v1/employees/{employee_id}", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Employee)))
	mux.Handle("/api/v1/user-access", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.UserAccesses)))
	mux.Handle("/api/v1/user-access/{user_id}", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.UserAccess)))
	mux.Handle("/api/v1/user-access/{user_id}/roles", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.UserRoles)))
	mux.Handle("/api/v1/roles", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Roles)))
	mux.Handle("/api/v1/roles/{role_id}/permissions", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.RolePermissions)))
	mux.Handle("/api/v1/permissions", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Permissions)))
	mux.Handle("/api/v1/module-entitlements", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Entitlements)))
	mux.Handle("/api/v1/module-entitlements/{module_key}", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.Entitlement)))
	mux.Handle("/api/v1/audit-logs", authHTTP.RequireSession(http.HandlerFunc(coreHTTP.AuditLogs)))
	mux.Handle("/api/v1/marketplaces", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Marketplaces)))
	mux.Handle("/api/v1/business-identities", authHTTP.RequireSession(http.HandlerFunc(marketplaceAccountHTTP.Identities)))
	mux.Handle("/api/v1/business-identities/{identity_id}", authHTTP.RequireSession(http.HandlerFunc(marketplaceAccountHTTP.Identity)))
	mux.Handle("/api/v1/marketplace-accounts", authHTTP.RequireSession(http.HandlerFunc(marketplaceAccountHTTP.Accounts)))
	mux.Handle("/api/v1/marketplace-accounts/{account_id}", authHTTP.RequireSession(http.HandlerFunc(marketplaceAccountHTTP.Account)))
	mux.Handle("/api/v1/products", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Products)))
	mux.Handle("/api/v1/products/{product_id}", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Product)))
	mux.Handle("/api/v1/products/{product_id}/department", authHTTP.RequireSession(http.HandlerFunc(productHTTP.ProductDepartment)))
	mux.Handle("/api/v1/product-department-assignments", authHTTP.RequireSession(http.HandlerFunc(productHTTP.DepartmentAssignments)))
	mux.Handle("/api/v1/product-departments", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Departments)))
	mux.Handle("/api/v1/sku-mappings", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Mappings)))
	mux.Handle("/api/v1/sku-mappings/resolve", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Resolve)))
	mux.Handle("/api/v1/sku-mappings/{mapping_id}", authHTTP.RequireSession(http.HandlerFunc(productHTTP.Mapping)))
	mux.Handle("/api/v1/flipkart/jobs", authHTTP.RequireSession(http.HandlerFunc(marketplaceHTTP.Jobs)))
	mux.Handle("/api/v1/flipkart/jobs/{job_id}", authHTTP.RequireSession(http.HandlerFunc(marketplaceHTTP.Job)))
	mux.Handle("/api/v1/amazon/jobs", authHTTP.RequireSession(http.HandlerFunc(amazonHTTP.Jobs)))
	mux.Handle("/api/v1/amazon/jobs/{job_id}", authHTTP.RequireSession(http.HandlerFunc(amazonHTTP.Job)))
	mux.Handle("/api/v1/meesho/jobs", authHTTP.RequireSession(http.HandlerFunc(meeshoHTTP.Jobs)))
	mux.Handle("/api/v1/meesho/jobs/{job_id}", authHTTP.RequireSession(http.HandlerFunc(meeshoHTTP.Job)))
	mux.Handle("/api/v1/myntra/jobs", authHTTP.RequireSession(http.HandlerFunc(myntraHTTP.Jobs)))
	mux.Handle("/api/v1/myntra/jobs/{job_id}", authHTTP.RequireSession(http.HandlerFunc(myntraHTTP.Job)))
	mux.Handle("/api/v1/snapdeal/jobs", authHTTP.RequireSession(http.HandlerFunc(snapdealHTTP.Jobs)))
	mux.Handle("/api/v1/snapdeal/jobs/{job_id}", authHTTP.RequireSession(http.HandlerFunc(snapdealHTTP.Job)))
	mux.Handle("/api/v1/batches", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.Batches)))
	mux.Handle("/api/v1/batches/{batch_id}", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.Batch)))
	mux.Handle("/api/v1/batches/{batch_id}/ready", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.Ready)))
	mux.Handle("/api/v1/batches/{batch_id}/cancel", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.Cancel)))
	mux.Handle("/api/v1/batch-eligible-orders", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.EligibleOrders)))
	mux.Handle("/api/v1/worker-assignment-rules", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.AssignmentRules)))
	mux.Handle("/api/v1/batches/{batch_id}/print-jobs", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.PrintJobs)))
	mux.Handle("/api/v1/print-jobs/{print_job_id}", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.PrintJob)))
	mux.Handle("/api/v1/print-jobs/{print_job_id}/reprints", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.Reprints)))
	mux.Handle("/api/v1/print-artifacts/{artifact_id}", authHTTP.RequireSession(http.HandlerFunc(batchHTTP.Artifact)))
	mux.Handle("/api/v1/printer-agents", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Agents)))
	mux.Handle("/api/v1/printer-agents/{agent_id}", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Agent)))
	mux.Handle("/api/v1/printers", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Printers)))
	mux.Handle("/api/v1/printers/{printer_id}", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Printer)))
	mux.Handle("/api/v1/print-library-assets", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Assets)))
	mux.Handle("/api/v1/print-library-assets/{asset_id}", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Asset)))
	mux.Handle("/api/v1/printer-jobs", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Jobs)))
	mux.Handle("/api/v1/print-artifacts/{artifact_id}/printer-jobs", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.ArtifactJobs)))
	mux.Handle("/api/v1/printer-jobs/{printer_job_id}/cancel", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Cancel)))
	mux.Handle("/api/v1/printer-jobs/{printer_job_id}/retry", authHTTP.RequireSession(http.HandlerFunc(printingHTTP.Retry)))
	mux.Handle("/api/v1/printer-agent/heartbeat", printingHTTP.RequireAgent(http.HandlerFunc(printingHTTP.AgentHeartbeat)))
	mux.Handle("/api/v1/printer-agent/jobs/claim", printingHTTP.RequireAgent(http.HandlerFunc(printingHTTP.AgentClaim)))
	mux.Handle("/api/v1/printer-agent/jobs/{printer_job_id}/artifact", printingHTTP.RequireAgent(http.HandlerFunc(printingHTTP.AgentArtifact)))
	mux.Handle("/api/v1/printer-agent/jobs/{printer_job_id}/status", printingHTTP.RequireAgent(http.HandlerFunc(printingHTTP.AgentReport)))
	mux.Handle("/api/v1/inventory", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.Balances)))
	mux.Handle("/api/v1/inventory/transactions", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.Transactions)))
	mux.Handle("/api/v1/inventory/stock-in", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.StockIn)))
	mux.Handle("/api/v1/inventory/adjustments", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.Adjust)))
	mux.Handle("/api/v1/inventory/corrections", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.Correct)))
	mux.Handle("/api/v1/inventory/batches/{batch_id}/confirm-outbound", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.EcommerceOutbound)))
	mux.Handle("/api/v1/inventory/reservations", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.Reservations)))
	mux.Handle("/api/v1/inventory/reservations/{reservation_id}/release", authHTTP.RequireSession(http.HandlerFunc(inventoryHTTP.ReleaseReservation)))
	mux.Handle("/api/v1/reports/dashboard", authHTTP.RequireSession(http.HandlerFunc(reportingHTTP.Dashboard)))
	mux.Handle("/api/v1/reports/operations-analytics", authHTTP.RequireSession(http.HandlerFunc(reportingHTTP.Analytics)))
	mux.Handle("/api/v1/cancellations", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Cancellations)))
	mux.Handle("/api/v1/cancellations/{cancellation_id}", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Cancellation)))
	mux.Handle("/api/v1/cancellations/{cancellation_id}/close", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.CloseCancellation)))
	mux.Handle("/api/v1/returns", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Returns)))
	mux.Handle("/api/v1/returns/{return_id}", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Return)))
	mux.Handle("/api/v1/returns/{return_id}/receive", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Receive)))
	mux.Handle("/api/v1/returns/{return_id}/inspect", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Inspect)))
	mux.Handle("/api/v1/returns/{return_id}/restock", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.Restock)))
	mux.Handle("/api/v1/returns/{return_id}/restock-corrections", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.CorrectRestock)))
	mux.Handle("/api/v1/returns/{return_id}/close", authHTTP.RequireSession(http.HandlerFunc(returnsHTTP.CloseReturn)))
	mux.Handle("/api/v1/consignment-departments", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Departments)))
	mux.Handle("/api/v1/consignment-departments/{department_id}", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Department)))
	mux.Handle("/api/v1/consignment-departments/{department_id}/members", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.DepartmentMembers)))
	mux.Handle("/api/v1/consignments", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Consignments)))
	mux.Handle("/api/v1/consignments/{consignment_id}", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Consignment)))
	mux.Handle("/api/v1/consignments/{consignment_id}/allocate", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Allocate)))
	mux.Handle("/api/v1/consignments/{consignment_id}/transition", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Transition)))
	mux.Handle("/api/v1/consignments/{consignment_id}/lines/{line_id}/progress", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Progress)))
	mux.Handle("/api/v1/consignments/{consignment_id}/confirm-outbound", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Outbound)))
	mux.Handle("/api/v1/consignments/{consignment_id}/cancel", authHTTP.RequireSession(http.HandlerFunc(consignmentHTTP.Cancel)))
	registerConsignmentTraceRoutes(mux, authHTTP, consignmentHTTP)
	registerTraceabilityRoutes(mux, authHTTP, traceabilityHTTP)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.Middleware(logger, cfg.AllowedOrigins, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		MaxHeaderBytes:    cfg.HTTPMaxHeaderBytes,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "address", cfg.HTTPAddr, "environment", cfg.Environment)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		logger.Info("http server shutting down")
		return server.Shutdown(shutdownCtx)
	}
}

func registerTraceabilityRoutes(mux *http.ServeMux, authHTTP *auth.HTTPHandler, traceabilityHTTP *traceability.HTTPHandler) {
	mux.Handle("/api/v1/trace-boxes", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.Boxes)))
	mux.Handle("/api/v1/trace-box-options", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.Options)))
	mux.Handle("/api/v1/trace-box-resolutions/{opaque_identifier}", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.Resolve)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.Box)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/contents", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.AddContent)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/contents/remove", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.RemoveContent)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/custody", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.TransferCustody)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/qc", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.RecordQC)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/work-requirements/{work_requirement_id}/complete", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.CompleteWork)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/handovers", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.SendHandover)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/handovers/{handover_id}/receive", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.ReceiveHandover)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/packing/complete", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.CompletePacking)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/final-checks", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.CompleteFinalCheck)))
	mux.Handle("/api/v1/trace-boxes/{trace_box_id}/shipment-readiness", authHTTP.RequireSession(http.HandlerFunc(traceabilityHTTP.MarkReady)))
}

func registerConsignmentTraceRoutes(mux *http.ServeMux, authHTTP *auth.HTTPHandler, handler *consignment.HTTPHandler) {
	mux.Handle("/api/v1/consignments/{consignment_id}/trace-box-links", authHTTP.RequireSession(http.HandlerFunc(handler.LinkTraceBox)))
	mux.Handle("/api/v1/consignments/{consignment_id}/trace-box-links/{allocation_event_id}/remove", authHTTP.RequireSession(http.HandlerFunc(handler.UnlinkTraceBox)))
	mux.Handle("/api/v1/consignments/{consignment_id}/trace-evidence", authHTTP.RequireSession(http.HandlerFunc(handler.RecordTraceEvidence)))
}

func newObjectStorage(ctx context.Context, cfg config.Config) (objectstorage.Storage, error) {
	switch cfg.ObjectStorageDriver {
	case "local":
		return objectstorage.NewLocal(cfg.FileStorageDir)
	case "s3":
		return objectstorage.NewS3(ctx, objectstorage.S3Options{
			Endpoint:  cfg.ObjectStorageEndpoint,
			Bucket:    cfg.ObjectStorageBucket,
			Region:    cfg.ObjectStorageRegion,
			AccessKey: cfg.ObjectStorageAccessKey,
			SecretKey: cfg.ObjectStorageSecretKey,
			PathStyle: cfg.ObjectStoragePathStyle,
		})
	default:
		return nil, fmt.Errorf("unsupported object storage driver %q", cfg.ObjectStorageDriver)
	}
}
