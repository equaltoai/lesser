# Release checklist

This checklist is for maintainers preparing a release/deployable build.

## Preflight

- [ ] Clean working tree (`git status` is clean)
- [ ] Confirm Go version matches `go.mod` (`go version`)

## Verify gates (local or CI)

- [ ] Build lambdas: `./lesser build lambdas`
- [ ] Run the CI gate: `./lesser verify ci`
  - Generates a module inventory snapshot in `report/module_inventory.txt`

## Release contract scope

- [ ] Confirm the release contract still matches reality in `docs/contracts/release-driven-deploy-contract.md`
- [ ] Treat Lambda deploy bundles as a future release asset until milestone M1 publishes them; current deploy validation
  still runs through the source-based `lesser up` path

## Deploy

- [ ] Deploy with `./lesser up ...` and validate the setup wizard endpoints
- [ ] Confirm monitoring/alerts and federation health checks look normal
