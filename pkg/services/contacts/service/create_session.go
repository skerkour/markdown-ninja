package service

import (
	"context"
	"net/http"
	"time"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) CreateSession(ctx context.Context, db db.Queryer, input contacts.CreateSessionInput) (session contacts.Session, cookie *http.Cookie, err error) {
	now := time.Now().UTC()

	if input.Verified {
		// we use time-based UUIDs because we need as much performance as possible
		sessionID := guid.NewTimeBased()
		var ssessionToken newSessionToken

		ssessionToken, err = service.generateSessionToken(input.ContactID, sessionID)
		if err != nil {
			return
		}

		session = contacts.Session{
			ID:                  sessionID,
			CreatedAt:           now,
			UpdatedAt:           now,
			SecretHash:          ssessionToken.hash[:],
			CodeHash:            "",
			FailedLoginAttempts: 0,
			Verified:            true,
			ContactID:           input.ContactID,
			WebsiteID:           input.WebsiteID,
		}
		sessionCookie := service.generateSessionCookie(ssessionToken.token)
		cookie = &sessionCookie
	} else {
		if input.LoginCodeHash == "" {
			// TODO: return error?
		}
		session = contacts.Session{
			ID:                  guid.NewTimeBased(),
			CreatedAt:           now,
			UpdatedAt:           now,
			SecretHash:          []byte{},
			CodeHash:            input.LoginCodeHash,
			FailedLoginAttempts: 0,
			Verified:            false,
			ContactID:           input.ContactID,
			WebsiteID:           input.WebsiteID,
		}
	}
	err = service.repo.CreateSession(ctx, db, session)
	if err != nil {
		return
	}

	return
}
