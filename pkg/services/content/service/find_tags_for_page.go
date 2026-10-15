package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindTagsForPage(ctx context.Context, db db.Queryer, pageID guid.GUID) (tags []content.Tag, err error) {
	tags, err = service.repo.FindTagsForPage(ctx, service.db, pageID)
	if err != nil {
		return
	}

	return
}
