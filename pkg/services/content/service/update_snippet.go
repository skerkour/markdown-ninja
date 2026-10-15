package service

import (
	"context"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
)

func (service *ContentService) UpdateSnippet(ctx context.Context, input content.UpdateSnippetInput) (snippet content.Snippet, err error) {
	snippet, err = service.repo.FindSnippetByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err == nil {
		err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, snippet.WebsiteID)
		if err != nil {
			return
		}
	} else {
		var website websites.Website
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			err = kernel.ErrPermissionDenied
			return
		}

		website, err = service.websitesService.FindWebsiteByID(ctx, service.db, snippet.WebsiteID)
		if err != nil {
			return
		}

		_, err = service.organizationsService.CheckCurrentApiKey(ctx, website.OrganizationID)
		if err != nil {
			return
		}
	}

	now := time.Now().UTC()
	name := strings.TrimSpace(input.Name)
	snippetContent := strings.TrimSpace(input.Content)

	err = service.validateSnippetName(name)
	if err != nil {
		return
	}

	// check that snippet with smae name doesn't already exists
	existingSnippet, err := service.repo.FindSnippetByName(ctx, service.db, snippet.WebsiteID, name)
	if err == nil && !snippet.ID.Equal(existingSnippet.ID) {
		err = content.ErrSnippetWithNameAlreadyExists(name)
		return
	} else if err != nil {
		if errs.IsNotFound(err) {
			err = nil
		}
	}
	if err != nil {
		return
	}

	err = service.validateSnippetContent(snippetContent)
	if err != nil {
		return
	}

	snippet.UpdatedAt = now
	snippet.Name = name
	snippet.Content = snippetContent
	contentHash := blake3.Sum256([]byte(snippetContent))
	snippet.Hash = contentHash[:]
	if input.RenderInEmails != nil {
		snippet.RenderInEmails = *input.RenderInEmails
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.UpdateSnippet(ctx, tx, snippet)
		if txErr != nil {
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, snippet.WebsiteID, now)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
