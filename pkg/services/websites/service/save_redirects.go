package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *WebsitesService) SaveRedirects(ctx context.Context, input websites.SaveRedirectsInput) (redirects []websites.Redirect, err error) {
	redirects = []websites.Redirect{}
	var website websites.Website

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		website, err = service.repo.FindWebsiteByID(ctx, service.db, input.WebsiteID, false)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckUserIsStaff(ctx, service.db, actorID, website.OrganizationID)
		if err != nil {
			return
		}
	} else {
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			err = kernel.ErrPermissionDenied
			return
		}

		website, err = service.repo.FindWebsiteByID(ctx, service.db, input.WebsiteID, false)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	if !input.WebsiteID.Equal(website.ID) {
		err = websites.ErrWebsiteNotFound
		return
	}

	existingRedirects, err := service.repo.FindRedirectsForWebsite(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}

	diff := service.diffRedirects(existingRedirects, input.Redirects)

	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		for _, redirectToCreate := range diff.RedirectsToCreate {
			// TODO: clean and validate input
			// TODO: parse pattern for domain
			// status := redirectToCreate.Status
			status := http.StatusMovedPermanently
			txErr = validateRedirectStatus(status)
			if txErr != nil {
				return txErr
			}

			to := strings.TrimSpace(redirectToCreate.To)
			txErr = validateRedirectDestination(redirectToCreate.To)
			if txErr != nil {
				return txErr
			}

			pattern := strings.TrimSpace(redirectToCreate.Pattern)
			txErr = validateRedirectPattern(pattern)
			if txErr != nil {
				return txErr
			}

			redirect := websites.Redirect{
				ID:          guid.NewTimeBased(),
				CreatedAt:   now,
				UpdatedAt:   now,
				Pattern:     pattern,
				Domain:      "",
				PathPattern: pattern,
				To:          to,
				Status:      int64(status),
				WebsiteID:   input.WebsiteID,
			}
			txErr = service.repo.CreateRedirect(ctx, tx, redirect)
			if txErr != nil {
				return txErr
			}
		}

		for _, redirectToRemove := range diff.RedirectsToRemove {
			txErr = service.repo.DeleteRedirect(ctx, tx, redirectToRemove.ID)
			if txErr != nil {
				return txErr
			}
		}

		// TODO: improve
		redirects, txErr = service.repo.FindRedirectsForWebsite(ctx, tx, input.WebsiteID)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
