package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/commerceops/commerceops/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

type ConsignmentMovement struct {
	ProductID string `json:"product_id"`
	Quantity  int64  `json:"quantity"`
}

func (s *Service) ReserveConsignment(ctx context.Context, tx pgx.Tx, p auth.Principal, consignmentID string, movements []ConsignmentMovement) error {
	if err := s.authorizeConsignment(ctx, p, "consignments.manage"); err != nil {
		return err
	}
	if err := validateConsignmentMovements(tx, consignmentID, movements); err != nil {
		return err
	}
	sortMovements(movements)
	for _, movement := range movements {
		if _, err := tx.Exec(ctx, `INSERT INTO inventory_balances(company_id,product_id) SELECT $1,id FROM products WHERE company_id=$1 AND id=$2 AND status='active' ON CONFLICT DO NOTHING`, p.CompanyID, movement.ProductID); err != nil {
			return mapDBError(err)
		}
	}
	for _, movement := range movements {
		var onHand, reserved int64
		if err := tx.QueryRow(ctx, `SELECT b.on_hand,b.reserved FROM inventory_balances b JOIN products p ON p.company_id=b.company_id AND p.id=b.product_id WHERE b.company_id=$1 AND b.product_id=$2 AND p.status='active' FOR UPDATE OF b`, p.CompanyID, movement.ProductID).Scan(&onHand, &reserved); err != nil {
			return mapDBError(err)
		}
		if reserved+movement.Quantity > onHand {
			return ErrInsufficientStock
		}
		key := digest("consignment-reserve\x00" + consignmentID + "\x00" + movement.ProductID)
		hash := hashJSON(struct {
			ConsignmentID string              `json:"consignment_id"`
			Movement      ConsignmentMovement `json:"movement"`
		}{consignmentID, movement})
		var reservationID string
		if err := tx.QueryRow(ctx, `INSERT INTO inventory_reservations(company_id,product_id,quantity,reason,source_type,source_id,created_by,create_idempotency_key,create_request_hash) VALUES($1,$2,$3,'Allocated to consignment','consignment',$4,$5,$6,$7) RETURNING id`, p.CompanyID, movement.ProductID, movement.Quantity, consignmentID, p.UserID, key, hash).Scan(&reservationID); err != nil {
			return mapDBError(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE inventory_balances SET reserved=reserved+$1,updated_at=now() WHERE company_id=$2 AND product_id=$3`, movement.Quantity, p.CompanyID, movement.ProductID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory.consignment_reserved", "inventory_reservation", reservationID, map[string]any{"consignment_id": consignmentID, "product_id": movement.ProductID, "quantity": movement.Quantity}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ReleaseConsignment(ctx context.Context, tx pgx.Tx, p auth.Principal, consignmentID, eventID, reason string) error {
	if err := s.authorizeConsignment(ctx, p, "consignments.manage"); err != nil {
		return err
	}
	if tx == nil || !uuidRE.MatchString(consignmentID) || !uuidRE.MatchString(eventID) || reason == "" {
		return ErrInvalidInput
	}
	reservations, err := lockConsignmentReservations(ctx, tx, p.CompanyID, consignmentID)
	if err != nil {
		return err
	}
	for _, reservation := range reservations {
		if reservation.Status != "active" {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET reserved=reserved-$1,updated_at=now() WHERE company_id=$2 AND product_id=$3 AND reserved >= $1`, reservation.Quantity, p.CompanyID, reservation.ProductID); err != nil {
			return err
		}
		key := digest("consignment-release\x00" + eventID + "\x00" + reservation.ProductID)
		hash := digest(reason + "\x00" + reservation.ID)
		if _, err = tx.Exec(ctx, `UPDATE inventory_reservations SET status='released',released_by=$1,release_reason=$2,release_idempotency_key=$3,release_request_hash=$4,released_at=now() WHERE company_id=$5 AND id=$6 AND status='active'`, p.UserID, reason, key, hash, p.CompanyID, reservation.ID); err != nil {
			return mapDBError(err)
		}
		if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory.consignment_reservation_released", "inventory_reservation", reservation.ID, map[string]any{"consignment_id": consignmentID, "product_id": reservation.ProductID, "quantity": reservation.Quantity, "reason": reason}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ConfirmConsignmentOutbound(ctx context.Context, tx pgx.Tx, p auth.Principal, consignmentID, eventID string) ([]Transaction, error) {
	if err := s.authorizeConsignment(ctx, p, "consignments.outbound"); err != nil {
		return nil, err
	}
	if tx == nil || !uuidRE.MatchString(consignmentID) || !uuidRE.MatchString(eventID) {
		return nil, ErrInvalidInput
	}
	reservations, err := lockConsignmentReservations(ctx, tx, p.CompanyID, consignmentID)
	if err != nil || len(reservations) == 0 {
		if err == nil {
			err = ErrNotFound
		}
		return nil, err
	}
	sort.Slice(reservations, func(i, j int) bool { return reservations[i].ProductID < reservations[j].ProductID })
	payload, _ := json.Marshal(reservations)
	requestHash := digest(string(payload))
	items := make([]Transaction, 0, len(reservations))
	for _, reservation := range reservations {
		if reservation.Status != "active" {
			return nil, ErrConflict
		}
		var previous, reserved int64
		if err = tx.QueryRow(ctx, `SELECT on_hand,reserved FROM inventory_balances WHERE company_id=$1 AND product_id=$2 FOR UPDATE`, p.CompanyID, reservation.ProductID).Scan(&previous, &reserved); err != nil {
			return nil, mapDBError(err)
		}
		if previous < reservation.Quantity || reserved < reservation.Quantity {
			return nil, ErrInsufficientStock
		}
		result := previous - reservation.Quantity
		key := digest("consignment-out\x00" + consignmentID + "\x00" + reservation.ProductID)
		var item Transaction
		err = scanTransaction(tx.QueryRow(ctx, `INSERT INTO inventory_transactions(company_id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,idempotency_key,request_hash) VALUES($1,$2,'consignment_out',$3,$4,$5,'Consignment outbound','consignment',$6,$7,$8,$9) RETURNING id,product_id,transaction_type,quantity_delta,previous_balance,resulting_balance,reason,reference_type,reference_id,actor_user_id,created_at`, p.CompanyID, reservation.ProductID, -reservation.Quantity, previous, result, consignmentID, p.UserID, key, requestHash), &item)
		if err != nil {
			return nil, mapDBError(err)
		}
		if _, err = tx.Exec(ctx, `UPDATE inventory_balances SET on_hand=$1,reserved=reserved-$2,updated_at=now() WHERE company_id=$3 AND product_id=$4`, result, reservation.Quantity, p.CompanyID, reservation.ProductID); err != nil {
			return nil, err
		}
		releaseKey := digest("consignment-consume\x00" + eventID + "\x00" + reservation.ProductID)
		if _, err = tx.Exec(ctx, `UPDATE inventory_reservations SET status='released',released_by=$1,release_reason='Consumed by consignment outbound',release_idempotency_key=$2,release_request_hash=$3,released_at=now() WHERE company_id=$4 AND id=$5 AND status='active'`, p.UserID, releaseKey, requestHash, p.CompanyID, reservation.ID); err != nil {
			return nil, mapDBError(err)
		}
		items = append(items, item)
	}
	if err = s.audit.Record(ctx, tx, p.CompanyID, p.UserID, "inventory.consignment_outbound", "consignment", consignmentID, map[string]any{"event_id": eventID, "product_count": len(items)}); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) authorizeConsignment(ctx context.Context, p auth.Principal, permission string) error {
	if err := s.authorizer.RequireModule(ctx, p, "inventory"); err != nil {
		return err
	}
	if err := s.authorizer.RequireModule(ctx, p, "consignments"); err != nil {
		return err
	}
	return s.authorizer.RequirePermission(ctx, p, permission)
}

func validateConsignmentMovements(tx pgx.Tx, consignmentID string, movements []ConsignmentMovement) error {
	if tx == nil || !uuidRE.MatchString(consignmentID) || len(movements) == 0 || len(movements) > 100 {
		return ErrInvalidInput
	}
	seen := map[string]bool{}
	for _, movement := range movements {
		if !uuidRE.MatchString(movement.ProductID) || movement.Quantity <= 0 || seen[movement.ProductID] {
			return ErrInvalidInput
		}
		seen[movement.ProductID] = true
	}
	return nil
}

func sortMovements(movements []ConsignmentMovement) {
	sort.Slice(movements, func(i, j int) bool { return movements[i].ProductID < movements[j].ProductID })
}

func lockConsignmentReservations(ctx context.Context, tx pgx.Tx, companyID, consignmentID string) ([]Reservation, error) {
	rows, err := tx.Query(ctx, `SELECT id,product_id,quantity,status,reason,source_type,source_id,created_by,released_by,release_reason,created_at,released_at FROM inventory_reservations WHERE company_id=$1 AND source_type='consignment' AND source_id=$2 ORDER BY product_id FOR UPDATE`, companyID, consignmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Reservation{}
	for rows.Next() {
		var item Reservation
		if err = scanReservation(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
