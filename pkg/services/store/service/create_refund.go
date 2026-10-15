package service

import (
	"context"
	"log/slog"
	"time"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *StoreService) CreateRefund(ctx context.Context, input store.CreateRefundInput) (refund store.Refund, err error) {
	httpCtx := httpctx.FromCtx(ctx)

	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	// for now only admins can create refunds
	if !httpCtx.AccessToken.IsAdmin {
		err = kernel.ErrPermissionDenied
		return
	}

	logger := slogx.FromCtx(ctx)

	order, err := service.repo.FindOrderByID(ctx, service.db, input.OrderID, false)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, order.WebsiteID)
	if err != nil {
		return
	}

	if order.Status != store.OrderStatusCompleted {
		err = store.ErrOrderMustBeCompletedToCreateRefund
		return
	}

	previousRefundsForOrder, err := service.repo.FindRefundsByOrderID(ctx, service.db, order.ID)
	if err != nil {
		return
	}

	// As of now, we allow only 1 refund per order
	if len(previousRefundsForOrder) == 0 {
		err = store.ErrOrderAlreadyRefunded
		return
	}

	// var alreadyRefundedAmount int64 = 0
	// for _, previousRefund := range previousRefundsForOrder {
	// 	if previousRefund.Status == store.RefundStatusCanceled || previousRefund.Status == store.RefundStatusFailed {
	// 		continue
	// 	}
	// 	alreadyRefundedAmount += previousRefund.Amount
	// }

	// if (alreadyRefundedAmount + input.Amount) > order.TotalAmount {
	// 	err = store.ErrRefundedAmountCantExceedOrderAmount
	// 	return
	// }

	err = service.validateRefundReason(input.Reason)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	refund = store.Refund{
		ID:             guid.NewTimeBased(),
		CreatedAt:      now,
		UpdatedAt:      now,
		Amount:         input.Amount,
		Currency:       order.Currency,
		Notes:          input.Notes,
		Status:         store.RefundStatusPending,
		Reason:         input.Reason,
		FailureReason:  nil,
		StripeRefundID: nil,
		WebsiteID:      order.WebsiteID,
		OrderID:        order.ID,
	}
	err = service.repo.CreateRefund(ctx, service.db, refund)
	if err != nil {
		return
	}

	job := queue.NewJobInput{
		Data: store.JobCreateStripeRefund{
			RefundID: refund.ID,
		},
	}
	pushJobErr := service.queue.Push(ctx, nil, job)
	if pushJobErr != nil {
		logger.Error("store.CreateRefund: error pushing job to queue",
			slog.String("refund.id", refund.ID.String()), slogx.Err(pushJobErr))
	}

	return
}
