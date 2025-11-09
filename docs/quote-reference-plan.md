# Quote Reference Refactor Plan

## Goals

1. Stop embedding quote metadata (URL/context) inside the serialized ActivityPub note.
2. Persist a single canonical reference from the quoting status to the quoted status, so data stays normalized.
3. Ensure GraphQL timelines (and any other consumers) still expose `quoteUrl`, `quoteContext`, and inline quote previews without duplicating storage.
4. Keep future schema changes predictable (no hidden fallbacks), and make it obvious how to migrate existing quote rows.

## Current State (2025-11-09)

- Status rows now include `QuoteTargetStatusID`/`QuoteTargetAuthorID`, so every quote keeps a canonical pointer to the original.
- The quote service (`pkg/services/quotes/quote_service.go`) writes these references via `setQuoteReference` and no longer mutates the serialized `activitypub.Note`.
- GraphQL (`graph/schema.resolvers.go`) derives `quoteUrl`/`quoteContext` by following the stored reference at read time, so no fallbacks or embedded metadata remain.
- Quote metadata assembly now defaults `quoteAllowed` to true (unless the source status was deleted), marks `withdrawn` only when the original post is gone/denied, hydrates `quoteContext.originalAuthor` via the actor loader so display names/avatars populate, and reuses the same cached status to serve `quoteContext.originalNote` for inline previews.
- Quote target lookups are always routed through the per-request `QuoteTargetLoader`, and each GraphQL request logs cache hits/misses so we can spot contexts that fail to attach loaders.
- `NoteField` is once again purely responsible for ActivityPub serialization; the note payload stays quote-free except for federation-only knobs like `quoteable`.

## Target Architecture

1. **Storage Model**
   - Add explicit fields on `models.Status`: `QuoteTargetStatusID string` and `QuoteTargetAuthorID string`.
   - Optionally introduce a lightweight `QuoteReference` struct if we prefer grouping.
   - Dynamo representation becomes a simple scalar attribute (and maybe a GSI if we need to find all quotes of a status quickly—though `quotes` service already tracks relationships in a separate table, so referencing that may suffice).

2. **Canonical Note Payload**
   - Keep the ActivityPub note free of quote metadata. `QuoteURL`/`QuoteContext` should be derived, not stored.
   - `NoteField.Marshal/Unmarshal` stay focused on base note fields (content, attachments, To/CC, etc.).

3. **Quote Service Flow**
   - When attaching a quote, set `quoteStatus.QuoteTargetStatusID = targetStatus.StatusID` (and `QuoteTargetAuthorID` if helpful), persist the status, and update quote counts/notifications as today.
   - `QuoteContext` logic moves out of Dynamo persistence and into read-time assembly.

4. **Read Path / GraphQL**
   - `convertStatusToObject` should detect `status.QuoteTargetStatusID`.
   - Fetch the target status via `Notes().GetNote` (with caching/data loader) and construct the GraphQL `quoteContext` + `quoteUrl` fields on the fly.
   - Inline preview data should include at least the quoted actor + note id so the UI can render a card (consider extending `model.Object` with a `quotedObject` field later if needed).

5. **Migration Strategy**
   - Because we want a single canonical model, we will:
     1. Deploy code that reads quote references if present, otherwise falls back to embedded metadata (temporary shim for migration window only).
     2. Run a scanner that inspects every status with `note.quoteUrl != ""`, copies the reference into `QuoteTargetStatusID`, and strips the embedded metadata.
     3. Deploy the final build that removes the fallback and enforces the reference-only approach.
   - If we wipe dev frequently, we can shortcut by simply recreating quotes after the code change, but production/shared environments will need the backfill script.

## Work Breakdown

### Phase 1 – Storage & Models
1. Extend `pkg/storage/models/status.go` with new fields and update `BeforeCreate/BeforeUpdate` to ensure they persist.
2. Update `pkg/services/notes/service.go` (composeStatus) to initialize those fields from the command (default empty).
3. Adjust `pkg/services/quotes/quote_service.go` so `applyQuoteContext` becomes `setQuoteReference`:
   - Set the new fields on the quoting status.
   - Remove direct mutation of `note.QuoteURL/QuoteContext`.

### Phase 2 – Resolver & Service Changes
1. ✅ `graph/schema.resolvers.go` now calls `resolveQuoteMetadata`, which looks up the referenced status and supplies live `QuoteURL` / `QuoteContext`.
   - ✅ A dedicated `QuoteTargetLoader` batches those lookups so timelines with lots of quotes avoid N+1 Dynamo traffic.
2. Ensure `pkg/services/quotes` exposes a helper for retrieving quote metadata so we don’t duplicate logic in the resolver.
3. Decide whether to expose a richer GraphQL shape (e.g., `quotedObject`) rather than only metadata—document tradeoffs for future work.
4. ✅ Loader contexts are attached for GraphQL and REST bridges, and we emit metrics for cache hits vs. misses so operational dashboards can catch regressions.

### Phase 3 – Migration / Data Hygiene
1. ✅ Write an admin script (Go or Python) that:
   - Scans the `lesser-<env>` table for statuses where `Note.note.quoteUrl` exists.
   - Writes the referenced status ID into `QuoteTargetStatusID` and removes the embedded `note.quote*` fields.
2. ✅ Run the script in each environment (or wipe/reseed dev as we’ve been doing).
3. ✅ After verification, remove the temporary fallback code from the resolver/unmarshaler so only the new reference path remains. GraphQL now exclusively populates `quoteUrl`/`quoteContext` from live references (including permissions metadata) so timelines stay accurate.

### Phase 4 – Tests & Validation
1. Unit tests covering:
   - Quote creation sets the reference field and leaves the note untouched (`pkg/services/quotes/quote_service_test.go`).
   - Resolver populates GraphQL `quoteUrl` / `quoteContext` when a reference is present (`graph/schema.resolvers_quote_test.go`, new file if needed).
2. Integration smoke test: create a quote via GraphQL mutation, query the timeline, confirm the UI fields are present.
3. Update documentation (this file + `docs/quote-permissions-plan.md`) to reflect the new approach.

## Open Questions

1. **Caching** – Should quote target lookups use the existing Notes service cache/data loader to avoid repeated Dynamo hits in timelines? (Recommended.)
2. **GraphQL Schema** – Do we eventually want to return the full quoted object (like Mastodon’s `status.reblog`)? If so, we can design that once references are in place.
3. **Backfill Ownership** – Who runs the migration script in shared environments? Need an owner before deploying the breaking change.

## Next Steps

1. Get sign-off on this plan and confirm whether we will migrate existing data or recreate environments.
2. Begin Phase 1 by updating the storage model and quote service, keeping temporary compatibility code until data is clean.
3. Schedule time to execute the migration or environment reset before removing fallbacks.
