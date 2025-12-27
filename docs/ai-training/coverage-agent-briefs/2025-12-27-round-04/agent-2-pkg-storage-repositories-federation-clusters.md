# Agent 2 Brief — `pkg/storage/repositories` FederationRepository (clustering + cohesion helpers)

## Goal

Raise coverage for:

- `pkg/storage/repositories/federation_repository.go` (currently 0%)

This round focuses strictly on the **pure clustering / scoring helpers** (no HTTP, no DB, no AWS).

## Constraints (must follow)

- Run via CLI only:
  - `./lesser test unit`
  - `./lesser test coverage --scope pkg`
- No AWS calls, no network.
- Do not use `httptest.NewServer` (port binding isn’t available here).
- Prefer deterministic tests (no sleeps).
- Tests should live in package `repositories` (so you can call unexported helper methods).

## What to cover

### 1) `clusterByConnectionStrength`

Target: `func (r *FederationRepository) clusterByConnectionStrength(...)`

Cover:

- empty nodes → returns empty slice
- 2 nodes + strongia edge (`Strength > 0.5`) → one cluster with both instances
- threshold behavior:
  - verify that `addConnectedNodes` only adds edges above threshold
  - recursive threshold increase (`threshold+0.1`) doesn’t “sprawl” to weak edges

### 2) `findCenterNode`

Target: `func (r *FederationRepository) findCenterNode(...)`

Cover:

- single instance → returns itself
- two instances with equal connection strength:
  - healthy vs unhealthy weighting picks healthy as center
- verify default health weight for unknown health values

### 3) `calculateCohesion`

Target: `func (r *FederationRepository) calculateCohesion(...)`

Cover:

- size 1 returns `1.0`
- size 2 with strength 0.8 returns `0.8`
- size 3 with partial connections returns expected average strength / possible connections

### 4) `generateClusterDescription`

Target: `func (r *FederationRepository) generateClusterDescription(...)`

Cover:

- single-instance description format
- dominant software selection (counts)
- cohesion buckets:
  - `> 0.7` → “tightly”
  - `> 0.4` → “moderately”
  - else → “loosely”

Assertions should be substring-based (avoid exact string dependence except when safe).

## Deliverables

- New tests in `pkg/storage/repositories/`, suggested filename:
  - `federation_repository_clustering_test.go`
- Validation:
  - `./lesser test unit`
  - `./lesser lint`
  - `./lesser test coverage --scope pkg`

