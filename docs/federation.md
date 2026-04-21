# Federation (ActivityPub)

<!-- AI Training: Federation checks and troubleshooting for Lesser -->

Lesser speaks ActivityPub and federates with Mastodon-compatible servers. This doc is the operator/dev “what to check”
guide (not a protocol deep dive).

## Quick checks

Replace `example.com` with your stage domain (`dev.example.com` or `example.com`).

### Discovery (WebFinger + NodeInfo)

```bash
curl -s "https://example.com/.well-known/nodeinfo" | jq .
curl -s "https://example.com/.well-known/webfinger?resource=acct:alice@example.com" | jq .
```

### Actor and object fetch

```bash
curl -s -H "Accept: application/activity+json" "https://example.com/users/alice" | jq .
curl -s -H "Accept: application/activity+json" "https://example.com/objects/<id>" | jq .
```

### Inbox surface truth

```bash
curl -i -X GET "https://example.com/inbox"
curl -i -X OPTIONS "https://example.com/inbox"
```

Expected after the shared inbox repair:

- `GET /inbox` returns `405 Method Not Allowed`
- `POST /inbox` is the real shared-inbox federation ingress
- `/users/{username}/inbox` continues to serve actor-scoped inbox flows

## Locked deployments (expected behavior)

New deployments come up **locked but reachable**.

While locked, you should expect:

- List/timeline endpoints to return empty collections.
- Many write paths (signup/publish) to return `403`.
- `/.well-known/webfinger` to return only the bootstrap actor; other users typically `404`.

Check lock state:

```bash
curl -s "https://example.com/setup/status" | jq .
```

## Where federation lives in this repo

- Protocol HTTP Lambdas: `cmd/actor`, `cmd/inbox`, `cmd/outbox`, `cmd/objects`, `cmd/collections`, `cmd/webfinger`
- Labs validation checklist: `docs/development/shared-inbox-validation.md`
- Core logic: `pkg/activitypub/`, `pkg/services/federation/`

## Troubleshooting

- If discovery works but delivery doesn’t: tail `federation-delivery` logs:

  ```bash
  ./lesser logs --app <app> --function federation-delivery --env dev --aws-profile <profile>
  ```

- If remote servers can’t resolve actors: confirm `/.well-known/webfinger` is routed to the `webfinger` Lambda (not the client UI).

### Problem: delivery queue grows (backlog)

1) Check queue depth (queue URLs are in `state.json`):

```bash
aws sqs get-queue-attributes \
  --queue-url "<queue-url>" \
  --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible
```

2) Tail processor logs (`federation-delivery`, `federation-aggregator`) to identify repeat failures.

### Problem: 401/403 from remote inboxes

Common causes:

- stale/invalid HTTP signatures (key mismatch)
- actor URL doesn’t resolve correctly from the public internet
- remote domain blocks
