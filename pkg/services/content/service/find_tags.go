package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindTags(ctx context.Context, db db.Queryer, websiteID guid.GUID) (tags []content.Tag, err error) {
	tags, err = service.repo.FindTagsForWebsite(ctx, db, websiteID)
	return
}
