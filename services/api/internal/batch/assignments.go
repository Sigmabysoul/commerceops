package batch

import (
	"context"
	"strings"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

type AssignmentRule struct {
	ID             string  `json:"id"`
	MarketplaceKey string  `json:"marketplace_key"`
	ProductID      *string `json:"product_id"`
	ProductCode    *string `json:"product_code"`
	ProductName    *string `json:"product_name"`
	EmployeeID     string  `json:"employee_id"`
	EmployeeName   string  `json:"employee_name"`
	Priority       int     `json:"priority"`
}

type AssignmentRuleInput struct {
	ProductID  *string `json:"product_id"`
	EmployeeID string  `json:"employee_id"`
	Priority   int     `json:"priority"`
}

type ReplaceAssignmentRulesInput struct {
	MarketplaceKey string                `json:"marketplace_key"`
	Rules          []AssignmentRuleInput `json:"rules"`
}

type WorkerTotal struct {
	EmployeeID     string `json:"employee_id"`
	EmployeeName   string `json:"employee_name"`
	TotalQuantity  int    `json:"total_quantity"`
	OrderLineCount int    `json:"order_line_count"`
	ProductCount   int    `json:"product_count"`
}

func (s *Service) ListAssignmentRules(ctx context.Context, principal auth.Principal, marketplace string) ([]AssignmentRule, error) {
	if err := s.authorize(ctx, principal); err != nil {
		return nil, err
	}
	if strings.TrimSpace(marketplace) != "flipkart" {
		return nil, ErrInvalidInput
	}
	return s.assignmentRules(ctx, principal.CompanyID, marketplace)
}

func (s *Service) ReplaceAssignmentRules(ctx context.Context, principal auth.Principal, input ReplaceAssignmentRulesInput) ([]AssignmentRule, error) {
	if err := s.authorizer.RequireModule(ctx, principal, "flipkart"); err != nil {
		return nil, err
	}
	if err := s.authorizer.RequirePermission(ctx, principal, "employees.manage"); err != nil {
		return nil, err
	}
	input.MarketplaceKey = strings.TrimSpace(input.MarketplaceKey)
	if input.MarketplaceKey != "flipkart" || len(input.Rules) == 0 || len(input.Rules) > 1000 {
		return nil, ErrInvalidInput
	}
	seen, defaults := make(map[string]struct{}, len(input.Rules)), 0
	for index := range input.Rules {
		rule := &input.Rules[index]
		rule.EmployeeID = strings.TrimSpace(rule.EmployeeID)
		if !uuidRE.MatchString(rule.EmployeeID) || rule.Priority < 0 || rule.Priority > 10000 {
			return nil, ErrInvalidInput
		}
		key := "default"
		if rule.ProductID == nil {
			defaults++
		} else {
			value := strings.TrimSpace(*rule.ProductID)
			rule.ProductID = &value
			key = value
			if !uuidRE.MatchString(value) {
				return nil, ErrInvalidInput
			}
		}
		if _, exists := seen[key]; exists {
			return nil, ErrInvalidInput
		}
		seen[key] = struct{}{}
	}
	if defaults != 1 {
		return nil, ErrInvalidInput
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `UPDATE worker_assignment_rules SET status='inactive',updated_at=now() WHERE company_id=$1 AND marketplace_key=$2 AND status='active'`, principal.CompanyID, input.MarketplaceKey); err != nil {
		return nil, err
	}
	for _, rule := range input.Rules {
		if _, err = tx.Exec(ctx, `INSERT INTO worker_assignment_rules(company_id,marketplace_key,product_id,employee_id,priority) SELECT $1,$2,$3,$4,$5 WHERE EXISTS(SELECT 1 FROM employees WHERE company_id=$1 AND id=$4 AND status='active') AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM products WHERE company_id=$1 AND id=$3 AND status='active'))`, principal.CompanyID, input.MarketplaceKey, rule.ProductID, rule.EmployeeID, rule.Priority); err != nil {
			return nil, mapDBError(err)
		}
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM worker_assignment_rules WHERE company_id=$1 AND marketplace_key=$2 AND status='active'`, principal.CompanyID, input.MarketplaceKey).Scan(&count); err != nil || count != len(input.Rules) {
		return nil, ErrInvalidInput
	}
	if err = s.audit.Record(ctx, tx, principal.CompanyID, principal.UserID, "worker_assignments.replaced", "marketplace", input.MarketplaceKey, map[string]any{"rule_count": count}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.assignmentRules(ctx, principal.CompanyID, input.MarketplaceKey)
}

func (s *Service) assignmentRules(ctx context.Context, companyID, marketplace string) ([]AssignmentRule, error) {
	rows, err := s.db.Query(ctx, `SELECT r.id,r.marketplace_key,r.product_id,p.internal_code,p.name,r.employee_id,e.display_name,r.priority FROM worker_assignment_rules r LEFT JOIN products p ON p.company_id=r.company_id AND p.id=r.product_id JOIN employees e ON e.company_id=r.company_id AND e.id=r.employee_id WHERE r.company_id=$1 AND r.marketplace_key=$2 AND r.status='active' ORDER BY r.product_id IS NULL,r.priority,p.internal_code,r.id`, companyID, marketplace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AssignmentRule, 0)
	for rows.Next() {
		var item AssignmentRule
		if err = rows.Scan(&item.ID, &item.MarketplaceKey, &item.ProductID, &item.ProductCode, &item.ProductName, &item.EmployeeID, &item.EmployeeName, &item.Priority); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func snapshotAssignments(ctx context.Context, tx pgx.Tx, companyID, batchID, marketplace string) error {
	var missing int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT moi.product_id FROM batch_members bm JOIN marketplace_order_items moi ON moi.company_id=bm.company_id AND moi.order_id=bm.marketplace_order_id WHERE bm.company_id=$1 AND bm.batch_id=$2 AND moi.product_id IS NOT NULL GROUP BY moi.product_id) totals WHERE NOT EXISTS(SELECT 1 FROM worker_assignment_rules r JOIN employees e ON e.company_id=r.company_id AND e.id=r.employee_id AND e.status='active' WHERE r.company_id=$1 AND r.marketplace_key=$3 AND r.status='active' AND (r.product_id=totals.product_id OR r.product_id IS NULL))`, companyID, batchID, marketplace).Scan(&missing)
	if err != nil {
		return err
	}
	if missing > 0 {
		return ErrAssignmentMissing
	}
	_, err = tx.Exec(ctx, `INSERT INTO batch_worker_assignments(company_id,batch_id,product_id,employee_id,assignment_rule_id,total_quantity,order_line_count) SELECT $1,$2,t.product_id,rule.employee_id,rule.id,t.total_quantity,t.order_line_count FROM (SELECT moi.product_id,sum(moi.quantity)::integer total_quantity,count(moi.id)::integer order_line_count FROM batch_members bm JOIN marketplace_order_items moi ON moi.company_id=bm.company_id AND moi.order_id=bm.marketplace_order_id WHERE bm.company_id=$1 AND bm.batch_id=$2 AND moi.product_id IS NOT NULL GROUP BY moi.product_id) t JOIN LATERAL (SELECT r.id,r.employee_id FROM worker_assignment_rules r JOIN employees e ON e.company_id=r.company_id AND e.id=r.employee_id AND e.status='active' WHERE r.company_id=$1 AND r.marketplace_key=$3 AND r.status='active' AND (r.product_id=t.product_id OR r.product_id IS NULL) ORDER BY r.product_id IS NULL,r.priority,r.id LIMIT 1) rule ON true`, companyID, batchID, marketplace)
	return err
}

func (s *Service) workerTotals(ctx context.Context, companyID, batchID string) ([]WorkerTotal, error) {
	rows, err := s.db.Query(ctx, `SELECT a.employee_id,e.display_name,sum(a.total_quantity)::integer,sum(a.order_line_count)::integer,count(a.product_id)::integer FROM batch_worker_assignments a JOIN employees e ON e.company_id=a.company_id AND e.id=a.employee_id WHERE a.company_id=$1 AND a.batch_id=$2 GROUP BY a.employee_id,e.display_name ORDER BY e.display_name,a.employee_id`, companyID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkerTotal, 0)
	for rows.Next() {
		var item WorkerTotal
		if err = rows.Scan(&item.EmployeeID, &item.EmployeeName, &item.TotalQuantity, &item.OrderLineCount, &item.ProductCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
