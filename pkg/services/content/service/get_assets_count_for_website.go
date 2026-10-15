package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) GetAssetsCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error) {
	return service.repo.GetAssetsCountForWebsite(ctx, db, websiteID)
}
