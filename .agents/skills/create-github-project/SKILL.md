---
name: create-github-project
description: Use after plan-roadmap is approved, if the roadmap warrants a tracked GitHub Project at the equaltoai org level. Translates a roadmap document into a Projects v2 kanban board with issues across the affected repos. Follows equaltoai's established project pattern — cross-repo, kanban-driven, README-rich.
---

# Create a GitHub Project

equaltoai tracks initiative-level work in **GitHub Projects v2** at the org level (`github.com/orgs/equaltoai/projects/<N>`). This is the equivalent of Linear for the org — and it's explicitly cross-repo by default. A single project often spans multiple equaltoai repos (lesser, body, host, soul, greater, sim) when the work has coordinated surface area.

This skill turns an approved roadmap into a project board with a clear README, status-kanban, and issues in the right repos.

## Check what tools you have

- **`gh` CLI** with project scope is the primary interface: `gh project create`, `gh project field-list`, `gh project item-add`, `gh issue create`, `gh issue edit --add-project`.
- **Linear MCP tools** — not relevant here; equaltoai uses GitHub Projects.
- If neither is available, produce a well-shaped markdown draft for the principal to adapt.

Surface which mode you're in at the start.

## When this skill runs

Invoke when:

- The roadmap is large enough to benefit from a tracked project (multiple phases, cross-repo coordination, multi-week cadence)
- The roadmap is an initiative (federation-readiness proof, Mastodon-compat hardening, schema migration) rather than a single bug fix
- The principal has asked for a project created

Skip when:

- The roadmap is small enough to track with a handful of issues on a single repo
- No kanban discipline adds value

## The equaltoai project shape (reference)

From observed org practice (e.g., Project 20 — "Federation Readiness — Second Instance Proof"):

- **Title**: short, specific, initiative-named. Format: `<Initiative> — <qualifier>`.
- **Short description**: one-sentence scope.
- **README**: structured by **Goal / Repos involved / Non-goals / Success means / Working method**. The README says explicitly "Treat this as a kanban. Move issues through explicit status as evidence is gathered and blockers become concrete."
- **Status field**: simple three-state kanban — `Todo` / `In Progress` / `Done`. No elaborate state machine.
- **Standard fields (10 total)**: Title, Assignees, Status, Labels, Linked pull requests, Milestone, Repository, Reviewers, Parent issue, Sub-issues progress.
- **Items**: GitHub Issues in one or more of the in-scope repos. Cross-repo is the default, not the exception.
- **Milestones**: separate from Status — milestones aggregate issues into delivery groupings.
- **Parent / sub-issue hierarchy**: used to break non-trivial work into trackable children.
- **Cross-repo**: list the in-scope repos in the README; create issues in the repo where each change actually lands.

## The create walk

### Step 1: Draft the project README

Every equaltoai project has a substantial README. Draft it first so the project is purposeful from creation:

```markdown
## <Initiative title>

<Brief paragraph on what this initiative proves / ships / unblocks.>

### Goal

<The specific outcome. What does "done" look like?>

### Repos involved

- **<repo>**: <specific work scoped to this repo>
- **<repo>**: <...>

### Non-goals

- <explicit out-of-scope items>
- <things this project does not claim to do>

### Success means

- <observable, testable condition>
- <...>

### Working method

Treat this as a kanban. Move issues through explicit status as evidence is gathered and blockers become concrete.
```

### Step 2: Create the project

```bash
gh project create \
  --owner equaltoai \
  --title "<initiative title>"
```

Capture the returned project number `<N>`.

### Step 3: Populate the README

```bash
gh project edit <N> --owner equaltoai \
  --readme "$(cat readme-draft.md)" \
  --description "<short-description>"
```

### Step 4: Confirm the default fields

```bash
gh project field-list <N> --owner equaltoai --format json
```

Expected default fields: Title, Assignees, Status (Todo / In Progress / Done), Labels, Linked pull requests, Milestone, Repository, Reviewers, Parent issue, Sub-issues progress. If any are missing, add as needed.

### Step 5: Create issues and link them

For each enumerated change in the roadmap, create an issue in the repo where it lands:

```bash
gh issue create \
  --repo equaltoai/<repo> \
  --title "<title>" \
  --body "$(cat issue-body.md)" \
  --label "<labels>" \
  --milestone "<milestone>"
```

Issue body template:

```markdown
**Source**: Roadmap <roadmap name>, Phase <phase-short-name>
**Enumerated item**: #<N>

## Paths
<files or directories touched>

## Surface
<cmd / activitypub / federation / services / storage / auth / agents / graph / infra/cdk / docs>

## Classification
<security / federation-trust / contract-stability / schema / operational-reliability / AGPL / framework-feedback / bug-fix / test-coverage / dependency-maintenance / docs>

## Specialist walks referenced
- Federation trust: <none / completed via protect-federation-trust>
- Contract: <none / backward-compatible / breaking — coordination plan>
- Schema: <none / completed via validate-schema>
- Framework: <idiomatic / reported via coordinate-framework-feedback>

## Acceptance criterion
<one sentence>

## Validation commands
<go test ./..., ./lesser verify ci or targeted contract verify modes, cdk synth>

## Stage rollout checkpoints
- [ ] Merged to main
- [ ] Deployed to dev
- [ ] Dev soak complete
- [ ] Deployed to staging (if used)
- [ ] Staging soak complete
- [ ] Deployed to live
- [ ] Post-deploy monitoring verified

## Planned commit subject
<type(scope): subject>

## Parent issue
<link to parent if this is a sub-issue>
```

Then link into the project:

```bash
gh issue edit <issue-number> --repo equaltoai/<repo> --add-project "<initiative title>"
```

Or using `gh project item-add`:

```bash
gh project item-add <N> --owner equaltoai --url <issue-url>
```

### Step 6: Set project fields on each item

For each item in the project, set the relevant fields via `gh project item-edit`:

- **Status**: `Todo` initially (move to `In Progress` / `Done` as work progresses)
- **Milestone**: the roadmap phase
- **Labels**: scope labels consistent with repo conventions (see labeling below)
- **Parent issue**: for sub-tasks of larger work

### Step 7: Establish cross-repo parent/sub hierarchy where appropriate

Use the Parent issue field to break down non-trivial work. Example:

- Parent: `equaltoai/lesser#XXX — "[Federation] Implement FEP-XXX support"`
- Sub-issues:
  - `equaltoai/lesser#YYY — activity-type handling in inbox Lambda`
  - `equaltoai/lesser#ZZZ — delivery signing for new activity type`
  - `equaltoai/greater-components#AAA — UI component for new activity rendering`
  - `equaltoai/lesser#BBB — contract regeneration`

Sub-issues progress is a standard field and is automatically tracked.

## Labels

Apply consistently. Scope labels are repo-specific, but shared label conventions in equaltoai:

- `federation` — federation-trust, delivery, signatures, inbox, outbox
- `mastodon-compat` — REST API contract work
- `graphql` — GraphQL schema or resolver work
- `activitypub` — actor shape, JSON-LD context, FEP adoption
- `schema` — PK/SK/GSI/TableTheory
- `framework-feedback` — AppTheory / TableTheory / greater-components upstream signal
- `AGPL` — license discipline, dependency vetting
- `security` — CVE response, auth hardening
- `reliability` — latency, availability, observability
- `docs` — documentation
- `deps` — dependency bumps
- `breaking` — breaking changes requiring coordination
- `advisor-brief` — issue originated from advisor dispatch
- Surface scopes: `lambda/<name>`, `storage/models`, `activitypub/httpsig`, etc.

## Priority and sequencing

Status field drives the kanban:

- **Todo** — accepted, waiting for work to begin
- **In Progress** — actively being worked
- **Done** — merged, promoted through stages, post-deploy monitoring verified

Milestone field groups items into delivery groupings (typically matching roadmap phases). Milestones progress through the project independent of individual issue status.

Priority is conveyed by label and by project ordering rather than a separate priority field.

## The markdown-draft fallback

If `gh` CLI isn't available or project creation fails, produce the draft for the principal to adapt:

```markdown
# GitHub Project draft: <initiative title>

## Project README
<the README draft from Step 1>

## Default fields
Status: Todo / In Progress / Done
Milestones: <phase names>
Labels: <list>

## Issues

### In equaltoai/lesser
1. **<issue title>** — [`<labels>`]
   - Paths: ...
   - Specialist walks: ...
   - Acceptance: ...
   - Validation: ...
   - Stage rollout checkpoints: ...
   - Parent: <link if applicable>

2. ...

### In equaltoai/<sibling-repo>
1. ...
```

## Persist

When the project exists, append a memory entry with the project URL and the scope it tracks. This helps future-you find the right project for follow-up work. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- Once the project exists and issues are linked, invoke `implement-milestone` with the first in-scope item.
- If the principal wants to revise the roadmap first, go back to `plan-roadmap`.
- If cross-repo coordination surfaces, ensure sibling stewards are looped in before their repos' issues begin work.
- If the roadmap is too small to warrant a project, skip this skill; the roadmap document drives `implement-milestone` directly.