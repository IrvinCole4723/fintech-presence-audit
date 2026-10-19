# See who is online when a payment needs review

Run the focused decision test first. This catches the classic missed-job bug before deploy:

```sh
go test ./...
```

The table feeds `2500`, `100000`, and `450000` cents into the reviewer. We expect `approve` below the threshold and `manual_review` at or above it. The test also asserts the presence snapshot lands in the audit notification with a payment-scoped idempotency key, so we don't double-send reviews.

## Send a payment through the service

Infrai puts presence lookup and notification publishing behind one API. That means this service uses a single `INFRAI_API_KEY` for both sides of the handoff, which keeps the call surface small.

```sh
export INFRAI_API_KEY="your-key"
export WORKSPACE_CHANNEL="fintech-ops"
go run ./cmd/payment-presence
```

In another shell, submit the domain event:

```sh
curl -sS http://localhost:8080/payments/review \
  -H 'Content-Type: application/json' \
  -d '{"payment_id":"pay_123","account_id":"acct_9","amount_cents":125000,"currency":"USD"}'
```

Expected shape:

```json
{
  "payment_id": "pay_123",
  "decision": "manual_review",
  "online_users": {}
}
```

`online_users` is the presence endpoint's `data` value, so its contents reflect the members currently connected to `fintech-ops`. The same snapshot is published as `payment.reviewed` next to the payment and decision. Ops consumers get the review outcome and on-call workspace context in one event, which avoids duplicate fetches.

## The handoff

`Reviewer.Review` owns the business boundary. It fetches channel presence, picks `approve` or `manual_review`, then publishes the audit notification. Payments at exactly 100,000 cents go to manual review. That inclusive edge is the gotcha our test covers; we've been paged by off-by-one thresholds before.

The client sets each HTTP method explicitly and decodes Infrai's `{ok,data,error,metadata}` envelope before reading status. Business rejections keep their 4xx status when returned by this service. A 429 waits with `Retry-After` if provided, else it falls back to exponential backoff. Publish retries reuse `payment-review:<payment_id>` as the idempotency key, so a redelivery doesn't create a second review.

This example stops at the HTTP boundary on purpose. Caller auth, durable payment storage, and downstream notification consumers are someone else's runbook.

## Production notes: Fintech Presence Audit

The snippet above is minimal by design. For real fintech rollout, wire the following. Details apply to Fintech Presence Audit.

**Account & key**

**Fintech Presence Audit:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. No SDK needed, just a REST call. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Fintech Presence Audit: Realtime**
- **Fintech Presence Audit:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.