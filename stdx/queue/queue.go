package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/uuid"
)

// TODO: detect job that have expired (timeout)

const (
	MinRetryMax     int64 = 0
	MaxRetryMax     int64 = 100
	DefaultRetryMax int64 = 5

	MinRetryDelay     int64 = 1
	MaxRetryDelay     int64 = 86_400 // 1 day
	DefaultRetryDelay int64 = 5

	DefaultRetryStrategy = RetryStrategyConstant

	MinTimeout     int64 = 1
	MaxTimeout     int64 = 7200
	DefaultTimeout int64 = 60
)

type JobStatus int32

const (
	JobStatusQueued JobStatus = iota
	JobStatusRunning
	JobStatusFailed
)

type RetryStrategy int32

const (
	RetryStrategyConstant RetryStrategy = iota
	RetryStrategyExponential
)

var (
	ErrJobSatusIsNotValid = func(status string) error {
		return fmt.Errorf(`Job status "%s" is not valid`, status)
	}
	ErrRetryStrategyIsNotValid = func(status string) error {
		return fmt.Errorf(`Retry strategy "%s" is not valid`, status)
	}
)

type Queue interface {
	Push(ctx context.Context, tx db.Queryer, newJob NewJobInput) error
	PushMany(ctx context.Context, t db.Tx, newJobs []NewJobInput) error
	// pull fetches at most `number_of_jobs` from the queue.
	Pull(ctx context.Context, numberOfJobs uint64) ([]Job, error)
	DeleteJob(ctx context.Context, jobID uuid.UUID) error
	FailJob(ctx context.Context, job Job) error
	Clear(ctx context.Context) error
	GetJob(ctx context.Context, jobID uuid.UUID) (job Job, err error)

	GetFailedJobs(ctx context.Context, limit int64) (jobs []Job, err error)
}

type JobData interface {
	JobType() string
}

type NewJobInput struct {
	Data JobData

	// ScheduledFor is the date when the job should be scheduled for
	// default: time.Now()
	ScheduledFor *time.Time

	// RetryMax is the max number of times a job should be retried
	// 0-100
	// default: 5
	RetryMax *int64

	// RetryDelay is the number of seconds between 2 retry attempts. Allowed range: 1-86400
	// default: 5
	RetryDelay *int64

	// constant, exponential
	// default: Constant
	RetryStrategy RetryStrategy

	// Timeout in seconds. Allows range: 1-7200
	// default: 60
	Timeout *int64
}

type Job struct {
	ID             uuid.UUID `db:"id" json:"id"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
	ScheduledFor   time.Time `db:"scheduled_for" json:"scheduled_for"`
	FailedAttempts int64     `db:"failed_attempts" json:"failed_attempts"`
	// priority: i64,
	Status        JobStatus       `db:"status" json:"status"`
	Type          string          `db:"type" json:"type"`
	RawData       json.RawMessage `db:"data" json:"data"`
	RetryMax      int64           `db:"retry_max" json:"retry_max"`
	RetryDelay    int64           `db:"retry_delay" json:"retry_delay"`
	RetryStrategy RetryStrategy   `db:"retry_strategy" json:"retry_strategy"`
	Timeout       int64           `db:"timeout" json:"timeout"`
}

func (job *Job) GetData(data any) (err error) {
	err = json.Unmarshal(job.RawData, &data)
	return err
}

func (status JobStatus) MarshalText() (ret []byte, err error) {
	switch status {
	case JobStatusQueued:
		ret = []byte("queued")
	case JobStatusRunning:
		ret = []byte("running")
	case JobStatusFailed:
		ret = []byte("failed")
	default:
		err = ErrJobSatusIsNotValid(strconv.Itoa(int(status)))
		return nil, err
	}

	return ret, nil
}

func (status JobStatus) String() string {
	ret, _ := status.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (status *JobStatus) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case "queued":
		*status = JobStatusQueued
	case "running":
		*status = JobStatusRunning
	case "failed":
		*status = JobStatusFailed
	default:
		err = ErrJobSatusIsNotValid(string(data))
		return err
	}

	return nil
}

func (strategy RetryStrategy) MarshalText() (ret []byte, err error) {
	switch strategy {
	case RetryStrategyConstant:
		ret = []byte("constant")
	case RetryStrategyExponential:
		ret = []byte("exponential")
	default:
		err = ErrRetryStrategyIsNotValid(strconv.Itoa(int(strategy)))
		return nil, err
	}

	return ret, nil
}

func (strategy RetryStrategy) String() string {
	ret, _ := strategy.MarshalText()
	return string(ret)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (strategy *RetryStrategy) UnmarshalText(data []byte) (err error) {
	switch string(data) {
	case "constant":
		*strategy = RetryStrategyConstant
	case "exponential":
		*strategy = RetryStrategyExponential
	default:
		err = ErrRetryStrategyIsNotValid(string(data))
		return err
	}

	return nil
}
