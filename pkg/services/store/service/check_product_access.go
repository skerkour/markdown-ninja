package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) CheckProductAccess(ctx context.Context, db db.Queryer, productID guid.GUID) (err error) {
	currentContact := service.contactsService.CurrentContact(ctx)
	if currentContact != nil {
		// check if contact has access to product
		_, err = service.repo.FindContactProductAccess(ctx, db, currentContact.ID, productID)
		if err != nil {
			if errs.IsNotFound(err) {
				return store.ErrProductNotFound
			}

			return err
		}
	} else {
		// check that the authenticated user has access to product
		var product store.Product
		currentUserID, err := service.kernel.CurrentUserID(ctx)
		if err != nil {
			return err
		}

		product, err = service.repo.FindProductByID(ctx, service.db, productID)
		if err != nil {
			return err
		}

		err = service.websitesService.CheckUserIsStaff(ctx, service.db, currentUserID, product.WebsiteID)
		if err != nil {
			return err
		}
	}

	return nil
}
