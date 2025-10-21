## Phase 1 – Environment & Access Prep

- [x] Confirm `dev.lesser.host` is reachable from the seed runner and GraphQL endpoints respond (`/graphql`, `/graphql/ws`).
- [x] Gather API credentials for seed automation (service account or admin tokens) and store them securely.
- [ ] Verify storage/CDN configuration for media uploads (S3 credentials, VAPID keys, media bucket access).
- [ ] Snapshot current dev data state or document rollback strategy in case the seeding run needs to be reverted.
- [ ] Assemble fixture assets (media files, preference JSON, push subscription stubs) in version control under `testdata/`.

## Phase 2 – Core Data Seeding

- [x] Create baseline personas (admin, moderator, member, locked user, bot) via REST helpers or seed tooling.
- [x] Establish follower/following graph coverage (dense cluster, sparse connections, blocked/muted pairs). _Admin/member/mod/bot/locked now form mutual clusters; see `report/dev-seed-phase2-status.md` for counts._
- [ ] Post representative content: hashtag-heavy threads, spoilerText entries, media attachments with varied `mediaType`, polls, boosts, and replies. _Baseline public posts exist for every persona, but analytics collisions currently return 503 for some `createNote` calls (see gaps doc); media/poll coverage still pending._
- [ ] Upload media assets and ensure alt text, spoiler settings, and media metadata persist.
- [x] Apply profile customization for target personas (display name, bio, avatar, header, metadata fields).
- [ ] Persist server preference states for each persona (streaming, notification, privacy) to mirror expected GraphQL coverage.
- [ ] Register push subscriptions for at least two personas, including one expired/invalid sample.

## Phase 3 – GraphQL Validation Sweep

- [x] Timelines: Query home/local/federated timelines to confirm seeded posts surface with correct ordering and spoiler/media flags. _Home and public timelines now return populated edges after context converter + reseed validation on 2025-10-21; federated timeline remains blocked on remote federation work._
- [ ] Hashtag timelines: Validate pagination and counts for popular and niche tags; ensure media filtering works.
- [ ] Thread context: Fetch conversation context for multi-reply threads and verify ancestor/descendant traversal matches expectations.
- [ ] Followers/following lists: Page through lists for dense and sparse personas, checking counts, relationships, and blocked user exclusion.
- [x] Profile editing: Run GraphQL `updateProfile` mutation and confirm changes appear in subsequent `actor` queries. _Validated 2025-10-20 with admin persona; Dynamo version increment observed in CloudWatch logs._
- [ ] Server preference sync: Query preference state, mutate values, and re-query to verify persistence across fields.
- [ ] Media upload GraphQL flow: Execute upload mutations, attach to posts, and confirm metadata (sensitive, spoilerText, mediaType) integrity.
- [ ] Push notifications: Use mutation to register/update/delete subscriptions, then query notification stream to confirm linkage.
- [ ] Search: Exercise full-text and hashtag search endpoints to confirm seeded data appears as configured.
- [ ] WebSocket subscription: Validate notification stream and any timeline subscriptions remain stable with seeded data.

## Phase 4 – Reporting & Documentation

- [ ] Capture GraphQL query/mutation examples and responses for each validation area in shared documentation.
- [ ] Log discrepancies or missing coverage in an issue tracker with repro steps tied to the checklist items.
- [ ] Update the seeding/validation runbook with lessons learned and manual intervention steps (if any).
