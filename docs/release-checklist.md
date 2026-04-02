# Release checklist

This checklist is for maintainers preparing a release/deployable build.

## Preflight

- [ ] Clean working tree (`git status` is clean)
- [ ] Confirm Go version matches `go.mod` (`go version`)

## Verify gates (local or CI)

- [ ] Build lambdas: `./lesser build lambdas`
- [ ] Run the CI gate: `./lesser verify ci`
  - Generates a module inventory snapshot in `report/module_inventory.txt`
- [ ] Run artifact-driven deploy certification: `bash scripts/verify_artifact_deploy.sh [dist/release]`

## Release contract scope

- [ ] Confirm the release contract still matches reality in `docs/contracts/release-driven-deploy-contract.md`
- [ ] Confirm `dist/release/` contains the published deploy assets:
  `lesser-lambda-bundle.tar.gz`, `lesser-lambda-bundle.json`, `lesser-auth-ui.tar.gz`,
  `lesser-deploy-assembly.tar.gz`, `lesser-deploy-assembly.json`, `lesser-release.json`, and `checksums.txt`
- [ ] Confirm the published release assets are sufficient for `./lesser up --release-dir ...` with a release-matched
      Lesser checkout, target-account AWS credentials, and `aws`/`cdk`/`go`

## Deploy

- [ ] Deploy with `./lesser up ...` and validate the setup wizard endpoints
- [ ] Confirm monitoring/alerts and federation health checks look normal
