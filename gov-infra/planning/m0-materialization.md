# M0 — lesser gov-infra materialization (planning record)

Resolves equaltoai/lesser#1403.

## Finding

lesser had no `gov-infra/` directory and never had (`git log --all -- gov-infra`
was empty; not gitignored). The equaltoai namespace governance profile resolves
lesser to `software_repo_gov_infra`, whose `gov_infra` block is
`required: true`, `role: ci_core`, `retirement_policy: never_retired`,
`ci_hook_required: true`, verifier `gov-infra/verifiers/gov-verify-rubric.sh`,
report `gov-infra/evidence/gov-rubric-report.json`, schema `gov_rubric_report.v1`.
lesser was the only fleet repo missing the materialization.

## Decision

Author lesser's own, Go-appropriate genome rather than copying a sibling's.
The verifier terminates in `./lesser verify ci` — lesser's existing comprehensive
gate (lint, auth UI CSP, audit gates, gosec + govulncheck, supply chain, lambda
set, inventory, docs, ai-training, graphql schema, graphql coverage, openapi, and
overall/pkg/cmd coverage floors) — plus the governance controls the profile names.

There is no parent-staged genome to checksum: lesser's gov-infra is fresh-authored,
not staged. The profile was resolved directly via `namespace_governance_profile_get`
on the equaltoai route, exactly as the pack records. Accordingly there is no
`gov-infra/genome-provenance.json` and no child-side genome-checksum control; the
committed `gov_rubric_report.v1` schema is the shared report contract.

## Control inventory

- QUA-1 Quality — `go build -o lesser ./cmd/lesser && ./lesser verify ci`
- CON-1 Consistency — branch/profile consistency across soul / steward / AGENTS.md / skills / CI
- COM-1 Completeness — required governance artifacts present
- SEC-1 Security — secrets hygiene (no tracked credentials)
- SEC-2 Security — CI gate integrity (no `continue-on-error`, `|| true`, or `set +e` in workflows)
- CMP-1 Compliance — profile resolution from `gov-infra/pack.json`
- CMP-2 Compliance — exact head/ref attestation for the commit under decision
- MAI-1 Maintainability — CI hook invokes the verifier
- MAI-2 Maintainability — verifier is non-stub and fail-closed (actually runs `./lesser verify ci`)
- DOC-1 Docs — `gov-infra/README.md` present

## Allowed write scope (this milestone)

`gov-infra/**` and `.github/workflows/**` only. No product source, no `cmd/`,
no `pkg/`, no test changes outside gov-infra.
