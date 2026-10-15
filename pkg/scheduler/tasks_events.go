package scheduler

import (
	"context"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (scheduler *Scheduler) eventsDispatchRotateAnonymousIDSalt(ctx context.Context) {
	job := queue.NewJobInput{
		Data: events.JobRotateAnonymousIDSalt{},
	}
	err := scheduler.queue.Push(ctx, nil, job)
	if err != nil {
		logger := slogx.FromCtx(ctx)
		logger.Error("scheduler.DispatchRotateAnonymousIDSalt: Pushing job to queue", slogx.Err(err))
		return
	}
}
