# Cost Optimization

<!-- AI Training: Operator cost levers and debugging workflow for Lesser -->

Lesser is designed to run “pay for what you use” on AWS serverless primitives. This doc focuses on the knobs operators
can turn and where to look when costs spike.

## Cost-related configuration knobs

Most cost levers live in CDK code + inventory (not ad-hoc environment variables).

Common levers:

- Lambda memory/timeouts/log retention: `infra/cdk/inventory/lambdas.go` (also summarized in `docs/specs/01-lambda-inventory-matrix.md`).
- DynamoDB PITR + deletion protection: enabled for the live stage in `infra/cdk/stacks/lesser_api_stack.go`.
- Observability volume: controlled by env vars like `MONITORING_ENABLED`, `EMF_METRICS_ENABLED`, and `DISABLE_METRICS` (see `docs/configuration.md`).

## Quick “what changed?” checks

1) Identify the expensive area (Lambda, DynamoDB, CloudFront, S3) in AWS Cost Explorer.
2) Validate you didn’t accidentally enable “always-on” style capacity.

DynamoDB should generally be on-demand unless you intentionally provision capacity:

```bash
AWS_PROFILE=<profile> aws dynamodb describe-table --table-name <table> --query 'Table.BillingModeSummary'
```

## If DynamoDB costs spike

- Look for scans and hot partitions in CloudWatch metrics.
- Verify new access patterns use GSIs (see `docs/architecture/dynamodb/gsi_usage_guide.md`).

## If Lambda costs spike

- Check error retries and queue backlogs (SQS DLQs + CloudWatch).
- Look for a single handler getting hammered; tail logs via `./lesser logs --app <app> --function api --env dev`.

## If CloudFront costs spike

Common causes:

- a client release that disables caching (re-fetching `index.html` or large media repeatedly)
- hotlinking / abuse (consider WAF or signed URLs for private content)

Quick checks:

- verify CloudFront cache behaviors are correct for static assets (`/l/*`, `/auth/*`)
- look for high 4xx/5xx or unusual geographic traffic patterns in CloudFront metrics
