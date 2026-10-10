package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Answers struct {
	CompletedAt         *string `json:"completedAt"`
	GSCNudgeDismissedAt *string `json:"gscNudgeDismissedAt"`
	UserCreatedAt       *string `json:"userCreatedAt"`
	Answers             Saved   `json:"answers"`
}
type Saved struct {
	InterestedFeatures []string `json:"interestedFeatures"`
	WorkFor            *string  `json:"workFor"`
	ClientWebsiteCount *string  `json:"clientWebsiteCount"`
	FoundVia           *string  `json:"foundVia"`
}
type SaveInput struct {
	InterestedFeatures *[]string
	WorkFor            *string
	ClientWebsiteCount *string
	FoundVia           *string
	Completed          bool
}
type Store interface {
	Get(context.Context, string) (Answers, error)
	Save(context.Context, string, string, SaveInput) error
	DismissGSCNudge(context.Context, string, string) error
}
type Service struct{ Store Store }

func (s *Service) Get(ctx context.Context, userID string) (Answers, error) {
	if s == nil || s.Store == nil {
		return Answers{}, fmt.Errorf("onboarding store unavailable")
	}
	return s.Store.Get(ctx, userID)
}
func (s *Service) Save(ctx context.Context, userID, organizationID string, in SaveInput) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("onboarding store unavailable")
	}
	return s.Store.Save(ctx, userID, organizationID, in)
}
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
