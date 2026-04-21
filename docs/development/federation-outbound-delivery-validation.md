# Federation Outbound Delivery Validation

This checklist is the proof contract for `M1.4I` complete outbound federation delivery resolution.

Do not treat one green create delivery as success. The round is only complete when both outbound seams stay truthful:

- public/unlisted follower fanout resolves canonical remote followers and reaches them once
- followers-only/private delivery expands the local `/followers` collection into remote delivery targets without duplicating public fanout

## Automated Regression Focus

Keep these failure shapes covered:

- public/unlisted follower fanout resolves canonical remote handles like `user@domain`
- local `/followers` collection expansion happens only inside federation delivery, not generic addressing classification
- public/unlisted recipient delivery skips local `/followers` expansion because follower fanout was already handled
- followers-only/private delivery expands the local `/followers` collection and dedupes against explicit remote recipients by actor identity, not recipient ordering
- outbox public `Create`, `Update`, `Announce`, and `Delete` families all keep the followers-plus-recipients delivery policy
- queue-driven delivery stays recipients-only for followers-only/private activities
- CMS article federation follows the same public/private delivery policy as note families

## Live Validation Order

Run this order exactly on two deployed instances where one remote actor already follows the sender:

1. Pick **sender instance A** and **remote follower instance B**.
2. Confirm B follows the actor on A before the proof starts.
3. Publish one **public note** on A.
4. Verify B receives that note exactly once.
5. Update the same note on A.
6. Verify B receives exactly one outbound update for the same object.
7. Announce/boost the same note from A if that path is enabled in the proof environment.
8. Verify B receives exactly one outbound announce.
9. Delete the note on A.
10. Verify B receives exactly one outbound delete/tombstone.
11. Publish one **followers-only/private** note on A addressed only to the local `/followers` collection.
12. Verify B receives that followers-only/private activity exactly once.
13. If CMS federation is enabled, publish one **public article** and then one **followers-only/private article** from A.
14. Verify B receives each article family exactly once, then verify delete propagation for the public article.
15. Only after all families pass, resume downstream browser, search, timeline, DM, MCP, or second-instance proof work.

## Close Conditions

Do not call the round complete unless all of the following are true:

- no canonical remote follower is skipped because it was treated like a local username
- no remote recipient receives the same public/unlisted activity twice through both fanout and recipient expansion
- followers-only/private delivery reaches remote followers even though the addressed `/followers` collection is local
- no new `failed to get follower actor` warnings appear for canonical remote handles involved in the proof
- `federation-delivery` stays out of DLQ for the proof activities

## Non-goals For This Round

Do not use this checklist to declare general federation complete. This round is only about truthful outbound recipient resolution and delivery semantics for the repaired public/unlisted and followers-only/private families.
