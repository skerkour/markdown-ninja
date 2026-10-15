package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/randutil"
)

func (service *SiteService) Login(ctx context.Context, input site.LoginInput) (ret site.LoginOutput, err error) {
	authenticatedContact := service.contactsService.CurrentContact(ctx)
	if authenticatedContact != nil {
		err = kernel.ErrMustNotBeAuthenticated
		return
	}

	service.kernel.SleepAuth()

	logger := slogx.FromCtx(ctx)
	httpCtx := httpctx.FromCtx(ctx)
	email := strings.ToLower(strings.TrimSpace(input.Email))

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, httpCtx.Hostname)
	if err != nil {
		return
	}

	if !service.rateLimiter.IsAllowed("SiteService.Login", website.ID.Bytes(), httpCtx.Client.IP.AsSlice(), time.Hour, 30) {
		err = errs.TooManyRequests()
		return
	}

	contact, err := service.contactsService.FindContactByEmail(ctx, service.db, website.ID, email)
	if err != nil {
		if errs.IsNotFound(err) {
			err = site.ErrAccountNotFound
			service.kernel.SleepAuthFailure()
		}
		return
	}

	if contact.BlockedAt != nil {
		err = site.ErrAccountBlocked
		return
	}

	// TODO: if contact can log-in: maybe we should mark it as verified
	// if !contact.Verified {
	// 	err = site.ErrAccountNotFound
	// 	return
	// }
	randomGenerator := crypto.NewRandomGenerator()
	codeBytes := randutil.RandAlphabet(randomGenerator, []byte(site.AuthCodeAlphabet), site.AuthCodeLength)
	code := string(codeBytes)
	codeHash := crypto.HashPassword(codeBytes, site.AuthCodeHashParams)

	createSessionInput := contacts.CreateSessionInput{
		Verified:      false,
		LoginCodeHash: codeHash,
		ContactID:     contact.ID,
		WebsiteID:     website.ID,
	}
	session, _, err := service.contactsService.CreateSession(ctx, service.db, createSessionInput)
	if err != nil {
		return
	}

	job := queue.NewJobInput{
		Data: site.JobSendLoginEmail{
			ContactID:     contact.ID,
			Name:          contact.Name,
			Email:         contact.Email,
			Code:          code,
			SessionID:     session.ID,
			WebsiteDomain: website.PrimaryDomain,
			WebsiteID:     website.ID,
		},
	}
	pushJobErr := service.queue.Push(ctx, nil, job)
	if pushJobErr != nil {
		logger.Error("site.Login: Pushing job to queue", slogx.Err(pushJobErr))
	}

	ret.SessionID = session.ID

	return
}
