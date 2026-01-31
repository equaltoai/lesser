# PAI Evidence-as-Code (High-Risk Domains)

This document describes a concept for standardizing “transparent controls” in high-risk domains (finance, healthcare,
etc.) by generating **traceable, reproducible evidence** of controls (or gaps) as part of normal development.

PAI is assumed to already exist with:

- domain-specific knowledge bases (KBs) (for example, DSS KB), and
- LLM access exposed via MCP (so PAI can query KBs and generate drafts in a structured way).

This is **not** a compliance certification system. It is “evidence-as-code”: a repeatable way to answer
“what controls exist today, what do we know, and what can we prove?”

## Executive summary

High-risk domains fail in predictable ways:

- requirements live in PDFs/spreadsheets and drift from engineering reality,
- evidence is assembled late (audit scramble),
- “we have a control” is asserted without a durable, testable proof,
- ownership is ambiguous (controls degrade silently).

PAI Evidence-as-Code turns standards into an engineering execution loop:

1. derive a **controls matrix** from a domain framework (PCI/HIPAA/etc),
2. freeze a **versioned rubric** (deterministic scoring; no moving goalposts),
3. generate a **roadmap** mapped to rubric IDs (work is measurable),
4. install **CI gates + evidence runners** that continuously produce evidence artifacts.

Outcome: any dev team or agent can maintain transparent controls because the system encodes:

- what “good” means (rubric),
- what to do next (roadmap),
- how to verify (commands/gates),
- where to find proof (evidence artifacts).

## Goals / non-goals

### Goals

- Produce a **single source of truth** for control status: `implemented | partial | missing | unknown`.
- Ensure every “implemented” control is backed by **reproducible evidence** (not prose).
- Make control drift visible via **CI regression gates** and periodic evidence refresh.
- Keep the process usable by:
  - human dev teams,
  - autonomous agents operating in a repo,
  - security/compliance reviewers reading artifacts.
- Support multiple domains via **domain packs** backed by KBs.

### Non-goals

- Replace auditors, assessors, or legal/compliance counsel.
- Claim formal compliance (“we are PCI compliant”) as an output.
- Store or redistribute copyrighted standards text in source control when licensing is unclear.

## Core principles

- **Evidence beats assertion**: the system must not mark controls “implemented” without referenced evidence.
- **Determinism first**: prefer machine-verifiable checks (tests, static analysis, IaC assertions) over manual checklists.
- **Traceability**: every claim points to:
  - a requirement ID,
  - a concrete control definition,
  - a verification mechanism,
  - evidence artifacts produced from a known commit/environment.
- **Versioned definitions**: rubrics must be versioned to prevent goalpost drift.
- **Scoped by design**: controls are meaningful only when scope (data flows, systems, environments) is explicit.

## Key artifacts (standard outputs)

These artifacts should be produced in-repo (docs/backlog) while evidence artifacts may be stored in CI artifact storage.

### 1) Controls matrix

A table mapping framework requirements to concrete controls.

Fields (minimum):

- framework + requirement ID (e.g., PCI DSS 3.5.1),
- requirement short name,
- control definition (what we implement),
- verification method (tests/gates/monitors/manual artifact check),
- evidence pointers (paths + commands + CI artifact links),
- owner.

Template: `docs/planning/templates/high-risk-controls-matrix.template.md`

### 2) Rubric (versioned)

Deterministic, pass/fail scoring by category (e.g., Security, Privacy, Compliance Readiness). The rubric is the
definition of “good”, and it is explicitly versioned.

Template: `docs/planning/templates/high-risk-rubric.template.md`

### 3) Roadmap mapped to rubric IDs

Milestones that each close specific rubric IDs with measurable acceptance criteria, verification commands, and evidence
locations.

Template: `docs/planning/templates/high-risk-roadmap.template.md`

### 4) Evidence bundles

Deterministic outputs produced by CI or a local runner, ideally stored as build artifacts and referenced from the repo.

Examples:

- security scan reports,
- dependency vulnerability reports,
- IaC policy diffs/assertion results,
- configuration snapshots,
- contract verification results,
- log-scrubber regression reports.

## Conceptual architecture

### Components

- **PAI CLI**
  - bootstraps the artifacts (controls matrix, rubric, roadmap),
  - installs CI tooling (workflows/jobs) to run verifiers and publish evidence artifacts,
  - provides local commands for developers and agents.

- **Domain pack**
  - identifies a framework (e.g., PCI DSS v4.0.1) and how to interpret requirement IDs,
  - defines recommended control categories and baseline gates (“P0”),
  - provides prompts and heuristics for converting requirements into actionable backlog items.

- **Knowledge base (KB)**
  - local or internal corpus of the standard (and related guidance),
  - queried via MCP so PAI can quote precise requirement language without copying large text into repos.

- **LLM (via MCP)**
  - generates drafts:
    - initial controls matrix suggestions,
    - rubric items and weights (based on risk),
    - roadmap milestones grouped by highest-risk gaps,
  - is constrained to “proposal” mode: it may suggest, but cannot assert “implemented” without evidence.

- **Evidence runner**
  - executes deterministic verifiers (tests, linters, scanners, policy checks),
  - produces timestamped evidence bundles with metadata (commit SHA, environment, tool versions),
  - emits a machine-readable summary (control status).

- **CI integration**
  - runs evidence runner on PRs and on schedule,
  - fails PRs on regression gates for P0 controls,
  - publishes evidence artifacts and updates status summary.

### Data flow (high level)

1. PAI reads scope hints from the repo and user input.
2. PAI queries the domain KB via MCP to enumerate requirements and ensure IDs/titles are correct.
3. The LLM proposes a first-pass controls matrix + rubric + roadmap using templates.
4. Humans/agents confirm scope and edit drafts (reviewable diffs).
5. CI runs the evidence runner to produce artifacts and a status summary.
6. Roadmap updates are driven by “missing/unknown” controls with the highest risk.

## The traceability model (how we prevent hand-waving)

### Control status is evidence-driven

Suggested statuses:

- `unknown`: insufficient info to claim anything; needs investigation/evidence.
- `missing`: we have strong reason to believe the control is not present (or no evidence exists).
- `partial`: control exists but does not meet the stated verification criteria (or is incomplete in scope).
- `implemented`: control meets verification criteria and has current evidence.

### Evidence pointers are explicit

Each control should link to:

- verifier command(s) (or CI job names),
- evidence artifact location(s),
- relevant code/IaC path(s),
- “last verified at” metadata (timestamp + commit SHA + environment).

### LLM outputs are drafts, not truth

The system should encode rules like:

- the LLM may propose mappings and tasks, but cannot set `implemented` without evidence pointers,
- “implemented” requires a verifier to be defined and runnable,
- evidence must be reproducible (a command or deterministic artifact) and tied to a commit.

## Example CLI workflows (illustrative)

Command shapes are illustrative; the key is the lifecycle.

### Bootstrap

- Initialize a domain plan from a KB:
  - `pai controls init --domain pci-dss-v4.0.1 --kb /path/to/kb --out docs/controls/pci/`

### Inspect and refine

- Show status by category / requirement:
  - `pai controls status --format table`
  - `pai controls status --format json`

- Propose a roadmap from gaps:
  - `pai controls roadmap --from-status missing,unknown --max-milestones 6`

### Run evidence and publish

- Run verifiers locally:
  - `pai evidence run --domain pci-dss-v4.0.1`

- Install CI workflows:
  - `pai ci install --domain pci-dss-v4.0.1 --provider github-actions`

## What makes this actionable for dev teams and agents

- **Stable templates**: matrix/rubric/roadmap formats are consistent across domains.
- **Deterministic verification hooks**: “how to verify” is always a command or artifact check.
- **Clear ownership**: each control has an owner and a location in the codebase.
- **Continuous refresh**: evidence isn’t a one-time doc; CI regenerates it.
- **Regression safety**: P0 gates fail fast to prevent drift in critical controls.

## Adoption model (how this lands without slowing engineering)

- Start with a single domain pack (e.g., PCI DSS v4.0.1) and a small set of P0 gates.
- Generate the initial matrix/rubric/roadmap quickly; accept that many items start as `unknown`.
- Convert `unknown → missing/partial/implemented` by adding verifiers and wiring evidence outputs.
- Expand domain coverage over time as evidence automation improves.

## Risks and mitigations

- **False confidence from AI-generated docs**
  - Mitigation: “implemented” requires evidence; LLM outputs are marked draft until verified.

- **Licensing issues with standards text**
  - Mitigation: keep standards content in a local/internal KB; store only IDs + short titles + links/paths in repo.

- **CI brittleness**
  - Mitigation: start with deterministic checks; separate “informational evidence jobs” from “hard gates”.

- **Evidence tampering or ambiguity**
  - Mitigation: include metadata (commit SHA, runner identity, tool versions); prefer signed artifacts where feasible.

## Related Lesser pattern (proven approach)

Lesser’s “10/10” work shows the shape of this approach applied to a codebase:

- versioned rubric: `docs/planning/lesser-10of10-rubric.md`
- roadmap mapped to rubric IDs: `docs/planning/lesser-10of10-roadmap.md`
- process summary and templates: `docs/planning/high-risk-process.md`

