package events

import (
	"context"
	"time"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type Service interface {
	Push(ctx context.Context, event Event)
	TrackPageView(ctx context.Context, input TrackPageViewInput)
	TrackEmailSent(ctx context.Context, input TrackEmailSentInput)
	TrackSubscribedToNewsletter(ctx context.Context, input TrackSubscribedToNewsletterInput)
	TrackUnsubscribedFromNewsletter(ctx context.Context, input TrackUnsubscribedFromNewsletterInput)
	TrackOrderPlaced(ctx context.Context, input TrackOrderPlacedInput)
	TrackOrderCompleted(ctx context.Context, input TrackOrderCompletedInput)
	TrackOrderCanceled(ctx context.Context, input TrackOrderCanceledInput)

	// TrackEventInBackground calls TrackEvent in a new goroutine which allow to avoid blocking when tracking
	// an event
	// TrackEventInBackground(ctx context.Context, input TrackEventInput)
	// TrackEvent(ctx context.Context, input TrackEventInput) (err error)
	GetAnalyticsData(ctx context.Context, input GetAnalyticsInput) (ret AnalyticsData, err error)
	ScheduleDeletionOfWebsiteData(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error)
	ScheduleDeletionOfOrganizationData(ctx context.Context, db db.Queryer, organizationID guid.GUID) (err error)
	GetEmailsSentCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID, from, to time.Time) (count int64, err error)

	// Jobs
	JobDeleteWebsiteEvents(ctx context.Context, input JobDeleteWebsiteEvents) (err error)
	JobDeleteOrganizationEvents(ctx context.Context, input JobDeleteOrganizationEvents) (err error)
	JobRotateAnonymousIDSalt(ctx context.Context, input JobRotateAnonymousIDSalt) error
}
