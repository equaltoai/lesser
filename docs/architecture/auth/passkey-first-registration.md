# Passkey-first registration

<!-- AI Training: how a Lesser account is created and which credential proves control -->

Lesser accounts are **passwordless**. There is no password field anywhere in the
system, and there never was one to migrate away from. An account is created by
proving control of a credential, and from then on that credential is how you
sign in.

This document describes the registration paths as they are implemented today,
with the code paths cited so the description can be checked rather than
believed. For the invariants that govern how many credentials an account may
have and when one may be removed, see
[`docs/security/multi-credential-accounts.md`](../../security/multi-credential-accounts.md).

## The two ways to create an account

A passkey and a wallet are **equal alternatives**. Neither requires the other,
and the registration form offers both without preferring one
(`auth-ui/src/components/PasswordlessLogin.svelte`).

`POST /api/v1/accounts` accepts exactly one of two proofs, and refuses the
request if it carries both or neither
(`cmd/api/handlers/accounts.go`, `HandleRegistrationLift`; the `oneOf` is
declared on `AccountRegistrationRequest` in `docs/contracts/openapi.yaml`):

- `passkey_registration_proof` — a single-use proof minted by the public passkey
  signup ceremony.
- `wallet_challenge_id` — a verified wallet challenge.

Username plus an agreement checkbox is **not** sufficient. That was the gap
closed in `docs/security-gaps.md` under "Registration proof missing".

### Passkey-first signup

The signup ceremony is deliberately separate from the login ceremony, because a
username that does not exist yet owns no credentials — asking `login/begin` about
it fails with "no passkeys registered for this user" by construction.

1. `POST /api/v1/auth/webauthn/signup/begin` with the desired username. Returns
   WebAuthn creation options and a challenge
   (`pkg/auth/webauthn.go`, `BeginSignup`).
2. The browser runs `navigator.credentials.create()`.
3. `POST /api/v1/auth/webauthn/signup/finish` with the attestation. The server
   verifies the ceremony binding and stores a **single-use**
   `PasskeyRegistrationProof`, returning its id
   (`pkg/auth/webauthn.go`, `FinishSignup`).
4. `POST /api/v1/accounts` with `passkey_registration_proof`. The proof is
   validated, bound to the same username, and marked consumed; the credential row
   becomes live only once the account exists
   (`pkg/services/accounts/service.go`).
5. The UI then signs in with the new passkey, so the person lands authenticated
   rather than on a login form.

Nothing in this path needs a browser extension.

### Wallet signup

1. `POST /auth/wallet/challenge` for a challenge bound to the username.
2. The wallet signs the challenge message (`personal_sign`).
3. `POST /auth/wallet/verify` verifies the signature.
4. `POST /api/v1/accounts` with `wallet_challenge_id`. On success the server
   marks the challenge `registrationCompleted`
   (`pkg/services/accounts/service.go`,
   `MarkWalletChallengeRegistrationCompleted`).
5. `POST /auth/wallet/link` links the wallet to the new account.

That `registrationCompleted` flag is a security gate, not bookkeeping: it is the
only thing that authorises the one **unauthenticated** call to
`/auth/wallet/link`, so an unauthenticated caller cannot link a wallet onto
somebody else's existing account (`cmd/api/handlers/wallet.go`,
`HandleLinkWalletLift`). Linking also requires the signature, a challenge bound
to the same username, and a challenge that has not already been spent.

Nothing in this path needs a passkey.

## Adding credentials to an existing account

Additional passkeys require authentication. The endpoints are distinct from the
signup ones and sit behind a bearer token:

- `POST /api/v1/auth/webauthn/register/begin`
- `POST /api/v1/auth/webauthn/register/finish`

(`cmd/api/handlers/webauthn.go`, `HandleBeginWebAuthnRegistrationLift` /
`HandleFinishWebAuthnRegistrationLift`.)

`BeginRegistration` does not send `excludeCredentials`
(`pkg/auth/webauthn.go`), which is what lets one account accumulate passkeys
across several devices.

Authenticated wallet linking uses the same `/auth/wallet/{challenge,link}` pair
with a bearer token; the token names the account and the signature proves control
of the wallet.

The first-party UI for all of this is `/auth/credentials`
(`auth-ui/src/components/CredentialManager.svelte`).

## The setup-admin bootstrap

A freshly deployed instance is **locked**. Activating it creates the first admin
account, and that account can be created **passkey-only** — no wallet is linked
to it at all.

The bootstrap wallet whose mnemonic `./lesser up` wrote to
`~/.lesser/<app>/<base-domain>/bootstrap.json` authenticates the *setup session*,
not the admin account:

1. `POST /setup/bootstrap/challenge` — only the configured bootstrap address is
   accepted.
2. `POST /setup/bootstrap/verify` — the signature is verified and a one-hour
   setup session token is issued.
3. `POST /setup/admin` — creates the admin. The body carries **exactly one** of
   `wallet` or `passkey_registration_proof`; both or neither is a 400
   (`cmd/api/handlers/setup.go`, `parseSetupCreateAdminRequest`). With a passkey
   proof, no wallet is linked to the admin account, so the bootstrap wallet never
   becomes an actor credential.
4. `POST /setup/finalize` — unlocks the instance.

### Retry semantics

`POST /setup/admin` can be retried, but a retry that finds an existing user row
is **not** treated as success by itself
(`pkg/services/accounts/service.go`, `RegisterAccountModeSetupAdminBootstrap`).

- If the persisted state proves a safe partial completion — the user row and the
  actor row are both intact and the credential/proof state is consistent — the
  retry self-heals by promoting the existing user to admin.
- Otherwise it fails with a `SetupAdminBootstrapStateError`, surfaced as a 422
  whose message names each piece of state an operator needs in order to repair
  it: actor presence, role, credential binding, and whether the proof was
  consumed.

The point of the distinction is that a half-created admin must never be reported
as a working one. An operator who retries into a broken state gets told exactly
what is broken.

## Registration over GraphQL is deprecated

The `registerAccount` mutation is deprecated in the schema and fails closed:

```graphql
registerAccount(input: RegisterAccountInput!): RegisterAccountPayload!
  @deprecated(reason: "Registration is not supported over GraphQL; use POST /api/v1/accounts.")
```

(`graph/core.graphql`, mirrored in `docs/contracts/graphql-schema.graphql`.)

It was removed as a functioning path because it offered account creation without
proof of credential control — the same gap that `POST /api/v1/accounts` now
closes. The resolver (`graph/mutation_resolvers_accounts.go`) returns an
actionable error pointing at the REST endpoint rather than silently doing
nothing.

## Canonical storage

Every credential lives in the single DynamoDB table, account-scoped. The
authoritative field-level definitions are the models; the key patterns are:

| Entity | PK | SK | GSI |
| --- | --- | --- | --- |
| Account | `USER#{username}` | `ACCOUNT` | — |
| WebAuthn credential | `USER#{username}` | `WEBAUTHN_CRED#{credentialID}` | GSI1: `WEBAUTHN_CREDENTIAL#{credentialID}` / `USER#{username}` |
| WebAuthn challenge | `CHALLENGE#{challenge}` | `WEBAUTHN` | — (TTL-expired) |
| Wallet credential | `USER#{username}` | `WALLET#{address}` (lowercased) | — |
| Wallet reverse index | `WALLET#{walletType}#{address}` | `USER#{username}` | — |
| Wallet challenge | `WALLET_CHALLENGE#{id}` | `CHALLENGE` | — (TTL-expired) |

Sources: `pkg/storage/models/webauthn_credential.go`,
`pkg/storage/models/webauthn_challenge.go`, `pkg/storage/models/wallet.go`.

Two consequences worth stating explicitly:

- **Credentials are account-scoped by partition.** Listing an account's
  authenticators is a single query on `USER#{username}`; there is no cross-account
  scan, and no path that returns another account's credentials.
- **GSI1 on the WebAuthn credential exists for lookup by credential id**, which
  is what an assertion carries. It projects back to `USER#{username}`, so the
  owning account is recovered from the credential rather than trusted from the
  request.

Challenges carry a `theorydb:"ttl"` tag and expire on their own; a challenge that
outlived its window cannot be replayed because it is no longer there to read.

## Related

- [Multi-credential accounts and the last-authenticator rule](../../security/multi-credential-accounts.md)
- [Auth error contract](auth-error-contract.md)
- [Passwordless OAuth](PASSWORDLESS_OAUTH.md)
- [Owner bootstrap requirements](OWNER_BOOTSTRAP_REQUIREMENTS.md)
