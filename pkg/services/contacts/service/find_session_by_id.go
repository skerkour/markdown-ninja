package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *ContactsService) FindSessionByID(ctx context.Context, db db.Queryer, sessionID guid.GUID) (session contacts.Session, err error) {
	session, err = service.repo.FindSessionByID(ctx, db, sessionID)
	return
}
