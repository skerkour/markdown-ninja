package pgqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"markdown.ninja/stdx/uuid"
)

func (queue *PostgreSQLQueue) Push(ctx context.Context, querier Querier, newJob NewJobInput) (err error) {
	now := time.Now().UTC()

	job, err := validateJob(now, newJob)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`INSERT INTO %s
		(id, created_at, updated_at, scheduled_for, deadline_at, failed_attempts, status, type, data, retry_max, retry_delay, retry_strategy, timeout)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`, pgx.Identifier{queue.table}.Sanitize())

	_, err = querier.Exec(ctx, query,
		job.ID, job.CreatedAt, job.UpdatedAt, job.ScheduledFor, job.DeadlineAt, job.FailedAttempts,
		job.Status, job.Type, job.DataJson, job.RetryMax, job.RetryDelay,
		job.RetryStrategy, job.Timeout)
	if err != nil {
		return fmt.Errorf("pgqueue: inserting job: %w", err)
	}

	return nil
}

func (queue *PostgreSQLQueue) PushMany(ctx context.Context, querier Querier, newJobs []NewJobInput) error {
	if len(newJobs) == 0 {
		return nil
	}

	now := time.Now().UTC()

	rows := make([][]any, len(newJobs))
	for i, newJob := range newJobs {
		job, err := validateJob(now, newJob)
		if err != nil {
			return fmt.Errorf("pgqueue: validating job %d: %w", i, err)
		}
		rows[i] = []any{
			job.ID, job.CreatedAt, job.UpdatedAt, job.ScheduledFor, job.DeadlineAt, job.FailedAttempts, job.Status,
			job.Type, job.DataJson, job.RetryMax, job.RetryDelay, job.RetryStrategy, job.Timeout,
		}
	}

	_, err := querier.CopyFrom(ctx,
		pgx.Identifier{queue.table},
		[]string{
			"id", "created_at", "updated_at", "scheduled_for", "deadline_at", "failed_attempts", "status",
			"type", "data", "retry_max", "retry_delay", "retry_strategy", "timeout",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("pgqueue: CopyFrom inserting jobs: %w", err)
	}

	return nil
}

func (queue *PostgreSQLQueue) Pull(ctx context.Context, numberOfJobs uint64) ([]Job, error) {
	now := time.Now().UTC()
	tableName := pgx.Identifier{queue.table}.Sanitize()
	query := fmt.Sprintf(`UPDATE %s
	SET status = $1, updated_at = $2, deadline_at = $2 + (timeout * INTERVAL '1 second')
	WHERE id IN (
		SELECT id
		FROM %s
		WHERE status = $3 AND scheduled_for <= $4 AND failed_attempts <= %s.retry_max
		ORDER BY scheduled_for, id
		FOR UPDATE SKIP LOCKED
		LIMIT $5
	)
	RETURNING *`, tableName, tableName, tableName)

	// If we don't find jobs, we continue to make requests, either until at least one job is found
	// or until approximately 1 second has elapsed.
	// This technique is known as long polling.
	pollingTimer := time.NewTimer(0) // fires immediately for first iteration
	for i := 0; i < 10; i++ {
		select {
		case <-ctx.Done():
			return nil, nil
		case <-pollingTimer.C:
		}

		rows, err := queue.pool.Query(ctx, query,
			JobStatusRunning, now, JobStatusQueued, now, int64(numberOfJobs))
		if err != nil {
			return nil, fmt.Errorf("pgqueue: pulling jobs: %w", err)
		}

		jobs, err := pgx.CollectRows(rows, pgx.RowToStructByName[Job])
		if err != nil {
			return nil, fmt.Errorf("pgqueue: scanning pulled jobs: %w", err)
		}

		if len(jobs) > 0 {
			return jobs, nil
		}
		pollingTimer.Reset(100 * time.Millisecond)
	}

	return nil, nil
}

func (queue *PostgreSQLQueue) DeleteJob(ctx context.Context, jobID uuid.UUID) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", pgx.Identifier{queue.table}.Sanitize())

	_, err := queue.pool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("pgqueue: deleting job: %w", err)
	}
	return nil
}

func (queue *PostgreSQLQueue) FailJob(ctx context.Context, jobID uuid.UUID) error {
	now := time.Now().UTC()

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
			ELSE $3 + (retry_delay * CASE WHEN retry_strategy = $4::integer THEN failed_attempts + 1 ELSE 1 END) * INTERVAL '1 second'
		END
	WHERE id = $5`, pgx.Identifier{queue.table}.Sanitize())

	_, err := queue.pool.Exec(ctx, query, JobStatusFailed, JobStatusQueued, now, RetryStrategyExponential, jobID)
	if err != nil {
		return fmt.Errorf("pgqueue: failing job: %w", err)
	}
	return nil
}

func (queue *PostgreSQLQueue) Clear(ctx context.Context) error {
	query := fmt.Sprintf("TRUNCATE %s", pgx.Identifier{queue.table}.Sanitize())

	_, err := queue.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("pgqueue: clearing queue: %w", err)
	}
	return nil
}

func (queue *PostgreSQLQueue) GetJob(ctx context.Context, jobID uuid.UUID) (Job, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1", pgx.Identifier{queue.table}.Sanitize())

	rows, err := queue.pool.Query(ctx, query, jobID)
	if err != nil {
		return Job{}, fmt.Errorf("pgqueue: getting job: %w", err)
	}

	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Job])
}

func (queue *PostgreSQLQueue) GetFailedJobs(ctx context.Context, limit int64) ([]Job, error) {
	query := fmt.Sprintf(`SELECT * FROM %s WHERE status = $1 ORDER BY created_at DESC LIMIT $2`, pgx.Identifier{queue.table}.Sanitize())

	rows, err := queue.pool.Query(ctx, query, JobStatusFailed, limit)
	if err != nil {
		return nil, fmt.Errorf("pgqueue: querying failed jobs: %w", err)
	}

	return pgx.CollectRows(rows, pgx.RowToStructByName[Job])
}
