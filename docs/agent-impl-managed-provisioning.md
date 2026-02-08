# Managed Provisioning (lesser) — Agent Implementation Prompt

You are the implementation agent for the `lesser` repo. Follow this plan exactly, commit after each milestone, and push.

Critical instruction:
- Use the `greater` CLI to update `greater-components` before UI work that depends on it.

Constraints and facts:
- Reserved wallets that must never be used as instance admin wallets:
- `0x80189edb676d51b2fb2257b2ad38e018b20ca46e` (lesser.host admin wallet)
- `0x1e14865a53a994b01b9ccfef42669dc0bfe98805` (Safe + 1% recipient, `TipSplitter.lesserWallet`)
- Managed provisioning should initialize with wallet as admin, then passkey optional.
- Support single-stage deploy for managed provisioning.

Branching and workflow:
1. Create a branch: `git checkout -b feat/managed-provisioning-init-admin`.
2. Work in milestones, 1 commit per milestone.
3. Push after each commit: `git push -u origin feat/managed-provisioning-init-admin`.
4. Update `docs/roadmap-managed-provisioning.md` if plan details change.

Milestone 0: Update `greater-components`
1. Run the `greater` CLI to update `greater-components` to the latest compatible version.
2. Commit: `chore: update greater-components`.

Milestone 1: Single-stage deploy support
1. Add or modify `lesser up` to accept a single `--stage` value for managed provisioning.
2. Ensure it can be called by the CodeBuild runner with only stage and config.
3. Commit: `feat: add single-stage deploy option to lesser up`.

Milestone 2: Init-admin command
1. Implement `lesser init-admin` or equivalent that:
1. Sets an admin account for the instance via wallet signature.
2. Enforces reserved wallet restrictions.
3. Accepts admin username defaulting to slug.
2. Add tests for reserved-wallet rejection.
3. Commit: `feat: init admin wallet flow for managed instances`.

Milestone 3: Provisioning inputs contract
1. Define a structured input contract for `lesser up`/runner:
1. admin wallet
2. admin username
3. stage
4. slug
2. Ensure backward compatibility for legacy flows if required.
3. Commit: `feat: provisioning inputs contract for managed instances`.

Milestone 4: Passkey optional flow
1. Ensure passkey setup is optional and can be completed after first wallet login.
2. Add a minimal UI step to recommend passkeys.
3. Commit: `feat: passkey optional setup flow`.

Milestone 5: Docs and tests
1. Update docs in `docs/roadmap-managed-provisioning.md`.
2. Add tests for init-admin and reserved wallet rules.
3. Commit: `test/docs: managed provisioning init-admin coverage`.

Notes:
- Keep CSP-safe UI changes.
- Coordinate with `lesser-host` for schema changes and runner flags.
