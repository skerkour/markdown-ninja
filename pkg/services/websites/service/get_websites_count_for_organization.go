package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) GetWebsitesCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (websitesCount int64, err error) {
	return service.repo.GetWebsitesCountForOrganization(ctx, db, organizationID)
}
