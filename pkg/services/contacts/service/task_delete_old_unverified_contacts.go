package service

import (
	"context"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/queue"
)

func (service *ContactsService) TaskDeleteOldUnverifiedContacts(ctx context.Context) (err error) {
	logger := slogx.FromCtx(ctx)

	job := queue.NewJobInput{
		Data: contacts.JobDeleteOldUnverifiedContacts{},
	}
	err = service.queue.Push(ctx, nil, job)
	if err != nil {
		errMessage := "contacts.TaskDeleteOldUnverifiedContacts: Pushing job to queue"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	return
}
