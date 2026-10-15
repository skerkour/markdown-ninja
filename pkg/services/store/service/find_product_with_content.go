package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) FindProductWithContent(ctx context.Context, db db.Queryer, productID guid.GUID) (product store.Product, err error) {
	product, err = service.repo.FindProductByID(ctx, service.db, productID)
	if err != nil {
		return
	}

	product.Content, err = service.repo.FindProductPagesForProduct(ctx, db, product.ID)
	if err != nil {
		return
	}

	return
}
