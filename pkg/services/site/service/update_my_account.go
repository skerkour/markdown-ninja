package service

import (
	"context"
	"strings"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *SiteService) UpdateMyAccount(ctx context.Context, input site.UpdateMyAccount) (retContact site.Contact, err error) {
	contact := service.contactsService.CurrentContact(ctx)
	if contact == nil {
		err = kernel.ErrAuthenticationRequired
		return
	}

	unsubscribedFromNewsletter := false
	subscribedToNewsletter := true
	httpCtx := httpctx.FromCtx(ctx)
	domain := httpCtx.Hostname
	sendVerifyEmailEmail := false
	var verifyEmailLink string
	var newEmail string

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, domain)
	if err != nil {
		return
	}

	if input.Email != nil {
		newEmail = strings.ToLower(strings.TrimSpace(*input.Email))
		if newEmail != contact.Email {
			// check if email is already in use
			_, err = service.contactsService.FindContactByEmail(ctx, service.db, website.ID, newEmail)
			if err != nil {
				if errs.IsNotFound(err) {
					err = nil
				} else {
					return
				}
			} else {
				err = site.ErrAccountAlreadyExists
				return
			}

			verifyEmailLink, err = service.contactsService.GenerateVerifyEmailLink(website.PrimaryDomain, contact.ID, contact.Email, newEmail)
			if err != nil {
				return
			}
			sendVerifyEmailEmail = true
		}
	}

	if input.SubscribedToNewsletter != nil {
		if input.SubscribedToNewsletter != nil {
			if *input.SubscribedToNewsletter && contact.SubscribedToNewsletterAt == nil {
				subscribedToNewsletter = true
			} else if *input.SubscribedToNewsletter == false && contact.SubscribedToNewsletterAt != nil {
				unsubscribedFromNewsletter = true
			}
		}
	}

	updateContactInput := contacts.UpdateContactInput{
		ID:                     contact.ID,
		Name:                   input.Name,
		SubscribedToNewsletter: input.SubscribedToNewsletter,
		BillingAddress:         input.BillingAddress,
	}
	err = service.contactsService.UpdateContactInternal(ctx, service.db, contact, updateContactInput)
	if err != nil {
		return
	}

	// contact.UpdatedAt = now
	// err = service.contactsService.UpdateContactInternal(ctx, service.db, *contact)
	// if err != nil {
	// 	return
	// }

	retContact = service.convertContact(*contact)

	if sendVerifyEmailEmail {
		job := queue.NewJobInput{
			Data: contacts.JobSendVerifyEmailEmail{
				Name:            contact.Name,
				Email:           newEmail,
				WebsiteID:       website.ID,
				ContactID:       contact.ID,
				VerifyEmailLink: verifyEmailLink,
			},
		}
		pushJobErr := service.queue.Push(ctx, nil, job)
		if pushJobErr != nil {
			logger := slogx.FromCtx(ctx)
			logger.Error("site.UpdateMyAccount: Pushing JobSendVerifyEmailEmail job to queue", slogx.Err(pushJobErr))
		}
	}

	// we track event at the end to be sure that the transaction succeeded
	if subscribedToNewsletter {
		trackEventInput := events.TrackSubscribedToNewsletterInput{
			WebsiteID: website.ID,
		}
		service.eventsService.TrackSubscribedToNewsletter(ctx, trackEventInput)
	} else if unsubscribedFromNewsletter {
		trackEventInput := events.TrackUnsubscribedFromNewsletterInput{
			WebsiteID: website.ID,
		}
		service.eventsService.TrackUnsubscribedFromNewsletter(ctx, trackEventInput)
	}

	return
}
