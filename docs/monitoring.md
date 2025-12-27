# Monitoring

<!-- AI Training: Monitoring and observability workflows for Lesser -->

Most day-to-day debugging starts with CloudWatch logs for the Lambda you care about.

## Resource naming

Most AWS resources created by Lesser follow the pattern `<app>-<stage>-<resource>` where stage is `dev|staging|live`.

## Quick start: open the dashboard

If you know your region:

```bash
./lesser dashboard --app <app> --env live --region us-east-1
```

## Tail logs (fast)

```bash
./lesser logs --app <app> --function api --env dev --aws-profile <profile>
./lesser logs --app <app> --function federation-delivery --env dev --aws-profile <profile>
```

## Scan recent errors (fast)

```bash
./lesser errors --app <app> --env dev --function api --aws-profile <profile>
```

## Recent errors (example)

```bash
AWS_PROFILE=<profile> aws logs filter-log-events \
  --log-group-name /aws/lambda/<app>-dev-api \
  --filter-pattern "ERROR" \
  --max-items 20
```

## What to watch (practical checklist)

### Lambda

- `Errors` / `Throttles` / `Duration` (p95/p99)
- Sudden increases in `IteratorAge` for stream processors (signals backlog)

### DynamoDB

- `ThrottledRequests` spikes (hot partition or missing GSI)
- `ConsumedReadCapacityUnits`/`ConsumedWriteCapacityUnits` growth

### SQS

- `ApproximateNumberOfMessagesVisible` growth (backlog)
- DLQ growth (systematic failures)

### CloudFront / API Gateway

- 4xx/5xx spikes correlated with deploys or client releases

## Deep dive

- Runbook: `docs/operations/runbook.md`
- CloudWatch workflow: `docs/operations/cloudwatch-debugging.md`
