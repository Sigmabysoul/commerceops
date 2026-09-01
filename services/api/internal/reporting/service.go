package reporting

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/commerceops/commerceops/services/api/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidRange = errors.New("invalid reporting range")
	uuidRE          = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	marketplaceRE   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)
)

type Service struct {
	db         *pgxpool.Pool
	authorizer *authorization.Service
}

type Filter struct {
	From        time.Time
	To          time.Time
	Marketplace string
	ProductID   string
	Limit       int
	Offset      int
}

type Summary struct {
	OrdersProcessed      int64  `json:"orders_processed"`
	LabelsGenerated      int64  `json:"labels_generated"`
	PrintRunsCompleted   int64  `json:"print_runs_completed"`
	Batches              int64  `json:"batches"`
	OutboundOrders       *int64 `json:"outbound_confirmed_orders,omitempty"`
	UnresolvedRecords    int64  `json:"unresolved_records"`
	DuplicateRecords     int64  `json:"duplicate_records"`
	FailedProcessingJobs int64  `json:"failed_processing_jobs"`
}

type InventorySummary struct {
	CurrentOnHand    int64 `json:"current_on_hand"`
	CurrentReserved  int64 `json:"current_reserved"`
	CurrentAvailable int64 `json:"current_available"`
	StockIn          int64 `json:"stock_in"`
	StockOut         int64 `json:"stock_out"`
	ConsignmentOut   int64 `json:"consignment_out"`
	ReturnRestock    int64 `json:"return_restock"`
	Adjustments      int64 `json:"adjustments"`
	NetMovement      int64 `json:"net_movement"`
}

type ReturnsSummary struct {
	Cancellations           int64   `json:"cancellations"`
	ReturnsReceived         int64   `json:"returns_received"`
	ReceivedQuantity        int64   `json:"received_quantity"`
	RestockedQuantity       int64   `json:"restocked_quantity"`
	DamagedQuantity         int64   `json:"damaged_quantity"`
	ClosedReturns           int64   `json:"closed_returns"`
	ClosedCancellations     int64   `json:"closed_cancellations"`
	CohortReturnedOrders    int64   `json:"cohort_returned_orders"`
	CohortResolvedOrders    int64   `json:"cohort_resolved_orders"`
	CohortReturnRatePercent float64 `json:"cohort_return_rate_percent"`
}

type MarketplaceBreakdown struct {
	Marketplace string `json:"marketplace"`
	Orders      int64  `json:"orders"`
	Resolved    int64  `json:"resolved"`
	NeedsReview int64  `json:"needs_review"`
	Duplicates  int64  `json:"duplicates"`
}

type ProductMovement struct {
	ProductID      string `json:"product_id"`
	InternalCode   string `json:"internal_code"`
	ProductName    string `json:"product_name"`
	OrderQuantity  int64  `json:"order_quantity"`
	StockIn        int64  `json:"stock_in"`
	StockOut       int64  `json:"stock_out"`
	ConsignmentOut int64  `json:"consignment_out"`
	ReturnRestock  int64  `json:"return_restock"`
	Adjustments    int64  `json:"adjustments"`
	NetMovement    int64  `json:"net_movement"`
}
type ConsignmentProductQuantity struct {
	ProductID        string `json:"product_id"`
	InternalCode     string `json:"internal_code"`
	ProductName      string `json:"product_name"`
	RequiredQuantity int64  `json:"required_quantity"`
}
type DepartmentWorkload struct {
	DepartmentID        string `json:"department_id"`
	DepartmentName      string `json:"department_name"`
	PendingConsignments int64  `json:"pending_consignments"`
	OutstandingQuantity int64  `json:"outstanding_quantity"`
}
type ConsignmentSummary struct {
	Pending                int64                        `json:"pending"`
	Completed              int64                        `json:"completed"`
	AverageCompletionHours float64                      `json:"average_completion_hours"`
	InventoryOut           int64                        `json:"inventory_out"`
	Products               []ConsignmentProductQuantity `json:"products"`
	Departments            []DepartmentWorkload         `json:"departments"`
}
type ProductQuantity struct {
	ProductID    string `json:"product_id"`
	InternalCode string `json:"internal_code"`
	ProductName  string `json:"product_name"`
	Quantity     int64  `json:"quantity"`
}

type Report struct {
	From              time.Time              `json:"from"`
	To                time.Time              `json:"to"`
	Marketplace       string                 `json:"marketplace,omitempty"`
	Summary           Summary                `json:"summary"`
	Marketplaces      []MarketplaceBreakdown `json:"marketplaces"`
	InventoryAccess   bool                   `json:"inventory_access"`
	Inventory         *InventorySummary      `json:"inventory,omitempty"`
	ReturnsAccess     bool                   `json:"returns_access"`
	Returns           *ReturnsSummary        `json:"returns,omitempty"`
	ConsignmentAccess bool                   `json:"consignment_access"`
	Consignment       *ConsignmentSummary    `json:"consignment,omitempty"`
	ProductMovements  []ProductMovement      `json:"product_movements"`
	MovementTotal     int64                  `json:"product_movement_total"`
	ProductQuantities []ProductQuantity      `json:"product_quantities"`
}

func NewService(db *pgxpool.Pool, authorizer *authorization.Service) *Service {
	return &Service{db: db, authorizer: authorizer}
}

func (s *Service) Dashboard(ctx context.Context, p auth.Principal, f Filter) (Report, error) {
	if err := s.authorizer.RequirePermission(ctx, p, "reports.view"); err != nil {
		return Report{}, err
	}
	f.Marketplace, f.ProductID = strings.TrimSpace(f.Marketplace), strings.TrimSpace(f.ProductID)
	if f.From.IsZero() || f.To.IsZero() || !f.From.Before(f.To) || f.To.Sub(f.From) > 366*24*time.Hour ||
		(f.Marketplace != "" && !marketplaceRE.MatchString(f.Marketplace)) || (f.ProductID != "" && !uuidRE.MatchString(f.ProductID)) ||
		f.Limit < 1 || f.Limit > 200 || f.Offset < 0 {
		return Report{}, ErrInvalidRange
	}
	r := Report{From: f.From, To: f.To, Marketplace: f.Marketplace, Marketplaces: make([]MarketplaceBreakdown, 0), ProductMovements: make([]ProductMovement, 0), ProductQuantities: make([]ProductQuantity, 0)}
	err := s.db.QueryRow(ctx, `
		SELECT
		 (SELECT count(*) FROM marketplace_orders o WHERE o.company_id=$1 AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
		 (SELECT COALESCE(sum(a.page_count),0) FROM print_artifacts a JOIN print_jobs j ON j.company_id=a.company_id AND j.id=a.print_job_id JOIN batches b ON b.company_id=j.company_id AND b.id=j.batch_id WHERE a.company_id=$1 AND a.kind='labels' AND j.status='ready' AND j.completed_at >= $2 AND j.completed_at < $3 AND ($4='' OR b.marketplace_key=$4)),
		 (SELECT count(*) FROM print_jobs j JOIN batches b ON b.company_id=j.company_id AND b.id=j.batch_id WHERE j.company_id=$1 AND j.status='ready' AND j.completed_at >= $2 AND j.completed_at < $3 AND ($4='' OR b.marketplace_key=$4)),
		 (SELECT count(*) FROM batches b WHERE b.company_id=$1 AND b.created_at >= $2 AND b.created_at < $3 AND ($4='' OR b.marketplace_key=$4)),
		 (SELECT count(*) FROM marketplace_orders o WHERE o.company_id=$1 AND o.status='needs_review' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
		 (SELECT count(*) FROM marketplace_orders o WHERE o.company_id=$1 AND o.status='duplicate' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
		 (SELECT count(*) FROM processing_jobs j WHERE j.company_id=$1 AND j.status='failed' AND j.created_at >= $2 AND j.created_at < $3 AND ($4='' OR j.marketplace_key=$4))`,
		p.CompanyID, f.From, f.To, f.Marketplace).Scan(&r.Summary.OrdersProcessed, &r.Summary.LabelsGenerated, &r.Summary.PrintRunsCompleted, &r.Summary.Batches, &r.Summary.UnresolvedRecords, &r.Summary.DuplicateRecords, &r.Summary.FailedProcessingJobs)
	if err != nil {
		return Report{}, err
	}

	rows, err := s.db.Query(ctx, `SELECT marketplace_key,count(*),count(*) FILTER (WHERE status='resolved'),count(*) FILTER (WHERE status='needs_review'),count(*) FILTER (WHERE status='duplicate') FROM marketplace_orders WHERE company_id=$1 AND created_at >= $2 AND created_at < $3 AND ($4='' OR marketplace_key=$4) GROUP BY marketplace_key ORDER BY marketplace_key`, p.CompanyID, f.From, f.To, f.Marketplace)
	if err != nil {
		return Report{}, err
	}
	for rows.Next() {
		var item MarketplaceBreakdown
		if err = rows.Scan(&item.Marketplace, &item.Orders, &item.Resolved, &item.NeedsReview, &item.Duplicates); err != nil {
			rows.Close()
			return Report{}, err
		}
		r.Marketplaces = append(r.Marketplaces, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return Report{}, err
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT p.id,p.internal_code,p.name,sum(oi.quantity) FROM marketplace_order_items oi JOIN marketplace_orders o ON o.company_id=oi.company_id AND o.id=oi.order_id JOIN products p ON p.company_id=oi.company_id AND p.id=oi.product_id WHERE oi.company_id=$1 AND o.status='resolved' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4) AND ($5='' OR p.id::text=$5) GROUP BY p.id,p.internal_code,p.name ORDER BY p.name,p.internal_code,p.id LIMIT 200`, p.CompanyID, f.From, f.To, f.Marketplace, f.ProductID)
	if err != nil {
		return Report{}, err
	}
	for rows.Next() {
		var item ProductQuantity
		if err = rows.Scan(&item.ProductID, &item.InternalCode, &item.ProductName, &item.Quantity); err != nil {
			rows.Close()
			return Report{}, err
		}
		r.ProductQuantities = append(r.ProductQuantities, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return Report{}, err
	}
	rows.Close()
	if err = s.loadConsignmentSummary(ctx, p, f, &r); err != nil {
		return Report{}, err
	}

	returnsModuleErr := s.authorizer.RequireModule(ctx, p, "returns")
	returnsPermissionErr := s.authorizer.RequirePermission(ctx, p, "returns.view")
	if returnsModuleErr == nil && returnsPermissionErr == nil {
		r.ReturnsAccess = true
		r.Returns = &ReturnsSummary{}
		err = s.db.QueryRow(ctx, `
			SELECT
			 (SELECT count(*) FROM cancellations c JOIN marketplace_orders o ON o.company_id=c.company_id AND o.id=c.marketplace_order_id WHERE c.company_id=$1 AND c.cancelled_at >= $2 AND c.cancelled_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT count(*) FROM return_events e JOIN return_cases rc ON rc.company_id=e.company_id AND rc.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=rc.company_id AND o.id=rc.marketplace_order_id WHERE e.company_id=$1 AND e.event_type='received' AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT COALESCE(sum(ri.received_quantity),0) FROM return_events e JOIN return_cases rc ON rc.company_id=e.company_id AND rc.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=rc.company_id AND o.id=rc.marketplace_order_id JOIN return_items ri ON ri.company_id=rc.company_id AND ri.return_case_id=rc.id WHERE e.company_id=$1 AND e.event_type='received' AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT COALESCE(sum(ri.restocked_quantity),0) FROM return_events e JOIN return_cases rc ON rc.company_id=e.company_id AND rc.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=rc.company_id AND o.id=rc.marketplace_order_id JOIN return_items ri ON ri.company_id=rc.company_id AND ri.return_case_id=rc.id WHERE e.company_id=$1 AND e.event_type='restocked' AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT COALESCE(sum(ri.received_quantity),0) FROM return_events e JOIN return_cases rc ON rc.company_id=e.company_id AND rc.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=rc.company_id AND o.id=rc.marketplace_order_id JOIN return_items ri ON ri.company_id=rc.company_id AND ri.return_case_id=rc.id WHERE e.company_id=$1 AND e.event_type='inspected' AND ri.disposition='damaged' AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT count(*) FROM return_events e JOIN return_cases rc ON rc.company_id=e.company_id AND rc.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=rc.company_id AND o.id=rc.marketplace_order_id WHERE e.company_id=$1 AND e.event_type='closed' AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT count(*) FROM cancellation_events e JOIN cancellations c ON c.company_id=e.company_id AND c.id=e.cancellation_id JOIN marketplace_orders o ON o.company_id=c.company_id AND o.id=c.marketplace_order_id WHERE e.company_id=$1 AND e.event_type='closed' AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR o.marketplace_key=$4)),
			 (SELECT count(*) FROM marketplace_orders o WHERE o.company_id=$1 AND o.status='resolved' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4) AND EXISTS(SELECT 1 FROM return_cases rc JOIN return_events e ON e.company_id=rc.company_id AND e.return_case_id=rc.id AND e.event_type='received' WHERE rc.company_id=o.company_id AND rc.marketplace_order_id=o.id AND EXISTS(SELECT 1 FROM return_items ri WHERE ri.company_id=rc.company_id AND ri.return_case_id=rc.id AND ri.received_quantity>0))),
			 (SELECT count(*) FROM marketplace_orders o WHERE o.company_id=$1 AND o.status='resolved' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4))`,
			p.CompanyID, f.From, f.To, f.Marketplace).Scan(
			&r.Returns.Cancellations,
			&r.Returns.ReturnsReceived,
			&r.Returns.ReceivedQuantity,
			&r.Returns.RestockedQuantity,
			&r.Returns.DamagedQuantity,
			&r.Returns.ClosedReturns,
			&r.Returns.ClosedCancellations,
			&r.Returns.CohortReturnedOrders,
			&r.Returns.CohortResolvedOrders,
		)
		if err != nil {
			return Report{}, err
		}
		if r.Returns.CohortResolvedOrders > 0 {
			r.Returns.CohortReturnRatePercent = float64(r.Returns.CohortReturnedOrders) * 100 / float64(r.Returns.CohortResolvedOrders)
		}
	} else if !errors.Is(returnsModuleErr, authorization.ErrModuleUnavailable) && returnsModuleErr != nil {
		return Report{}, returnsModuleErr
	} else if !errors.Is(returnsPermissionErr, authorization.ErrPermissionDenied) && returnsPermissionErr != nil {
		return Report{}, returnsPermissionErr
	}

	moduleErr := s.authorizer.RequireModule(ctx, p, "inventory")
	permissionErr := s.authorizer.RequirePermission(ctx, p, "inventory.view")
	if moduleErr == nil && permissionErr == nil {
		r.InventoryAccess = true
		r.Inventory = &InventorySummary{}
		var outbound int64
		err = s.db.QueryRow(ctx, `SELECT count(*) FROM inventory_outbound_events e JOIN batches b ON b.company_id=e.company_id AND b.id=e.batch_id JOIN batch_members bm ON bm.company_id=b.company_id AND bm.batch_id=b.id WHERE e.company_id=$1 AND e.created_at >= $2 AND e.created_at < $3 AND ($4='' OR b.marketplace_key=$4)`, p.CompanyID, f.From, f.To, f.Marketplace).Scan(&outbound)
		if err != nil {
			return Report{}, err
		}
		r.Summary.OutboundOrders = &outbound
		err = s.db.QueryRow(ctx, `SELECT COALESCE(sum(on_hand),0),COALESCE(sum(reserved),0),COALESCE(sum(on_hand-reserved),0) FROM inventory_balances WHERE company_id=$1 AND ($2='' OR product_id::text=$2)`, p.CompanyID, f.ProductID).Scan(&r.Inventory.CurrentOnHand, &r.Inventory.CurrentReserved, &r.Inventory.CurrentAvailable)
		if err != nil {
			return Report{}, err
		}
		err = s.db.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta) FILTER(WHERE transaction_type='stock_in'),0),COALESCE(-sum(quantity_delta) FILTER(WHERE transaction_type='ecommerce_out'),0),COALESCE(-sum(quantity_delta) FILTER(WHERE transaction_type='consignment_out'),0),COALESCE(sum(quantity_delta) FILTER(WHERE transaction_type='return_restock' OR (transaction_type='correction' AND reference_type='return_restock_correction')),0),COALESCE(sum(quantity_delta) FILTER(WHERE transaction_type='manual_adjustment' OR (transaction_type='correction' AND reference_type IS DISTINCT FROM 'return_restock_correction')),0),COALESCE(sum(quantity_delta),0) FROM inventory_transactions i WHERE i.company_id=$1 AND i.created_at >= $2 AND i.created_at < $3 AND ($4='' OR i.product_id::text=$4) AND ($5='' OR (i.transaction_type='ecommerce_out' AND EXISTS(SELECT 1 FROM batches b WHERE b.company_id=i.company_id AND b.id::text=i.reference_id AND i.reference_type='batch' AND b.marketplace_key=$5)) OR (i.transaction_type='return_restock' AND EXISTS(SELECT 1 FROM return_cases r JOIN marketplace_orders o ON o.company_id=r.company_id AND o.id=r.marketplace_order_id WHERE r.company_id=i.company_id AND r.id::text=i.reference_id AND i.reference_type='return_case' AND o.marketplace_key=$5)) OR (i.transaction_type='correction' AND i.reference_type='return_restock_correction' AND EXISTS(SELECT 1 FROM return_events e JOIN return_cases r ON r.company_id=e.company_id AND r.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=r.company_id AND o.id=r.marketplace_order_id WHERE e.company_id=i.company_id AND e.id::text=i.reference_id AND o.marketplace_key=$5)) OR (i.transaction_type NOT IN ('ecommerce_out','return_restock') AND NOT (i.transaction_type='correction' AND i.reference_type='return_restock_correction')))`, p.CompanyID, f.From, f.To, f.ProductID, f.Marketplace).Scan(&r.Inventory.StockIn, &r.Inventory.StockOut, &r.Inventory.ConsignmentOut, &r.Inventory.ReturnRestock, &r.Inventory.Adjustments, &r.Inventory.NetMovement)
		if err != nil {
			return Report{}, err
		}
		if err = s.loadProductMovements(ctx, p, f, &r); err != nil {
			return Report{}, err
		}
	} else if !errors.Is(moduleErr, authorization.ErrModuleUnavailable) && moduleErr != nil {
		return Report{}, moduleErr
	} else if !errors.Is(permissionErr, authorization.ErrPermissionDenied) && permissionErr != nil {
		return Report{}, permissionErr
	}
	return r, nil
}

func (s *Service) loadProductMovements(ctx context.Context, p auth.Principal, f Filter, r *Report) error {
	base := ` FROM products p LEFT JOIN LATERAL (SELECT COALESCE(sum(i.quantity_delta) FILTER(WHERE i.transaction_type='stock_in'),0) stock_in,COALESCE(-sum(i.quantity_delta) FILTER(WHERE i.transaction_type='ecommerce_out'),0) stock_out,COALESCE(-sum(i.quantity_delta) FILTER(WHERE i.transaction_type='consignment_out'),0) consignment_out,COALESCE(sum(i.quantity_delta) FILTER(WHERE i.transaction_type='return_restock' OR (i.transaction_type='correction' AND i.reference_type='return_restock_correction')),0) return_restock,COALESCE(sum(i.quantity_delta) FILTER(WHERE i.transaction_type='manual_adjustment' OR (i.transaction_type='correction' AND i.reference_type IS DISTINCT FROM 'return_restock_correction')),0) adjustments,COALESCE(sum(i.quantity_delta),0) net FROM inventory_transactions i WHERE i.company_id=p.company_id AND i.product_id=p.id AND i.created_at >= $2 AND i.created_at < $3 AND ($4='' OR (i.transaction_type='ecommerce_out' AND EXISTS(SELECT 1 FROM batches b WHERE b.company_id=i.company_id AND b.id::text=i.reference_id AND i.reference_type='batch' AND b.marketplace_key=$4)) OR (i.transaction_type='return_restock' AND EXISTS(SELECT 1 FROM return_cases r JOIN marketplace_orders o ON o.company_id=r.company_id AND o.id=r.marketplace_order_id WHERE r.company_id=i.company_id AND r.id::text=i.reference_id AND i.reference_type='return_case' AND o.marketplace_key=$4)) OR (i.transaction_type='correction' AND i.reference_type='return_restock_correction' AND EXISTS(SELECT 1 FROM return_events e JOIN return_cases r ON r.company_id=e.company_id AND r.id=e.return_case_id JOIN marketplace_orders o ON o.company_id=r.company_id AND o.id=r.marketplace_order_id WHERE e.company_id=i.company_id AND e.id::text=i.reference_id AND o.marketplace_key=$4)) OR (i.transaction_type NOT IN ('ecommerce_out','return_restock') AND NOT (i.transaction_type='correction' AND i.reference_type='return_restock_correction')))) m ON true LEFT JOIN LATERAL (SELECT COALESCE(sum(oi.quantity),0) quantity FROM marketplace_order_items oi JOIN marketplace_orders o ON o.company_id=oi.company_id AND o.id=oi.order_id WHERE oi.company_id=p.company_id AND oi.product_id=p.id AND o.status='resolved' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4)) q ON true WHERE p.company_id=$1 AND ($5='' OR p.id::text=$5) AND (m.net<>0 OR q.quantity<>0)`
	if err := s.db.QueryRow(ctx, `SELECT count(*)`+base, p.CompanyID, f.From, f.To, f.Marketplace, f.ProductID).Scan(&r.MovementTotal); err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT p.id,p.internal_code,p.name,q.quantity,m.stock_in,m.stock_out,m.consignment_out,m.return_restock,m.adjustments,m.net`+base+` ORDER BY p.name,p.internal_code,p.id LIMIT $6 OFFSET $7`, p.CompanyID, f.From, f.To, f.Marketplace, f.ProductID, f.Limit, f.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProductMovement
		if err = rows.Scan(&item.ProductID, &item.InternalCode, &item.ProductName, &item.OrderQuantity, &item.StockIn, &item.StockOut, &item.ConsignmentOut, &item.ReturnRestock, &item.Adjustments, &item.NetMovement); err != nil {
			return err
		}
		r.ProductMovements = append(r.ProductMovements, item)
	}
	return rows.Err()
}

func (s *Service) loadConsignmentSummary(ctx context.Context, p auth.Principal, f Filter, r *Report) error {
	moduleErr := s.authorizer.RequireModule(ctx, p, "consignments")
	if moduleErr != nil {
		if errors.Is(moduleErr, authorization.ErrModuleUnavailable) {
			return nil
		}
		return moduleErr
	}
	broad := s.authorizer.RequirePermission(ctx, p, "consignments.view") == nil || s.authorizer.RequirePermission(ctx, p, "consignments.manage") == nil
	if !broad {
		if err := s.authorizer.RequirePermission(ctx, p, "consignments.work"); err != nil {
			if errors.Is(err, authorization.ErrPermissionDenied) {
				return nil
			}
			return err
		}
	}
	r.ConsignmentAccess = true
	r.Consignment = &ConsignmentSummary{Products: []ConsignmentProductQuantity{}, Departments: []DepartmentWorkload{}}
	access := `($4 OR EXISTS(SELECT 1 FROM consignment_lines al JOIN consignment_department_members dm ON dm.company_id=al.company_id AND dm.department_id=al.department_id JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE al.company_id=c.company_id AND al.consignment_id=c.id AND e.user_id=$5))`
	err := s.db.QueryRow(ctx, `SELECT
		count(*) FILTER(WHERE c.status NOT IN ('completed','cancelled')),
		count(*) FILTER(WHERE c.status='completed' AND c.completed_at >= $2 AND c.completed_at < $3),
		COALESCE(avg(EXTRACT(EPOCH FROM (c.completed_at-c.created_at))/3600) FILTER(WHERE c.status='completed' AND c.completed_at >= $2 AND c.completed_at < $3),0)
		FROM consignments c WHERE c.company_id=$1 AND `+access, p.CompanyID, f.From, f.To, broad, p.UserID).Scan(&r.Consignment.Pending, &r.Consignment.Completed, &r.Consignment.AverageCompletionHours)
	if err != nil {
		return err
	}
	err = s.db.QueryRow(ctx, `SELECT COALESCE(-sum(i.quantity_delta),0) FROM inventory_transactions i JOIN consignments c ON c.company_id=i.company_id AND c.id::text=i.reference_id AND i.reference_type='consignment' WHERE i.company_id=$1 AND i.transaction_type='consignment_out' AND i.created_at >= $2 AND i.created_at < $3 AND ($6='' OR i.product_id::text=$6) AND $4 AND `+access, p.CompanyID, f.From, f.To, broad, p.UserID, f.ProductID).Scan(&r.Consignment.InventoryOut)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT p.id,p.internal_code,p.name,sum(l.required_quantity) FROM consignment_lines l JOIN consignments c ON c.company_id=l.company_id AND c.id=l.consignment_id JOIN products p ON p.company_id=l.company_id AND p.id=l.product_id WHERE l.company_id=$1 AND c.created_at >= $2 AND c.created_at < $3 AND ($6='' OR p.id::text=$6) AND ($4 OR EXISTS(SELECT 1 FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=l.company_id AND dm.department_id=l.department_id AND e.user_id=$5)) GROUP BY p.id,p.internal_code,p.name ORDER BY p.name,p.internal_code,p.id`, p.CompanyID, f.From, f.To, broad, p.UserID, f.ProductID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var item ConsignmentProductQuantity
		if err = rows.Scan(&item.ProductID, &item.InternalCode, &item.ProductName, &item.RequiredQuantity); err != nil {
			rows.Close()
			return err
		}
		r.Consignment.Products = append(r.Consignment.Products, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT d.id,d.name,count(DISTINCT c.id),sum(l.required_quantity-l.packed_quantity) FROM consignment_lines l JOIN consignments c ON c.company_id=l.company_id AND c.id=l.consignment_id JOIN consignment_departments d ON d.company_id=l.company_id AND d.id=l.department_id WHERE l.company_id=$1 AND c.status NOT IN ('completed','cancelled') AND ($2 OR EXISTS(SELECT 1 FROM consignment_department_members dm JOIN employees e ON e.company_id=dm.company_id AND e.id=dm.employee_id WHERE dm.company_id=l.company_id AND dm.department_id=l.department_id AND e.user_id=$3)) GROUP BY d.id,d.name ORDER BY d.name,d.id`, p.CompanyID, broad, p.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item DepartmentWorkload
		if err = rows.Scan(&item.DepartmentID, &item.DepartmentName, &item.PendingConsignments, &item.OutstandingQuantity); err != nil {
			return err
		}
		r.Consignment.Departments = append(r.Consignment.Departments, item)
	}
	return rows.Err()
}
