# Security Review Checklist (Quick)

Date: 2026-04-02  
Repo: `lesser`

Use this checklist for PR review to prevent regressions in authz/privacy and HTML safety.

Related:
- Public surface policy: `docs/security-public-surface.md`
- Remediation roadmap: `docs/security-remediation-roadmap.md`
- Gaps inventory: `docs/security-gaps.md`

---

## Public surface (default-deny)

- Any new unauthenticated exposure is **explicitly documented** in `docs/security-public-surface.md`.
- Any new REST route in `cmd/api` is either:
  - allowlisted in `cmd/api/public_surface_middleware.go`, or
  - expected to be auth-only (default), with tests proving it is blocked when unauthenticated.
- GraphQL remains **auth-required by default** at the HTTP boundary (`cmd/graphql`), with any anonymous allowlist
  explicitly documented and test-covered.

Helpful commands:

```bash
rg -n "app\\.(Get|Post|Put|Delete|Patch)\\(" cmd/api/routes.go
```

---

## Authz (role + ownership)

- Moderation/admin controls require role gating (mod/admin), not just authentication.
- Any “optional auth” endpoint must still enforce:
  - viewer-aware privacy (public/unlisted only when unauthenticated)
  - ownership checks for mutable resources

Helpful commands:

```bash
rg -n "requireAuth\\(|requireModeratorOrAdmin\\(|requireAdmin\\(" graph
rg -n "authenticateWithScope\\(|RespondMissingAuth\\(" cmd/api/handlers
```

---

## Viewer-aware privacy (notes/status by ID)

- Public reads do not call privacy-bypassing getters (avoid `Notes.GetNote` on network surfaces).
- Prefer viewer-aware reads (e.g., `Notes.GetNoteWithViewer`) everywhere a status is fetched by ID.

Helpful commands:

```bash
rg -n "GetNote\\(" cmd/api graph | rg -v "GetNoteWithViewer"
```

---

## HTML safety

- Any `text/html` response uses `html/template` (preferred) or escapes all interpolated fields.
- No untrusted data is concatenated into HTML strings or HTML attributes without escaping.
- “HTML-by-contract” JSON fields (Mastodon: `status.content`, `account.note`, `fields[].value`, etc.) are sanitized at
  write time using the shared sanitizer.

Helpful commands:

```bash
rg -n "text/html|Content-Type\\s*:\\s*text/html" cmd graph pkg
rg -n "fmt\\.Sprintf\\(\"<|\\+\\s*\\\"<" cmd graph pkg
```
