# Runbook

This runbook covers operational triage for Lesser deployments created via `./lesser up`.

## Know your deployment

- Deployment receipt: `~/.lesser/<app>/<base-domain>/state.json` (written by `./lesser up`)
  - Includes stage domains, stack names, DynamoDB table names, and CDK outputs (queue URLs, etc).
- Resource naming: most AWS resources follow `<app>-<stage>-<resource>` where stage is `dev|staging|live`.

## Quick checks

Replace `<stage-domain>` with `dev.<base-domain>`, `staging.<base-domain>` (if deployed), or `<base-domain>` (live).

```bash
# Deployment lock state (new deploys are locked until setup is finalized)
curl -s "https://<stage-domain>/setup/status" | jq .

# Mastodon-compatible instance endpoint
curl -s "https://<stage-domain>/api/v1/instance" | jq .
```

## Logs

```bash
./lesser logs --app <app> --function api --env dev --aws-profile <profile>
./lesser logs --app <app> --function graphql --env dev --aws-profile <profile>
./lesser logs --app <app> --function federation-delivery --env dev --aws-profile <profile>
```

Recent errors (quick scan):

```bash
./lesser errors --app <app> --env dev --function api --aws-profile <profile>
```

## Queues (backlog + DLQ)

Queue URLs are emitted as CloudFormation outputs and are stored in `state.json` under each stage’s `stack_outputs`.

Typical checks:

```bash
aws sqs get-queue-attributes \
  --queue-url "<queue-url>" \
  --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible ApproximateNumberOfMessagesDelayed
```

If a DLQ is growing:

- Tail the owning processor logs.
- Fix the root cause before replaying.
- Prefer replay tooling that preserves ordering and avoids thundering herds.

## Common incidents

### “Everything is empty” after deploy

Most endpoints are reachable while locked, but timelines and writes are blocked.

- Confirm lock status: `GET /setup/status`
- Finalize activation via the setup wizard: `https://<stage-domain>/auth/setup`

### Elevated 5xx from API

- Tail API logs: `./lesser logs --app <app> --function api --env <stage> --aws-profile <profile>`
- Check for DynamoDB throttling and Lambda timeouts in CloudWatch.
- If errors correlate with a deploy, re-run `./lesser up --app <app> --base-domain <base-domain> --aws-profile <profile> --rebuild-lambdas`.

### Unexpected 403/429 from crawler protection

Several HTTP Lambdas include a crawler protection middleware. Behavior is controlled by Lambda env vars:

- Mode: `CRAWLER_PROTECTION_MODE=off|observe|limit|block`
- Emergency bypass (skip block + rate limiting for matching client IPs):
  `CRAWLER_PROTECTION_BYPASS_CIDRS=203.0.113.0/24,2001:db8::/32`
- Rate limiter kill switch (use only for debugging): `DISABLE_RATE_LIMITING=true`
- Limit tuning (per hour): `CRAWLER_LIMIT_SEARCH_ENGINE_PER_HOUR`, `CRAWLER_LIMIT_GENERIC_BOT_PER_HOUR`, `CRAWLER_LIMIT_SUSPICIOUS_PER_HOUR`
- EMF metrics toggle: `CRAWLER_METRICS_ENABLED=true` (also requires `DISABLE_METRICS=false` and `EMF_METRICS_ENABLED=true`)

Triage:

- Look for `crawler classification` log entries (category + reason + client IP).
- Check the CloudWatch dashboard “Crawler” section for `Lesser/Crawler` metrics (blocked + rate limited).
- If you see false positives impacting legitimate traffic, switch to `CRAWLER_PROTECTION_MODE=observe` or add the
  affected client IP/CIDR to `CRAWLER_PROTECTION_BYPASS_CIDRS`, then redeploy.

### Federation delivery stuck

- Tail `federation-delivery` logs.
- Check the federation delivery queue depth (queue URL from `state.json`).
- Validate `/.well-known/webfinger` routing and that your stage domain resolves correctly.

## Recovery

Start with `docs/backup-recovery.md` and use table/bucket names from `state.json` (or CDK outputs), not hard-coded examples.
