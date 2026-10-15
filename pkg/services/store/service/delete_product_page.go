package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
)

func (service *StoreService) DeleteProductPage(ctx context.Context, input store.DeleteProductPageInput) (err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	pageToDelete, err := service.repo.FindProductPageByID(ctx, service.db, input.ID)
	if err != nil {
		return
	}

	product, err := service.repo.FindProductByID(ctx, service.db, pageToDelete.ProductID)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, product.WebsiteID)
	if err != nil {
		return
	}

	pagesCount, err := service.repo.GetProductPagesCountForProduct(ctx, service.db, pageToDelete.ProductID)
	if err != nil {
		return
	}

	if pagesCount <= 1 {
		err = store.ErrProductShouldHaveAtLeastOnePage
		return
	}

	now := time.Now().UTC()

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		allPages, txErr := service.repo.FindProductPagesForProduct(ctx, tx, product.ID)
		if txErr != nil {
			return txErr
		}

		i := int64(0)
		for _, page := range allPages {
			if page.ID.Equal(pageToDelete.ID) {
				continue
			}
			page.UpdatedAt = now
			page.Position = i
			txErr = service.repo.UpdateProductPage(ctx, tx, page)
			if txErr != nil {
				return txErr
			}

			i += 1
		}

		txErr = service.repo.DeleteProductPage(ctx, tx, pageToDelete.ID)
		return txErr
	})
	if err != nil {
		return
	}

	return
}
