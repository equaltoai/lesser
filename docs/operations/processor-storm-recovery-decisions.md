# Processor storm M4 recovery decisions

Date: 2026-05-18
Issues: [#993](https://github.com/equaltoai/lesser/issues/993),
[#1004](https://github.com/equaltoai/lesser/issues/1004),
[#1005](https://github.com/equaltoai/lesser/issues/1005)

This packet records the current Project 34 M4 recovery decisions for the dev-stage test instances after creator
clarified that dev/test backfill or reconciliation may run when safety preconditions are satisfied. It is a decision
record and dry-run evidence summary, not a production runbook receipt.

## Boundary and authorization state

Authorized for this packet:

- prepare recovery/backfill tooling, runbooks, and decision records;
- use AWS profiles `Sim` and `Theory` for dev-stage test evidence;
- run read-only/dry-run previews;
- perform dev-stage backfill or reconciliation only where the surface-specific safety preconditions are satisfied.

Not authorized or not performed in this packet:

- production resource access;
- deploys;
- source-mapping or EventBridge mutation;
- processor re-enable/disable;
- secret-value reads;
- framework issue filing;
- write-path backfill/reconciliation when M1/M2/M5 deployed-code prerequisites are not live.

## Current dev-stage evidence

Read-only snapshots were collected with `scripts/processor_storm_recovery_snapshot.sh`.

| Instance | Profile | Account | Stage/domain | Snapshot window | Stack last updated | Prerequisite status |
| --- | --- | ---: | --- | --- | --- | --- |
| `simulacrum-dev` | `Sim` | `209468748504` | `dev.simulacrum.greater.website` | `2026-05-18T10:09:04Z`–`2026-05-18T11:09:04Z` | `2026-05-17T23:56:38Z` | M1/M2/M5 fixes not deployed; active errors remain |
| `theory-dev` | `Theory` | `251050869245` | `dev.theory.greater.website` | `2026-05-18T10:13:18Z`–`2026-05-18T11:13:18Z` | `2026-05-17T23:56:33Z` | M1/M2/M5 fixes not deployed; active errors remain |

The candidate source commit for eventual deploy remains `e0f7f35997d1da70f2a08fc5d1b007a76d4bd495`, which includes
M1/M1.4/M2/M5 source-control fixes. Neither dev stack exposes a deployed Git SHA in the inspected metadata, but
both stack update times predate the M1/M2/M5 merged main commits.

### Processor health signals

| Instance | Processor | Invocations | Errors | Throttles | Max iterator age |
| --- | --- | ---: | ---: | ---: | ---: |
| `simulacrum-dev` | `metrics-processor` | 255 | 255 | 0 | 108,485,363 ms |
| `simulacrum-dev` | `ai-processor` | 1212 | 1212 | 0 | 21,063 ms |
| `simulacrum-dev` | `ml-training-processor` | 1213 | 1213 | 0 | 19,672 ms |
| `simulacrum-dev` | `severance-processor` | 1209 | 1209 | 0 | 22,424 ms |
| `simulacrum-dev` | `dlq-processor` | 12 | 12 | 0 | n/a |
| `simulacrum-dev` | `federation-aggregator` | 3 | 3 | 0 | n/a |
| `theory-dev` | `metrics-processor` | 238 | 238 | 1 | 99,556,093 ms |
| `theory-dev` | `ai-processor` | 8 | 8 | 0 | 5,579 ms |
| `theory-dev` | `ml-training-processor` | 8 | 8 | 0 | 4,534 ms |
| `theory-dev` | `severance-processor` | 8 | 8 | 0 | 8,319 ms |
| `theory-dev` | `dlq-processor` | 12 | 12 | 0 | n/a |
| `theory-dev` | `federation-aggregator` | 3 | 3 | 0 | n/a |

All relevant DynamoDB stream mappings are still enabled with pre-M2 guardrail posture in the snapshots: no
`OnFailure` destination, `maxAge=-1`, and some `retry=-1` mappings. This blocks write-path recovery for stream-derived
surfaces because recovery writes could be immediately re-poisoned or obscured by the still-failing processors.

SQS processor queues and DLQs in both snapshots had `visible=0`, `notVisible=0`, `delayed=0`, and
`oldestAgeSeconds=0`. That supports a no-op DLQ decision for retained SQS messages at the snapshot time, but it is
not proof that historical stream-only work was recovered.

## Surface decisions

| Surface | Current decision state | Source-of-truth inputs | Dry-run / preview evidence | Idempotency / rerun posture | Stop conditions | Residual rationale |
| --- | --- | --- | --- | --- | --- | --- |
| Metrics projections | **Deferred by prerequisite**; planned mode `backfill` or `reconcile` after fixes deploy | Source tables plus CloudWatch metrics / metric aggregate rows | Metrics processor is still 100% failing on both dev instances; stream mappings have no poison destination and unbounded age | Future recomputation must use idempotent upserts and never overwrite newer aggregate windows | Any metrics errors, iterator-age growth, poison/DLQ growth, or throttles after deploy | Cannot safely backfill now; exact stream-only history may later be accepted stale if source rows cannot reconstruct it |
| AI analysis | **Deferred by prerequisite**; planned mode `backfill` for eligible content or `accept stale` for deleted/ineligible content | Analyzable notes/objects plus AI analysis rows | AI processor is still 100% failing on both dev instances | Future replay must skip deleted/private/blocked/superseded content and be batch bounded | AI errors, KMS/Secrets errors, queue/poison growth, or moderation-policy mismatch | Replaying before startup/lazy-secret fixes are live would amplify the active storm |
| ML job state | **Deferred by prerequisite** for write reconciliation; read-only Bedrock/job-status planning allowed later | `MLJOB#*`, `MLPOLL#*`, model metadata rows, Bedrock job state | ML training processor is still 100% failing on both dev instances | Future reconciliation must preserve terminal Bedrock state and must not restart training without explicit cost/data authorization | Bedrock status mismatch, processor errors, throttles, or unexpected model-version changes | Local job rows cannot be trusted as sole source; remote Bedrock state must win during execution |
| Severance state | **Deferred by prerequisite**; planned mode `reconcile` from current policy and relationship truth | Domain blocks, federation health, issue records, relationships, severance rows | Severance processor is still 100% failing on both dev instances | Future reconciliation must be monotonic with current moderation/federation policy and audited | Unexpected relationship-count deltas or severance processor errors | Historical stream-only severance gaps may be accepted stale if current source truth cannot reconstruct them |
| DLQ processing | **Accepted no-op for retained SQS DLQ messages at snapshot time**; rerun snapshot before any later redrive | SQS queues/DLQs, DLQ repository records where present | All observed SQS queues/DLQs had zero visible, not-visible, delayed, and oldest-message age values | No purge/redrive was run; if messages appear later, group by source/error class and prove idempotency first | Any nonzero DLQ/poison depth without idempotency proof, duplicate writes, or repeated poison growth | Empty retained queues mean there was no safe SQS redrive work to perform now; stream-only loss remains separate |
| Federation aggregation | **Deferred by prerequisite**; planned mode `backfill` or `reconcile` after routing/table fixes deploy | Persisted federation activity, route metrics, delivery outcomes, federation rollup rows | Federation aggregator still errors on scheduled/queue invocations; queues are empty at snapshot time | Future recompute must be oldest-to-newest and idempotent to avoid double-counting deliveries | Aggregator errors, queue growth, poison/DLQ growth, or mismatched source/aggregate counts | Lost scheduled invocations or expired queue messages may require accepted-stale windows |

## Per-instance completion matrix fields

Carry these fields forward when recovery actually runs:

| Instance | Surface | Mode | Target profile | Source inputs | Preview counts | Write execution | Completion state | Residual stale rationale | Stop owner | Verified at |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `simulacrum-dev` | metrics | deferred | `Sim` | source tables + CloudWatch | see snapshot | none | prerequisite blocked | stream-only gaps possible | `<owner>` | `<pending>` |
| `simulacrum-dev` | AI analysis | deferred | `Sim` | content + analysis rows | see snapshot | none | prerequisite blocked | deleted/ineligible content may remain stale | `<owner>` | `<pending>` |
| `simulacrum-dev` | ML job state | deferred | `Sim` | ML rows + Bedrock state | see snapshot | none | prerequisite blocked | remote state must be reconciled later | `<owner>` | `<pending>` |
| `simulacrum-dev` | severance | deferred | `Sim` | policy + relationship truth | see snapshot | none | prerequisite blocked | historical stream-only gaps possible | `<owner>` | `<pending>` |
| `simulacrum-dev` | DLQ processing | accept no-op | `Sim` | SQS/DLQ depths | zero retained messages | none | accepted stale/no-op at snapshot | rerun before later recovery | `<owner>` | `<pending>` |
| `simulacrum-dev` | federation aggregation | deferred | `Sim` | federation activity + metrics | see snapshot | none | prerequisite blocked | scheduled/expired gaps possible | `<owner>` | `<pending>` |
| `theory-dev` | metrics | deferred | `Theory` | source tables + CloudWatch | see snapshot | none | prerequisite blocked | stream-only gaps possible | `<owner>` | `<pending>` |
| `theory-dev` | AI analysis | deferred | `Theory` | content + analysis rows | see snapshot | none | prerequisite blocked | deleted/ineligible content may remain stale | `<owner>` | `<pending>` |
| `theory-dev` | ML job state | deferred | `Theory` | ML rows + Bedrock state | see snapshot | none | prerequisite blocked | remote state must be reconciled later | `<owner>` | `<pending>` |
| `theory-dev` | severance | deferred | `Theory` | policy + relationship truth | see snapshot | none | prerequisite blocked | historical stream-only gaps possible | `<owner>` | `<pending>` |
| `theory-dev` | DLQ processing | accept no-op | `Theory` | SQS/DLQ depths | zero retained messages | none | accepted stale/no-op at snapshot | rerun before later recovery | `<owner>` | `<pending>` |
| `theory-dev` | federation aggregation | deferred | `Theory` | federation activity + metrics | see snapshot | none | prerequisite blocked | scheduled/expired gaps possible | `<owner>` | `<pending>` |

## Verdict

The M4 tooling and decision record can land now. Write-path recovery is still blocked for metrics, AI analysis, ML job
state, severance state, and federation aggregation because the dev instances are still running pre-M1/M2/M5 artifacts
and their processors are actively failing. DLQ processing has no retained SQS messages to redrive at the snapshot
time, so the current safe decision is no-op/accepted stale for retained SQS DLQ work, with a required fresh snapshot
before any future redrive.
