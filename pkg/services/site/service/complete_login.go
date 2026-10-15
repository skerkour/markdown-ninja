package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/countries"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/db"
)

func (service *SiteService) CompleteLogin(ctx context.Context, input site.CompleteLoginInput) (retContact site.Contact, err error) {
	authenticatedContact := service.contactsService.CurrentContact(ctx)
	if authenticatedContact != nil {
		err = kernel.ErrMustNotBeAuthenticated
		return
	}

	service.kernel.SleepAuth()

	httpCtx := httpctx.FromCtx(ctx)
	sessionID := input.SessionID
	code := strings.ToLower(strings.TrimSpace(input.Code))
	domain := httpCtx.Hostname
	now := time.Now().UTC()

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, domain)
	if err != nil {
		return
	}

	session, err := service.contactsService.FindSessionByID(ctx, service.db, sessionID)
	if err != nil {
		if errors.Is(err, contacts.ErrSessionNotFound) {
			err = site.ErrAuthCodeInvalidOrExpired
			service.kernel.SleepAuthFailure()
		}
		return
	}

	if !session.WebsiteID.Equal(website.ID) {
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	if session.Verified {
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	if session.FailedLoginAttempts >= site.LoginMaxAttempts {
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	since := now.Sub(session.CreatedAt)
	if since >= time.Hour {
		err = site.ErrAuthCodeInvalidOrExpired
		service.kernel.SleepAuthFailure()
		return
	}

	if !crypto.VerifyPasswordHash([]byte(code), session.CodeHash) {
		err = site.ErrAuthCodeInvalidOrExpired
		_ = service.contactsService.FailSessionLoginAttempt(ctx, service.db, session)
		service.kernel.SleepAuthFailure()
		return
	}

	// valdiation has passed, we can generate the session
	contact, err := service.contactsService.FindContact(ctx, service.db, session.ContactID)
	if err != nil {
		return
	}

	country := httpCtx.Client.CountryCode
	if country == countries.CodeUnknown {
		country = contact.Country
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		// mark the contact as verified
		updateContactInput := contacts.UpdateContactInput{
			ID:       contact.ID,
			Country:  &country,
			Verified: new(true),
		}
		txErr = service.contactsService.UpdateContactInternal(ctx, tx, &contact, updateContactInput)
		if txErr != nil {
			return txErr
		}

		txErr = service.contactsService.DeleteOlderVerifiedSessionsForContact(ctx, tx, contact.ID)
		if txErr != nil {
			return txErr
		}

		sessionCookie, txErr := service.contactsService.MarkSessionAsVerified(ctx, tx, &session)
		if txErr != nil {
			return txErr
		}
		httpCtx.Response.Cookies = append(httpCtx.Response.Cookies, sessionCookie)

		return nil
	})
	if err != nil {
		return
	}

	retContact = service.convertContact(contact)

	return
}
