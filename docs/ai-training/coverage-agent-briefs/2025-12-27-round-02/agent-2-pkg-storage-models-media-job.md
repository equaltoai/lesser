# Agent 2 Brief — `pkg/storage/models/media_job.go`

## Goal

Increase `pkg/storage/models` coverage by adding thorough unit tests for the `MediaJob` model lifecycle methods and helpers.

Primary target:

- `pkg/storage/models/media_job.go`

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Prefer table-driven tests + `stretchr/testify`.
- Avoid `time.Sleep`; use `assert.WithinDuration` for timestamp checks.

## What to cover

### 1) Key generation + validation

Test `UpdateKeys()`:

- Missing `JobID` returns `ErrMediaJobIDRequired` (wrapped) or at least `require.Error`
- Sets:
  - `PK == "JOB#<jobID>"`, `SK == "JOB#<jobID>"`
  - when `Username` set: `GSI1PK == "USER_JOBS#<username>"`, `GSI1SK` contains RFC3339 timestamp + job id
  - `GSI2PK` uses the status pattern and `GSI2SK` starts with `"UPDATED#"`

### 2) `BeforeCreate` defaults

Test `BeforeCreate()`:

- Generates `JobID` when empty
- Generates `IdempotencyKey` when empty
- Defaults:
  - `Status == StatusPending`
  - `Results` initialized to non-nil map
  - `ProcessingTasks` initialized to empty slice (non-nil)
  - `MaxRetries == 3` when 0
  - `MaxProcessingTime` set via default timeout helper when 0
  - `ExpiresAt` set (~24h from now)
- Calls `UpdateKeys()` and `Validate()` successfully when required fields are present

Also add one negative test: missing required fields (e.g. `MediaID` or `S3Key`) returns error via `Validate()`.

### 3) `BeforeUpdate`

Test `BeforeUpdate()` updates `UpdatedAt`, calls `UpdateKeys()`, and enforces validation.

### 4) State transitions

Test:

- `SetProcessing()` sets status, timestamps, clears error, and doesn’t panic
- `SetCompleted(results)` sets `Progress=100`, clears TTL, sets `CompletedAt`, stores results
- `SetFailed(msg)` sets status, sets TTL (~7 days), copies last error, sets `CompletedAt`
- `IsCompleted/IsFailed/IsProcessing/IsPending` truth table

## Deliverables

- New test file: `pkg/storage/models/media_job_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

