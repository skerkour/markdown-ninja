package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) GetUsedStorageForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (storage int64, err error) {
	return service.repo.GetUsedAssetsStorageForOrganization(ctx, db, organizationID)
}
