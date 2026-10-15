package service

import (
	"context"
	"fmt"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/queue"
)

// TODO: handle case where newsletter is created but sending newsletter job fails
func (service *EmailsService) JobSendPostAsNewsletter(ctx context.Context, input emails.JobSendPostAsNewsletter) error {
	now := time.Now().UTC()

	post, err := service.contentService.FindPageByID(ctx, service.db, input.PostID)
	if err != nil {
		return err
	}

	hostname := input.WebsiteDomain + service.httpConfig.WebsitesPort
	bodyMarkdown := fmt.Sprintf(`[Read online](%s://%s%s)
	<br />
	%s`, service.httpConfig.WebsitesBaseUrl.Scheme, hostname, post.Path, post.BodyMarkdown)
	size := int64(len(bodyMarkdown))
	bodyHash := blake3.Sum256([]byte(bodyMarkdown))

	newsletter := emails.Newsletter{
		ID:             guid.NewTimeBased(),
		CreatedAt:      now,
		UpdatedAt:      now,
		ScheduledFor:   &now,
		Subject:        post.Title,
		Size:           size,
		Hash:           bodyHash[:],
		SentAt:         &now,
		LastTestSentAt: nil,
		BodyMarkdown:   bodyMarkdown,
		WebsiteID:      post.WebsiteID,
		PostID:         &post.ID,
	}
	err = service.repo.CreateNewsletter(ctx, service.db, newsletter)
	if err != nil {
		return err
	}

	job := queue.NewJobInput{
		Data: emails.JobSendNewsletter{
			NewsletterID: newsletter.ID,
			Test:         false,
			SentAt:       now,
		},
		Timeout: new(int64(600)),
	}
	err = service.queue.Push(ctx, nil, job)
	if err != nil {
		return fmt.Errorf("error pushing SendNewsletter job to queue: %w", err)
	}

	return nil
}
