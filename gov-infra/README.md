# lesser governance infrastructure (`gov-infra/`)

Repo-local governance materialization for the **`software_repo_gov_infra`**
profile, resolved via `namespace_governance_profile_get` on the equaltoai
namespace route. It makes lesser's quality / consistency / completeness /
security / compliance / maintainability / docs posture explicit, versioned,
deterministic, and fail-closed.

This surface is **CI-core and never retired** for this profile. MCP changes how
governance guidance is *managed*, not whether repo-local `gov-infra/` exists.
Resolves equaltoai/lesser#1403.

## Quick start

From the repository root:

1. Build the CLI once, then run the deterministic rubric verifier:
   - `bash gov-infra/verifiers/gov-verify-rubric.sh`
2. Read the machine report (schema `gov_rubric_report.v1`):
   - `gov-infra/evidence/gov-rubric-report.json`
3. Inspect per-check evidence (regenerated each run; not committed):
   - `gov-infra/evidence/*-output.log`

Verifier scripts are safe to commit **without** execute permission; always run
via `bash …`.

## What each control actually checks — and how it can fail

Factory judges whether this gate has teeth. Every control below runs a real
command; none is simulated, stubbed, or short-circuited. A control is marked
`PASS` only when its command actually ran and exited zero.

| ID | Category | Command | What it checks | How it fails |
|----|----------|---------|----------------|--------------|
| QUA-1 | Quality | `go build -o lesser ./cmd/lesser && ./lesser verify ci` | lesser's existing comprehensive CI gate: golangci-lint, auth UI CSP, audit gates, gosec, govulncheck, supply chain, lambda set, inventory, docs, ai-training, graphql schema, graphql coverage `--strict`, openapi `--strict`, and coverage floors (overall ≥85.0, pkg ≥90.0, cmd ≥90.0) | any lint/security/contract/coverage regression; a compile failure; a removed or renamed verify step |
| CON-1 | Consistency | `check_branch_profile_consistency` (python) | branch/profile consistency across the five profile-named surfaces — soul (`.codex/steward.md` records `software_repo_gov_infra`, `feature → staging → main`, `verify ci`), skills (`apply-and-verify-governance`, `run-rubric-gate`, `github-via-theorymcp` present), `AGENTS.md` present, and `pack.json` declaring the profile + five surfaces | the soul drifts off the branch contract; a governance skill is removed; `pack.json` profile/surfaces drift |
| COM-1 | Completeness | `check_governance_artifacts` (bash) | required governance artifacts present: `pack.json`, the verifier, the report schema, this README, and a non-empty `planning/` | a required governance file is deleted or emptied |
| SEC-1 | Security | `check_secrets_hygiene` (python + git) | no tracked secret-like files (`.env`, `.pem`, `.p12`, `.pfx`, `id_rsa`) and no committed credential material (`aws_secret_access_key=…`, `AKIA…` outside test fixtures) | a credential file or live secret is committed to the tree |
| SEC-2 | Security | `check_ci_gate_integrity` (python) | no `continue-on-error`, `|| true`, or `set +e` anywhere in `.github/workflows/*` | a workflow is edited to swallow a gate failure and stay green |
| CMP-1 | Compliance | `check_profile_resolution` (python) | `pack.json` declares `software_repo_gov_infra` and the exact verifier command / report path / report schema | `pack.json` is edited to drift off the profile |
| CMP-2 | Compliance | `check_head_ref_attestation` (git) | records the exact `git.head`, ref, tree, merge-base, and dirty-path count the verifier ran against | fails only if the working tree is not a git tree; otherwise it *attests* (the evidence commits as a descendant, CI re-runs at the PR head) |
| MAI-1 | Maintainability | `check_ci_hook` (bash) | `.github/workflows/ci.yml` invokes `bash gov-infra/verifiers/gov-verify-rubric.sh` | the CI hook is removed so the rubric no longer runs in CI |
| MAI-2 | Maintainability | `check_verifier_integrity` (python) | the verifier is non-stub and fail-closed: it actually contains `./lesser verify ci`, `set -uo pipefail`, and a control runner | someone stubs the verifier to `exit 0` without running the real gate |
| DOC-1 | Docs | `run_file_check` (bash) | this `gov-infra/README.md` is present | the docs artifact is removed |

The terminal control is **QUA-1**: the rubric *terminates in* `./lesser verify ci`,
lesser's already-comprehensive gate. The remaining controls add the governance
surface the profile names (branch/profile consistency, secrets hygiene, CI gate
integrity, profile resolution, head/ref attestation, CI-hook wiring, and
verifier integrity) — none of which `./lesser verify ci` already covers.

## Why fresh-authored (not parent-staged)

lesser never had a `gov-infra` genome, so there is no parent-staged genome to
checksum-verify and no `gov-infra/genome-provenance.json`. The profile was
resolved directly from the namespace route (`namespace_governance_profile_get`),
and the verifier was authored for a **Go** repository rather than copied from a
sibling (contentus's verifiers are JS-specific; lesser-soul's are CDK/static-site
specific). The one shared artifact is the `gov_rubric_report.v1` report schema,
committed at `gov-infra/schemas/gov-rubric-report.schema.json` — the shared report
contract, not a per-domain verifier.

## Authority (denied)

This governance surface grants **no** deploy, merge, branch-delete, signing,
cloud-mutation, or repository-mutation authority. Signing is retired. MCP
guidance does not replace repo CI. The verifier never deploys, never mutates
cloud/on-chain state, and never changes federation/API/schema contracts.
