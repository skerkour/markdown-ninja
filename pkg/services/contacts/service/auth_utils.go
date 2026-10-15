package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/jwt"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/guid"
)

const (
	sessionSecretSize = 32
	sessionHashSize   = 32
)

type sessionJwtClaims struct {
	ContactID guid.GUID `json:"contact_id"`
	SessionID guid.GUID `json:"session_id"`
	Secret    []byte    `json:"secret"`
}

type newSessionToken struct {
	token string
	hash  [sessionHashSize]byte
}

func (service *ContactsService) CurrentContact(ctx context.Context) (contact *contacts.Contact) {
	httpCtx := httpctx.FromCtx(ctx)

	return httpCtx.Contact
}

func (service *ContactsService) generateSessionCookie(sessionToken string) (cookie http.Cookie) {
	cookie = http.Cookie{
		Name:     contacts.AuthCookie,
		Value:    sessionToken,
		Expires:  time.Now().Add(contacts.AuthCookieTimeout),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	}
	return
}

func (service *ContactsService) GenerateLogoutCookie() (cookie http.Cookie) {
	cookie = http.Cookie{
		Name:     contacts.AuthCookie,
		Value:    "",
		Expires:  time.Unix(0, 0),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	}
	return
}

func (service *ContactsService) generateSessionToken(contactID, sessionID guid.GUID) (token newSessionToken, err error) {
	// Add one hour to account for clock drift
	sessionJwtExpiresAt := time.Now().UTC().Add(contacts.AuthCookieTimeout).Add(1 * time.Hour)

	// secret
	var secret [sessionSecretSize]byte

	_, err = rand.Read(secret[:])
	if err != nil {
		err = fmt.Errorf("error generating new session secret: %w", err)
		return
	}

	// token
	jwtClaims := sessionJwtClaims{
		ContactID: contactID,
		SessionID: sessionID,
		Secret:    secret[:],
	}
	token.token, err = service.jwtProvider.NewSignedToken(jwtClaims, &jwt.TokenOptions{
		ExpirationTime: &sessionJwtExpiresAt,
	})
	if err != nil {
		err = fmt.Errorf("generating session JWT: %w", err)
		return
	}

	// hash
	token.hash = generateSessionHash(contactID, sessionID, secret[:])

	return token, nil
}

func generateSessionHash(contactID, sessionID guid.GUID, secret []byte) (out [sessionHashSize]byte) {
	hasher := blake3.New()
	hasher.Write(contactID.Bytes())
	hasher.Write(sessionID.Bytes())
	hasher.Write(secret)
	hasher.Sum(out[:0])
	return
}
