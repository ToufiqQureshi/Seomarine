package spend

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteHTTPErrorAnswersOnlyRefusals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		status   int
		code     string
		answered bool
	}{
		{"no credits", fmt.Errorf("hold: %w", ErrNoCredits), http.StatusPaymentRequired, "insufficient_credits", true},
		{"paused", fmt.Errorf("hold: %w", ErrPaused), http.StatusServiceUnavailable, "usage_paused", true},
		{"workspace day used", fmt.Errorf("hold: %w", ErrWorkspaceDailyLimit), http.StatusTooManyRequests, "daily_credit_limit", true},
		{"duplicate request", fmt.Errorf("hold: %w", ErrIdempotentReplay), http.StatusConflict, "duplicate_request", true},
		{"anything else", errors.New("provider down"), 0, "", false},
		{"nil", nil, 0, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if got := WriteHTTPError(recorder, tc.err); got != tc.answered {
				t.Fatalf("WriteHTTPError() = %v, want %v", got, tc.answered)
			}
			if Refused(tc.err) != tc.answered || (Message(tc.err) != "") != tc.answered {
				t.Fatalf("Refused/Message disagree with WriteHTTPError for %v", tc.err)
			}
			if !tc.answered {
				if recorder.Body.Len() != 0 {
					t.Fatalf("wrote %q for an error it does not own", recorder.Body.String())
				}
				return
			}
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", recorder.Body.String(), err)
			}
			if recorder.Code != tc.status || body.Error.Code != tc.code || body.Error.Message != Message(tc.err) {
				t.Fatalf("got %d %+v", recorder.Code, body)
			}
		})
	}
}

func TestFeaturesListsEveryPaidFeatureOnce(t *testing.T) {
	seen := map[Feature]bool{}
	for _, f := range Features() {
		if f == "" || f == Reference || seen[f] {
			t.Fatalf("Features() has %q twice, empty or free", f)
		}
		seen[f] = true
	}
	if len(seen) != 9 {
		t.Fatalf("Features() has %d features, want 9", len(seen))
	}
}
