# Phase 7 Final Cleanup Plan

## Current State
- MockStorageAdapter has been completely removed from all active Go files
- Build scripts (Makefile) are clean and don't reference old storage patterns
- Federation package still imports storage types instead of models
- storage/types.go contains 150+ type definitions that duplicate models

## Remaining Work

### 1. Federation Package Cleanup (In Progress)
- ✅ Updated FederationActivity references to use models
- TODO: Update other types:
  - InstanceMetadata → models.FederationInstance
  - FederationEdge → models.FederationStats
  - InstanceConnection → models.FederationInstance
  - RelayInfo → models.RelayInfo
  - InstanceHealthReport → models.InstanceHealthReport

### 2. storage/types.go Cleanup
This file contains 150+ type definitions that mostly duplicate models:
- Some are aliases (type TrustRelationship = models.TrustRelationship)
- Many are full duplicates that should be removed
- Need to:
  1. Identify which types are still used
  2. Update all references to use models instead
  3. Remove the duplicate definitions

### 3. Final Validation
- Run full build to ensure no compilation errors
- Check for any remaining imports of storage package (except models)
- Update documentation if needed

## Types Still Using storage Package
- pkg/federation/* - Multiple files still using storage types
- pkg/moderation/advanced/pattern_repository_bridge.go - Local adapter (OK)
- graph/dataloader_test.go - Test file
- scripts/generate_mocks.go - Script file

## Migration Strategy
1. Start with federation package - update all type references
2. Then tackle storage/types.go - migrate type by type
3. Finally clean up test and script files