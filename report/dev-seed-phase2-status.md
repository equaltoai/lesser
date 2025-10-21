# Phase 2 – Core Data Seeding (dev.lesser.host)

## Persona Accounts

| Persona | Username | Role | Initial Password | OAuth Client ID | OAuth Client Secret | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Admin | `admin` | admin | `LUpbgE5kovM0W00g` | `Pu7FZdvg3nMC0vYSmNVxXV` | `vQS3wrZGQ9piMgX21Yp6rAGV7B06PGjm8v0sfDTE9Wk` | Created via `scripts/generate_bootstrap_data.js`; assets in `bootstrap_admin_1760887370251/`. |
| Moderator | `mod` | moderator | `AHALRF0dpj25CqSx` | `BzI0JfW3Qn4SzcLtjL8cen` | `4wEp00eIM53zVRcZB2C0rnU9uqE3q3OlTXWThyD0udX` | `user.json` patched to set `"role": "moderator"` before deployment (dir `bootstrap_mod_1760887404485/`). |
| Member | `member` | user | `56WDAZp3Aiy77jxT` | `dS0pcWc1ExFpm4aWHicStV` | `X2nsq2BqL3mfTZy5a8RsoFLNKIxIWHRn97v0NuMbd03` | Standard persona (`bootstrap_member_1760887450104/`). |
| Locked | `locked` | user | `OtnfsibwFg1xYLZc` | `857hQYMIz03ySCNrZYCch8` | `8kXCVsqRpaxVMGy30gGp3bKdBDYDBW67XLkSAu0W06F` | `actor.json` updated with `manuallyApprovesFollowers: true` (`bootstrap_locked_1760887469839/`). |
| Bot | `bot` | user | `tdpAzJpR0U7LhkSH` | `99Shoc0hPrfYrgpKuH0Kjv` | `fwTheoY0OJXuIpMTkTeD6U0DQT73x1uIy89T2u8NJ9U` | `actor.json` updated to `type: "Service"` and bot summary (`bootstrap_bot_1760887501118/`). |

- Each directory contains `actor.json`, `user.json`, `oauth_client.json`, `deploy.sh`, `credentials.txt`, and helper scripts.  All deploy scripts were executed with `AWS_PROFILE=Lesser`, inserting items into `lesser-development`.
- Secrets: JWT signing key pulled from `lesser/jwt-secret` and reused (`JWT_SECRET=|UQ)}RtvjE[+s:$6QS?|kv[ZkOWAVs5680RpZh*[y,]$({dLT7bqPh;b[uPh>_V^`).  Keep this value confidential.
- Temporary Node dependency for bcrypt installed at `tmp/node-deps/node_modules`; use `NODE_PATH=tmp/node-deps/node_modules node scripts/generate_bootstrap_data.js …` to rebuild if needed.

### Token Generation
To mint short-lived Bearer tokens (1 hour expiry) for the personas:

```bash
python - <<'PY'
import base64, json, time, hmac, hashlib
secret = "|UQ)}RtvjE[+s:$6QS?|kv[ZkOWAVs5680RpZh*[y,]$({dLT7bqPh;b[uPh>_V^"
clients = {
    "admin":  "Pu7FZdvg3nMC0vYSmNVxXV",
    "mod":    "BzI0JfW3Qn4SzcLtjL8cen",
    "member": "dS0pcWc1ExFpm4aWHicStV",
    "locked": "857hQYMIz03ySCNrZYCch8",
    "bot":    "99Shoc0hPrfYrgpKuH0Kjv",
}
now = int(time.time())
for user, client_id in clients.items():
    payload = {
        "sub": user,
        "iat": now,
        "exp": now + 3600,
        "nbf": now,
        "username": user,
        "scopes": ["read", "write", "follow", "push"],
        "client_id": client_id,
    }
    header = {"alg": "HS256", "typ": "JWT"}
    def b64url(obj): return base64.urlsafe_b64encode(json.dumps(obj, separators=(',',':')).encode()).rstrip(b'=')
    signing_input = b'.'.join([b64url(header), b64url(payload)])
    signature = base64.urlsafe_b64encode(hmac.new(secret.encode(), signing_input, hashlib.sha256).digest()).rstrip(b'=')
    print(f"{user}: Bearer {signing_input.decode()}.{signature.decode()}")
PY
```

## Current Status

- API Lambda redeployed successfully (`make build` + `make deploy ENV=dev` with `AWS_PROFILE=Lesser`).  
- `GET https://dev.lesser.host/api/v1/accounts/verify_credentials`
  - Returns `401` without credentials (expected).
  - Returns `200` with the freshly minted admin token, hydrating user + actor payload from Dynamo.
- GraphQL `mutation { followActor(id:"member") }`
  - Executes successfully with the admin token.
  - Response includes non-null `published` timestamp and hydrated actor metadata (object payload now populated).
- Follower graph snapshot (2025-10-20 @ 17:45 UTC)
  - `admin` follows 4 (`bot`, `locked`, `member`, `mod`) and is followed by 4 (`member`, `bot`, `mod`, `locked`).
  - `member` now follows `admin`/`mod`/`bot` and is followed by all other personas (4 total).
  - `locked` and `bot` contribute to sparse/dense coverage; see GraphQL counts for exact follow/follower totals.
- GraphQL `updateProfile` mutation
  - Executes successfully (validated 2025-10-20 @ 16:32 UTC); Dynamo version increments logged.
  - Subsequent `actor(id:"admin")` query reflects updated display name and summary.
- GraphQL `createNote` mutation
  - Persists status and returns hydrated activity/object payload for most requests (admin/member/mod/bot verified).
  - Analytics service now uses the correct table but hits `ConditionalCheckFailed` when incrementing daily metrics, causing intermittent HTTP 503 responses (especially for repeat posts on the same day). Logged as a gap.
- Actor status counters
  - `actor(username:...) { statusesCount }` still reports `0` for all personas despite existing `status#…` items, indicating counter backfills are missing.
- GraphQL `timeline` query (`type: HOME`)
  - Currently fails with `failed to get timeline`; likely missing timeline entries in Dynamo. Logged in gaps doc for follow-up.

## Next Steps

1. Expand follow graph seeding (followers, blocks/mutes) now that the API and GraphQL mutations are stable.
2. Post representative statuses and media to populate timelines prior to Phase 3 validation.
3. Backfill user preference and push-subscription fixtures.
4. Continue updating the checklist/runbooks after each validation pass.
