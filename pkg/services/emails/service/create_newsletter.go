package service

import (
	"context"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/guid"
)

// TODO: increase website's used storage? see also DeleteNewsletter and UpdateNewsletter
func (service *EmailsService) CreateNewsletter(ctx context.Context, input emails.CreateNewsletterInput) (newsletter emails.Newsletter, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return
	}

	website, err := service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	subject := strings.TrimSpace(input.Subject)
	scheduledFor := input.ScheduledFor

	bodyMarkdown := input.BodyMarkdown
	err = service.validateNewsletterBodyMarkdown(bodyMarkdown)
	if err != nil {
		return
	}

	size := int64(len(bodyMarkdown))
	bodyHash := blake3.Sum256([]byte(bodyMarkdown))

	if scheduledFor != nil {
		scheduledForUtc := scheduledFor.UTC()
		scheduledFor = &scheduledForUtc
		err = service.validateNewsletterScheduledFor(scheduledFor)
		if err != nil {
			return
		}
	}

	err = service.validateNewsletterSubject(subject)
	if err != nil {
		return
	}

	newsletter = emails.Newsletter{
		ID:             guid.NewTimeBased(),
		CreatedAt:      now,
		UpdatedAt:      now,
		ScheduledFor:   scheduledFor,
		Subject:        subject,
		Size:           size,
		Hash:           bodyHash[:],
		SentAt:         nil,
		LastTestSentAt: nil,
		BodyMarkdown:   bodyMarkdown,
		WebsiteID:      website.ID,
		PostID:         nil,
	}
	err = service.repo.CreateNewsletter(ctx, service.db, newsletter)
	if err != nil {
		return
	}

	return
}
