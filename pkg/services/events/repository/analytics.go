package repository

import (
	"context"
	"fmt"
	"math"
	"time"

	"markdown.ninja/pkg/services/events"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

// if COUNT(DISTINCT) ever becomes a problem, we can speed it up:
// https://stackoverflow.com/questions/11250253/postgresql-countdistinct-very-slow
// https://stackoverflow.com/questions/44962440/postgresql-group-count-distinct-using-fast-way

// TODO: improve: timescale apache edition (available on most cloud/DB providers) does not support the
// time_bucket_gapfill function. See here for more details:
// https://docs.timescale.com/about/latest/timescaledb-editions/
//
// Here is the same query with time_bucket_gapfill
//
// SELECT
//
//		time_bucket_gapfill('1 day', time, start => $2 , finish => $3) AS day,
//		COALESCE(COUNT(*), 0) AS page_views,
//		COALESCE(COUNT(DISTINCT anonymous_id), 0) AS visitors
//	FROM events
//	WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $4
//	GROUP BY day, website_id
//	ORDER BY day;
//
// another way to query is
//
// WITH intervals AS (
//
//	SELECT generate_series(
//		date_trunc('day', $2),
//		date_trunc('day', $3),
//		'1 day'::interval
//	) AS time_start
//
// )
// SELECT
//
//	time_bucket('1 day', COALESCE(e.time, i.time_start)) as day,
//	COALESCE(COUNT(e), 0) AS page_views,
//	COALESCE(COUNT(DISTINCT anonymous_id), 0) AS visitors
//
// FROM
//
//	intervals i
//
// LEFT OUTER JOIN
//
//	(SELECT * FROM events WHERE website_id = $1
//		AND type = $4
//		AND time >= $2
//		AND time <= $3) e ON time_bucket('1 day', e.time) = i.time_start
//
// GROUP BY day, e.website_id
// ORDER BY day;
func (repo *EventsRepository) GetPageViewsAndVisitors(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	from, to time.Time) (ret []events.PageViewsAndVisitors, err error) {
	ret = []events.PageViewsAndVisitors{}

	cacheKey := fmt.Sprintf("PageViewsAndVisitors-%s-%d-%d", websiteID.String(), from.Unix(), to.Unix())
	cacheRes := repo.cache.Get(cacheKey)
	if cacheRes != nil {
		return cacheRes.Value().([]events.PageViewsAndVisitors), nil
	}

	// const query = `
	// WITH intervals AS (
	// 	SELECT generate_series(
	// 		date_trunc('day', $2::timestamp with time zone),
	// 		date_trunc('day', $3::timestamp with time zone),
	// 		'1 day'::interval
	// 	) AS time_start
	// )
	// SELECT
	// 	time_bucket('1 day', COALESCE(events.time, intervals.time_start)) as day,
	// 	COALESCE(COUNT(events), 0) AS page_views,
	// 	COALESCE(COUNT(DISTINCT events.anonymous_id), 0) AS visitors
	// FROM intervals
	// LEFT OUTER JOIN events ON time_bucket('1 day', events.time) = intervals.time_start
	// 	AND events.website_id = $1
	// 	AND events.type = $4
	// GROUP BY day, events.website_id
	// ORDER BY day;
	// 	`

	// 	const query = `
	// WITH days AS (
	// 	SELECT generate_series(
	// 		date_trunc('day', $2::timestamp with time zone),
	// 		date_trunc('day', $3::timestamp with time zone),
	// 		'1 day'::interval
	// 	) AS time_start
	// ),
	// events AS (
	// 	SELECT time, anonymous_id FROM events WHERE events.time >= $2 AND events.time <= $3
	// 		AND website_id = $1 AND type = $4
	// )
	// SELECT time_bucket('1 day', COALESCE(events.time, days.time_start)) as day,
	// 		COALESCE(COUNT(events), 0) AS page_views,
	// 		COALESCE(COUNT(DISTINCT events.anonymous_id), 0) AS visitors
	// 	FROM days
	// 	LEFT OUTER JOIN events ON time_bucket('1 day', events.time) = days.time_start
	// 	GROUP BY day
	// 	ORDER BY day;
	// `

	// performance can be further improved with hyperLogLog: https://docs.timescale.com/api/latest/hyperfunctions/approximate-count-distinct/hyperloglog
	// requires TimescaleDB Toolkit
	const query = `
WITH time_range AS (
	SELECT generate_series(
		date_trunc('day', $3::timestamp with time zone),
		date_trunc('day', $4::timestamp with time zone),
		'1 day'::interval
	) AS time_start
),
events AS (
	SELECT time_bucket('1 day', time) AS event_day,
		COALESCE(COUNT(*), 0) AS page_views,
		anonymous_id AS visitors
	FROM events
	WHERE time >= $3 AND time <= $4
		AND type = $1 AND website_id = $2
	GROUP BY event_day, anonymous_id
)
SELECT COALESCE(events.event_day, time_range.time_start) as day,
	COALESCE(SUM(events.page_views), 0) AS page_views,
	COALESCE(COUNT(events.visitors), 0) AS visitors
FROM time_range
LEFT OUTER JOIN events ON events.event_day = time_range.time_start
GROUP BY day
ORDER BY day
`

	err = db.Select(ctx, &ret, query, events.EventTypePageView, websiteID, from, to)
	if err != nil {
		err = fmt.Errorf("events.GetPageViewsAndVisitors: %w", err)
		return
	}

	repo.cache.Set(cacheKey, ret, 2*time.Minute)

	return
}

// if limit < 1 then no limit
func (repo *EventsRepository) GetTopPages(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	from, to time.Time, limit int64) (ret []events.Counter, err error) {
	ret = make([]events.Counter, 0, max(limit, 10))
	if limit < 1 {
		limit = math.MaxInt64
	}

	cacheKey := fmt.Sprintf("TopPages-%s-%d-%d-%d", websiteID.String(), from.Unix(), to.Unix(), limit)
	cacheRes := repo.cache.Get(cacheKey)
	if cacheRes != nil {
		return cacheRes.Value().([]events.Counter), nil
	}
	// const query = `SELECT (data->>'page')::TEXT AS label, COUNT(DISTINCT anonymous_id) AS count
	// 	FROM events
	// 	WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $5
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	// const query = `
	// 	SELECT label, COUNT(anonymous_id) FROM (
	// 		SELECT (data->>'page')::TEXT AS label, anonymous_id
	// 			FROM events
	// 			WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
	// 			GROUP BY label, anonymous_id
	// 	) AS subquery
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	const query = `
	SELECT label, COUNT(anonymous_id) FROM (
		SELECT path AS label, anonymous_id
			FROM events
			WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
			GROUP BY label, anonymous_id
	) AS subquery
	GROUP BY label
	ORDER BY count DESC
	LIMIT $4
`

	err = db.Select(ctx, &ret, query, websiteID, from, to, limit, events.EventTypePageView)
	if err != nil {
		err = fmt.Errorf("events.GetTopPages: %w", err)
		return
	}

	repo.cache.Set(cacheKey, ret, 2*time.Minute)

	return
}

// if limit < 1 then no limit
func (repo *EventsRepository) GetTopCountries(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	from, to time.Time, limit int64) (ret []events.Counter, err error) {
	ret = make([]events.Counter, 0, max(limit, 10))
	if limit < 1 {
		limit = math.MaxInt64
	}

	cacheKey := fmt.Sprintf("TopCountries-%s-%d-%d-%d", websiteID.String(), from.Unix(), to.Unix(), limit)
	cacheRes := repo.cache.Get(cacheKey)
	if cacheRes != nil {
		return cacheRes.Value().([]events.Counter), nil
	}

	// const query = `SELECT (data->>'country_code')::TEXT AS label, COUNT(DISTINCT anonymous_id) AS count
	// 	FROM events
	// 	WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $5
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	// const query = `
	// 	SELECT label, COUNT(anonymous_id) FROM (
	// 		SELECT (data->>'country_code')::TEXT AS label, anonymous_id
	// 			FROM events
	// 			WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
	// 			GROUP BY label, anonymous_id
	// 	) AS subquery
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	const query = `
		SELECT label, COUNT(anonymous_id) FROM (
			SELECT country AS label, anonymous_id
				FROM events
				WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
				GROUP BY label, anonymous_id
		) AS subquery
		GROUP BY label
		ORDER BY count DESC
		LIMIT $4
	`

	err = db.Select(ctx, &ret, query, websiteID, from, to, limit, events.EventTypePageView)
	if err != nil {
		err = fmt.Errorf("events.GetTopCountries: %w", err)
		return
	}

	repo.cache.Set(cacheKey, ret, 2*time.Minute)

	return
}

// if limit < 1 then no limit
func (repo *EventsRepository) GetTopReferrers(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	from, to time.Time, limit int64) (ret []events.Counter, err error) {
	ret = make([]events.Counter, 0, max(limit, 10))
	if limit < 1 {
		limit = math.MaxInt64
	}

	cacheKey := fmt.Sprintf("TopReferrers-%s-%d-%d-%d", websiteID.String(), from.Unix(), to.Unix(), limit)
	cacheRes := repo.cache.Get(cacheKey)
	if cacheRes != nil {
		return cacheRes.Value().([]events.Counter), nil
	}

	// const query = `SELECT (data->>'referrer')::TEXT AS label, COUNT(DISTINCT anonymous_id) AS count
	// 	FROM events
	// 	WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $5
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	// const query = `
	// 	SELECT label, COUNT(anonymous_id) FROM (
	// 		SELECT (data->>'referrer')::TEXT AS label, anonymous_id
	// 			FROM events
	// 			WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
	// 			GROUP BY label, anonymous_id
	// 	) AS subquery
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	const query = `
		SELECT label, COUNT(anonymous_id) FROM (
			SELECT referrer AS label, anonymous_id
				FROM events
				WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
				GROUP BY label, anonymous_id
		) AS subquery
		GROUP BY label
		ORDER BY count DESC
		LIMIT $4
	`

	err = db.Select(ctx, &ret, query, websiteID, from, to, limit, events.EventTypePageView)
	if err != nil {
		err = fmt.Errorf("events.GetTopReferrers: %w", err)
		return
	}

	repo.cache.Set(cacheKey, ret, 2*time.Minute)

	return
}

// if limit < 1 then no limit
func (repo *EventsRepository) GetTopBrowsers(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	from, to time.Time, limit int64) (ret []events.CounterBrowser, err error) {
	ret = make([]events.CounterBrowser, 0, max(limit, 10))
	if limit < 1 {
		limit = math.MaxInt64
	}

	cacheKey := fmt.Sprintf("TopBrowsers-%s-%d-%d-%d", websiteID.String(), from.Unix(), to.Unix(), limit)
	cacheRes := repo.cache.Get(cacheKey)
	if cacheRes != nil {
		return cacheRes.Value().([]events.CounterBrowser), nil
	}

	// const query = `SELECT (data->>'browser')::TEXT AS label, COUNT(DISTINCT anonymous_id) AS count
	// 	FROM events
	// 	WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $5
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `

	// const query = `
	// 	SELECT label, COUNT(anonymous_id) FROM (
	// 		SELECT (data->>'browser')::TEXT AS label, anonymous_id
	// 			FROM events
	// 			WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
	// 			GROUP BY label, anonymous_id
	// 	) AS subquery
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	const query = `
	SELECT label, COUNT(anonymous_id) FROM (
		SELECT browser AS label, anonymous_id
			FROM events
			WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
			GROUP BY label, anonymous_id
	) AS subquery
	GROUP BY label
	ORDER BY count DESC
	LIMIT $4
`

	err = db.Select(ctx, &ret, query, websiteID, from, to, limit, events.EventTypePageView)
	if err != nil {
		err = fmt.Errorf("events.GetTopBrowsers: %w", err)
		return
	}

	repo.cache.Set(cacheKey, ret, 2*time.Minute)

	return
}

// if limit < 1 then no limit
func (repo *EventsRepository) GetTopOses(ctx context.Context, db db.Queryer, websiteID guid.GUID,
	from, to time.Time, limit int64) (ret []events.CounterOperatingSystem, err error) {
	ret = make([]events.CounterOperatingSystem, 0, max(limit, 10))
	if limit < 1 {
		limit = math.MaxInt64
	}

	cacheKey := fmt.Sprintf("TopOses-%s-%d-%d-%d", websiteID.String(), from.Unix(), to.Unix(), limit)
	cacheRes := repo.cache.Get(cacheKey)
	if cacheRes != nil {
		return cacheRes.Value().([]events.CounterOperatingSystem), nil
	}

	// const query = `SELECT (data->>'os')::TEXT AS label, COUNT(DISTINCT anonymous_id) AS count
	// 	FROM events
	// 	WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $5
	// 	GROUP BY label
	// 	ORDER BY count DESC
	// 	LIMIT $4
	// `
	const query = `
		SELECT label, COUNT(anonymous_id) FROM (
			SELECT operating_system AS label, anonymous_id
				FROM events
				WHERE time >= $2 AND time <= $3 AND website_id = $1 AND type = $5
				GROUP BY label, anonymous_id
		) AS subquery
		GROUP BY label
		ORDER BY count DESC
		LIMIT $4
	`

	err = db.Select(ctx, &ret, query, websiteID, from, to, limit, events.EventTypePageView)
	if err != nil {
		err = fmt.Errorf("events.GetTopOses: %w", err)
		return
	}

	repo.cache.Set(cacheKey, ret, 2*time.Minute)

	return
}

func (repo *EventsRepository) GetNewSubscribersCount(ctx context.Context, db db.Queryer, websiteID guid.GUID, from, to time.Time) (newSubscribersCount int64, err error) {
	cacheKey := fmt.Sprintf("NewSubscribersCount-%s-%d-%d", websiteID.String(), from.Unix(), to.Unix())
	if cacheRes := repo.cache.Get(cacheKey); cacheRes != nil {
		return cacheRes.Value().(int64), nil
	}

	const query = `SELECT COUNT(*)
		FROM events
		WHERE website_id = $1 AND time >= $2 AND time <= $3 AND type = $4`

	err = db.Get(ctx, &newSubscribersCount, query, websiteID, from, to, events.EventTypeSubscribedToNewsletter)
	if err != nil {
		err = fmt.Errorf("events.GetNewSubscribersCount: %w", err)
		return
	}

	repo.cache.Set(cacheKey, newSubscribersCount, 2*time.Minute)

	return
}
