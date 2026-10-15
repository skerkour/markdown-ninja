package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/crypto"
)

func (service *ContactsService) VerifyEmail(ctx context.Context, input contacts.VerifyEmailInput) (err error) {
	contact := service.CurrentContact(ctx)
	if contact == nil {
		err = kernel.ErrAuthenticationRequired
		return
	}

	var jwtClaims jwtClaimsUpdateEmail
	err = service.jwtProvider.ParseAndVerifyToken(input.Token, &jwtClaims)
	if err != nil {
		err = contacts.ErrUpdateEmailTokenIsNotValid
		return
	}

	if jwtClaims.Action != jwtActionUpdateEmail ||
		!jwtClaims.ContactID.Equal(contact.ID) ||
		!crypto.ConstantTimeCompare([]byte(jwtClaims.OldEmail), []byte(contact.Email)) {
		err = contacts.ErrUpdateEmailTokenIsNotValid
		return
	}

	contact.UpdatedAt = time.Now().UTC()
	contact.Email = jwtClaims.NewEmail
	err = service.repo.UpdateContact(ctx, service.db, *contact)
	if err != nil {
		return
	}

	return
}
