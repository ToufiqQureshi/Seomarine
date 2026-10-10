package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRankTrackerToolIsRegisteredAsReadOnly(t *testing.T) {
	var got *tool
	for _, candidate := range registry() {
		if candidate.Name == "get_rank_tracker" {
			got = candidate
			break
		}
	}
	if got == nil {
		t.Fatal("get_rank_tracker is not registered")
	}
	if got.Annotations["readOnlyHint"] != true || got.Annotations["destructiveHint"] != false {
		t.Fatalf("annotations = %#v, want read-only and non-destructive", got.Annotations)
	}
	var schema map[string]any
	if err := json.Unmarshal(got.InputSchema, &schema); err != nil {
		t.Fatalf("decode input schema: %v", err)
	}
	if schema["additionalProperties"] != false {
		t.Fatal("input schema must reject unknown fields")
	}
}

func TestRankTrackingMutationToolsAreRegistered(t *testing.T) {
	want := map[string]bool{"add_rank_tracking_keywords": false, "remove_rank_tracking_keywords": true}
	for _, candidate := range registry() {
		if destructive, ok := want[candidate.Name]; ok {
			if candidate.Annotations["destructiveHint"] != destructive || candidate.Annotations["readOnlyHint"] != false {
				t.Errorf("%s annotations = %#v", candidate.Name, candidate.Annotations)
			}
			delete(want, candidate.Name)
		}
	}
	for name := range want {
		t.Errorf("%s is not registered", name)
	}
}

func TestGetRankTrackerRejectsInvalidTrackerID(t *testing.T) {
	env := &callEnv{}
	_, err := handleGetRankTracker(context.Background(), json.RawMessage(`{"projectId":"project","trackerId":"not-a-uuid"}`), env)
	appErr, ok := err.(*appError)
	if !ok || appErr.code != "VALIDATION_ERROR" {
		t.Fatalf("error = %#v, want VALIDATION_ERROR", err)
	}
}

func TestRankTrackingMutationsRejectInvalidIDsBeforeAuthorization(t *testing.T) {
	for name, handler := range map[string]func(context.Context, json.RawMessage, *callEnv) (*callResult, error){
		"add":    handleAddRankTrackingKeywords,
		"remove": handleRemoveRankTrackingKeywords,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(`{"projectId":"project","trackerId":"bad","keywords":["term"],"keywordIds":["bad"]}`), &callEnv{})
			appErr, ok := err.(*appError)
			if !ok || appErr.code != "VALIDATION_ERROR" {
				t.Fatalf("error = %#v, want VALIDATION_ERROR", err)
			}
		})
	}
}
