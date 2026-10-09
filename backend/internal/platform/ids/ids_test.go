package ids

import (
	"regexp"
	"testing"
)

func TestNewIsAVersion4UUID(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 1000 {
		id, err := New()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(id) {
			t.Fatalf("New() = %q, want a version 4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("New() repeated %q", id)
		}
		seen[id] = true
	}
}
