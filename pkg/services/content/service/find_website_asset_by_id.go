package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindWebsiteAssetByID(ctx context.Context, db db.Queryer, websiteID, assetID guid.GUID) (asset content.Asset, err error) {
	asset, err = service.repo.FindAssetByID(ctx, db, assetID)
	if err != nil {
		return
	}
	if !asset.WebsiteID.Equal(websiteID) {
		err = content.ErrAssetNotFound
		return
	}
	return
}
