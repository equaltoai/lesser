# Agent Governance Legacy Metadata Inventory

## Purpose

This document freezes the P0 inventory for agent-governance state that historically lived inside `User.Metadata`.
It exists to make the `AgentGovernanceState` cutover complete instead of partial.

## Legacy Metadata Keys

The legacy governance keys found on agent `USER#{username} / METADATA` rows are:

- `agent_quarantine_status`
- `agent_quarantine_start`
- `agent_quarantine_end`
- `agent_quarantine_approved_by`
- `agent_quarantine_approved_at`
- `agent_delegated_scopes`
- `agent_self_scopes`
- `agent_self_sovereign`
- `agent_verified`
- `agent_verified_at`
- `agent_verified_by`
- `agent_verified_reason`
- `agent_unverified_at`
- `agent_unverified_by`
- `agent_unverified_reason`
- `agent_key_rotated_at`

## Non-Test Writers

### REST API

- `cmd/api/handlers/agents.go`
  creates delegated agents, updates agents, and returns agent payloads
- `cmd/api/handlers/agent_governance.go`
  verifies, unverifies, and quarantine-approves agents
- `cmd/api/handlers/agent_self_sovereign.go`
  creates self-sovereign agents and records key rotation state

### GraphQL

- `graph/agent_resolvers_stubs.go`
  creates delegated agents, updates agents, verifies agents, unverifies agents, and performs quarantine exits

## Non-Test Readers

### REST API

- `cmd/api/handlers/agents.go`
  agent detail/list/update responses, delegation-envelope validation, capability ceilings
- `cmd/api/handlers/agent_safety.go`
  quarantine enforcement and verified-rate-limit selection
- `cmd/api/handlers/interactions.go`
  verified follow-limit selection
- `cmd/api/handlers/helpers.go`
  status attribution delegated-scope fallback
- `cmd/api/handlers/agent_self_sovereign.go`
  self-sovereign token scope resolution
- `cmd/api/handlers/agent_access_leases.go`
  delegated-scope enforcement for lease challenges

### GraphQL

- `graph/agent_model_helpers.go`
  agent payload conversion for delegated scopes and verified state
- `graph/agent_resolvers_stubs.go`
  agent list filtering, delegation-envelope validation, capability ceilings, and admin governance mutations
- `graph/agent_access_lease_resolvers.go`
  delegated-scope enforcement for lease challenges

### Live Data / Migration Boundary

- existing agent `USER#{username} / METADATA` rows in DynamoDB still carry legacy governance keys and can break generic user hydration until backfilled and cleaned

## Cutover Dependency List

P0 must land in this order:

1. Freeze this inventory and the typed storage contract.
2. Ship typed `AgentGovernanceState` model, repository, and service accessors.
3. Move all live mutation paths to typed writes.
4. Move all live read paths to typed reads.
5. Run the backfill against live agent rows and verify parity in Sim.
6. Remove legacy governance keys from user rows so `GetUser` and `GetAccount` no longer depend on governance-map decoding.

## P0 Exit Criteria

- no non-test runtime path reads quarantine or delegated-scope state from `User.Metadata`
- no non-test runtime path writes governance state into `User.Metadata`
- live agent rows have typed governance state present before legacy keys are removed
- post-migration account hydration succeeds without legacy governance metadata
