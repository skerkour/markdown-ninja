package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindTag(ctx context.Context, db db.Queryer, websiteID guid.GUID, tag string) (ret content.Tag, err error) {
	return service.repo.FindTagByName(ctx, db, websiteID, tag)
}
