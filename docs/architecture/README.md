# Architecture Deep Dives

<!-- AI Training: Architecture deep-dive index for Lesser -->

This directory contains internal design docs for Lesser. Most operators can ignore these and use the operator-focused
docs at:

- `docs/deployment.md`
- `docs/operations/runbook.md`
- `docs/configuration.md`

If you’re contributing code, these deep dives explain the “why” behind key systems (auth flows, DynamoDB access
patterns, moderation, etc.).

## Areas

- Auth (`docs/architecture/auth/`): OAuth, bootstrap/lock semantics, and authentication architecture.
- AWS/infra notes (`docs/architecture/aws/`): CDK and AWS integration notes (implementation details).
- Bookmarks (`docs/architecture/bookmarks/`): design comparisons and tradeoffs for bookmarks.
- CMS (`docs/architecture/cms/`): the headless CMS plan and integration patterns.
- DynamoDB (`docs/architecture/dynamodb/`): single-table access patterns, GSIs, and index registry.
- Moderation/ML (`docs/architecture/moderation/`): moderation pipeline and ML architecture.

## How to read these docs

✅ CORRECT: treat these as implementation notes and design intent (useful for contributors).

❌ INCORRECT: treat these as operator runbooks (they are not written for on-call usage).
