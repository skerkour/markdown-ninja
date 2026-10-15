package service

import (
	"context"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) FindWebsitesForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (websites []websites.Website, err error) {
	websites, err = service.repo.FindWebsitesForOrganization(ctx, db, organizationID)
	return
}
