package scheduler

import (
	"context"

	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (scheduler *Scheduler) organizationsDispatchSendUsageData(ctx context.Context) {
	job := queue.NewJobInput{
		Data:       organizations.JobDispatchSendUsageData{},
		Timeout:    new(int64(300)),
		RetryDelay: new(int64(300)),
	}
	err := scheduler.queue.Push(ctx, nil, job)
	if err != nil {
		logger := slogx.FromCtx(ctx)
		logger.Error("scheduler.DispatchSendUsageData: Pushing job to queue", slogx.Err(err))
		return
	}
}

func (scheduler *Scheduler) organizationsDispatchInvoiceMonthlyUsage(ctx context.Context) {
	job := queue.NewJobInput{
		Data:       organizations.JobDispatchInvoiceMonthlyUsage{},
		Timeout:    new(int64(300)),
		RetryDelay: new(int64(300)),
	}
	err := scheduler.queue.Push(ctx, nil, job)
	if err != nil {
		logger := slogx.FromCtx(ctx)
		logger.Error("scheduler.DispatchInvoiceMonthlyUsage: Pushing job to queue", slogx.Err(err))
		return
	}
}
