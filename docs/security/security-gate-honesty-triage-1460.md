# Security-Gate Honesty — Triage Record

Date: 2026-08-25
Repo: `equaltoai/lesser`
Issue: #1460 — closes on this milestone's PR
Origin: GLM-5.3 adversarial attack on PR #1459 (task `20260825T024136Z`), finding **F1 HIGH**: `lesser sec-scan` is vacuous.

## Summary

`lesser sec-scan` batched `go list ./...` output as **import paths** and passed them to gosec,
which resolves import-path arguments to **zero files**. The gate therefore reported success
without scanning anything. This record documents the wrapper fix, the full finding set the
honest gate exposes, and the per-finding triage that justifies every exclusion added to the
gate. **Final honest count: 97 findings triaged — 97 false positives, 0 true positives
remaining.** One additional genuine finding (gosec G115 in `pkg/services/cms/draft_review.go`)
was **fixed in this PR** rather than excluded; it is the only real issue the honest gate
surfaced.

## 1. The wrapper bug and the fix

`cmd/lesser/security_cmd.go` (`runGoPackageSecurityTool`) listed packages with
`go list ./...`, then invoked gosec with the import paths, e.g.
`gosec -exclude-generated github.com/equaltoai/lesser/pkg/services/cms`. gosec resolves
import-path arguments to zero files (it needs directory paths or package patterns).

Before/after proof (gosec **v2.28.0** — the version the gate resolved from `$GOPATH/bin`
when this scan was captured; see the toolchain note below):

| invocation | Files | Issues | exit |
|---|---|---|---|
| `gosec -exclude-generated github.com/equaltoai/lesser/pkg/services/cms` (import path) | 0 | 0 | 0 |
| `gosec -exclude-generated ./pkg/services/cms` (directory) | 14 | 1 | 1 |
| `./lesser sec-scan` at base (pre-fix) | 0 (no batches scan anything) | 0 | 0 |
| `./lesser sec-scan` at head (post-fix, before excludes) | 1565 (root) + 22 (cdk) | 95 + 2 | 1 |

Fix: the gosec batch target list now comes from `go list -f '{{.Dir}}' ./...` normalized to
`./rel` directory paths (the same directory-form target list the lint batching already uses).
govulncheck keeps import-path targets (it resolves them correctly). Batching is unchanged.
`tmp/` and `infra/` package dirs are dropped from the target list because gosec's
`-exclude-dir` flag only filters directories during package-pattern expansion, not explicit
directory arguments — the flags' intent is preserved.

Scan environment and toolchain (must match the gate): `GOTOOLCHAIN=auto` and `$GOPATH/bin`
prepended to `PATH` (the CLI does this for every subprocess via `mergeEnvForDir` in
`cmd/lesser/exec.go`). The 97-finding capture above ran under gosec **v2.28.0** resolved from
`$GOPATH/bin`.

**Toolchain-hazard note (why the environment had to be pinned):** the CI installer
(`scripts/install_ci_tools.sh`) originally pinned gosec **v2.22.11** — a release without the
taint rules — while the local `$GOPATH/bin` binary (the one the gate's `mergeEnvForDir` PATH
prepend resolves first, ahead of any other PATH entry) was v2.28.0. The triage therefore ran
under v2.28.0 (95 root findings, including G703/G702/G117/G710/G704), while CI enforcement
ran v2.22.11 (only 22 root findings, none of the taint rules): CI silently enforced a subset
of the triaged set, and any machine with a newer gosec would fail the gate on untriaged
rules. (Re-verified for this remediation with the gate's exact flags: v2.28.0 → 94–95 root
findings across runs, v2.22.11 → 22, zero taint rules. The taint counts are nondeterministic
run-to-run under identical version and flags — G703 47–48, G702 0–5 for v2.28.0 — with no
effect on the excluded set; the gate's excluded-rule set makes this immaterial.) Remediation:
the
gate now **pins and asserts** gosec v2.28.0 —
`scripts/install_ci_tools.sh` installs the version named by the `pinnedGosecVersion`
constant in `cmd/lesser/security_cmd.go` (one source of truth), and `sec-scan` fails closed
if the resolved binary's `go version -m` module version differs (`assertPinnedGosecVersion`),
so the enforcement environment cannot silently drift from the triaged toolchain again.

## 2. Classification methodology

Every finding was individually reviewed against the flagged source. Three evidence bases
underpin a `false-positive` classification:

1. **In-code intent comment.** The flagged line (or its immediate scope) carries a
   `//nolint:gosec // <reason>` comment explaining why the behavior is intentional.
   golangci-lint honors these; direct gosec does not, which is why the findings surface in
   `sec-scan` while `./lesser lint` remains green on them.
2. **Operator-only provenance.** The flagged code is reachable only from operator-invoked
   CLI/tooling paths (`cmd/lesser`, `cmd/*` entrypoints, `tools/openapi`, release tooling,
   CDK synth) — inputs are operator-controlled flags/env/cwd, never untrusted network input.
3. **Call-site guard.** The flagged value passes through a real sanitizer/guard at the call
   site (e.g. `ValidateRedirectURL`, an explicit length clamp, a config-driven cookie
   policy, arg-array `exec.Command*` with no shell).

Findings that are genuinely secret-bearing serialization (G117) are classified
`false-positive` because the marshaling is the *correct* storage/wire surface for that
material (Secrets Manager, SQS handoff consumed by an IAM-confined processor, Web Push
encrypted payload), not a leak.

## 3. Findings by rule

| rule | count | severity | classification | justification |
|---|---|---|---|---|
| G703 | 48 | HIGH | false-positive | Path-traversal taint on file/dir creation in operator CLI (`cmd/lesser`): paths derive from the discovered repo root (cwd), operator env overrides (`GOCACHE`, `XDG_CACHE_HOME`), CLI flags, or containment-checked release paths; most carry in-code intent comments. No untrusted-input → filesystem flow exists (the CLI has no network request surface). |
| G204 | 9 | MEDIUM | false-positive | Subprocess with variable arg: every site is `exec.Command*` with a **fixed binary** and operator-supplied arguments passed as a separate argv entries (no shell): `secret-tool` keyring calls, OS browser launcher (`xdg-open`/`open`/`rundll32`) with the operator-approved URL, the shared `captureCommandOutput`/`runCommand` exec wrappers, `cdk synth` invocation. |
| G304 | 9 | MEDIUM | false-positive | File inclusion via variable: reads/writes at operator-supplied or containment-checked paths (`--auth-secret-file`, release staging dir, bundle `outDir`, CDK synth asset paths). |
| G117 | 7 | MEDIUM | false-positive | Marshaled secret-pattern field — each site intentionally serializes secret material into its correct store/wire surface: CloudFront keygen tool (`PrivateKey` → Secrets Manager), `owner-bootstrap` wallet/OAuth secrets (→ Secrets Manager), `push-delivery` Web Push `AccessToken` (→ encrypted payload), `notifications/push.go` SQS handoff token (IAM-confined queue), VAPID `PrivateKey` (→ Secrets Manager), CLI auth session `RefreshToken` (local 0600 session file). All carry in-code intent comments. |
| G702 | 5 | HIGH | false-positive | Command-injection taint at the same arg-array `exec.Command*` sites as G204; no shell interpolation exists. |
| G306 | 4 | MEDIUM | false-positive | `os.WriteFile` mode > 0600 in `pkg/releaseassets`: the files are **public** release artifacts (Lambda bundles, checksum lists, manifests, deployment assembly) that must be world-readable for distribution/upload; 0644 is the correct mode. |
| G302 | 4 | MEDIUM | false-positive | Same release-artifact rationale for `os.Chmod`/file mode (0644). |
| G301 | 4 | MEDIUM | false-positive | Directory mode 0755 for release publication directories and a `os.MkdirTemp`-scoped certification workspace (parent is 0700); no secret material. |
| G101 | 3 | HIGH (LOW confidence) | false-positive | Pattern-matched credential **names/descriptions**, not values: env-var names in `pkg/config/validator.go`, a DynamoDB partition-key format string in `passkey_registration_proof.go`, OAuth scope names in `tools/openapi/main.go`. |
| G710 | 1 | MEDIUM | false-positive | Open-redirect taint on `pkg/common/redirect.go:88`: the target passes `ValidateRedirectURL(redirectTo, r.Host)` and falls back to the default path when validation fails — the sanitizer is invisible to the taint analysis. |
| G704 | 1 | HIGH | false-positive | SSRF taint on `pkg/observability/http_tracker.go:87`: `HTTPTracker` only decorates an injected client's `Do` path; destination and dial policy belong to the caller-provided transport (in-code intent comment). |
| G124 | 1 | MEDIUM | false-positive | `http.Cookie` literal in `pkg/common/cookies.go:35`: `Secure`/`HttpOnly`/`SameSite` are driven by `CookieConfig` whose default (`DefaultCookieConfig`) sets `SameSiteStrict` and secure flags; the helper is config-driven by design. |
| G115 | 1 | HIGH | false-positive | `pkg/storage/models/federation_relationship.go:501`: `byte(len(stateTransitions))` is explicitly clamped to the one-byte wire maximum (255) at lines 497–499 with an in-code intent comment. The **other** G115 (this PR's `draft_review.go:71` conversion) was a genuine unguarded conversion and is **fixed** in this PR. |

## 4. Exclusion map (every exclusion traces to entries above)

The sec-scan gate (direct gosec, v2.28.0, pinned and asserted by `sec-scan`) supports only
rule-level exclusion (`-exclude=...`); per-line `#nosec` annotations across ~50 files are
out of scope for this milestone (allowed write scope is `cmd/lesser/security_cmd.go`, the
draft-review G115 fix, and this record). Rule-level excludes are therefore the narrowest
honest mechanism, and each excluded rule maps 1:1 to the uniformly-false-positive class in
§3.

Residual visibility is the honest caveat of rule-level exclusion, and it differs per rule:

- **4 of the 13 excluded rules have zero automated coverage: G101, G702, G710, G124.**
  - **G101** is excluded here AND pre-existing in `.golangci.yml` (the gosec linter's
    `excludes` list), so neither `sec-scan` nor `./lesser lint` reports it. The 3 triaged
    G101 rows above were verified by hand to be credential **names**, not values (env-var
    names in `pkg/config/validator.go`, a DynamoDB partition-key format string, OAuth scope
    names); a future genuine hardcoded-credential finding would be invisible to every
    automated gate until the lint config changes.
  - **G702, G710, G124 postdate the lint toolchain's embedded gosec.** The repo pins
    golangci-lint **v2.11.4** (`scripts/install_ci_tools.sh`), which embeds gosec
    **v2.24.8-dev** — a build predating those three rules. Verified with an isolated probe:
    gosec v2.28.0 fires all three on identical trigger code; the pinned lint fires none.
    The lint surface therefore cannot report them, on any machine, until the golangci-lint
    pin is bumped (see Follow-ups).
- **In CI, all 13 excluded rules have zero lint coverage.** `./lesser verify ci` runs lint
  with `--disable-gosec` (`cmd/lesser/verify_cmd.go`), so gosec never runs in the CI lint
  pass; the "lint still reports" surface below exists only on a developer's manual
  `./lesser lint` with the pinned golangci-lint.
- For the other **9** excluded rules that do exist in the pinned lint's embedded gosec
  (G703, G204, G304, G117, G306, G302, G301, G704, G115), a manual `./lesser lint`
  (`golangci-lint` gosec, `.golangci.yml`, which enforces G115 and the rest of the excluded
  rules) still reports new genuine findings — but only where the flagged line carries no
  `//nolint:gosec` annotation; annotated lines remain invisible to lint while direct gosec
  would flag them.

| gate | exclude | traces to (§3 entries) |
|---|---|---|
| root sec-scan | `G703,G204,G304,G117,G702,G306,G302,G301,G101,G710,G704,G124,G115` | all 95 root-module rows below |
| cdk sec-scan | `G304,G301` | the 2 `infra/cdk` rows below |

## 5. Honest final count

- Root module: **95 findings** → 95 false-positive (all excluded above, each traceable to §3
  and its row in §6).
- CDK module: **2 findings** → 2 false-positive (excluded above).
- Fixed in this PR instead of excluded: **1** — gosec G115 `int64 → uint64` conversion in
  `pkg/services/cms/draft_review.go:71` (production code; the conversion now carries an
  explicit non-negative guard and a byte-identical `MaxUint64` nil-position sentinel, with a
  golden-hash regression test).
- True positives remaining: **0**.

## 6. Full per-finding table (97 rows)

| location | rule | severity | classification |
|---|---|---|---|
| `cmd/cloudfront-keygen/main.go:118` | G117 | MEDIUM | false-positive |
| `cmd/lesser/auth_keyring_linux.go:36` | G702 | HIGH | false-positive |
| `cmd/lesser/auth_keyring_linux.go:36-39` | G204 | MEDIUM | false-positive |
| `cmd/lesser/auth_keyring_linux.go:61` | G702 | HIGH | false-positive |
| `cmd/lesser/auth_keyring_linux.go:61-64` | G204 | MEDIUM | false-positive |
| `cmd/lesser/auth_keyring_linux.go:69` | G702 | HIGH | false-positive |
| `cmd/lesser/auth_keyring_linux.go:69-73` | G204 | MEDIUM | false-positive |
| `cmd/lesser/auth_loopback.go:276` | G204 | MEDIUM | false-positive |
| `cmd/lesser/auth_loopback.go:278` | G204 | MEDIUM | false-positive |
| `cmd/lesser/auth_loopback.go:280` | G204 | MEDIUM | false-positive |
| `cmd/lesser/auth_session_store.go:165` | G117 | MEDIUM | false-positive |
| `cmd/lesser/auth_session_store.go:52` | G304 | MEDIUM | false-positive |
| `cmd/lesser/auth_session_store.go:52` | G703 | HIGH | false-positive |
| `cmd/lesser/bootstrap.go:160` | G703 | HIGH | false-positive |
| `cmd/lesser/bootstrap.go:166` | G703 | HIGH | false-positive |
| `cmd/lesser/bootstrap.go:170` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:123` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:129` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:133` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:156` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:173` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:188` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:195` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:38` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:53` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:72` | G703 | HIGH | false-positive |
| `cmd/lesser/build.go:96` | G702 | HIGH | false-positive |
| `cmd/lesser/build_cmd.go:187` | G703 | HIGH | false-positive |
| `cmd/lesser/build_cmd.go:195` | G703 | HIGH | false-positive |
| `cmd/lesser/cdk.go:291` | G703 | HIGH | false-positive |
| `cmd/lesser/cdk.go:302` | G703 | HIGH | false-positive |
| `cmd/lesser/cdk.go:87` | G703 | HIGH | false-positive |
| `cmd/lesser/dev_cmd.go:267` | G204 | MEDIUM | false-positive |
| `cmd/lesser/exec.go:179` | G703 | HIGH | false-positive |
| `cmd/lesser/exec.go:187` | G703 | HIGH | false-positive |
| `cmd/lesser/exec.go:195` | G703 | HIGH | false-positive |
| `cmd/lesser/exec.go:203` | G703 | HIGH | false-positive |
| `cmd/lesser/exec.go:32` | G204 | MEDIUM | false-positive |
| `cmd/lesser/exec.go:32` | G702 | HIGH | false-positive |
| `cmd/lesser/lambda_asset_metadata.go:33` | G703 | HIGH | false-positive |
| `cmd/lesser/lambda_asset_metadata.go:37` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:263` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:428` | G304 | MEDIUM | false-positive |
| `cmd/lesser/release_deploy_assets.go:428` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:436` | G304 | MEDIUM | false-positive |
| `cmd/lesser/release_deploy_assets.go:436` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:445` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:450` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:455` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:457` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_assets.go:487` | G304 | MEDIUM | false-positive |
| `cmd/lesser/release_deploy_assets.go:487` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_install.go:580` | G703 | HIGH | false-positive |
| `cmd/lesser/release_deploy_install.go:584` | G703 | HIGH | false-positive |
| `cmd/lesser/repo.go:48` | G703 | HIGH | false-positive |
| `cmd/lesser/s3_upload.go:150` | G703 | HIGH | false-positive |
| `cmd/lesser/s3_upload.go:42` | G703 | HIGH | false-positive |
| `cmd/lesser/s3_upload.go:66` | G703 | HIGH | false-positive |
| `cmd/lesser/state.go:119` | G703 | HIGH | false-positive |
| `cmd/lesser/state.go:125` | G703 | HIGH | false-positive |
| `cmd/lesser/state.go:129` | G703 | HIGH | false-positive |
| `cmd/lesser/test_cmd.go:501` | G703 | HIGH | false-positive |
| `cmd/lesser/ui_deploy.go:104` | G703 | HIGH | false-positive |
| `cmd/lesser/ui_deploy.go:73` | G703 | HIGH | false-positive |
| `cmd/lesser/ui_deploy.go:86` | G703 | HIGH | false-positive |
| `cmd/lesser/ui_deploy.go:91` | G703 | HIGH | false-positive |
| `cmd/lesser/up_release_assembly.go:54` | G703 | HIGH | false-positive |
| `cmd/owner-bootstrap/main.go:371-378` | G117 | MEDIUM | false-positive |
| `cmd/owner-bootstrap/main.go:384-391` | G117 | MEDIUM | false-positive |
| `cmd/push-delivery/main.go:362` | G117 | MEDIUM | false-positive |
| `infra/cdk/stacks/lambda_asset_root_certification.go:121` | G304 | MEDIUM | false-positive |
| `infra/cdk/stacks/lambda_asset_root_certification.go:33` | G301 | MEDIUM | false-positive |
| `pkg/common/cookies.go:35` | G124 | MEDIUM | false-positive |
| `pkg/common/redirect.go:88` | G710 | MEDIUM | false-positive |
| `pkg/config/validator.go:226-233` | G101 | HIGH | false-positive |
| `pkg/notifications/push.go:76` | G117 | MEDIUM | false-positive |
| `pkg/observability/http_tracker.go:87` | G704 | HIGH | false-positive |
| `pkg/releaseassets/auth_ui.go:33` | G301 | MEDIUM | false-positive |
| `pkg/releaseassets/auth_ui.go:41` | G302 | MEDIUM | false-positive |
| `pkg/releaseassets/auth_ui.go:41` | G304 | MEDIUM | false-positive |
| `pkg/releaseassets/bundle.go:139` | G301 | MEDIUM | false-positive |
| `pkg/releaseassets/bundle.go:147` | G302 | MEDIUM | false-positive |
| `pkg/releaseassets/bundle.go:147` | G304 | MEDIUM | false-positive |
| `pkg/releaseassets/bundle.go:248` | G306 | MEDIUM | false-positive |
| `pkg/releaseassets/checksums.go:45` | G306 | MEDIUM | false-positive |
| `pkg/releaseassets/deploy_assembly.go:224` | G301 | MEDIUM | false-positive |
| `pkg/releaseassets/deploy_assembly.go:393` | G306 | MEDIUM | false-positive |
| `pkg/releaseassets/deploy_assembly.go:585` | G204 | MEDIUM | false-positive |
| `pkg/releaseassets/deploy_assembly.go:753` | G302 | MEDIUM | false-positive |
| `pkg/releaseassets/deploy_assembly.go:753` | G304 | MEDIUM | false-positive |
| `pkg/releaseassets/files.go:80` | G302 | MEDIUM | false-positive |
| `pkg/releaseassets/files.go:80` | G304 | MEDIUM | false-positive |
| `pkg/releaseassets/release_manifest.go:137` | G306 | MEDIUM | false-positive |
| `pkg/storage/models/federation_relationship.go:501` | G115 | HIGH | false-positive |
| `pkg/storage/models/passkey_registration_proof.go:14` | G101 | HIGH | false-positive |
| `pkg/storage/repositories/push_subscription_repository.go:348` | G117 | MEDIUM | false-positive |
| `tools/openapi/main.go:379-408` | G101 | HIGH | false-positive |

## 7. Follow-ups

- **Bump the golangci-lint pin (named follow-up, deliberately NOT in this PR).** The repo
  pins golangci-lint **v2.11.4** (`scripts/install_ci_tools.sh`), which embeds gosec
  **v2.24.8-dev** — predating rules G702, G710, G124 — so the lint surface cannot report
  them (see §4). Bumping to a golangci-lint release embedding gosec ≥ v2.28.0 can surface
  new lint findings, which is a new triage pass under this record's methodology; it is
  therefore a separate, named change rather than a silent addition here.
