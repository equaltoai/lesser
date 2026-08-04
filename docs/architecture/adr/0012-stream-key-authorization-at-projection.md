# ADR 0012: Enforce stream-key authorization invariants at status projection

**Status:** Accepted (2026-08-03)

## Context

Lesser's streaming publisher routes events by a producer-selected stream key.
`broadcastToStream` does not perform per-message authorization. Authorization
therefore cannot be recovered from the payload after a producer selects the
wrong key: **the stream key is the whole authorization control**.

The relevant key classes have different audiences:

- `user:<username>` is private. The subscriber must be authenticated as that
  same local user.
- `public:local` carries public statuses attributed to local actors.
- `public:remote` carries public statuses attributed to remote actors.

An inbound ActivityPub object can cross several producer envelopes before a
DynamoDB Streams event reaches the stream router. Envelope-level guards are
valuable defense in depth, but each is a single point of failure for the path it
guards. If an untrusted remote `attributedTo` value is projected into a canonical
`Status` row as a bare local `AuthorUsername`, downstream code can select
`user:<username>` even when the original activity arrived through a remote
envelope.

## Decision

1. Treat stream-key selection as an authorization decision, not as presentation
   or transport metadata.
2. Establish origin and attribution invariants at the **projection layer**, where
   `BuildCanonicalRemoteStatus` constructs the canonical `Status` row.
3. A remotely supplied Note must not produce a status projection unless its
   attribution survives this fail-closed ladder:
   1. reject empty or unparseable actor URLs;
   2. reject actor URLs with no usable domain anchor;
   3. reject actor hosts matching the configured local host.
4. Preserve a complete remote account identity in `AuthorUsername`; never
   collapse an unanchored or malformed remote attribution into a bare local
   username.
5. Keep envelope guards and stream-router locality checks as independent
   defense-in-depth controls. They do not replace the projection invariant and
   must not be the only barrier protecting private stream keys.

## Consequences

- Every canonical remote status is safe for downstream consumers to classify
  without reconstructing trust from the original ActivityPub envelope.
- A malformed or ambiguous remote actor is rejected before any status-row write,
  so DynamoDB Streams cannot fan it out to a private `user:<username>` stream.
- Honest remote actor paths remain supported when they have a usable, non-local
  domain anchor, including global IPv6 literals.
- Projection becomes a deliberate federation trust boundary. Changes to actor
  identity normalization or `BuildCanonicalRemoteStatus` require adversarial
  tests for private-stream routing consequences.
- Envelope and router checks remain intentionally redundant. This adds code but
  prevents a bypass in one producer path from becoming an authorization breach.

## Verification

- Projection unit tests reject unparseable, no-domain-anchor, and local-host
  attribution before building a `Status`.
- Inbox update tests prove rejected attribution neither rewrites the stored
  object nor writes the status row that would trigger fanout.
- Stream-router tests retain the authenticated-self-only contract for
  `user:<username>` and the `public:local` / `public:remote` split.

## References

- [lesser#1324](https://github.com/equaltoai/lesser/pull/1324)
- [lesser#1298](https://github.com/equaltoai/lesser/issues/1298)
- [`BuildCanonicalRemoteStatus`](../../../pkg/federation/remote_note_status_projection.go)
- [`broadcastToStream`](../../../cmd/stream-router/main.go)
