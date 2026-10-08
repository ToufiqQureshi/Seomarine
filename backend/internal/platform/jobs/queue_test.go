package jobs

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestValidateInput(t *testing.T) {
	tests := []struct {
		name string
		in   EnqueueInput
		want bool
	}{
		{name: "valid", in: EnqueueInput{Queue: "rank-check", Payload: json.RawMessage(`{"project":"p"}`)}},
		{name: "empty queue", in: EnqueueInput{Payload: json.RawMessage(`{}`)}, want: true},
		{name: "invalid JSON", in: EnqueueInput{Queue: "q", Payload: json.RawMessage(`{`)}, want: true},
		{name: "attempt limit", in: EnqueueInput{Queue: "q", Payload: json.RawMessage(`{}`), MaxAttempts: 101}, want: true},
		{name: "timeout too small", in: EnqueueInput{Queue: "q", Payload: json.RawMessage(`{}`), Timeout: time.Millisecond}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInput(tt.in)
			if errors.Is(err, ErrInvalidJob) != tt.want {
				t.Fatalf("validateInput() error = %v, want invalid=%v", err, tt.want)
			}
		})
	}
}

func TestBackoffIsExponentialAndBounded(t *testing.T) {
	if got := backoff(1); got != time.Second {
		t.Fatalf("backoff(1) = %s, want 1s", got)
	}
	if got := backoff(4); got != 8*time.Second {
		t.Fatalf("backoff(4) = %s, want 8s", got)
	}
	if got := backoff(100); got != maxBackoff {
		t.Fatalf("backoff(100) = %s, want %s", got, maxBackoff)
	}
}
