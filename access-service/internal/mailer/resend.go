package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	resendURL = "https://api.resend.com/emails"
	// httpTimeout is short: sending the OTP is on the /activate request path.
	httpTimeout = 15 * time.Second
)

// Resend sends OTP mail via the Resend HTTP API. Request/error idiom mirrors
// the app's internal/updater client. Requires a verified sending domain in
// production (the default MAIL_FROM only reaches your own address).
type Resend struct {
	apiKey     string
	from       string
	httpClient *http.Client
}

// NewResend creates a Resend mailer. from is the "From:" address/label.
func NewResend(apiKey, from string) *Resend {
	return &Resend{
		apiKey:     apiKey,
		from:       from,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

var _ Mailer = (*Resend)(nil)

// SendOTP emails the code as plain text. The message states the 10-minute
// expiry that the server enforces (see server.otpTTL).
func (r *Resend) SendOTP(ctx context.Context, toEmail, code string) error {
	if r.apiKey == "" {
		return fmt.Errorf("mailer: Resend API key is not configured")
	}

	payload := map[string]any{
		"from":    r.from,
		"to":      []string{toEmail},
		"subject": "Your Mogi verification code",
		"text":    fmt.Sprintf("Your Mogi verification code is %s.\n\nIt expires in 10 minutes. If you didn't request this, you can ignore this email.", code),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mailer: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, resendURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mailer: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mailer: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mailer: Resend returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
