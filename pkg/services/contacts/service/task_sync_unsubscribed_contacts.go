package service

import (
	"context"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContactsService) TaskSyncUnsubscribedContacts(ctx context.Context) {
	logger := slogx.FromCtx(ctx)

	job := queue.NewJobInput{
		Data:    contacts.JobSyncUnsubscribedContacts{},
		Timeout: new(int64(120)),
	}
	err := service.queue.Push(ctx, nil, job)
	if err != nil {
		logger.Error("contacts.TaskDeleteOldUnverifiedSessions: Pushing job to queue", slogx.Err(err))
	}
}
