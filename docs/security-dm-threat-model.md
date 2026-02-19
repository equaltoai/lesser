# Direct Messages (DM) — Threat Model (v1.1 / M6)

Date: 2026-02-19

This document captures the DM threat model used for **M6** (“Abuse Resistance + Security Hardening”). It focuses on
**DM v1** (1:1 only) with **Inbox + Requests** semantics and serverless GraphQL subscriptions.

## Scope

In scope:
- Local DMs (stored as `Status{visibility=direct}`) + conversation/request lifecycle.
- Authorization + enforcement for sending DMs and creating DM threads.
- Spam/abuse resistance measures at the API/service layer.
- Audit logging for DM-related security events (metadata only).

Out of scope (for now):
- Cross-instance (“federated”) DM abuse signals and global reputation scoring.
- ML moderation / semantic content classification for DM bodies.
- Client-side UX decisions beyond “safe defaults” (e.g., link previews).

## Assets to protect

- **DM content confidentiality** (message bodies, media, attachments).
- **DM metadata privacy** (who messaged whom, when, request states, read/unread signals).
- **User safety** (harassment, spam, unwanted media).
- **Block/mute guarantees** (blocked accounts must not deliver DMs or requests).
- **System availability** (avoid DM pathways becoming a spam amplifier or resource exhaustion vector).

## Adversaries & abuse patterns

- **Single-account spammer**: sends many requests/messages rapidly, tries to force inbox attention.
- **Targeted harasser**: repeatedly reopens requests after declines, uses “message requests” to evade social graph.
- **Blocked actor**: attempts to bypass via thread recreation or alternate endpoints.
- **Media attacker**: sends unwanted or dangerous content via attachments, especially in Requests.
- **Link-preview attacker**: relies on clients fetching URLs to leak recipient IP/device metadata.

## Security controls (implemented in M6)

### 1) Block enforcement (hard deny)

If either side has blocked the other, DM operations are denied:
- `createConversation` (prevents thread recreation as a bypass)
- `sendDirectMessage` / `sendMessage` (prevents delivery in any request state)

Design note: errors are intentionally generic to avoid leaking block direction.

### 2) Message request anti-spam rules (state enforcement)

To prevent spamming within a pending request thread:
- If a recipient’s request state is `PENDING` **and** inbox policy still requires a request,
  additional messages are rejected until the recipient accepts.

This preserves a modern “requests” UX while preventing the “infinite pending thread spam” failure mode.

### 3) Rate limiting (server-side)

Fixed limits are applied per sender (DM namespace):
- **Overall DM send throughput**: `dm_send_total` = **60 / minute**
- **Total request creation attempts**: `dm_request_total` = **20 / hour**
- **Per-recipient request attempts**: `dm_request_to:<recipient>` = **1 / 24 hours**

Notes:
- Rate limiting is bypassed when `DisableRateLimiting=true` (instance operator control).
- These limits are intentionally conservative defaults and can be tuned as usage data grows.

### 4) Content safety defaults (Requests)

To reduce risk in “message requests”:
- **Media attachments are not allowed** in messages that would land in Requests (until accepted).

This reduces the likelihood of malicious or unwanted media reaching recipients before consent.

### 5) Audit logging (metadata only)

Audit events are written for:
- DM send attempts (`dm.send`) — success + failure reasons (no message bodies)
- Request accept/decline (`dm.request.accept`, `dm.request.decline`)
- Relationship block/unblock (`relationship.block`, `relationship.unblock`)

Audit metadata explicitly avoids DM content; only identifiers + counts + lengths are stored.

## Client safety defaults (recommended)

- **No automatic link preview fetching** in DMs, especially in Requests, to avoid IP/device metadata leakage.
- Prefer explicit user action (click) before any remote fetch.

## Residual risks & follow-ups

- **Compromised accounts** can still DM within allowed limits (mitigation: account security, reporting, moderation).
- **Distributed spam** across many accounts may still generate requests (mitigation: optional lesser-host signals, ML).
- **Federated abuse** is not fully addressed here; if DMs federate, additional rate limits and trust checks are needed.

Recommended follow-ups:
- Per-recipient inbound throttles (recipient-side) in addition to sender throttles.
- Optional `lesser-host` spam scoring for DM requests (never as a hard dependency).
- Admin visibility into aggregate DM abuse metrics (counts only, never content).
