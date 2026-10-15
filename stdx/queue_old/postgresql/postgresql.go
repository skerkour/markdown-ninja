package postgresql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"log/slog"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/uuid"
)

var (
	ErrJobTypeIsNotValid          = errors.New("queue.postgresql: job type is not valid")
	ErrJobDataIsNotValid          = errors.New("queue.postgresql: job data is not valid")
	ErrJobRetryMaxIsNotValid      = errors.New("queue.postgresql: retry_max is not valid")
	ErrJobRetryDelayIsNotValid    = errors.New("queue.postgresql: retry_delay is not valid")
	ErrJobRetryStrategyIsNotValid = errors.New("queue.postgresql: retry_strategy is not valid")
	ErrJobTimeoutIsNotValid       = errors.New("queue.postgresql: timeout is not valid")
)

type PostgreSQLQueue struct {
	db           db.DB
	shuttingDown atomic.Bool
	logger       *slog.Logger
}

func NewPostgreSQLQueue(db db.DB, logger *slog.Logger) *PostgreSQLQueue {
	var shuttingDown atomic.Bool
	shuttingDown.Store(false)

	queue := PostgreSQLQueue{
		db:           db,
		shuttingDown: shuttingDown,
		logger:       logger,
	}

	// TODO: improve?
	ctx := context.Background()

	go func() {
		for {
			if shuttingDown.Load() {
				break
			}
			queue.failTimedOutJobs(ctx)
			time.Sleep(time.Second)
		}
	}()

	return &queue
}

func (pgqueue *PostgreSQLQueue) Push(ctx context.Context, tx db.Queryer, newJob queue.NewJobInput) (err error) {
	now := time.Now().UTC()
	var db db.Queryer

	db = pgqueue.db
	if tx != nil {
		db = tx
	}

	scheduledFor := now
	if newJob.ScheduledFor != nil {
		scheduledFor = (*newJob.ScheduledFor).UTC()
	}

	jobType := strings.TrimSpace(newJob.Data.JobType())
	if jobType == "" {
		err = ErrJobTypeIsNotValid
		return
	}

	if newJob.Data == nil {
		err = ErrJobDataIsNotValid
		return
	}

	rawData, err := json.Marshal(newJob.Data)
	if err != nil {
		err = fmt.Errorf("queue.postgresql: marshalling job data to JSON")
		return
	}

	retryMax := queue.DefaultRetryMax
	if newJob.RetryMax != nil {
		retryMax = *newJob.RetryMax
	}
	if retryMax < queue.MinRetryMax || retryMax > queue.MaxRetryMax {
		err = ErrJobRetryMaxIsNotValid
		return
	}

	retryDelay := queue.DefaultRetryDelay
	if newJob.RetryDelay != nil {
		retryDelay = *newJob.RetryDelay
	}
	if retryDelay < queue.MinRetryDelay || retryDelay > queue.MaxRetryDelay {
		err = ErrJobRetryDelayIsNotValid
		return
	}

	retryStrategy := queue.DefaultRetryStrategy
	if newJob.RetryStrategy != queue.DefaultRetryStrategy {
		retryStrategy = newJob.RetryStrategy
	}
	if retryStrategy != queue.RetryStrategyConstant && retryStrategy != queue.RetryStrategyExponential {
		err = ErrJobRetryStrategyIsNotValid
		return
	}

	jobTimeout := queue.DefaultTimeout
	if newJob.Timeout != nil {
		jobTimeout = *newJob.Timeout
	}
	if jobTimeout < queue.MinTimeout || jobTimeout > queue.MaxTimeout {
		err = ErrJobTimeoutIsNotValid
		return
	}

	// we use time-based GUIDs to avoid index fragmentation and increase insert performance
	// see https://www.cybertec-postgresql.com/en/unexpected-downsides-of-uuid-keys-in-postgresql
	// https://news.ycombinator.com/item?id=36429986
	job := queue.Job{
		ID:             uuid.NewV7(),
		CreatedAt:      now,
		UpdatedAt:      now,
		ScheduledFor:   scheduledFor,
		FailedAttempts: 0,
		Status:         queue.JobStatusQueued,
		Type:           jobType,
		RawData:        rawData,
		RetryMax:       retryMax,
		RetryDelay:     retryDelay,
		RetryStrategy:  retryStrategy,
		Timeout:        jobTimeout,
	}
	query := `INSERT INTO queue
		(id, created_at, updated_at, scheduled_for, failed_attempts, status, type, data, retry_max, retry_delay, retry_strategy, timeout)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err = db.Exec(ctx, query, job.ID, job.CreatedAt, job.UpdatedAt, job.ScheduledFor, job.FailedAttempts,
		job.Status, job.Type, job.RawData, job.RetryMax, job.RetryDelay, job.RetryStrategy, job.Timeout)
	if err != nil {
		return
	}

	return
}

// pull fetches at most `number_of_jobs` from the queue.
func (pgqueue *PostgreSQLQueue) Pull(ctx context.Context, numberOfJobs int64) ([]queue.Job, error) {
	if numberOfJobs > 200 || numberOfJobs < 0 {
		numberOfJobs = 200
	}

	now := time.Now().UTC()
	query := `UPDATE queue
	SET status = $1, updated_at = $2
	WHERE id IN (
		SELECT id
		FROM queue
		WHERE status = $3 AND scheduled_for <= $4 AND failed_attempts <= queue.retry_max
		ORDER BY scheduled_for
		FOR UPDATE SKIP LOCKED
		LIMIT $5
	)
	RETURNING *`
	ret := []queue.Job{}

	err := pgqueue.db.Select(ctx, &ret, query, queue.JobStatusRunning, now, queue.JobStatusQueued, now, numberOfJobs)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func (pgqueue *PostgreSQLQueue) DeleteJob(ctx context.Context, jobID guid.GUID) error {
	query := "DELETE FROM queue WHERE id = $1"

	_, err := pgqueue.db.Exec(ctx, query, jobID)
	if err != nil {
		return err
	}
	return nil
}

func (pgqueue *PostgreSQLQueue) FailJob(ctx context.Context, job queue.Job) error {
	query := `UPDATE queue
	SET status = $1, updated_at = $2, scheduled_for = $3, failed_attempts = $4
	WHERE id = $5`

	now := time.Now().UTC()
	status := queue.JobStatusQueued
	failedAttempt := job.FailedAttempts + 1

	if failedAttempt >= job.RetryMax {
		status = queue.JobStatusFailed
	}

	var factor int64 = 1
	if job.RetryStrategy == queue.RetryStrategyExponential {
		factor = failedAttempt
	}
	scheduledFor := now.Add(time.Second * time.Duration(job.RetryDelay) * time.Duration(factor))

	_, err := pgqueue.db.Exec(ctx, query, status, now, scheduledFor, failedAttempt, job.ID)
	if err != nil {
		return err
	}
	return nil
}

func (pgqueue *PostgreSQLQueue) Clear(ctx context.Context) error {
	query := "DELETE FROM queue"

	_, err := pgqueue.db.Exec(ctx, query)
	if err != nil {
		return err
	}
	return nil
}

// TODO
func (pgqueue *PostgreSQLQueue) failTimedOutJobs(ctx context.Context) {
	// query := `UPDATE queue
	// SET status = $1, updated_at = $2
	// WHERE id IN (
	// 	SELECT id
	// 	FROM queue
	// 	WHERE status = $3 AND scheduled_for <= $4 AND failed_attempts < queue.retry_max
	// 	ORDER BY scheduled_for
	// 	FOR UPDATE SKIP LOCKED
	// 	LIMIT $5
	// )
	// RETURNING *`

	// now := time.Now().UTC()

	// 	_, err := queue.db.Exec(ctx, query, queue.JobStatusQueued, now,
	// 		queue.JobStatusRunning, now, scheduledFor, failedAttempt, job.ID)

	// 		if err != nil {
	// 	queue.logger.Error("queue.failTimedOutJobs: updating timedout jobs", slogx.Err(err))
	// 	return
	// }

	// query := `UPDATE queue
	// SET status = $1, updated_at = $2, scheduled_for = $3, failed_attempts = $4
	// WHERE id = $5`

	// now := time.Now().UTC()
	// failedAttempt := job.FailedAttempts + 1
	// var factor int64 = 1
	// if job.RetryStrategy == queue.RetryStrategyExponential {
	// 	factor = failedAttempt
	// }
	// scheduledFor := now.Add(time.Second * time.Duration(job.RetryDelay) * time.Duration(factor))

	// _, err = queue.db.Exec(ctx, query, queue.JobStatusFailed, now, scheduledFor, failedAttempt, job.ID)
	// if err != nil {
	// 	return err
	// }

	return
}

func (pgqueue *PostgreSQLQueue) Stop(ctx context.Context) {
	pgqueue.shuttingDown.Store(true)
}
