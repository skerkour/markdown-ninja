package service

import (
	"context"
	"unicode/utf8"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindPublishedPagesMetadata(ctx context.Context, db db.Queryer, websiteID guid.GUID, pageTypes []content.PageType, limit int64) (pages []content.PageMetadata, err error) {
	pages, err = service.repo.FindPublishedPagesMetadataForWebsite(ctx, db, websiteID, pageTypes, limit)
	return pages, err
}

func (service *ContentService) FindPublishedPagesMetadataForTag(ctx context.Context, db db.Queryer, websiteID guid.GUID, pageTypes []content.PageType, tagName string) (pages []content.PageMetadata, err error) {
	if !utf8.ValidString(tagName) {
		err = content.ErrTagNotFound
		return
	}

	tag, err := service.repo.FindTagByName(ctx, db, websiteID, tagName)
	if err != nil {
		return
	}

	pages, err = service.repo.FindPublishedPagesMetadataForTag(ctx, db, pageTypes, tag.ID)
	if err != nil {
		return
	}

	return
}
