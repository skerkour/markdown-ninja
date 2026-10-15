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
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) CreateSnippet(ctx context.Context, input content.CreateSnippetInput) (snippet content.Snippet, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	_, err = service.kernel.CurrentUserID(ctx)
	if err == nil {
		accessToken := httpCtx.AccessToken
		if !accessToken.IsAdmin {
			return snippet, kernel.ErrPermissionDenied
		}

		// err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
		// if err != nil {
		// 	return
		// }
	} else {
		var website websites.Website
		httpCtx := httpctx.FromCtx(ctx)
		if httpCtx.ApiKey == nil {
			return snippet, kernel.ErrPermissionDenied
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

	now := time.Now().UTC()
	name := strings.TrimSpace(input.Name)
	snippetContent := strings.TrimSpace(input.Content)
	contentHash := blake3.Sum256([]byte(snippetContent))
	renderInEmails := false
	if input.RenderInEmails != nil {
		renderInEmails = *input.RenderInEmails
	}

	err = service.validateSnippetName(name)
	if err != nil {
		return
	}

	err = service.validateSnippetContent(snippetContent)
	if err != nil {
		return
	}

	existingSnippets, err := service.repo.FindSnippetsForWebsite(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}
	if len(existingSnippets) >= 50 {
		err = errs.InvalidArgument("Snippets limit reached. Please contact support if you need more.")
		return
	}

	snippet = content.Snippet{
		ID:             guid.NewTimeBased(),
		CreatedAt:      now,
		UpdatedAt:      now,
		Name:           name,
		Content:        snippetContent,
		Hash:           contentHash[:],
		RenderInEmails: renderInEmails,
		WebsiteID:      input.WebsiteID,
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		txErr = service.repo.CreateSnippet(ctx, tx, snippet)
		if txErr != nil {
			if db.IsErrAlreadyExists(txErr) {
				return content.ErrSnippetWithNameAlreadyExists(name)
			}
			return txErr
		}

		txErr = service.websitesService.UpdateWebsiteModifiedAt(ctx, tx, input.WebsiteID, now)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
