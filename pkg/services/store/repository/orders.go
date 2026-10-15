package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) CreateOrder(ctx context.Context, db db.Queryer, order store.Order) (err error) {
	const query = `INSERT INTO orders
			(id, created_at, updated_at, total_amount, currency, notes, status, completed_at, canceled_at,
				email, country, additional_invoice_information, stripe_checkout_session_id, stripe_payment_intent_id, stripe_invoice_id, stripe_invoice_url,
				contact_id, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

	_, err = db.Exec(ctx, query, order.ID, order.CreatedAt, order.UpdatedAt, order.TotalAmount, order.Currency,
		order.Notes, order.Status, order.CompletedAt, order.CanceledAt,
		order.Email, order.Country, order.AdditionalInvoiceInformation,
		order.StripeCheckoutSessionID, order.StripPaymentItentID, order.StripeInvoiceID, order.StripeInvoiceUrl,
		order.ContactID, order.WebsiteID)
	if err != nil {
		err = fmt.Errorf("store.CreateOrder: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) UpdateOrder(ctx context.Context, db db.Queryer, order store.Order) (err error) {
	const query = `UPDATE orders
		SET updated_at = $1, notes = $2, status = $3, completed_at = $4, canceled_at = $5,
			email = $6, country = $7, stripe_invoice_id = $8, stripe_payment_intent_id = $9,
			stripe_checkout_session_id = $10, stripe_invoice_url = $11, total_amount = $12,
			additional_invoice_information = $13
		WHERE id = $14
`

	_, err = db.Exec(ctx, query, order.UpdatedAt, order.Notes, order.Status, order.CompletedAt, order.CanceledAt,
		order.Email, order.Country, order.StripeInvoiceID,
		order.StripPaymentItentID, order.StripeCheckoutSessionID, order.StripeInvoiceUrl, order.TotalAmount,
		order.AdditionalInvoiceInformation,
		order.ID)
	if err != nil {
		err = fmt.Errorf("store.UpdateOrder: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindOrdersMetadataForWebsite(ctx context.Context, db db.Queryer, websiteId guid.GUID, limit int64, after *guid.GUID) (ret []store.OrderMetadata, err error) {
	ret = make([]store.OrderMetadata, 0)
	query := `SELECT id, created_at, updated_at, email, total_amount, currency, status, completed_at, canceled_at, contact_id, country
		FROM orders
		WHERE website_id = $1`

	args := []any{websiteId, limit}
	if after != nil {
		args = append(args, *after)
		query += ` AND id < $3`
	}
	query += `
		ORDER BY id DESC
		LIMIT $2
	`

	err = db.Select(ctx, &ret, query, args...)
	if err != nil {
		err = fmt.Errorf("store.FindOrdersMetadataForWebsite: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindOrdersForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (ret []store.Order, err error) {
	ret = make([]store.Order, 0)
	const query = `SELECT * FROM orders
		WHERE contact_id = $1
		ORDER BY id DESC
	`

	err = db.Select(ctx, &ret, query, contactID)
	if err != nil {
		err = fmt.Errorf("store.FindOrdersForContact: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindOrdersWithStatusForContact(ctx context.Context, db db.Queryer, contactID guid.GUID, status store.OrderStatus) (ret []store.Order, err error) {
	ret = make([]store.Order, 0)
	const query = `SELECT * FROM orders
		WHERE contact_id = $1 AND status = $2
		ORDER BY id DESC
	`

	err = db.Select(ctx, &ret, query, contactID, status)
	if err != nil {
		err = fmt.Errorf("store.FindOrdersWithStatusForContact: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindOrdersForProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (ret []store.Order, err error) {
	ret = make([]store.Order, 0, 10)
	const query = `SELECT * FROM orders WHERE id = ANY (
		SELECT order_id FROM order_line_items WHERE product_id = $1
	)
	ORDER BY id DESC`

	err = db.Select(ctx, &ret, query, productID)
	if err != nil {
		err = fmt.Errorf("store.FindOrdersForProduct: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindOrderByID(ctx context.Context, db db.Queryer, orderID guid.GUID, forUpdate bool) (order store.Order, err error) {
	query := "SELECT * FROM orders WHERE id = $1"
	if forUpdate {
		query += " FOR UPDATE"
	}

	err = db.Get(ctx, &order, query, orderID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrOrderNotFound(orderID)
		} else {
			err = fmt.Errorf("store.FindOrderByID: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) GetWebsiteRevenue(ctx context.Context, db db.Queryer, websiteID guid.GUID, from, to time.Time) (revenue int64, err error) {
	const query = `SELECT
		(SELECT COALESCE(SUM(total_amount), 0) AS sales FROM orders
			WHERE website_id = $1 AND status = $2 AND created_at >= $3 AND created_at <= $4)
		- (SELECT COALESCE(SUM(amount), 0) FROM refunds
			WHERE website_id = $1 AND created_at >= $3 AND created_at <= $4)
	`

	err = db.Get(ctx, &revenue, query, websiteID, store.OrderStatusCompleted, from, to)
	if err != nil {
		err = fmt.Errorf("store.GetWebsiteRevenue: %w", err)
		return
	}

	return
}
