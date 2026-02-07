# Security Remediation Roadmap (Living)

Date: 2026-02-07  
Repo: `lesser`  

This roadmap turns the findings in `docs/security-gaps.md` into an implementation plan that:

1. Establishes **consistent patterns + shared primitives** for HTML safety and access control.
2. Locks down exposure so that **only clearly-designated public content** (consistent with ActivityPub visibility
   models) is accessible without authentication; everything else requires appropriate authorization.
3. Adds **tests + tooling** so these classes of gaps do not regress.

Related tracking:
- Gaps inventory: `docs/security-gaps.md`
- Security-sensitive placeholders/stubs: `docs/security-stubs-and-placeholders.md`

---

## Target-state patterns (what “good” looks like)

### 1) Public surface is explicit (default-deny)

- **Default**: endpoints/resolvers are treated as **non-public** unless explicitly designated public.
- **Public** means: content is publicly-addressed per ActivityPub conventions (e.g., `Public` / `public` / `unlisted`),
  and responses do not include private metadata.
- **Non-public** means: requires auth (and often role gating) even if the implementation is currently “stubbed”.

Deliverable: a single source of truth document + code-level enforcement:
- **Doc**: public surface matrix (REST + GraphQL) with intended access (public / auth / moderator / admin).
- **Code**: deny-by-default enforcement at boundaries (especially GraphQL), with an explicit allowlist for any public
  exposures that remain.

### 2) Authorization checks are consistent and centralized

- Use one consistent set of helpers for:
  - `requireAuth` (authenticated session)
  - `requireModeratorOrAdmin` (role gate)
  - `requireAdmin` / operator role (for infra/budget/federation controls)
- Avoid “optional auth” for endpoints that return sensitive data; if auth is optional, the handler must still enforce
  visibility rules and return only public data when unauthenticated.

### 3) Visibility/privacy is enforced at the service layer (viewer-aware)

For any object/status fetch by ID:
- The service API should require viewer context (even if viewer is empty) and enforce visibility rules *once*.
- “Get by ID” functions without viewer checks should be treated as **unsafe** and restricted to internal-only use.

### 4) HTML output is safe by construction (escape vs sanitize)

Define and enforce two distinct rules:

- **HTML pages / HTML responses** (`text/html`): use `html/template` (preferred) or `html.EscapeString` for interpolation.
  Never concatenate untrusted strings into HTML or attributes without escaping.
- **“HTML-by-contract” JSON fields** (Mastodon-compatible `status.content`, `account.note`, `fields[].value`, etc.):
  enforce a **sanitized HTML invariant at write time**, using the existing bluemonday policy (`pkg/common/sanitize.go`),
  and treat stored/rendered HTML as “safe HTML”.

### 5) Defense in depth for HTML surfaces

All HTML responses should emit security headers consistently, including CSP. Where inline scripts are unavoidable (e.g.,
oEmbed height messaging), use a nonce-based CSP or eliminate inline scripts.

---

## Milestones

### Milestone 0 — Define and enforce the public surface (policy + guardrails)

**Goal:** only explicitly public, ActivityPub-consistent content is accessible without auth; everything else is gated.

**Work:**
- Write a public surface matrix (REST + GraphQL) documenting intended access levels.
- Implement deny-by-default behavior at boundaries:
  - GraphQL: require authentication by default; allowlist any explicitly-public queries (if any).
  - REST: ensure any optional-auth endpoints enforce visibility rules and return only public/unlisted data to
    unauthenticated callers.
- Add regression tests: unauthenticated access fails for non-public surfaces.

**Acceptance criteria:**
- There is a reviewed, current public-surface matrix (doc) that covers:
  - ActivityPub endpoints (actor/object JSON)
  - Mastodon-compatible REST endpoints that are intentionally public (only for public/unlisted visibility)
  - GraphQL endpoints/fields (expected to be non-public by default)
- GraphQL requests without valid auth are rejected (except any explicitly allowlisted public queries).
- Test coverage exists proving that non-public surfaces reject unauthenticated callers.

---

### Milestone 1 — Introduce shared primitives for authz + safe HTML

**Goal:** eliminate ad-hoc checks and hand-built HTML/escaping so fixes are repeatable.

**Work:**
- Add shared helpers/packages (naming TBD) for:
  - Role/auth checks (reusable across REST + GraphQL).
  - Safe HTML construction (template-first) and HTML sanitization for “HTML-by-contract” fields.
- Provide small, easy-to-use APIs so call sites stop doing `fmt.Sprintf("<div>%s</div>", userValue)`.

**Acceptance criteria:**
- New primitives exist and are used by at least one HTML surface and one GraphQL/REST surface.
- A lint/checklist exists for reviewers: “HTML responses must use templates/escaping helpers” and “GraphQL must be gated”.

---

### Milestone 2 — Fix HTML XSS on browser-facing pages (actor + oEmbed + other HTML)

**Goal:** browser-rendered surfaces cannot execute attacker-controlled scripts.

**Work (apply primitives from Milestone 1):**
- Actor HTML profile page: move to `html/template` or escape all interpolated fields; escape URL attributes.
- oEmbed HTML: do not embed unsafe HTML; ensure `note.Content` is sanitized/escaped according to the chosen invariant;
  escape `attachment.URL` and validate schemes.
- Add CSP headers to all HTML responses (actor, objects, oEmbed), using nonce-based CSP where needed.

**Acceptance criteria:**
- Unit tests demonstrate that common XSS payloads are rendered inert in HTML surfaces.
- All HTML responses emit CSP (and baseline security headers) consistently.

---

### Milestone 3 — Enforce sanitized-HTML invariants for Mastodon-compatible fields

**Goal:** stored + returned “HTML-by-contract” fields are safe to render as HTML in clients.

**Work:**
- Sanitize at write time for:
  - Status content
  - Account bio/note
  - Profile fields values
  - Community notes content
- Ensure read/transform paths do not reintroduce unsanitized content (converters should assume the invariant holds).
- Add a one-time backfill/migration path to sanitize already-stored content, since write-time fixes do not remediate
  existing stored payloads.

**Acceptance criteria:**
- Stored content satisfies “sanitized HTML” policy; tests cover scripts, event handlers, `javascript:` URLs, and attribute
  injection patterns.
- Existing stored content is remediated (backfill/migration) or clearly gated/disabled until remediated.

---

### Milestone 4 — Enforce viewer-aware privacy everywhere (notes/status fetch by ID)

**Goal:** private/direct content cannot be fetched by ID without appropriate viewer permissions.

**Work:**
- Refactor Notes service API so “get by ID” is always viewer-aware (or clearly split into safe public-only vs
  viewer-aware methods).
- Replace call sites that currently fetch without privacy checks (REST + GraphQL + oEmbed + translation + context).
- Add tests for:
  - unauthenticated viewer (public/unlisted only)
  - authenticated follower vs non-follower for private
  - direct-message recipient vs non-recipient

**Acceptance criteria:**
- Private/direct notes are not retrievable by unauthorized callers across *all* surfaces (REST, GraphQL, oEmbed,
  translation, context).
- Behavior is regression-tested (no “one-off” handlers bypass the service-layer privacy gate).

---

### Milestone 5 — Lock down GraphQL moderation + operator controls

**Goal:** moderation and operator controls are accessible only to appropriate roles.

**Work:**
- Moderation: ensure all moderation queries/mutations/subscriptions require moderator/admin.
- Federation/operator controls: gate federation-control mutations behind admin/operator role.
- Ops/insights queries (cost/health/AI/etc.): gate behind admin (or explicit authenticated policy) per Milestone 0.
- Add tests to prove:
  - unauth cannot query these fields
  - authenticated non-mod cannot access mod data or control mutations

**Acceptance criteria:**
- All moderation and operator-control GraphQL fields enforce role checks.
- A test suite exists that fails if any moderation/operator field becomes callable by unauth/regular users.

---

### Milestone 6 — Streaming/list privacy completion (remove placeholders)

**Goal:** authenticated users cannot subscribe to data streams they don’t have access to.

**Work:**
- Implement list membership validation for list streams before subscribing.
- Ensure streaming events also respect status visibility rules (ties into Milestone 4).

**Acceptance criteria:**
- Non-members cannot subscribe to another user’s list stream.
- Streaming surfaces do not leak private/direct content.

---

### Milestone 7 — Tooling + continuous verification (prevent regressions)

**Goal:** security patterns are enforceable, not just documented.

**Work:**
- Add a lightweight “security verification” step to CI (or `./lesser verify …`) that checks for:
  - new GraphQL resolvers added without auth policy documentation/tests
  - new HTML responses without CSP headers
  - new “security-sensitive placeholders” without an entry in `docs/security-stubs-and-placeholders.md`
- Keep the stubs inventory current and require owners + remediation plans for high-risk entries.

**Acceptance criteria:**
- CI fails when new high-risk placeholder/auth bypass is introduced without updating the relevant docs/tests.
- The stubs inventory is maintained and used to drive closure of security-sensitive incomplete implementations.

