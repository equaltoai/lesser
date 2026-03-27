# Status Note Persistence Inventory

Date: 2026-03-27

This inventory freezes every persisted `Status.Note` field that currently depends on nested ActivityPub payload structure, with special attention to fields whose runtime shape includes interfaces, maps, or slices of dynamic values.

## Source of truth

- `pkg/activitypub/types.go`
- `pkg/storage/models/status.go`
- `pkg/services/notes/service.go`
- `pkg/services/conversations/service.go`
- `pkg/services/quotes/quote_service.go`
- `cmd/lesser/verify_status_contract.go`

## Persisted note payload

`models.Status.Note` persists the following top-level fields today:

| Field path | Stored shape | Current runtime producers | Public notes | Direct messages |
| --- | --- | --- | --- | --- |
| `BaseObject.Context` | `activitypub.ContextValue` (`[]any` containing a string plus a JSON-LD extension map in current live DM/public notes) | `notes.Service.buildActivityPubNote`, `conversations.Service.buildActivityPubNote`, quote wrappers that reuse note create | yes | yes |
| `BaseObject.ID` | string | all note builders | yes | yes |
| `BaseObject.Type` | string | all note builders | yes | yes |
| `BaseObject.Published` / `BaseObject.Updated` | timestamp pointers | all note builders | yes | yes |
| `BaseObject.To` | `[]string` audience actor ids / collections | public and DM note builders | yes | yes |
| `BaseObject.CC` | `[]string` audience actor ids / collections | public note builder | yes | no in current DM builder |
| `BaseObject.BTo` | `[]string` blind audience | public note builder | yes when caller sets it | no in current DM builder |
| `BaseObject.BCC` | `[]string` blind audience | public note builder | yes when caller sets it | no in current DM builder |
| `BaseObject.InReplyTo` | string | public and DM reply builders | yes | yes |
| `BaseObject.Summary` | string | public and DM builders | yes | yes |
| `BaseObject.Sensitive` | bool | public and DM builders | yes | yes |
| `Content` | string | all note builders | yes | yes |
| `AttributedTo` | string actor id | all note builders | yes | yes |
| `Attachment` | `[]activitypub.Attachment` | public note create with media | yes | not populated by current DM builder |
| `Tag` | `[]activitypub.Tag` | hashtag/mention builders and DM recipient mention builder | yes | yes |
| `ConversationID` | string | conversation-aware note builders | replies/threads/quotes | yes |
| `Visibility` | string Lesser extension | all note builders | yes | yes |
| `QuoteURL` | string | quote-capable note flows | yes when quoting | no |
| `Quoteable` | bool | quote-capable note flows | yes when quote metadata is present | no |
| `QuoteNotifications` | bool | quote-capable note flows | yes when quote metadata is present | no |
| `QuoteContext` | `*activitypub.QuoteContext` nested struct | quote note flows | yes when quoting | no |
| `AgentAttribution` | `*activitypub.AgentPostAttribution` nested struct | agent-authored public notes and DMs | yes | yes |

## Dynamic and map-shaped risk points

These are the fields that currently rely on open-ended nested shapes rather than simple scalar storage:

1. `BaseObject.Context`
   - Type: `activitypub.ContextValue`
   - Effective live shape: `[]any{string, map[string]any}`
   - Current live DM and public notes both use `activitypub.Context`, so they persist the same mixed string-plus-map payload.
2. `QuoteContext`
   - Type: nested struct with optional fields
   - Risk: it is not interface-typed, but it is still a nested payload that should remain inside the same canonical serializer boundary as `Context`, `Tag`, and `Attachment`.
3. `AgentAttribution`
   - Type: nested struct stored as JSON-shaped object
   - Risk: it is marshaled through custom note JSON behavior today and should not depend on whichever Dynamo marshaler happens to touch it first.
4. `Attachment`
   - Type: `[]activitypub.Attachment`
   - Risk: representative public notes persist a list of nested maps even though current DM notes do not.
5. `Tag`
   - Type: `[]activitypub.Tag`
   - Risk: public notes mix hashtag and mention entries; DM notes persist mention entries derived from resolved recipients.

## Current live-shaped DM note contract

The DM outage path was grounded in the note shape produced by `conversations.Service.buildActivityPubNote`:

- `BaseObject.Context = activitypub.Context`
- `BaseObject.To = []string{<resolved recipient actor ids>}`
- `Tag = []activitypub.Tag{Mention...}` for the same resolved recipients
- `ConversationID = <canonical DM conversation id>`
- `Visibility = direct`
- `Attachment = nil` in the current DM send builder

This means the failing live DM shape was not an exotic edge case; it was the standard persisted DM note payload with mixed `Context` values and mention tags.

## Explicit non-persisted note fields

These fields exist on ActivityPub note-related structs but are not part of the stored `Status.Note` contract:

- `activitypub.QuoteContext.OriginalStatus`
  - `json:"-"`; runtime-only helper, not persisted.

## Contract requirement going forward

The persistence boundary must own the full nested `Status.Note` payload, not just `ContextValue` in isolation. `Context`, audiences, mentions/tags, attachments, quote metadata, and agent attribution all need one canonical serializer so public notes and DM sends cannot diverge by write mode.
