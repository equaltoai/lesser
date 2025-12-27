# Agent 2 Brief — `pkg/storage/repositories` EnhancedPatternRepository (analysis + matching helpers)

## Goal

Raise `pkg/storage/repositories` coverage by adding unit tests for the **deterministic pattern-analysis logic** in:

- `pkg/storage/repositories/enhanced_pattern_repository.go`

This round should prioritize logic that doesn’t require DynamoDB calls (pure matching + scoring).

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer table-driven tests + `stretchr/testify`.
- Avoid `time.Sleep`.
- Keep tests deterministic (assert ranges, not exact timestamps).

## Setup guidance

Create the repository via constructor so `r.logger` is non-nil:

- `NewEnhancedPatternRepository(mockDB, "test-table", zap.NewNop(), nil)`

You can pass `mockDB := new(mocks.MockDB)` even when you don’t set expectations (as long as the tested method doesn’t touch DB).

Avoid DB-heavy methods in this round:

- `GetPattern`, `UpdatePattern*`, `RecordMatch`, caches/metrics CRUD, cleanup, etc.

## What to cover

### 1) Low-level matching helpers

File: `pkg/storage/repositories/enhanced_pattern_repository.go`

Cover these:

- `matchDomainPattern`
  - exact domain match
  - subdomain match (`foo.example.com` matches `example.com`)
  - negative cases
- `matchTextPattern`
  - exact match
  - prefix match
  - negative cases
- `matchRegexPattern`
  - currently delegates to `matchTextPattern` (assert parity)
- `getSeverityWeight`
  - `critical/high/medium/low` and default

### 2) `analyzePatternMatch` for pattern types

Cover the switch branches:

- `url_exact`
- `url_domain` / `url_subdomain`
- `url_regex`
- default (generic text)

Assertions:

- `IsMatch` and expected confidence values (1.0, 0.9, or `pattern.ConfidenceScore`)
- `MatchTime` is set (>= 0)
- `Position` stays `-1` (current implementation)

### 3) Aggregate scoring: `calculateAnalysisMetrics`

Provide a `PatternAnalysis` with multiple `PatternMatch` entries and assert:

- `RiskScore` increases with higher severity + confidence and is capped at 1.0
- `Confidence` is the average confidence
- `Categories` contains unique categories (order not guaranteed; compare as sets)

### 4) Reputation adjustment: `calculateReputationAdjustment`

Cover representative cases:

- brand-new account (`AccountAge < 7`) increases adjustment
- very old account (`AccountAge > 365`) decreases adjustment
- low follower count increases adjustment; high follower count decreases
- violations increase adjustment; cap at `2.0`

### 5) Optimality scoring: `calculateOptimalityScore`

Construct patterns to hit branches:

- with/without `AverageMatchTime`
- with/without `LastUsed`
- priority influence
- cap at `1.0`

### 6) Optional: safe coverage for `AnalyzeContentPatterns`

`AnalyzeContentPatterns` calls `RecordMatch` only when `match.IsMatch == true`.

To avoid DB mocking, you can:

- include patterns that are `Active: true` but **do not match** content
- assert that:
  - `Matches` stays empty
  - `ProcessTime` is set (>= 0)
  - `RiskScore` and `Confidence` remain 0

## Deliverables

- New tests in `pkg/storage/repositories/`, suggested filename:
  - `enhanced_pattern_repository_analysis_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

