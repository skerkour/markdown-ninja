package service

import (
	"context"

	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/queue"
)

func (service *KernelService) ListFailedBackgroundJobs(ctx context.Context, input kernel.EmptyInput) (ret kernel.PaginatedResult[queue.Job], err error) {
	httpCtx := httpctx.FromCtx(ctx)

	_, err = service.CurrentUserID(ctx)
	if err != nil {
		return
	}

	if !httpCtx.AccessToken.IsAdmin {
		service.SleepAuth()
		err = kernel.ErrPermissionDenied
		return
	}

	ret.Data, err = service.queue.GetFailedJobs(ctx, 1000)
	return
}
