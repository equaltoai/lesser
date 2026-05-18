# Processor storm containment runbook

This runbook is for a live Lambda processor failure storm where derived-data processors are repeatedly failing on
stream, queue, or scheduled inputs. It is intentionally lightweight for the current two-instance operating shape:
keep the state explicit in a matrix instead of building fleet orchestration before the incident is contained.

Use this with deployment receipts from `~/.lesser/<app>/<base-domain>/state.json` and AWS account/region context
from the operator. Do not use hard-coded table, queue, or function names when a receipt or CloudFormation output is
available.

## Scope

This runbook covers the incident-control sequence for these failing sources, in containment order:

1. `metrics-processor` DynamoDB stream mapping
2. `ai-processor` DynamoDB stream mapping
3. `ml-training-processor` DynamoDB stream mapping
4. `severance-processor` DynamoDB stream mapping
5. `dlq-processor` SQS or scheduled routes
6. `federation-aggregator` EventBridge or SQS routes

Containment pauses derived features. It does not delete data, delete event-source mappings, purge queues, drop
tables, change signing keys, or destroy stacks. Destructive remediation requires a separate, explicit operator
decision.

## Two-instance state matrix

Create one row per active instance before changing any source state. Keep this matrix in the incident ticket, PR, or
operational packet so every reviewer can see which instance is contained, patched, guarded, re-enabled, and
recovered.

| Instance | Account | Region | Stage/domain | Deployed Lesser SHA | Failing sources disabled/throttled | Iterator age / backlog | Patch deployed | Guardrails deployed | Processors re-enabled | Backfill / recovery state | Residual risks | Last verified by | Last verified at |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Instance A | `<account>` | `<region>` | `<stage-domain>` | `<sha>` | `none|partial|all`; list sources | stream age, queue depth, DLQ depth | `no|sha` | `no|sha` | `none|partial|all`; list sources | `not started|in progress|complete|accepted stale`; note projections | blockers, stale derived data, unknowns | `<operator>` | `<ISO-8601>` |
| Instance B | `<account>` | `<region>` | `<stage-domain>` | `<sha>` | `none|partial|all`; list sources | stream age, queue depth, DLQ depth | `no|sha` | `no|sha` | `none|partial|all`; list sources | `not started|in progress|complete|accepted stale`; note projections | blockers, stale derived data, unknowns | `<operator>` | `<ISO-8601>` |

### Required matrix fields

- **Instance / account / region / stage-domain**: identify the deployment unambiguously.
- **Deployed Lesser SHA**: use the release manifest, deployment receipt, or deployed Lambda version metadata. If
  uncertain, record `unknown` and who is resolving it.
- **Failing sources disabled/throttled**: name every contained source. Do not write `done` without the source list.
- **Iterator age / backlog**: include stream iterator age, relevant queue depths, DLQ depth, and whether the value
  is pre- or post-containment.
- **Patch deployed**: record the exact Lesser SHA or release tag that contains processor startup/routing fixes.
- **Guardrails deployed**: record the exact SHA/tag that adds finite retry, max record age, poison-record
  destination, and minimum alarms.
- **Processors re-enabled**: list processors and verification result; never roll up as `all` unless the list is also
  present.
- **Backfill / recovery state**: record one decision per derived feature: backfilled, reconciled, or accepted stale
  with rationale.
- **Residual risks**: name open unknowns, stale projections, or missing evidence.
- **Last verified by / at**: every state transition must identify the operator or agent that verified it and an
  ISO-8601 timestamp.

## Snapshot checklist

Capture snapshots before containment and again after every source-state change. Attach the raw command output or
CloudWatch links to the incident record.

| Snapshot | Evidence to preserve |
| --- | --- |
| Lambda failures | Errors, invocations, throttles, and duration for each failing processor |
| Stream state | Iterator age and event-source mapping state for stream processors |
| Queue state | Visible, not-visible, delayed, oldest-message age, and DLQ depth for SQS-backed processors |
| Schedule state | EventBridge rule enabled/disabled state and recent failed invocations |
| Cost amplifiers | Secrets Manager and KMS request rates during the storm window |
| Deploy state | Deployed Lesser SHA/release, stack outputs, and relevant Lambda aliases/versions |

## Runbook order

Execute the sequence for one instance at a time unless the incident commander explicitly chooses parallel containment.

### 1. Contain failing sources

1. Capture the pre-containment snapshot.
2. Disable or throttle sources in the required order:
   1. `metrics-processor`
   2. `ai-processor`
   3. `ml-training-processor`
   4. `severance-processor`
   5. `dlq-processor`
   6. `federation-aggregator`
3. Capture the post-containment snapshot.
4. Update the matrix with the exact contained sources, backlog/iterator age, residual risks, verifier, and timestamp.

Do not re-enable `metrics-processor` until finite retry, max record age, and a poison-record destination are
deployed for its stream mapping.

### 2. Deploy the active processor patch

Deploy the focused processor repair through the normal `./lesser up` stage path for the target instance. The patch
phase should fix active startup/routing failures without bundling a broad storage-bootstrap rewrite.

Evidence to record:

- release tag or commit SHA deployed;
- stage and stack outputs used;
- processor startup logs showing fail-closed startup behavior rather than per-record nil panics;
- matrix `Patch deployed` update with verifier and timestamp.

### 3. Deploy retry guardrails and minimum alarms

Before broad re-enable, deploy guardrails through the same stage path:

- finite stream retry attempts;
- finite max record age;
- batch bisect or partial-batch behavior only where handler semantics support it;
- poison-record destination for derived stream processors;
- minimum processor alarms for errors, iterator age, queue age/depth, DLQ depth, and scheduled failures.

Evidence to record:

- synthesized or live event-source mapping output proving retry/max-age/poison destination state;
- CloudWatch alarm list for the target instance;
- matrix `Guardrails deployed` update with verifier and timestamp.

### 4. Re-enable one processor at a time

Re-enable only one source, then wait for its health evidence before moving to the next. Use the same processor
order as containment unless the incident commander documents a different reason.

For each processor:

1. Enable or unthrottle the source.
2. Watch Lambda errors, iterator age/backlog, queue depth, DLQ depth, and relevant logs.
3. Record PASS/FAIL, residual risks, verifier, and timestamp in the matrix.
4. If the processor fails, contain it again and do not proceed to the next processor until the incident commander
   records the decision.

### 5. Backfill or reconcile derived data

For every derived feature paused during containment, record one recovery decision:

- **Backfilled** from source-of-truth records.
- **Reconciled** against current service/job state.
- **Accepted stale** with a user/operator-facing rationale.

Minimum projections to evaluate during this storm class:

- metrics projections;
- AI analysis results;
- ML training jobs;
- severance state;
- DLQ processing outcomes;
- federation aggregation rollups.

### 6. Repeat for the second instance

After Instance A is contained, patched, guarded, re-enabled, and recovered or has accepted residuals, repeat the same
sequence for Instance B. If both instances are handled in parallel, keep separate matrix rows and do not copy state
between them without fresh verification.

## Completion criteria

The incident is not complete until the matrix shows, for every active instance:

- failing sources are either re-enabled with guardrails or intentionally disabled with a named residual risk;
- deployed Lesser SHA is known;
- iterator age/backlog is stable, draining, or explicitly explained;
- patch and guardrail deployment state is recorded;
- derived feature recovery decisions are recorded;
- residual risks are named;
- `Last verified by` and `Last verified at` are filled for every row.
