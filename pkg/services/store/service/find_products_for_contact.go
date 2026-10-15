package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) FindProductsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (products []store.Product, err error) {
	products, err = service.repo.FindProductsForContact(ctx, db, contactID)
	return
}
