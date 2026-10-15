package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) GetVerifiedAndSubscribedToNewsletterContactsCount(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error) {
	count, err = service.repo.GetVerifiedAndSubscribedToNewsletterContactsCount(ctx, service.db, websiteID)
	return
}
