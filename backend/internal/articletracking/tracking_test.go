package articletracking

import (
	"errors"
	"testing"
)

func TestNormalizePublishedURL(t *testing.T) {
	tests := []struct {
		name, input, want string
		invalid           bool
	}{
		{name: "normalizes host and drops fragment", input: "HTTPS://Example.COM/post#section", want: "https://example.com/post"},
		{name: "rejects credentials", input: "https://user:pass@example.com/post", invalid: true},
		{name: "rejects non-http scheme", input: "javascript:alert(1)", invalid: true},
		{name: "rejects missing hostname", input: "https:///post", invalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePublishedURL(tc.input)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("NormalizePublishedURL()=(%q,%v), want %q", got, err, tc.want)
			}
		})
	}
}
