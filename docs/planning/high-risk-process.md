# High-Risk Domain Planning Standard (Rubric + Roadmap + Evidence)

This is a reusable process for turning “high-risk domain” requirements (finance, healthcare, etc.) into **versioned,
measurable, repeatable** engineering work. It generalizes the approach used by:

- `docs/planning/lesser-10of10-rubric.md`
- `docs/planning/lesser-10of10-roadmap.md`
- `docs/planning/lesser-10of10-plan.md` (historical)

This is not legal advice and does not replace compliance or security professionals. Treat it as an engineering
execution pattern: scope → controls → gates → evidence.

## Inputs (what must be decided up front)

- **Domain + frameworks**: e.g., PCI DSS (payments), HIPAA (healthcare), SOC 2 (trust services), etc.
- **Data classification**: CHD/SAD, PAN tokens, PHI, PII, secrets, telemetry.
- **Scope boundaries**: which services, accounts, environments, and third parties are in-scope.
- **Assurance target**: “best effort hardening” vs “audit-ready evidence” vs “certification/attestation”.

## Outputs (the standardized artifacts)

- **Controls matrix**: framework requirement → system control → verification → evidence.
- **Rubric**: deterministic pass/fail scoring, versioned (prevents goalpost drift).
- **Roadmap**: milestones mapped directly to rubric IDs (keeps execution honest).
- **Evidence plan**: where audit artifacts live and how they’re generated/re-generated.
- **Gates**: CI/local verifiers that block regressions for the highest-risk controls.

Templates live in:

- `docs/planning/templates/high-risk-controls-matrix.template.md`
- `docs/planning/templates/high-risk-rubric.template.md`
- `docs/planning/templates/high-risk-roadmap.template.md`

## Workflow (repeatable)

### Step 0 — Scope and invariants

Write a 1-page scope statement:

- What data exists, where it flows, and what is explicitly out of scope.
- What environments are in scope (dev/staging/prod) and what “prod-like” means.
- Which third parties are in scope and what evidence you can realistically obtain from them.

### Step 1 — Build the controls matrix (requirements → controls)

Start from the framework(s) and list every applicable requirement. For each requirement, record:

- the concrete **control** you will implement (code/infra/process),
- how you will **verify** it (tests, CI gates, monitors, manual checks),
- what **evidence** is produced (logs, reports, configs, diagrams, tickets).

If you maintain local standards knowledge-bases, keep the raw standard text **out of the repo** when licensing or
distribution is uncertain, and link to the local path instead (example: `/home/aron/Downloads/pci/knowledge-base`).

### Step 2 — Freeze a rubric version (no moving goalposts)

Convert the controls matrix into a small rubric:

- 0–10 per category.
- fixed point weights; pass/fail items only.
- each item has a single “how to verify” source of truth (command or deterministic artifact check).

### Step 3 — Map rubric items to milestones (roadmap)

Create milestones that each close specific rubric IDs (no “floating” work). For each milestone:

- acceptance criteria are measurable,
- verification commands are listed,
- evidence locations are defined.

### Step 4 — Add gates for P0 controls

For highest-risk controls, add CI-enforceable gates (examples: regression baselines, denylist patterns, IaC assertions,
contract drift checks). The goal is to stop backsliding while improvements are in progress.

### Step 5 — Iterate with evidence

After each milestone:

- run the verifiers,
- store/refresh evidence,
- update the rubric scorecard (with the rubric version noted).

