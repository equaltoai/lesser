# CLI Authentication (OAuth Device Flow)

Lesser’s CLI authenticates via an OAuth **device authorization** flow so it works in headless environments and
does not need to handle wallets locally. Wallet login + consent happen in the web UI, and the CLI polls until
the user approves.

## Quickstart

Build the CLI:

```bash
go build -o lesser ./cmd/lesser
```

Authenticate:

```bash
./lesser auth login --base-url https://dev.example.com
```

The CLI prints a verification URL and (if needed) a short `user_code`. Complete the wallet login in the browser,
choose the username to authorize (required for wallets with multiple actors), then approve the consent prompt.

Verify your session:

```bash
./lesser auth status --base-url https://dev.example.com
./lesser auth whoami --base-url https://dev.example.com
```

Make an authenticated request:

```bash
./lesser api request \
  --base-url https://dev.example.com \
  --method GET \
  --path /api/v1/accounts/verify_credentials
```

## Headless-friendly mode (automation)

Print machine-readable login instructions:

```bash
./lesser auth login --base-url https://dev.example.com --json
```

## Local credential encryption

The CLI stores a single encrypted session blob locally (includes `client_id` and `refresh_token`) under:

- `~/.lesser/auth/<base_url_hash>/session.enc` (permissions: `0600`)

The encryption key is derived from:

1. `LESSER_AUTH_SECRET` (recommended for headless/CI), or
2. a machine-derived secret (default) + `base_url`.

Options:

- `LESSER_AUTH_SECRET`: stable secret string used to derive the local encryption key.
- `LESSER_AUTH_SECRET_FILE` / `--secret-file`: read the secret from a file path.

## Troubleshooting

- `device authorization timed out`: restart `./lesser auth login` and approve before the code expires.
- `device authorization denied`: restart `./lesser auth login` and approve the consent prompt.
- `refresh token invalid; re-auth required`: run `./lesser auth login` again (tokens were revoked/expired).
- Clear local session: `./lesser auth logout --base-url https://dev.example.com`.

## Server-side safety rails for CLI tokens

Tokens minted via the device flow are classified as `client_class=cli` server-side and are governed by stricter
automation limits (concurrency caps, throttles, and lockouts), regardless of username.

See `docs/configuration.md` for `ALLOW_DEVICE_FLOW` and `CLI_AUTOMATION_*` tuning knobs.

