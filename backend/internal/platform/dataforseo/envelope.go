package dataforseo

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const statusOK = 20000

var (
	// ErrBillingIssue means the provider account has no balance or a payment
	// problem, so every call fails until it is topped up.
	ErrBillingIssue = errors.New("DataForSEO account has a billing or balance issue")
	// ErrUpstreamUnavailable means DataForSEO's own backend failed the task.
	// Retrying in a moment usually works.
	ErrUpstreamUnavailable = errors.New("DataForSEO is temporarily unavailable")
)

// TaskError is a failed task that is neither a billing issue nor an outage.
type TaskError struct {
	StatusCode int
	Message    string
	// InvalidField is true when the provider rejected a field of our request.
	InvalidField bool
}

func (e *TaskError) Error() string {
	return fmt.Sprintf("DataForSEO task failed with status %d: %s", e.StatusCode, e.Message)
}

var (
	billingStatuses = []int{40200, 40210, 402}
	billingSignals  = []string{"insufficient funds", "balance is too low", "payment required", "billing", "balance", "problem billing", "recharged"}
	// upstreamStatuses are provider-side failures returned on an HTTP 200. An
	// explicit list, not a range: 50100 "Not Implemented" means we posted a
	// task that does not exist, which is our bug.
	upstreamStatuses  = []int{40101, 40103, 50000, 50301, 50302, 50303, 50304, 50401, 50402}
	invalidFieldError = regexp.MustCompile(`(?i)Invalid Field:\s*'[^']+'`)
)

// Results returns the result entries of the first task of a live-endpoint
// response, or an error that says why the task failed.
func Results(payload json.RawMessage) ([]json.RawMessage, error) {
	if len(payload) == 0 {
		return nil, errors.New("DataForSEO returned an empty response")
	}
	var envelope struct {
		StatusCode    int    `json:"status_code"`
		StatusMessage string `json:"status_message"`
		Tasks         []struct {
			StatusCode    int               `json:"status_code"`
			StatusMessage string            `json:"status_message"`
			Result        []json.RawMessage `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode DataForSEO response: %w", err)
	}
	if envelope.StatusCode != statusOK {
		return nil, classify(envelope.StatusCode, envelope.StatusMessage)
	}
	if len(envelope.Tasks) == 0 {
		return nil, errors.New("DataForSEO response has no task")
	}
	task := envelope.Tasks[0]
	if task.StatusCode != statusOK {
		return nil, classify(task.StatusCode, task.StatusMessage)
	}
	return task.Result, nil
}

func classify(status int, message string) error {
	lower := strings.ToLower(message)
	if slices.Contains(billingStatuses, status) || slices.ContainsFunc(billingSignals, func(s string) bool { return strings.Contains(lower, s) }) {
		return fmt.Errorf("%w: %s", ErrBillingIssue, message)
	}
	if slices.Contains(upstreamStatuses, status) {
		return fmt.Errorf("%w: status %d: %s", ErrUpstreamUnavailable, status, message)
	}
	return &TaskError{StatusCode: status, Message: message, InvalidField: invalidFieldError.MatchString(message)}
}
