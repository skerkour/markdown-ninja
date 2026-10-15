package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) FindContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (contact contacts.Contact, err error) {
	contact, err = service.repo.FindContactByID(ctx, db, contactID)
	return
}
