package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *StoreRepository) CreateContactProductAccess(ctx context.Context, db db.Queryer, productAccess store.ContactProductAccess) (err error) {
	const query = `INSERT INTO contact_product_access
			(created_at, contact_id, product_id)
		VALUES ($1, $2, $3)`

	_, err = db.Exec(ctx, query, productAccess.CreatedAt, productAccess.ContactID, productAccess.ProductID)
	if err != nil {
		err = fmt.Errorf("store.CreateContactProductAccess: %w", err)
		return
	}

	return
}

func (repo *StoreRepository) FindContactProductAccess(ctx context.Context, db db.Queryer, contactID, productID guid.GUID) (productAccess store.ContactProductAccess, err error) {
	const query = `SELECT * FROM contact_product_access
		WHERE contact_id = $1 AND product_id = $2
	`

	err = db.Get(ctx, &productAccess, query, contactID, productID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = store.ErrProductAccessNotFound
		} else {
			err = fmt.Errorf("store.FindContactProductAccess: %w", err)
		}
		return
	}

	return
}

func (repo *StoreRepository) DeleteAccessToProduct(ctx context.Context, db db.Queryer, relation store.ContactProductAccess) (err error) {
	const query = `DELETE FROM contact_product_access WHERE contact_id = $1 AND product_id = $2`

	_, err = db.Exec(ctx, query, relation.ContactID, relation.ProductID)
	if err != nil {
		err = fmt.Errorf("store.DeleteAccessToProduct: %w", err)
		return
	}

	return
}
