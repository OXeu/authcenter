package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Resend sends verification messages through the Resend API.
type Resend struct {
	APIKey   string
	From     string
	Client   *http.Client
	Endpoint string
}

// Send sends a single short-lived code using an idempotent request.
func (r Resend) Send(ctx context.Context, email, code, id string) error {
	body, err := json.Marshal(map[string]any{"from": r.From, "to": []string{email}, "subject": "AuthCenter verification code", "text": "Your AuthCenter verification code is " + code + ". It expires in 5 minutes. 验证码有效期为 5 分钟。"})
	if err != nil {
		return fmt.Errorf("encode email: %w", err)
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = "https://api.resend.com/emails"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create email request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "authcenter/"+id)
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	closeErr := response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read email response: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close email response: %w", closeErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("verification email status %d", response.StatusCode)
	}
	return nil
}
