package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) FindOrdersForContact(ctx context.Context, db db.Queryer, contactID guid.GUID) (orders []store.Order, err error) {
	orders, err = service.repo.FindOrdersForContact(ctx, db, contactID)
	return
}
