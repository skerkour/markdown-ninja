package service

import (
	"context"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *EmailsService) FindWebsiteConfiguration(ctx context.Context, db db.Queryer, websiteID guid.GUID) (configuration emails.WebsiteConfiguration, err error) {
	configuration, err = service.repo.FindWebsiteConfiguration(ctx, db, websiteID)
	return
}
