package pgqueue

import (
	"encoding/json"
	"testing"
	"time"

	"markdown.ninja/stdx/uuid"
)

type testJobData struct {
	Message string `json:"message"`
}

func (d testJobData) JobType() string {
	return "test_job"
}

type emptyTypeJobData struct{}

func (d emptyTypeJobData) JobType() string {
	return ""
}

type nonPointerJobData struct {
	Msg string `json:"msg"`
}

func (d nonPointerJobData) JobType() string {
	return "non_pointer"
}

func TestValidateJob_Valid(t *testing.T) {
	now := time.Now().UTC()
	input := NewJobInput{
		Data: testJobData{Message: "hello"},
	}

	job, err := validateJob(now, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Type != "test_job" {
		t.Errorf("expected type 'test_job', got %q", job.Type)
	}
	if job.Status != JobStatusQueued {
		t.Errorf("expected status queued, got %v", job.Status)
	}
	if job.FailedAttempts != 0 {
		t.Errorf("expected 0 failed attempts, got %d", job.FailedAttempts)
	}
	if job.ID == (uuid.UUID{}) {
		t.Error("expected non-nil UUID")
	}
	if !job.ScheduledFor.Equal(now) {
		t.Errorf("expected scheduled_for %v, got %v", now, job.ScheduledFor)
	}
	if job.RetryMax != DefaultRetryMax {
		t.Errorf("expected default retry_max %d, got %d", DefaultRetryMax, job.RetryMax)
	}
	if job.RetryDelay != DefaultRetryDelay {
		t.Errorf("expected default retry_delay %d, got %d", DefaultRetryDelay, job.RetryDelay)
	}
	if job.RetryStrategy != DefaultRetryStrategy {
		t.Errorf("expected default retry_strategy %v, got %v", DefaultRetryStrategy, job.RetryStrategy)
	}
	if job.Timeout != DefaultTimeout {
		t.Errorf("expected default timeout %d, got %d", DefaultTimeout, job.Timeout)
	}

	var decoded testJobData
	if err := json.Unmarshal(job.DataJson, &decoded); err != nil {
		t.Fatalf("unmarshal raw data: %v", err)
	}
	if decoded.Message != "hello" {
		t.Errorf("expected message 'hello', got %q", decoded.Message)
	}
}

func TestValidateJob_CustomValues(t *testing.T) {
	now := time.Now().UTC()
	scheduledFor := now.Add(time.Hour)
	retryMax := int32(10)
	retryDelay := int32(30)
	timeout := int32(120)

	input := NewJobInput{
		Data:          testJobData{Message: "custom"},
		ScheduledFor:  &scheduledFor,
		RetryMax:      &retryMax,
		RetryDelay:    &retryDelay,
		RetryStrategy: RetryStrategyExponential,
		Timeout:       &timeout,
	}

	job, err := validateJob(now, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !job.ScheduledFor.Equal(scheduledFor) {
		t.Errorf("expected scheduled_for %v, got %v", scheduledFor, job.ScheduledFor)
	}
	if job.RetryMax != retryMax {
		t.Errorf("expected retry_max %d, got %d", retryMax, job.RetryMax)
	}
	if job.RetryDelay != retryDelay {
		t.Errorf("expected retry_delay %d, got %d", retryDelay, job.RetryDelay)
	}
	if job.RetryStrategy != RetryStrategyExponential {
		t.Errorf("expected exponential strategy, got %v", job.RetryStrategy)
	}
	if job.Timeout != timeout {
		t.Errorf("expected timeout %d, got %d", timeout, job.Timeout)
	}
}

func TestValidateJob_EmptyType(t *testing.T) {
	_, err := validateJob(time.Now().UTC(), NewJobInput{
		Data: emptyTypeJobData{},
	})
	if err != ErrJobTypeIsNotValid {
		t.Errorf("expected ErrJobTypeIsNotValid, got %v", err)
	}
}

func TestValidateJob_NilData(t *testing.T) {
	_, err := validateJob(time.Now().UTC(), NewJobInput{})
	if err != ErrJobDataIsNotValid {
		t.Errorf("expected ErrJobDataIsNotValid, got %v", err)
	}
}

func TestValidateJob_RetryMaxOutOfRange(t *testing.T) {
	tooHigh := int32(MaxRetryMax + 1)
	_, err := validateJob(time.Now().UTC(), NewJobInput{
		Data:     testJobData{},
		RetryMax: &tooHigh,
	})
	if err != ErrJobRetryMaxIsNotValid {
		t.Errorf("expected ErrJobRetryMaxIsNotValid, got %v", err)
	}
}

func TestValidateJob_RetryDelayOutOfRange(t *testing.T) {
	tooLow := int32(MinRetryDelay - 1)
	_, err := validateJob(time.Now().UTC(), NewJobInput{
		Data:       testJobData{},
		RetryDelay: &tooLow,
	})
	if err != ErrJobRetryDelayIsNotValid {
		t.Errorf("expected ErrJobRetryDelayIsNotValid, got %v", err)
	}
}

func TestValidateJob_TimeoutOutOfRange(t *testing.T) {
	tooHigh := int32(MaxTimeout + 1)
	_, err := validateJob(time.Now().UTC(), NewJobInput{
		Data:    testJobData{},
		Timeout: &tooHigh,
	})
	if err != ErrJobTimeoutIsNotValid {
		t.Errorf("expected ErrJobTimeoutIsNotValid, got %v", err)
	}
}

func TestPostgreSQLQueueImplementsQueue(t *testing.T) {
	var _ Queue = (*PostgreSQLQueue)(nil)
}

func TestScheduledForZero(t *testing.T) {
	now := time.Now().UTC()

	job, err := validateJob(now, NewJobInput{
		Data: testJobData{Message: "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.ScheduledFor.IsZero() {
		t.Error("expected scheduled_for to be set, got zero")
	}
}
