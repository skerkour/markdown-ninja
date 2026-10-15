package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/queue"
)

func (service *Service) ScheduleDeletionOfWebsiteData(ctx context.Context, tx db.Queryer, websiteID guid.GUID) (err error) {
	in10Minutes := time.Now().UTC().Add(10 * time.Minute)

	job := queue.NewJobInput{
		ScheduledFor: &in10Minutes,
		Data: events.JobDeleteWebsiteEvents{
			WebsiteID: websiteID,
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
