package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/countries"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) FindOrCreateContact(ctx context.Context, db db.Queryer, websiteID guid.GUID, email string, subscribedToNewsletter bool) (contact contacts.Contact, err error) {
	contact, err = service.repo.FindContactByEmail(ctx, db, websiteID, email)
	if err == nil {
		return
	} else if err != nil && !errs.IsNotFound(err) {
		return
	}
	err = nil

	createContactInput := contacts.CreateContactInternalInput{
		Email:                  email,
		Name:                   "",
		Verified:               false,
		Country:                countries.CodeUnknown,
		SubscribedToNewsletter: subscribedToNewsletter,
		WebsiteID:              websiteID,
	}
	contact, err = service.CreateContactInternal(ctx, db, createContactInput)
	if err != nil {
		return
	}

	return
}
