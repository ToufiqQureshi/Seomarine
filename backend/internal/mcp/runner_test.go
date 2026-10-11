package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

func newTestRunner() *Runner {
	return NewRunner(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func TestRunnerListsEveryRegisteredTool(t *testing.T) {
	infos := newTestRunner().Tools()
	if len(infos) != len(registry()) {
		t.Fatalf("Tools() = %d, registry has %d", len(infos), len(registry()))
	}
	for _, info := range infos {
		if info.Name == "" || info.Description == "" || !json.Valid(info.InputSchema) {
			t.Errorf("incomplete tool info: %+v", info)
		}
	}
}

func TestRunnerCallUnknownToolIsNotOK(t *testing.T) {
	if _, ok := newTestRunner().Call(context.Background(), Auth{}, "no_such_tool", nil); ok {
		t.Fatal("unknown tool reported ok")
	}
}

func TestRunnerCallPassesArgumentsAndFlattensErrors(t *testing.T) {
	// A known tool with no projectId answers in-band, not with a Go error.
	result, ok := newTestRunner().Call(context.Background(), Auth{}, "get_search_console_performance", json.RawMessage(`{}`))
	if !ok || !result.IsError || result.Text != "projectId is required" {
		t.Fatalf("result = %+v ok = %v", result, ok)
	}
}

func TestRunnerCallDefaultsMissingArgumentsToEmptyObject(t *testing.T) {
	result, ok := newTestRunner().Call(context.Background(), Auth{}, "get_search_console_performance", nil)
	if !ok || result.Text != "projectId is required" {
		t.Fatalf("nil arguments should behave like {}: %+v", result)
	}
}
