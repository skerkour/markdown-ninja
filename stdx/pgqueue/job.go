package pgqueue

import (
	"encoding/json"
	"strconv"
	"time"

	"markdown.ninja/stdx/uuid"
)

type JobData interface {
	JobType() string
}

type Job struct {
	ID             uuid.UUID       `db:"id" json:"id"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
	ScheduledFor   time.Time       `db:"scheduled_for" json:"scheduled_for"`
	FailedAttempts int32           `db:"failed_attempts" json:"failed_attempts"`
	Status         JobStatus       `db:"status" json:"status"`
	Type           string          `db:"type" json:"type"`
	DataJson       json.RawMessage `db:"data" json:"data"`
	// Maximum number of retries
	RetryMax      int32         `db:"retry_max" json:"retry_max"`
	RetryDelay    int32         `db:"retry_delay" json:"retry_delay"`
	RetryStrategy RetryStrategy `db:"retry_strategy" json:"retry_strategy"`
	// Time allowed for the job to complete before being re-queued or marked as failed, in seconds
	// Time allowed for the job to complete before being re-queued or marked as failed, in seconds. Only relevant when status = 1 (running).
	Timeout int32 `db:"timeout" json:"timeout"`
	// DeadlineAt is computed at Pull time as now + (timeout * 1s). Only relevant when status = 1 (running). Enables efficient timeout scans.
	DeadlineAt time.Time `db:"deadline_at" json:"deadline_at"`
}

type NewJobInput struct {
	Data JobData

	// ScheduledFor is the date when the job should be scheduled for
	// default: time.Now()
	ScheduledFor *time.Time

	// RetryMax is the max number of times a job should be retried
	// 0-100
	// default: 5
	RetryMax *int32

	// RetryDelay is the number of seconds between 2 retry attempts. Allowed range: 1-86400
	// default: 5
	RetryDelay *int32

	// constant, exponential
	// default: Constant
	RetryStrategy RetryStrategy

	// Timeout in seconds. Allowed range: 1-7200
	// default: 60
	Timeout *int32
}

type JobStatus int32

const (
	JobStatusQueued JobStatus = iota
	JobStatusRunning
	JobStatusFailed
)

func (status JobStatus) String() string {
	switch status {
	case JobStatusQueued:
		return "queued"
	case JobStatusRunning:
		return "running"
	case JobStatusFailed:
		return "failed"
	default:
		return strconv.Itoa(int(status))
	}
}

func (status JobStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(status.String())
}

func (status *JobStatus) UnmarshalJSON(data []byte) error {
	var dataStr string
	if err := json.Unmarshal(data, &dataStr); err != nil {
		return err
	}

	switch dataStr {
	case "queued":
		*status = JobStatusQueued
	case "running":
		*status = JobStatusRunning
	case "failed":
		*status = JobStatusFailed
	default:
		return ErrJobSatusIsNotValid(dataStr)
	}

	return nil
}

type RetryStrategy int32

const (
	RetryStrategyConstant RetryStrategy = iota
	RetryStrategyExponential
)

func (strategy RetryStrategy) String() string {
	switch strategy {
	case RetryStrategyConstant:
		return "constant"
	case RetryStrategyExponential:
		return "exponential"
	default:
		return strconv.Itoa(int(strategy))
	}
}

func (strategy RetryStrategy) MarshalJSON() ([]byte, error) {
	return json.Marshal(strategy.String())
}

func (strategy *RetryStrategy) UnmarshalJSON(data []byte) error {
	var dataStr string
	if err := json.Unmarshal(data, &dataStr); err != nil {
		return err
	}
	switch dataStr {
	case "constant":
		*strategy = RetryStrategyConstant
	case "exponential":
		*strategy = RetryStrategyExponential
	default:
		return ErrRetryStrategyIsNotValid(string(data))
	}

	return nil
}
