# Agent 4 Brief — `pkg/observability` webhook delivery (no-network tests)

## Goal

Raise `pkg/observability` coverage by adding unit tests for:

- `pkg/observability/webhook_delivery.go`

Focus on deterministic delivery behavior using a custom `http.RoundTripper` stub (no servers, no network).

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Use `http.RoundTripper` stubs to simulate responses and errors.
- Prefer `stretchr/testify` assertions.

## What to cover

### 1) URL validation

Cover `ValidateWebhookURL`:

- empty URL
- invalid URL parse
- invalid scheme (not http/https)
- missing host
- valid `http://` and `https://`

### 2) Signature generation

Cover `generateHMACSignature`:

- deterministic output for a known payload + secret
- prefix `sha256=` is included

### 3) Payload generation

Cover `prepareWebhookPayload`:

- includes required fields (`alert_id`, `type`, `severity`, `fired_at`, etc.)
- includes `resolved_at` only when `ResolvedAt != nil`

### 4) HTTP delivery behavior (`deliverWebhook`)

Test using a RoundTripper stub:

- **Success (2xx)**:
  - returns nil error
  - `delivery.Status == "success"`
  - `delivery.ResponseCode` and `delivery.ResponseBody` captured
  - `delivery.RequestBody` is set (non-empty JSON)
  - when `SecretToken` is set, request includes `X-Webhook-Signature` and `X-Webhook-Signature-256`
- **HTTP error (non-2xx)**:
  - returns error
  - `delivery.Status` becomes `"retrying"` when attempts remain
  - `delivery.ErrorType` matches `categorizeHTTPError`
  - response body captured
- **Transport error**:
  - RoundTripper returns an error string containing `timeout`, `no such host`, or `tls` and assert `categorizeError` output is used
- **Response read failure**:
  - RoundTripper returns a response body that errors on `Read` and ensure delivery is marked failed

Implementation tip:

- Construct service directly to inject HTTP client:
  - `w := &WebhookDeliveryService{logger: zaptest.NewLogger(t), httpClient: &http.Client{Transport: rt}}`
- Keep `webhookRepo/alertRepo/deadLetterRepo` nil for these tests (they aren’t used by `deliverWebhook`).

### 5) HTTP error categorization

Cover `categorizeHTTPError`:

- 408 → `timeout`
- 429 → `rate_limit`
- other 4xx → `client_error`
- 5xx → `server_error`

## Deliverables

- New tests in `pkg/observability/`, suggested filename:
  - `webhook_delivery_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

