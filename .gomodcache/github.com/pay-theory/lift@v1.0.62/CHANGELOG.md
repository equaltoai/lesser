# Changelog

## Unreleased

- Router: Unmatched HTTP routes now return structured 404 `LiftError` instead of a generic error.
- Response: `Binary` responses are correctly base64-encoded and flagged with `isBase64Encoded=true`; JSON marshalling respects base64 mode.
- Middleware (Lift IP Authorization): Stop writing responses via deprecated context helpers; now returns `LiftError`s (`ParameterError`, `SystemError`, `AuthorizationError`).
- Health Endpoints: Added optional structured logging support via `HealthEndpointsConfig.Logger`; falls back to standard logging when not provided.
- Docs sweep for accuracy and consistency:
  - Replace `middleware.JWT(...)` with `middleware.JWTAuth(...)` and remove erroneous error returns.
  - Replace `ctx.Bind(...)` with `ctx.ParseRequest(...)`.
  - Use `lift.NewLiftError(...)` and dedicated error constructors (`ValidationError`, `NotFound`, `SystemError`) instead of `NewError`/`BadRequest` patterns.
  - Fix response examples to use `ctx.JSON(data)` for 200 or `ctx.Status(code).JSON(data)` otherwise.
  - Update training and guidance files: `_patterns.yaml`, `_decisions.yaml`, `troubleshooting.md`, `migration-guide.md`, `api-reference.md`, `core-patterns.md`, `dynamorm-integration.md`, `README.md`, `development-guidelines.md`, `cdk/event-driven-api-pattern.md`.
- Tests: Added tests for router 404 behavior and binary response encoding.

### JWT Consolidation

- Canonical API is `middleware.JWTAuth(middleware.JWTConfig)`.
- `lift.WithJWTAuth` and `lift.WithSimpleJWTAuth` are now deprecated; see `docs/migrations/jwt-consolidation.md`.

> Note: `HealthEndpointsConfig` gained an optional `Logger` field; this is backwards-compatible. Existing uses continue to work without changes.
