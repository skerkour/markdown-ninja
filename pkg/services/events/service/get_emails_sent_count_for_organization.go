package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (service *Service) GetEmailsSentCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID, from, to time.Time) (count int64, err error) {
	return service.repo.GetEventsTypeCountForOrganization(ctx, db, events.EventTypeEmailSent, organizationID, from, to)
}
