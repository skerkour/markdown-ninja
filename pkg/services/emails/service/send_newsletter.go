package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *EmailsService) SendNewsletter(ctx context.Context, input emails.SendNewsletterInput) (newsletter emails.Newsletter, err error) {
	logger := slogx.FromCtx(ctx)
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	newsletter, err = service.repo.FindNewsletterByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	website, err := service.websitesService.FindWebsiteByID(ctx, service.db, newsletter.WebsiteID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, newsletter.WebsiteID)
	if err != nil {
		return
	}

	if newsletter.SentAt != nil {
		err = emails.ErrNewsletterAlreadySent
		return
	}

	emailConfig, err := service.repo.FindWebsiteConfiguration(ctx, service.db, newsletter.WebsiteID)
	if err != nil {
		return
	}

	if !emailConfig.DomainVerified {
		err = emails.ErrNoCustomEmailDomainConfigured
		return
	}

	if !input.Test {
		err = service.organizationsService.CheckBillingGatedAction(ctx, service.db, website.OrganizationID, organizations.BillingGatedActionSendNewsletter{})
		if err != nil {
			return
		}
	}

	now := time.Now().UTC()
	newsletter.UpdatedAt = now
	if input.Test {
		if newsletter.LastTestSentAt != nil && newsletter.LastTestSentAt.After(now.Add(time.Minute)) {
			err = errs.InvalidArgument("Please wait a minute before send a new test newsletter.")
			return
		}

		newsletter.LastTestSentAt = &now
	} else {
		newsletter.SentAt = &now
		newsletter.ScheduledFor = nil // TODO: or &now?
	}

	testEmails := make([]string, 0, 1)
	if input.Test {
		testEmails = append(testEmails, httpCtx.AccessToken.Email)
	}
	job := queue.NewJobInput{
		Data: emails.JobSendNewsletter{
			NewsletterID: newsletter.ID,
			Test:         input.Test,
			TestEmails:   testEmails,
			SentAt:       now,
		},
		Timeout: new(int64(600)),
	}

	err = service.queue.Push(ctx, nil, job)
	if err != nil {
		errMessage := "emails.SendNewsletter: Pushing SendNewsletter job to queue"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	err = service.repo.UpdateNewsletter(ctx, service.db, newsletter)
	if err != nil {
		return
	}

	return
}
