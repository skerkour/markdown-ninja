package repository

import (
	"context"
	"fmt"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) CreateOrderLineItem(ctx context.Context, db db.Queryer, lineItem store.OrderLineItem) (err error) {
	const query = `INSERT INTO order_line_items
			(product_name, original_product_price, quantity, order_id, product_id)
		VALUES ($1, $2, $3, $4, $5)`

	_, err = db.Exec(ctx, query, lineItem.ProductName, lineItem.OriginalProductPrice, lineItem.Quantity, lineItem.OrderID, lineItem.ProductID)
	if err != nil {
		err = fmt.Errorf("store.CreateOrderLineItem: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindOrderLineItems(ctx context.Context, db db.Queryer, orderID guid.GUID) (ret []store.OrderLineItem, err error) {
	ret = []store.OrderLineItem{}
	const query = `SELECT * FROM order_line_items
		WHERE order_id = $1
	`

	err = db.Select(ctx, &ret, query, orderID)
	if err != nil {
		err = fmt.Errorf("store.FindOrderLineItems: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) UpdateOrderLineItem(ctx context.Context, db db.Queryer, lineItem store.OrderLineItem) (err error) {
	const query = `UPDATE order_line_items
		SET product_name = $1, original_product_price = $2, quantity = $3
		WHERE order_id = $4 AND product_id = $5
`

	_, err = db.Exec(ctx, query, lineItem.ProductName, lineItem.OriginalProductPrice,
		lineItem.Quantity, lineItem.OrderID, lineItem.ProductID)
	if err != nil {
		err = fmt.Errorf("store.UpdateOrderLineItem: %w", err)
		return
	}

	return
}
