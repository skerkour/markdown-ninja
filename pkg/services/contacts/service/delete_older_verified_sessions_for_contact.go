package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) DeleteOlderVerifiedSessionsForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (err error) {
	err = service.repo.DeleteOlderVerifiedSessionsForContact(ctx, db, contactID, 10)
	return
}
