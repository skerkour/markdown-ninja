package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/crypto"
)

func (service *ContactsService) VerifySessionToken(ctx context.Context, tokenStr string) (contactAndSession contacts.ContactAndSession, err error) {
	var jwtClaims sessionJwtClaims

	err = service.jwtProvider.ParseAndVerifyToken(tokenStr, &jwtClaims)
	if err != nil {
		err = kernel.ErrSessionIsNotValid
		return
	}

	contactAndSession, err = service.repo.FindContactWithSession(ctx, service.db, jwtClaims.SessionID)
	if err != nil {
		if errs.IsNotFound(err) {
			service.kernel.SleepAuthFailure()
			err = kernel.ErrSessionIsNotValid
		}
		return
	}

	if !contactAndSession.Session.Verified {
		service.kernel.SleepAuthFailure()
		err = kernel.ErrSessionIsNotValid
		return
	}

	sessionHash := generateSessionHash(contactAndSession.Contact.ID, jwtClaims.SessionID, jwtClaims.Secret)
	if !crypto.ConstantTimeCompare(sessionHash[:], contactAndSession.Session.SecretHash) {
		service.kernel.SleepAuthFailure()
		err = kernel.ErrSessionIsNotValid
		return
	}

	return
}
