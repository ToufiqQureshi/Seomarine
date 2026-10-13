package emailauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Captcha checks a challenge response from the browser.
type Captcha interface {
	// Verify returns nil when the response is a valid solved challenge.
	Verify(ctx context.Context, response, remoteIP string) error
}

// errCaptchaRejected is a response Cloudflare judged invalid, as opposed to a
// verification service that could not be reached.
var errCaptchaRejected = fmt.Errorf("captcha rejected")

// Turnstile verifies Cloudflare Turnstile responses.
type Turnstile struct {
	Secret string
	HTTP   *http.Client
	// URL overrides the verification endpoint; tests point it at a fake.
	URL string
}

// Verify asks Cloudflare whether the response is valid.
func (t Turnstile) Verify(ctx context.Context, response, remoteIP string) (retErr error) {
	target := t.URL
	if target == "" {
		target = turnstileVerifyURL
	}
	form := url.Values{"secret": {t.Secret}, "response": {response}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := t.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call turnstile: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close turnstile response: %w", err))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("turnstile returned %s", resp.Status)
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&result); err != nil {
		return fmt.Errorf("decode turnstile response: %w", err)
	}
	if !result.Success {
		return errCaptchaRejected
	}
	return nil
}
