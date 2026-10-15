package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) FindProduct(ctx context.Context, db db.Queryer, productID guid.GUID) (product store.Product, err error) {
	product, err = service.repo.FindProductByID(ctx, db, productID)
	return
}
