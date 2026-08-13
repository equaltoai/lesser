#!/usr/bin/env bash
# lesser GovTheory rubric verifier (repo-local entrypoint)
#
# Profile: software_repo_gov_infra  (resolved via namespace_governance_profile_get,
#          equaltoai namespace route; profile_version 3)
# Command: bash gov-infra/verifiers/gov-verify-rubric.sh
# Report:  gov-infra/evidence/gov-rubric-report.json
# Schema:  gov_rubric_report.v1  (gov-infra/schemas/gov-rubric-report.schema.json)
#
# This verifier is repo-local, deterministic, and fail-closed. It runs lesser's
# real, comprehensive CI gate (`./lesser verify ci` — lint, auth UI CSP, audit
# gates, gosec + govulncheck, supply chain, lambda set, inventory, docs,
# ai-training, graphql schema, graphql coverage, openapi, and overall/pkg/cmd
# coverage floors) plus the governance controls the software_repo_gov_infra
# profile names: branch/profile consistency, secrets hygiene, CI gate integrity,
# profile resolution, head/ref attestation, and CI-hook wiring.
#
# It NEVER deploys, mutates cloud state, publishes, signs, changes namespace
# semantics, or replaces branch protection. A missing check is BLOCKED, never
# simulated; a control is never marked PASS unless its command actually ran and
# exited zero.
#
# Exit codes: 0 = PASS ; 1 = FAIL or BLOCKED ; 2 = verifier/script error.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
GOV_INFRA="${REPO_ROOT}/gov-infra"
EVIDENCE_DIR="${GOV_INFRA}/evidence"
REPORT_PATH="${EVIDENCE_DIR}/gov-rubric-report.json"
RESULTS_FILE="${EVIDENCE_DIR}/.gov-rubric-results.jsonl"
PACK_FILE="${GOV_INFRA}/pack.json"
SOUL_FILE="${REPO_ROOT}/.codex/steward.md"
CI_WORKFLOW="${REPO_ROOT}/.github/workflows/ci.yml"
VERIFIER_FILE="${GOV_INFRA}/verifiers/gov-verify-rubric.sh"

cd "${REPO_ROOT}"
mkdir -p "${EVIDENCE_DIR}"
rm -f "${REPORT_PATH}" "${RESULTS_FILE}" "${EVIDENCE_DIR}"/*-output.log

PASS_COUNT=0
FAIL_COUNT=0
BLOCKED_COUNT=0

append_result() {
  local id="$1" category="$2" status="$3" message="$4" evidence="$5"
  case "${status}" in
    PASS) PASS_COUNT=$((PASS_COUNT + 1)) ;;
    FAIL) FAIL_COUNT=$((FAIL_COUNT + 1)) ;;
    BLOCKED) BLOCKED_COUNT=$((BLOCKED_COUNT + 1)) ;;
    *) echo "Internal error: invalid status ${status}" >&2; exit 2 ;;
  esac
  python3 - "${RESULTS_FILE}" "${id}" "${category}" "${status}" "${message}" "${evidence}" <<'PY'
import json, sys
path, cid, cat, status, msg, ev = sys.argv[1:]
obj = {"id": cid, "category": cat, "status": status}
if msg:
    obj["message"] = msg
if ev:
    obj["evidencePath"] = ev
with open(path, "a", encoding="utf-8") as f:
    f.write(json.dumps(obj) + "\n")
PY
}

run_check() {
  local id="$1" category="$2" cmd="$3"
  local log="${EVIDENCE_DIR}/${id}-output.log"
  echo "=== ${id} ${category} ==="
  printf '$ %s\n\n' "${cmd}" >"${log}"
  local ec=0
  ( set -uo pipefail; eval "${cmd}" ) >>"${log}" 2>&1 || ec=$?
  if [[ ${ec} -eq 0 ]]; then
    append_result "${id}" "${category}" "PASS" "Command succeeded" "gov-infra/evidence/${id}-output.log"
    echo "${id}: PASS"
  else
    append_result "${id}" "${category}" "FAIL" "Command failed with exit ${ec}" "gov-infra/evidence/${id}-output.log"
    echo "${id}: FAIL (exit ${ec})"
  fi
}

run_file_check() {
  local id="$1" category="$2" path="$3"
  local log="${EVIDENCE_DIR}/${id}-output.log"
  echo "=== ${id} ${category} (required file ${path}) ==="
  if [[ -f "${REPO_ROOT}/${path}" ]]; then
    printf 'required file present: %s\n' "${path}" >"${log}"
    append_result "${id}" "${category}" "PASS" "Required file present" "gov-infra/evidence/${id}-output.log"
    echo "${id}: PASS"
  else
    printf 'required file missing: %s\n' "${path}" >"${log}"
    append_result "${id}" "${category}" "BLOCKED" "Required file missing" "gov-infra/evidence/${id}-output.log"
    echo "${id}: BLOCKED"
  fi
}

# --- CON-1: branch/profile consistency across soul/steward/AGENTS/skills/pack ---
check_branch_profile_consistency() {
  python3 - <<'PY'
import json, pathlib, sys

root = pathlib.Path(".")
errs = []

# Surface: soul (the steward's soul materialization carries the branch contract).
soul = root / ".codex/steward.md"
if not soul.is_file():
    errs.append("missing soul surface: .codex/steward.md")
else:
    text = soul.read_text(encoding="utf-8")
    for needle in ("software_repo_gov_infra", "feature → staging → main", "verify ci"):
        if needle not in text:
            errs.append(f".codex/steward.md missing fact: {needle!r}")

# Surface: skills (governance skills encode the same discipline).
skills = [
    ".kimi-code/skills/apply-and-verify-governance/SKILL.md",
    ".kimi-code/skills/run-rubric-gate/SKILL.md",
    ".kimi-code/skills/github-via-theorymcp/SKILL.md",
]
for s in skills:
    if not (root / s).is_file():
        errs.append(f"missing governance skill surface: {s}")

# Surface: AGENTS.md (repo guidelines surface must be present and non-empty).
agents = root / "AGENTS.md"
if not agents.is_file() or agents.stat().st_size == 0:
    errs.append("AGENTS.md surface missing or empty")

# Surface: pack.json declares the profile and the five consistency surfaces.
pack_path = root / "gov-infra/pack.json"
if not pack_path.is_file():
    errs.append("missing gov-infra/pack.json")
else:
    pack = json.loads(pack_path.read_text(encoding="utf-8"))
    profile = pack.get("profile") or {}
    if profile.get("id") != "software_repo_gov_infra":
        errs.append(f"pack.profile.id drift: {profile.get('id')!r}")
    if profile.get("branch_profile_consistency_surfaces") != ["soul", "steward", "AGENTS.md", "skills", "CI"]:
        errs.append(f"pack branch_profile_consistency_surfaces drift: {profile.get('branch_profile_consistency_surfaces')!r}")

print("branch/profile consistency surfaces:")
print("  soul      : .codex/steward.md (software_repo_gov_infra, feature → staging → main, verify ci)")
print("  skills    : apply-and-verify-governance, run-rubric-gate, github-via-theorymcp")
print("  AGENTS.md : repo guidelines surface (present)")
print("  CI        : .github/workflows/ci.yml (invokes gov-verify-rubric.sh — checked by MAI-1)")
print("  pack.json : profile id + branch_profile_consistency_surfaces")

if errs:
    print("CONSISTENCY FAILURES:")
    for e in errs:
        print("  - " + e)
    sys.exit(1)
print("PASS: soul/steward/AGENTS/skills/pack consistent with software_repo_gov_infra branch contract")
PY
}

# --- COM-1: required governance artifacts present ---
check_governance_artifacts() {
  local fail=0
  local required=(
    "gov-infra/pack.json"
    "gov-infra/verifiers/gov-verify-rubric.sh"
    "gov-infra/schemas/gov-rubric-report.schema.json"
    "gov-infra/README.md"
  )
  for f in "${required[@]}"; do
    if [[ -f "${f}" ]]; then
      echo "present: ${f}"
    else
      echo "MISSING: ${f}"
      fail=1
    fi
  done
  if [[ -d gov-infra/planning ]] && [[ -n "$(ls -A gov-infra/planning 2>/dev/null)" ]]; then
    echo "present: gov-infra/planning/ (non-empty)"
  else
    echo "MISSING: gov-infra/planning/ (non-empty)"
    fail=1
  fi
  return "${fail}"
}

# --- SEC-1: secrets hygiene (no tracked credential material) ---
check_secrets_hygiene() {
  python3 - <<'PY'
import subprocess, sys, re

def run(args):
    return subprocess.run(args, capture_output=True, text=True)

files = run(["git", "ls-files"]).stdout.splitlines()
name_re = re.compile(r"(^|/)(\.env$|.*\.pem$|.*\.p12$|.*\.pfx$|id_rsa$)")
bad_files = [f for f in files if name_re.search(f)]

errs = []
if bad_files:
    errs.append("tracked secret-like files:\n" + "\n".join("  - " + f for f in bad_files))

# A real, long AWS secret-key assignment (40+ base64 chars) in tracked files.
r = run(["git", "grep", "-nE", r"aws_secret_access_key\s*=\s*[A-Za-z0-9/+]{40}", "--", "."])
if r.returncode == 0:
    errs.append("AWS secret_access_key assignment in tracked files:\n" + r.stdout)
elif r.returncode not in (1,):
    errs.append("git grep error (secret assignment): " + r.stderr)

# AKIA access-key-id literals outside test fixtures.
r2 = run(["git", "grep", "-nE", r"AKIA[0-9A-Z]{16}", "--", ".", ":(exclude)*_test.go", ":(exclude)testdata/**"])
if r2.returncode == 0:
    errs.append("AKIA access-key-id literal in non-test tracked files:\n" + r2.stdout)
elif r2.returncode not in (1,):
    errs.append("git grep error (AKIA): " + r2.stderr)

print(f"scanned {len(files)} tracked file(s) for secret-like names and credential material")
if errs:
    print("SECRETS HYGIENE FAILURES:")
    for e in errs:
        print(e)
    sys.exit(1)
print("secrets hygiene PASS: no tracked secret files or credential material")
PY
}

# --- SEC-2: CI gate integrity (workflows must not weaken the gates) ---
check_ci_gate_integrity() {
  python3 - <<'PY'
import pathlib, sys

wf_dir = pathlib.Path(".github/workflows")
if not wf_dir.is_dir():
    print("FAIL: no .github/workflows directory")
    sys.exit(1)

errs = []
files = sorted(wf_dir.glob("*.yml")) + sorted(wf_dir.glob("*.yaml"))
if not files:
    errs.append("no workflow files found")

for p in files:
    text = p.read_text(encoding="utf-8")
    if "continue-on-error" in text:
        errs.append(f"{p}: contains 'continue-on-error' (would swallow a gate failure)")
    for line_no, line in enumerate(text.splitlines(), 1):
        if "|| true" in line:
            errs.append(f"{p}:{line_no}: contains '|| true'")
        if "set +e" in line:
            errs.append(f"{p}:{line_no}: contains 'set +e'")

print(f"CI gate integrity over {len(files)} workflow file(s):")
for p in files:
    print(f"  - {p}")
if errs:
    print("GATE INTEGRITY FAILURES:")
    for e in errs:
        print("  - " + e)
    sys.exit(1)
print("PASS: no continue-on-error / || true / set +e in workflows")
PY
}

# --- CMP-1: profile resolution from repo-local pack manifest ---
check_profile_resolution() {
  python3 - <<'PY'
import json, pathlib, sys

pack_path = pathlib.Path("gov-infra/pack.json")
if not pack_path.is_file():
    print("FAIL: missing gov-infra/pack.json")
    sys.exit(1)
pack = json.loads(pack_path.read_text(encoding="utf-8"))
prof = (pack.get("profile") or {}).get("id")
if prof != "software_repo_gov_infra":
    print(f"FAIL: pack.profile.id={prof!r} (expected software_repo_gov_infra)")
    sys.exit(1)
ver = pack.get("verifier") or {}
checks = {
    "command": "bash gov-infra/verifiers/gov-verify-rubric.sh",
    "report_path": "gov-infra/evidence/gov-rubric-report.json",
    "report_schema": "gov_rubric_report.v1",
}
for k, expect in checks.items():
    if ver.get(k) != expect:
        print(f"FAIL: verifier.{k} drift: {ver.get(k)!r} (expected {expect!r})")
        sys.exit(1)
print("profile resolution PASS: software_repo_gov_infra")
print(f"  verifier: {ver['command']}")
print(f"  report:   {ver['report_path']} (schema {ver['report_schema']})")
PY
}

# --- CMP-2: exact head/ref attestation for the commit under decision ---
check_head_ref_attestation() {
  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "FAIL: not inside a git work tree; cannot attest head/ref"
    return 1
  fi
  local head ref tree dirty base
  head="$(git rev-parse HEAD)"
  ref="$(git rev-parse --abbrev-ref HEAD)"
  tree="$(git rev-parse 'HEAD^{tree}')"
  dirty="$(git status --porcelain --untracked-files=all | wc -l | tr -d ' ')"
  base="unavailable"
  if git rev-parse --verify origin/staging >/dev/null 2>&1; then
    base="$(git merge-base HEAD origin/staging)"
  fi
  echo "head/ref attestation (commit the verifier ran against):"
  echo "  git.head=${head}"
  echo "  git.ref=${ref}"
  echo "  git.head_tree=${tree}"
  echo "  merge_base_origin_staging=${base}"
  echo "  working_tree_changed_paths_after_run=${dirty}"
  echo ""
  echo "  Attestation semantics: git.head is the commit checked out when this report"
  echo "  was generated. The PR head that commits the evidence is an ancestor-or-equal"
  echo "  descendant of git.head (evidence-only delta); CI re-runs this verifier at"
  echo "  the PR head to produce a fresh report at the exact head SHA."
  return 0
}

# --- MAI-1: CI hook invokes this verifier ---
check_ci_hook() {
  if [[ ! -f "${CI_WORKFLOW}" ]]; then
    echo "FAIL: missing ${CI_WORKFLOW}"
    return 1
  fi
  if grep -q 'bash gov-infra/verifiers/gov-verify-rubric.sh' "${CI_WORKFLOW}"; then
    echo "CI hook PASS: .github/workflows/ci.yml invokes 'bash gov-infra/verifiers/gov-verify-rubric.sh'"
    return 0
  fi
  echo "FAIL: .github/workflows/ci.yml does not invoke the governance verifier"
  return 1
}

# --- MAI-2: verifier is non-stub and fail-closed ---
check_verifier_integrity() {
  python3 - <<'PY'
import pathlib, sys

v = pathlib.Path("gov-infra/verifiers/gov-verify-rubric.sh")
if not v.is_file():
    print("FAIL: missing gov-infra/verifiers/gov-verify-rubric.sh")
    sys.exit(1)
src = v.read_text(encoding="utf-8")
errs = []
# The verifier must actually invoke lesser's real, comprehensive CI gate. A
# stub that exits 0 without running ./lesser verify ci fails this control.
if "./lesser verify ci" not in src:
    errs.append("verifier does not invoke './lesser verify ci' (stubbed?)")
if "set -uo pipefail" not in src:
    errs.append("verifier is not fail-closed (missing 'set -uo pipefail')")
if "run_check " not in src and "run_file_check " not in src:
    errs.append("verifier has no control runner (missing run_check / run_file_check)")

print("verifier integrity:")
print(f"  file: gov-infra/verifiers/gov-verify-rubric.sh ({len(src.splitlines())} lines)")
print("  invokes './lesser verify ci': " + ("yes" if "./lesser verify ci" in src else "no"))
print("  fail-closed ('set -uo pipefail'): " + ("yes" if "set -uo pipefail" in src else "no"))
if errs:
    print("VERIFIER INTEGRITY FAILURES:")
    for e in errs:
        print("  - " + e)
    sys.exit(1)
print("PASS: verifier is non-stub and fail-closed")
PY
}

echo "=== lesser GovTheory Rubric Verifier ==="
echo "Profile: software_repo_gov_infra"
echo ""

# === Quality (QUA) ===
# The terminal gate: build the CLI from the source under decision, then run the
# full CI gate. Any lint / CSP / audit / gosec / vuln / supply-chain / contract /
# coverage failure fails this control.
run_check "QUA-1" "Quality" "go build -o lesser ./cmd/lesser && ./lesser verify ci"

# === Consistency (CON) ===
run_check "CON-1" "Consistency" "check_branch_profile_consistency"

# === Completeness (COM) ===
run_check "COM-1" "Completeness" "check_governance_artifacts"

# === Security (SEC) ===
run_check "SEC-1" "Security" "check_secrets_hygiene"
run_check "SEC-2" "Security" "check_ci_gate_integrity"

# === Compliance (CMP) ===
run_check "CMP-1" "Compliance" "check_profile_resolution"
run_check "CMP-2" "Compliance" "check_head_ref_attestation"

# === Maintainability (MAI) ===
run_check "MAI-1" "Maintainability" "check_ci_hook"
run_check "MAI-2" "Maintainability" "check_verifier_integrity"

# === Docs (DOC) ===
run_file_check "DOC-1" "Docs" "gov-infra/README.md"

# === Report finalization ===
if [[ ${FAIL_COUNT} -gt 0 ]]; then
  OVERALL_STATUS="FAIL"
elif [[ ${BLOCKED_COUNT} -gt 0 ]]; then
  OVERALL_STATUS="BLOCKED"
else
  OVERALL_STATUS="PASS"
fi

PACK_VERSION="$(python3 -c 'import json;print(json.load(open("gov-infra/pack.json"))["pack"]["version"])' 2>/dev/null || printf 'unknown')"
PACK_DIGEST="$(sha256sum gov-infra/pack.json 2>/dev/null | awk '{print $1}')"
[[ -z "${PACK_DIGEST}" ]] && PACK_DIGEST="unavailable"

python3 - "${RESULTS_FILE}" "${REPORT_PATH}" "${OVERALL_STATUS}" "${PASS_COUNT}" "${FAIL_COUNT}" "${BLOCKED_COUNT}" "${PACK_VERSION}" "${PACK_DIGEST}" <<'PY'
import json, sys, re
from datetime import datetime, timezone
(results_file, report_path, status, passed, failed, blocked, pack_version, pack_digest) = sys.argv[1:]
results = []
try:
    with open(results_file, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                results.append(json.loads(line))
except FileNotFoundError:
    pass
report = {
    "$schema": "https://gov.pai.dev/schemas/gov-rubric-report.schema.json",
    "schemaVersion": 1,
    "timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "pack": {"version": pack_version, "digest": "sha256:" + pack_digest},
    "project": {"name": "lesser", "slug": "lesser"},
    "summary": {
        "status": status,
        "pass": int(passed),
        "fail": int(failed),
        "blocked": int(blocked),
    },
    "results": results,
}
# Fail-closed structural self-validation against gov_rubric_report.v1.
id_re = re.compile(r"^(QUA|CON|COM|SEC|CMP|MAI|DOC)-[0-9]+$")
cats = {"Quality", "Consistency", "Completeness", "Security", "Compliance", "Maintainability", "Docs"}
sts = {"PASS", "FAIL", "BLOCKED"}
allowed_top = {"$schema", "schemaVersion", "timestamp", "pack", "project", "summary", "results"}
allowed_item = {"id", "category", "status", "message", "evidencePath"}
errs = []
extra_top = set(report) - allowed_top
if extra_top:
    errs.append(f"unexpected top-level keys: {sorted(extra_top)}")
if not re.match(r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$", report["timestamp"]):
    errs.append("timestamp not ISO-8601 Z")
for r in results:
    extra = set(r) - allowed_item
    if extra:
        errs.append(f"{r.get('id')}: extra keys {sorted(extra)}")
    if not id_re.match(r.get("id", "")):
        errs.append(f"bad id: {r.get('id')!r}")
    if r.get("category") not in cats:
        errs.append(f"{r.get('id')}: bad category {r.get('category')!r}")
    if r.get("status") not in sts:
        errs.append(f"{r.get('id')}: bad status {r.get('status')!r}")
if errs:
    sys.stderr.write("REPORT SELF-VALIDATION FAILED (gov_rubric_report.v1):\n" + "\n".join("  - " + e for e in errs) + "\n")
    sys.exit(2)
with open(report_path, "w", encoding="utf-8") as f:
    json.dump(report, f, indent=2)
    f.write("\n")
PY
finalize_ec=$?
if [[ ${finalize_ec} -ne 0 ]]; then
  echo "ERROR: report finalization/self-validation failed (exit ${finalize_ec})" >&2
  exit 2
fi

rm -f "${RESULTS_FILE}"

echo ""
echo "=== Summary ==="
echo "Report: gov-infra/evidence/gov-rubric-report.json"
echo "Status: ${OVERALL_STATUS} (${PASS_COUNT} pass / ${FAIL_COUNT} fail / ${BLOCKED_COUNT} blocked)"

if [[ "${OVERALL_STATUS}" == "PASS" ]]; then
  exit 0
fi
exit 1
