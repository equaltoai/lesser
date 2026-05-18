# Processor storm recovery preparation runbook

This runbook prepares and, when explicitly authorized for a dev/test instance, guides Project 34 M4 recovery work
after a processor failure storm. It records recovery decisions, evidence to capture, dry-run snapshot commands, and
execution gates for later backfill/reconciliation after the M1/M2/M5 fixes have been deployed and the M3 live gate
authorizes processor re-enable.

Do not use this runbook by itself as permission to mutate live state. Recovery execution requires an explicit
creator/Ops window and the per-instance gate in `docs/operations/processor-storm-runbook.md`.

## Hard boundaries

Unless creator/Ops explicitly authorizes a dev/test or production recovery window:

- do **not** deploy;
- do **not** mutate Lambda event-source mappings or EventBridge rules;
- do **not** re-enable or disable processors;
- do **not** purge, redrive, replay, backfill, or reconcile live data;
- do **not** read secret values; metadata-only Secrets Manager/KMS inspection is enough for prep;
- do **not** file framework issues from recovery evidence alone.

## Recovery preconditions for execution

Recovery may start only after all of these are recorded per instance. If any prerequisite is missing, keep that
surface in `deferred` state and record the dependency instead of running write-path recovery:

1. **Candidate deployed:** the deployed Lesser SHA/release includes the M1/M1.4 startup and routing fixes, M2 retry
   guardrails, and M5 explicit bootstrap / lazy-secret fixes.
2. **Guardrails verified:** stream processors have finite retry, finite max record age, partial-batch behavior where
   supported, and poison-record destinations; critical alarms exist for processor errors, iterator age/backlog,
   queue/DLQ/poison depth, and scheduled failures.
3. **M3 gate passed:** processors are re-enabled one at a time and the gated processor has stable errors,
   throttles, iterator age, queue depth, DLQ depth, and poison depth for the agreed observation window.
4. **Decision owner named:** an incident commander or Ops owner is named for stop/recontain decisions.
5. **Residual policy chosen:** creator/Ops has accepted whether stale or missing stream-only history can remain stale
   for any projection that cannot be exactly reconstructed.

## Recovery decision matrix

Create one row per surface per instance. Keep the completed matrix in the incident issue or operational packet.

| Instance | Surface | Recovery mode | Source of truth | Planned action | Dry-run evidence | Execution owner | Status | Residual stale / missing history | Verified by | Verified at |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `<instance>` | metrics | `backfill|reconcile|accept stale` | source tables + CloudWatch | `<planned command or manual procedure>` | `<snapshot path / query>` | `<owner>` | `planned|ready|executed|accepted stale` | `<rationale>` | `<operator>` | `<ISO-8601>` |
| `<instance>` | AI analysis | `backfill|reconcile|accept stale` | analyzable content + AI analysis rows | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` |
| `<instance>` | ML job state | `reconcile|accept stale` | Bedrock job state + ML metadata rows | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` |
| `<instance>` | severance | `backfill|reconcile|accept stale` | domain blocks + federation health + relationships | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` |
| `<instance>` | DLQ processing | `reprocess|accept retained|accept stale` | retained SQS DLQs + DLQ repository records | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` |
| `<instance>` | federation aggregation | `backfill|reconcile|accept stale` | persisted federation activity + route metrics | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` | `<...>` |

### Required state-matrix fields

Preserve these fields in the broader incident state matrix for every instance:

- account, region, stage/domain, deployed Lesser SHA/release;
- source-state baseline: stream mappings, SQS mappings, EventBridge rules, queue/DLQ/poison depths;
- startup-fix verification result;
- guardrail verification result;
- re-enable gate result per processor;
- recovery decision per derived surface;
- residual stale/missing-history rationale;
- final verifier and timestamp.

## Dry-run snapshot tooling

Use the read-only helper to collect the evidence needed to fill the matrix. The helper only calls AWS read APIs and
writes local files under `tmp/` by default.

```bash
scripts/processor_storm_recovery_snapshot.sh \
  --app simulacrum \
  --stage dev \
  --domain dev.simulacrum.greater.website \
  --profile Sim \
  --region us-east-1
```

Useful outputs:

- `summary.md` — operator-readable matrix inputs;
- `functions.json` — Lambda function metadata and `CodeSha256` values;
- `event-source-mappings.jsonl` — mapping state, retry/max-age/poison settings, and source ARNs;
- `eventbridge-rules.json` / `eventbridge-targets.jsonl` — scheduled source state;
- `sqs-attributes.jsonl` — queue, DLQ, and poison depth / redrive metadata;
- `tables/*.json` — table stream status;
- `secrets.json` / `kms-aliases.json` — metadata only, no secret values.

Attach `summary.md` and raw JSON outputs to the issue or incident packet before any live recovery execution.

## Surface recovery decisions

### Metrics projections

**Default decision:** reconcile from retained source tables and CloudWatch metrics where possible; accept stream-only
history gaps when DynamoDB Streams retention prevents exact replay.

Preparation checklist:

- Identify metric record and aggregate keys affected by the outage window.
- Capture CloudWatch `Invocations`, `Errors`, `Throttles`, `Duration`, and `IteratorAge` for the failing processors.
- Capture queue/DLQ/poison depth for SQS-backed metric paths.
- Determine whether the target aggregate can be recomputed from source-of-truth rows or only from lost stream
  records.
- Record one decision: `backfill`, `reconcile`, or `accept stale`.

Execution guardrails later:

- Run recomputation in dry-run/count mode first.
- Use idempotent upserts or versioned writes; never overwrite a newer aggregate with an older window.
- Stop if CloudWatch or poison queues show the metrics processor failing again.

### AI analysis

**Default decision:** replay analyzable content created or updated during the outage only after AI processor startup
and secret behavior are verified; accept gaps for records whose source content is gone or no longer analyzable.

Preparation checklist:

- Define the outage window per instance.
- Count candidate notes/objects that are eligible for AI analysis.
- Count existing AI analysis rows in the same window.
- Identify content that should not be replayed: deleted, private beyond policy, already superseded, or blocked by
  current moderation state.
- Record the replay decision and privacy/moderation constraints.

Execution guardrails later:

- Replay in small batches with current moderation/federation policy checks.
- Do not re-analyze content solely from stale stream images if the source object has been deleted or governance state
  now forbids analysis.
- Stop on AI processor errors, KMS/Secrets Manager errors, or queue/poison growth.

### ML job state

**Default decision:** reconcile existing ML training job rows and poll requests against Bedrock job state; do not
invent training outcomes from local state alone.

Preparation checklist:

- List `MLJOB#*`, `MLPOLL#*`, active model metadata, and effectiveness metric rows for the outage window.
- Identify in-progress or pending jobs that have no recent poll result.
- Prepare a Bedrock status-check list for the eventual execution window.
- Record whether each job will be reconciled, retried, or accepted stale.

Execution guardrails later:

- Query Bedrock status before writing local job state.
- Preserve terminal remote states even when local poll rows are stale.
- Do not restart training jobs unless creator/Ops explicitly authorizes model-training cost and data use.

### Severance state

**Default decision:** recompute or reconcile severance from current domain blocks, federation health, moderation
issue records, and relationship data; accept historical severance gaps if required source events were stream-only.

Preparation checklist:

- Capture current domain blocks and federation health records.
- Count active severance rows and affected relationship rows.
- Identify domains or actors whose relationship state changed during the outage window.
- Decide whether severance should be recomputed from current truth or accepted stale for historical-only cases.

Execution guardrails later:

- Prefer reconciliation that is monotonic with current moderation/federation policy.
- Do not reconnect or sever relationships without a named policy reason and audit trail.
- Stop on unexpected relationship-count deltas or severance processor errors.

### DLQ processing

**Default decision:** process retained DLQ messages only after idempotency is validated for each message class; accept
expired or missing messages as stale with rationale.

Preparation checklist:

- Capture every relevant queue and DLQ depth, redrive policy, oldest-message age, and retention setting.
- Group retained messages by source queue and error class without exposing sensitive payloads in the incident record.
- For each group, identify the idempotency key and the safe reprocessing target.
- Decide whether to reprocess, leave retained for manual inspection, or accept stale/missing history.

Execution guardrails later:

- Start with one message or a small bounded batch in dry-run/log-only mode where available.
- Never purge a DLQ as a recovery shortcut.
- Stop on repeat poison growth, duplicate writes, signature/federation delivery errors, or missing idempotency proof.

### Federation aggregation

**Default decision:** recompute hourly/daily/weekly rollups from persisted federation activity or route metrics after
routing/table fixes are deployed; accept gaps where the only source was lost scheduled invocations or expired queue
messages.

Preparation checklist:

- Capture federation aggregator queue depth, DLQ depth, scheduled rule state, and recent invocation/error metrics.
- Identify persisted federation activity, route metrics, and delivery outcomes for the outage window.
- Define rollup windows needing recomputation (hourly/daily/weekly) per instance.
- Record whether each window is backfilled, reconciled from current aggregates, or accepted stale.

Execution guardrails later:

- Recompute oldest-to-newest to avoid newer aggregate overwrite.
- Keep rollups additive/idempotent; do not double-count federation deliveries.
- Stop on aggregator errors, queue growth, or mismatched source/aggregate counts.

## Execution packet template

When creator/Ops authorizes dev/test or live recovery, create an execution packet using this shape:

```markdown
## Recovery execution packet

- Instance:
- Account / region:
- Stage / domain:
- Deployed Lesser SHA:
- M3 re-enable gate evidence:
- Guardrail evidence:
- Recovery owner:
- Stop/recontain owner:
- Window start / end:

### Surface decisions
| Surface | Mode | Dry-run evidence | Execution command/procedure | Stop criteria | Result | Residual stale rationale |
| --- | --- | --- | --- | --- | --- | --- |

### Post-run verification
- Lambda errors:
- Iterator age:
- Queue/DLQ/poison depth:
- Aggregate counts:
- Residual risks:
```

## Completion criteria

M4 is not complete until every active instance has a recorded decision for all six surfaces:

- metrics;
- AI analysis;
- ML job state;
- severance state;
- DLQ processing;
- federation aggregation.

Each decision must be one of `backfilled`, `reconciled`, or `accepted stale`, with evidence, residual rationale,
verifier, and timestamp. A successful deploy, a green processor gate, or an empty queue is not by itself a recovery
completion signal.
