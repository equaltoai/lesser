# Lesser: 10/10 Rubric (Quality, Consistency, Completeness, Security)

This rubric defines what “10/10” means for Lesser and how category grades are computed. The goal is to prevent
goalpost drift between audit passes by making scoring **versioned, measurable, and repeatable**.

## Versioning (no moving goalposts)

- **Rubric version:** `v1.0` (2026-01-04)
- **Comparability rule:** grades are only comparable within the same rubric version.
- **Change rule:** any rubric change must bump the version and include a brief changelog entry (what changed + why).

## Scoring (deterministic)

- Each category is scored **0–10**.
- Each category has a small set of requirements with fixed point weights that sum to **10**.
- Requirements are **pass/fail** (either earn the full points or earn 0).
- A category is **10/10 only if all requirements in that category pass**.

## Verification (commands are the source of truth)

All checks below are driven by repo commands. “Strict” checks are used where the repo supports drift-checked generators.

Notes:

- `./lesser verify ci` is the recommended “one-shot” smoke for contributor safety, but it is not the *only* input to
  this rubric (coverage + strict OpenAPI are intentionally separate).
- Coverage commands generate `coverage*.out` and `coverage*.html` files (untracked artifacts).

---

## Quality (QUA) — maintainable, testable, change-friendly

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| QUA-1 | 5 | Unit tests stay green | `./lesser test unit` |
| QUA-2 | 3 | `pkg/` coverage stays at or above **85.0%** (generated code excluded) | `./lesser test coverage --scope pkg` then `go tool cover -func=coverage_pkg.out \| tail -n 1` |
| QUA-3 | 2 | `pkg/` coverage stays at or above **90.0%** (generated code excluded) | `./lesser test coverage --scope pkg` then `go tool cover -func=coverage_pkg.out \| tail -n 1` |

**10/10 definition:** QUA-1 through QUA-3 pass.

---

## Consistency (CON) — one way to do the important things

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| CON-1 | 6 | Lint stays green (formatting, static analysis, complexity budgets) | `./lesser lint` |
| CON-2 | 4 | Audit gates stay green (tracked baselines + toolchain alignment) | `./lesser verify audit` |

**10/10 definition:** CON-1 and CON-2 pass.

---

## Completeness (COM) — no drift, no mystery meat

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| COM-1 | 2 | Docs verification stays green | `./lesser verify docs` |
| COM-2 | 2 | GraphQL schema verification stays green | `./lesser verify schema` |
| COM-3 | 2 | GraphQL route coverage stays green (strict) | `./lesser verify graphql-coverage --strict` |
| COM-4 | 2 | OpenAPI verification stays green (strict, drift-checked) | `./lesser verify openapi --strict` |
| COM-5 | 1 | Inventory verification stays green | `./lesser verify inventory` |
| COM-6 | 1 | Lambda-set parity verification stays green | `./lesser verify lambda-set` |

**10/10 definition:** COM-1 through COM-6 pass.

---

## Security (SEC) — abuse-resilient and reviewable by default

| ID | Points | Requirement | How to verify |
| --- | ---: | --- | --- |
| SEC-1 | 3 | Static security scan stays green (gosec) | `./lesser sec-scan` |
| SEC-2 | 3 | Dependency vulnerability scan stays green (govulncheck) | `./lesser vuln-check` |
| SEC-3 | 2 | Supply-chain verification stays green | `./lesser verify supply-chain` |
| SEC-4 | 2 | Audit gates stay green (security regression baselines) | `./lesser verify audit` |

**10/10 definition:** SEC-1 through SEC-4 pass.

---

## Maintaining 10/10 (recommended CI surface)

To keep grades stable over time, CI should run (at minimum):

```bash
./lesser verify ci
./lesser verify openapi --strict
./lesser test coverage --scope pkg
```

If any of the above fail, at least one category cannot be 10/10 under this rubric.
