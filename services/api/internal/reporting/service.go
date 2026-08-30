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
	Adjustments      int64 `json:"adjustments"`
	NetMovement      int64 `json:"net_movement"`
}

type MarketplaceBreakdown struct {
	Marketplace string `json:"marketplace"`
	Orders      int64  `json:"orders"`
	Resolved    int64  `json:"resolved"`
	NeedsReview int64  `json:"needs_review"`
	Duplicates  int64  `json:"duplicates"`
}

type ProductMovement struct {
	ProductID     string `json:"product_id"`
	InternalCode  string `json:"internal_code"`
	ProductName   string `json:"product_name"`
	OrderQuantity int64  `json:"order_quantity"`
	StockIn       int64  `json:"stock_in"`
	StockOut      int64  `json:"stock_out"`
	Adjustments   int64  `json:"adjustments"`
	NetMovement   int64  `json:"net_movement"`
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
		err = s.db.QueryRow(ctx, `SELECT COALESCE(sum(quantity_delta) FILTER(WHERE transaction_type='stock_in'),0),COALESCE(-sum(quantity_delta) FILTER(WHERE transaction_type='ecommerce_out'),0),COALESCE(sum(quantity_delta) FILTER(WHERE transaction_type IN('manual_adjustment','correction')),0),COALESCE(sum(quantity_delta),0) FROM inventory_transactions WHERE company_id=$1 AND created_at >= $2 AND created_at < $3 AND ($4='' OR product_id::text=$4)`, p.CompanyID, f.From, f.To, f.ProductID).Scan(&r.Inventory.StockIn, &r.Inventory.StockOut, &r.Inventory.Adjustments, &r.Inventory.NetMovement)
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
	base := ` FROM products p LEFT JOIN LATERAL (SELECT COALESCE(sum(i.quantity_delta) FILTER(WHERE i.transaction_type='stock_in'),0) stock_in,COALESCE(-sum(i.quantity_delta) FILTER(WHERE i.transaction_type='ecommerce_out'),0) stock_out,COALESCE(sum(i.quantity_delta) FILTER(WHERE i.transaction_type IN('manual_adjustment','correction')),0) adjustments,COALESCE(sum(i.quantity_delta),0) net FROM inventory_transactions i WHERE i.company_id=p.company_id AND i.product_id=p.id AND i.created_at >= $2 AND i.created_at < $3) m ON true LEFT JOIN LATERAL (SELECT COALESCE(sum(oi.quantity),0) quantity FROM marketplace_order_items oi JOIN marketplace_orders o ON o.company_id=oi.company_id AND o.id=oi.order_id WHERE oi.company_id=p.company_id AND oi.product_id=p.id AND o.status='resolved' AND o.created_at >= $2 AND o.created_at < $3 AND ($4='' OR o.marketplace_key=$4)) q ON true WHERE p.company_id=$1 AND ($5='' OR p.id::text=$5) AND (m.net<>0 OR q.quantity<>0)`
	if err := s.db.QueryRow(ctx, `SELECT count(*)`+base, p.CompanyID, f.From, f.To, f.Marketplace, f.ProductID).Scan(&r.MovementTotal); err != nil {
		return err
	}
	rows, err := s.db.Query(ctx, `SELECT p.id,p.internal_code,p.name,q.quantity,m.stock_in,m.stock_out,m.adjustments,m.net`+base+` ORDER BY p.name,p.internal_code,p.id LIMIT $6 OFFSET $7`, p.CompanyID, f.From, f.To, f.Marketplace, f.ProductID, f.Limit, f.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ProductMovement
		if err = rows.Scan(&item.ProductID, &item.InternalCode, &item.ProductName, &item.OrderQuantity, &item.StockIn, &item.StockOut, &item.Adjustments, &item.NetMovement); err != nil {
			return err
		}
		r.ProductMovements = append(r.ProductMovements, item)
	}
	return rows.Err()
}
