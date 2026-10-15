package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"log/slog"

	"markdown.ninja/stdx/db"
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

const (
	MaxPullBatchSize          = 1000
	POSTGRES_MAX_QUERY_PARAMS = 65_535 - 1
	jobNumberOfColumns        = 12
)

// ensure that PostgreSQLQueue satisfies the Queue interface
var _ queue.Queue = (*PostgreSQLQueue)(nil)

func buildQuery(initialQuery string, argsPerRecord int, arguments []any) (query string, err error) {
	argsLen := len(arguments)

	if argsLen%argsPerRecord != 0 {
		return "", errors.New("BuildQuery: len(arguments) %% argsPerRecord != 0")
	}

	queryBuilder := strings.Builder{}
	queryBuilder.Grow(len(arguments)*5 + 2)

	queryBuilder.WriteString(initialQuery)
	if !strings.HasSuffix(initialQuery, " ") {
		queryBuilder.WriteRune(' ')
	}
	queryBuilder.WriteRune('(')

	for i := 1; i <= argsLen; i += 1 {
		queryBuilder.WriteString(fmt.Sprintf("$%d", i))

		if i%argsPerRecord == 0 {
			if i == argsLen {
				queryBuilder.WriteRune(')')
			} else {
				queryBuilder.WriteString("),(")

			}
		} else {
			queryBuilder.WriteString(",")
		}
	}

	return queryBuilder.String(), nil
}

type PostgreSQLQueue struct {
	db     db.DB
	logger *slog.Logger
}

func NewPostgreSQLQueue(ctx context.Context, db db.DB, logger *slog.Logger) *PostgreSQLQueue {
	queue := &PostgreSQLQueue{
		db:     db,
		logger: logger,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				queue.failTimedOutJobs(ctx)
			}
		}
	}()

	return queue
}

func (pgqueue *PostgreSQLQueue) Push(ctx context.Context, tx db.Queryer, newJob queue.NewJobInput) (err error) {
	var db db.Queryer
	now := time.Now().UTC()

	db = pgqueue.db
	if tx != nil {
		db = tx
	}

	job, err := pgqueue.validateJob(now, newJob)
	if err != nil {
		return
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

func (pgqueue *PostgreSQLQueue) PushMany(ctx context.Context, tx db.Tx, newJobs []queue.NewJobInput) error {
	// Postgres only accepts queries with a limited number of parameters. Thus, because we may have a huge
	// number of items to insert (20k+) where each have many columns, we need to chunk
	// the batch inserts by POSTGRES_MAX_QUERY_PARAMS / number of columns.
	//
	// But for now we use smaller batch size to reduce the amount of used memory.
	// TODO: use UNNEST
	BATCH_SIZE := POSTGRES_MAX_QUERY_PARAMS / jobNumberOfColumns

	now := time.Now().UTC()
	var err error

	// we commit / rollback the transaction only if it was started by "us"
	commitTransaction := false
	if tx == nil {
		commitTransaction = true
		tx, err = pgqueue.db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("queue: Starting DB transaction: %w", err)
		}
		defer tx.Rollback()
	}

	// batch insert up to the limit
	for jobsChunk := range slices.Chunk(newJobs, BATCH_SIZE) {
		query := `INSERT INTO queue
				(id, created_at, updated_at, scheduled_for, failed_attempts, status, type, data, retry_max, retry_delay, retry_strategy, timeout)
				VALUES`
		valuesToInsert := make([]any, 0, len(jobsChunk)*jobNumberOfColumns)
		for _, newJobInput := range jobsChunk {
			job, err := pgqueue.validateJob(now, newJobInput)
			if err != nil {
				return err
			}
			valuesToInsert = append(valuesToInsert, job.ID, job.CreatedAt, job.UpdatedAt, job.ScheduledFor, job.FailedAttempts,
				job.Status, job.Type, job.RawData, job.RetryMax, job.RetryDelay, job.RetryStrategy, job.Timeout)
		}

		query, err = buildQuery(query, jobNumberOfColumns, valuesToInsert)
		if err != nil {
			return fmt.Errorf("queue: building PushMany PostgreSQL query: %w", err)
		}

		_, err = tx.Exec(ctx, query, valuesToInsert...)
		if err != nil {
			return fmt.Errorf("queue: inserting jobs: %w", err)
		}
	}

	if commitTransaction {
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("queue: Comitting DB transaction: %w", err)
		}
	}

	return nil
}

func (pgqueue *PostgreSQLQueue) validateJob(now time.Time, newJob queue.NewJobInput) (job queue.Job, err error) {
	scheduledFor := now
	if newJob.ScheduledFor != nil {
		scheduledFor = newJob.ScheduledFor.UTC()
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
		err = fmt.Errorf("queue.postgresql: marshalling job data to JSON: %w", err)
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

	// UUIDv7 are used to avoid index fragmentation and increase insert performance
	// see https://www.cybertec-postgresql.com/en/unexpected-downsides-of-uuid-keys-in-postgresql
	// https://news.ycombinator.com/item?id=36429986
	// note that for some distributed databases this may have a performance impact as it will produce
	// hot partitions but it can be solved with hashing
	job = queue.Job{
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
	return job, nil
}

// pull fetches at most `number_of_jobs` from the queue.
func (pgqueue *PostgreSQLQueue) Pull(ctx context.Context, numberOfJobs uint64) (ret []queue.Job, err error) {
	if numberOfJobs > MaxPullBatchSize {
		err = fmt.Errorf("queue.postgresql: you can't pull more than %d jobs", MaxPullBatchSize)
		return
	}
	ret = make([]queue.Job, 0, numberOfJobs)

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

	// if we don't find jobs, we continue to make requests, eitheir until at least on job is found
	// or until approximately 1 second has elapsed
	// this technique is also known as long polling
	for i := 0; i < 10; i += 1 {
		// make sure that the context has not be canceled
		select {
		case <-ctx.Done():
			return ret, nil
		case <-time.After(100 * time.Millisecond):
		}

		err := pgqueue.db.Select(ctx, &ret, query, queue.JobStatusRunning, now, queue.JobStatusQueued, now, int64(numberOfJobs))
		if err != nil {
			return ret, err
		}

		if len(ret) != 0 {
			return ret, nil
		}
	}

	// finally, we didn't find any job to return within 1 sec, so we return an empty response
	return ret, nil
}

func (pgqueue *PostgreSQLQueue) DeleteJob(ctx context.Context, jobID uuid.UUID) error {
	query := "DELETE FROM queue WHERE id = $1"

	_, err := pgqueue.db.Exec(ctx, query, jobID)
	return err
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
	return err
}

func (pgqueue *PostgreSQLQueue) Clear(ctx context.Context) error {
	query := "DELETE FROM queue"

	_, err := pgqueue.db.Exec(ctx, query)
	return err
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

	// return
}

func (pgqueue *PostgreSQLQueue) GetFailedJobs(ctx context.Context, limit int64) (jobs []queue.Job, err error) {
	jobs = make([]queue.Job, 0)
	query := `SELECT * FROM queue WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2`

	err = pgqueue.db.Select(ctx, &jobs, query, queue.JobStatusFailed, limit)
	return jobs, err
}

func (pgqueue *PostgreSQLQueue) GetJob(ctx context.Context, jobID uuid.UUID) (job queue.Job, err error) {
	query := "SELECT * FROM queue WHERE id = $1"
	err = pgqueue.db.Get(ctx, &job, query, jobID)
	return job, err
}
