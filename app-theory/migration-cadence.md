# lesser: AppTheory/TableTheory Migration Cadence (Dedicated Branch)

Generated: 2026-01-31

This document defines the working cadence for executing `app-theory/migrate-lift-roadmap.md` as a series of clean,
verified, pushed cycles on a dedicated branch, using the current local checkout as the starting point.

## Principles
- **No shims / no dual runtime:** each cycle lands real migration progress (no compatibility wrappers that preserve Lift/DynamORM call sites).
- **Small, complete loops:** every cycle ends with verification + commit + push.
- **Always push green commits:** do not push “known failing” commits to the migration branch.
- **One branch, many commits:** work is staged via commits on the same branch (not via long-lived feature branches).

## Branch setup (one-time)
1. Ensure the working tree is clean and up to date with your local foundation:
   - `git status`
   - If you have local changes that are intended to be the baseline, commit them first (see “Commit rules”).
2. Create the dedicated migration branch from the current local HEAD:
   - `git checkout -b migration/apptheory-tabletheory-sunset-lift-dynamorm`
3. Verify remote exists:
   - `git remote -v`
4. Push the empty branch (or first baseline commit) once:
   - `git push -u origin migration/apptheory-tabletheory-sunset-lift-dynamorm`

## Cycle template (repeat until complete)

### 1) Choose a “portion” of the roadmap
Pick a portion that can be completed end-to-end without leaving Lift/DynamORM/DynamoDB-SDK usage behind in that area.

Each cycle MUST define:
- **Target area:** directories/files you will touch (explicit list).
- **Success criteria:** concrete grep/test gates for that portion (examples below).
- **Expected removals:** which imports/packages/types must disappear as a result of the cycle.

Recommended portion boundaries (examples; pick one per cycle):
- `infra/cdk` Lift CDK removal (construct-by-construct)
- AppTheory runtime conversion for one Lambda entrypoint (HTTP / Stream / SQS / Scheduled / WebSocket)
- TableTheory conversion for one storage layer boundary (models + repositories + callers)
- “Repo hygiene” batches (rename dirs, remove scripts/tools, update docs), but only if the cycle still ends green

### 2) Implement the portion (edit/move/delete as required)
Work directly against the migration branch.

Hard rules while implementing:
- Do not introduce new Lift/DynamORM/DynamoDB-SDK imports.
- Rename legacy directories early (e.g., `cmd/api/lift/` → `cmd/api/handlers/`) so new code cannot keep old names.
- When converting Lift event handlers to AppTheory, register by **exact** route keys (table/queue/rule), not `"*"` globs.

### 3) Verify locally (cycle gate)
Run these in order. Do not proceed to commit until they pass.

**A. “No legacy imports” gates (always):**
- `rg -n "github.com/pay-theory/lift" --glob '*.go' -S .`
- `rg -n "github.com/pay-theory/dynamorm" --glob '*.go' -S .`
- `rg -n "aws-sdk-go-v2/service/dynamodb" --glob '*.go' -S .`

**B. Build + unit tests (always):**
- `go test ./...`

**C. Repo test runner (always):**
- `./lesser test`

**D. Full repo verification (required at least once per major milestone):**
- `make verify`

Milestone recommendation for running `make verify`:
- After completing each of these milestones:
  - All Lift runtime removal (`cmd/*`, `pkg/*`, `tools/*`, scripts)
  - All TableTheory migration + removal of DynamORM + removal of DynamoDB SDK imports
  - All `infra/cdk` removal of Lift CDK constructs + successful synth/tests

### 4) Commit (cycle output)
Commit only once the cycle gate passes.

**Commit rules**
- One logical portion per commit (or a tight series of commits if necessary).
- Commit messages: short, present tense, lowercase preferred (repo guideline).
- Include the portion scope in the message when helpful.

**Commands**
- Review: `git status` and `git diff`
- Stage: `git add -A`
- Commit: `git commit -m "sunset lift: <portion>"` (examples below)

Example commit messages:
- `sunset lift: migrate api lambda to apptheory`
- `sunset lift: replace lift cdk rest api with apptheory cdk`
- `sunset dynamorm: port models to tabletheory tags`
- `sunset dynamodb sdk: replace config validator with tabletheory`

### 5) Push (cycle completion)
Push after every successful cycle commit:
- `git push`

If this is your first push of the branch:
- `git push -u origin migration/apptheory-tabletheory-sunset-lift-dynamorm`

## Suggested cadence schedule (timeboxed)
Use a fixed rhythm to keep progress predictable:
- **Cycle length:** 60–120 minutes of implementation + 15–30 minutes of verification.
- **End of day:** push a green commit; do not end the day with only unpushed local work.
- **Every 3–5 cycles:** run `make verify` even if not at a milestone, to catch drift early.

## Recommended cycle sequence (no shims, but still manageable)
This is a suggested order; adjust based on conflicts you find. Each bullet should be treated as “one or more cycles”.

1. **Create migration branch + baseline gates**
2. **Rename legacy framework directories and update tooling references**
   - `cmd/api/lift/` → `cmd/api/handlers/`
   - `cmd/api/routes_lift.go` → `cmd/api/routes.go`
   - Update `tools/openapi/*` package paths
3. **Replace Lift runtime with AppTheory runtime across all Lambdas**
   - HTTP Lambdas first (API/GraphQL/etc.)
   - Then Streams/SQS/Scheduled
   - Then WebSockets + SSE
4. **Replace Lift-derived helper packages**
   - `pkg/lift/*`, `pkg/lift/patterns/*`, `pkg/testing/lift/*`, Lift mocks
   - `pkg/deploy/naming/*` must not import Lift naming
5. **Replace DynamORM + native DynamoDB SDK usage with TableTheory**
   - Convert model tags (`dynamorm:"..."` → `theorydb:"..."`)
   - Port repositories to `pkg/storage/tabletheory/*`
   - Replace errors/mocks
   - Remove `pkg/storage/dynamorm/*` and all remaining DynamoDB SDK imports
6. **Replace Lift CDK constructs in `infra/cdk`**
   - Migrate to `github.com/theory-cloud/apptheory/cdk-go/apptheorycdk` constructs where applicable
   - Replace remaining Lift-only constructs with native AWS CDK constructs or local constructs
7. **Final dependency removal + tidy**
   - Remove Lift/DynamORM from `go.mod` and Lift from `infra/cdk/go.mod`
   - `go mod tidy` (root + `infra/cdk`)
8. **Final full verification**
   - All “no legacy imports” gates pass
   - `go test ./...`
   - `./lesser test`
   - `make verify`

## Progress tracking
Maintain a simple running log as you go (append-only in PR description or a local note):
- Cycle number
- Portion name
- Key files changed
- Commands run (especially `make verify`)
- Any follow-ups discovered (queued for later cycles)

