# Immediate Next Steps for AI Agent Service Integration

## ✅ COMPLETED: Compilation Fixes

**Fixed Issues:**
- ❌ ActivityPub `Actor` field in `BaseObject` → ✅ Moved to correct location in `Activity` struct
- ❌ `IsBlocking()` method missing → ✅ Changed to `IsBlocked()` (existing method)
- ❌ `IsMuting()` method missing → ✅ Changed to `IsMuted()` via `Social()` repository
- ❌ `CreateMute()`/`DeleteMute()` wrong repo → ✅ Fixed to use `Social()` repository 
- ❌ `storage.MuteOptions` undefined → ✅ Replaced with proper `storage.Mute` struct
- ❌ `DeliverToActor()` missing → ✅ Changed to `DeliverToRecipients()` (existing method)
- ❌ Unused imports → ✅ Cleaned up

**Current Status:** 🟢 **FULL COMPILATION SUCCESS**
- `go build ./cmd/api/lift/` ✅ 
- `go build ./cmd/api/` ✅

## 🎯 READY FOR SERVICE INTEGRATION

**Available Resources:**
- ✅ All 7 domain services fully implemented with 120+ tests
- ✅ Service registry properly wired up in handlers (`h.registry`)
- ✅ Detailed implementation guide created (`AI_AGENT_INTEGRATION_GUIDE.md`)
- ✅ Precise task breakdown in updated checklist
- ✅ Working codebase with no compilation errors

**Key Service Methods Available:**
- `h.registry.Notes().CreateNote()`, `GetNote()`, `DeleteNote()`
- `h.registry.Accounts().UpdateProfile()`, `GetAccount()`, `SearchAccounts()`  
- `h.registry.Relationships().Follow()`, `Unfollow()`, `Block()`, `Mute()`, `GetRelationship()`
- `h.registry.Conversations().SendDirectMessage()`, `ListConversations()`
- `h.registry.Media().UploadMedia()`, `UpdateMedia()`, `GetMedia()`
- `h.registry.Lists().CreateList()`, `UpdateList()`, `DeleteList()`
- `h.registry.Notifications().CreateNotification()`, `MarkAsRead()`, `Clear()`

## 🚀 NEXT PHASE: Start Service Integration

### Step 1: Create Authentication Helper (PREREQUISITE)

**Task**: Add `authenticateWithScope()` helper to `cmd/api/lift/handler.go`

**Exact location**: After line 80 (after `getBearerTokenLift` method)

**Code to add**:
```go
// authenticateWithScope handles authentication and scope validation
func (h *Handler) authenticateWithScope(ctx *lift.Context, requiredScope string) (*auth.Claims, error) {
	token := h.getBearerTokenLift(ctx)
	if token == "" {
		return nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": "missing token"})
	}

	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return nil, ctx.Status(http.StatusUnauthorized).JSON(map[string]string{"error": err.Error()})
	}

	if !claims.HasScope(requiredScope) {
		return nil, ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "insufficient scope"})
	}

	return claims, nil
}
```

**Validation**: After adding, run `go build ./cmd/api/lift/` to confirm it compiles.

### Step 2: Integrate First Endpoint - POST /api/v1/statuses

**Task**: Replace `HandleCreateStatusFull()` in `cmd/api/lift/statuses_full.go`

**Target**: Lines 33-280 (248 lines of business logic)
**Goal**: Replace with ~20 lines using `h.registry.Notes().CreateNote()`

**Exact replacement**:
```go
// HandleCreateStatusFull creates a new status using the Notes service
func (h *Handler) HandleCreateStatusFull(ctx *lift.Context) error {
	// Parse request
	var req models.CreateStatusRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid request format"})
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Notes service
	result, err := h.registry.Notes().CreateNote(ctx.Context, &notes.CreateNoteCommand{
		AuthorID:    claims.Username,
		Content:     req.Status,
		Visibility:  req.Visibility,
		Sensitive:   req.Sensitive,
		Language:    req.Language,
		InReplyToID: req.InReplyToID,
		MediaIDs:    req.MediaIDs,
	})
	if err != nil {
		h.logger.Error("failed to create note", zap.Error(err))
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to create status"})
	}

	// Return created status
	return ctx.Status(http.StatusCreated).JSON(result.Note)
}
```

**Success Criteria:**
- Handler reduced from 248 lines to ~25 lines
- No `activitypub` imports in handler
- No direct `h.repos.*` calls
- No manual federation/event logic
- Compiles successfully
- Uses existing, tested service layer

### Step 3: Validate Service Integration Works

After Step 2, test that the service integration actually works:

```bash
# 1. Compilation check
go build ./cmd/api/lift/

# 2. Verify service layer is being used
grep -n "registry.Notes()" cmd/api/lift/statuses_full.go  # Should show the service call

# 3. Verify business logic removal
grep -n "activitypub\.\|federation\.\|streamQueue\." cmd/api/lift/statuses_full.go | wc -l  # Should be 0

# 4. Line count verification  
wc -l cmd/api/lift/statuses_full.go  # Should be significantly reduced
```

### Step 4: Continue with Remaining Endpoints

If Step 2 succeeds, continue with:
- **Task 3B.2**: `HandleGetStatusFull()` → `registry.Notes().GetNote()`
- **Task 3B.3**: `HandleDeleteStatusFull()` → `registry.Notes().DeleteNote()`
- **Task 3B.4**: `HandleVerifyCredentialsFull()` → `registry.Accounts().GetAccount()`
- **Task 3B.5**: `HandleUpdateCredentialsFull()` → `registry.Accounts().UpdateProfile()`

## 🔍 Key Architecture Violations to Fix

The current `*_full.go` handlers are doing what services should do:

**❌ WRONG (current handlers):**
- Creating ActivityPub objects manually
- Direct repository calls (`h.repos.Object().CreateObject()`)
- Manual federation queuing (`deliveryService.DeliverToRecipients()`)  
- Manual event publishing (`h.streamQueue.QueueEvent()`)
- Complex business logic (247 lines per handler)

**✅ RIGHT (target pattern):**
- Single service call (`h.registry.ServiceName().Method()`)
- Parse → Auth → Service → Return (12-20 lines total)
- All business logic in tested services
- Services handle ActivityPub, federation, events automatically

## 📋 Success Metrics

After each integration task:

- [ ] **Compilation**: Handler compiles without errors
- [ ] **Line Reduction**: 80%+ reduction in handler code  
- [ ] **Architecture Compliance**: Zero business logic in handlers
- [ ] **Service Usage**: 100% of operations via `registry.ServiceName()`
- [ ] **No Violations**: Zero ActivityPub/federation/streaming imports in handlers

## 🎯 Final Goal

Transform this bloated handler:
```go
// 247 lines of ActivityPub creation, federation, events, etc.
func (h *Handler) HandleCreateStatusFull(ctx *lift.Context) error {
    // Parse request (30 lines)
    // Create ActivityPub objects (50 lines)  
    // Store objects manually (40 lines)
    // Queue federation manually (60 lines)
    // Emit events manually (67 lines)
    // Build response manually (50+ lines)
}
```

Into this clean service adapter:
```go  
// 20 lines total
func (h *Handler) HandleCreateStatusFull(ctx *lift.Context) error {
    // Parse request (5 lines)
    // Auth (1 line)
    // Service call (7 lines)
    // Return (2 lines)
}
```

**The AI agent now has everything needed to succeed methodically.**