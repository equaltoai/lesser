# JWT Consolidation Migration

This guide helps migrate projects from legacy JWT helpers in `lift` to the canonical middleware API in `middleware`.

## Summary

- Canonical API: `middleware.JWTAuth(middleware.JWTConfig)`
- Deprecated (to be removed in a future major version):
  - `lift.WithJWTAuth(lift.JWTAuthConfig)`
  - `lift.WithSimpleJWTAuth(secret string)`

## Before

```go
app := lift.New(
    lift.WithJWTAuth(lift.JWTAuthConfig{
        Secret:    os.Getenv("JWT_SECRET"),
        Algorithm: "HS256",
        SkipPaths: []string{"/health"},
    }),
)
```

## After

```go
app := lift.New()
app.Use(middleware.JWTAuth(middleware.JWTConfig{
    Secret:     os.Getenv("JWT_SECRET"),
    Algorithm:  "HS256",
    TokenLookup: "header:Authorization",
    SkipPaths:  []string{"/health"},
}))
```

## Config Mapping

- `JWTAuthConfig.Secret`       -> `middleware.JWTConfig.Secret`
- `JWTAuthConfig.Algorithm`    -> `middleware.JWTConfig.Algorithm`
- `JWTAuthConfig.TokenLookup`  -> `middleware.JWTConfig.TokenLookup` (default `header:Authorization`)
- `JWTAuthConfig.SkipPaths`    -> `middleware.JWTConfig.SkipPaths`
- `JWTAuthConfig.Validator`    -> `middleware.JWTConfig.Validator`
- `JWTAuthConfig.ErrorHandler` -> `middleware.JWTConfig.ErrorHandler`

## Rationale

- Eliminates duplicated JWT implementations.
- Keeps a single, well-documented API surface for authentication.
- Avoids import cycles between `lift` and `middleware` packages.

## Timeline

- Current release: wrappers are marked as `Deprecated`.
- Next major release: wrappers will be removed. Please migrate at your earliest convenience.

## Tips

- Prefer route grouping with `SkipPaths` for public routes.
- Keep validation in `JWTConfig.Validator` for claim semantics (expiry, roles, tenant constraints, etc.).
- Use `ctx.SetClaims` only for synthetic/test scenarios; middleware handles this in production.

