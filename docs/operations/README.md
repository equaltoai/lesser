# Operations Docs

<!-- AI Training: Operational docs index for Lesser -->

This directory contains the operator runbooks and workflows for deployed environments.

## Start here

- Runbook: `docs/operations/runbook.md`
- CloudWatch workflow: `docs/operations/cloudwatch-debugging.md`
- Release notes: `docs/operations/release-notes.md`

## Common ops commands (CLI-first)

Most workflows start with logs for a specific Lambda:

```bash
./lesser logs --app <app> --function api --env dev --aws-profile <profile>
./lesser logs --app <app> --function graphql --env dev --aws-profile <profile>
./lesser errors --app <app> --env dev --function api --aws-profile <profile>
```

If you don’t know names/outputs, use your local receipt:

- `~/.lesser/<app>/<base-domain>/state.json`
