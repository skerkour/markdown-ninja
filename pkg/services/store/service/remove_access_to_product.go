package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/slicesx"
)

func (service *StoreService) RemoveAccessToProduct(ctx context.Context, input store.RemoveAccessToProductInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	product, err := service.repo.FindProductByID(ctx, service.db, input.ProductID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, product.WebsiteID)
	if err != nil {
		return
	}

	emails := slicesx.Unique(input.Emails)

	err = service.db.Transaction(ctx, func(tx db.Tx) (errTx error) {
		// TODO: improve performance
		for _, email := range emails {
			var contact contacts.Contact
			var productAccess store.ContactProductAccess
			contact, errTx = service.contactsService.FindContactByEmail(ctx, tx, product.WebsiteID, email)
			if errTx != nil {
				return errTx
			}

			productAccess, errTx = service.repo.FindContactProductAccess(ctx, tx, contact.ID, product.ID)
			if errTx != nil {
				return errTx
			}

			errTx = service.repo.DeleteAccessToProduct(ctx, tx, productAccess)
			if errTx != nil {
				return errTx
			}
		}

		return nil
	})

	return
}
