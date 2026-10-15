package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindPageByID(ctx context.Context, db db.Queryer, pageID guid.GUID) (page content.Page, err error) {
	page, err = service.repo.FindPageByID(ctx, service.db, pageID)
	if err != nil {
		return
	}

	return
}
