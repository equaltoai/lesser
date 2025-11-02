# Lesser Auth UI

⚠️ **STATELESS STATIC APPLICATION - NO SESSIONS OR COOKIES PERMITTED** ⚠️

## Architecture

This is a **completely stateless** static web application that provides passwordless authentication UI for Lesser's OAuth flow.

### Key Principles

1. **NO COOKIES**: This application NEVER sets cookies
2. **NO SESSIONS**: This application NEVER creates or manages sessions
3. **JWT-ONLY**: Authentication state is passed via JWTs from Lesser's backend
4. **sessionStorage ONLY**: Used temporarily during OAuth flow, cleared immediately after use
5. **STATIC DEPLOYMENT**: Deployed to S3 + CloudFront as static HTML/JS/CSS

### How It Works

#### Authentication Flow

1. User visits login page
2. User authenticates via WebAuthn or Wallet
3. **Lesser's backend** returns a JWT
4. JWT stored in `sessionStorage` (temporary, tab-scoped)
5. Redirect to Lesser's `/oauth/authorize` with JWT in URL
6. Lesser validates JWT (stateless) and continues OAuth flow
7. JWT cleared from sessionStorage

#### Consent Flow

1. Lesser redirects to `/consent` with JWT in URL
2. Component extracts JWT from URL and stores in sessionStorage temporarily
3. User approves/denies
4. JWT sent to Lesser via Authorization header
5. Lesser validates JWT (stateless) and completes OAuth flow
6. JWT cleared from sessionStorage

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

# Build for production
pnpm build
```

### Deployment

Deployed as static files to:
- **S3 Bucket**: `lesser-auth-ui-{domain}`
- **CloudFront**: CDN distribution
- **DNS**: `auth.{domain}` (e.g., `auth.dev.lesser.host`)

### Important Notes

- **Cross-Origin**: Auth UI runs on `auth.domain`, API runs on `domain`
- **sessionStorage Scope**: Data doesn't persist across origins (by design)
- **JWT in URL**: Temporary, validated once by Lesser, never logged or persisted
- **No Backend**: This is pure static HTML/JS/CSS served from S3

### Security

- JWTs are short-lived and validated by Lesser on every request
- sessionStorage is tab-scoped and cleared after OAuth flow completes
- No persistent state means no session hijacking, no cookie theft
- Cross-origin isolation prevents JWT leakage between domains

### Files

- `src/pages/login.astro` - Login page (static template)
- `src/pages/consent.astro` - Consent page (static template)
- `src/pages/register.astro` - Registration page (static template)
- `src/components/PasswordlessLogin.svelte` - WebAuthn/Wallet login component
- `src/components/OAuthConsentScreen.svelte` - OAuth consent component

**All `.astro` files are static templates with no server-side logic.**
