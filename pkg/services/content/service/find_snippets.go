package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) FindSnippets(ctx context.Context, db db.Queryer, websiteID guid.GUID) (snippets []content.Snippet, err error) {
	snippets, err = service.repo.FindSnippetsForWebsite(ctx, db, websiteID)
	return
}
