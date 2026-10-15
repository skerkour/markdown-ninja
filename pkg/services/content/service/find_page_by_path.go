package service

import (
	"context"
	"unicode/utf8"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindPageByPath(ctx context.Context, db db.Queryer, websiteID guid.GUID, path string) (page content.Page, err error) {
	if !utf8.ValidString(path) {
		err = content.ErrPageNotFound
		return
	}

	page, err = service.repo.FindPageByPath(ctx, service.db, websiteID, path)
	if err != nil {
		return
	}

	return
}
