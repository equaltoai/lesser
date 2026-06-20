# Release branching and branch protection

Lesser uses the release-alignment branch model:

```text
feature branch -> staging -> main -> manual v* release tag
```

## Branch roles

- **Feature branches** (`aron/*`, `chore/*`, `codex/*`, `feat/*`, `fix/*`, `milestone/*`) branch from current `main` and open PRs to `staging`.
- **`staging`** is the integration branch. Feature -> staging PRs require the existing GitHub Actions `verify` job, which runs `./lesser verify ci`. The staging protection spec requires branches to be up to date before merge.
- **`main`** is canonical, always deployable, protected, and operator-owned. Main promotion accepts PRs from `staging` only. Do not require the `verify` check on `main`; staging -> main promotion is intentionally branch-rules/default-checks only and must not re-run the lesser verify/rubric gate as a required check.
- **Releases** are manual `v*` tags cut from `main`. The release workflow asserts the tagged commit is an ancestor of `origin/main` before publishing assets.

`premain` is not part of lesser's active release model. Treat stale `premain` refs as legacy unless an operator explicitly directs cleanup.

## Branch-protection specs

The committed specs are:

- `.github/branch-protection/staging.json`
- `.github/branch-protection/main.json`

They contain two layers:

1. `policy`: the human-readable Lesser release policy.
2. `github_branch_protection.payload`: the exact GitHub REST branch-protection payload to apply.

GitHub classic branch protection does not expose a PR head-branch allowlist field. The `main` spec therefore records `allowed_pr_sources: ["staging"]` as the operator merge policy, while the API payload enforces the machine-enforceable parts: required PR, operator-owned branch updates, no direct pushes, no force-pushes, and no required `verify` status check on `main`.

## Operator apply commands

Branch-protection application is an operator action. Run these from a checkout containing the committed specs after confirming the operator actor restrictions in `main.json` are still correct.

```bash
# staging: feature -> staging requires existing verify and up-to-date branches
jq '.github_branch_protection.payload' .github/branch-protection/staging.json \
  | gh api --method PUT \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2026-03-10" \
      /repos/equaltoai/lesser/branches/staging/protection \
      --input -

# main: operator-owned; PRs only by policy from staging; no required verify check
jq '.github_branch_protection.payload' .github/branch-protection/main.json \
  | gh api --method PUT \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2026-03-10" \
      /repos/equaltoai/lesser/branches/main/protection \
      --input -
```

## Operator proof commands

After applying, capture the live protection dumps:

```bash
gh api /repos/equaltoai/lesser/branches/staging/protection | jq .
gh api /repos/equaltoai/lesser/branches/main/protection | jq .
```

For a live negative test, confirm direct pushes to `main` and force-pushes to both protected branches are rejected. Because classic branch protection cannot machine-enforce the PR source branch, the operator-owned staging-only promotion rule is verified by review/merge discipline: reject or retarget any PR to `main` whose head branch is not `staging`.
