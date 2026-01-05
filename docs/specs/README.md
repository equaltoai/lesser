# Lesser Specs (Internal)

This directory contains **active, machine-consumable specs/contracts** that are used by tooling and CI to prevent API
and infrastructure drift.

If you’re looking for **operator** documentation (deploy/run), start at `docs/README.md`.

## What belongs here

- `graphql_coverage.yaml` — Route → GraphQL parity inventory (generated/verified).
- `01-lambda-inventory-matrix.md` — Generated lambda inventory matrix (from CDK inventory).
- `graphql-coverage.md` — The GraphQL coverage contract and verifier semantics.
- `openapi-client-generation.md` — Recommended REST client generation workflow.

Client-facing API contract files live under `docs/contracts/`:

- `docs/contracts/openapi.yaml`
- `docs/contracts/graphql-schema.graphql`

## How these specs are used

✅ CORRECT: use these as repo guardrails (they should stay current as code changes).

These are enforced by the CLI:

```bash
./lesser verify inventory
./lesser verify graphql-coverage --strict
```

## Specs vs contracts

- `docs/contracts/`: “what client teams generate against” (OpenAPI + published GraphQL schema)
- `docs/specs/`: “how we prevent drift in this repo” (coverage inventory, lambda inventory matrix, tooling semantics)
