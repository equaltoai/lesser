# Status Create Entry-Point Inventory

Date: 2026-03-27

This inventory freezes the first-party production paths that create `Status` rows, the repository/service boundary each path uses today, and the side effects that must stay attached to canonical status creation.

## Required canonical side effects

- always included in canonical status creation:
  - persist the `Status` row with the fully materialized note payload and derived index attributes
- best-effort after the row exists:
  - bump instance `TOTAL_STATUSES`
  - bump instance `LOCAL_COMMENTS` for local replies
  - canonicalize index attributes on the stored status row
  - create supplemental hashtag timeline index rows

These behaviors are implemented in `pkg/storage/repositories/status_repository.go` and are the invariants S0 must preserve across every note type.

## Public note create

- Entry point:
  - `pkg/services/notes/service.go`
  - `(*Service).CreateNote`
- Current status-row owner:
  - `StatusRepository.CreateStatus`
- Status-create shape:
  - canonical repository create
- Notes:
  - this is the baseline first-party note path for public, unlisted, private, and direct note creation outside DM conversation orchestration

## DM create-and-send (new thread)

- Entry point:
  - `pkg/services/conversations/service.go`
  - `(*Service).SendDirectMessage`
- Current status-row owner before S0 unification:
  - `ConversationRepository.ApplyDirectMessageSend`
- Status-create shape before S0 unification:
  - raw transaction-owned `tx.Create(status)` inside `pkg/storage/repositories/conversation_send_repository.go`
- Notes:
  - this is the path that currently fails on live `Note.Context` values because it bypasses the canonical status repository contract

## DM send-to-existing-thread

- Entry point:
  - `pkg/services/conversations/service.go`
  - `(*Service).SendMessage`
- Current status-row owner before S0 unification:
  - `ConversationRepository.ApplyDirectMessageSend`
- Status-create shape before S0 unification:
  - raw transaction-owned `tx.Create(status)` inside `pkg/storage/repositories/conversation_send_repository.go`
- Notes:
  - shares the same alternate status persistence path as new-thread DM send
  - must keep optimistic-lock and retry semantics when status creation moves back under canonical ownership

## Boost wrapper create

- Entry point:
  - `pkg/services/notes/service.go`
  - boost persistence path via `(*Service).persistBoostStatus`
- Current status-row owner:
  - `StatusRepository.CreateBoostStatus`
- Status-create shape:
  - canonical repository create, delegated to `CreateStatus`

## Import / backfill paths

- Current finding:
  - no first-party migration or backfill command was found that creates live `Status` rows directly
- Notes:
  - current DM and account migrations backfill compatibility rows, canonical DM state, governance state, or metadata
  - status-indexer and analytics tooling create secondary index/analytics records, not primary `Status` rows

## Remaining first-party status-create paths

### Quote service

- Entry point:
  - `pkg/services/quotes/quote_service.go`
  - `(*QuoteService).createQuoteStatus`
- Current status-row owner:
  - `StatusRepository.CreateStatus`
- Status-create shape:
  - canonical repository create
- Notes:
  - GraphQL `createQuoteNote` currently routes through the notes service, but the dedicated quote service remains a first-party runtime caller of the canonical create contract

## Explicit non-entry-points

- `pkg/storage/repositories/status_repository.go`
  - supplemental hashtag index creates are side effects, not independent status entry points
- `cmd/status-indexer/main.go`
  - creates search/trending index rows, not `Status` rows
- `pkg/testing/inmemory/status_repository.go`
  - test-only helper, not production runtime code

## Source files

- `pkg/services/notes/service.go`
- `pkg/services/conversations/service.go`
- `pkg/services/quotes/quote_service.go`
- `pkg/storage/repositories/status_repository.go`
- `pkg/storage/repositories/conversation_send_repository.go`
