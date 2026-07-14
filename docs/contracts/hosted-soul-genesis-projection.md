# Hosted Soul Genesis GraphQL Projection Table

Date: 2026-06-21
Repo: `equaltoai/lesser`
Project: Project 49 — Durable Async Hosted Soul Genesis
Milestone: M1.2 — Lock Lesser GraphQL projection table for hosted genesis statuses
Tracking issue: `equaltoai/lesser#1173`
Parent epic: `equaltoai/lesser-host#737`
Host dependency: `equaltoai/lesser-host#730` (M1.1)

This contract locks how Lesser projects Host durable hosted-genesis state into the Lesser GraphQL soul bootstrap surface.
It exists to prevent the observed live failure where Host returned `200 + in_progress` with a durable conversation id, but
Lesser collapsed that progress into `phase=ERROR`, `state=error.host_unavailable`, `HOST_RESPONSE_INVALID`, and lost the
Host conversation id.

M1.2 is a contract/spec lock only. It does **not** implement the M3 resolver/state-machine, mutate Host/Greater/Sim, deploy,
or change publish-path runtime behavior.

## Source and Host alignment

Authoritative project brief: `/home/aron/ai-workspace/codebases/equaltoai/agents/arch/projects/project49-async-hosted-genesis/contract-and-brief.md`.

Host M1.1 owns final Host JSON field/status names. As of the M1.2 lock on 2026-06-21, no open Host M1.1 PR was observed;
therefore this Lesser table locks to the shared names from the Project 49 brief:

- `in_progress`
- `assistant_turn_ready`
- `declaration_extraction_pending`
- `declaration_ready`
- `failed`
- `published` / `bound`

If Host M1.1 lands different names, M3 must reconcile this table and fixture before implementing resolver behavior.
No Lesser implementation should start against an unstated Host status.

### Project 51 M5 Host v1.0.6 producer alignment

Project 51 M5 updates the runtime producer contract against the public Host release `lesser-host@v1.0.6` and keeps
Lesser consuming Host's AppTheory-backed `HostedGenesisSession` projection as the source of truth. AppTheory MicroVM
registry/lifecycle state remains execution/cache state only; Lesser does not create browser-authored retry state, local
MicroVM recovery substitutes, or LocalStorage/sessionStorage recovery authority.

The Host `v1.0.6` sources inspected for this alignment are:

- `docs/contracts/hosted-genesis-conversation.md`
- `docs/spec/v3/schemas/hosted-genesis.conversation.response.schema.json`
- `docs/spec/v3/fixtures/hosted-genesis.conversation.in-progress.example.json`
- `docs/spec/v3/fixtures/hosted-genesis.conversation.completed-declaration-ready.example.json`
- `docs/spec/v3/fixtures/hosted-genesis.conversation.failed.example.json`
- `docs/runbooks/hosted-genesis-sqs-demotion.md`
- `docs/adr/0009-app-theory-microvm-lab-gates.md`

M5 locks these Lesser-side producer rules:

1. `conversation.conversation_id` is persisted as soon as Host returns it, including `HTTP 202` / `status=in_progress`.
2. Host `failure.recovery.action` is the only recovery-truth input. Lesser projects the locked actions
   `refresh_state`, `retry_same_step`, `restart_soul_bootstrap`, and `operator_action` into bounded GraphQL
   `typedNextAction` / `recoveryCategory` / `recoveryAction` values. Host `v1.0.6` does not expose a public
   MicroVM resume/restart recovery action; Lesser must not invent one. Failed Host snapshots that omit a locked
   recovery action are treated as invalid Host responses rather than deriving retry/restart behavior locally.
3. Publish/finalize readiness requires `status=declaration_ready` plus active-conversation Host declaration evidence.
   The Host `produced_declarations` envelope is validated for `source=host_conversation`, registration id,
   conversation id, agent id, message count, request id, declaration id/hash, produced timestamp, and declaration
   shape before Lesser stores a compact declaration checkpoint. For hosted instance-trust genesis, Lesser then drives
   the guarded Host publish + local soul/body bind path from that terminal evidence; it must not strand a valid
   declaration at `PUBLISH_HOSTED_SOUL` when no extra user proof is required.
4. Browser-visible GraphQL state remains sanitized: no Host bearer tokens, Instance API keys, raw Host routes,
   MicroVM endpoints/tokens, provider keys/responses, wallet signatures, SSM values, raw transcripts, or raw provider
   responses are exposed.

## Existing GraphQL operations in scope

The hosted operations already exist in `docs/contracts/graphql-schema.graphql` and are the only operations this table
projects for:

- `startHostedSoulBootstrap`
- `sendHostedSoulGenesisMessage`
- `completeHostedSoulGenesis`
- `publishHostedSoul`
- `restartSoulBootstrap`

## Locked projection fields

The published GraphQL schema currently exposes the core projection through `SoulBootstrapState`:
`hostRegistrationId`, `hostSoulAgentId`, `hostConversationId`, `phase`, `state`, `typedNextAction`, `recoveryCategory`,
`recoveryAction`, `signingCheckpoints`, `publication`, `correlation`, and `lastHostRequestId`.

M1.2 also locks the compact M3 names that client adapters can prepare for without changing this milestone's runtime:

| Persisted Lesser field | Current GraphQL projection | Compact M3 projection name | Contract rule |
| --- | --- | --- | --- |
| `host_registration_id` | `hostRegistrationId` | `hostRegistrationId` | Persist once Host registration exists. |
| `host_soul_agent_id` | `hostSoulAgentId` | `hostSoulAgentId` | Persist with Host registration. |
| `host_conversation_id` | `hostConversationId` | `hostConversationId` | Persist early before returning any post-conversation next action. |
| `phase` | `phase` | `phase` | Canonical Lesser phase from this table. |
| `status` | `state` | `hostConversationStatus` | `state` remains string-compatible; compact Host status may be added additively in M3. |
| `next_action` | `typedNextAction` | `typedNextAction` | Lesser authors exactly one next action per response. |
| `recovery_category` | `recoveryCategory` | `recoveryCategory` | Typed bounded recovery only. |
| `recovery_action` | `recoveryAction` | `recoveryAction` | Typed bounded recovery only. |
| `terminal_declaration_evidence` | `signingCheckpoints` entry `hosted_conversation/completed` | `terminalDeclarationEvidence` | Must bind the active `hostConversationId` before publish. |
| `publication_evidence` | `publication` | `publicationEvidence` | Present only after Host publication and Lesser binding. |
| correlation/request ids | `correlation`, `lastHostRequestId` | same names | Diagnostics/idempotency only; request ids are not terminal evidence. |
| restart supersession ids | `correlation.supersededHostRegistrationId`, `correlation.supersededHostConversationId` | same names | Restart supersedes prior ids instead of mutating history. |

## Projection invariants

1. `in_progress` is progress: it never maps to `HOST_RESPONSE_INVALID`, `error.host_unavailable`, or `ERROR`.
2. Whenever Host supplies `conversation_id`, Lesser persists `host_conversation_id` before returning any next action.
3. Every response has exactly one server-authored `typedNextAction`; UI clients do not infer hidden Host state.
4. `PUBLISH_HOSTED_SOUL` is impossible without active-conversation terminal declaration evidence. Hosted instance-trust
   refresh/complete paths should immediately consume that evidence to publish and bind; the explicit action remains a
   guarded retry/compatibility entry point, not the normal terminal state.
5. Recovery is typed and bounded: refresh/poll, retry same step, restart with supersession ids, or operator action.
6. Restart creates a new attempt and records superseded Host ids; it does not mutate historical Host state.
7. HTTP status is transport state only. `200`/`202` is not terminal success without explicit Host status + evidence.

## Canonical Host status projection table

The machine-readable fixture with full examples for every row is checked in at
`docs/contracts/examples/hosted-soul-genesis-projection-table.example.json`.

| Host status | Lesser `phase` | Lesser `state` | Allowed `typedNextAction` values | Example action | Recovery | Host ids | Terminal declaration evidence | Publish gate |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| no registration | `NOT_STARTED` | `not_started` | `START_HOSTED_BOOTSTRAP` | `START_HOSTED_BOOTSTRAP` | - | none | absent | blocked: no Host registration |
| registration active, no conversation | `CONVERSATION` | `conversation.registration_active` | `SEND_HOSTED_SOUL_GENESIS_MESSAGE` | `SEND_HOSTED_SOUL_GENESIS_MESSAGE` | - | `hostRegistrationId`, `hostSoulAgentId`; no conversation yet | absent | blocked: no Host conversation |
| `in_progress` | `CONVERSATION` | `conversation.in_progress` | `REFRESH_STATE` or `SEND_HOSTED_SOUL_GENESIS_MESSAGE` | `REFRESH_STATE` | poll via `REFRESH_STATE` | registration id, soul agent id, and conversation id required; conversation id persists early | absent | blocked: conversation in progress |
| `assistant_turn_ready` | `CONVERSATION` | `conversation.assistant_turn_ready` | `SEND_HOSTED_SOUL_GENESIS_MESSAGE` or `COMPLETE_HOSTED_SOUL_GENESIS` | `COMPLETE_HOSTED_SOUL_GENESIS` | - | registration id, soul agent id, and conversation id required | absent | blocked: terminal declaration evidence absent |
| `declaration_extraction_pending` | `CONVERSATION` | `conversation.declaration_extraction_pending` | `REFRESH_STATE` | `REFRESH_STATE` | poll via `REFRESH_STATE` | registration id, soul agent id, and conversation id required | absent | blocked: extraction pending |
| `declaration_ready` | `COMPLETE` after guarded auto publish/bind; `FINALIZE` only transiently before publish | `complete.bound` after publish/bind; `conversation.declaration_ready` only transiently before publish | `COMPLETE` after auto publish/bind; explicit `PUBLISH_HOSTED_SOUL` remains a guarded retry/compat action | `COMPLETE` | - | registration id, soul agent id, and conversation id required | present and bound to active conversation | complete after publish/bind |
| `failed` | `ERROR` | `error.host_failed` | `RETRY_SAME_STEP` or `RESTART_SOUL_BOOTSTRAP` | `RETRY_SAME_STEP` | typed/bounded | ids preserved if Host supplied them; restart records superseded ids | absent | blocked: Host failure |
| `published` / `bound` (published/bound) | `COMPLETE` | `complete.bound` | `COMPLETE` | `COMPLETE` | - | active ids retained for diagnosis | terminal evidence retained | complete: already published and bound |

## Per-row examples

The examples below are abridged GraphQL projection excerpts. The fixture contains the corresponding Host envelope,
correlation/request ids, evidence payloads, and publication evidence.

### no registration

```json
{
  "hostRegistrationId": null,
  "hostSoulAgentId": null,
  "hostConversationId": null,
  "phase": "NOT_STARTED",
  "state": "not_started",
  "typedNextAction": "START_HOSTED_BOOTSTRAP",
  "recoveryCategory": null,
  "recoveryAction": null,
  "terminalDeclarationEvidence": null,
  "publication": null,
  "publishGate": { "canPublishHostedSoul": false, "reason": "blocked:no_host_registration" }
}
```

### registration active, no conversation

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": null,
  "phase": "CONVERSATION",
  "state": "conversation.registration_active",
  "typedNextAction": "SEND_HOSTED_SOUL_GENESIS_MESSAGE",
  "terminalDeclarationEvidence": null,
  "publishGate": { "canPublishHostedSoul": false, "reason": "blocked:no_host_conversation" }
}
```

### in_progress

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": "hconv_example_001",
  "phase": "CONVERSATION",
  "state": "conversation.in_progress",
  "typedNextAction": "REFRESH_STATE",
  "recoveryCategory": "REFRESH_STATE",
  "recoveryAction": "REFRESH_STATE",
  "lastHostRequestId": "host-req-turn-001",
  "terminalDeclarationEvidence": null,
  "publishGate": { "canPublishHostedSoul": false, "reason": "blocked:conversation_in_progress" }
}
```

### assistant_turn_ready

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": "hconv_example_001",
  "phase": "CONVERSATION",
  "state": "conversation.assistant_turn_ready",
  "typedNextAction": "COMPLETE_HOSTED_SOUL_GENESIS",
  "terminalDeclarationEvidence": null,
  "publishGate": { "canPublishHostedSoul": false, "reason": "blocked:terminal_declaration_evidence_absent" }
}
```

### declaration_extraction_pending

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": "hconv_example_001",
  "phase": "CONVERSATION",
  "state": "conversation.declaration_extraction_pending",
  "typedNextAction": "REFRESH_STATE",
  "recoveryCategory": "REFRESH_STATE",
  "recoveryAction": "REFRESH_STATE",
  "terminalDeclarationEvidence": null,
  "publishGate": { "canPublishHostedSoul": false, "reason": "blocked:declaration_extraction_pending" }
}
```

### declaration_ready

For hosted instance-trust genesis, `declaration_ready` is terminal evidence that Lesser immediately consumes through the
Host publish route and local soul/body binding. A `PUBLISH_HOSTED_SOUL` state may exist only transiently or as an explicit
retry/compatibility path; the normal refreshed projection is complete and bound.

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": "hconv_example_001",
  "phase": "COMPLETE",
  "state": "complete.bound",
  "typedNextAction": "COMPLETE",
  "terminalDeclarationEvidence": {
    "conversationId": "hconv_example_001",
    "hostStatus": "declaration_ready",
    "hostRequestId": "host-req-complete-002",
    "declarationsHash": "sha256:7af7a1c0f7c74c872a3a8a0a3e0af49a4a7c7f1f9b59f6827b1849d5f3f3a001"
  },
  "publication": { "agentId": "hsoul_example_001", "publishedVersion": 1, "authorityModel": "INSTANCE_TRUST" },
  "publishGate": { "canPublishHostedSoul": false, "reason": "complete:already_published_bound" }
}
```

### failed

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": "hconv_example_001",
  "phase": "ERROR",
  "state": "error.host_failed",
  "typedNextAction": "RETRY_SAME_STEP",
  "recoveryCategory": "RETRY_SAME_STEP",
  "recoveryAction": "RETRY_SAME_STEP",
  "error": { "code": "HOST_CONVERSATION_FAILED", "source": "host", "retryable": true },
  "terminalDeclarationEvidence": null,
  "publishGate": { "canPublishHostedSoul": false, "reason": "blocked:host_failure" }
}
```

### published / bound (published/bound)

```json
{
  "hostRegistrationId": "hreg_example_001",
  "hostSoulAgentId": "hsoul_example_001",
  "hostConversationId": "hconv_example_001",
  "phase": "COMPLETE",
  "state": "complete.bound",
  "typedNextAction": "COMPLETE",
  "terminalDeclarationEvidence": { "conversationId": "hconv_example_001", "hostStatus": "declaration_ready" },
  "publication": { "agentId": "hsoul_example_001", "publishedVersion": 1, "authorityModel": "INSTANCE_TRUST" },
  "publishGate": { "canPublishHostedSoul": false, "reason": "complete:already_published_bound" }
}
```

## Contract audit

- Mastodon REST: no impact; no `/api/v1/*` shape changes.
- GraphQL: no schema regeneration in M1.2; this is a file-only projection contract and fixture. Future M3 schema additions
  must be additive and regenerated through `./lesser schema`.
- ActivityPub / federation: no actor, inbox/outbox, signing, WebFinger, object, collection, or delivery changes.
- DynamoDB schema: no PK/SK/GSI/TableTheory-tag changes in M1.2.
- Sibling consumers: Greater M1.3 and Sim must be able to represent every row before M3 resolver work ships.

## P52 — MicroVM-only genesis: accept-then-poll transport + turn-state surface

Project 52 (parent `equaltoai/lesser#1213`; host dependency `lesser-host#867` H1.2) makes the genesis
turn a MicroVM-dispatched async operation. Host returns `202 Accepted-Pending` and processes the turn
asynchronously; Lesser's transport and state machine honor a 202-then-poll contract. This section is the
contract snapshot Greater consumes to sync soul-bootstrap adapters for the turn-state surface and accept
semantics. It is additive to the M1.2 projection table above — no GraphQL schema change, no status-vocabulary
change, no DynamoDB schema change.

The machine-readable fixture for this section is checked in at
`docs/contracts/examples/hosted-soul-genesis-accept-semantics.example.json`.

### Accept semantics (transport)

`sendHostedSoulGenesisMessage` relays one mint-conversation turn to Host's durable instance-key route.
The HTTP status is transport state only:

| Host HTTP status | Lesser transport behavior | Lesser result status | Inline assistant messages | Conversation id |
| --- | --- | --- | --- | --- |
| `202 Accepted` | Accepted-pending. The body is **not** parsed as a full conversation snapshot. | `in_progress` (canonical pending status) | None — `messages` is empty, `messageCount` is 0. | Persisted early: extracted best-effort from a minimal body (top-level or nested `conversation`), else the caller's existing id. |
| `200 OK` | Synchronous snapshot (legacy / non-MicroVM path). Parsed and validated as a full snapshot. | Host-authored status from the snapshot. | Projected from the snapshot when Host supplies them. | From the snapshot. |

The send POST is bounded by a short **accept timeout** (`defaultSoulBootstrapAcceptTimeout`, <2s), sized for
Host's 202 accept — not for the turn to complete. Discovery and read/poll HTTP calls keep the longer
`defaultSoulHTTPTimeout` (10s). A send that does not receive Host's accept in time surfaces `HOST_UNAVAILABLE`.

### Turn-state surface (state machine)

Pending turn statuses route the client to **poll** (`REFRESH_STATE`), not to re-send. This realigns the
resolver with the M1.2 projection table; the code had drifted to `SEND` for pending statuses.

| Lesser `state` | Host status | `typedNextAction` | `availableActions` | Notes |
| --- | --- | --- | --- | --- |
| `conversation.in_progress` | `in_progress` / `created` | `REFRESH_STATE` | `[REFRESH_STATE, SEND_HOSTED_SOUL_GENESIS_MESSAGE]` | Pending turn. Poll is primary; SEND remains an allowed alternative. |
| `conversation.declaration_extraction_pending` | `declaration_extraction_pending` | `REFRESH_STATE` | `[REFRESH_STATE]` | Declaration extraction pending. Poll only — do not send again. |
| `conversation.assistant_turn_ready` | `assistant_turn_ready` | `COMPLETE_HOSTED_SOUL_GENESIS` | `[SEND_HOSTED_SOUL_GENESIS_MESSAGE, COMPLETE_HOSTED_SOUL_GENESIS]` | Unchanged. |
| `conversation.registration_active` | registration active, no conversation | `SEND_HOSTED_SOUL_GENESIS_MESSAGE` | `[SEND_HOSTED_SOUL_GENESIS_MESSAGE]` | Unchanged — no conversation yet. |

### Error routing (accept timeout)

An accept timeout (`HOST_UNAVAILABLE`) on the send POST routes the client to **poll** (`REFRESH_STATE`), never
to `RETRY_SAME_STEP`. `RETRY_SAME_STEP` would instruct the client to re-issue the same blocking send — the
binding constraint this project removes. Host may have accepted the turn and be processing it asynchronously;
the safe forward motion is to poll, which reconciles via the read path and recovers the conversation id. A
genuine Host-authored `failed` conversation with a `retry_same_step` recovery action still routes to
`RETRY_SAME_STEP` (the `failed` row of the M1.2 table) — the timeout→poll change is scoped to
`HOST_UNAVAILABLE`.

### Observability (no silent rewrites)

- An empty/unknown Host status is not silently coerced to `in_progress`; the existing state's status is
  preserved, falling back to `in_progress` only for a brand-new conversation.
- Reconcile logs Host read errors and service-resolution errors (best-effort, non-fatal) instead of silently
  swallowing them.
- The errored→send retry repair is logged at both call sites; it is only reachable for a genuine
  Host-authored `retry_same_step`, not for accept timeouts.

### Greater sync notes

- No GraphQL schema change in P52; Greater adapters consume the existing `SoulBootstrapState` and
  `SoulBootstrapHostedGenesisConversation` surfaces. The turn-state routing changes are observable through
  `typedNextAction` / `availableActions` values, which are already in the `SoulBootstrapNextAction` enum.
- Greater's step (G4) is to update soul-bootstrap adapters so a pending turn (`in_progress`,
  `declaration_extraction_pending`) drives a poll/refresh UX rather than an immediate re-send, and so an
  accept-timeout error surfaces as "polling state" rather than "re-send". Lesser only produces this snapshot.
- Integration proof (send returns accepted in <2s while a >30s turn completes via poll) is BLOCKED on
  `lesser-host#867` H1.2 lab evidence; the unit proof ships with this snapshot.

## P52 contract audit (additive)

- Mastodon REST: no impact; no `/api/v1/*` shape changes.
- GraphQL: no schema change; `SoulBootstrapNextAction` already includes `REFRESH_STATE`. No regeneration.
- ActivityPub / federation: no actor, inbox/outbox, signing, WebFinger, object, collection, or delivery changes.
- DynamoDB schema: no PK/SK/GSI/TableTheory-tag changes.
- Sibling consumers: Greater consumes this accept-semantics snapshot to sync soul-bootstrap adapters (G4).

