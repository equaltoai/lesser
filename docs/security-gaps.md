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

**Status:** confirmed  
**Confidence:** 9/10  

The actor HTML profile renderer interpolates actor-controlled fields directly into HTML without escaping, and the actor
service does not set a CSP header.

**Primary locations:**
- `cmd/actor/main.go:242` (`generateHTMLProfile`) renders unescaped `actor.Name`, `actor.BaseObject.Summary`, `actor.Icon.URL`,
  `actor.PreferredUsername`, `actor.ID` into HTML and HTML attributes.
- `cmd/actor/main.go:236` sets `content-type: text/html; charset=utf-8`.
- `cmd/actor/main.go:540` actor security headers middleware does **not** set `Content-Security-Policy`.

**Write path (stored source):**
- `pkg/services/accounts/service.go:872` stores `cmd.DisplayName` into `account.Actor.Name` without sanitization.
- `pkg/services/accounts/service.go:876` stores `cmd.Bio` into `account.Actor.Summary` without sanitization.

**Why this matters even in ActivityPub:** the HTML profile page is a browser-facing surface (`/users/:username`) and is
reachable whenever the client negotiates HTML.

**Notes:**
- The objects HTML renderer shows the intended pattern (escape everything): `cmd/objects/main.go:404`.

---

## P0 — Stored HTML injection / client-XSS: Community notes content in Mastodon-compatible fields

**Status:** confirmed  
**Confidence:** 8/10  

Community note `Content` is stored verbatim and later embedded into an HTML string returned in a Mastodon-style response.

**Write path:**
- `cmd/api/handlers/notes.go:89` stores `req.Content` directly into `storage.CommunityNote.Content`.

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

**Status:** confirmed (systemic)  
**Confidence:** 8/10  

Multiple user-controlled fields that Mastodon clients treat as server-sanitized HTML are stored and returned without
sanitization/escaping.

### Status content (`/api/v1/statuses`, timelines, status fetch)

**Write path:**
- `cmd/api/handlers/statuses.go:111` passes `Content: req.Status` directly to `Notes.CreateNote`.
- `pkg/services/notes/service.go:1092` stores the content into ActivityPub `Note.Content` verbatim.
- `pkg/services/notes/service.go:385` stores `models.Status.Content = cmd.Content` verbatim.

**Read/response shaping path:**
- `cmd/api/handlers/helpers.go:403` maps `storageStatus.Content` directly into the API status `content` field.
- `pkg/transformations/converters.go:104` maps `obj["content"]` directly into API status `Content` without sanitization.

**Additional HTML surface:**
- `cmd/api/handlers/oembed.go:633` embeds `note.Content` directly into an HTML document (comment says “Already HTML”).
- `cmd/api/handlers/oembed.go:660` writes `attachment.URL` into an `<img src="...">` attribute without escaping (attribute
  injection if an attacker can influence attachment URLs).

### Account bio + profile fields (`/api/v1/accounts/*`)

**Write path:**
- `pkg/services/accounts/service.go:876` stores `cmd.Bio` into `account.Actor.Summary` without sanitization.
- `pkg/services/accounts/service.go:844` / `pkg/services/accounts/service.go:914` store profile field `Name`/`Value` verbatim.

**Read/response shaping path:**
- `pkg/transformations/converters.go:50` maps `actor.Summary` into Mastodon `Account.Note` (HTML field by Mastodon contract).
- `pkg/transformations/converters.go:253` maps attachment `name`/`value` into Mastodon `fields` without sanitization.

**Why this is important:** Mastodon-compatible clients commonly render `status.content` and `account.note` as HTML. If Lesser
does not enforce “sanitized HTML” invariants, a malicious client can submit HTML/JS which is later executed by other
clients.

---

## P0 — Authorization bypass: Moderation GraphQL resolvers lack role checks

**Status:** confirmed  
**Confidence:** 9/10  

GraphQL auth is optional at middleware level (`cmd/graphql/main.go:797`), so resolvers must enforce authorization.
`graph/query_resolvers_moderation.go` contains moderation data resolvers with **no** authz checks, and several moderation
mutations/subscriptions require only authentication instead of moderator/admin role.

**Unauthenticated moderation queries (no `requireAuth` / `requireModeratorOrAdmin`):**
- `graph/query_resolvers_moderation.go:20` (`ModerationQueue`)
- `graph/query_resolvers_moderation.go:86` (`ModerationDashboard`)
- `graph/query_resolvers_moderation.go:156` (`ModerationEffectiveness`)
- `graph/query_resolvers_moderation.go:207` (`ModerationPatterns`)
- `graph/query_resolvers_moderation.go:280` (`ModeratorActivity`)
- `graph/query_resolvers_moderation.go:387` (`PatternEffectiveness`)

**Pattern management mutations only require login:**
- `graph/mutation_resolvers_moderation.go:420` (`DeleteModerationPattern`)
- `graph/mutation_resolvers_moderation.go:446` (`UpdateModerationPattern`)
- `graph/mutation_resolvers_moderation.go:470` (`CreateModerationPattern`)

**Moderation subscriptions only require login:**
- `graph/subscription_resolvers_moderation.go:15` (`ModerationEvents`)
- `graph/subscription_resolvers_moderation.go:52` (`ModerationAlerts`)
- `graph/subscription_resolvers_moderation.go:90` (`ModerationQueueUpdate`)

**Correct pattern exists (role gating):**
- `graph/auth_helpers_roles.go:11` (`requireModeratorOrAdmin`)
- `graph/query_resolvers_moderation_parity.go:63` shows `requireModeratorOrAdmin` use for moderation queries.

---

## P1 — Missing authorization: GraphQL “ops/insights” queries are publicly callable

**Status:** confirmed  
**Confidence:** 7/10  

Because GraphQL authentication is optional at middleware level (`cmd/graphql/main.go:797`), any query resolver that does not
call `requireAuth` / `requireAdmin` is callable by unauthenticated clients.

Several “operations/insights” resolvers currently have no auth checks, which likely exposes internal analytics, budgets,
and system performance data to the public internet.

**Examples (not exhaustive):**
- Cost + performance:
  - `graph/query_resolvers_cost.go:15` (`CostBreakdown`) — no auth
  - `graph/query_resolvers_cost.go:103` (`InfrastructureHealth`) — no auth
  - `graph/query_resolvers_cost.go:121` (`SlowQueries`) — no auth
  - `graph/query_resolvers_cost.go:140` (`PerformanceMetrics`) — no auth
- Federation management:
  - `graph/query_resolvers_federation.go:148` (`InstanceRelationships`) — no auth
  - `graph/query_resolvers_federation.go:168` (`InstanceBudgets`) — no auth
  - `graph/query_resolvers_federation.go:188` (`InstanceHealthReport`) — no auth
  - `graph/query_resolvers_federation.go:208` (`FederationMap`) — no auth
- AI analysis / object introspection:
  - `graph/query_resolvers_ai.go:18` (`ExplainObject`) — no auth
  - `graph/query_resolvers_ai.go:95` (`AiAnalysis`) — no auth
  - `graph/query_resolvers_ai.go:216` (`AiCapabilities`) — no auth

**Recommendation:** decide per-field intended exposure (public vs authenticated vs admin), then apply `requireAuth` /
`requireAdmin` consistently and add regression tests.

**Note:** Round12 tests currently call several of these “ops/insights” resolvers without auth (suggesting this exposure may
be intentional today). If so, document that policy explicitly and ensure any genuinely-sensitive fields are gated.
- `graph/cost_resolvers_round12_test.go:70` calls `CostBreakdown` unauthenticated.
- `graph/query_resolvers_federation_round12_test.go:15` calls `InstanceMetrics` unauthenticated.

---

## P0 — Missing authorization: federation control GraphQL mutations are unauthenticated

**Status:** confirmed  
**Confidence:** 8/10  

Several federation-cost/budget/limit mutations do not authenticate or authorize the caller at all (they ignore `ctx` and
do not call `requireAuth`/`requireAdmin`). Even where implementations are currently stubbed, these are high-risk footguns
once backed by real persistence.

**Locations:**
- `graph/mutation_resolvers_federation.go:16` (`OptimizeFederationCosts`)
- `graph/mutation_resolvers_federation.go:42` (`SetFederationLimit`)
- `graph/mutation_resolvers_federation.go:59` (`PauseFederation`)
- `graph/mutation_resolvers_federation.go:71` (`ResumeFederation`)
- `graph/mutation_resolvers_federation.go:83` (`SetInstanceBudget`)

**Recommendation:** gate these behind `requireAdmin` (or a dedicated operator role) before enabling real behavior.

**Note:** the current implementations ignore `ctx` entirely (e.g., `OptimizeFederationCosts(_ context.Context, ...)`), so
even an auth middleware change won’t help unless resolvers themselves enforce authorization.

---

## P0 — Privacy/authorization bypass: `Notes.GetNote` skips visibility checks and is used in public+auth endpoints

**Status:** confirmed  
**Confidence:** 8/10  

The notes service exposes two getters:

- `pkg/services/notes/service.go:624` (`GetNote`) — **does not** enforce visibility/view-permissions.
- `pkg/services/notes/service.go:644` (`GetNoteWithViewer`) — enforces privacy via `checkViewPermissions`.

Multiple endpoints and resolvers call `GetNote` even when the request is unauthenticated or the viewer is only optionally
authenticated. This enables fetching private/direct content by ID (and related derivatives like context, oEmbed, and
translation), bypassing intended privacy rules.

**Representative call sites (non-exhaustive):**
- REST status fetch: `cmd/api/handlers/statuses.go:574` (`HandleGetStatusLift`)
- REST status context: `cmd/api/handlers/statuses.go:774` + `cmd/api/handlers/statuses.go:835` (`HandleGetStatusContextLift`)
- REST oEmbed: `cmd/api/handlers/oembed.go:145` + `cmd/api/handlers/oembed.go:450`
- REST translation: `cmd/api/handlers/translation.go:150`
- GraphQL object lookup: `graph/query_resolvers_notes.go:22`
- GraphQL status history: `graph/query_resolvers_statuses_parity.go:104`

**Why this is a gap:** the `checkViewPermissions` logic clearly exists and appears to encode the intended privacy model
(`pkg/services/notes/service.go:678`). The issue is that the “no-check” getter is used in places that should be
privacy-aware.

**Note:** the comment on `GetNote` is misleading (“with privacy checks”), but the function currently performs no privacy
checks beyond `Deleted` filtering (`pkg/services/notes/service.go:623`).

---

## P1 — Authorization gap: SSE list stream lacks list membership validation

**Status:** confirmed  
**Confidence:** 7/10  

The SSE “list” stream requires authentication, but does not validate that the authenticated user is allowed to subscribe
to the requested list ID. This can allow an authenticated user to subscribe to another user’s list stream if list IDs are
guessable/enumerable.

**Location:**
- `cmd/sse/main.go:234` (`handleListStream`) has `claims` available but does not validate membership.
- `cmd/sse/main.go:248` explicitly notes this is a placeholder (“future list membership validation”).

**Recommendation:** enforce list ownership/membership before subscribing, and add tests that a non-member cannot access a
list stream.

---

## P1 — Privacy leak: GraphQL `accountQuotePermissions` exposes `blockList` publicly

**Status:** confirmed  
**Confidence:** 7/10  

`accountQuotePermissions(username: ...)` can be queried without authentication and returns the account’s quote-permission
configuration including `blockList` (a user preference that is typically private).

**Location:**
- Schema: `graph/core.graphql:1076`
- Resolver: `graph/query_resolvers_accounts.go:83` (no `requireAuth` / role gating; returns `blockList`)

**Recommendation:** decide intended exposure:
- If this is only for the account owner: require auth and ensure the requester matches the username.
- If others need to know “can I quote this user?”: return a computed boolean for the *viewer*, not the raw block list.

---

## P1 — Defense-in-depth: No CSP on HTML responses

**Status:** confirmed  
**Confidence:** 7/10 (defense-in-depth; most valuable once XSS surfaces are addressed)  

HTML is served by multiple handlers/services, but `Content-Security-Policy` is not set anywhere in the repo.

**Examples:**
- Actor HTML profile response: `cmd/actor/main.go:236` + headers middleware `cmd/actor/main.go:540`
- Object HTML response: `cmd/objects/main.go:245`
- oEmbed HTML response: `cmd/api/handlers/oembed.go:399`

Given current HTML injection surfaces (above), CSP would significantly reduce blast radius.

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
