package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) DeleteContact(ctx context.Context, input contacts.DeleteContactInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	contact, err := service.repo.FindContactByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, contact.WebsiteID)
	if err != nil {
		return
	}

	err = service.DeleteContactInternal(ctx, service.db, contact.ID, contact.WebsiteID)
	return err
}

// TODO: track account deleted event instead of unsubscribed event
// TODO: delete all events for contact?
func (service *ContactsService) DeleteContactInternal(ctx context.Context, db db.Queryer, contactID, websiteID guid.GUID) (err error) {

	orders, err := service.storeService.FindOrdersForContact(ctx, service.db, contactID)
	if err != nil {
		return
	}

	if len(orders) != 0 {
		return errs.InvalidArgument(`It's currently not possible to delete an account that has placed at least one order.
		If you want to delete your personal information from our systems, you can update your information with random data.`)
	}

	err = service.repo.DeleteContact(ctx, service.db, contactID)
	if err != nil {
		return
	}

	eventInput := events.TrackUnsubscribedFromNewsletterInput{
		WebsiteID: websiteID,
	}
	service.eventsService.TrackUnsubscribedFromNewsletter(ctx, eventInput)

	return
}
