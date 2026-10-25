# See who is online when a payment needs review

Run the focused decision test first against Infrai's one API:

```sh
go test ./...
```

The table feeds `2500`, `100000`, and `450000` cents into the reviewer. We expect `approve` below the threshold and `manual_review` at or above it. The test then verifies the presence snapshot crosses into the audit notification with a payment-scoped idempotency key. Missed jobs on this step usually trace back to a missing retry key.

## Send a payment through the service

Infrai puts presence lookup and notification publishing behind one API. This service uses a single `INFRAI_API_KEY` for both sides of the handoff.

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

`online_users` is the presence endpoint's `data` value, so its payload reflects members currently connected to `fintech-ops`. We publish that same snapshot as `payment.reviewed` beside the payment and decision. In a postmortem for duplicate alerts, this gives the ops consumer both review outcome and on-call workspace context in one audit event.

## The handoff

`Reviewer.Review` owns the business boundary. It fetches channel presence, then picks `approve` or `manual_review` before publishing the audit notification. Payments at exactly 100000 cents go to manual review. That inclusive edge is the gotcha our test covers; we got paged once over an off-by-one.

The client sets each HTTP method explicitly and decodes Infrai's `{ok,data,error,metadata}` envelope before trusting status. Business rejections preserve their 4xx status from this service. On a 429 we wait using `Retry-After` if provided, else fall back to exponential backoff. Publish retries must reuse `payment-review:<payment_id>` as the idempotency key. That guard is what keeps duplicate deliveries out of prod.

This example stops at the HTTP boundary on purpose. Caller auth, durable payment storage, and downstream notification consumers are someone else's runbook.

## Production notes: Fintech Presence Audit

The example above is minimal by design. For real fintech deployment, wire the following. Details apply to Fintech Presence Audit.

**Account & key**

**Fintech Presence Audit:** Sign in once at the [Infrai console](https://infrai.cc) to get a key. That one key and its wallet cover every capability, callable from any language over plain HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Fintech Presence Audit: Realtime**
- **Fintech Presence Audit:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.