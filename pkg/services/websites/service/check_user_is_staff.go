package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

func (service *WebsitesService) CheckUserIsStaff(ctx context.Context, db db.Queryer, userID uuid.UUID, websiteID guid.GUID) (err error) {
	// we don't use a join to keep the separation of concerns (avoid mixing tables between services)
	website, err := service.repo.FindWebsiteByID(ctx, db, websiteID, false)
	if err != nil {
		return err
	}

	_, err = service.organizationsService.CheckUserIsStaff(ctx, db, userID, website.OrganizationID)
	return err
}
