package queue

import (
	"context"
	"encoding/json"
	"time"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

// TODO: detect job that have expired (timeout)
type RetryStrategy int32
type JobStatus int32

const (
	MinRetryMax     int64 = 0
	MaxRetryMax     int64 = 100
	DefaultRetryMax int64 = 5

	MinRetryDelay     int64 = 1
	MaxRetryDelay     int64 = 86_400
	DefaultRetryDelay int64 = 5

	DefaultRetryStrategy = RetryStrategyConstant

	MinTimeout     int64 = 1
	MaxTimeout     int64 = 7200
	DefaultTimeout int64 = 60

	RetryStrategyConstant    RetryStrategy = 0
	RetryStrategyExponential RetryStrategy = 1

	JobStatusQueued  JobStatus = 0
	JobStatusRunning JobStatus = 1
	JobStatusFailed  JobStatus = 2
)

type Queue interface {
	Push(ctx context.Context, tx db.Queryer, newJob NewJobInput) error
	// pull fetches at most `number_of_jobs` from the queue.
	Pull(ctx context.Context, numberOfJobs int64) ([]Job, error)
	DeleteJob(ctx context.Context, jobID guid.GUID) error
	FailJob(ctx context.Context, job Job) error
	Clear(ctx context.Context) error
	Stop(ctx context.Context)
}

type NewJobInput struct {
	Type string
	Data any

	// ScheduledFor is the date when the job should be scheduled for
	// now() if empty
	ScheduledFor *time.Time

	// RetryMax is the max number of times a job should be retried
	// 0-100
	// default: 5
	RetryMax *int64

	// RetryDelay is the number of seconds between 2 retry attempts
	// 1-86400
	// default: 5
	RetryDelay *int64

	// constant, exponential
	// default: Constant
	RetryStrategy RetryStrategy

	// 1-7200
	// default: 60
	Timeout *int64
}

type Job struct {
	ID             guid.GUID `db:"id"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
	ScheduledFor   time.Time `db:"scheduled_for"`
	FailedAttempts int64     `db:"failed_attempts"`
	// priority: i64,
	Status        JobStatus       `db:"status"`
	Type          string          `db:"type"`
	RawData       json.RawMessage `db:"data"`
	RetryMax      int64           `db:"retry_max"`
	RetryDelay    int64           `db:"retry_delay"`
	RetryStrategy RetryStrategy   `db:"retry_strategy"`
	Timeout       int64           `db:"timeout"`
}

func (job *Job) GetData(data any) (err error) {
	err = json.Unmarshal(job.RawData, &data)
	if err != nil {
		return
	}

	return
}
