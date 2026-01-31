# Rubric Template (High-Risk Domains)

This template is intended to be copied and filled per project.

## Versioning (no moving goalposts)

- **Rubric version:** `v0.1` (YYYY-MM-DD)
- **Comparability rule:** grades compare only within the same rubric version.
- **Change rule:** any change bumps the version and adds a brief changelog note.

### Changelog

- `v0.1` (YYYY-MM-DD): initial rubric.

## Scoring (deterministic)

- Each category is 0–10.
- Point weights sum to 10 per category.
- Requirements are pass/fail and earn full points or zero.
- A category is 10/10 only if all its requirements pass.

## Verification (commands + artifacts are the source of truth)

Each rubric item must name exactly one verification mechanism:

- a command (`./tool verify ...`, `go test ...`, `terraform validate`, etc.), or
- a deterministic artifact check (required doc exists and matches an agreed format).

---

## Security (SEC) — abuse-resilient by default

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 3 | Static security scan stays green | [command] |
| SEC-2 | 3 | Dependency vulnerability scan stays green | [command] |
| SEC-3 | 2 | Supply-chain / provenance verification stays green | [command] |
| SEC-4 | 2 | P0 regression gates stay green (baselines/assertions) | [command] |

**10/10 definition:** SEC-1 through SEC-4 pass.

---

## Privacy (PRV) — minimize and protect sensitive data

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| PRV-1 | 4 | Sensitive fields are scrubbed from logs/telemetry | [command/test] |
| PRV-2 | 3 | Access controls are least-privilege for sensitive data | [IaC assertions / policy checks] |
| PRV-3 | 3 | Encryption at rest/in transit is enforced for sensitive data flows | [config check / integration test] |

**10/10 definition:** PRV-1 through PRV-3 pass.

---

## Compliance Readiness (CMP) — auditability and evidence

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CMP-1 | 4 | Controls matrix is complete for in-scope frameworks | [artifact check] |
| CMP-2 | 3 | Evidence plan exists and is reproducible | [artifact check] |
| CMP-3 | 3 | Risk assessment / threat model exists and is current | [artifact check] |

**10/10 definition:** CMP-1 through CMP-3 pass.

