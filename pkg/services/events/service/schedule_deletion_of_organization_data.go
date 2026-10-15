package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/queue"
)

func (service *Service) ScheduleDeletionOfOrganizationData(ctx context.Context, tx db.Queryer, organizationID guid.GUID) (err error) {
	in10Minutes := time.Now().UTC().Add(10 * time.Minute)

	job := queue.NewJobInput{
		ScheduledFor: &in10Minutes,
		Data: events.JobDeleteOrganizationEvents{
			OrganizationID: organizationID,
		},
		Timeout:    new(int64(1200)),
		RetryDelay: new(int64(3600)),
	}
	err = service.queue.Push(ctx, tx, job)
	if err != nil {
		return
	}
	return
}
