## Phase 1 – Environment & Access Prep

- [x] Confirm `dev.lesser.host` is reachable from the seed runner and GraphQL endpoints respond (`/graphql`, `/graphql/ws`).
- [x] Gather API credentials for seed automation (service account or admin tokens) and store them securely.
- [ ] Verify storage/CDN configuration for media uploads (S3 credentials, VAPID keys, media bucket access).
- [ ] Snapshot current dev data state or document rollback strategy in case the seeding run needs to be reverted.
- [ ] Assemble fixture assets (media files, preference JSON, push subscription stubs) in version control under `testdata/`.

## Phase 2 – Core Data Seeding

- [x] Create baseline personas (admin, moderator, member, locked user, bot) via REST helpers or seed tooling.
- [ ] Establish follower/following graph coverage (dense cluster, sparse connections, blocked/muted pairs). _Needs reseed after 2025-10-22 table reset; prior clusters were cleared._
- [ ] Post representative content: hashtag-heavy threads, spoilerText entries, media attachments with varied `mediaType`, polls, boosts, and replies. _Baseline posts were removed with the reset; rebuild once follower graph is back in place._
- [ ] Upload media assets and ensure alt text, spoiler settings, and media metadata persist. _Baseline `uploadMedia` + `createNote` flow verified 2025-10-21; need alt-text/NSFW/spoiler regression checks._
- [x] Apply profile customization for target personas (display name, bio, avatar, header, metadata fields).
- [ ] Persist server preference states for each persona (streaming, notification, privacy) to mirror expected GraphQL coverage.
- [ ] Register push subscriptions for at least two personas, including one expired/invalid sample.

## Phase 3 – GraphQL Validation Sweep

- [x] Timelines: Query home/local/federated timelines to confirm seeded posts surface with correct ordering and spoiler/media flags. _Home and public timelines now return populated edges after context converter + reseed validation on 2025-10-21; federated timeline remains blocked on remote federation work._
- [x] Hashtag timelines: Validate pagination and counts for popular and niche tags; ensure media filtering works. _`hashtagTimeline` now accepts `mediaOnly` and hydrates attachments by fetching full statuses; validated on `dev.lesser.host` (2025-10-22 08:07 UTC) with admin media post for `#lesser` surfacing attachments._
- [x] Thread context: Fetch conversation context for multi-reply threads and verify ancestor/descendant traversal matches expectations. _Admin root note `9a8db6af-c9cf-4ebf-8981-573253880629` with replies from `member` and `mod` returns `replyCount=2`, `participantCount=3`, `syncStatus=COMPLETE`; nested replies (e.g., bot replying to member) require querying that child note’s context separately._
- [x] Followers/following lists: Page through lists for dense and sparse personas, checking counts, relationships, and blocked user exclusion. _Pagination now returns base64 cursors and advances correctly after the relationship repository fix; blocking currently fails to unwind follow rows (DeleteRelationship model registration bug), so blocked accounts may still appear pending storage fix (tracked in gaps doc)._
- [x] Profile editing: Run GraphQL `updateProfile` mutation and confirm changes appear in subsequent `actor` queries. _Validated 2025-10-20 with admin persona; Dynamo version increment observed in CloudWatch logs._
- [x] Server preference sync: Query preference state, mutate values, and re-query to verify persistence across fields. _Admin persona mutation on 2025-10-21 confirmed updates across `reading` and `streaming` preferences._
- [x] Media upload GraphQL flow: Execute upload mutations, attach to posts, and confirm metadata (sensitive, spoilerText, mediaType) integrity. _Verified via GraphQL `uploadMedia` + `createNote` on `dev.lesser.host` (2025-10-22 08:15 UTC); attachments carry alt text + spoiler metadata. Media still needs automated promotion to `ready`—manual Dynamo update applied during validation (tracked in gaps doc)._
- [x] Push notifications: Use mutation to register/update/delete subscriptions, then query notification stream to confirm linkage. _Validated via GraphQL `registerPushSubscription` / `updatePushSubscription` / `deletePushSubscription` on `dev.lesser.host` (2025-10-22 09:08 UTC); repository now reads VAPID keys from Secrets Manager. Manual SQS injection at 2025-10-22 14:24 UTC confirmed the `push-delivery` Lambda decrypts payloads, loads VAPID keys, and prunes invalid subscriptions. Notification creation now wires through the dispatcher, so push jobs are enqueued automatically; GraphQL `notifications` queries return cleanly even when a user has no stored notifications (currently the case in dev)._
- [ ] Search: Exercise full-text and hashtag search endpoints to confirm seeded data appears as configured.
- [ ] WebSocket subscription: Validate notification stream and any timeline subscriptions remain stable with seeded data. _2025-10-22 22:22 UTC: `timelineUpdates` connects (`connection_ack`) but `StreamingConnectionRepository.WriteSubscription` keeps hitting Dynamo timeouts when writing `SUB#user:admin`, so no `next` payload arrives (see gaps doc)._

## Phase 4 – Reporting & Documentation

- [ ] Capture GraphQL query/mutation examples and responses for each validation area in shared documentation.
- [ ] Log discrepancies or missing coverage in an issue tracker with repro steps tied to the checklist items.
- [ ] Update the seeding/validation runbook with lessons learned and manual intervention steps (if any).
