package service

import (
	"context"
	"strings"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) FindWebsiteByID(ctx context.Context, db db.Queryer, websiteID guid.GUID) (website websites.Website, err error) {
	website, err = service.repo.FindWebsiteByID(ctx, db, websiteID, false)
	return
}

func (service *WebsitesService) FindWebsiteByDomain(ctx context.Context, db db.Queryer, domain string) (website websites.Website, err error) {
	if strings.HasSuffix(domain, service.websitesRootDomain) {
		slug := strings.TrimSuffix(domain, "."+service.websitesRootDomain)
		website, err = service.repo.FindWebsiteBySlug(ctx, db, slug)
	} else {
		website, err = service.repo.FindWebsiteForDomain(ctx, db, domain)
	}
	return
}
