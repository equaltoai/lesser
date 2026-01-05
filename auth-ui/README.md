# Lesser Auth UI

⚠️ **STATELESS STATIC APPLICATION - NO SESSIONS OR COOKIES PERMITTED** ⚠️

## Architecture

This is a **completely stateless** static web application that provides passwordless authentication UI for Lesser's OAuth flow.

### Key Principles

1. **NO COOKIES**: This application NEVER sets cookies
2. **NO SESSIONS**: This application NEVER creates or manages sessions
3. **JWT-ONLY**: Authentication state is passed via JWTs from Lesser's backend
4. **sessionStorage ONLY**: Used temporarily during OAuth flow, cleared after completion
5. **STATIC DEPLOYMENT**: Deployed to S3 + CloudFront as static HTML/JS/CSS

### How It Works

#### Authentication Flow

1. User visits login page
2. User authenticates via WebAuthn or Wallet
3. **Lesser's backend** returns a JWT
4. JWT stored in `sessionStorage` (temporary, tab-scoped)
5. Auth UI calls `/oauth/authorize?mode=ui` with `Authorization: Bearer <jwt>`
6. Lesser returns `{ next_url }` to either consent UI or the client callback
7. JWT cleared from sessionStorage after flow completion

#### Consent Flow

1. Lesser redirects to `/auth/consent` for user approval
2. User approves/denies
3. Auth UI POSTs `/oauth/consent` with `Authorization: Bearer <jwt>`
4. Lesser returns `{ redirect_uri }` for the client callback
5. JWT cleared from sessionStorage

### Technology Stack

- **Astro**: Static site generator
- **Svelte 5**: Component framework
- **Greater Components**: UI component library
- **TypeScript**: Type safety

### Development

```bash
# Install dependencies
pnpm install

# Run dev server (port 4322)
pnpm dev

# (Optional) Point auth-ui at a different API origin in local dev
PUBLIC_LESSER_API_ORIGIN=http://localhost:8080 pnpm dev

# Build for production
pnpm build
```

### Deployment

Deployed as static files to:
- **S3 Bucket**: stage `auth-ui` bucket (see `lesser up` outputs)
- **CloudFront**: stage distribution on the stage apex domain
- **Path**: `https://<stage-domain>/auth/*`

### Important Notes

- **Single-Origin**: Auth UI and API share the same origin (CloudFront routes by path)
- **sessionStorage Scope**: Data doesn't persist across tabs (by design)
- **No JWT-in-URL**: OAuth continues via UI-mode and Authorization headers
- **No Backend**: This is pure static HTML/JS/CSS served from S3

### Security

- JWTs are short-lived and validated by Lesser on every request
- sessionStorage is tab-scoped and cleared after OAuth flow completes
- No persistent state means no session hijacking, no cookie theft
- No token-in-URL reduces accidental leakage via referrers/logs

### Files

- `src/pages/login.astro` - Login page (static template)
- `src/pages/consent.astro` - Consent page (static template)
- `src/pages/register.astro` - Registration page (static template)
- `src/components/PasswordlessLogin.svelte` - WebAuthn/Wallet login component
- `src/components/OAuthConsentScreen.svelte` - OAuth consent component

**All `.astro` files are static templates with no server-side logic.**
