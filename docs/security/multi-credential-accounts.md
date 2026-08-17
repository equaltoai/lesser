# Multi-credential accounts and the last-authenticator rule

<!-- AI Training: the invariants governing how many authenticators an account has and when one may be removed -->

A Lesser account is reachable through one or more **authenticators**. There are
two kinds, and they are interchangeable for the purpose of signing in:

- **passkeys** (WebAuthn credentials) — at least one, at most ten;
- **wallets** — zero or more.

The binding invariant is that the two counts may never both reach zero. An
account with no authenticator cannot be signed into and cannot be recovered:
Lesser has no password, no email reset, and no support channel that can vouch
for you. Losing the last authenticator is not an inconvenience, it is the
permanent loss of the account.

This document states the invariants, how they are enforced, and what is
deliberately never exposed. For how accounts and credentials are created, see
[`docs/architecture/auth/passkey-first-registration.md`](../architecture/auth/passkey-first-registration.md).

## The invariants

### At least one authenticator, always

Every account is created with exactly one authenticator, because
`POST /api/v1/accounts` requires exactly one proof of control and refuses both or
neither. From there the count can grow, and it can shrink — but never to zero.
The rule is enforced at removal time, on both removal surfaces:

- `DELETE /api/v1/auth/webauthn/credentials/{credentialId}`
- `DELETE /auth/wallet/unlink/{address}`

### One to ten passkeys

`pkg/auth.MaxCredentialsPerUser` is 10. `FinishRegistration` refuses an eleventh
(`pkg/auth/webauthn.go`). The ceiling exists because the allow-list an
authentication ceremony carries grows with the credential count, and because ten
devices is already past the point where an account holder can reason about which
key is where.

The refusal is currently reported as a 500 (`ErrMaxCredentialsReached` carries a
quota code that the auth-error switch in `cmd/api/handlers/helpers.go` does not
special-case). The first-party UI therefore does not rely on that response: it
disables "Add passkey" at ten and says why, rather than starting a ceremony the
server will reject
(`auth-ui/src/components/CredentialManager.svelte`, `MAX_PASSKEYS`). A client
that wants a specific message here should apply the same client-side ceiling.

### Zero or more wallets

A wallet is optional at every point in an account's life. An account may be
created without one, may never link one, and may unlink the last one as long as a
passkey survives.

## How the rule is enforced

The naive implementation — count the authenticators, decide, then delete — is
unsafe. Two concurrent removals can each read a state in which a survivor exists,
each conclude they are permitted, and together leave the account with nothing.
That is a real race, not a theoretical one: two browser tabs are enough.

Lesser therefore enforces the invariant **at write time**, as an atomic
condition, so one of two racing removers cleanly loses.

1. **Plan.** `planAuthenticatorRemoval` (`pkg/auth/authenticator_invariant.go`)
   reads both credential kinds and picks a specific **survivor** — another
   passkey if there is one, otherwise a wallet, checking across kinds in both
   directions. If no survivor exists, it returns `ErrLastAuthMethodDelete`
   without touching storage.
2. **Write conditionally.** The delete is submitted as a DynamoDB transaction
   containing the delete of the target *and* a `ConditionCheck` asserting the
   chosen survivor row still exists
   (`pkg/storage/repositories/account_repository_webauthn.go`,
   `DeleteWebAuthnCredentialConditionedOnSurvivor`, and its wallet twin). If a
   concurrent removal deleted the survivor first, the condition fails and the
   whole transaction is refused.
3. **Classify.** A failed transaction is not self-describing — a condition can
   fail because the survivor went away or because the target did — so
   `classifyGuardedWebAuthnRemovalFailure` / `classifyGuardedWalletRemovalFailure`
   re-read to decide which happened, and map to the right error.

The account is never observed in a zero-authenticator state, including under
concurrency.

## The error contract

The three outcomes mean different things and are reported differently. Clients
must keep them apart.

| Outcome | Status | Body `error` | What actually happened |
| --- | --- | --- | --- |
| Invariant refused it | `400` | `cannot delete last authentication method` | Nothing was removed. The account still has exactly what it had. |
| Target already gone | `404` | `credential not found` / `wallet not found` | Nothing was removed *by this call*; it was already absent. The caller's list is stale. |
| Genuine failure | `500` | `failed to delete credential` / `failed to unlink wallet` | Unknown; the authenticator is probably still there. Retrying is reasonable. |

Sources: `cmd/api/handlers/webauthn.go`
(`HandleDeleteWebAuthnCredentialLift`), `cmd/api/handlers/wallet.go`
(`HandleUnlinkWalletLift`).

**Collapsing these into one message is a defect, not a simplification.** The 400
means "you are safe, and here is what to do first"; the 404 means "your view was
out of date, and re-reading fixes it"; the 500 means "we do not know, try again".
Telling somebody who was just protected from locking themselves out that their
passkey is gone, or telling somebody whose network dropped that they are down to
their last authenticator, are both worse than saying nothing.

The first-party UI maps all three distinctly, re-reads the list on a 404 so the
message and the page agree, and reports a transport failure as a transport
failure rather than as a server refusal. The mapping is pinned by browser tests
in `auth-ui/tests/credential-management.spec.ts`; collapsing the 400 branch to a
generic string fails them.

## What is never exposed

### Credential material never reaches a client

- The passkey list response (`WebAuthnCredentialsResponse`) carries **id, name,
  created-at and last-used-at only**. The stored public key, AAGUID, sign count
  and backup flags are not in it (`cmd/api/handlers/webauthn.go`,
  `HandleListWebAuthnCredentialsLift`).
- The wallet list response *does* carry a `public_key` field. The first-party UI
  drops it at the transport boundary into a view type that has no field for it,
  so it cannot be rendered by a later edit
  (`auth-ui/src/components/CredentialManager.svelte`, `WalletView`). Other
  clients should do the same.
- Private key material never exists server-side for passkeys — it stays on the
  authenticator — and is never logged under any circumstance.

### Credential details are never in the ActivityPub actor document

The actor document (`pkg/activitypub/types.go`, `Actor`) has **no field for a
WebAuthn credential or a linked wallet**, and none is added at serialization.
Remote instances learn nothing about how an account authenticates.

The `publicKey` field that *is* present on an actor is the instance's HTTP
Signature key (`PublicKey` / `publicKeyPem`). That is federation identity: it is
the key remote servers use to verify that an activity came from this actor, and
publishing it is required for federation to work at all. It is unrelated to how
a human signs in, and it is not derived from any authenticator.

Both facts matter for the same reason: how you sign in is nobody else's business,
including instances you federate with.

## Removing an authenticator safely

The order that always works:

1. Add the replacement first — a second passkey, or a linked wallet.
2. Confirm it appears in the list at `/auth/credentials`.
3. Remove the old one.

If step 3 answers `400 cannot delete last authentication method`, step 1 did not
land. Nothing was lost; add the replacement and try again.

## Related

- [Passkey-first registration](../architecture/auth/passkey-first-registration.md)
- [Auth error contract](../architecture/auth/auth-error-contract.md)
- [Security gaps inventory](../security-gaps.md)
- [Security guide](../security.md)
