# ADR 0001: DM v1 inbox + request handshake

**Status:** Proposed (2026-02-19)

## Context

- Lesser already stores direct message conversations along with participant metadata (`pkg/services/conversations`, `pkg/storage/models/conversation.go`), but the surfaced GraphQL `Conversation` type only shows a bare minimum (`id`, `accounts`, `lastStatus`, `unread`). DM v1 requires richer semantics: 1:1 only conversations, an explicit “inbox” for accepted conversations, a “requests” bucket for untrusted senders, and a default that only people you follow can push you into an inbox without an explicit handshake.
- Users expect to control when a stranger can message them, so we need to stop automatically delivering every `visibility: DIRECT` note straight to the conversation list.

## Decision

1. **Keep conversations 1:1 by construction.**  The existing storage model (two-participant `Participants` slice, lookup key by sorted participants) already enforces that conversations represent exactly the set of people involved. We will continue to mint and reuse these conversations only when the sender and recipient pair match exactly, so DM v1 does not introduce group chats.
2. **Model inbox vs. request states inside per-recipient metadata.**  Every recipient of a DM maintains a flag (`RequestStatus`: `PENDING`, `ACCEPTED`, `DECLINED`) plus the timestamp the request was delivered. When a recipient does not follow the sender (see #3), the command that creates the message will mark the recipient’s `RequestStatus` to `PENDING` and surface that conversation under a new “requests” channel until it is accepted. Once accepted, the same metadata flips to `ACCEPTED`, the conversation appears in the inbox, and any queued content is delivered normally (no duplicate conversation rows).
3. **Provide a following-only default DM policy.**  New accounts default to a `PrivacyPreferences.defaultDmPolicy` of `FOLLOWING_ONLY`. That means the GraphQL / UI layer will only “accept” requests automatically if the sender is already being followed by the recipient; otherwise they land in `requests`. Administrators and power users can opt into a more permissive policy (`ANYONE`) or stricter one (`FOLLOWING_ONLY`) via the user preferences mutation.
4. **Expose inbox/requests via GraphQL queries.**  The schema will add two dedicated queries (`dmInbox` and `dmRequests`) that filter the participant metadata by request status. This keeps the UI/greater-components code from having to reimplement the filtering logic it already performs on the conversation list today.
5. **Keep data for audit + federation even when requests are declined.**  A declined request is represented in metadata (status flips to `DECLINED`), and the underlying conversation stays intact for logging/federation so that we can respond to abuse without replaying deleted data.

## Consequences

- We have to extend `ConversationParticipantRecord` to store per-participant DM metadata (`RequestStatus`, timestamps, follow-state) and ensure the repository reads/writes that metadata when listing inbox vs. requests.
- GraphQL clients (greater-components) will depend on the new queries/fields, so the contract change must land before client releases so that DM UI can show the separate inbox/request tabs.
- Following-only default simplifies moderation but requires the onboarding UI to explain why strangers land in requests, which is an interface change we need to coordinate with product docs.
- Declined requests still appear in the request feed for a short period so recipients can “reconsider” before cleanup; this introduces extra state we must garbage collect eventually.

## Next steps

- Draft the GraphQL schema diff that adds `dmInbox`, `dmRequests`, and the new metadata fields, and feed it into this M0 ADR set (see companion doc).
- Surface the default DM policy in `PrivacyPreferences` and expose it via `updateUserPreferences`.
- Update streaming/push subscriptions so that inbox/request counts respect the new states.

## References

- `pkg/services/conversations`
- `pkg/storage/models/conversation.go`
- `graph/core.graphql` conversation section
- `docs/planning/lesser-llm-agent-support-roadmap.md` (DM notes)
