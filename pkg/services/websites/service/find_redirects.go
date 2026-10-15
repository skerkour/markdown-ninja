package service

import (
	"context"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) FindRedirects(ctx context.Context, db db.Queryer, websiteID guid.GUID) (redirects []websites.Redirect, err error) {
	redirects, err = service.repo.FindRedirectsForWebsite(ctx, service.db, websiteID)
	return
}
