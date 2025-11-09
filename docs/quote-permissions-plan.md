# Quote Permissions Defaulting Plan

## Objective
Ensure every newly-created user automatically receives a `QuotePermissions` record so quote creation never fails because permissions are missing. Default values should mirror the user’s posting defaults (e.g., public/unlisted → allow public quotes, followers-only → allow followers, etc.). Since only one disposable test account exists, we will wipe it and rely on the new registration logic rather than building a backfill.

## Work Plan

1. **Model & Defaults Review**
   - Revisit `pkg/storage/models/quote_permissions.go:SetDefaults` to confirm it reflects the desired "open" defaults.
   - Define how defaults map from posting visibility (e.g., `public/unlisted` → allow public; `private` → followers; `direct` → mentioned only).

2. **Registration Flow Integration**
   - Locate account creation path (likely in `pkg/services/accounts` and registration handlers).
   - After a user record is created, instantiate a `QuotePermissions` model with defaults derived from the user’s initial posting settings and persist it via `QuoteRepository.CreateQuotePermissions`.
   - Ensure the create call is idempotent so retries don’t fail if the record already exists.

3. **Quote Service Safety Net**
   - Keep `QuoteService.GetQuotePermissions` falling back to defaults if a record is missing, but log a warning so we can detect unexpected gaps.

4. **Tests & Validation**
   - Add unit tests for registration to ensure permissions are saved.
   - Add integration/unit tests for QuoteService to confirm default mapping based on visibility.
   - After shipping, wipe/recreate the lone test account to let the new logic run.

5. **Deployment & Monitoring**
   - Deploy the registration change.
   - After recreating the test account, verify quote creation works and monitor `lesser-development-graphql` logs for any remaining permission warnings.

## Next Session
In the next session we will:
1. Wire the registration flow to create default quote permissions.
2. Adjust/confirm `SetDefaults` + mapping to posting visibility.
3. Add the necessary tests and logging.
