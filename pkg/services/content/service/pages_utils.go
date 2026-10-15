package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContentService) getDescriptionFromContentHtml(ctx context.Context, db db.DB, websiteID guid.GUID, contentHtml string) (description string, err error) {
	if strings.Contains(contentHtml, "{{<") {
		var snippets []content.Snippet

		snippets, err = service.repo.FindSnippetsForWebsite(ctx, db, websiteID)
		if err != nil {
			return
		}
		if len(snippets) != 0 {
			contentHtml = service.RenderSnippets(contentHtml, snippets, false)
		}
	}

	strippedHtml := service.htmlStripper.Sanitize(contentHtml)
	words := strings.Fields(strippedHtml)

	n := len(words)
	if n > 30 {
		n = 30
	}

	description = strings.Join(words[:n], " ")
	return
}

func (service *ContentService) convertPageMetadata(input content.Page) (output content.PageMetadata) {
	return content.PageMetadata{
		ID:               input.ID,
		CreatedAt:        input.CreatedAt.Truncate(time.Minute),
		UpdatedAt:        input.UpdatedAt.Truncate(time.Minute),
		Date:             input.Date.Truncate(time.Minute),
		Type:             input.Type,
		Title:            input.Title,
		Path:             input.Path,
		Language:         input.Language,
		Size:             input.Size,
		BodyHash:         input.BodyHash,
		Status:           input.Status,
		SendAsNewsletter: input.SendAsNewsletter,
		NewsletterSentAt: input.NewsletterSentAt,
	}
}

func (service *ContentService) convertPageMetadataMany(input []content.Page) (output []content.PageMetadata) {
	if input == nil {
		return nil
	}

	output = make([]content.PageMetadata, len(input))

	for i, content := range input {
		output[i] = service.convertPageMetadata(content)
	}
	return
}
