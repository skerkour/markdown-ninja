package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/countries"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/db"
)

// TODO: event
func (service *SiteService) CompleteSubscription(ctx context.Context, input site.CompleteSubscriptionInput) (retContact site.Contact, err error) {
	authenticatedContact := service.contactsService.CurrentContact(ctx)
	if authenticatedContact != nil {
		err = kernel.ErrMustNotBeAuthenticated
		return
	}

	service.kernel.SleepAuth()
	httpCtx := httpctx.FromCtx(ctx)
	contactID := input.ContactID
	code := strings.ToLower(strings.TrimSpace(input.Code))
	domain := httpCtx.Hostname
	now := time.Now().UTC()

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, domain)
	if err != nil {
		return
	}

	contact, err := service.contactsService.FindContact(ctx, service.db, contactID)
	if err != nil {
		return
	}

	if contact.Verified {
		err = site.ErrAccountAlreadyExists
		return
	}

	if !contact.WebsiteID.Equal(website.ID) {
		err = site.ErrAccountNotFound
		service.kernel.SleepAuthFailure()
		return
	}

	if contact.FailedSignupAttempts >= site.SignupMaxAttempts {
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	since := now.Sub(contact.UpdatedAt)
	if since >= time.Hour {
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	if !crypto.VerifyPasswordHash([]byte(code), contact.SignupCodeHash) {
		err = kernel.ErrAuthCodeIsNotValid
		// session.UpdatedAt = now
		updateContactInput := contacts.UpdateContactInput{
			ID:                   contact.ID,
			FailedSignupAttempts: new(contact.FailedSignupAttempts + 1),
		}
		err = service.contactsService.UpdateContactInternal(ctx, service.db, &contact, updateContactInput)
		if err != nil {
			return
		}
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	country := httpCtx.Client.CountryCode
	if country == countries.CodeUnknown {
		country = contact.Country
	}

	var sessionCookie *http.Cookie
	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		updateContactInput := contacts.UpdateContactInput{
			ID:                     contact.ID,
			SubscribedToNewsletter: new(true),
			Country:                &country,
			FailedSignupAttempts:   new(int64(0)),
			SignupCodeHash:         new(""),
			Verified:               new(true),
		}
		txErr = service.contactsService.UpdateContactInternal(ctx, tx, &contact, updateContactInput)
		if txErr != nil {
			return txErr
		}

		createSessionInput := contacts.CreateSessionInput{
			Verified:  true,
			ContactID: contact.ID,
			WebsiteID: website.ID,
		}
		_, sessionCookie, txErr = service.contactsService.CreateSession(ctx, tx, createSessionInput)
		return txErr
	})
	if err != nil {
		return
	}

	if sessionCookie != nil {
		httpCtx.Response.Cookies = append(httpCtx.Response.Cookies, *sessionCookie)
	}

	retContact = service.convertContact(contact)

	trackEventInput := events.TrackSubscribedToNewsletterInput{
		WebsiteID: website.ID,
	}
	service.eventsService.TrackSubscribedToNewsletter(ctx, trackEventInput)

	return
}
