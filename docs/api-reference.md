# API Reference

<!-- AI Training: Canonical API usage patterns for Lesser (REST + GraphQL + streaming) -->

Lesser exposes three primary API surfaces:

- Mastodon-compatible REST API (`/api/v1`, `/api/v2`)
- GraphQL API (`/api/graphql`, `/graphql`) + GraphQL WebSocket subscriptions
- Streaming (Mastodon-style SSE + a Mastodon-style WebSocket streaming API)

This doc focuses on “how do clients actually use this?” patterns (auth, pagination, common requests, streaming, and
schema/contract consumption).

## Base URLs (stage-aware)

For a deployment created with `./lesser up`:

- dev: `https://dev.<base-domain>`
- staging (optional): `https://staging.<base-domain>`
- live: `https://<base-domain>`

When this doc says `<stage-domain>`, substitute one of the above.

## Contracts and sources of truth

Client-facing, file-only contracts (checked into the repo):

- REST OpenAPI: `docs/contracts/openapi.yaml`
- GraphQL published schema: `docs/contracts/graphql-schema.graphql`

Sources of truth:

- OpenAPI is generated from route code (`./tools/openapi`).
- GraphQL schema is authored as modular files in `graph/*.graphql` and aggregated for client teams.

Regenerate/verify:

```bash
./lesser generate openapi
./lesser verify openapi --strict

./lesser schema
./lesser verify schema
```

## Authentication (tokens and headers)

### OAuth access tokens (normal API access)

Most authenticated API calls use:

```http
Authorization: Bearer <access_token>
```

Both REST and GraphQL accept this header.

### Setup tokens (bootstrap / activation)

Some `setup/*` endpoints use a separate bearer token (“setup session”) during initial activation.

If you’re writing a client, treat setup tokens as high-privilege and short-lived.

### Scopes

The OpenAPI spec includes `x-oauth-scopes` metadata on operations. If you generate clients, you can surface scopes in
developer UX (for example, warn when calling an endpoint that requires `admin:read`).

## REST API (Mastodon-compatible)

REST is served from the stage apex domain (`https://<stage-domain>`).

### Pattern: register an OAuth app

```bash
curl -s -X POST "https://<stage-domain>/api/v1/apps" \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "my-client",
    "redirect_uris": "https://my-client.example.com/callback",
    "scopes": "read write follow",
    "website": "https://my-client.example.com"
  }' | jq .
```

### Pattern: exchange an authorization code for a token

The `/oauth/token` endpoint uses `application/x-www-form-urlencoded`:

```bash
curl -s -X POST "https://<stage-domain>/oauth/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "client_id=<client_id>" \
  --data-urlencode "client_secret=<client_secret>" \
  --data-urlencode "grant_type=authorization_code" \
  --data-urlencode "redirect_uri=https://my-client.example.com/callback" \
  --data-urlencode "code=<code>" | jq .
```

### Pattern: call an authenticated endpoint

```bash
curl -s \
  -H "Authorization: Bearer <access_token>" \
  "https://<stage-domain>/api/v1/accounts/verify_credentials" | jq .
```

### Pattern: paginate list endpoints (Link header)

Mastodon-compatible list endpoints return RFC 8288 `Link` headers.

Example (public timeline):

```bash
curl -i -s "https://<stage-domain>/api/v1/timelines/public?limit=20" | sed -n '1,40p'
```

Follow the `rel="next"` URL (it typically includes `max_id=...`) to fetch the next page.

### Error and rate-limit handling

Common responses:

- `401 Unauthorized`: missing/invalid token
- `403 Forbidden`: locked instance or insufficient privilege
- `422 Unprocessable Entity`: validation error (common on write endpoints)
- `429 Too Many Requests`: rate limiting (see `X-RateLimit-*` headers)

## GraphQL

GraphQL HTTP is served from:

- `POST https://<stage-domain>/api/graphql` (recommended)
- `POST https://<stage-domain>/graphql` (alias)

### Pattern: call GraphQL with JSON + variables

✅ CORRECT: use a JSON body with `query` + `variables`.

```bash
curl -s "https://<stage-domain>/api/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query":"query Instance { instance { title } }"}' | jq .
```

### Pattern: authenticated GraphQL query

```bash
curl -s "https://<stage-domain>/api/graphql" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"query":"query Me { viewer { id username displayName } }"}' | jq .
```

### Pattern: paginate a timeline (connections)

The `timeline` query returns an `ObjectConnection` with `edges` + `pageInfo`.

```graphql
query Timeline($first: Int!, $after: Cursor) {
  timeline(type: PUBLIC, first: $first, after: $after) {
    edges {
      cursor
      node {
        id
        url
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

First page:

```bash
curl -s "https://<stage-domain>/api/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query":"query Timeline($first:Int!,$after:Cursor){timeline(type:PUBLIC,first:$first,after:$after){edges{cursor node{id url}} pageInfo{hasNextPage endCursor}}}","variables":{"first":20,"after":null}}' | jq .
```

Then pass `pageInfo.endCursor` back as `$after` to fetch the next page.

### Pattern: create a post (mutation)

The `createNote` mutation accepts a `CreateNoteInput` with `content` and `visibility`.

```graphql
mutation Create($input: CreateNoteInput!) {
  createNote(input: $input) {
    object { id url }
    cost { operationCost }
  }
}
```

```bash
curl -s "https://<stage-domain>/api/graphql" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"query":"mutation Create($input:CreateNoteInput!){createNote(input:$input){object{id url} cost{operationCost}}}","variables":{"input":{"content":"Hello from GraphQL","visibility":"PUBLIC"}}}' | jq .
```

### GraphQL error handling

✅ CORRECT: handle `errors[]` even when `data` is present (partial successes are valid GraphQL behavior).

## Streaming

Lesser supports both Mastodon-style SSE streaming (HTTP) and a Mastodon-style WebSocket streaming API.

### SSE (Mastodon streaming)

SSE endpoints are served under `/api/v1/streaming/*` by the `sse` Lambda (`cmd/sse`).

```bash
curl -N \
  -H "Authorization: Bearer <access_token>" \
  -H "Accept: text/event-stream" \
  "https://<stage-domain>/api/v1/streaming/user"
```

### WebSocket streaming API (Mastodon-style)

The WebSocket streaming API is served from:

- `wss://ws.<stage-domain>/stream`

Auth can be provided as:

- query: `?access_token=<access_token>` (recommended for browser clients)
- header: `Authorization: Bearer <access_token>` (works for non-browser clients that can set headers)

Client message format is JSON:

```json
{ "type": "subscribe", "stream": "public" }
```

Supported stream values include:

- `public`, `public:local`, `public:remote`
- `user`, `user:notification`
- `direct`
- `list:<id>`
- `hashtag:<tag>`

Example (browser/WebSocket):

```js
const ws = new WebSocket("wss://ws.dev.example.com/stream?access_token=" + encodeURIComponent(token));
ws.onopen = () => ws.send(JSON.stringify({ type: "subscribe", stream: "public" }));
ws.onmessage = (ev) => console.log(JSON.parse(ev.data));
```

### GraphQL subscriptions (WebSocket)

GraphQL subscriptions are served from:

- `wss://ws.<stage-domain>` (root of the WebSocket custom domain)

✅ CORRECT: use a GraphQL WebSocket client that supports the `graphql-transport-ws` protocol (for example, the
`graphql-ws` npm package).

Browser clients typically pass auth via query string (WebSocket handshake can’t set custom headers reliably):

```js
import { createClient } from "graphql-ws";

const url = new URL("wss://ws.dev.example.com");
url.searchParams.set("access_token", token);

const client = createClient({ url: url.toString() });
client.subscribe(
  { query: "subscription { timelineUpdates(type: HOME) { id url } }" },
  { next: (data) => console.log(data), error: console.error, complete: () => {} },
);
```

## Federation endpoints

Federation (ActivityPub, WebFinger, NodeInfo) is covered in `docs/federation.md`.

## Setup / activation flows

Activation flows are REST-first (`/setup/*`) and are described in `docs/deployment.md`.
