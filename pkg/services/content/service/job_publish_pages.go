package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

type postToSendAsNewsletter struct {
	postID        guid.GUID
	websiteDomain string
}

func (service *ContentService) JobPublishPages(ctx context.Context, input content.JobPublishPages) (err error) {
	logger := slogx.FromCtx(ctx)

	now := time.Now().UTC()

	var postsToSendAsNewsletter []postToSendAsNewsletter

	// TODO: use a transaction per page instead of for all pages?
	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		pagesToPublish, txErr := service.repo.FindScheduledPagesToPublish(ctx, tx, true)
		if txErr != nil {
			txErr = fmt.Errorf("finding pages to publish: %w", txErr)
			return txErr
		}

		postsToSendAsNewsletter = make([]postToSendAsNewsletter, 0, len(pagesToPublish))

		for _, page := range pagesToPublish {
			if page.SendAsNewsletter && page.NewsletterSentAt == nil {
				var website websites.Website
				website, txErr = service.websitesService.FindWebsiteByID(ctx, tx, page.WebsiteID)
				if txErr != nil {
					txErr = fmt.Errorf("creating newsletter from page (%s): %w", page.ID.String(), txErr)
					return txErr
				}

				postToSend := postToSendAsNewsletter{
					postID:        page.ID,
					websiteDomain: website.PrimaryDomain,
				}
				postsToSendAsNewsletter = append(postsToSendAsNewsletter, postToSend)
				page.NewsletterSentAt = &now
			}
			// page.UpdatedAt = now
			page.Status = content.PageStatusPublished
			txErr = service.repo.UpdatePage(ctx, tx, page)
			if txErr != nil {
				txErr = fmt.Errorf("updating page (%s): %w", page.ID.String(), txErr)
				return txErr
			}
		}

		return nil
	})
	if err != nil {
		logger.Error("content.JobPublishPages: error publishing pages", slogx.Err(err))
		return
	}

	// TODO: batch push and improve reliability
	for _, post := range postsToSendAsNewsletter {
		// send job to send newsletter
		job := queue.NewJobInput{
			Data: emails.JobSendPostAsNewsletter{
				PostID:        post.postID,
				WebsiteDomain: post.websiteDomain,
			},
		}
		err = service.queue.Push(ctx, nil, job)
		if err != nil {
			errMessage := "content.JobPublishPages: error pushing SendPostAsNewsletter job to queue"
			logger.Error(errMessage, slogx.Err(err), slog.String("post.id", post.postID.String()))
			continue
		}
	}

	return nil
}
