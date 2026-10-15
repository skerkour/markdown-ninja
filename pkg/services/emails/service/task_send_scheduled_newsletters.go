package service

import (
	"context"
	"time"

	"log/slog"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *EmailsService) TaskSendScheduledNewsletters(ctx context.Context) {
	logger := slogx.FromCtx(ctx)
	now := time.Now().UTC()
	newsletters, err := service.repo.FindScheduledNewsletters(ctx, service.db, now)
	if err != nil {
		logger.Error("emails.TaskSendScheduledNewsletters: error finding scheduled newsletters", slogx.Err(err))
		return
	}

	for _, newsletter := range newsletters {
		newsletter.UpdatedAt = now
		newsletter.SentAt = &now

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
			errMessage := "emails.TaskSendScheduledNewsletters: error pushing SendNewsletter job to queue"
			logger.Error(errMessage, slogx.Err(err), slog.String("newsletter.id", newsletter.ID.String()))
			continue
		}

		err = service.repo.UpdateNewsletter(ctx, service.db, newsletter)
		if err != nil {
			continue
		}
	}
}
