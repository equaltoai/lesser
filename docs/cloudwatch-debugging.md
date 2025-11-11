# CloudWatch API Debugging Guide

This doc captures the basic workflow we use when digging through CloudWatch Logs for the serverless API stack. Everything here assumes you are working from the Lesser mono-repo with the standard tooling already installed.

## Prerequisites

1. **AWS CLI v2** authenticated for the target account. Example:
   ```bash
   aws sso login --profile Lesser
   ```
2. **Know the environment mapping** – the Lambda function names follow the pattern `lesser-<environment>-<function>`, e.g.
   - Development: `lesser-development-api`
   - Staging/Test: `lesser-staging-api`
   - Production: `lesser-production-api`
3. **Set the profile per command** or export it once:
   ```bash
   export AWS_PROFILE=Lesser
   export AWS_REGION=us-east-1
   ```

## Quick Tail via Makefile Target

For most day-to-day debugging, the `make logs` helper is enough. It handles the correct log group naming based on `ENV`.

```bash
make logs FUNCTION=api ENV=dev AWS_PROFILE=Lesser
```

Supported `FUNCTION` values match the entries in `LAMBDAS` inside the root `Makefile` (api, graphql, graphql-ws, inbox, etc.).

## Manual `aws logs tail`

When you need more control (custom time ranges, multiple functions, or piping into other tools) use the raw CLI:

```bash
AWS_PROFILE=Lesser aws logs tail \
  /aws/lambda/lesser-development-api \
  --since 15m \
  --format short
```

Tips:
* Swap the log group for GraphQL: `/aws/lambda/lesser-development-graphql`, streaming: `/aws/lambda/lesser-development-streaming`, etc.
* Pipe to `rg`/`jq` to zero in on specific events:
  ```bash
  AWS_PROFILE=Lesser aws logs tail /aws/lambda/lesser-development-api --since 30m --format short |
    rg "POST /api/v1/apps" -C2
  ```

## Filtering With CloudWatch Logs Insights

For longer sessions or historical queries, the Logs Insights console is still the best option. A few ready-to-paste queries:

**API endpoint errors**
```sql
fields @timestamp, @message
| filter path = "/api/v1/apps" and status >= 400
| sort @timestamp desc
| limit 50
```

**GraphQL resolver failures**
```sql
fields @timestamp, request_id, message
| filter caller like /graphql/ and level = "error"
| sort @timestamp desc
| limit 100
```

Set the log group to `/aws/lambda/lesser-<environment>-api` (or `graphql`, etc.) before executing the query.

## Common Patterns

| Scenario | Command/Notes |
|----------|---------------|
| Inspect a single request lifecycle | `aws logs tail ... --since 5m --format short` and look for `request_start` / `request_complete` pairs emitted from `cmd/api/middleware.go`. |
| Trace GraphQL timeline issues | Tail `/aws/lambda/lesser-<env>-graphql` and search for `convertStatusToObject` or `notes/service.go`. |
| Debug WebSockets | Tail `/aws/lambda/lesser-<env>-graphql-ws` for authentication failures (`websocket connect failed token validation`). |
| Review cost/latency metrics | Look for log entries from `repositories/metrics_repository.go` – failures usually indicate DynamoDB throttling or bad IAM permissions. |

## Alerting & Follow-ups

* **Repeated `ValidationException` entries** usually mean our DynamoDB schema and the structs in `pkg/storage/models` drifted. Fix the model rather than filtering logs.
* **`context canceled`** create metric failures typically mean the Lambda hit the timeout while we were still pushing monitoring datapoints. Increase the Lambda timeout or move metrics writes off the critical path.
* **Missing log data** – ensure the Lambda’s IAM role still has `logs:CreateLogStream` / `logs:PutLogEvents` permissions (managed by CDK’s shared roles).

Keeping these patterns in one place saves a few minutes every time we have to spelunk through CloudWatch, and makes sure we stick to our “never bypass DynamORM” and “log in JSON” conventions.
