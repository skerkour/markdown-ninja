package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/db"
)

func (service *StoreService) CancelOrder(ctx context.Context, input store.CancelOrderInput) error {
	httpCtx := httpctx.FromCtx(ctx)
	hostname := httpCtx.Hostname

	website, err := service.websitesService.FindWebsiteByDomain(ctx, service.db, hostname)
	if err != nil {
		return err
	}

	err = service.db.Transaction(ctx, func(tx db.Tx) (txErr error) {
		order, txErr := service.repo.FindOrderByID(ctx, tx, input.OrderID, true)
		if txErr != nil {
			return txErr
		}

		if !order.WebsiteID.Equal(website.ID) {
			return store.ErrOrderNotFound(input.OrderID)
		}

		if order.Status == store.OrderStatusCompleted {
			return errs.InvalidArgument("Order cannot be canceled after completion.")
		} else if order.Status == store.OrderStatusCanceled {
			// do nothing
			return nil
		}

		now := time.Now().UTC()
		order.UpdatedAt = now
		order.Status = store.OrderStatusCanceled
		order.CanceledAt = &now
		txErr = service.repo.UpdateOrder(ctx, tx, order)
		if txErr != nil {
			return txErr
		}

		service.eventsService.TrackOrderCanceled(ctx, events.TrackOrderCanceledInput{
			OrderID:   order.ID,
			WebsiteID: order.WebsiteID,
			Country:   httpCtx.Client.CountryCode,
		})

		return nil
	})

	return err
}
