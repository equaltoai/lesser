# Federation Follow Establishment Validation

This checklist is the proof contract for the `M1.4F` follow-establishment slice.

Do not treat receiver-side accepted state alone as success. The round is only complete when the outbound response activity returns to the sender and the sender leaves `pending`.

## Automated Regression Focus

Keep these failure shapes covered:

- inbox auto-accept resolves a remote actor on cache miss and still delivers `Accept`
- inbox auto-accept preserves accepted state when remote actor resolution fails and delivery is skipped
- manual accept resolves remote followers without local-only actor assumptions
- manual accept queues `Accept`, not `Follow`
- manual reject resolves the real remote follower and queues `Reject`
- local manual approval remains local-only and does not queue outbound federation

## Live Validation Order

Run this order exactly:

1. Start with one clean Sim -> Theory follow where Theory auto-accepts.
2. Verify Theory persists the follower relationship in `accepted`.
3. Verify Theory emits an outbound `Accept`.
4. Verify Sim receives `Accept` and the local relationship leaves `pending`.
5. Only after the auto-accept loop closes, run one locked/manual approval probe.
6. Verify manual approve emits `Accept` to the resolved remote follower.
7. Verify manual reject emits `Reject` to the resolved remote follower.
8. Only after those pass, resume downstream note, search, timeline, browser, MCP, DM, or cron-loop verification.

## Non-goals For This Round

Do not use this checklist to declare general federation complete. This round is only about truthful two-sided follow establishment across auto-accept and manual approval flows.
