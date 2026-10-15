package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
)

func (service *StoreService) hydrateProduct(ctx context.Context, db db.Queryer, product *store.Product) (err error) {
	product.Content, err = service.repo.FindProductPagesForProduct(ctx, db, product.ID)
	if err != nil {
		return err
	}

	product.Assets, err = service.contentService.FindProductAssets(ctx, db, product.ID)
	if err != nil {
		return err
	}

	return nil
}
