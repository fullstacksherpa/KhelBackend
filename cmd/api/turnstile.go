package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrTurnstileFailed = errors.New("turnstile validation failed")

type turnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
}

func (app *application) verifyTurnstile(ctx context.Context, token string, remoteIP string) (*turnstileVerifyResponse, error) {
	if token == "" {
		return nil, ErrTurnstileFailed
	}
	if app.config.turnstile.secretKey == "" {
		return nil, errors.New("TURNSTILE_SECRET_KEY is not set")
	}

	form := url.Values{}
	form.Set("secret", app.config.turnstile.secretKey)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 8 * time.Second}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var out turnstileVerifyResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}

	if !out.Success {
		return &out, ErrTurnstileFailed
	}

	// Optional hardening: verify hostname
	if app.config.turnstile.expectedHostname != "" && out.Hostname != app.config.turnstile.expectedHostname {
		return &out, ErrTurnstileFailed
	}

	return &out, nil
}
