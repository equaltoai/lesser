---
name: github-via-theorymcp
description: Use for any GitHub operation — branches, commits, pull requests, reviews, issues, comments, checks. Prefer the route-integrated TheoryMCP github_* tools (governed, agent-scoped, attributable to your principal) over the gh CLI; fall back to gh only for operations TheoryMCP does not expose, and say so explicitly.
---

# GitHub via TheoryMCP

Your agent route exposes governed GitHub tools. Use them first. They act as your agent principal, on agent-scoped branches, under server-side policy — repo allowlists, merge and closeout gates, attributable provenance. The `gh` CLI runs under ambient local credentials with none of that governance. The integrated tools are the default; `gh` is the fallback.

## The rule

1. **Default to the integrated tools.** For any GitHub action, reach for the `github_*` tools on your TheoryMCP agent route before anything else.
2. **Fall back to `gh` only when TheoryMCP does not expose the behavior** you need — and when you do, name the gap ("TheoryMCP exposes no X, using gh for this step") so the boundary stays visible.
3. **Never use `gh` to bypass a gate** the integrated tools enforce. A refused governed operation is a signal, not an obstacle to route around — honor the refusal.

## Discover what's exposed

Do not assume the tool set from memory — it grows. Discover the live `github_*` tools on your route (via your host's tool discovery / ToolSearch) before deciding something is missing. Typical coverage includes:

- **Branches & commits:** create an agent-scoped branch; commit bounded file sets.
- **Pull requests:** open, update, get, list files/commits/review-threads, merge, close out; create and reply to reviews and review threads.
- **Issues:** create, get, update, comment (create/upsert), close, reopen.
- **Checks:** create check runs; read check-status summaries.
- **Repos:** list accessible repositories.

If your operation maps to any of these, use the integrated tool.

## When gh is legitimate

`gh` is appropriate only for an operation with no TheoryMCP equivalent, and only within a repository you are authorized to touch. State the reason in the moment. Prefer read-only `gh` for gaps; treat a `gh` write that duplicates a governed tool as a red flag.

## Red flags

- Reaching for `gh push` / `gh pr merge` when `github_commit_files` / `github_merge_pr` exist.
- Falling back to `gh` because a governed tool refused you — that refusal is policy; honor it.
- Choosing `gh` without first discovering the route's current `github_*` tools.
- Using `gh` with credentials or in repositories outside your authorized scope.