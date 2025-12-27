# OpenAPI client generation (Greater/TypeScript)

Lesser ships a **file-only** OpenAPI contract at `docs/contracts/openapi.yaml`. It is not served by deployed instances;
use it for **build-time client generation**.

## Recommended TypeScript approach

Use `openapi-typescript` for type generation and `openapi-fetch` for a minimal, typed client.

The spec also includes a vendor extension `x-oauth-scopes` on authenticated operations to indicate the expected OAuth scope(s).

### 1) Generate types

```bash
npx openapi-typescript ./docs/contracts/openapi.yaml -o src/lib/lesser/api/types.ts
```

This generates a `paths` type you can use for a typed fetch client.

### 2) Create a typed client

```ts
import createClient from 'openapi-fetch';
import type { paths } from './types';

export function lesserClient(baseUrl: string) {
  return createClient<paths>({ baseUrl });
}
```

### 3) Auth header injection

The spec models two bearer-token schemes:
- `bearerAuth`: normal API access (`Authorization: Bearer <access_token>`)
- `setupBearer`: temporary setup session access (`Authorization: Bearer <setup_token>`)

Both are the same HTTP header shape; the **token source differs**.

```ts
export function withBearer(token: string) {
  return { Authorization: `Bearer ${token}` };
}
```

Example call:

```ts
const client = lesserClient('https://dev.example.com');
const { data, error } = await client.GET('/api/v1/instance', { headers: withBearer(accessToken) });
```

## Verification (repo-side)

Keep `docs/contracts/openapi.yaml` fresh and strictly typed:

```bash
./lesser generate openapi
./lesser verify openapi --strict
```
