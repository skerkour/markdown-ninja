package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) FindVerifiedAndSubscribedToNewsletterContacts(ctx context.Context, db db.Queryer, websiteID guid.GUID) (contacts []contacts.Contact, err error) {
	contacts, err = service.repo.FindVerifiedAndSubscribedToNewsletterContacts(ctx, db, websiteID)
	return
}
