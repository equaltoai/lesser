# Federation Follow Reconciliation Validation

This validation gate closes the local follow reconciliation milestone for delivered remote `Accept` and `Reject` responses.

Delivery alone is not success. The round is only complete when Lesser's local follow state tells the truth after the remote response comes back.

## Automated Coverage

These tests cover the contract for new rows and legacy fallback rows:

- `pkg/services/relationships/service_round21_follow_activity_persistence_test.go`
- `pkg/services/relationships/service_round22_relationship_activity_id_contract_test.go`
- `cmd/inbox/internal/routing/follow_response_round13_reconciliation_test.go`

## Live Validation Order

1. Start from a clean Sim state with no pre-existing pending row for the target remote account.
2. Execute one fresh Sim -> Theory follow.
3. Verify Sim creates a pending relationship row.
4. Verify the Sim relationship row stores the canonical full local follow activity URL in `ActivityID`.
5. Verify Sim persists the original outbound local `Follow` activity in `ActivityRepository`.
6. Verify Theory persists the follower row and reaches accepted state.
7. Verify Theory delivers `Accept` back to Sim.
8. Verify Sim moves the same local relationship row from `pending` to `accepted`.
9. Repeat the proof with a clean pending row that receives a delivered `Reject`.
10. Verify Sim reconciles that row to `rejected` without mutating unrelated rows.
11. Resume downstream federation-proof work only after both response paths pass.

## Close Condition

The milestone is closed only when all of the following are true:

- new outbound local follows generate one canonical local activity URL used across queueing, relationship storage, and activity persistence
- delivered remote `Accept` reconciles the local pending follow row
- delivered remote `Reject` reconciles the local pending follow row
- fallback reconciliation only applies to matching local activity URLs and does not mutate mismatched rows
