package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/markdown"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContentService) CreatePage(ctx context.Context, input content.CreatePageInput) (page content.Page, err error) {
	var website websites.Website
	logger := slogx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
		if err != nil {
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
		if err != nil {
			return
		}
	} else {
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			err = kernel.ErrPermissionDenied
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	date := input.Date.UTC().Truncate(time.Second)

	title := strings.TrimSpace(input.Title)
	err = service.ValidatePageTitle(title)
	if err != nil {
		return
	}

	language := strings.ToLower(strings.TrimSpace(input.Language))
	err = service.validateLanguage(language)
	if err != nil {
		return
	}
	if language == "" {
		language = website.Language
	}

	status := content.PageStatusPublished
	if input.Draft {
		status = content.PageStatusDraft
	} else {
		if input.Date.UTC().After(now) {
			status = content.PageStatusScheduled
		}
	}

	var newsletterSentAt *time.Time = nil
	sendAsNewsletter := input.SendAsNewsletter
	sendNewsletter := false
	if sendAsNewsletter && date.Before(now) && status == content.PageStatusPublished {
		newsletterSentAt = &now
		sendNewsletter = true
	}

	bodyMarkdown := input.BodyMarkdown
	err = service.ValidatePageBodyMarkdown(bodyMarkdown)
	if err != nil {
		return
	}

	size := int64(len(input.BodyMarkdown))
	bodyHash := blake3.Sum256([]byte(bodyMarkdown))

	bodyHtml, err := markdown.ToHtmlPage(
		bodyMarkdown,
		service.httpConfig.WebsitesBaseUrl.Scheme+"://"+website.PrimaryDomain+service.httpConfig.WebsitesPort,
	)
	if err != nil {
		return
	}

	// We can only create pages, posts with this function
	pageType := input.Type
	if pageType != content.PageTypePage &&
		pageType != content.PageTypePost {
		err = content.ErrPageTypeIsNotValid
		return
	}

	err = service.validatePageSendAsNewsletter(sendAsNewsletter, pageType)
	if err != nil {
		return
	}
	if sendAsNewsletter {
		emailsConfig, err := service.emailsService.FindWebsiteConfiguration(ctx, service.db, input.WebsiteID)
		if err != nil {
			return content.Page{}, err
		}
		if !emailsConfig.DomainVerified {
			return content.Page{}, emails.ErrNoCustomEmailDomainConfigured
		}
	}

	// check if path is not already in use
	path := strings.TrimSpace(input.Path)
	err = service.validatePagePath(path)
	if err != nil {
		return
	}

	description := strings.TrimSpace(input.Description)
	err = service.validatePageDescription(description)
	if err != nil {
		return
	}
	if len(description) == 0 {
		description, err = service.getDescriptionFromContentHtml(ctx, service.db, website.ID, bodyHtml)
		if err != nil {
			return
		}
	}

	siteTags, err := service.repo.FindTagsForWebsite(ctx, service.db, website.ID)
	if err != nil {
		return
	}

	tagsDiff, err := service.diffTags([]content.Tag{}, siteTags, input.Tags)
	if err != nil {
		return
	}

	err = service.organizationsService.CheckBillingGatedAction(ctx, service.db, website.OrganizationID, organizations.BillingGatedActionCreatePage{
		WebsiteID: website.ID,
	})
	if err != nil {
		return
	}

	metadataHash := content.HashPageMetadata(pageType, path, date, sendAsNewsletter, language, title, description, input.Tags)

	page = content.Page{
		ID:               guid.NewTimeBased(),
		CreatedAt:        now,
		UpdatedAt:        date,
		Date:             date,
		Type:             pageType,
		Title:            title,
		Path:             path,
		Description:      description,
		Language:         language,
		Size:             size,
		BodyHash:         bodyHash[:],
		MetadataHash:     metadataHash[:],
		Status:           status,
		BodyMarkdown:     bodyMarkdown,
		SendAsNewsletter: sendAsNewsletter,
		NewsletterSentAt: newsletterSentAt,
		WebsiteID:        website.ID,
	}

	var newsletter emails.Newsletter

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreatePage(ctx, tx, page)
		if txErr != nil {
			if db.IsErrAlreadyExists(txErr) {
				return content.ErrPageWithPathAlreadyExists
			}
			return txErr
		}

		txErr = service.associateTagsToPage(ctx, tx, page, tagsDiff)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, website.ID, now)
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
			errMessage := "content.CreatePage: error pushing SendNewsletter job to queue"
			logger.Error(errMessage, slogx.Err(err), slog.String("newsletter.id", newsletter.ID.String()))
			err = nil
		}
	}

	return
}
