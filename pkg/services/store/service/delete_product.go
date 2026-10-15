package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
)

// TODO: cleanup automatically instad of checking for number of assets, book versions...
func (service *StoreService) DeleteProduct(ctx context.Context, input store.DeleteProductInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	product, err := service.repo.FindProductByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, product.WebsiteID)
	if err != nil {
		return
	}

	orders, err := service.repo.FindOrdersForProduct(ctx, service.db, product.ID)
	if err != nil {
		return
	}

	if len(orders) != 0 {
		err = store.ErrCantDeleteProductWithOrders
		return
	}

	assets, err := service.contentService.FindProductAssets(ctx, service.db, product.ID)
	if err != nil {
		return
	}

	if len(assets) != 0 {
		err = errs.InvalidArgument("A product can't be delete if it has assets")
		return
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (errTx error) {
		productPages, errTx := service.repo.FindProductPagesForProduct(ctx, tx, product.ID)
		if errTx != nil {
			return
		}

		for _, page := range productPages {
			errTx = service.repo.DeleteProductPage(ctx, tx, page.ID)
			if errTx != nil {
				return errTx
			}
		}

		errTx = service.repo.DeleteProduct(ctx, tx, product.ID)
		return errTx
	})
	if err != nil {
		return
	}

	return
}
