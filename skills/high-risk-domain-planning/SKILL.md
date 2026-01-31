---
name: high-risk-domain-planning
description: Standardize “high-risk domain” engineering work (finance, healthcare, regulated data) by producing a versioned rubric, milestone roadmap, controls matrix, and evidence plan. Use when planning or auditing systems that handle sensitive data (CHD/SAD, PHI, PII, secrets) or need compliance-oriented hardening (e.g., PCI DSS, HIPAA, SOC2) with measurable verification and CI regression gates.
---

# High-Risk Domain Planning

## Overview

Produce a repeatable, audit-friendly plan for high-risk domains:

1. Controls matrix (requirements → controls → verification → evidence)
2. Versioned rubric (deterministic pass/fail scoring)
3. Roadmap (milestones mapped to rubric IDs)
4. Gates (CI/local verifiers to prevent regressions)

Prefer automation and deterministic verification. Record assumptions and scope explicitly.

## Workflow

### Step 0 — Ask scoping questions (minimum set)

- What domain(s) apply (finance, healthcare, other)?
- What sensitive data exists (CHD/SAD, PHI, PII, secrets) and where does it flow?
- What are the system boundaries (in-scope services, accounts, environments, vendors)?
- What assurance level is needed (hardening vs audit-ready evidence vs certification)?
- What framework(s) are in scope (PCI DSS, HIPAA, SOC2, internal policy)?

### Step 1 — Build the controls matrix

Create a single table that covers every in-scope requirement:

- requirement ID + short description,
- control to implement (code/infra/process),
- verification method (tests/gates/monitors/artifact checks),
- evidence location (reproducible output),
- owner.

If the user has a local standards knowledge base (example: `/home/aron/Downloads/pci/knowledge-base`), use it to keep
requirement language precise without copying large standard text into the repo.

### Step 2 — Convert the matrix into a rubric (freeze a version)

Create a small set of categories (e.g., Security, Privacy, Compliance Readiness). For each category:

- 0–10 score, fixed weights, pass/fail items only.
- every item has exactly one “how to verify” source of truth.
- bump rubric version on any definition change.

### Step 3 — Produce a roadmap (milestones mapped to rubric IDs)

For each milestone:

- list the rubric IDs it closes,
- define acceptance criteria and verification commands,
- define evidence artifacts and where they live.

### Step 4 — Identify P0 regression gates

Pick the highest-risk controls and define CI-enforceable gates (denylist patterns, IaC assertions, contract drift
checks, baseline enforcement). Make failures actionable and non-ambiguous.

## Repo examples (when available)

- `docs/planning/lesser-10of10-rubric.md` shows rubric versioning + deterministic scoring.
- `docs/planning/lesser-10of10-roadmap.md` shows rubric-to-milestone mapping.
- `docs/planning/high-risk-process.md` and `docs/planning/templates/` provide reusable templates.

