---
description: Increase unit test coverage for pkg/common package focusing on pure helper functions
---
# pkg/common Coverage Improvement Plan

## Current State
- **Coverage:** 48.8% (1,519 / 3,113 statements)
- **Target:** 70% coverage
- **Gap:** ~650 additional statements needed

## Strategy: Pure Functions First
Focus on files with 0% coverage that contain pure, testable functions. Avoid files that require HTTP mocks, complex state, or Lambda-specific behavior.

## Phase 1: Zero-Coverage Pure Functions (Priority)

### 1.1 slug.go (0% → 100%)
- **File:** `pkg/common/slug.go` 
- **Statements:** ~13 uncovered
- **Function:** `Slugify(value string) string`
- **Test Pattern:** Table-driven tests
- **Test File:** `pkg/common/slug_test.go`

```go
// Test cases to cover:
// - Empty string → ""
// - Whitespace only → ""
// - Simple lowercase → same
// - Uppercase → lowercase
// - Spaces → dashes
// - Special characters → removed/replaced
// - Unicode characters → handled
// - Multiple dashes → collapsed
// - Leading/trailing dashes → trimmed
```

### 1.2 headers.go (0% → 100%)
- **File:** `pkg/common/headers.go`
- **Statements:** ~18 uncovered
- **Functions:** `GetCORSHeaders()`, `GetAPIHeaders()`, `AddLinkHeader()`, `AddPaginationHeaders()`
- **Test Pattern:** Direct assertions on returned maps
- **Test File:** `pkg/common/headers_test.go`

```go
// Test cases:
// GetCORSHeaders - verify all expected headers present
// GetAPIHeaders - verify includes CORS + Content-Type
// AddLinkHeader - empty cursor does nothing
// AddLinkHeader - with cursor builds correct URL
// AddLinkHeader - with params encodes correctly
// AddPaginationHeaders - adds X-Total-Count when > 0
```

### 1.3 json.go (0% → 100%)
- **File:** `pkg/common/json.go`
- **Statements:** ~10 uncovered
- **Functions:** `Marshal`, `Unmarshal`, `MarshalString`, `NewEncoder`, `NewDecoder`
- **Test Pattern:** Simple serialization tests
- **Test File:** `pkg/common/json_test.go`

```go
// Test cases:
// Marshal - struct to JSON bytes
// Unmarshal - JSON bytes to struct
// MarshalString - struct to JSON string
// MarshalString - error case
// NewEncoder - returns encoder with EscapeHTML disabled
// NewDecoder - returns working decoder
```

### 1.4 form.go (0% → 100%)
- **File:** `pkg/common/form.go`
- **Statements:** ~29 uncovered
- **Functions:** `ParseMultipartForm`, `ParseFormURLEncoded`
- **Test Pattern:** String input → parsed map output
- **Test File:** `pkg/common/form_test.go`

```go
// Test cases:
// ParseFormURLEncoded - simple key=value
// ParseFormURLEncoded - multiple keys
// ParseFormURLEncoded - URL encoding
// ParseFormURLEncoded - empty string
// ParseMultipartForm - valid multipart
// ParseMultipartForm - missing boundary error
// ParseMultipartForm - invalid content type
```

## Phase 2: Low-Coverage Pure Functions

### 2.1 sanitize.go (30% → 80%)
- **File:** `pkg/common/sanitize.go`
- **Current:** 30.4% (14/46)
- **Functions:** `EscapeHTML`, `SanitizeContent`, `ValidateAndSanitizeMediaType`, `SanitizeHTML`, `SanitizeActivityPubObject`
- **Test File:** `pkg/common/sanitize_test.go`

```go
// Pure function tests:
// EscapeHTML - escapes &, <, >, ", '
// ValidateAndSanitizeMediaType - allowed types pass
// ValidateAndSanitizeMediaType - blocked types fail
// ValidateAndSanitizeMediaType - path traversal stripped
// SanitizeContent - delegates to default sanitizer
// SanitizeHTML - removes script tags
// SanitizeActivityPubObject - sanitizes content/summary/name fields
```

### 2.2 resources.go (8% → 60%)
- **File:** `pkg/common/resources.go`
- **Current:** 8.3% (4/48)
- **Focus:** Resource monitor methods that don't depend on actual memory state
- **Test File:** `pkg/common/resources_test.go`

```go
// Test cases:
// NewLambdaResourceMonitor - creates with defaults
// GetCheckpoints - returns copy of checkpoints
// GetElapsedTime - returns positive duration
// GetResourceUtilization - returns percentages
// WrapWithResourceCheck - calls fn and returns result
```

### 2.3 runtime_env.go (11.5% → 70%)
- **File:** `pkg/common/runtime_env.go`
- **Current:** 11.5% (3/26)
- **Test Pattern:** Environment variable helpers with table tests
- **Test File:** `pkg/common/runtime_env_test.go`

## Phase 3: Moderate Coverage Improvement

### 3.1 auth_helpers.go (20.6% → 50%)
- **File:** `pkg/common/auth_helpers.go`
- **Current:** 20.6% (21/102)
- **Focus ONLY on:** `HasAnyScope`, `HasAllScopes`, `ValidateReadScopes`, `ValidateWriteScopes`
- **AVOID:** Functions requiring lift.Context or OAuthService mocks
- **Test File:** `pkg/common/auth_helpers_pure_test.go`

```go
// Pure function tests using mock Claims:
// HasAnyScope - returns true when one scope matches
// HasAnyScope - returns false when none match
// HasAnyScope - handles nil claims
// HasAllScopes - returns true when all match
// HasAllScopes - returns false when some missing
```

### 3.2 responses.go (38.6% → 60%)
- **File:** `pkg/common/responses.go`
- **Current:** 38.6% (34/88)
- **Focus on:** Response building helpers that don't require lift.Context
- **Test File:** `pkg/common/responses_pure_test.go`

## Execution Instructions

// turbo-all
### Step 1: Create and run Phase 1 tests
```bash
# Create slug_test.go with table-driven tests
# Create headers_test.go with direct assertions
# Create json_test.go with serialization tests
# Create form_test.go with parsing tests
```

// turbo
### Step 2: Verify Phase 1 coverage
```bash
cd /home/aron/ai-workspace/codebases/penny-advanced-interfaces/lesser
go test -v -cover ./pkg/common/... -run "Test.*Slug|Test.*Header|Test.*JSON|Test.*Form" 2>&1 | tail -20
```

// turbo
### Step 3: Check overall pkg/common coverage
```bash
cd /home/aron/ai-workspace/codebases/penny-advanced-interfaces/lesser
./lesser test coverage --scope pkg 2>&1 | grep "pkg/common"
```

### Step 4: Iterate on Phase 2 if time permits

## Files to Create

1. `pkg/common/slug_test.go` - Slugify tests
2. `pkg/common/headers_test.go` - Header helper tests  
3. `pkg/common/json_test.go` - JSON wrapper tests
4. `pkg/common/form_test.go` - Form parsing tests
5. `pkg/common/sanitize_test.go` - Sanitization tests (extend existing if present)
6. `pkg/common/auth_helpers_pure_test.go` - Pure auth helper tests

## Constraints

1. All tests must pass `./lesser test unit`
2. No AWS calls, no httptest.NewServer
3. Use table-driven tests with `stretchr/testify`
4. Pure functions only - avoid lift.Context dependencies
5. Each test file should be self-contained

## Success Criteria

- Phase 1 complete: Coverage reaches 55%+
- Phase 2 complete: Coverage reaches 65%+
- All tests pass in CI
- No lint errors

## Iteration Loop

After each phase:
1. Run `./lesser test unit` to verify tests pass
2. Run `./lesser coverage scoreboard --profile coverage.out --mode file --package github.com/equaltoai/lesser/pkg/common`
3. Identify remaining low-coverage files
4. Continue with next priority

## DO NOT ATTEMPT

- `lambda_helpers.go` - Lambda-specific, requires AWS context
- `lambda_init.go` - Lambda initialization, global state
- `error_middleware.go` - Requires lift.Context mocking
- `redirect.go` - HTTP redirect logic with lift.Context
- `security_logger.go` - Logging with zap dependencies
- `cookies.go` - Requires http.ResponseWriter mocking
- Functions in `error_responses.go` that take `*lift.Context`
