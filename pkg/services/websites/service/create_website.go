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

func (service *WebsitesService) CreateWebsite(ctx context.Context, input websites.CreateWebsiteInput) (website websites.Website, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, input.OrganizationID)
	if err != nil {
		return
	}

	name := strings.TrimSpace(input.Name)
	slug := strings.TrimSpace(input.Slug)

	err = validateWebsiteName(name)
	if err != nil {
		return
	}

	err = validateWebsiteSlug(slug, httpCtx.AccessToken.IsAdmin)
	if err != nil {
		return
	}

	currency := websites.CurrencyEUR
	if input.Currency != nil {
		err = validateCurrency(*input.Currency)
		if err != nil {
			return
		}
		currency = *input.Currency
	}

	err = service.organizationsService.CheckBillingGatedAction(ctx, service.db, input.OrganizationID, organizations.BillingGatedActionCreateWebsite{})
	if err != nil {
		return
	}

	websiteID := guid.NewTimeBased()
	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		// check if site already exists
		_, txErr = service.repo.FindWebsiteBySlug(ctx, tx, slug)
		if txErr == nil {
			txErr = websites.ErrWebsiteSlugNotAvailable
			return txErr
		} else {
			if !errs.IsNotFound(txErr) {
				return txErr
			}
			txErr = nil
		}

		website = websites.Website{
			ID:             websiteID,
			CreatedAt:      now,
			UpdatedAt:      now,
			ModifiedAt:     now,
			BlockedAt:      nil,
			BlockedReason:  "",
			Name:           name,
			Slug:           slug,
			Header:         "",
			Footer:         "",
			Navigation:     websites.DefaultWebsiteNavigation,
			Language:       websites.DefaultWebsiteLanguage,
			PrimaryDomain:  service.getSubdomainForSlug(slug),
			Description:    "",
			RobotsTxt:      websites.DefaultRobotsTxt,
			Currency:       currency,
			CustomIcon:     false,
			CustomIconHash: nil,
			Colors:         websites.DefaultColors,
			Theme:          websites.DefaultTheme,
			Announcement:   nil,
			Ad:             nil,
			PoweredBy:      true,

			OrganizationID: input.OrganizationID,
		}
		txErr = service.repo.CreateWebsite(ctx, tx, website)
		if txErr != nil {
			return txErr
		}

		txErr = service.contentService.InitNewWebsiteData(ctx, tx, website)
		if txErr != nil {
			return txErr
		}

		_, txErr = service.emailsService.InitWebsiteConfiguration(ctx, tx, website.ID, website.Name)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
