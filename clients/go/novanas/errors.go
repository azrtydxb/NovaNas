package novanas

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is returned for any non-2xx response from the NovaNAS API. It
// wraps the standard JSON envelope: {"error":"<code>","message":"<human>"}.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

// Error implements error.
func (e *APIError) Error() string {
	if e.Code == "" && e.Message == "" {
		return fmt.Sprintf("novanas: HTTP %d", e.StatusCode)
	}
	if e.Message == "" {
		return fmt.Sprintf("novanas: HTTP %d (%s)", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("novanas: HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// IsNotFound reports whether err is an *APIError with status 404.
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// IsForbidden reports whether err is an *APIError with status 401 or 403.
// Both are surfaced because the API uses 401 for missing/invalid bearer
// tokens and 403 for authenticated-but-unauthorized.
func IsForbidden(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.StatusCode == http.StatusForbidden || ae.StatusCode == http.StatusUnauthorized
}

// IsConflict reports whether err is an *APIError with status 409.
func IsConflict(err error) bool { return statusIs(err, http.StatusConflict) }

func statusIs(err error, status int) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.StatusCode == status
}

// JobFailedError is returned by WaitJob when the job ends in a terminal
// non-success state (failed, cancelled, or interrupted). It carries the
// final Job representation so callers can inspect stderr, exit code, etc.
type JobFailedError struct {
	Job *Job
}

// Error implements error.
func (e *JobFailedError) Error() string {
	if e.Job == nil {
		return "novanas: job failed"
	}
	msg := fmt.Sprintf("novanas: job %s ended in state %q", e.Job.ID, e.Job.State)
	if e.Job.Error != nil && *e.Job.Error != "" {
		msg += ": " + *e.Job.Error
	}
	return msg
}
