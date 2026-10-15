package service

import (
	"context"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
)

func (service *WebsitesService) DeleteWebsite(ctx context.Context, input websites.DeleteWebsiteInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	website, err := service.repo.FindWebsiteByID(ctx, service.db, input.ID, false)
	if err != nil {
		return
	}

	_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, website.OrganizationID)
	if err != nil {
		return
	}

	products, err := service.storeService.FindProductsForWebsite(ctx, service.db, website.ID, 5)
	if err != nil {
		return
	}

	if len(products) != 0 {
		err = websites.ErrCantDeleteWebsiteWithProducts
		return
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.eventsService.ScheduleDeletionOfWebsiteData(ctx, tx, website.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.emailsService.RemoveWebsiteConfiguration(ctx, tx, website.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.contentService.DeleteWebsiteData(ctx, tx, website.ID)
		if txErr != nil {
			return txErr
		}

		txErr = service.repo.DeleteWebsite(ctx, tx, website.ID)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
