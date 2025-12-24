# Greater Components: Auth Setup Wizard Requirements

> **Status**: Draft  
> **Created**: 2025-12-24  
> **Depends On**: `docs/specs/AUTH_UI_GREATER_CLI_MIGRATION.md`, `docs/specs/AUTH_SETUP_WIZARD_UI.md`

## Summary

The Auth UI setup wizard (`/setup`) should be implementable using vendored Greater Components with minimal bespoke UI work. This document lists:

1. The **existing** Greater Components we intend to use (MVP).
2. The **optional additions** to Greater Components that would reduce duplication and improve UX consistency.

## Context / Constraints

- Auth UI framework: Astro 5 + Svelte 5 (runes enabled).
- Greater distribution: vendored via `greater` CLI (`installMode: "vendored"`).
- Required CSS layers (per Greater CSS architecture):
  - `$lib/styles/greater/tokens.css`
  - `$lib/styles/greater/primitives.css`
- The setup wizard UI is served from `https://<stage-domain>/auth/setup` and calls system APIs on the same origin (`https://<stage-domain>/…`).
- Wizard state persistence is via `sessionStorage` only (no cookies/server sessions).

## MVP: Required Existing Greater Packages

### Core packages

- `primitives` (forms, alerts, layout, status)
- `icons` (workflow/status icons)
- `tokens` (vendored token helpers, plus required CSS layers)

### Required primitives (MVP)

These components should be sufficient to implement the setup wizard UI described in `docs/specs/AUTH_SETUP_WIZARD_UI.md`:

- `Button` (must support `loading` and `disabled`)
- `TextField` (username input)
- `Checkbox` (acknowledgements + “I understand” steps)
- `Alert` (API errors + warnings)
- `Card` / `Container` / `Section` (page layout)
- `StepIndicator` (step progress)
- `Spinner` and/or `LoadingState` (loading UX)
- `Modal` (finalize confirmation)
- `CopyButton` (copy debug info, URLs, addresses)
- `List` / `ListItem` (simple checklists + summaries)
- `Badge` / `IconBadge` (locked/unlocked indicators)

## Optional Additions (Nice-to-have)

These are **not required** for the MVP, but would reduce repeated UI glue and help keep setup flows consistent across apps.

### 1) `DefinitionList` / `DefinitionItem` (display-only key/value rows)

**Problem:** The wizard needs to render small “status cards” (stage URLs, bootstrap actor identifiers, API status fields) in a consistent, copy-friendly way. Today we’d likely hand-roll a `<dl>` with local CSS.

**Proposed package:** `$lib/greater/primitives`

**Proposed API (sketch):**

- `DefinitionList`
  - Props:
    - `density?: 'sm' | 'md'` (default `md`)
    - `dividers?: boolean` (default `true`)
    - `class?: string`
  - Children: `DefinitionItem` entries
- `DefinitionItem`
  - Props:
    - `label: string`
    - `monospace?: boolean` (default `false`) (applies to value region)
    - `wrap?: boolean` (default `true`)
    - `class?: string`
  - Slots/snippets:
    - default: value content
    - optional `actions`: trailing actions area (copy button / external link)

**A11y requirements:**

- Use semantic `<dl>`/`<dt>`/`<dd>` markup.
- Ensure keyboard focus order reaches any interactive elements in the `actions` region.

### 2) `truncateMiddle()` string util (addresses, JWT-like IDs)

**Problem:** EVM addresses and long IDs are common in setup/debug output. A middle truncation utility avoids repeated local implementations.

**Proposed package:** `$lib/greater/utils`

**Proposed API (sketch):**

```ts
truncateMiddle(value: string, opts?: { head?: number; tail?: number; ellipsis?: string }): string;
```

### 3) Icon gap check (wallet + external link)

If the icon set does not already include a clear wallet icon, add a `wallet` icon export (e.g. `WalletIcon`) to `$lib/greater/icons`. If it already exists, no action is required.

## Notes / Non-goals

- No request for a `Link` component: Greater intentionally uses HTML `<a>` directly (per component inventory guidance).
- No setup-wizard-specific compound component is requested at this stage; the wizard flow should remain application-owned and reflect Lesser’s backend contract.
