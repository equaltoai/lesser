# Release checklist

This checklist is for maintainers preparing a release/deployable build.

## Preflight

- [ ] Clean working tree (`git status` is clean)
- [ ] Confirm Go version matches `go.mod` (`go version`)

## Verify gates (local or CI)

- [ ] Build lambdas: `./lesser build lambdas`
- [ ] Build auth UI and verify CloudFront CSP hashes:
      `corepack pnpm --dir auth-ui -s build && bash scripts/verify_auth_ui_csp.sh`
- [ ] Run the CI gate: `./lesser verify ci`
  - Generates a module inventory snapshot in `report/module_inventory.txt`
- [ ] Run artifact-driven deploy certification: `bash scripts/verify_artifact_deploy.sh [dist/release]`

## Theory Cloud framework baseline

- [ ] Confirm release notes name the pinned framework baseline:
  - AppTheory `v1.6.0`
  - TableTheory `v1.8.3`
  - FaceTheory `v3.1.2` for client-app guidance
- [ ] Confirm auth UI dependency remediation remains intact (`cd auth-ui && corepack pnpm audit --prod` when touching
      `auth-ui/package.json` or `auth-ui/pnpm-lock.yaml`)
- [ ] Confirm the auth UI CSP hash gate passed after the final release auth UI build. The release asset builder runs
      `bash scripts/verify_auth_ui_csp.sh` before packaging `lesser-auth-ui.tar.gz`.
- [ ] Confirm any framework-release notes say what did **not** change: no DynamoDB schema migration, no Mastodon REST /
      GraphQL / ActivityPub response-shape change, no federation signing/verification behavior change, and no release
      artifact shape change unless the release intentionally includes one.
- [ ] For TableTheory timeout work, confirm Lambda-optimized clients retain the timeout safety buffer and that
      timeout-sensitive changes passed `./lesser verify ci`.
- [ ] For AppTheory strict-route work, confirm the affected route inventory/parity tests passed and the OpenAPI/GraphQL
      static contract files did not drift unexpectedly.
- [ ] For AppTheory CDK function work, confirm a representative `cdk synth`/template diff was reviewed and no unexpected
      Lambda replacement, permission, event-source, or log-retention changes were introduced. Never set timeouts on CDK
      commands.

## Release contract scope

- [ ] Confirm the release contract still matches reality in `docs/contracts/release-driven-deploy-contract.md`
- [ ] Confirm `dist/release/` contains the published deploy assets:
  `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json`, `lesser-auth-ui.tar.gz`,
  `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json`, `lesser-release.json`, and `checksums.txt`
- [ ] Confirm the published release assets are sufficient for `./lesser up --release-dir ...` with a release-matched
      Lesser checkout, target-account AWS credentials, and `aws`/`cdk`/`go`

## Deploy

- [ ] Deploy with `./lesser up ...` and validate the setup wizard endpoints
- [ ] Roll out through `dev` → optional `staging` → `live` with soak evidence; framework-maintenance releases do not
      skip stage soak
- [ ] Confirm monitoring/alerts and federation health checks look normal
- [ ] Keep prior Lambda function versions and the previous release/commit available as rollback targets
