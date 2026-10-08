package dataforseo

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestResults(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		want        []string
		wantErr     error
		wantTaskErr *TaskError
		wantAny     bool
	}{
		{name: "returns the first task's results", payload: `{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"a":1},{"b":2}]},{"status_code":20000,"result":[{"c":3}]}]}`, want: []string{`{"a":1}`, `{"b":2}`}},
		{name: "a task with no results is empty, not an error", payload: `{"status_code":20000,"tasks":[{"status_code":20000,"result":null}]}`},
		{name: "top-level billing failure", payload: `{"status_code":40200,"status_message":"Payment Required.","tasks":[]}`, wantErr: ErrBillingIssue},
		{name: "task billing failure by status", payload: `{"status_code":20000,"tasks":[{"status_code":40210,"status_message":"anything"}]}`, wantErr: ErrBillingIssue},
		{name: "task billing failure by message", payload: `{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"Your account balance is too low."}]}`, wantErr: ErrBillingIssue},
		{name: "provider outage", payload: `{"status_code":20000,"tasks":[{"status_code":40101,"status_message":"Internal SE Server Error."}]}`, wantErr: ErrUpstreamUnavailable},
		{name: "provider timeout", payload: `{"status_code":50401,"status_message":"Internal Error - Timeout."}`, wantErr: ErrUpstreamUnavailable},
		{name: "not implemented is our bug, not an outage", payload: `{"status_code":20000,"tasks":[{"status_code":50100,"status_message":"Not Implemented."}]}`, wantTaskErr: &TaskError{StatusCode: 50100, Message: "Not Implemented."}},
		{name: "invalid field is flagged", payload: `{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"Invalid Field: 'target'."}]}`, wantTaskErr: &TaskError{StatusCode: 40501, Message: "Invalid Field: 'target'.", InvalidField: true}},
		{name: "no search results is a failure like any other", payload: `{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"No Search Results."}]}`, wantTaskErr: &TaskError{StatusCode: 40501, Message: "No Search Results."}},
		{name: "no task", payload: `{"status_code":20000,"tasks":[]}`, wantAny: true},
		{name: "empty body", payload: ``, wantAny: true},
		{name: "not JSON", payload: `<html>`, wantAny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Results(json.RawMessage(tt.payload))
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Results() error = %v, want %v", err, tt.wantErr)
				}
			case tt.wantTaskErr != nil:
				var taskErr *TaskError
				if !errors.As(err, &taskErr) || *taskErr != *tt.wantTaskErr {
					t.Fatalf("Results() error = %v, want %+v", err, tt.wantTaskErr)
				}
				if errors.Is(err, ErrBillingIssue) || errors.Is(err, ErrUpstreamUnavailable) {
					t.Fatalf("Results() error %v is also classified as billing or outage", err)
				}
			case tt.wantAny:
				if err == nil {
					t.Fatalf("Results() = %v, want an error", got)
				}
			default:
				if err != nil {
					t.Fatalf("Results() error = %v", err)
				}
				var gotStrings []string
				for _, raw := range got {
					gotStrings = append(gotStrings, string(raw))
				}
				if !reflect.DeepEqual(gotStrings, tt.want) {
					t.Fatalf("Results() = %v, want %v", gotStrings, tt.want)
				}
			}
		})
	}
}
