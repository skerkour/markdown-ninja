package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"markdown.ninja/pkg/dbx"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/iterx"
)

// https://www.timescale.com/blog/boosting-postgres-insert-performance/
func (repo *EventsRepository) SaveEvents(ctx context.Context, db db.Queryer, eventsInput []events.Event) error {
	const query = `INSERT INTO events
		(time, type, data, website_id, anonymous_id, order_id, newsletter_id,
			path, country, browser, operating_system, referrer)
		SELECT * FROM UNNEST($1::TIMESTAMP WITH TIME ZONE[], $2::BIGINT[], $3::JSONB[], $4::UUID[],
			$5::UUID[], $6::UUID[], $7::UUID[], $8::TEXT[], $9::TEXT[], $10::INT[],
			$11::INT[], $12::TEXT[]
		)`
	var err error

	times := slices.AppendSeq(make([]time.Time, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) time.Time {
		return event.Time
	}))
	types := slices.AppendSeq(make([]events.EventType, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) events.EventType {
		return event.Type
	}))
	data := make([][]byte, 0, len(eventsInput))
	for _, event := range eventsInput {
		eventDataJson, err := json.Marshal(event.Data)
		if err != nil {
			return fmt.Errorf("events.SaveEvents: error serializing event data to JSON: %w", err)
		}
		data = append(data, eventDataJson)
	}
	websiteIds := slices.AppendSeq(make([]guid.GUID, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) guid.GUID {
		return event.WebsiteID
	}))
	anonymousIDs := slices.AppendSeq(make([]dbx.Nullable[guid.GUID], 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) dbx.Nullable[guid.GUID] {
		return dbx.NewNullable(event.AnonymousID)
	}))
	orderIDs := slices.AppendSeq(make([]dbx.Nullable[guid.GUID], 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) dbx.Nullable[guid.GUID] {
		return dbx.NewNullable(event.OrderID)
	}))
	newsletterIDs := slices.AppendSeq(make([]dbx.Nullable[guid.GUID], 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) dbx.Nullable[guid.GUID] {
		return dbx.NewNullable(event.NewsletterID)
	}))
	paths := slices.AppendSeq(make([]*string, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) *string {
		return event.Path
	}))
	countries := slices.AppendSeq(make([]*string, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) *string {
		return event.Country
	}))
	browsers := slices.AppendSeq(make([]*events.Browser, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) *events.Browser {
		return event.Browser
	}))
	operatingSystems := slices.AppendSeq(make([]*events.OperatingSystem, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) *events.OperatingSystem {
		return event.OperatingSystem
	}))
	referrers := slices.AppendSeq(make([]*string, 0, len(eventsInput)), iterx.Map(slices.Values(eventsInput), func(event events.Event) *string {
		return event.Referrer
	}))

	_, err = db.Exec(ctx, query, times, types, data, websiteIds, anonymousIDs, orderIDs, newsletterIDs,
		paths, countries, browsers, operatingSystems, referrers)
	if err != nil {
		return fmt.Errorf("events.SaveEvent: error inserting events: %w", err)
	}

	return nil

	// args := make([]any, 0, len(eventsInput)*events.EventDatabaseColumns)
	// for _, event := range eventsInput {
	// 	dataJson, err := json.Marshal(event.Data)
	// 	if err != nil {
	// 		return fmt.Errorf("events.SaveEvents: error serializing event data to JSON: %w", err)

	// 	}
	// 	args = append(args, event.Time, event.Type, dataJson, event.WebsiteID,
	// 		event.AnonymousID, event.ContactID, event.OrderID, event.NewsletterID,
	// 		event.Path, event.Country, event.Browser, event.OperatingSystem, event.Referrer,
	// 	)
	// }

	// query, err = dbx.BuildQuery(query, events.EventDatabaseColumns, args)
	// if err != nil {
	// 	return fmt.Errorf("events.SaveEvents: error building PostgreSQL query: %w", err)
	// }

	// const query = `INSERT INTO events
	// 			(time, type, data, website_id, anonymous_id, contact_id, order_id, newsletter_id)
	// 		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	// dataJson, err := json.Marshal(event.Data)
	// if err != nil {
	// 	err = fmt.Errorf("events.SaveEvent: serializing event data to JSON: %w", err)
	// 	return
	// }

	// _, err = db.Exec(ctx, query, event.Time, event.Type, dataJson, event.WebsiteID,
	// 	event.AnonymousID, event.ContactID, event.OrderID, event.NewsletterID)
	// if err != nil {
	// 	err = fmt.Errorf("events.SaveEvent: inserting event: %w", err)
	// 	return
	// }

}

func (repo *EventsRepository) DeleteWebsiteEvents(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error) {
	const query = "DELETE FROM events WHERE website_id = $1"

	_, err = db.Exec(ctx, query, websiteID)
	if err != nil {
		err = fmt.Errorf("events.DeleteWebsiteEvent: %w", err)
		return
	}

	return
}

func (repo *EventsRepository) DeleteOrganizationEvents(ctx context.Context, db db.Queryer, organizationID guid.GUID) (err error) {
	const query = "DELETE FROM events WHERE website_id = ANY(SELECT id FROM websites WHERE organization_id = $1)"

	_, err = db.Exec(ctx, query, organizationID)
	if err != nil {
		err = fmt.Errorf("events.DeleteWebsiteEvent: %w", err)
		return
	}

	return
}

func (repo *EventsRepository) GetEventsTypeCountForOrganization(ctx context.Context, db db.Queryer, eventsType events.EventType, organizationID guid.GUID, from, to time.Time) (count int64, err error) {
	const query = `SELECT COUNT(*) FROM events
		WHERE time >= $1 AND time <= $2
			AND website_id = ANY(SELECT id FROM websites WHERE organization_id = $3)
			AND type = $4
		`

	err = db.Get(ctx, &count, query, from, to, organizationID, eventsType)
	if err != nil {
		err = fmt.Errorf("events.GetEmailsSentEventsCountForOrganization: %w", err)
		return
	}

	return
}
