package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/markdown"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContentService) UpdatePage(ctx context.Context, input content.UpdatePageInput) (page content.Page, err error) {
	logger := slogx.FromCtx(ctx)
	var website websites.Website

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		page, err = service.repo.FindPageByID(ctx, service.db, input.PageID)
		if err != nil {
			return
		}

		err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, page.WebsiteID)
		if err != nil {
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, page.WebsiteID)
		if err != nil {
			return
		}
	} else {
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			err = kernel.ErrPermissionDenied
			return
		}

		page, err = service.repo.FindPageByID(ctx, service.db, input.PageID)
		if err != nil {
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, page.WebsiteID)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	// We can only update pages, posts and broadcast with this function
	if page.Type != content.PageTypePage &&
		page.Type != content.PageTypePost {
		err = content.ErrPageCantBeUpdated
		return
	}

	// TODO: clean and validate input
	// TODO: validate tags
	now := time.Now().UTC().Truncate(time.Second)

	page.Date = input.Date.UTC().Truncate(time.Second)

	page.Title = strings.TrimSpace(input.Title)
	err = service.ValidatePageTitle(page.Title)
	if err != nil {
		return
	}

	page.Path = strings.TrimSpace(input.Path)
	err = service.validatePagePath(page.Path)
	if err != nil {
		return
	}

	page.Language = strings.ToLower(strings.TrimSpace(input.Language))
	err = service.validateLanguage(page.Language)
	if err != nil {
		return
	}

	sendNewsletter := false
	if page.SendAsNewsletter != input.SendAsNewsletter && page.NewsletterSentAt != nil {
		err = content.ErrSendAsNewsletterCantBeUpdatedAfterBeingSent
		return
	}
	page.SendAsNewsletter = input.SendAsNewsletter
	if page.SendAsNewsletter {
		emailsConfig, err := service.emailsService.FindWebsiteConfiguration(ctx, service.db, page.WebsiteID)
		if err != nil {
			return content.Page{}, err
		}
		if !emailsConfig.DomainVerified {
			return content.Page{}, emails.ErrNoCustomEmailDomainConfigured
		}
	}

	if input.Draft {
		page.Status = content.PageStatusDraft
	} else {
		if page.Date.After(now) {
			page.Status = content.PageStatusScheduled
		} else {
			page.Status = content.PageStatusPublished
			if input.SendAsNewsletter && page.NewsletterSentAt == nil {
				sendNewsletter = true
				page.NewsletterSentAt = &now
			}
		}
	}

	if input.BodyMarkdown != nil {
		page.BodyMarkdown = *input.BodyMarkdown
		err = service.ValidatePageBodyMarkdown(page.BodyMarkdown)
		if err != nil {
			return
		}

		page.Size = int64(len(page.BodyMarkdown))
		bodyHash := blake3.Sum256([]byte(page.BodyMarkdown))
		page.BodyHash = bodyHash[:]
	}
	bodyHtml, err := markdown.ToHtmlPage(
		page.BodyMarkdown,
		service.httpConfig.WebsitesBaseUrl.Scheme+"://"+website.PrimaryDomain+service.httpConfig.WebsitesPort,
	)
	if err != nil {
		return
	}

	err = service.validatePageSendAsNewsletter(page.SendAsNewsletter, page.Type)
	if err != nil {
		return
	}

	page.UpdatedAt = now
	if input.UpdatedAt != nil {
		page.UpdatedAt = input.UpdatedAt.UTC().Truncate(time.Second)
		if page.UpdatedAt.Before(page.Date) {
			err = content.ErrPageUpdatedAtCantBeBeforeDate
			return
		}
	}

	var existingPageWithSamePath content.Page
	// check if path is not already in use
	existingPageWithSamePath, err = service.repo.FindPageByPath(ctx, service.db, page.WebsiteID, page.Path)
	if err == nil && !existingPageWithSamePath.ID.Equal(page.ID) {
		err = content.ErrPageWithPathAlreadyExists
	} else if err != nil {
		if errs.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		return
	}

	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		err = service.validatePageDescription(description)
		if err != nil {
			return
		}
		if len(description) == 0 {
			description, err = service.getDescriptionFromContentHtml(ctx, service.db, page.WebsiteID, bodyHtml)
			if err != nil {
				return
			}
		}
		page.Description = description
	}

	siteTags, err := service.repo.FindTagsForWebsite(ctx, service.db, page.WebsiteID)
	if err != nil {
		return
	}

	pageTags, err := service.repo.FindTagsForPage(ctx, service.db, page.ID)
	if err != nil {
		return
	}

	tagsDiff, err := service.diffTags(pageTags, siteTags, input.Tags)
	if err != nil {
		return
	}

	metadataHash := content.HashPageMetadata(page.Type, page.Path, page.Date, page.SendAsNewsletter, page.Language, page.Title, page.Description, input.Tags)
	page.MetadataHash = metadataHash[:]

	var newsletter emails.Newsletter

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.UpdatePage(ctx, tx, page)
		if txErr != nil {
			return txErr
		}

		txErr = service.associateTagsToPage(ctx, tx, page, tagsDiff)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, page.WebsiteID, now)
		if txErr != nil {
			return txErr
		}

		page.Tags, txErr = service.repo.FindTagsForPage(ctx, tx, page.ID)
		if txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		return
	}

	if sendNewsletter {
		job := queue.NewJobInput{
			Data: emails.JobSendPostAsNewsletter{
				PostID:        page.ID,
				WebsiteDomain: website.PrimaryDomain,
			},
		}
		err = service.queue.Push(ctx, nil, job)
		if err != nil {
			errMessage := "content.UpdatePage: error pushing SendNewsletter job to queue"
			logger.Error(errMessage, slogx.Err(err), slog.String("newsletter.id", newsletter.ID.String()))
			err = nil
		}
	}

	return
}
