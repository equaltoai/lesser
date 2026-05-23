---
name: github-provenance
description: "Use when Codex handles GitHub activity for equaltoai/lesser and should preserve routed-agent provenance: reading issues or PRs, commenting, reviewing, creating agent-scoped branches, committing bounded file changes, opening PRs, or creating truthful check runs. Prefer the mcp__lesser_lab__ GitHub tools whenever they can perform the needed operation; fall back to the general GitHub plugin or gh only for capabilities the routed tools do not expose."
---

# GitHub Provenance

## Purpose

Use the routed `mcp__lesser_lab__` GitHub tools first so GitHub activity is attributable to this lesser steward endpoint. Preserve lesser's normal review, CI, schema, API, federation-trust, and AGPL discipline; provenance improves traceability but does not relax any gate.

## Tool preference

1. Prefer `mcp__lesser_lab__` for supported GitHub actions:
   - `github_list_repos`
   - `github_get_issue`
   - `github_get_pr`
   - `github_create_issue_comment`
   - `github_create_pr_review` (comment-only review)
   - `github_create_branch` (agent-scoped branch)
   - `github_commit_files` (bounded file changes)
   - `github_open_pr` (from an agent-scoped branch)
   - `github_create_check_run` (truthful completed check run on an allowed SHA)
2. Use the general GitHub plugin or `gh` only when needed for missing capabilities such as diffs, inline review comments, unresolved threads, Actions logs, labels, search, approvals, or large local git pushes.
3. If a fallback is used for a write, state why in the final response and keep the routed-tool provenance where possible by adding a bounded routed comment/review/check-run only when it is useful and not noisy.

## Read workflow

- Start with `github_list_repos` if repo scope is unknown; do not assume access beyond the returned repositories.
- Use `github_get_issue` / `github_get_pr` for the canonical routed snapshot before deeper inspection.
- Use fallback tools for details the routed snapshot does not expose: patch/diff, review threads, comments, CI logs, commit details, or search.
- Treat routed errors explicitly:
  - `*_forbidden` / `*_not_found`: stop or fall back only if the user clearly has another accessible path.
  - `*_rate_limited` / `*_unavailable`: retry later or use a read-only fallback if timely progress matters.
  - `*_invalid_request`: fix the request; do not retry unchanged.

## Write workflow

Before any GitHub write:

- Confirm the write is part of the user-requested task. Safe reads do not require confirmation; comments, reviews, branches, commits, PRs, labels, and check-runs do.
- Keep comments bounded, factual, and useful. Do not spam provenance-only comments.
- Preserve lesser discipline: no bypassing required review, no force pushes, no skipping `./lesser verify ci` for PR readiness, no production deploy actions.

### Comments and reviews

- Use `github_create_issue_comment` for issue/PR comments that summarize investigation, handoff, validation, or coordination.
- Use `github_create_pr_review` for top-level comment-only reviews when inline placement is not required.
- Use fallback tools only for inline review comments, approvals, requests for changes, reactions, labels, or thread resolution.
- Include a concise provenance footer when appropriate:

```markdown
---
Provenance: posted via lesser steward routed GitHub tool (`mcp__lesser_lab__`).
```

### Branches, commits, and PRs

- Create GitHub branches with `github_create_branch` when publishing work through the routed endpoint. Use a short bounded `branch_suffix` such as `fix-notification-projection` or `docs-github-provenance`.
- Prefer `github_commit_files` when the change set is bounded text files and can be represented exactly by file path + full content.
- Use local editing and tests freely, then read the final changed file contents into `github_commit_files` when feasible.
- Fall back to local `git commit`/`git push` only when the routed commit API is a poor fit, such as large changes, binary assets, generated artifacts that must be preserved exactly, or many-file changes. Explain the fallback.
- Open PRs with `github_open_pr` from the routed branch when possible. PR bodies must include lesser-relevant contract notes: federation trust, Mastodon/API surface, schema impact, AGPL/dependency impact, validation run, and rollout/soak notes where applicable.
- Do not mark a PR ready for review unless the repo-required local gate has passed when applicable:

```bash
go build -o lesser ./cmd/lesser
./lesser build lambdas
./lesser verify ci
```

### Check runs

- Use `github_create_check_run` only for truthful, completed steward evidence on a specific SHA.
- Use a distinct name such as `lesser-steward/local-validation`; never impersonate GitHub Actions or CI jobs.
- Set `success` only when the named commands actually passed. Use `failure`, `neutral`, or `skipped` when that is the truthful result.
- Summaries should list exact commands and notable omissions. Do not claim remote deployment, live soak, or CI success from local checks.

## Final response discipline

When this skill shaped the work, include a short provenance summary:

- Which routed GitHub tools were used.
- Which fallback tools, if any, were used and why.
- Any GitHub artifacts created: branch, commit SHA, PR, issue comment, review, or check run.
- Validation actually run, with explicit note if the full lesser gate was not run.

