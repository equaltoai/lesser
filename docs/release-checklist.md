# Release checklist

This checklist is for maintainers preparing a release/deployable build.

## Preflight

- [ ] Clean working tree (`git status` is clean)
- [ ] Confirm Go version matches `go.mod` (`go version`)

## Verify gates (local or CI)

- [ ] Build lambdas: `./lesser build lambdas`
- [ ] Run the CI gate: `./lesser verify ci`
  - Generates a module inventory snapshot in `report/module_inventory.txt`

## Deploy

- [ ] Deploy with `./lesser up ...` and validate the setup wizard endpoints
- [ ] Confirm monitoring/alerts and federation health checks look normal
