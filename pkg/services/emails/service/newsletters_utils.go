package service

import (
	"fmt"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/guid"
)

func convertNewsletterMetadata(input []emails.Newsletter) (output []emails.NewsletterMetadata) {
	output = make([]emails.NewsletterMetadata, len(input))

	for i, item := range input {
		output[i] = emails.NewsletterMetadata{
			ID:             item.ID,
			CreatedAt:      item.CreatedAt,
			UpdatedAt:      item.UpdatedAt,
			ScheduledFor:   item.ScheduledFor,
			Subject:        item.Subject,
			Size:           item.Size,
			Hash:           item.Hash,
			SentAt:         item.SentAt,
			LastTestSentAt: item.LastTestSentAt,
		}
	}

	return
}

func getWebsiteConfigurationCacheKey(websiteID guid.GUID) string {
	return fmt.Sprintf("WebsiteEmailConfiguration:%s", websiteID.String())
}

func getSenderApiTokenCacheKey(websiteID guid.GUID) string {
	return fmt.Sprintf("SenderApiToken:%s", websiteID.String())
}
