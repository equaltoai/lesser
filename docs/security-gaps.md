# Security Gaps Inventory (Living)

Date: 2026-02-07  
Repo: `lesser`  

This document captures **confirmed security gaps** (and likely-adjacent gaps) discovered during review. Lesser is an
ActivityPub system, so some endpoints are intentionally public; this inventory focuses on:

- Places where **untrusted content** is rendered as **HTML** (or embedded into HTML attributes) without escaping/sanitizing.
- Places where **authorization/privacy expectations** exist in comments or parity implementations, but are not enforced.

Related:
- Remediation plan: `docs/security-remediation-roadmap.md`
- Security-sensitive placeholders/stubs: `docs/security-stubs-and-placeholders.md`

## Legend

- **Severity:** P0 (critical) → P3 (low)
- **Confidence:** 1–10 (how sure we are this is exploitable in practice)
- **Status:** confirmed / likely / needs verification

---

## P0 — Stored XSS: Actor profile HTML rendering

**Status:** fixed (Milestone 2)  
**Confidence:** 9/10  

The actor HTML profile renderer previously interpolated actor-controlled fields directly into HTML without escaping, and
the actor service did not set a CSP header.

**Primary locations:**
- `cmd/actor/main.go` now renders the profile via `html/template` (`generateHTMLProfile` uses `pkg/security/htmlsafe.RenderTemplate`)
  and sanitizes the bio via `common.SanitizeContent`.
- `cmd/actor/main.go` now emits a restrictive `Content-Security-Policy` for HTML responses in `actorActivityPubSecurityHeaders`.

**Write path (stored source):**
- `pkg/services/accounts/service.go` now sanitizes `cmd.Bio` into `account.User.Note` and `account.Actor.Summary`
  (Milestone 3).
- `pkg/services/accounts/service.go` stores `cmd.DisplayName` verbatim into `account.Actor.Name` (plain-text field); HTML
  surfaces escape at render time.

**Why this matters even in ActivityPub:** the HTML profile page is a browser-facing surface (`/users/:username`) and is
reachable whenever the client negotiates HTML.

**Notes:**
- The objects HTML renderer shows the intended pattern (escape everything): `cmd/objects/main.go:404`.
- Write-time sanitization for the bio and profile fields is enforced in Milestone 3; Milestone 2 ensures the
  browser-facing profile page is safe even if legacy stored values exist.

---

## P0 — Stored HTML injection / client-XSS: Community notes content in Mastodon-compatible fields

**Status:** fixed (Milestone 3)  
**Confidence:** 8/10  

Community note `Content` is stored verbatim and later embedded into an HTML string returned in a Mastodon-style response.

**Write path:**
- `pkg/services/notes/service.go` now escapes `storage.CommunityNote.Content` at write time (`CreateCommunityNote`),
  ensuring it is always safe to embed in HTML-by-contract output.
  - **Additional fix:** GraphQL `addCommunityNote` previously wrote community notes directly via storage
    (`r.Storage.CommunityNote().CreateCommunityNote`), bypassing the write-time invariant. It now uses
    `Registry.Notes().CreateCommunityNote` so escaping is enforced consistently.

**Read/response shaping path:**
- `cmd/api/handlers/notes.go:371` returns `Content: fmt.Sprintf("<p>Community Note: %s</p>", note.Content)` without
  escaping/sanitization.

**Impact model:** the response is JSON, but the field is *HTML by contract* and is typically rendered as HTML by
Mastodon-compatible clients. This becomes stored client-side XSS if unsafe HTML is present.

**Existing primitives that are currently not applied here:**
- `pkg/common/sanitize.go:115` (`common.SanitizeContent`)
- Inbound ActivityPub object sanitization exists and is used on inbox processing, but is not reused here:
  - `cmd/inbox/internal/routing/inbox.go:2019` (`common.SanitizeActivityPubObjectDefault`)
  - `cmd/inbox/internal/validation/activity_validation.go:68` (`common.SanitizeActivityPubObjectDefault`)

---

## P0 — Stored HTML injection / client-XSS: Status content + profile bio/fields returned as HTML without sanitization

**Status:** fixed (Milestone 3)  
**Confidence:** 8/10  

Multiple user-controlled fields that Mastodon clients treat as server-sanitized HTML are stored and returned without
sanitization/escaping.

### Status content (`/api/v1/statuses`, timelines, status fetch)

**Write path:**
- `pkg/services/notes/service.go` now sanitizes status `Content` (and `SpoilerText`) at write time (`CreateNote` /
  `UpdateNote`) using the shared ActivityPub/Mastodon bluemonday policy.
- `cmd/api/handlers/statuses.go` now sanitizes status edits at write time (`applyStatusUpdates`).

**Read/response shaping path:**
- `cmd/api/handlers/helpers.go:403` maps `storageStatus.Content` directly into the API status `content` field.
- `pkg/transformations/converters.go:104` maps `obj["content"]` directly into API status `Content` without sanitization.

**Additional HTML surface:**
- `cmd/api/handlers/oembed.go` now sanitizes `note.Content` when rendering the `/embed/:id` HTML page and validates/escapes
  attachment URLs (Milestone 2). This mitigates browser-facing XSS on that surface.

### Account bio + profile fields (`/api/v1/accounts/*`)

**Write path:**
- `pkg/services/accounts/service.go` now sanitizes `cmd.Bio` at write time into both the Mastodon-compatible user record
  (`account.User.Note`) and the ActivityPub actor summary (`account.Actor.Summary`).
- `pkg/services/accounts/service.go` now sanitizes profile field values at write time for both the user record and actor
  attachments (Mastodon “fields”).

**Read/response shaping path:**
- `pkg/transformations/converters.go:50` maps `actor.Summary` into Mastodon `Account.Note` (HTML field by Mastodon contract).
- `pkg/transformations/converters.go:253` maps attachment `name`/`value` into Mastodon `fields` without sanitization.

**Why this is important:** Mastodon-compatible clients commonly render `status.content` and `account.note` as HTML. If Lesser
does not enforce “sanitized HTML” invariants, a malicious client can submit HTML/JS which is later executed by other
clients.

---

## P0 — Authorization bypass: Moderation GraphQL resolvers lack role checks

**Status:** fixed (Milestone 5)  
**Confidence:** 9/10  

GraphQL requests are authenticated at the HTTP boundary (`cmd/graphql/main.go` `handleGraphQL`), but moderation surfaces
must still be role-gated so that regular users cannot read or control moderation state.

**Fix summary:**
- Moderation queries in `graph/query_resolvers_moderation.go` now require `requireModeratorOrAdmin`.
- Pattern management mutations now require `requireModeratorOrAdmin`.
- Moderation subscriptions now require `requireModeratorOrAdmin`.

**Pre-fix locations (for audit context):**
- `graph/query_resolvers_moderation.go` (`ModerationQueue`, `ModerationDashboard`, `ModerationEffectiveness`,
  `ModerationPatterns`, `ModeratorActivity`, `PatternEffectiveness`)
- `graph/mutation_resolvers_moderation.go` (`DeleteModerationPattern`, `UpdateModerationPattern`,
  `CreateModerationPattern`)
- `graph/subscription_resolvers_moderation.go` (`ModerationEvents`, `ModerationAlerts`, `ModerationQueueUpdate`)

**Implementation note:** the parity moderation resolvers already used the correct role gate
(`graph/auth_helpers_roles.go:11` / `requireModeratorOrAdmin`).

---

## P1 — Missing authorization: GraphQL “ops/insights” queries are publicly callable

**Status:** fixed (Milestone 5)  
**Confidence:** 8/10  

GraphQL is authenticated at the HTTP boundary, but several “ops/insights” resolvers were callable by any authenticated
user. These are now admin-only (`requireAdmin`) to avoid leaking internal analytics/budgets/health data to regular users.

**Pre-fix examples (not exhaustive):**
- Cost + performance:
  - `graph/query_resolvers_cost.go` (`CostBreakdown`, `InfrastructureHealth`, `SlowQueries`, `PerformanceMetrics`)
- Federation management:
  - `graph/query_resolvers_federation.go` (`InstanceRelationships`, `InstanceBudgets`, `InstanceHealthReport`,
    `FederationMap`)
- AI analysis / object introspection:
  - `graph/query_resolvers_ai.go` (`ExplainObject`, `AiAnalysis`, `AiCapabilities`)

**Fix summary:**
- Ops/insights queries now use `requireAdmin`.
- Ops/insights subscriptions now use `requireAdmin`.

---

## P0 — Missing authorization: federation control GraphQL mutations are unauthenticated

**Status:** fixed (Milestone 5)  
**Confidence:** 9/10  

Several federation-cost/budget/limit mutations did not authenticate or authorize the caller at all (they ignored `ctx`
and did not call `requireAuth`/`requireAdmin`). Even where implementations are stubbed, these are high-risk footguns once
backed by real persistence.

**Fix summary:**
- Federation control mutations now require `requireAdmin` and accept a real `ctx`.
- Severance/operator mutations (`acknowledgeSeverance`, `attemptReconnection`) are also admin-only.

**Pre-fix locations (for audit context):**
- `graph/mutation_resolvers_federation.go` (`OptimizeFederationCosts`, `SetFederationLimit`, `PauseFederation`,
  `ResumeFederation`, `SetInstanceBudget`)

---

## P0 — Privacy/authorization bypass: `Notes.GetNote` skips visibility checks and is used in public+auth endpoints

**Status:** fixed (Milestone 4)  
**Confidence:** 9/10  

The notes service exposes two getters:

- `pkg/services/notes/service.go` (`GetNote`) — enforces visibility as an unauthenticated viewer (public/unlisted only).
- `pkg/services/notes/service.go` (`GetNoteWithViewer`) — enforces privacy via `checkViewPermissions`.

Previously, multiple endpoints and resolvers called `GetNote` even when the request was unauthenticated or the viewer was
only optionally authenticated. That enabled fetching private/direct content by ID (and related derivatives like context,
oEmbed, and translation), bypassing intended privacy rules.

**Fix summary:**
- `GetNote` now performs a service-layer visibility check as if the viewer is unauthenticated (public/unlisted only).
- Viewer-aware access for authenticated flows uses `GetNoteWithViewer`.
- REST + GraphQL call sites were updated to use `GetNoteWithViewer` when viewer context exists/should exist (status
  update/delete, translation, notifications, GraphQL object/status conversion, etc.).

**Representative call sites (pre-fix, non-exhaustive):**
- REST status fetch: `cmd/api/handlers/statuses.go` (`HandleGetStatusLift`)
- REST status context: `cmd/api/handlers/statuses.go` (`HandleGetStatusContextLift`)
- REST oEmbed: `cmd/api/handlers/oembed.go`
- REST translation: `cmd/api/handlers/translation.go`
- GraphQL object lookup: `graph/query_resolvers_notes.go`
- GraphQL status history: `graph/query_resolvers_statuses_parity.go`

**Why this is a gap:** the `checkViewPermissions` logic clearly exists and appears to encode the intended privacy model
(`pkg/services/notes/service.go:678`). The issue is that the “no-check” getter is used in places that should be
privacy-aware.

**Note:** the comment on `GetNote` previously claimed it performed privacy checks, but it did not. The implementation and
comment are now aligned.

---

## P0 — Authorization bypass: inbox collection authorization uses substring matching

**Status:** fixed (2026-02-07)  
**Confidence:** 8/10  

`verifyCollectionAuthorization` previously used a substring check (`strings.Contains`) between an actor handle and the
collection owner username. This allowed cross-domain impersonation (e.g., `@alice@attacker.com` matching `alice`) and
returned early, bypassing subsequent guards (including the stricter featured-collection check).

**Primary location:**
- `cmd/inbox/internal/routing/inbox.go` (`verifyCollectionAuthorization`)

**Fix summary:**
- Authorization now requires **exact** username equality and requires the actor’s domain to match the collection’s
  domain.
- Regression tests cover cross-domain same-username attempts and prevent substring-based bypass.

---

## P0 — Account takeover: unauthenticated wallet linking can target arbitrary usernames

**Status:** confirmed  
**Confidence:** 9/10  

`POST /auth/wallet/link` accepts a `username` in the request body when no bearer token is present (“registration flow”).
Without a registration-only gate, this allows an unauthenticated caller to attempt linking a wallet to **any existing**
username (including a victim’s account), enabling account takeover if the attacker can complete the wallet signature
steps.

**Primary location:**
- `cmd/api/handlers/wallet.go` (`HandleLinkWalletLift`)
  - Uses `req.Username` when unauthenticated
  - Historically, signature verification was conditional (linking could proceed without enforcing proof)
  - No check that unauth linking was limited to *new* accounts created by that caller

**Impact model:** a wallet link is an authentication factor. Allowing unauthenticated linking against arbitrary usernames
turns “link wallet” into an account-takeover vector.

**Recommendation (implemented as part of remediation):**
- Require signature-based proof for wallet linking, even for authenticated users (wallet ownership).
- For unauthenticated linking (“registration flow”), require a registration-only gate (e.g., a challenge ID stored in
  user metadata during registration) so unauth linking cannot be used against existing accounts.

---

## P1 — Registration proof missing: `/api/v1/accounts` does not require wallet/WebAuthn verification

**Status:** confirmed  
**Confidence:** 8/10  

Registration currently accepts a username + agreement without requiring a passwordless proof-of-control step (wallet
signature or WebAuthn attestation). This makes “registration” a remotely callable operation without the intended
WebAuthn/wallet proof.

**Primary location:**
- `cmd/api/handlers/accounts.go` (`HandleRegistrationLift`)

**Recommendation (implemented as part of remediation):**
- Require a verified wallet challenge (or WebAuthn flow) as a precondition to account creation.

---

## P1 — Authorization gap: SSE list stream lacks list membership validation

**Status:** fixed (Milestone 6)  
**Confidence:** 7/10  

The SSE “list” stream requires authentication and now validates that the authenticated user is allowed to subscribe to
the requested list ID (list ownership/membership), preventing authenticated users from subscribing to other users’ list
streams even if list IDs are guessable/enumerable.

**Location:**
- `cmd/sse/main.go:236` (`handleListStream`)

**Fix summary:**
- `handleListStream` now enforces list ownership/membership before streaming.
- A regression test exists proving a non-member cannot subscribe.

---

## P0 — Authorization bypass: WebSocket streaming can subscribe to other users’ private streams

**Status:** fixed (Milestone 6)  
**Confidence:** 9/10  

The WebSocket streaming “subscribe” handler accepted arbitrary stream names for user-scoped streams and did not ensure
that the stream’s target matched the authenticated user. In addition, `direct:<username>` was previously not treated as
an authenticated stream (only the exact `direct` alias required auth).

This enabled:
- unauthenticated subscription attempts to `direct:<victim>`
- authenticated subscription to `user:<victim>` / `user:notification:<victim>`
- authenticated subscription to `list:<id>` without list ownership validation

These streams can carry private/direct statuses and notifications; subscribing to another user’s stream can leak private
content.

**Primary location:**
- `cmd/streaming/main.go` (`handleSubscribe`)

**Fix summary:**
- Stream aliases (`user`, `user:notification`, `direct`) are canonicalized to user-scoped stream names.
- User/direct streams are restricted to the authenticated user’s own username.
- List streams require list ownership/membership validation before subscribing.
- Unit tests cover the enforced behavior and prevent regressions.

---

## P1 — Privacy leak: GraphQL `accountQuotePermissions` exposes `blockList` publicly

**Status:** fixed
**Confidence:** 9/10

Account-level quote permissions are live: the authenticated GraphQL mutation `updateAccountQuotePermissions` persists
them, and the GraphQL quote path enforces them through `QuoteService.checkQuotePermissions` in
`pkg/services/quotes/quote_service.go`. The REST quote-permission GET and PUT routes return `501 Not Implemented` because
their REST implementation genuinely does not exist; they do not fabricate or persist preferences.

The authenticated GraphQL `accountQuotePermissions(username: ...)` read also returns a not-implemented error for every
target. It fails closed without fabricating defaults while a real read is pending an explicit authorization decision:
whether an account owner, another viewer, or both may read the raw block list is a product decision, not a safe default.
The REST quote-creation twin now uses the same account-level `QuoteService.CheckQuotePermissions` predicate and maps its
denial and storage-error classes into the established REST error contract.

---

## P1 — Defense-in-depth: No CSP on HTML responses

**Status:** fixed (Milestone 2)  
**Confidence:** 7/10 (defense-in-depth; most valuable once XSS surfaces are addressed)  

HTML is served by multiple handlers/services, and a restrictive `Content-Security-Policy` is now emitted for the
browser-facing HTML surfaces in-scope for this remediation (actor, objects, oEmbed embed page).

**Examples:**
- Actor HTML profile response: `cmd/actor/main.go` (`actorActivityPubSecurityHeaders`)
- Object HTML response: `cmd/objects/main.go` (`objectsActivityPubSecurityHeaders`)
- oEmbed embed HTML response: `cmd/api/handlers/oembed.go` (`HandleEmbedPageLift`)

Given current HTML injection surfaces (above), CSP would significantly reduce blast radius.

---

## P2 — OAuth authorization code TOCTOU: non-atomic code consumption on exchange

**Status:** fixed (2026-02-07)  
**Confidence:** 7/10  

The OAuth `authorization_code` exchange previously read the authorization code, generated tokens, and then deleted the
code in a non-atomic sequence. Deletion failure was treated as non-critical. This created a theoretical race window where
the same code could be exchanged twice via concurrent requests (duplicate session/token issuance for the same user
context).

**Practical exploitation requires:**
- Intercepting a non-PKCE-protected authorization code, and
- Racing two concurrent exchange requests within the 5-minute TTL window.

**Impact:** limited to duplicate session/token creation for the same user context.

**Fix summary:**
- Consume (delete) the authorization code **before** issuing tokens, and treat consumption failures as `invalid_grant`.
- Make authorization code deletion conditional (`IfExists`) so only one concurrent exchange can succeed.

**Primary locations:**
- `cmd/api/handlers/oauth.go` (`exchangeAuthorizationCode`)
- `pkg/storage/repositories/oauth_helpers.go` / `pkg/storage/repositories/account_repository_oauth.go` (`DeleteAuthorizationCode*`)

---

## P2 — Weak HTML sanitization: instance extended description

**Status:** confirmed (admin-controlled today, but still a footgun)  
**Confidence:** 7/10  

Instance extended description is returned as HTML to clients (`/api/v1/instance`), but sanitization is implemented as
ad-hoc string replacements and is bypassable for many common XSS payloads (e.g., event-handler attributes).

**Locations:**
- `cmd/api/handlers/instance.go:162` returns `ExtendedDescription` to clients.
- `pkg/storage/repositories/instance_repository.go:1110` implements `sanitizeDescription` via string replacement.

**Recommendation:** replace ad-hoc logic with the existing bluemonday sanitizer (`pkg/common/sanitize.go:22`) and align
policy to Mastodon’s “sanitized HTML” expectations.

---

## Root causes / patterns to address

1. **HTML invariants not enforced:** content treated as “already HTML” without a guarantee it’s sanitized.
2. **Escaping vs sanitization confusion:** some surfaces need `html.EscapeString` (plain-text into HTML), others need
   HTML sanitization (allowed tags, strip scripts).
3. **Parity drift:** “*_parity.go” files contain correct auth patterns, while the active resolver files omit them.
4. **Privacy checks exist but are bypassed:** `GetNoteWithViewer` exists but `GetNote` is used broadly.

---

## Suggested follow-up audit checklist (next pass)

- Inventory all **HTML-serving** paths (`text/html`) and ensure:
  - `html/template` or `html.EscapeString` is used for interpolation.
  - A CSP policy exists (nonce-based if inline scripts are required).
- Inventory all API fields that are **HTML by contract** (Mastodon: `status.content`, `account.note`, profile `fields[].value`)
  and enforce sanitization at **write time** (preferred) or **response time**.
- Replace or restrict privacy-bypassing getters:
  - Either remove `GetNote` from public call sites, or change `GetNote` to enforce public-only and add a
    `GetNoteWithViewer` usage everywhere else.
- For GraphQL:
  - Enumerate all moderation/admin resolvers and ensure role checks match REST equivalents.
  - Add regression tests that unauthenticated/regular users cannot access moderation data/mutations/subscriptions.
