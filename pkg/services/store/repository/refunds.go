package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) CreateRefund(ctx context.Context, db db.Queryer, refund store.Refund) (err error) {
	const query = `INSERT INTO refunds
		(id, created_at, updated_at, amount, currency, notes, status, reason, failure_reason,
			stripe_refund_id, order_id, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err = db.Exec(ctx, query, refund.ID, refund.CreatedAt, refund.UpdatedAt, refund.Amount,
		refund.Currency, refund.Notes, refund.Status, refund.Reason, refund.FailureReason,
		refund.StripeRefundID, refund.OrderID, refund.WebsiteID)
	if err != nil {
		err = fmt.Errorf("store.CreateRefund: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) UpdateRefund(ctx context.Context, db db.Queryer, refund store.Refund) (err error) {
	const query = `UPDATE products
		SET updated_at = $1, notes = $2, status = $3, reason = $4, failure_reason = $5, stripe_refund_id = $6
		WHERE id = $7
		`

	_, err = db.Exec(ctx, query, refund.UpdatedAt, refund.Notes, refund.Status, refund.Reason, refund.FailureReason,
		refund.StripeRefundID, refund.ID)
	if err != nil {
		err = fmt.Errorf("store.UpdateRefund: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindRefundByID(ctx context.Context, db db.Queryer, refundID guid.GUID) (refund store.Refund, err error) {
	const query = "SELECT * FROM refunds WHERE id = $1"

	err = db.Get(ctx, &refund, query, refundID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrRefundNotFound
		} else {
			err = fmt.Errorf("store.FindRefundByID: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) FindRefundsByWebsiteID(ctx context.Context, db db.Queryer, websiteID guid.GUID, limit int64) (ret []store.Refund, err error) {
	ret = make([]store.Refund, 0)
	const query = `SELECT * FROM refunds
		WHERE website_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	err = db.Select(ctx, &ret, query, websiteID, limit)
	if err != nil {
		err = fmt.Errorf("store.FindRefundsByWebsiteID: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindRefundsByOrderID(ctx context.Context, db db.Queryer, orderID guid.GUID) (ret []store.Refund, err error) {
	ret = make([]store.Refund, 0)
	const query = `SELECT * FROM refunds
		WHERE order_id = $1
		ORDER BY created_at DESC
	`

	err = db.Select(ctx, &ret, query, orderID)
	if err != nil {
		err = fmt.Errorf("store.FindRefundsByOrderID: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindPendingRefunds(ctx context.Context, db db.Queryer) (ret []store.Refund, err error) {
	ret = make([]store.Refund, 0)
	const query = `SELECT * FROM refunds
		WHERE status = $1
		ORDER BY created_at DESC
	`

	err = db.Select(ctx, &ret, query, store.RefundStatusPending)
	if err != nil {
		err = fmt.Errorf("store.FindPendingRefunds: %w", err)
		return
	}

	return
}
