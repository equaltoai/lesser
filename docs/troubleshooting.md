# Troubleshooting

<!-- AI Training: Common problems and verified fixes for Lesser operators -->

This is the “start here” troubleshooting guide. For deeper operational procedures, see `docs/operations/runbook.md`.

## Deployment fails immediately

### Problem: hosted zone not found

**Symptom:** `lesser up` errors about Route53 hosted zone lookup.

**Fix:** Ensure a **public** Route53 hosted zone exists that exactly matches `--base-domain` (e.g. `example.com`).

## Instance is up but “nothing works”

### Problem: timelines empty / publishing forbidden

**Cause:** the instance is still **locked** (expected right after deploy).

**Check:**

```bash
curl -s "https://dev.example.com/setup/status" | jq .
```

**Fix:** complete activation via the setup wizard at `https://<stage-domain>/auth/setup` (if configured for your deploy).

## 404s on API routes

### Problem: `/api/v1/*` returns 404

**Check:** verify you’re hitting the stage apex domain and the correct path prefix:

```bash
curl -s -o /dev/null -w "%{http_code}\n" "https://dev.example.com/api/v1/instance"
```

If you recently changed routing, confirm:

- `/l` and `/l/*` still point at the FaceTheory SSR host
- `/l/_assets/*` still points at the client asset bucket
- `/auth` and `/auth/*` still point at the auth UI bucket
- `/auth/wallet/*` still bypasses the auth bucket and reaches the API origin
- `/.well-known/*` and `/api/*` still fall through to the API origin

## GraphQL issues

### Problem: GraphQL returns 404

**Check:** use the correct stage apex domain and endpoint:

- `POST https://<stage-domain>/api/graphql` (recommended)

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST \
  -H "Content-Type: application/json" \
  -d '{"query":"query { instance { title } }"}' \
  "https://dev.example.com/api/graphql"
```

### Problem: WebSocket subscriptions fail to connect

**Check:** use the GraphQL WebSocket domain (not the REST domain):

- `wss://ws.<stage-domain>`

Browser clients must use the `graphql-transport-ws` subprotocol and pass auth via the `connection_init` payload
(`connectionParams` in most clients). Query string tokens are ignored for GraphQL subscriptions.

Example (`graphql-ws`):

```js
createClient({
  url: "wss://ws.<stage-domain>",
  connectionParams: { Authorization: "Bearer <token>" },
});
```

## Debugging with CloudWatch

- Tail API logs:

  ```bash
  ./lesser logs --app <app> --function api --env dev --aws-profile <profile>
  ```

- Full guide: `docs/operations/cloudwatch-debugging.md`
- Overview: `docs/monitoring.md`
