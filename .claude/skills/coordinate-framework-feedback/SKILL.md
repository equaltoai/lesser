---
name: coordinate-framework-feedback
description: Use when building or maintaining lesser surfaces framework awkwardness — an AppTheory middleware limitation, a TableTheory query-builder friction, a FaceTheory (for client surfaces) or greater-components consumption pattern that doesn't fit. Produces a cleanly-shaped signal for the relevant Theory Cloud framework steward rather than a local patch. lesser's role as flagship example means framework-consumption awkwardness here is scope-evidence for framework evolution.
---

# Coordinate framework feedback

lesser is the flagship open-source application example of the Theory Cloud stack. AppTheory and TableTheory took their shape from lesser's original needs; lesser now runs on them. That relationship is reciprocal: when consuming the frameworks is awkward here, that awkwardness is **the canonical signal** for the framework's evolution — not license to patch locally.

This skill handles the signal cleanly. It walks the awkwardness, separates "lesser is expressing the concern wrong" from "the framework has a genuine gap," and produces a shaped report for the relevant framework steward.

## The frameworks lesser consumes

- **AppTheory** (v0.19.1 pinned) — Lambda runtime, middleware chain, MCP server runtime, CDK constructs. Steward: Theory Cloud AppTheory steward.
- **TableTheory** (v1.5.1 pinned) — DynamoDB ORM, single-table tag semantics (`theorydb:"pk"`, `theorydb:"gsi1pk"`, `theorydb:"version"`, `theorydb:"ttl"`, etc.), query builder, model lifecycle. Steward: Theory Cloud TableTheory steward.
- **FaceTheory** (where consumed — lesser's UI-adjacent surfaces, simulacrum's installed client) — SSR/SSG/ISR client delivery. Steward: Theory Cloud FaceTheory steward.
- **greater-components** (sibling equaltoai repo) — Svelte 5 Fediverse UI library consumed by lesser's UI surfaces.

## When this skill runs

Invoke when:

- A handler pattern would require an AppTheory middleware feature or lifecycle hook that doesn't exist
- A model or query pattern would require a TableTheory tag, query-builder capability, or lifecycle semantic that isn't supported
- A CDK construct used by lesser forces a workaround or duplication that feels like a framework gap
- A FaceTheory usage pattern doesn't fit lesser's client-surface needs
- A greater-components API doesn't compose cleanly with lesser's specific needs
- `scope-need` flags a change as "framework-awkward"
- `investigate-issue` surfaces a root cause traced to a framework limitation rather than lesser's own code

## Preconditions

- **The awkwardness is described concretely.** "TableTheory is hard to use here" is too vague; "to query notes-by-actor-and-time-range with pagination, the current TableTheory query-builder requires constructing a manual condition expression rather than supporting a `Between` helper on GSI sort keys, resulting in 15 lines of code where 3 would suffice" is concrete.
- **The idiomatic attempt is captured.** What would the code look like if the framework supported the concern cleanly? Show the idiomatic sketch.
- **The current workaround (if any) is captured.** What does lesser currently do? Is there a workaround, or has the need been blocked?
- **MCP tools healthy**, `memory_recent` first — prior framework-feedback signals are important for continuity (avoid duplicating an already-reported signal).

## The three-step walk

### Step 1: Is lesser expressing the concern wrong?

Before assuming framework limitation, check:

- **Idiomatic framework usage**: what does AppTheory / TableTheory / FaceTheory / greater-components offer for this concern? Consult `query_knowledge` against the framework's knowledge base.
- **Alternative patterns**: is there a different way to express the same concern that fits the framework's grain?
- **Recent framework versions**: the pinned version may lag current capability. Is there a newer version that addresses the concern?

If lesser's usage is bent rather than idiomatic, the fix is local: reshape lesser's code. No framework-feedback signal needed. Proceed to `scope-need` for the local change.

### Step 2: Is the framework genuinely limiting?

If lesser's usage is idiomatic and the framework still doesn't fit, characterize the gap:

- **The concern, concretely**: one-to-two sentences on what lesser is trying to do
- **The ideal framework support**: what would the framework offer if it covered this concern cleanly?
- **The current gap**: specifically what is missing — a middleware hook, a query-builder operator, a tag semantic, a CDK construct pattern, a lifecycle event?
- **The workaround shape (if any)**: what lesser currently does, and what cost that workaround carries (code complexity, test burden, performance impact, maintenance drag)
- **The scope of the gap**: is this specific to lesser's use case, or would other framework consumers benefit? Other known consumers of the framework may have surfaced the same gap.

### Step 3: Shape the signal for the framework steward

The signal is a report, not a PR against the framework. The Theory Cloud framework steward scopes whether and how the framework grows.

Produce:

```markdown
## Framework-feedback signal: <short name>

### Target framework
<AppTheory / TableTheory / FaceTheory / greater-components>

### Framework version in use
<pinned version from lesser's go.mod or equivalent>

### The concern
<one-to-two sentences on what lesser is trying to do>

### The idiomatic code lesser would write if the framework supported it
```<language>
// Code sketch — illustrative, not a PR
```

### The current workaround in lesser (or "blocked")
```<language>
// Current code, with comments on why this is awkward
```

### Cost of the workaround
- Code complexity: <...>
- Test burden: <...>
- Performance impact: <...>
- Maintenance drag: <...>

### Scope of the gap
- Specific to lesser: <yes / likely broader>
- Other known consumers affected: <list if known from query_knowledge — AppTheory, TableTheory, FaceTheory knowledge bases>

### Lesser's workaround posture
- Continue workaround while framework evolves: <yes / no>
- Workaround is temporary / awaits framework: <yes / no>

### Proposed next step
<the framework steward scopes the framework change via the framework's own scope-need flow; lesser's steward does not patch the framework locally>
```

This report goes to the framework steward through the user — you do not edit the framework repo. You surface the signal.

## The explicit refusal to patch locally

The discipline is absolute:

- **No monkey-patches** to AppTheory runtime, middleware, or MCP server code in lesser's tree
- **No forked copies** of TableTheory's query builder or tag handling
- **No CDK construct duplication** that bypasses AppTheory's constructs
- **No "temporary" framework overrides** that accumulate over time
- **No pinning to an unreleased framework commit** that hasn't been published
- **No vendoring** of framework code into lesser's `pkg/` or `internal/`

If the framework genuinely blocks critical work, escalate to the user. The user may decide to prioritize the framework evolution, accept a workaround, or rethink lesser's approach. These are scope-level decisions, not steward-level ones.

## The continuity discipline

Framework-feedback signals accumulate. When a signal is sent:

- **Record in memory** — which framework, what concern, what signal was sent, when. This prevents duplicate-signal noise for the framework steward.
- **Track the framework's response** — when the framework steward responds (via a scoped need, a feature release, a decline, or a redirect), update the memory entry.
- **Revisit on framework version bumps** — when lesser bumps AppTheory or TableTheory, check whether pending signals are now addressed.

## Refusal cases

- **"Just patch this AppTheory middleware locally; the framework steward will get around to it."** Refuse. Local patches degrade the framework's coherence and cost future contributors context.
- **"Fork TableTheory's query builder for a one-off optimization."** Refuse. The optimization belongs in the query builder, or lesser's usage should shape differently.
- **"Skip the framework-feedback signal; we need this to ship."** Refuse. The signal doesn't block lesser's work — lesser documents the workaround and proceeds. The signal goes to the framework steward for their scoping.
- **"Send a framework-feedback signal for every minor awkwardness."** Refuse. Signals are for genuine gaps, not taste differences. Over-reporting dilutes the signal.
- **"Copy the framework's construct into lesser's tree and modify it."** Refuse. Vendoring framework code creates silent drift.
- **"The framework steward isn't responsive, so we should fork."** Escalate to the user. Forking a Theory Cloud framework is a scope-level decision.

## Persist

Append every framework-feedback signal sent. This is high-signal memory material — the record of what lesser has reported to framework stewards is part of the flagship-example feedback loop's artifact. Include: target framework, concern summary, date, framework steward's response (when received), whether the signal resulted in framework evolution.

Five meaningful entries is the right scale; routine "this is slightly awkward" observations don't belong here.

## Handoff

- **Signal shaped and sent to framework steward (via the user)** — stop. Record the signal and continue lesser's local work (documenting the workaround if applicable) through normal `scope-need` → `enumerate-changes` → `implement-milestone` → `deploy-instance`.
- **Signal reveals lesser is using the framework wrong** — route through `scope-need` for the local change; no framework-feedback signal needed.
- **Signal is a duplicate of a prior one** — don't re-send; update memory with the additional data point and let the framework steward know via the user.
- **Signal reveals a framework bug rather than a gap** — framework bugs are investigation work for the framework steward, not scope-need work. Report to user as a bug, not a scoping request.
- **Signal reveals cross-framework impact** (e.g. AppTheory + TableTheory together) — scope via both framework stewards in coordinated conversation.
