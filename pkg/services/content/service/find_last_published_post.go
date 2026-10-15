package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindLastPublishedPost(ctx context.Context, db db.Queryer, websiteID guid.GUID) (page content.Page, err error) {
	page, err = service.repo.FindLastPublishedPostForWebsite(ctx, db, websiteID)
	return
}
