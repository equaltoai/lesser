# Phase 2 – Core Data Seeding (dev.lesser.host)

## Persona Accounts

| Persona | Username | Role | Initial Password | OAuth Client ID | OAuth Client Secret | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Admin | `admin` | admin | `zzsuWRko0nc03s00` | `4NQBEFCFIwtk9jd0r4u2Wa` | `TCCH2JOvuh0M3UmNDlgNH77PiObjCoO0KdOJoQZaKjD` | Regenerated 2025-10-21 (`bootstrap_admin_1761048479439/`). Continue using shared JWT secret (`|UQ)…`) in Lambda env. |
| Moderator | `mod` | moderator | `lO81GnTyZj3bK9HL` | `YRXR6QBEE7BwAs0HdGXl1k` | `Chze8Y0tlt300CsVQBDtLuVXqiWm3z6yVKdTIBX6WgZ` | Role + summary patched post-generation (`bootstrap_mod_1761170674818/`). |
| Member | `member` | user | `jRywBTOBTOkFLfGw` | `ovv9JDXyegBtz0rDoPAIVr` | `FFXYtxMrtN4x0q6hMbxC4tZCk9qn0CR1HjBnvWmvY0Z` | Standard persona (`bootstrap_member_1761170680290/`). |
| Locked | `locked` | user | `XxmBNHChBw0LcO5V` | `85KvqrHdLjTfwy05Bz0aPl` | `WKGo5Wq95VRJomwvbbMevT0FudicovWtUXMGLVCwsnw` | `actor.json` updated with `manuallyApprovesFollowers: true`; `Locked.BOOL` toggled (`bootstrap_locked_1761170685184/`). |
| Bot | `bot` | user | `GlTBx1Bap0yssZG6` | `WBJ3KxP0lOc0Ai8hz2uV1Y` | `8VHFaeUGDIgOHaPpiZjnn9gz1pmsNSfBkruRn5u0HZ5` | Actor `type` flipped to `Service` with automation summary (`bootstrap_bot_1761170689672/`). |

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
    "admin":  "4NQBEFCFIwtk9jd0r4u2Wa",
    "mod":    "YRXR6QBEE7BwAs0HdGXl1k",
    "member": "ovv9JDXyegBtz0rDoPAIVr",
    "locked": "85KvqrHdLjTfwy05Bz0aPl",
    "bot":    "WBJ3KxP0lOc0Ai8hz2uV1Y",
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

- 2025-10-22 22:05 UTC: Re-seeded the five baseline personas from the new `bootstrap_*_1761170…` directories. DynamoDB now only contains fresh account + OAuth rows; follower links, media posts, preferences, and push subscriptions were lost in the rebuild and still need to be replayed.  
- Admin authentication continues to work with the shared JWT secret (`|UQ)}RtvjE[+s:$6QS?|kv[ZkOWAVs5680RpZh*[y,]$({dLT7bqPh;b[uPh>_V^`). `actor(username:"admin")` now reports `followers:0`, `following:0`, `statusesCount:0` until the graph is reseeded.  
- GraphQL `createNote` executes successfully on the clean dataset; the 2025-10-22 22:22 UTC run produced status `f5854df2-0cb3-4cd4-9c1d-e473b750ffb0` and queued `STREAM_EVENT#evt_1761171732…` with `TargetID=admin`.  
- Real-time gap: `timelineUpdates` subscriptions connect but fail to persist (`StreamingConnectionRepository.WriteSubscription` hits a DynamoDB `PutItem` timeout for `SUB#user:admin`), so stream-router never delivers a `next` payload. See `docs/graphql-validation-gaps.md` for remediation details.  
- Action items: rebuild the follower graph, republish representative statuses/media, restore user preferences + push subscriptions, then rerun Phase 3 GraphQL validation once the subscription persistence bug is fixed.

## Next Steps

1. Expand follow graph seeding (followers, blocks/mutes) now that the API and GraphQL mutations are stable.
2. Post representative statuses and media to populate timelines prior to Phase 3 validation.
3. Backfill user preference and push-subscription fixtures.
4. Continue updating the checklist/runbooks after each validation pass.
