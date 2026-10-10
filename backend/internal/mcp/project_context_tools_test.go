package mcp

import (
	"encoding/json"
	"testing"
)

func TestProjectContextToolsAreRegisteredWithValidContracts(t *testing.T) {
	index := map[string]*tool{}
	for _, registered := range registry() {
		index[registered.Name] = registered
	}
	for _, name := range []string{"get_project_context", "update_project_context"} {
		tool, ok := index[name]
		if !ok {
			t.Fatalf("tool %q is not registered", name)
		}
		if !json.Valid(tool.InputSchema) || !json.Valid(tool.OutputSchema) {
			t.Fatalf("tool %q has invalid JSON schema", name)
		}
		if tool.Handler == nil || tool.Annotations == nil {
			t.Fatalf("tool %q has incomplete definition", name)
		}
	}
	if index["update_project_context"].Annotations["readOnlyHint"] != false {
		t.Fatal("update_project_context must be marked as mutating")
	}
}
