package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) FindProductsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID, limit int64) (products []store.Product, err error) {
	products, err = service.repo.FindProductsByWebsiteID(ctx, db, websiteID, limit)
	return
}
