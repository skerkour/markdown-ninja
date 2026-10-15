package service

import (
	"context"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContentService) TaskPublishPages(ctx context.Context) {
	logger := slogx.FromCtx(ctx)

	job := queue.NewJobInput{
		Data: content.JobPublishPages{},
	}
	err := service.queue.Push(ctx, nil, job)
	if err != nil {
		errMessage := "content.TaskPublishPages: error pushing PublishPages job to queue"
		logger.Error(errMessage, slogx.Err(err))
		return
	}
}
