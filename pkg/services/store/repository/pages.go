package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) GetProductPagesCountForProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (count int64, err error) {
	const query = "SELECT COUNT(id) FROM product_pages WHERE product_id = $1"

	err = db.Get(ctx, &count, query, productID)
	if err != nil {
		err = fmt.Errorf("store.GetProductPagesCountForProduct: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) CreateProductPage(ctx context.Context, db db.Queryer, page store.ProductPage) (err error) {
	const query = `INSERT INTO product_pages
			(id, created_at, updated_at, position, title, size, hash, body_markdown, product_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = db.Exec(ctx, query, page.ID, page.CreatedAt, page.UpdatedAt, page.Position,
		page.Title, page.Size, page.Hash, page.BodyMarkdown,
		page.ProductID)
	if err != nil {
		err = fmt.Errorf("store.CreateProductPage: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindProductPageByID(ctx context.Context, db db.Queryer, pageID guid.GUID) (page store.ProductPage, err error) {
	const query = "SELECT * FROM product_pages WHERE id = $1"

	err = db.Get(ctx, &page, query, pageID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrProductPageNotFound
		} else {
			err = fmt.Errorf("store.FindProductPageByID: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) DeleteProductPage(ctx context.Context, db db.Queryer, pageID guid.GUID) (err error) {
	const query = `DELETE FROM product_pages WHERE id = $1`

	_, err = db.Exec(ctx, query, pageID)
	if err != nil {
		err = fmt.Errorf("store.DeleteProductPage: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) UpdateProductPage(ctx context.Context, db db.Queryer, page store.ProductPage) (err error) {
	const query = `UPDATE product_pages
		SET updated_at = $1, position = $2, title = $3,
			size = $4, hash = $5, body_markdown = $6
		WHERE id = $7
`

	_, err = db.Exec(ctx, query, page.UpdatedAt, page.Position, page.Title, page.Size,
		page.Hash, page.BodyMarkdown,
		page.ID)
	if err != nil {
		err = fmt.Errorf("store.UpdateProductPage: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindProductPagesForProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (ret []store.ProductPage, err error) {
	ret = make([]store.ProductPage, 0)
	const query = `SELECT * FROM product_pages
		WHERE product_id = $1
		ORDER BY position
	`

	err = db.Select(ctx, &ret, query, productID)
	if err != nil {
		err = fmt.Errorf("store.FindProductPagesForProduct: %w", err)
		return
	}

	return
}
