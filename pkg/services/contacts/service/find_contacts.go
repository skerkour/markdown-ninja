package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) FindContactsByEmail(ctx context.Context, db db.Queryer, websiteID guid.GUID, emails []string) (contacts []contacts.Contact, err error) {
	contacts, err = service.repo.FindContactsByEmails(ctx, db, websiteID, emails)
	return
}
