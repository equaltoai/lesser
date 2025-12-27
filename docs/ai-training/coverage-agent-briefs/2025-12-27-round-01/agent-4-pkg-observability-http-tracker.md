# Agent 4 Brief — `pkg/observability` HTTP tracking (no servers)

## Goal

Increase `pkg/` unit test coverage by testing HTTP tracking + helper logic without starting any servers. Focus on deterministic unit tests for error categorization, federation endpoint detection, and request tracking dimension building.

Primary target: `pkg/observability/http_tracker.go`

## Constraints (must follow)

- Run tests via the CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- Unit tests must be AWS/network independent.
- Do not use `httptest.NewServer` (environment cannot bind/listen to ports).
  - Use a custom `http.Client` with a stub `RoundTripper` instead.
- Prefer table-driven tests + `stretchr/testify`.
- Avoid `time.Sleep` where possible; use channels if you must synchronize.

## Setup patterns you should use

### RoundTripper stub (no network)

Create a transport like:

```go
type rtFunc func(*http.Request) (*http.Response, error)
func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
```

Then:

```go
client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
  return &http.Response{
    StatusCode: 200,
    Body:       io.NopCloser(strings.NewReader("ok")),
    Header:     make(http.Header),
    Request:    req,
  }, nil
})}
```

### MetricsRecorder stub

Implement a tiny stub for the `MetricsRecorder` interface that records inputs and optionally signals a channel.

Note: `HTTPTracker.Do` records metrics in a goroutine; if you assert calls, synchronize via a channel (don’t sleep).

## What to cover (must)

### 1) Error categorization

Test:

- `categorizeHTTPError(nil) == ""`
- Timeout classification:
  - `context.DeadlineExceeded` and an `error` containing “timeout” → `ErrorTypeTimeout`
- Context cancellation classification:
  - `context.Canceled` → `"context_canceled"`
- DNS/connection/TLS classification:
  - errors containing “no such host” → `"dns_error"`
  - errors containing “connection refused” → `"connection_error"`
  - errors containing “x509” / “tls” → `"tls_error"`

### 2) Status-code categorization

Test `categorizeHTTPStatusError` for:

- 401/403/404/408/409/429 mappings
- 4xx default → `ErrorTypeValidation`
- 5xx → `"server_error"`
- non-error status → `""`

### 3) Federation endpoint detection + type

Test:

- `isFederationRequest(nil) == false`
- Paths that should match: `/inbox`, `/.well-known/webfinger`, `/users/alice`, `/objects/xyz`, etc.
- `getFederationType` returns expected bucket (`inbox`, `discovery`, `actor`, `activity`, `object`, `other`)

### 4) URL-based tracker

Test `HTTPLatencyTracker.TrackRequest`:

- When URL is invalid → host dimension is `"unknown"` (and operation is `http_request`).
- When URL is a federation URL → operation is `federation_request` and adds `federation_type`.
- Verify `RecordLatency` is called with:
  - operation
  - host
  - duration
  - success computed from `statusCode` (2xx/3xx = true)

### 5) `HTTPTracker.Do` basic behavior (no goroutine assertions required)

Using a stub transport:

- Ensure returned metrics reflect method/url/status/success
- Ensure error type classification is set for:
  - non-nil error
  - 4xx/5xx response

To avoid flakiness, you can set `metricsRecorder=nil` so the goroutine becomes a no-op.

## Deliverables

- New test file(s) in `pkg/observability/`:
  - Suggested: `http_tracker_test.go`
- All tests pass:
  - `./lesser test unit`
  - `./lesser lint`
- Coverage improved:
  - `./lesser test coverage --scope pkg`
  - `./lesser coverage scoreboard --profile coverage_pkg.out --top 25`

