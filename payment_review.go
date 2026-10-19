package presenceaudit

import (
	"context"
	"encoding/json"
	"fmt"
)

const ReviewThresholdCents int64 = 100_000

type PaymentEvent struct {
	PaymentID   string `json:"payment_id"`
	AccountID   string `json:"account_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type ReviewResult struct {
	PaymentID string          `json:"payment_id"`
	Decision  string          `json:"decision"`
	Presence  json.RawMessage `json:"online_users"`
}

type Realtime interface {
	Presence(context.Context, string) (json.RawMessage, error)
	Publish(context.Context, string, string, string, string, any) error
}

type Reviewer struct {
	Realtime Realtime
	Channel  string
}

func (r Reviewer) Review(ctx context.Context, payment PaymentEvent) (ReviewResult, error) {
	if payment.PaymentID == "" || payment.AccountID == "" || payment.AmountCents <= 0 || payment.Currency == "" {
		return ReviewResult{}, fmt.Errorf("payment_id, account_id, positive amount_cents, and currency are required")
	}
	presence, err := r.Realtime.Presence(ctx, r.Channel)
	if err != nil {
		return ReviewResult{}, err
	}

	decision := "approve"
	if payment.AmountCents >= ReviewThresholdCents {
		decision = "manual_review"
	}
	audit := struct {
		Payment  PaymentEvent    `json:"payment"`
		Decision string          `json:"decision"`
		Presence json.RawMessage `json:"online_users"`
	}{Payment: payment, Decision: decision, Presence: presence}
	if err := r.Realtime.Publish(ctx, r.Channel, "payment.reviewed", payment.AccountID, "payment-review:"+payment.PaymentID, audit); err != nil {
		return ReviewResult{}, err
	}
	return ReviewResult{PaymentID: payment.PaymentID, Decision: decision, Presence: presence}, nil
}
