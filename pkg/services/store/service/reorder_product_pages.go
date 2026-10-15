package service

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *StoreService) ReorderProductPages(ctx context.Context, input store.ReorderProductPagesInput) (err error) {
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

	if product.Type != store.ProductTypeCourse {
		err = store.ErrProductIsNotACourse
		return
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		allPages, txErr := service.repo.FindProductPagesForProduct(ctx, tx, product.ID)
		if txErr != nil {
			return txErr
		}

		if len(input.Pages) != len(allPages) {
			txErr = store.ErrAllPagesMusBeProvidedForReordering
			return txErr
		}

		// check for duplicates
		pagesSet := make(map[guid.GUID]bool, 0)
		for _, pageID := range input.Pages {
			if pagesSet[pageID] {
				txErr = store.ErrDuplicatePageFound
				return txErr
			}
			pagesSet[pageID] = true
		}

		now := time.Now().UTC()
		for _, page := range allPages {
			newPosition := slices.IndexFunc(input.Pages, func(pageID guid.GUID) bool {
				return page.ID.Equal(pageID)
			})
			if newPosition == -1 {
				txErr = store.ErrProductPageNotFound
				return txErr
			}

			page.Position = int64(newPosition)
			page.UpdatedAt = now
			txErr = service.repo.UpdateProductPage(ctx, tx, page)
			if txErr != nil {
				return txErr
			}
		}

		return nil
	})
	if err != nil {
		return
	}

	return
}
