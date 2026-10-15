package pgqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"markdown.ninja/stdx/uuid"
)

const (
	MinRetryMax     int32 = 0
	MaxRetryMax     int32 = 100
	DefaultRetryMax int32 = 5

	MinRetryDelay     int32 = 1
	MaxRetryDelay     int32 = 86_400 // 1 day
	DefaultRetryDelay int32 = 5

	DefaultRetryStrategy = RetryStrategyConstant

	MinTimeout     int32 = 1
	MaxTimeout     int32 = 7200
	DefaultTimeout int32 = 60
)

type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

type Queue interface {
	Push(ctx context.Context, q Querier, newJob NewJobInput) error
	PushMany(ctx context.Context, q Querier, newJobs []NewJobInput) error
	Pull(ctx context.Context, numberOfJobs uint64) ([]Job, error)
	DeleteJob(ctx context.Context, jobID uuid.UUID) error
	FailJob(ctx context.Context, jobID uuid.UUID) error
	Clear(ctx context.Context) error
	GetJob(ctx context.Context, jobID uuid.UUID) (Job, error)
	GetFailedJobs(ctx context.Context, limit int64) ([]Job, error)
	Stop()
}

var (
	ErrJobTypeIsNotValid          = errors.New("pgqueue: job type is not valid")
	ErrJobDataIsNotValid          = errors.New("pgqueue: job data is not valid")
	ErrJobRetryMaxIsNotValid      = errors.New("pgqueue: retry_max is not valid")
	ErrJobRetryDelayIsNotValid    = errors.New("pgqueue: retry_delay is not valid")
	ErrJobRetryStrategyIsNotValid = errors.New("pgqueue: retry_strategy is not valid")
	ErrJobTimeoutIsNotValid       = errors.New("pgqueue: timeout is not valid")

	ErrJobSatusIsNotValid = func(status string) error {
		return fmt.Errorf(`Job status "%s" is not valid`, status)
	}
	ErrRetryStrategyIsNotValid = func(status string) error {
		return fmt.Errorf(`Retry strategy "%s" is not valid`, status)
	}
)

type PostgreSQLQueue struct {
	pool   *pgxpool.Pool
	table  string
	logger *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
}

// ensure that PostgreSQLQueue satisfies the Queue interface
var _ Queue = (*PostgreSQLQueue)(nil)

func New(pool *pgxpool.Pool, table string, logger *slog.Logger) *PostgreSQLQueue {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	ctx, cancel := context.WithCancel(context.Background())
	queue := &PostgreSQLQueue{
		pool:   pool,
		table:  table,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	go queue.failTimedOutJobsLoop()

	return queue
}

func (queue *PostgreSQLQueue) CreateTable(ctx context.Context) error {
	tableName := pgx.Identifier{queue.table}.Sanitize()

	tx, err := queue.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("pgqueue: beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id UUID PRIMARY KEY,
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL,
		scheduled_for TIMESTAMPTZ NOT NULL,
		deadline_at TIMESTAMPTZ NOT NULL,
		failed_attempts INTEGER NOT NULL,
		status INTEGER NOT NULL,
		type TEXT NOT NULL,
		data JSONB NOT NULL,
		retry_max INTEGER NOT NULL,
		retry_delay INTEGER NOT NULL,
		retry_strategy INTEGER NOT NULL,
		timeout INTEGER NOT NULL
	)`, tableName))
	if err != nil {
		return fmt.Errorf("pgqueue: creating table: %w", err)
	}

	_, err = tx.Exec(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (scheduled_for) WHERE status = 0`, // JobStatusQueued
		pgx.Identifier{"index_" + queue.table + "_on_scheduled_for_queued"}.Sanitize(), tableName))
	if err != nil {
		return fmt.Errorf("pgqueue: creating scheduled_for_queued index: %w", err)
	}

	_, err = tx.Exec(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (deadline_at) WHERE status = 1`, // JobStatusRunning
		pgx.Identifier{"index_" + queue.table + "_on_deadline_at_running"}.Sanitize(), tableName))
	if err != nil {
		return fmt.Errorf("pgqueue: creating deadline_at_running index: %w", err)
	}

	_, err = tx.Exec(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON %s (created_at DESC) WHERE status = 2`, // JobStatusFailed
		pgx.Identifier{"index_" + queue.table + "_on_created_at_failed"}.Sanitize(), tableName))
	if err != nil {
		return fmt.Errorf("pgqueue: creating created_at_failed index: %w", err)
	}

	return tx.Commit(ctx)
}

func (queue *PostgreSQLQueue) Stop() {
	queue.cancel()
}

func (queue *PostgreSQLQueue) failTimedOutJobsLoop() {
	// we use a random diration for the first iteration to avoid the thundering herd problem if multiple
	// instances restart at the same time
	firstDuration := time.Duration(500+rand.Intn(500)) * time.Millisecond
	timer := time.NewTimer(firstDuration)
	for {
		select {
		case <-queue.ctx.Done():
			return
		case <-timer.C:
			queue.failTimedOutJobs(queue.ctx)
		}
		timer.Reset(time.Second)
	}
}

func (queue *PostgreSQLQueue) failTimedOutJobs(ctx context.Context) {
	now := time.Now().UTC()
	tableName := pgx.Identifier{queue.table}.Sanitize()

	query := fmt.Sprintf(`UPDATE %s
	SET
		status = CASE
			WHEN failed_attempts + 1 >= retry_max THEN $1::integer
			ELSE $2::integer
		END,
		updated_at = $3,
		failed_attempts = failed_attempts + 1,
		scheduled_for = CASE
			WHEN failed_attempts + 1 >= retry_max THEN scheduled_for
			ELSE $3 + (retry_delay * CASE WHEN retry_strategy = $5::integer THEN failed_attempts + 1 ELSE 1 END) * INTERVAL '1 second'
		END
	WHERE status = $4::integer
	  AND deadline_at < $3`, tableName)

	result, err := queue.pool.Exec(ctx, query,
		JobStatusFailed, JobStatusQueued, now, JobStatusRunning, RetryStrategyExponential)
	if err != nil {
		queue.logger.Error("pgqueue: timing out jobs", slog.String("error", err.Error()))
		return
	}

	if n := result.RowsAffected(); n > 0 {
		queue.logger.Info("pgqueue: timed out jobs", slog.Int64("count", n))
	}
}

func validateJob(now time.Time, newJob NewJobInput) (Job, error) {
	scheduledFor := now
	if newJob.ScheduledFor != nil {
		scheduledFor = newJob.ScheduledFor.UTC()
	}

	if newJob.Data == nil {
		return Job{}, ErrJobDataIsNotValid
	}

	jobType := strings.TrimSpace(newJob.Data.JobType())
	if jobType == "" {
		return Job{}, ErrJobTypeIsNotValid
	}

	dataJson, err := json.Marshal(newJob.Data)
	if err != nil {
		return Job{}, fmt.Errorf("pgqueue: marshalling job data: %w", err)
	}

	retryMax := DefaultRetryMax
	if newJob.RetryMax != nil {
		retryMax = *newJob.RetryMax
	}
	if retryMax < MinRetryMax || retryMax > MaxRetryMax {
		return Job{}, ErrJobRetryMaxIsNotValid
	}

	retryDelay := DefaultRetryDelay
	if newJob.RetryDelay != nil {
		retryDelay = *newJob.RetryDelay
	}
	if retryDelay < MinRetryDelay || retryDelay > MaxRetryDelay {
		return Job{}, ErrJobRetryDelayIsNotValid
	}

	retryStrategy := DefaultRetryStrategy
	if newJob.RetryStrategy != DefaultRetryStrategy {
		retryStrategy = newJob.RetryStrategy
	}
	if retryStrategy != RetryStrategyConstant && retryStrategy != RetryStrategyExponential {
		return Job{}, ErrJobRetryStrategyIsNotValid
	}

	jobTimeout := DefaultTimeout
	if newJob.Timeout != nil {
		jobTimeout = *newJob.Timeout
	}
	if jobTimeout < MinTimeout || jobTimeout > MaxTimeout {
		return Job{}, ErrJobTimeoutIsNotValid
	}

	job := Job{
		ID:             uuid.NewV7(),
		CreatedAt:      now,
		UpdatedAt:      now,
		ScheduledFor:   scheduledFor,
		DeadlineAt:     now,
		FailedAttempts: 0,
		Status:         JobStatusQueued,
		Type:           jobType,
		DataJson:       dataJson,
		RetryMax:       retryMax,
		RetryDelay:     retryDelay,
		RetryStrategy:  retryStrategy,
		Timeout:        jobTimeout,
	}
	return job, nil
}
