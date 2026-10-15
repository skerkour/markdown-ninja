package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
)

func (service *WebsitesService) RemoveDomain(ctx context.Context, input websites.RemoveDomainInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	domain, err := service.repo.FindDomainByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	website, err := service.repo.FindWebsiteByID(ctx, service.db, domain.WebsiteID, false)
	if err != nil {
		return
	}

	_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, website.OrganizationID)
	if err != nil {
		return
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		if website.PrimaryDomain == domain.Hostname {
			now := time.Now().UTC()
			website.UpdatedAt = now
			website.ModifiedAt = now
			website.PrimaryDomain = service.getSubdomainForSlug(website.Slug)

			txErr = service.repo.UpdateWebsite(ctx, tx, website)
			if txErr != nil {
				return txErr
			}
		}

		txErr = service.repo.DeleteDomain(ctx, tx, domain.ID)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
