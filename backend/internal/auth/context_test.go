package auth

import (
	"context"
	"testing"
)

func TestUserFromContext(t *testing.T) {
	if _, ok := UserFromContext(context.Background()); ok {
		t.Fatal("UserFromContext(background) unexpectedly found a user")
	}

	want := User{ID: "user-1", OrganizationID: "org-1", Role: "owner"}
	got, ok := UserFromContext(WithUser(context.Background(), want))
	if !ok || got != want {
		t.Fatalf("UserFromContext() = (%+v, %v), want (%+v, true)", got, ok, want)
	}
}
