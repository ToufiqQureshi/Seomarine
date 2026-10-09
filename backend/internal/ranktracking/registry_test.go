package ranktracking

import (
	"context"
	"errors"
	"testing"
)

type fakeRegistry struct {
	names []string
	err   error
	orgs  []string
	codes []string
}

func (f *fakeRegistry) LocationNames(_ context.Context, org, country string) ([]string, error) {
	f.orgs = append(f.orgs, org)
	f.codes = append(f.codes, country)
	return f.names, f.err
}

func TestRegistryChecker(t *testing.T) {
	reg := &fakeRegistry{names: []string{"Mumbai,Maharashtra,India"}}
	c := RegistryChecker{Registry: reg}
	ctx := context.Background()

	if err := c.Check(ctx, "org1", "Mumbai,Maharashtra,India", "en", 2356); err != nil {
		t.Fatalf("known name rejected: %v", err)
	}
	if reg.orgs[0] != "org1" || reg.codes[0] != "in" {
		t.Errorf("registry asked for org %q country %q", reg.orgs[0], reg.codes[0])
	}
	var v ValidationError
	if err := c.Check(ctx, "org1", "Atlantis", "en", 2356); !errors.As(err, &v) {
		t.Errorf("unknown name: want ValidationError, got %v", err)
	}
	reg.err = errors.New("redis down")
	if err := c.Check(ctx, "org1", "Mumbai,Maharashtra,India", "en", 2356); !errors.Is(err, ErrLocationUnavailable) {
		t.Errorf("registry error must fail closed, got %v", err)
	}
}
