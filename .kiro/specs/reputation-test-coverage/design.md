# Design Document: Reputation Package Test Coverage

## Overview

This design document outlines the approach for improving unit test coverage in the `pkg/reputation` package from 49.1% to approximately 90%. The strategy focuses on testing the three main untested components: Calculator, VouchManager, and Verifier crypto functions.

## Architecture

The reputation package follows a layered architecture:

```
┌─────────────────────────────────────────────────────────────┐
│                      Service Layer                          │
│  (Service - orchestrates all operations)                    │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌───────────────┐    ┌───────────────┐    ┌───────────────┐
│  Calculator   │    │ VouchManager  │    │    Crypto     │
│ (score calc)  │    │ (vouch ops)   │    │ (sign/verify) │
└───────────────┘    └───────────────┘    └───────────────┘
        │                     │                     │
        └─────────────────────┼─────────────────────┘
                              ▼
                    ┌───────────────┐
                    │   Storage     │
                    │ (interfaces)  │
                    └───────────────┘
```

## Components and Interfaces

### Calculator Component

The Calculator computes reputation scores using four sub-scores:
- **TrustScore** (0-250): Based on trust graph relationships, weighted by recency
- **ActivityScore** (0-250): Based on account age, post frequency, followers, recency
- **ModerationScore** (0-250): Starts at 250, deducted for violations
- **CommunityScore** (0-250): Based on vouches and community contributions

Testing approach: Use table-driven tests with mock storage to test each calculation function independently.

### VouchManager Component

The VouchManager handles vouch lifecycle:
- Creation with validation (confidence, monthly limit, reputation threshold)
- Revocation (only by voucher)
- Import with signature verification
- Query operations

Testing approach: Use mock storage and signer to test all paths including error conditions.

### Crypto Component (Verifier)

The Verifier handles:
- Portable reputation verification
- Instance public key fetching and caching
- Domain trust checking (block lists, allow lists)
- Vouch signature verification

Testing approach: Use HTTP test server for key fetching, mock storage for domain checks.

## Data Models

The existing data models are well-defined in `types.go`. Tests will use these models directly with test fixtures.

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Score Bounds Invariant

*For any* valid CalculationInput, all individual scores (TrustScore, ActivityScore, ModerationScore, CommunityScore) returned by the Calculator SHALL be within the range [0, 250].

**Validates: Requirements 1.4, 1.5, 1.9**

### Property 2: Total Score Composition

*For any* Reputation calculated by the Calculator, the TotalScore SHALL equal the sum of TrustScore + ActivityScore + ModerationScore + CommunityScore.

**Validates: Requirements 1.2**

### Property 3: Vouch Boost Cap

*For any* list of vouches passed to GetBoostFromVouches, the returned boost SHALL never exceed 200.

**Validates: Requirements 1.10**

### Property 4: Confidence Validation

*For any* CreateVouchInput with confidence outside the range [0, 1], CreateVouch SHALL return an error.

**Validates: Requirements 2.2**

### Property 5: Reputation Threshold

*For any* CreateVouchInput where the voucher's reputation is below 500, CreateVouch SHALL return an error.

**Validates: Requirements 2.4**

### Property 6: Moderation Score Monotonicity

*For any* CalculationInput, adding an upheld report SHALL result in a lower or equal ModerationScore compared to the same input without that report.

**Validates: Requirements 1.7**

## Error Handling

Tests will verify error handling for:
- Nil/invalid inputs
- Storage failures
- Signature verification failures
- HTTP failures for key fetching
- Domain block/allow list edge cases

## Testing Strategy

### Unit Tests

Unit tests will cover:
- Constructor functions (NewCalculator, NewVouchManager, NewService)
- Individual calculation functions with various inputs
- Error paths and edge cases
- Mock-based isolation of dependencies

### Property-Based Tests

Property-based tests using `testing/quick` or table-driven exhaustive tests will verify:
- Score bounds invariants
- Total score composition
- Vouch boost caps
- Input validation boundaries

### Test File Organization

```
pkg/reputation/
├── calculator_test.go      # NEW: Calculator tests
├── vouch_test.go           # NEW: VouchManager tests  
├── crypto_test.go          # EXTEND: Add Verifier tests
├── service_test.go         # EXTEND: Add NewService tests
└── service_round20_coverage_test.go  # Existing tests
```

### Mock Strategy

Create focused mock implementations for:
- `core.RepositoryStorage` - for storage operations
- `http.RoundTripper` - for HTTP key fetching
- Existing mock patterns from `service_round20_coverage_test.go`

### Coverage Targets by File

| File | Current | Target |
|------|---------|--------|
| calculator.go | 0% | 90%+ |
| vouch.go | 0% | 90%+ |
| crypto.go | ~40% | 85%+ |
| service.go | ~70% | 90%+ |
| **Total** | **49.1%** | **~90%** |
