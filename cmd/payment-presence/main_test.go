package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	presenceaudit "github.com/example/fintech-presence-audit"
)

type rejectingRealtime struct{}

func (rejectingRealtime) Presence(context.Context, string) (json.RawMessage, error) {
	return nil, &presenceaudit.APIError{
		Code:       "UPSTREAM_FAILURE",
		Envelope:   json.RawMessage(`{"ok":false,"error":{"code":"UPSTREAM_FAILURE"},"metadata":{"request_id":"req_123"}}`),
		HTTPStatus: http.StatusBadGateway,
	}
}

func (rejectingRealtime) Publish(context.Context, string, string, string, string, any) error {
	return errors.New("unexpected publish")
}

func TestReviewHandlerPreservesInfraiStatusAndEnvelope(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/payments/review", strings.NewReader(
		`{"payment_id":"pay_123","account_id":"acct_9","amount_cents":125000,"currency":"USD"}`,
	))
	response := httptest.NewRecorder()

	reviewHandler(presenceaudit.Reviewer{Realtime: rejectingRealtime{}, Channel: "fintech-ops"})(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadGateway)
	}
	const want = `{"ok":false,"error":{"code":"UPSTREAM_FAILURE"},"metadata":{"request_id":"req_123"}}` + "\n"
	if response.Body.String() != want {
		t.Fatalf("body = %s, want %s", response.Body.String(), want)
	}
}
