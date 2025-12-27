# Agent 2 Brief — `pkg/dlq` error classification

## Goal

Increase `pkg/` unit test coverage by adding deterministic tests around DLQ error classification logic (pattern matching, JSON/text extraction, and AppError mapping).

Primary target: `pkg/dlq/error_classifier.go`

## Constraints (must follow)

- Run tests via the CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- Unit tests must be AWS/network independent.
- Don’t use `httptest.NewServer` (port binding is not available).
- Prefer table-driven tests + `stretchr/testify`.
- Don’t change production logic unless a testability issue blocks you.

## What to cover (must)

### 1) Pattern initialization

Test `NewErrorClassifier()`:

- returns non-nil classifier
- `GetPatterns()` contains key defaults (at least a couple: `validation_error`, `network_error`, `timeout_error`)

### 2) JSON extraction paths

Test `ClassifyError()` via JSON message bodies to exercise:

- `errorMessage`
- `error`
- `message`
- `stackTrace` as string
- `stackTrace` as `[]interface{}` (joined with `\n`)
- `errorType` mapped to `ErrorInfo.Category`

Example JSON (adjust as needed):

```json
{"errorMessage":"validation failed: missing field","errorType":"TypeError","stackTrace":["line1","line2"]}
```

Assert classification ends up as `validation_error` and stack trace is joined.

### 3) Plain-text extraction paths (stack parsing)

Exercise `extractFromText` behavior via `ClassifyError()`:

- A message containing stack-trace-like lines (`"at ..."`, `".go:"`) produces:
  - `ErrorInfo.ErrorMessage` containing only the “error lines” (no stack)
  - `ErrorInfo.StackTrace` containing only stack lines joined with `\n`
- A message without stack patterns leaves `StackTrace` empty.

### 4) Pattern scoring / best match

Test `classifyByPatterns` behavior indirectly via `ClassifyError()`:

- Provide an error message that matches multiple patterns and assert the “best” match is selected (the implementation scores by matched substring length).
- Provide an unknown error message and assert the default type is `"processing_error"`.

### 5) Service-specific overrides

Test that the service-specific classifiers set fields as intended:

- `processorNotification`:
  - “user not found” → `ErrorType=user_not_found`, permanent, low, reason string set
- `processorActivity`:
  - “signature verification” → `signature_verification_error`, permanent, high
- `processorMedia`:
  - “format unsupported” → `unsupported_media_format`, permanent, low
- `processorFederationDelivery` and `processorSearchIndexer` (at least one path each)

Only assert the fields the classifier sets (type/permanent/priority/reason) to keep tests stable.

### 6) Trend analysis

Test `AnalyzeErrorTrends(messages)`:

- total count, per-type counts
- permanent vs transient totals
- percent rates are computed when `TotalMessages > 0`

### 7) AppError creation and mapping

Test `CreateAppError(messageBody, service)`:

- `ErrorCode` + `ErrorCategory` match the mapping for a known error type
- retryable behavior:
  - transient errors → `Retryable=true`
  - permanent errors → `Retryable=false`
- metadata keys set: `service`, `error_type`, `priority`, `category`
- `InternalMessage` is set from the extracted error message

## Deliverables

- New test file(s) in `pkg/dlq/`:
  - Suggested: `error_classifier_test.go`
- All tests pass:
  - `./lesser test unit`
  - `./lesser lint`
- Coverage improved:
  - `./lesser test coverage --scope pkg`
  - `./lesser coverage scoreboard --profile coverage_pkg.out --top 25`

