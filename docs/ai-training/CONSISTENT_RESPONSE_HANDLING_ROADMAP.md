# Consistent Response Handling Roadmap (Lesser)

This roadmap formalizes a single, predictable contract for how Lesser converts failures (from DynamoDB/DynamORM, AWS SDKs, business rules, validation, auth) into **stable domain errors** and then into **stable client responses** (HTTP, Lambda, GraphQL).

The goal is not “more errors” or “more refactors” — it is to eliminate mismatches such as **expected conflicts returning 500s**, “not found” returning different shapes/statuses depending on call path, and repository/service layers losing error semantics after wrapping.

## Definition: “Consistent Response Handling”

Across the entire codebase:

- A failure condition maps to **exactly one canonical error code** (`pkg/errors.ErrorCode`) at the boundary of each layer.
- That error code maps to a **single HTTP status code** (or GraphQL extension / Lambda pattern) everywhere.
- Error semantics survive wrapping: `errors.Is`, `errors.As`, and code/category checks work reliably through the unwrap chain.
- “Expected” conditions (e.g. *already exists*, *not found*, *validation failed*, *unauthorized*) never bubble up as 500.
- Idempotent operations are explicit: “already pinned”, “already followed”, “delete missing item”, etc have one defined behavior.

## Current Problems Observed

- Repository helpers (e.g. `pkg/storage/repositories/errors.go`) re-wrap already-mapped `*pkg/errors.AppError` as `CodeInternal`, causing **409/404/400 to become 500** depending on code path.
- Some layers rely on DynamORM sentinel checks (`dynamorm/pkg/errors.IsNotFound`) but other layers return mapped `AppError` without preserving the sentinel in the unwrap chain, preventing consistent behavior and testability.
- Several places compare sentinel errors via pointer equality (e.g. `err == storage.ErrNotFound`), which is fragile and breaks as soon as errors are wrapped or contextualized.
- Multiple response patterns exist simultaneously (middleware-based vs handler-writes-response), leading to drift in status codes and error envelope shape.

## Guiding Principles

1. **One error type across layers:** `*pkg/errors.AppError` (wrapping other errors as needed).
2. **No pointer-equality checks for behavior:** use `errors.Is`, `errors.As`, or `pkg/errors.HasCode`.
3. **Wrapping must preserve semantics:** when mapping external/sentinel errors, keep them discoverable via `errors.Is` / `errors.As` (including joins when multiple sentinels must remain true).
4. **Handlers decide behavior, not repositories:** repositories map persistence concerns; API/service layers decide whether conflicts are errors vs idempotent success.
5. **Measure via tests:** each milestone includes “golden” tests validating status code + response envelope invariants.

## Progress (as of 2025-12-29)

- [x] Milestone 1 — Canonical Error Introspection
- [x] Milestone 2 — Repository + Storage Error Translation Contract
- [x] Milestone 3 — Remove Pointer-Equality Error Handling
- [ ] Milestone 4 — API Error Envelope and Status Code Standardization
- [ ] Milestone 5 — GraphQL + Lambda Consistency
- [ ] Milestone 6 — Documentation + Guardrails

---

## Milestones

### Milestone 1 — Canonical Error Introspection (pkg/errors)

**Goal:** guarantee that “is this an AppError / what code is it” works regardless of wrapping.

**Work:**
- Update `pkg/errors.AsAppError`/`IsAppError` to use `errors.As` over the unwrap chain.
- Ensure `pkg/errors.HasCode`, `HasCategory`, `GetHTTPStatus`, etc work for wrapped errors.
- Add focused tests for wrapped AppErrors (e.g. `fmt.Errorf("ctx: %w", appErr)`).

**Acceptance criteria:**
- Wrapped errors still satisfy `errors.As(err, *AppError)` and `pkg/errors.HasCode(err, <code>)`.
- No call sites rely on direct type assertions for AppError detection.

**Validation:**
- `./lesser test unit`

---

### Milestone 2 — Repository + Storage Error Translation Contract

**Goal:** repository helpers never convert expected conditions to 500, and preserve sentinel semantics through mapping.

**Work:**
- Update `pkg/storage/repositories/errors.go`:
  - `HandleGetError/HandleCreateError/HandleUpdateError/HandleDeleteError` must pass through `*pkg/errors.AppError` without re-wrapping as internal errors.
  - `HandleDeleteError` treats “not found” consistently (idempotent delete) even when error is already mapped.
- Update `MapDynamoDBError` to avoid returning shared mutable sentinel instances; preserve semantics via unwrap-chain joins when needed.
- Add tests proving:
  - `HandleCreateError(storage.ErrAlreadyExists, ...)` returns an error that maps to **409**, not 500.
  - `HandleGetError(storage.ErrNotFound, ...)` returns an error that maps to **404**, not 500.
  - DynamORM sentinel checks still work when they must (via unwrap chain).

**Acceptance criteria:**
- No repository helper converts `CodeNotFound`/`CodeAlreadyExists`/`CodeValidationFailed` into `CodeInternal`.
- Not-found and condition-failed are consistently detectable via codes and `errors.Is`.

**Validation:**
- `./lesser test unit`
- Optional: `./lesser test` (broader)

---

### Milestone 3 — Remove Pointer-Equality Error Handling

**Goal:** eliminate fragile comparisons that cause response drift when errors are wrapped.

**Work:**
- Replace `err == storage.ErrNotFound` / `err != storage.ErrNotFound` and similar patterns with `errors.Is(err, storage.ErrNotFound)` and/or `pkg/errors.HasCode(err, CodeNotFound)` where appropriate.
- Update any switch statements that match on sentinel pointers (e.g. Lift error mappers) to use semantic checks.
- Add tests for the critical mappers (API and Lift) to validate stable output for wrapped errors.

**Acceptance criteria:**
- No production logic depends on pointer equality for AppError instances.
- Wrapped not-found / already-exists errors are handled identically to direct sentinel values.

**Validation:**
- `./lesser test unit`

---

### Milestone 4 — API Error Envelope and Status Code Standardization (cmd/api + pkg/common)

**Goal:** HTTP clients receive a stable error envelope and stable status code mapping everywhere.

**Work:**
- Decide and document a canonical JSON error envelope:
  - Minimum: `{"error": "...", "error_code": "..."}` (optionally `error_description`).
- Ensure `pkg/common/error_middleware.go` and response helpers produce the same envelope for the same `ErrorCode`.
- Remove “string keyword” heuristics as primary routing (keep only as defensive fallback).
- Add golden tests for representative endpoints (small set, focused on error mapping).

**Acceptance criteria:**
- Expected conflicts return 409, not 500.
- Not found returns 404 with consistent JSON structure.
- Validation returns 400 with consistent JSON structure.

**Validation:**
- `./lesser test unit`

---

### Milestone 5 — GraphQL + Lambda Consistency

**Goal:** GraphQL resolvers and Lambda processors preserve the same domain error codes and map them consistently.

**Work:**
- Standardize GraphQL error extensions (`extensions.code`, `extensions.http_status`) derived from `*pkg/errors.AppError`.
- Standardize Lambda error patterns (`pkg/lambda/error_patterns.go`) to preserve codes and to avoid “expected” errors being treated as internal.
- Add tests around the mappers, not every resolver.

**Acceptance criteria:**
- Same domain error produces consistent code/status across HTTP and GraphQL/Lambda.

**Validation:**
- `./lesser test unit`

---

### Milestone 6 — Documentation + Guardrails

**Goal:** prevent regressions and stop future “coverage fixes” from changing semantics.

**Work:**
- Add a short “Error Handling Contract” doc:
  - When to create `AppError`, when to wrap, when to join.
  - How to check error conditions (no pointer equality).
  - Where response mapping must happen.
- Add lightweight lint guardrails (where feasible) for patterns like `err == storage.ErrNotFound`.

**Acceptance criteria:**
- Clear written contract + examples; reviewers can enforce consistently.

---

## Immediate Next Actions (What This Roadmap Enables)

Start with Milestones 1–3 first; they unlock correctness and make later API/GraphQL standardization low-risk and mechanically testable.
