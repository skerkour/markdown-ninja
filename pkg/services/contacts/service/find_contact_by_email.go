package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) FindContactByEmail(ctx context.Context, db db.Queryer, websiteID guid.GUID, email string) (contact contacts.Contact, err error) {
	contact, err = service.repo.FindContactByEmail(ctx, db, websiteID, email)
	return
}
