package audit

import "strings"

// ErrorCode is the closed vocabulary written to audits.error_code by the
// workflow's mark-failed step and by the stale-audit reconciler. Keeping it
// closed makes failures aggregable in SQL and lets the UI map codes to friendly
// copy without leaking raw infrastructure errors to users.
type ErrorCode string

// The known audit failure codes, mirroring AUDIT_ERROR_CODES.
const (
	// ErrorStepTimeout means a workflow step exceeded its timeout.
	ErrorStepTimeout ErrorCode = "step_timeout"
	// ErrorOOM means the worker was killed for exceeding its memory limit.
	ErrorOOM ErrorCode = "oom"
	// ErrorCPULimit means the worker was killed for exceeding its CPU limit.
	ErrorCPULimit ErrorCode = "cpu_limit"
	// ErrorDB means a database query failed permanently.
	ErrorDB ErrorCode = "db_error"
	// ErrorStepOutputTooLarge means a step tried to persist more than the
	// durable-state cap.
	ErrorStepOutputTooLarge ErrorCode = "step_output_too_large"
	// ErrorWorkflowInternal means a workflow internal error not caused by us.
	ErrorWorkflowInternal ErrorCode = "workflow_internal"
	// ErrorInstanceLost means the workflow instance no longer exists while the
	// audit row still said "running".
	ErrorInstanceLost ErrorCode = "instance_lost"
	// ErrorUnknown is anything that could not be classified.
	ErrorUnknown ErrorCode = "unknown"
)

// errorDetailMaxChars bounds the stored detail.
const errorDetailMaxChars = 500

// ErrorInfo classifies an audit failure.
type ErrorInfo struct {
	ErrorCode   ErrorCode `json:"errorCode"`
	ErrorDetail string    `json:"errorDetail"`
}

// ClassifyAuditError classifies an error (thrown in the workflow, or read back
// from a dead workflow instance) into an ErrorCode, truncating the detail
// to a bounded length. Mirrors classifyAuditError.
func ClassifyAuditError(err error) ErrorInfo {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return ErrorInfo{
		ErrorCode:   classifyMessage(message),
		ErrorDetail: truncateChars(message, errorDetailMaxChars),
	}
}

// classifyMessage maps a message onto the closed vocabulary, matching the exact
// failure messages observed in production instances.
func classifyMessage(message string) ErrorCode {
	switch {
	case strings.Contains(message, "exceeded memory limit"):
		return ErrorOOM
	case strings.Contains(message, "exceeded CPU time limit"):
		return ErrorCPULimit
	case strings.Contains(message, "WorkflowTimeoutError"), strings.Contains(message, "Execution timed out"):
		return ErrorStepTimeout
	case strings.Contains(message, "output is too large"):
		return ErrorStepOutputTooLarge
	case strings.Contains(message, "WorkflowInternalError"):
		return ErrorWorkflowInternal
	case strings.HasPrefix(message, "Failed query:"), strings.Contains(message, "D1_ERROR"), strings.Contains(message, "Postgres database accessed outside a request scope"):
		return ErrorDB
	default:
		return ErrorUnknown
	}
}

// truncateChars returns at most max characters (runes) of value.
func truncateChars(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
