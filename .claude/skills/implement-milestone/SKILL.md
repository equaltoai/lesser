---
name: implement-milestone
description: Use to execute a single milestone (or GitHub Project phase) of work — feature branch off main, commits per enumerated task, PR review, merge to main. Runs one milestone at a time. Deploys themselves go through deploy-instance.
---

# Implement a milestone

This skill moves lesser work through code, review, and merge to `main`. lesser uses a single-main branch model with feature branches; there is no staging or premain branch. Once a change merges to `main`, `deploy-instance` owns the per-stage rollout.

## Hard preconditions

Do not start without all of the following:

- **A specific milestone named**, coming from `plan-roadmap` or a GitHub Project phase.
- **Clean working tree on `main`** at a known-green commit.
- **MCP tools healthy.** Call `memory_recent` first.
- **`go test ./...` passes** on `main` as of your checkout.
- **`go vet ./...` passes.**
- **`gofmt -l .`** returns empty.
- **Smoke tests are never run.** Contract-adjacent milestones use static contract verification (`./lesser verify ci` or targeted `./lesser verify schema`, `./lesser verify openapi`, `./lesser verify graphql-coverage`) plus targeted tests instead.
- **The enumerated tasks are in-mission work** — not scope growth that slipped through.
- **Specialist walks are complete** for tasks that touch federation trust, API contract, schema, framework consumption, or deploy.
- **Advisor-dispatched milestones** have Aron's authorization from `review-advisor-brief` recorded.

If any precondition fails, stop and surface it.

## Branch and PR setup

One feature branch per milestone. One PR per milestone. One commit per task.

- **Branch name**: descriptive, milestone-scoped. Observed patterns: `aron/<topic>`, `codex/<milestone-identifier>`, `chore/<dep-bump>`, `feat/<feature>`, `fix/<symptom>`. Issue number suffixes welcome: `codex/federation-m1.4g`.
- **Branched from**: `main` at a known-green commit.
- **PR target**: `main`.
- **PR title**: clear. Conventional Commits style welcome (`fix(federation): tighten signature verification`); lowercase present-tense also welcome (`feat: complete federation m1.4g`).
- **Open the PR as a draft** with the milestone goal and an unchecked task list.

PR description template:

```markdown
## Milestone
<short-name> — <goal from roadmap or project README>

## Classification
<security / federation-trust / contract-stability / schema / operational-reliability / AGPL / framework-feedback / bug-fix / test-coverage / dependency-maintenance / docs>

## Surfaces affected
<enumerated>

## Specialist walks referenced
- Federation trust: <...>
- Contract / API: <...>
- Schema: <...>
- Framework: <idiomatic / reported upstream>

## Consumer impact
<operators / Mastodon clients / remote AP peers / sibling repos>

## Tasks
- [ ] <issue 1 title>
- [ ] <issue 2 title>

## Validation
- `go test ./...`
- `go vet ./...`
- `gofmt -l .` (empty)
- `./lesser verify ci` or targeted contract verify modes when contract-adjacent
- Targeted: <specific go test ./path>
- `cdk synth` for representative stage (if CDK changed)

## Stage rollout plan (handoff to deploy-instance)
- [ ] Merged to main
- [ ] Deployed to dev
- [ ] Dev soak complete
- [ ] Deployed to staging (if used)
- [ ] Staging soak complete
- [ ] Deployed to live

## Cross-repo coordination
<any required coordination or "none">

## Advisor-brief authorization (if applicable)
<summary from review-advisor-brief>
```

## The per-task loop

For each issue in the milestone, in enumerated order:

1. **Read the issue.** Confirm acceptance and planned commit subject. If either has drifted, stop and surface it.
2. **`memory_recent`** — refresh recent context.
3. **For bug fixes: add the regression test first.** The test should fail against current code.
4. **Make the change.** Only files in the enumerated paths. Scope creep becomes a new task; it doesn't silently grow the current commit.
5. **Run validation.** `go test ./...` minimum. Focused work: `go test ./pkg/<package>/...`. `go vet ./...`. `gofmt -l .` returns empty.
6. **For contract-adjacent changes**: do not run smoke tests. Use static contract verification (`./lesser verify ci` or targeted `./lesser verify schema`, `./lesser verify openapi`, `./lesser verify graphql-coverage`) plus targeted handler/resolver tests. Regenerate `docs/contracts/graphql-schema.graphql` with `./lesser schema` if GraphQL changed. Update `docs/contracts/openapi.yaml` alongside REST handler changes.
7. **For schema-touching changes**: validate that TableTheory model tags match intended PK/SK/GSI semantics. Run storage-layer tests.
8. **For federation-trust changes**: exercise signing and verification paths with test fixtures. If possible, test against a known Mastodon test server.
9. **For CDK changes**: `cdk synth --context app=<name> --context stage=dev --context baseDomain=<domain>` succeeds. Never set timeouts on synth.
10. **For dependency bumps**: run full suite; AppTheory / TableTheory / AWS SDK bumps can affect unexpected areas.
11. **Commit.** Use the planned subject. First line under 72 characters. Explain the *why* in the body, especially for federation / schema / contract / AGPL-adjacent changes. Never `--no-verify`. Never `--amend` a pushed commit.
12. **Push.** Never force-push.
13. **Check the task off** in the PR description; update the linked GitHub Project item status (Todo → In Progress → Done) via `gh project item-edit` if the project tracks this work.
14. **`memory_append`** only when something worth remembering happened — a federation edge case, a schema subtlety, a framework awkwardness, a Mastodon-client quirk, an AGPL-adjacent discipline finding. Routine commits aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## The mission rule enforced at commit time

Inside a milestone:

- **Every commit must be in-mission.** Scope growth is caught here at latest; if a task is scope growth, revert and invoke `scope-need`.
- **Bug-fix commits follow the test-first pattern.**
- **Dependency-bump commits are isolated** for bisect clarity.
- **CDK commits are isolated** from Lambda code.
- **GraphQL schema-file changes include regenerated `docs/contracts/graphql-schema.graphql`** in the same commit.
- **OpenAPI spec updates** ride with the handler change that affects them.
- **No hardcoded secrets, JWT secrets, signing keys, partner credentials, or `.env` files.**
- **No full-JWT / full-actor-private-key / raw-password / raw-credential logging.**
- **No changes to `AGENTS.md`, branch protection rules, or CODEOWNERS without explicit governance authorization.**
- **No federation-trust weakening.**
- **No breaking contract changes without the completed `preserve-mastodon-api-compat` walk.**
- **No schema changes without the completed `validate-schema` walk.**
- **No framework patches** to AppTheory / TableTheory / CDK constructs in the lesser tree.
- **No AGPL-incompatible dependencies or proprietary blobs.**

## If the test suite goes red mid-milestone

- **Do not** add a "fix tests" commit that touches unrelated code.
- **Do** stop, investigate, surface the failure.
- **Do not** weaken a test to make it pass.
- If the failure is caused by your most recent commit, revert with a new revert commit (not `git reset --hard`) and re-plan the task.

## Finishing the milestone (PR side)

When all tasks are committed, pushed, and linked project items closed (if applicable):

1. Run `go test ./...` one more time on the tip.
2. Run `go vet ./...`, `gofmt -l .`.
3. For contract-adjacent changes, run static contract verification (`./lesser verify ci` or targeted `./lesser verify schema`, `./lesser verify openapi`, `./lesser verify graphql-coverage`) plus targeted tests. Do not run smoke tests.
4. Run `./lesser schema` if GraphQL changed; commit regenerated contract if not already present.
5. Run `cdk synth` if CDK changed.
6. Promote the PR out of draft.
7. Update PR description: check all task boxes (stage-rollout boxes check as `deploy-instance` runs).
8. Request required review.
9. **Leave merging to a reviewer.** You do not merge PRs.

The PR merges to `main`. Hand off to `deploy-instance` for the per-stage rollout.

## Hand off to deploy-instance

Once the milestone is merged to `main`, `deploy-instance` owns:

- `./lesser up --stage dev` deploys per operator
- Dev soak
- `./lesser up --stage staging` (where used)
- Staging soak
- `./lesser up --stage live`
- Post-deploy monitoring
- Release artifact production (GitHub Release, checksums, release notes)

`implement-milestone` does not run deploy commands. Its output is a merged PR on `main` and a handoff.

## What this skill will not do

- Will not implement more than one milestone per run.
- Will not accept scope growth as a task — scope growth redirects to `scope-need`.
- Will not merge PRs — required review is the process.
- Will not skip required review for "small" changes.
- Will not run deploy commands — that's `deploy-instance`.
- Will not run smoke tests; they are not part of the lesser steward workflow.
- Will not skip specialist walks for federation, contract, schema, framework, or advisor-brief work.
- Will not delete published Lambda function versions.
- Will not force-push, amend pushed commits, or rewrite history.
- Will not bump the Go runtime version as part of an ordinary milestone — that is a coordinated change.
- Will not add unsanitized logging, raw credentials, or raw signing material.
- Will not set timeouts on CDK commands.
- Will not patch AppTheory / TableTheory / CDK locally.
- Will not introduce AGPL-incompatible dependencies or proprietary code.
- Will not act on advisor briefs without `review-advisor-brief` having established Aron's authorization.
