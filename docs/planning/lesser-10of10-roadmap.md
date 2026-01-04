# Lesser: 10/10 Roadmap (Rubric v1.0)

This roadmap is the execution plan for achieving and maintaining **10/10** across **Quality**, **Consistency**,
**Completeness**, and **Security**, as defined by:

- `docs/planning/lesser-10of10-rubric.md` (source of truth; versioned)

## Current scorecard (Rubric v1.0, 2026-01-04)

| Category | Grade | Blocking rubric items |
| --- | ---: | --- |
| Quality | 8/10 | QUA-3 (pkg coverage ≥ 90.0%) |
| Consistency | 10/10 | — |
| Completeness | 10/10 | — |
| Security | 10/10 | — |

Evidence (most recent):

- `./lesser verify ci` is green.
- `./lesser coverage scoreboard --profile coverage_pkg.out` reports `total: 89.8% (83219/92720 statements)`.
- `./lesser verify openapi --strict` is green (`ok: docs/contracts/openapi.yaml (215 paths)`).

## Rubric-to-milestone mapping

| Rubric ID | Status | Milestone |
| --- | --- | --- |
| QUA-1 | ✅ passing | Maintain via CI |
| QUA-2 | ✅ passing | Maintain via CI |
| QUA-3 | ❌ failing | M2 |
| CON-1 | ✅ passing | Maintain via CI |
| CON-2 | ✅ passing | Maintain via CI |
| COM-1 | ✅ passing | Maintain via CI |
| COM-2 | ✅ passing | Maintain via CI |
| COM-3 | ✅ passing | Maintain via CI |
| COM-4 | ✅ passing | ✅ completed (M1) |
| COM-5 | ✅ passing | Maintain via CI |
| COM-6 | ✅ passing | Maintain via CI |
| SEC-1 | ✅ passing | Maintain via CI |
| SEC-2 | ✅ passing | Maintain via CI |
| SEC-3 | ✅ passing | Maintain via CI |
| SEC-4 | ✅ passing | Maintain via CI |

## Milestones (map directly to rubric IDs)

### M0 — Freeze “10/10” (rubric + scoring) (done)

**Closes:** (meta) prevents goalpost drift  
**Output:** `docs/planning/lesser-10of10-rubric.md`

**Acceptance criteria**
- Rubric is versioned and defines deterministic scoring.
- All future audits reference the rubric version they used.

---

### M1 — Make OpenAPI strict verification pass (COM-4) (done)

**Closes:** COM-4  
**Goal:** make OpenAPI a drift-checked contract (not just route presence).

**Acceptance criteria**
- `./lesser verify openapi --strict` is green.
- `docs/contracts/openapi.yaml` is updated via `./lesser generate openapi` and stays in sync.

**Suggested verification**
```bash
./lesser generate openapi
./lesser verify openapi --strict
```

---

### M2 — Raise `pkg/` coverage to ≥ 90.0% (QUA-3)

**Closes:** QUA-3  
**Goal:** move `pkg/` coverage from **89.8% → 90.0%+** (generated excluded).

**Acceptance criteria**
- `./lesser test coverage --scope pkg` generates `coverage_pkg.out`.
- `go tool cover -func=coverage_pkg.out | tail -n 1` reports **≥ 90.0%**.

**Suggested verification**
```bash
./lesser test coverage --scope pkg
go tool cover -func=coverage_pkg.out | tail -n 1
```

**Targeting guidance (keep it efficient)**
- Cover at least ~**229 additional statements** (delta to reach 90.0% at current statement count).
- Start with the highest “uncovered statements” packages from:
  `./lesser coverage scoreboard --profile coverage_pkg.out --mode package --top 20`.

---

### M3 — Make “maintain 10/10” automatic in CI (QUA-3 + COM-4)

**Closes:** (maintenance) keeps QUA-3 + COM-4 from regressing silently  
**Goal:** ensure the rubric’s “10/10” gates run in CI, not just locally.

**Acceptance criteria**
- CI runs (or `./lesser verify ci` runs) these rubric-critical checks:
  - `./lesser verify openapi --strict`
  - `./lesser test coverage --scope pkg` + a hard threshold check for **≥ 90.0%**
- A failing gate produces a clear, actionable error message.

**Suggested verification**
```bash
./lesser verify ci
./lesser verify openapi --strict
./lesser test coverage --scope pkg
```
