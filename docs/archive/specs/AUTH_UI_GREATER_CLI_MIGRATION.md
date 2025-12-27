# Auth UI: Greater CLI Vendored Migration

> **Status**: Implemented (MVP)  
> **Created**: 2025-12-23  
> **Completed**: 2025-12-24

## Overview

Migrate the `auth-ui` application from using the `@equaltoai/greater-components` NPM package to the vendored CLI model using the `greater` CLI tool. This aligns with the Greater Components project's architectural shift away from NPM-based distribution.

Related wizard work is specified separately in `docs/specs/AUTH_SETUP_WIZARD_UI.md`.

This migration has been completed; Greater packages are now vendored under `auth-ui/src/lib/greater/*`, with styling applied via local CSS layers.

## Background

### Current State
- **Package**: `@equaltoai/greater-components@1.0.20` installed via NPM
- **Framework**: Astro 5 + Svelte 5
- **Usage**: Primitives (Button, TextField), Icons (KeyIcon, CreditCardIcon), CSS tokens (theme.css)

### Target State
- **Distribution**: Components vendored locally via `greater` CLI
- **Import paths**: `src/lib/greater/*` (Vite alias `src`) for core packages (primitives, icons, tokens, etc)
- **Styles**: two required CSS layers:
  - `src/lib/styles/greater/tokens.css`
  - `src/lib/styles/greater/primitives.css`
- **Dependency**: No runtime dependency on `@equaltoai/greater-components`
- **Hosting (planned)**: Auth UI served from `https://<stage-domain>/auth/*` (same origin as system endpoints)

## Benefits

1. **Ownership**: Full control over component source code
2. **Stability**: No surprise breaking changes from NPM updates
3. **Customization**: Direct modification of components without forking
4. **Consistency**: Aligns with Greater Components' recommended distribution model
5. **Offline**: No NPM registry dependency for builds

---

## Scope

### In Scope

| Component | Description |
|-----------|-------------|
| CLI initialization | Run `greater init` to create `components.json` |
| Component vendoring | Run `greater add` for required packages |
| Path alias configuration | Configure `$lib` alias in tsconfig and Vite |
| Import path updates | Update all imports to use vendored paths |
| Dependency cleanup | Remove NPM package from `package.json` |
| Vite config cleanup | Remove `@equaltoai/greater-components` exclusions |
| Build verification | Ensure `astro build` succeeds |
| Visual verification | Confirm UI renders correctly |

### Out of Scope

| Item | Rationale |
|------|-----------|
| Replacing custom components with `shared/auth` module | Existing WebAuthn/wallet implementation is highly specialized |
| Auth flow changes | Migration is infrastructure only, no behavioral changes |
| New feature development | Focus on parity with current implementation |

---

## Current Usage Inventory

### Files Affected

| File | Current Imports |
|------|-----------------|
| `src/layouts/AuthLayout.astro` | `@equaltoai/greater-components/tokens/theme.css` |
| `src/components/PasswordlessLogin.svelte` | `Button`, `TextField`, `KeyIcon`, `CreditCardIcon` |
| `src/components/OAuthConsentScreen.svelte` | `Button` |

### Import Mapping

| Current (NPM) | Target (Vendored) |
|---------------|-------------------|
| `@equaltoai/greater-components/primitives` | `$lib/greater/primitives` |
| `@equaltoai/greater-components/icons` | `$lib/greater/icons` |
| `@equaltoai/greater-components/tokens/theme.css` | `$lib/styles/greater/tokens.css` + `$lib/styles/greater/primitives.css` |

---

## Implementation Plan

### Phase 1: Setup (15 min)

1. **Initialize Greater CLI**
   ```bash
   cd auth-ui
   greater init
   ```

2. **Configure `components.json`**
   Use the CLI-created file as the source of truth, but ensure it matches the documented schema and vendored model:
   ```json
   {
     "$schema": "https://greater.components.dev/schema.json",
     "version": "1.0.0",
     "ref": "<pin a commit SHA or tag>",
     "installMode": "vendored",
     "style": "default",
     "aliases": {
       "components": "src/lib/components",
       "utils": "src/lib/utils",
       "ui": "src/lib/components/ui",
       "lib": "src/lib",
       "hooks": "src/lib/primitives",
       "greater": "src/lib/greater"
     },
     "css": {
       "tokens": true,
       "primitives": true,
       "face": null,
       "source": "local",
       "localDir": "styles/greater"
     },
     "installed": []
   }
   ```
   Notes:
   - `$lib` is an import alias; for Astro we must wire it up via TS + Vite (see Phase 2).
   - `css.localDir` is relative to `aliases.lib` and should stay `styles/greater` (default).

3. **Add required packages**
   ```bash
   greater add primitives icons tokens utils
   ```

### Phase 2: Configuration (15 min)

4. **Update TypeScript + Vite aliases**
   - Ensure `auth-ui/astro.config.mjs` has a Vite alias for `src`.
   - Ensure `auth-ui/tsconfig.json` includes `paths` support for `src/*` so editor tooling resolves vendored imports.

### Phase 3: Migration (20 min)

6. **Update imports in `AuthLayout.astro`**
   Replace the old single CSS import with the required two-layer Greater CSS imports:
   ```diff
   - import '@equaltoai/greater-components/tokens/theme.css';
   + import 'src/lib/styles/greater/tokens.css';
   + import 'src/lib/styles/greater/primitives.css';
   ```

7. **Update imports in `PasswordlessLogin.svelte`**
   ```diff
   - import { Button, TextField } from '@equaltoai/greater-components/primitives';
   - import { KeyIcon, WalletIcon } from '@equaltoai/greater-components/icons';
   + import Button from 'src/lib/greater/primitives/components/Button.svelte';
   + import TextField from 'src/lib/greater/primitives/components/TextField.svelte';
   + import KeyIcon from 'src/lib/greater/icons/icons/key.svelte';
   + import WalletIcon from 'src/lib/greater/icons/icons/wallet.svelte';
   ```
   Note: prefer direct component imports over barrel exports to avoid pulling unused assets into the bundle.

8. **Update imports in `OAuthConsentScreen.svelte`**
   ```diff
   - import { Button } from '@equaltoai/greater-components/primitives';
   + import Button from 'src/lib/greater/primitives/components/Button.svelte';
   ```

### Phase 4: Cleanup (10 min)

9. **Update `package.json`**
   - Remove `@equaltoai/greater-components` from dependencies

10. **Reinstall dependencies**
    ```bash
    pnpm remove @equaltoai/greater-components
    pnpm install
    ```

### Phase 5: Verification (15 min)

11. **Build verification**
    ```bash
    pnpm build
    ```

12. **Visual verification**
    - Run dev server: `pnpm dev`
    - Test login page renders correctly
    - Test OAuth consent screen renders correctly
    - Verify icons display properly
    - Verify button interactions work

---

## Decisions Required

| # | Decision | Options | Recommendation |
|---|----------|---------|----------------|
| 1 | Ref pinning strategy | `latest`, specific tag, commit SHA | Pin initially (tag/SHA), then update intentionally |
| 2 | `$lib` mapping | `$lib -> src/lib` (recommended), `$lib -> src` | Use `src/lib` to match Greater defaults |
| 3 | Custom auth module | Keep custom, adopt `shared/auth` | Keep custom (current WebAuthn/wallet logic is specialized) |

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Missing CSS layer imports | UI styling breaks | Import both `$lib/styles/greater/tokens.css` and `$lib/styles/greater/primitives.css` |
| `$lib` alias not wired in Astro | Build/runtime import failures | Configure TS `paths` + Vite `resolve.alias` in `astro.config.mjs` |
| Missing transitive dependencies | Build fails | Run `greater add` with verbose output to catch missing deps |
| Svelte 5 compatibility | Components don't render | Already validated - current NPM version works with Svelte 5 |

---

## Success Criteria

- [x] `greater init` creates valid `components.json`
- [x] `greater add primitives icons tokens utils` succeeds
- [x] All import paths updated to vendored equivalents
- [x] `@equaltoai/greater-components` removed from `package.json`
- [x] `pnpm build` succeeds without errors
- [ ] Login page renders identically to pre-migration (manual verification)
- [ ] OAuth consent screen renders identically to pre-migration (manual verification)
- [ ] No runtime console errors (manual verification)

---

## Setup Wizard

The setup wizard is implemented (MVP) per `docs/specs/AUTH_SETUP_WIZARD_UI.md` and served from `https://<stage-domain>/auth/setup`.

---

## Future Considerations

1. **shared/auth module**: Evaluate adopting Greater's auth module for new auth features rather than extending custom implementation
2. **Adapters**: If auth-ui needs to interact with Lesser's GraphQL API directly, consider `greater add adapters`
3. **Auto-updates**: Establish cadence for running `greater update` to pull component updates
