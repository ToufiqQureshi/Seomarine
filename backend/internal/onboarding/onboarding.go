package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Answers contains a user onboarding questionnaire response.
type Answers struct {
	CompletedAt         *string `json:"completedAt"`
	GSCNudgeDismissedAt *string `json:"gscNudgeDismissedAt"`
	UserCreatedAt       *string `json:"userCreatedAt"`
	Answers             Saved   `json:"answers"`
}

// Saved contains persisted onboarding state.
type Saved struct {
	InterestedFeatures []string `json:"interestedFeatures"`
	WorkFor            *string  `json:"workFor"`
	ClientWebsiteCount *string  `json:"clientWebsiteCount"`
	FoundVia           *string  `json:"foundVia"`
}

// SaveInput contains onboarding answers to persist.
type SaveInput struct {
	InterestedFeatures *[]string
	WorkFor            *string
	ClientWebsiteCount *string
	FoundVia           *string
	Completed          bool
}

// Store persists onboarding state.
type Store interface {
	// Get returns saved onboarding state for a user.
	Get(context.Context, string) (Answers, error)
	// Save persists onboarding answers for a user and organization.
	Save(context.Context, string, string, SaveInput) error
	// DismissGSCNudge records that a user dismissed the Search Console prompt.
	DismissGSCNudge(context.Context, string, string) error
}

// Service reads and updates onboarding state.
type Service struct{ Store Store }

// Get returns saved onboarding state for a user.
func (s *Service) Get(ctx context.Context, userID string) (Answers, error) {
	if s == nil || s.Store == nil {
		return Answers{}, fmt.Errorf("onboarding store unavailable")
	}
	return s.Store.Get(ctx, userID)
}

// Save persists onboarding answers for a user and organization.
func (s *Service) Save(ctx context.Context, userID, organizationID string, in SaveInput) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("onboarding store unavailable")
	}
	return s.Store.Save(ctx, userID, organizationID, in)
}

// DismissGSCNudge records that a user dismissed the Search Console prompt.
func (s *Service) DismissGSCNudge(ctx context.Context, userID, organizationID string) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("onboarding store unavailable")
	}
	return s.Store.DismissGSCNudge(ctx, userID, organizationID)
}
func marshalFeatures(values *[]string) (string, error) {
	if values == nil {
		return "[]", nil
	}
	if *values == nil {
		return "[]", nil
	}
	raw, err := json.Marshal(*values)
	return string(raw), err
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
