package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportToolsExposeSchemasAndAnnotations(t *testing.T) {
	want := map[string]struct{ readOnly, openWorld, destructive bool }{
		"save_report": {false, true, true}, "list_reports": {true, false, false}, "get_report": {true, false, false},
		"delete_report": {false, true, true}, "set_report_sharing": {false, true, true},
		"list_report_templates": {true, false, false}, "save_report_template": {false, false, true}, "delete_report_template": {false, false, true},
	}
	got := map[string]*tool{}
	for _, item := range reportTools() {
		got[item.Name] = item
		if !json.Valid(item.InputSchema) || !json.Valid(item.OutputSchema) {
			t.Errorf("%s has invalid schema JSON", item.Name)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("tool count = %d, want %d", len(got), len(want))
	}
	for name, expected := range want {
		item, ok := got[name]
		if !ok {
			t.Errorf("%s missing from MCP report tools", name)
			continue
		}
		if actual := item.Annotations["readOnlyHint"].(bool); actual != expected.readOnly {
			t.Errorf("%s readOnlyHint = %v; want %v", name, actual, expected.readOnly)
		}
		for annotation, value := range map[string]bool{"openWorldHint": expected.openWorld, "destructiveHint": expected.destructive} {
			if actual, ok := item.Annotations[annotation].(bool); !ok || actual != value {
				t.Errorf("%s %s = %v; want %v", name, annotation, actual, value)
			}
		}
	}
	registryNames := map[string]bool{}
	for _, item := range registry() {
		if registryNames[item.Name] {
			t.Fatalf("duplicate MCP tool %q", item.Name)
		}
		registryNames[item.Name] = true
	}
	for name := range want {
		if !registryNames[name] {
			t.Errorf("%s is not in the MCP registry", name)
		}
	}
}

func TestReportToolDecoderRejectsUnknownAndTrailingJSON(t *testing.T) {
	for _, raw := range []string{`{"projectId":"p","unknown":true}`, `{"projectId":"p"}{"projectId":"other"}`, `null`} {
		t.Run(raw, func(t *testing.T) {
			var args reportArgs
			if err := decodeReportArgs(json.RawMessage(raw), &args); err == nil {
				t.Fatal("invalid report tool args were accepted")
			}
		})
	}
	var args reportArgs
	if err := decodeReportArgs(json.RawMessage(`{"projectId":"p","limit":10}`), &args); err != nil || args.ProjectID != "p" || args.Limit != 10 {
		t.Fatalf("valid args decoded as %+v, error %v", args, err)
	}
}

func TestReportToolPreviewTruncatesWithoutSplittingUTF8(t *testing.T) {
	input := strings.Repeat("a", 239) + "🧭"
	if got := preview(input); got != input {
		t.Fatalf("preview changed 240-rune value: got %d runes", len([]rune(got)))
	}
	input += "x"
	if got := preview(input); len([]rune(got)) != 240 || !strings.HasSuffix(got, "...") {
		t.Fatalf("preview did not truncate to 240 runes: %q", got)
	}
}
