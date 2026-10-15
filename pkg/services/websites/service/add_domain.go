package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) AddDomain(ctx context.Context, input websites.AddDomainInput) (domain websites.Domain, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	accessToken := httpCtx.AccessToken

	website, err := service.repo.FindWebsiteByID(ctx, service.db, input.WebsiteID, false)
	if err != nil {
		return
	}

	_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, website.OrganizationID)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	hostname := strings.ToLower(strings.TrimSpace(input.Hostname))
	err = service.validateDomainName(hostname, accessToken.IsAdmin)
	if err != nil {
		return
	}

	err = service.organizationsService.CheckBillingGatedAction(ctx, service.db, website.OrganizationID, organizations.BillingGatedActionAddWebsiteCustomDomain{
		WebsiteID: website.ID,
	})
	if err != nil {
		return
	}

	// check that domain is not already in use
	_, err = service.repo.FindWebsiteForDomain(ctx, service.db, hostname)
	if err == nil {
		err = websites.ErrDomainNameIsAlreadyInUse
	} else {
		if errs.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		return
	}

	domain = websites.Domain{
		ID:        guid.NewTimeBased(),
		CreatedAt: now,
		UpdatedAt: now,
		Hostname:  hostname,
		TlsActive: false,
		WebsiteID: website.ID,
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreateDomain(ctx, tx, domain)
		if txErr != nil {
			if db.IsErrAlreadyExists(txErr) {
				return websites.ErrDomainNameIsAlreadyInUse
			}
			return txErr
		}

		if input.Primary {
			website.UpdatedAt = now
			website.PrimaryDomain = hostname

			txErr = service.repo.UpdateWebsite(ctx, tx, website)
			if txErr != nil {
				return txErr
			}
		}

		return txErr
	})
	if err != nil {
		return
	}

	return
}
