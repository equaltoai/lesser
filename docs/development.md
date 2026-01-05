# Development Setup

<!-- AI Training: Local development workflows for the Lesser monorepo -->

This repo is primarily Go + AWS Lambda, with CDK infrastructure under `infra/cdk/` and a separate `auth-ui/` frontend
that is built as part of deployment.

## Prerequisites

- Go 1.25+
- Node.js + `pnpm` (needed when working on `infra/cdk/` or `auth-ui/`, and for `./lesser up`)
- Docker (optional; for local DynamoDB via `./lesser dev dynamodb`)

## Quick start: run the API locally

Build the operator/dev CLI (the only “build step” we document explicitly):

```bash
go build -o lesser ./cmd/lesser
```

Initialize a local `.env` (one-time):

```bash
./lesser dev init
```

Run the API:

```bash
./lesser dev
```

`./lesser dev` runs `go run ./cmd/api` with values sourced from `.env`.

✅ CORRECT: use `./lesser dev init` + `.env` for local development.

❌ INCORRECT: hand-maintain a long list of exported env vars in your shell profile (it makes debugging and onboarding
harder).

## Optional: local DynamoDB

Start DynamoDB Local:

```bash
./lesser dev dynamodb
```

Point the app at it by adding to `.env`:

```bash
DYNAMODB_ENDPOINT=http://localhost:8000
```

## Editing GraphQL

GraphQL schema sources live in `graph/*.graphql`.

When you change schema files:

```bash
./lesser schema
./lesser verify schema
```

Published contract for client teams:

- `docs/contracts/graphql-schema.graphql` (generated)

## Editing REST routes (OpenAPI)

OpenAPI is generated from code and published as:

- `docs/contracts/openapi.yaml`

Regenerate/verify:

```bash
./lesser generate openapi
./lesser verify openapi --strict
```

## Build Lambda artifacts (for deploy)

Build all Lambda zip artifacts:

```bash
./lesser build lambdas
```

Force rebuild:

```bash
./lesser build lambdas --rebuild
```

## Seed + validate a deployed environment (destructive)

`./lesser dev seed-and-validate` exists to support a “clear → seed → validate” workflow against a deployed stage.

⚠️ WARNING: this clears the target DynamoDB table. Never point it at live.

Example (dev stage):

```bash
./lesser dev seed-and-validate \
  --aws-profile <profile> \
  --dynamodb-table <table-name> \
  --base-url https://dev.example.com
```

## Quality checks

```bash
./lesser fmt
./lesser lint
./lesser test unit
./lesser verify
```
