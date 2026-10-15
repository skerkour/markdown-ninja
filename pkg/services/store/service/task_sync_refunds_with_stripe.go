package service

import (
	"context"

	"markdown.ninja/pkg/services/store"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

// TODO: batch push jobs
func (service *StoreService) TaskSyncRefundsWithStripe(ctx context.Context) {
	logger := slogx.FromCtx(ctx)

	pendingRefunds, err := service.repo.FindPendingRefunds(ctx, service.db)
	if err != nil {
		logger.Error("stroe.TaskSyncRefundsFromStripe: error finding pending refunds", slogx.Err(err))
		return
	}

	for _, refund := range pendingRefunds {
		job := queue.NewJobInput{
			Data: store.JobSyncRefundWithStripe{
				RefundID: refund.ID,
			},
		}
		err := service.queue.Push(ctx, nil, job)
		if err != nil {
			logger.Error("stroe.TaskSyncRefundsFromStripe: Pushing job to queue", slogx.Err(err))
			continue
		}
	}
}
