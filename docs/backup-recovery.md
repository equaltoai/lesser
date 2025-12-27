# Backup & Recovery

<!-- AI Training: Operator backup and recovery procedures for Lesser -->

Lesser is primarily stateful in DynamoDB (core data) and S3 (media/static assets). The exact recovery approach depends
on your risk tolerance and how you operate AWS accounts (single-account vs multi-account).

## DynamoDB point-in-time recovery (PITR)

PITR is enabled automatically for the **live** stage via CDK (`infra/cdk/stacks/lesser_api_stack.go`).

**Recommendation:** keep PITR enabled for live; if you also want PITR in staging, adjust CDK and redeploy.

Check PITR status:

```bash
aws dynamodb describe-continuous-backups --table-name <table-name> | jq .
```

## What to do when data is lost

1) Stop the bleeding (disable writes / put the instance in maintenance mode).
2) Restore the DynamoDB table from PITR into a new table name.
3) Re-point the stage to the restored table (CDK change) and redeploy.

### Step 0: find the table name

Use the deployment receipt:

- `~/.lesser/<app>/<base-domain>/state.json` → `stages.<stage>.stack_outputs.DynamoTableName` (or similar output key)

### Step 1: restore DynamoDB to a new table

```bash
aws dynamodb restore-table-to-point-in-time \
  --source-table-name <current-table> \
  --target-table-name <restored-table> \
  --use-latest-restorable-time
```

### Step 2: re-point the stage and redeploy

Lesser’s stage stacks normally create and wire their own DynamoDB table. “Re-pointing” is an explicit infrastructure
change (importing/aliasing a table) and is environment-specific.

Start here:

- `docs/operations/runbook.md` (triage + receipt usage)
- `infra/cdk/stacks/lesser_api_stack.go` (stage wiring)

## S3 (media/static assets)

S3 buckets are durable by default, but “oops deleted” recovery depends on your bucket configuration.

✅ CORRECT: enable versioning + lifecycle policies if you need recovery semantics for media.

❌ INCORRECT: assume you can always recover deleted objects without versioning.
