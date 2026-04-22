# Release Notes

## Remote reply-parent acquisition on create-status

`POST /api/v1/statuses` now accepts canonical remote status URLs in `in_reply_to_id` in addition to local status IDs.

When a canonical remote reply parent is not yet materialized locally, Lesser now performs request-scoped remote parent
acquisition on the create path. The resolved parent is reused for:

- canonical `inReplyTo`
- conversation inheritance
- reply audience derivation
- remote parent delivery targeting

### Operator impact

- Create-status requests that reply to unresolved remote parents can now incur synchronous upstream latency.
- Followers-only/private remote parent acquisition uses authorized fetch in the local replying-actor context.
- Read paths do **not** pick up new live remote fetch behavior from this change.

### New create-status failure classes

- `400 Bad Request` — invalid `in_reply_to_id` shape or unsupported identifier form
- `408 Request Timeout` — remote parent acquisition timed out
- `422 Unprocessable Entity` — the parent resolved but is not usable as a reply parent
- `503 Service Unavailable` — remote parent acquisition could not reach a usable upstream

### Monitoring focus

- create-status latency for replies against unresolved remote parents
- distribution of `400` / `408` / `422` / `503` responses on `POST /api/v1/statuses`
- remote reply delivery success after parent acquisition
- structured `remote reply parent acquisition` logs on the `api` Lambda

### Explicit boundary

Direct / DM reply integrity remains conversations-owned and is not changed by this Notes-service remediation.
