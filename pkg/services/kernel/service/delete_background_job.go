package service

import (
	"context"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/queue"
)

func (service *KernelService) DeleteBackgroundJob(ctx context.Context, input kernel.DeleteBackgroundJobInput) (err error) {
	httpCtx := httpctx.FromCtx(ctx)

	_, err = service.CurrentUserID(ctx)
	if err != nil {
		return err
	}

	if !httpCtx.AccessToken.IsAdmin {
		service.SleepAuth()
		err = kernel.ErrPermissionDenied
		return err
	}

	job, err := service.queue.GetJob(ctx, input.JobID)
	if err != nil {
		return err
	}

	if job.Status != queue.JobStatusFailed {
		err = kernel.ErrOnlyFailedJobsCanBeDeleted
		return err
	}

	err = service.queue.DeleteJob(ctx, input.JobID)
	return err
}
