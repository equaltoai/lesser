# Stub Mapping Analysis

## Overview
Lesser is a functional ActivityPub implementation with working core features. However, there are stubs scattered throughout that create gaps in functionality. This document maps where they are and their impact.

## Architecture Understanding

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   API Handlers  │────▶│  Storage Layer   │────▶│    DynamoDB     │
│   (Working)     │     │  (Working)       │     │   (Working)     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
         │                       ▲
         │                       │
         ▼                       │
┌─────────────────┐             │
│ Lambda Functions│─────────────┘
│ (Mixed: Some    │
│  have stubs)    │
└─────────────────┘
```

## Stub Categories

### 1. Export Generator Stubs (Isolated Lambda)
**Location**: `cmd/export-generator/main.go`
**Impact**: Export functionality produces empty files
**Why**: This Lambda doesn't use the storage layer that has real implementations

```go
// These are stubs:
getFollowers()      // returns []string{}, nil
getFollowing()      // returns []string{}, nil
getBlocks()         // returns []string{}, nil
getMutes()          // returns []MuteInfo{}, nil
getOutbox()         // returns []any{}, 0, nil
getLikes()          // returns []any{}, nil
// ... 6 more
```

### 2. Import/Export Job Management
**Location**: `cmd/api/handlers/imports.go`, `exports.go`
**Impact**: Can't list import/export history
**Functions**:
- `getUserImportJobs()` - returns empty array
- `getUserExportJobs()` - returns empty array

### 3. GraphQL API
**Location**: `graph/schema.resolvers.go`
**Impact**: GraphQL endpoint mostly non-functional
**Status**: 58/60 methods panic("not implemented")

### 4. Media Processing Stubs
**Location**: `cmd/media-processor/main.go`
**Impact**: Wrong metadata for video/audio
**Functions**:
- `processVideo()` - returns hardcoded 30s duration
- `processAudio()` - returns hardcoded 3min duration

### 5. Minor Feature Stubs
Various small features that return empty/default values:
- Follow requests (locked accounts not implemented)
- Some admin endpoints
- Translation caching
- Some search features

## What IS Working

### ✅ Core Social Network Features
- **Follow/Unfollow** - Full implementation
- **Posts (Create/Read/Update/Delete)** - Working
- **Likes** - Working
- **Timelines** - Working
- **Federation** - ActivityPub inbox/outbox working
- **Authentication** - OAuth2 implementation working

### ✅ Storage Layer
- All DynamoDB operations implemented
- Proper indexing with GSIs
- Pagination support
- Real data storage and retrieval

### ✅ API Layer
- Mastodon-compatible API endpoints
- Proper request/response handling
- Authentication/authorization

## Impact Analysis

### Critical Gaps (Blocking Features)
1. **Data Portability**: Export generates empty files
2. **Import/Export Management**: Can't see job history
3. **GraphQL**: Alternative API doesn't work

### Medium Impact
1. **Media**: Incorrect duration metadata
2. **Search**: Some search types not implemented

### Low Impact
1. **Locked Accounts**: Feature not implemented (returns empty)
2. **Translation Cache**: Falls back to direct translation

## The Pattern

Most stubs follow this pattern:
```go
func someFeature() ([]string, error) {
    // TODO: Implement this
    // This would normally query DynamoDB
    // For now, return empty to avoid errors
    return []string{}, nil
}
```

They're not blocking the core app - they're creating feature gaps.

## Fix Priority

### Week 1: Connect What Exists
1. Wire export-generator to use storage layer (~2-3 days)
2. Implement job listing queries (~1 day)

### Week 2: Fill Critical Gaps  
3. Media processing with real ffmpeg (~2-3 days)
4. Start GraphQL implementation (~ongoing)

### Week 3+: Complete Features
5. Remaining search implementations
6. Advanced features (locked accounts, etc.)

## Key Insight

Lesser is not "full of stubs" - it's a working system with specific feature gaps. The core social network functionality works. The stubs are primarily in:
1. Auxiliary services (export generator)
2. Alternative interfaces (GraphQL)
3. Advanced features (media processing)

This is a normal state for a complex system under development, not a fundamental flaw. 