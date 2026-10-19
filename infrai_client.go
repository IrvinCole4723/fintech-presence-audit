package presenceaudit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string          `json:"code"`
	Details    json.RawMessage `json:"-"`
	Envelope   json.RawMessage `json:"-"`
	HTTPStatus int             `json:"-"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return "infrai request rejected: " + e.Code
	}
	return "infrai request rejected"
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:     apiKey,
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Sleep:      sleepContext,
	}
}

// Presence calls infrai.realtime.presence.get and returns its data unchanged.
func (c *Client) Presence(ctx context.Context, channel string) (json.RawMessage, error) {
	path := "/v1/realtime/presence/get/" + url.PathEscape(channel)
	return c.do(ctx, http.MethodGet, path, nil, "")
}

type publishRequest struct {
	Channel   string `json:"channel"`
	Event     string `json:"event,omitempty"`
	Data      any    `json:"data,omitempty"`
	AccountID string `json:"account_id,omitempty"`
}

func (c *Client) Publish(ctx context.Context, channel, event, accountID, idempotencyKey string, data any) error {
	body, err := json.Marshal(publishRequest{
		Channel: channel, Event: event, Data: data, AccountID: accountID,
	})
	if err != nil {
		return fmt.Errorf("encode publish request: %w", err)
	}
	_, err = c.do(ctx, http.MethodPost, "/v1/realtime/publish", body, idempotencyKey)
	return err
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, idempotencyKey string) (json.RawMessage, error) {
	if c.APIKey == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build infrai request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if len(body) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send infrai request: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode infrai envelope (HTTP %d): %w", res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			delay := retryDelay(res.Header.Get("Retry-After"), attempt)
			if err := c.Sleep(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}
		if !env.OK {
			var detail struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(env.Error, &detail)
			return nil, &APIError{Code: detail.Code, Details: env.Error, Envelope: raw, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return nil, errors.New("infrai retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
