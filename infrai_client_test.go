package presenceaudit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientPreservesRejectedEnvelope(t *testing.T) {
	const response = `{"ok":false,"error":{"code":"UPSTREAM_FAILURE","message":"vendor unavailable"},"metadata":{"request_id":"req_123"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.BaseURL = server.URL
	_, err := client.Presence(context.Background(), "fintech-ops")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *APIError", err)
	}
	if apiErr.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("HTTP status = %d, want %d", apiErr.HTTPStatus, http.StatusBadGateway)
	}
	if string(apiErr.Envelope) != response {
		t.Fatalf("envelope = %s, want %s", apiErr.Envelope, response)
	}
}
