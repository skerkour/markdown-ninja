package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindProductAssets(ctx context.Context, db db.Queryer, productID guid.GUID) (assets []content.Asset, err error) {
	assets, err = service.repo.FindProductAssets(ctx, db, productID)
	return
}
