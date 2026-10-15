package service

import (
	"context"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) DeleteSession(ctx context.Context, db db.Queryer, sessionID guid.GUID) (err error) {
	err = service.repo.DeleteSession(ctx, db, sessionID)
	return
}
