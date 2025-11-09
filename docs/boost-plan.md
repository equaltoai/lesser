# Boost UX Completion Plan

## Goals

- Users should clearly see when they have boosted a status (button state, counters).
- Boosted posts should appear in the appropriate timelines (home/followers) and be undoable.
- Existing streaming/notification plumbing should continue to work; no regressions to ActivityPub Announce delivery.

## Phase 1 – Backend Enhancements
1. **Mutation Response:** Extend `boostObject` GraphQL mutation to return the updated `Object` (with `sharesCount`, `boosted` flag) so the client can update immediately.
2. **Boost State Field:** Add a per-viewer flag on `model.Object` (e.g., `boosted: Boolean!`) populated in `convertStatusToObject` by checking whether the viewer has an active reblog relationship.
3. **Timeline Entry:** Ensure boosts insert the target status into the booster’s home timeline when appropriate (confirm repository logic handles shared posts; if not, add it).
4. **Undo Path:** Confirm `unboostObject` mutation mirrors the response shape and updates counts/flags.
5. **Tests:** Add unit tests for `boostObject`/`unboostObject` resolvers covering new response fields and state computation.

## Phase 2 – Frontend / GraphQL Consumer
1. **Button State:** Update the UI (Greater app) so the boost icon toggles immediately based on the `boosted` flag and mutation response.
2. **Counter Update:** Optimistically update `sharesCount` using the mutation payload.
3. **Timeline Injection:** When a boost is created, optionally insert the shared status into the user’s feed (or mark the original card as “boosted by you”).
4. **Undo UX:** Allow unboosting by clicking the same button; handle disable states while the mutation is in-flight.
5. **Visual Indicator:** Add a badge (“boosted”) similar to other platforms so the user knows they’ve already shared the post.

## Phase 3 – Streaming & Notifications Validation
1. **Streaming Events:** Confirm boost events emit over websockets so connected clients update in real time.
2. **Notifications:** Verify target authors receive the proper notification (already wired) and that the UI displays it.
3. **ActivityPub:** Run federation smoke tests to ensure remote followers still receive Announce activities.

## Phase 4 – QA & Rollout
1. End-to-end manual tests: boost/unboost, verify counts, verify timeline entries, verify state after refresh.
2. Update documentation (README, product docs) describing the boost UX.
3. Deploy to dev/staging, gather feedback, then promote to live.

## Validation Log – Backend Enhancements (Current Pass)
- **Streaming:** `emitReblogEvents` now emits the `status.boosted` event with refreshed `sharesCount` data; tests cover both user and public stream fan-out.
- **Notifications:** Boost actions call the notifications service so authors get a “boosted your post” alert (tested via `notifyBoost` unit coverage).
- **Federation:** Successful boosts enqueue an ActivityPub Announce via the federation service to keep remote followers in sync.
- **Automation:** `make test` verifies the full suite (GraphQL + services) after these changes.
