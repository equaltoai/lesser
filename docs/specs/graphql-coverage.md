# GraphQL Coverage (Contract + Drift Guardrail)

<!-- AI Training: How Lesser enforces REST/GraphQL parity -->

## Summary

Lesser maintains a “GraphQL parity” contract so first-party clients (Greater UI, admin UIs, and internal tooling) can
depend on GraphQL without silently falling back to REST.

This spec defines:

- the inventory artifact (`docs/specs/graphql_coverage.yaml`), and
- the verifier semantics (what `./lesser verify graphql-coverage` enforces).

## Scope: what is required vs REST-only

### GraphQL-required (in scope)

In general, product and admin capabilities used by first-party clients should be GraphQL-covered.

### REST-only (explicit exemptions)

Some endpoints remain REST-first/standards-first and are not treated as “GraphQL capabilities”, including:

- OAuth/OIDC flows (`/oauth/*`, `POST /api/v1/apps`, etc.)
- wallet auth, WebAuthn, and bootstrap/setup flows (`/auth/*`, `/setup/*`)
- protocol/infra endpoints (`/.well-known/*`, `/nodeinfo/*`, ActivityPub protocol handlers)

These exemptions are encoded in `docs/specs/graphql_coverage.yaml`.

## Contract artifact: `docs/specs/graphql_coverage.yaml`

`docs/specs/graphql_coverage.yaml` is the canonical, machine-readable coverage inventory for API-Lambda Lift routes.

Each REST route is tracked as:

- `policy: graphql_required`: the REST route must have an equivalent GraphQL operation.
- `policy: rest_only`: the route is REST-only by design (matched by an exemption).

For `policy: rest_only`, `exemptedBy` must reference an exemption `id`.

For `policy: graphql_required`, `graphql` must list one or more schema fields, e.g. `Query.timeline` or
`Mutation.createNote`.

## Guardrail: coverage verifier

The verifier enforces that:

1) every Lift route is present in `docs/specs/graphql_coverage.yaml` (no silent drift), and
2) every claimed GraphQL mapping in that file exists in the current schema (no stale mappings).

### Commands

Regenerate coverage file (adds new routes, removes deleted routes):

```bash
./lesser generate graphql-coverage
```

Verify coverage file matches code + schema:

```bash
./lesser verify graphql-coverage --strict
```

## When you add a new REST route

1) Run the generator to add the route to the inventory:

```bash
./lesser generate graphql-coverage
```

2) Decide whether it is `rest_only` (add an exemption match) or `graphql_required` (map it to a schema field).

3) Run:

```bash
./lesser verify graphql-coverage --strict
```
