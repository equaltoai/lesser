# Error Handling Contract (Lesser)

This document formalizes the **contract** for how Lesser represents errors and maps them to client-visible responses across **HTTP (Lift)**, **GraphQL (gqlgen)**, and **Lambda**.

It is the enforcement companion to `docs/ai-training/CONSISTENT_RESPONSE_HANDLING_ROADMAP.md`.

## Goals

- A failure condition maps to **one canonical error code** (`pkg/errors.ErrorCode`) at layer boundaries.
- That code maps to **one stable HTTP status** everywhere.
- Error semantics survive wrapping: `errors.Is`, `errors.As`, and `pkg/errors.HasCode` must work reliably.
- Expected conditions (**404**, **409**, validation, auth) never leak as **500**.
- Idempotent operations are explicit (e.g. “already pinned”, “delete missing item”).

## Canonical Error Type

Use `*pkg/errors.AppError` everywhere outside the narrowest legacy seams.

- Create errors via constructors in `pkg/errors` (e.g. `errors.NotFound`, `errors.ValidationFailed`, `errors.Forbidden`).
- Wrap internal causes using `errors.InternalWithCause` or `errors.WrapError`.
- Never mutate shared instances; if you need to add metadata to a shared error, use `(*AppError).Clone()`.

## How To Check Errors

Use semantic checks:

- `pkg/errors.AsAppError(err)` for extracting the canonical error.
- `pkg/errors.HasCode(err, errors.CodeNotFound)` for stable behavioral routing.
- `errors.Is(err, storage.ErrNotFound)` for sentinel compatibility (only when needed).

Avoid fragile checks:

- Do not use pointer equality: `err == storage.ErrNotFound` (guardrailed by unit tests).
- Avoid routing on `strings.Contains(err.Error(), "...")` except as a last-resort defensive fallback.

## Preserving Semantics When Mapping

When translating external library errors (DynamORM, AWS SDK, etc), **preserve semantics** through wrapping:

- If the mapped error must satisfy multiple “is” checks, join sentinels into the unwrap chain:
  - Example: `errors.Join(dynamormErr, storage.ErrNotFound)`
- Repository/storage mapping must not convert expected conditions into `CodeInternal`.

## Response Mapping (Where It Happens)

### HTTP (Lift) — `cmd/api`

Preferred pattern:

- Handlers **return errors** and let `common.CreateAPIErrorMiddleware(logger)` write the response.
- If a handler must write responses directly, use `pkg/common` responders (they emit `error_code`).

Canonical JSON envelope:

```json
{"error":"…","error_code":"…"}
```

### GraphQL — `cmd/graphql`

GraphQL responses must attach:

- `errors[].extensions.code` (domain code)
- `errors[].extensions.http_status` (mapped HTTP status)

This is implemented via the gqlgen `ErrorPresenter`.

### Lambda

Prefer `pkg/lambda.ErrorPattern` middleware; it maps `*pkg/errors.AppError` directly and converts legacy error types without losing status/code intent.

## Status Mapping (Core)

Status is derived from `pkg/errors.ErrorCode.GetHTTPStatusCode()` (single-source-of-truth):

- `NOT_FOUND` → `404`
- `ALREADY_EXISTS` / `CONFLICT` → `409`
- `VALIDATION_FAILED` / `BAD_REQUEST` / `INVALID_INPUT` → `400`
- `UNAUTHORIZED` / `AUTH_FAILED` / `TOKEN_*` → `401`
- `FORBIDDEN` / `INSUFFICIENT_SCOPE` → `403`
- `GONE` → `410`
- `UNPROCESSABLE_ENTITY` → `422`

## Testing Requirements

Every new “expected error” behavior must have at least one golden test asserting:

- HTTP status code
- JSON envelope includes `error` and `error_code` (or GraphQL extensions for GraphQL)
- Error code matches the intended domain code

## Anti-Patterns (Do Not Do These)

- Writing an HTTP response and also returning a non-nil error (double-response risk).
- Wrapping an already-canonical `*pkg/errors.AppError` into `CodeInternal`.
- Changing behavior to increase coverage (moving logic into new files, “coverage gaming”).

