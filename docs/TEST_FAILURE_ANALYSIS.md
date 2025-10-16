# Test Failure Analysis - October 16, 2025

## Executive Summary

The `make test` command reveals **two categories of failures**:

1. **Critical initialization panic (affects 51 packages)**: A nil pointer dereference in error handling during package initialization
2. **3 actual test failures**: Specific test cases failing in production code

### Overall Status
- **Total Packages Tested**: ~120
- **Packages Passing**: 51
- **Packages with Init Panic**: 51
- **Packages with Test Failures**: 3
- **Severity**: HIGH (blocks all testing for half the codebase)

---

## Category 1: Critical - Initialization Panic (51 packages affected)

### Root Cause
**Location**: `pkg/errors/common.go:86` calling `WithMetadata()` on a nil `*AppError`

```go
// File: pkg/errors/common.go:86
func ProcessingFailed(processType string, err error) *AppError {
	return NewLambdaInternalError(CodeEventProcessingFailed, "Processing failed", err).
		WithMetadata("process_type", processType).AsRetryable()
}
```

**Problem**: `NewLambdaInternalError()` is returning `nil` instead of a valid `*AppError` during package initialization, then `.WithMetadata()` is called on nil, causing:

```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x50 pc=0x...]
github.com/equaltoai/lesser/pkg/errors.(*AppError).WithMetadata(...)
	/home/aron/ai-workspace/codebases/lesser/pkg/errors/context.go:58
```

### Impact
This panic occurs during `init()` functions in `pkg/common/errors.go:54-55`, which defines package-level error variables:

```go
// File: pkg/common/errors.go:55
ErrOperationExecutionFailed = errors.ProcessingFailed("operation", stdErrors.New("operation execution failed"))
```

Because this happens during package initialization (before any tests run), it causes **immediate panic** for all packages that import `pkg/common`.

### Affected Packages (51 total)

#### Command Packages (12):
- `cmd/activity-processor`
- `cmd/actor`
- `cmd/api`
- `cmd/api/lift`
- `cmd/federation-delivery`
- `cmd/inbox`
- `cmd/media-processor`
- `cmd/moderation-processor`
- `cmd/notification-processor`
- `cmd/objects`
- `cmd/outbox`
- `graph` (GraphQL resolver)

#### Core Service Packages (10):
- `pkg/activitypub`
- `pkg/auth`
- `pkg/common`
- `pkg/cost`
- `pkg/dlq`
- `pkg/federation`
- `pkg/httpclient`
- `pkg/jsonld`
- `pkg/services`
- `pkg/streaming`

#### Storage Packages (10):
- `pkg/storage/dynamorm`
- `pkg/storage/dynamorm/batch`
- `pkg/storage/dynamorm/hooks`
- `pkg/storage/dynamorm/marshalers`
- `pkg/storage/dynamorm/migrations`
- `pkg/storage/dynamorm/patterns`
- `pkg/storage/dynamorm/repositories`
- `pkg/storage/dynamorm/repositories/testing`
- `pkg/storage/dynamorm/stream`
- `pkg/storage/dynamorm/validation`
- `pkg/storage/dynamorm_test`
- `pkg/storage/models`
- `pkg/storage/repositories`

#### Service Layer Packages (9):
- `pkg/services/accounts`
- `pkg/services/conversations`
- `pkg/services/importexport`
- `pkg/services/lists`
- `pkg/services/media`
- `pkg/services/notes`
- `pkg/services/notifications`
- `pkg/services/relationships`

#### Other Packages (10):
- `pkg/federation/circuit`
- `pkg/federation/cost`
- `pkg/federation/routing`
- `pkg/lift`
- `pkg/lift/testing`
- `pkg/mastodon`
- `pkg/mastodon/transformers`
- `pkg/media`
- `pkg/media/streaming`
- `pkg/middleware`
- `pkg/moderation`
- `pkg/moderation/advanced`
- `pkg/observability`
- `pkg/privacy`
- `pkg/ratelimit`
- `pkg/reputation`
- `pkg/streaming/handlers`
- `pkg/transformations`

### Resolution Steps

#### Step 1: Fix `NewLambdaInternalError` Function
The function must ensure it never returns `nil`:

```go
// File: pkg/errors/constructors.go or similar
func NewLambdaInternalError(code, message string, err error) *AppError {
	appErr := &AppError{
		Code:          code,
		Message:       message,
		Category:      CategoryInternal,
		InternalError: err,
		Metadata:      make(map[string]interface{}),
		Timestamp:     time.Now(),
	}
	if err != nil {
		appErr.InternalMessage = err.Error()
	}
	return appErr
}
```

#### Step 2: Add Defensive Nil Check in `WithMetadata`
Even though we fix the root cause, add defensive programming:

```go
// File: pkg/errors/context.go:57-63
func (e *AppError) WithMetadata(key string, value interface{}) *AppError {
	if e == nil {
		// Log error but don't panic - return a new error
		return NewAppError(CodeInternal, CategoryInternal, "attempted to add metadata to nil error")
	}
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}
```

#### Step 3: Review All Error Constructor Functions
Audit all error constructor functions in `pkg/errors/` to ensure none can return `nil`:
- `NewAppError()`
- `NewLambdaInternalError()`
- `NewLambdaValidationError()`
- All factory functions like `ProcessingFailed()`, `ParsingFailed()`, etc.

#### Step 4: Add Test Coverage
Create test file `pkg/errors/constructors_test.go`:

```go
package errors_test

import (
	"errors"
	"testing"
	
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestAllConstructorsReturnNonNil(t *testing.T) {
	tests := []struct {
		name string
		err  *errors.AppError
	}{
		{"ProcessingFailed", errors.ProcessingFailed("test", errors.New("test"))},
		{"ParsingFailed", errors.ParsingFailed("test", errors.New("test"))},
		{"MarshalingFailed", errors.MarshalingFailed("test", errors.New("test"))},
		// Add all other constructor functions
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.err, "constructor must not return nil")
			assert.NotNil(t, tt.err.Metadata, "Metadata map must be initialized")
		})
	}
}
```

---

## Category 2: Actual Test Failures

### Failure 1: `pkg/storage/dynamorm/patterns` - Soft Delete Tests

**Package**: `github.com/equaltoai/lesser/pkg/storage/dynamorm/patterns`  
**Tests Failing**: 2
- `TestSoftDeleteRepository_HardDelete`
- `TestSoftDeleteRepository_Get`

#### Issue 1.1: `TestSoftDeleteRepository_HardDelete`
**Error**: Mock expectations not met

```
soft_delete_test.go:234: FAIL: 0 out of 2 expectation(s) were met.
    The code you are testing needs to make 2 more call(s).
```

**Root Cause**: The test expects the mock to call `WithContext()` and `Delete()`, but the actual implementation doesn't make these calls in the expected way.

**Location**: `pkg/storage/dynamorm/patterns/soft_delete_test.go:228-234`

**Fix**: Update mock expectations to match actual implementation behavior:

```go
// Current (failing):
db.On("WithContext", mock.Anything).Return(db)
db.On("Delete", mock.Anything).Return(nil)

// Should be:
db.On("Model", mock.Anything).Return(mockQuery)
mockQuery.On("Where", mock.Anything, mock.Anything).Return(mockQuery)
mockQuery.On("Delete").Return(nil)
```

#### Issue 1.2: `TestSoftDeleteRepository_Get`
**Error**: Type assertion panic

```
panic: interface conversion: *patterns.MockQuery is not core.Query: missing method AllPaginated
```

**Root Cause**: `MockQuery` struct doesn't implement all methods of `core.Query` interface, specifically missing `AllPaginated()`.

**Location**: `pkg/storage/dynamorm/patterns/soft_delete_test.go:26`

**Fix**: Add missing method to `MockQuery`:

```go
type MockQuery struct {
	mock.Mock
}

// Add this method:
func (m *MockQuery) AllPaginated(dest interface{}, opts ...interface{}) error {
	args := m.Called(dest, opts)
	return args.Error(0)
}
```

### Failure 2: `pkg/federation` - HTTP Signature Tests

**Package**: `github.com/equaltoai/lesser/pkg/federation`  
**Test**: `TestParseSignatureHeader/minimal_signature_header`

**Error**:
```
httpsig_test.go:79: Received unexpected error:
    Invalid HTTP signature
Error: validation failed for signature: missing required parameter: algorithm
```

**Root Cause**: The test case "minimal_signature_header" expects a signature with only `keyId` and `signature` to be valid, but the validator requires `algorithm` parameter.

**Location**: `pkg/federation/httpsig_test.go:70-79`

**Current Test Case**:
```go
{
	name:   "minimal_signature_header",
	header: `keyId="test-key",signature="dGVzdA=="`,
	want: &HTTPSignature{
		KeyID:     "test-key",
		Signature: []byte("test"),
	},
	wantErr: false,  // Expects success
},
```

**Fix Options**:

**Option A**: Make algorithm optional in parser (if spec allows):
```go
// File: pkg/federation/httpsig.go
func validateSignature(sig *HTTPSignature) error {
	if sig.KeyID == "" {
		return fmt.Errorf("missing required parameter: keyId")
	}
	if len(sig.Signature) == 0 {
		return fmt.Errorf("missing required parameter: signature")
	}
	// Make algorithm optional or default to "rsa-sha256"
	if sig.Algorithm == "" {
		sig.Algorithm = "rsa-sha256"
	}
	return nil
}
```

**Option B**: Update test expectation to require algorithm:
```go
{
	name:   "minimal_signature_header",
	header: `keyId="test-key",algorithm="rsa-sha256",signature="dGVzdA=="`,
	want: &HTTPSignature{
		KeyID:     "test-key",
		Algorithm: "rsa-sha256",
		Signature: []byte("test"),
	},
	wantErr: false,
},
```

**Recommendation**: Choose Option B (update test) - HTTP Signatures spec typically requires algorithm for security reasons.

### Failure 3: `pkg/lift` - Test Application Tests

**Package**: `github.com/equaltoai/lesser/pkg/lift`  
**Tests Failing**: Unknown count (truncated output)

**Evidence**: Package shows as failed but specific test output was truncated in logs.

**Investigation Required**: Run targeted test:
```bash
cd /home/aron/ai-workspace/codebases/lesser
go test -v ./pkg/lift/...
```

---

## Recommendations

### Immediate Actions (Priority 1)

1. **Fix the initialization panic** in `pkg/errors/`
   - Ensure all error constructors return non-nil values
   - Add defensive nil checks in chaining methods
   - **Impact**: This alone will fix 51 package failures

2. **Fix soft delete mock tests** in `pkg/storage/dynamorm/patterns/`
   - Implement missing `AllPaginated()` in mock
   - Correct mock expectations for `HardDelete` test
   - **Impact**: 2 test failures resolved

3. **Fix HTTP signature test** in `pkg/federation/`
   - Update test case to include required `algorithm` parameter
   - **Impact**: 1 test failure resolved

### Medium Priority Actions

4. **Investigate `pkg/lift` failures**
   - Run isolated test to see actual failures
   - Fix based on investigation

5. **Add comprehensive error package tests**
   - Test all constructor functions return non-nil
   - Test all chaining methods handle nil gracefully
   - Add integration tests for package initialization

### Long-term Improvements

6. **CI/CD Integration**
   - Ensure tests run on every PR
   - Block merges on test failures
   - Add test coverage reporting

7. **Mock Infrastructure**
   - Create complete mock implementations of all interfaces
   - Use code generation for mocks (e.g., mockery)
   - Standardize mock patterns across test suite

8. **Error Handling Review**
   - Audit all packages for nil pointer vulnerabilities
   - Establish error handling best practices
   - Document error creation patterns

---

## Testing Strategy

### Phase 1: Quick Wins (Est. 1-2 hours)
1. Fix error constructors
2. Run `make test` to verify 51 packages now pass
3. Fix HTTP signature test
4. Fix soft delete mocks

### Phase 2: Full Resolution (Est. 2-3 hours)
1. Investigate and fix `pkg/lift` failures
2. Run full test suite
3. Verify all packages pass

### Phase 3: Prevention (Est. 4-6 hours)
1. Add test coverage for error package
2. Set up CI/CD test automation
3. Document testing patterns
4. Create developer guidelines

---

## Success Metrics

- [ ] All 51 initialization panics resolved
- [ ] `pkg/storage/dynamorm/patterns` tests passing
- [ ] `pkg/federation` HTTP signature test passing
- [ ] `pkg/lift` tests identified and fixed
- [ ] `make test` exits with code 0
- [ ] Test coverage > 70% for error handling code
- [ ] CI/CD pipeline running tests automatically

---

## Appendix: Complete List of Affected Packages

### Category A: Init Panic (51 packages)
```
cmd/activity-processor
cmd/actor
cmd/api
cmd/api/lift
cmd/federation-delivery
cmd/inbox
cmd/media-processor
cmd/moderation-processor
cmd/notification-processor
cmd/objects
cmd/outbox
graph
pkg/activitypub
pkg/auth
pkg/common
pkg/cost
pkg/dlq
pkg/federation
pkg/federation/circuit
pkg/federation/cost
pkg/federation/routing
pkg/httpclient
pkg/jsonld
pkg/lift
pkg/lift/testing
pkg/mastodon
pkg/mastodon/transformers
pkg/media
pkg/media/streaming
pkg/middleware
pkg/moderation
pkg/moderation/advanced
pkg/observability
pkg/privacy
pkg/ratelimit
pkg/reputation
pkg/services
pkg/services/accounts
pkg/services/conversations
pkg/services/importexport
pkg/services/lists
pkg/services/media
pkg/services/notes
pkg/services/notifications
pkg/services/relationships
pkg/storage/dynamorm
pkg/storage/dynamorm/batch
pkg/storage/dynamorm/hooks
pkg/storage/dynamorm/marshalers
pkg/storage/dynamorm/migrations
pkg/storage/dynamorm/patterns
pkg/storage/dynamorm/repositories
pkg/storage/dynamorm/repositories/testing
pkg/storage/dynamorm/stream
pkg/storage/dynamorm/validation
pkg/storage/dynamorm_test
pkg/storage/models
pkg/storage/repositories
pkg/streaming
pkg/streaming/handlers
pkg/transformations
```

### Category B: Real Test Failures (3 packages)
```
pkg/storage/dynamorm/patterns (2 test failures)
pkg/federation (1 test failure)
pkg/lift (unknown count - needs investigation)
```

### Category C: Passing (51 packages)
All other packages including:
- `pkg/config`
- `pkg/services/threads`
- `pkg/storage/dynamorm/*` (when not init panic)
- And 48 others

---

**Document Version**: 1.0  
**Generated**: October 16, 2025  
**Author**: AI Analysis  
**Status**: Ready for Review

