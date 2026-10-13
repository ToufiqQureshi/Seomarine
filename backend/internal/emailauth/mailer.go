package emailauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/email"
)

// Mailer sends the account emails. A nil Mailer turns verification and password
// reset off, which is how a deployment without an email provider behaves.
type Mailer interface {
	SendVerification(ctx context.Context, address, confirmationURL string) error
	SendPasswordReset(ctx context.Context, address, resetURL string) error
	// UpsertSignupContact adds the new user to the marketing audience. Failure
	// is logged by the caller and never blocks sign-up.
	UpsertSignupContact(ctx context.Context, userID, address, name string) error
}

const (
	appName          = "Uplumo"
	loopsContactsURL = "https://app.loops.so/api/v1/contacts/update"
)

// LoopsMailer sends account emails through Loops transactional templates.
type LoopsMailer struct {
	Client           email.LoopsClient
	VerifyTemplateID string
	ResetPasswordID  string
	HTTP             *http.Client
	contactsURL      string
}

// SendVerification sends the confirm-your-email message.
func (m LoopsMailer) SendVerification(ctx context.Context, address, confirmationURL string) error {
	return m.Client.SendTransactional(ctx, address, m.VerifyTemplateID, map[string]string{
		"appName": appName, "confirmationUrl": confirmationURL,
	})
}

// SendPasswordReset sends the reset-your-password message.
func (m LoopsMailer) SendPasswordReset(ctx context.Context, address, resetURL string) error {
	return m.Client.SendTransactional(ctx, address, m.ResetPasswordID, map[string]string{
		"appName": appName, "resetUrl": resetURL,
	})
}

// UpsertSignupContact records the new user as a Loops contact.
func (m LoopsMailer) UpsertSignupContact(ctx context.Context, userID, address, name string) (retErr error) {
	if strings.TrimSpace(m.Client.APIKey) == "" {
		return nil
	}
	payload := map[string]any{
		"email": address, "userId": userID, "source": "uplumo-signup", "userGroup": "app-user",
	}
	if parts := strings.Fields(name); len(parts) > 0 {
		payload["firstName"] = parts[0]
		if len(parts) > 1 {
			payload["lastName"] = strings.Join(parts[1:], " ")
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Loops contact: %w", err)
	}
	target := m.contactsURL
	if target == "" {
		target = loopsContactsURL
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build Loops contact request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.Client.APIKey)
	client := m.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("update Loops contact: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close Loops contact response: %w", err))
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.New("Loops contact update returned " + resp.Status)
	}
	return nil
}
