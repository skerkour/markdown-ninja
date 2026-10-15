package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) CreateProduct(ctx context.Context, db db.Queryer, product store.Product) (err error) {
	const query = `INSERT INTO products
			(id, created_at, updated_at, name, description, type, status, price, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(ctx, query, product.ID, product.CreatedAt, product.UpdatedAt,
		product.Name, product.Description, product.Type, product.Status, product.Price,
		product.WebsiteID)
	if err != nil {
		err = fmt.Errorf("store.CreateProduct: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) UpdateProduct(ctx context.Context, db db.Queryer, product store.Product) (err error) {
	const query = `UPDATE products
		SET updated_at = $1, name = $2, description = $3, status = $4, price = $5
		WHERE id = $6
`

	_, err = db.Exec(ctx, query, product.UpdatedAt, product.Name, product.Description,
		product.Status, product.Price,
		product.ID)
	if err != nil {
		err = fmt.Errorf("store.UpdateProduct: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindProductByID(ctx context.Context, db db.Queryer, productID guid.GUID) (product store.Product, err error) {
	const query = "SELECT * FROM products WHERE id = $1"

	err = db.Get(ctx, &product, query, productID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrProductNotFound
		} else {
			err = fmt.Errorf("store.FindProductByID: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) FindProductsByWebsiteID(ctx context.Context, db db.Queryer, websiteID guid.GUID, limit int64) (ret []store.Product, err error) {
	ret = make([]store.Product, 0)
	const query = `SELECT * FROM products
		WHERE website_id = $1
		ORDER BY name
		LIMIT $2
	`

	err = db.Select(ctx, &ret, query, websiteID, limit)
	if err != nil {
		err = fmt.Errorf("store.FindProductsByWebsiteID: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindWebsiteProductsIn(ctx context.Context, db db.Queryer, websiteID guid.GUID, productIDs []guid.GUID) (ret []store.Product, err error) {
	const query = `SELECT * FROM products
		WHERE website_id = $1 AND id = ANY ($2)`

	ret = make([]store.Product, 0)
	if len(productIDs) == 0 {
		return
	}

	err = db.Select(ctx, &ret, query, websiteID, productIDs)
	if err != nil {
		err = fmt.Errorf("store.FindWebsiteProductsIn: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindProductsForCoupon(ctx context.Context, db db.Queryer, couponID guid.GUID) (ret []store.Product, err error) {
	ret = make([]store.Product, 0)
	const query = `SELECT * FROM products WHERE id = ANY (
		SELECT product_id FROM coupons_products WHERE coupon_id = $1
	)`

	err = db.Select(ctx, &ret, query, couponID)
	if err != nil {
		err = fmt.Errorf("store.FindProductsForCoupon: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindProductsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (ret []store.Product, err error) {
	ret = make([]store.Product, 0)
	const query = `SELECT * FROM products WHERE id = ANY (
		SELECT product_id FROM contact_product_access WHERE contact_id = $1
	)
	ORDER BY name`

	err = db.Select(ctx, &ret, query, contactID)
	if err != nil {
		err = fmt.Errorf("store.FindProductsForContact: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindProductsForOrder(ctx context.Context, db db.Queryer, orderID guid.GUID) (ret []store.Product, err error) {
	ret = make([]store.Product, 0)
	const query = `SELECT * FROM products WHERE id = ANY (
		SELECT product_id FROM order_line_items WHERE order_id = $1
	)
	ORDER BY name`

	err = db.Select(ctx, &ret, query, orderID)
	if err != nil {
		err = fmt.Errorf("store.FindProductsForOrder: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) DeleteProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (err error) {
	const query = `DELETE FROM products WHERE id = $1`

	_, err = db.Exec(ctx, query, productID)
	if err != nil {
		err = fmt.Errorf("store.DeleteProduct: %w", err)
		return
	}

	return
}
