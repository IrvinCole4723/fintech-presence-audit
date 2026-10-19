package presenceaudit

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeRealtime struct {
	presence      json.RawMessage
	publishedData any
	idempotency   string
}

func (f *fakeRealtime) Presence(context.Context, string) (json.RawMessage, error) {
	return f.presence, nil
}

func (f *fakeRealtime) Publish(_ context.Context, _, _, _ string, idempotencyKey string, data any) error {
	f.idempotency = idempotencyKey
	f.publishedData = data
	return nil
}

func TestReviewerDecisionAndAuditHandoff(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		decision string
	}{
		{name: "routine payment", amount: 25_00, decision: "approve"},
		{name: "threshold payment", amount: ReviewThresholdCents, decision: "manual_review"},
		{name: "larger payment", amount: 450_000, decision: "manual_review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realtime := &fakeRealtime{presence: json.RawMessage(`{"members":["risk-analyst-7"]}`)}
			reviewer := Reviewer{Realtime: realtime, Channel: "fintech-ops"}
			result, err := reviewer.Review(context.Background(), PaymentEvent{
				PaymentID: "pay_123", AccountID: "acct_9", AmountCents: tt.amount, Currency: "USD",
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != tt.decision {
				t.Fatalf("decision = %q, want %q", result.Decision, tt.decision)
			}
			if realtime.idempotency != "payment-review:pay_123" {
				t.Fatalf("idempotency key = %q", realtime.idempotency)
			}
			if realtime.publishedData == nil {
				t.Fatal("expected an audit notification")
			}
		})
	}
}
