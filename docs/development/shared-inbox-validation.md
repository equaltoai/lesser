# Shared inbox validation

Use this after deploying a build that advertises shared inbox support.

## Dev proof sequence

1. **Public Create**
   - Sender delivers to `POST /inbox`
   - Receiver materializes the remote note
2. **Followers-only/private Create**
   - Sender delivers to `POST /inbox`
   - Receiver accepts the activity once and materializes the note
3. **Control activity**
   - Exercise either `Follow` or `Accept` through `POST /inbox`
   - Confirm the relationship state changes as expected
4. **Actor inbox regression**
   - Replay a known-good request to `/users/{username}/inbox`
   - Confirm the existing actor-scoped flow still succeeds

## Endpoint truth checks

```bash
curl -i -X GET "https://dev.<domain>/inbox"
curl -i -X POST "https://dev.<domain>/inbox" \
  -H 'Content-Type: application/activity+json' \
  --data @activity.json
```

Expected:

- `GET /inbox` returns `405`
- `POST /inbox` returns `202` for accepted ActivityPub deliveries
- actor metadata advertises the same shared inbox URL that the route serves
