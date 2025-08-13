# AI Agent Integration Guide: Service-First Handler Migration

## Critical Context

**DO NOT CREATE STUBS OR NEW BUSINESS LOGIC**

All 7 domain services are fully implemented with 120+ comprehensive tests:
- `pkg/services/notes/` - Complete with CreateNote, DeleteNote, etc.
- `pkg/services/accounts/` - Complete with UpdateProfile, GetAccount, etc.
- `pkg/services/relationships/` - Complete with Follow, Block, etc.
- Plus 4 other fully implemented services

**Your job**: Replace bloated handlers with thin service adapters.

## Handler Pattern (MANDATORY)

Every integrated handler must follow this EXACT pattern:

```go
func (h *Handler) SomeEndpoint(ctx *lift.Context) error {
    // 1. Parse request (3-5 lines)
    var req models.SomeRequest  
    if err := ctx.ParseRequest(&req); err != nil {
        return ctx.Status(400).JSON(map[string]string{"error": "invalid request"})
    }
    
    // 2. Authenticate (1 line using helper)
    claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
    if err != nil {
        return err // helper returns proper HTTP response
    }
    
    // 3. Call service (1-3 lines)
    result, err := h.registry.ServiceName().MethodName(ctx.Context, ServiceCommand{
        Field1: req.Field1,
        Field2: req.Field2,
        // Map request fields to command fields
    })
    if err != nil {
        return ctx.Status(500).JSON(map[string]string{"error": err.Error()})
    }
    
    // 4. Return response (1-2 lines)
    return ctx.JSON(result.SomeField) // or convert if needed
}
```

**Total lines**: 12-18 max per handler

## Task 3A.0: Authentication Helper (PREREQUISITE)

**File**: `cmd/api/lift/handler.go`
**Add this method after line 80**:

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

## Task 3B.1: POST /api/v1/statuses Integration

**File**: `cmd/api/lift/statuses_full.go`  
**Replace lines 33-280 with this EXACT implementation**:

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

**Pre-implementation checks**:
1. Verify Notes service method exists: `grep -n "CreateNote.*CreateNoteCommand" pkg/services/notes/service.go`
2. Check command struct: `grep -A 10 "type CreateNoteCommand struct" pkg/services/notes/service.go` 
3. Verify registry accessor: `grep -n "Notes()" pkg/services/registry.go`

**Success criteria**:
- Handler is <25 lines
- No `activitypub` imports
- No `h.repos.*` calls  
- No manual federation logic
- No manual event publishing
- Compiles without errors

## Task 3B.2: GET /api/v1/statuses/:id Integration

**File**: `cmd/api/lift/statuses_full.go`
**Replace lines 282-344 with**:

```go
// HandleGetStatusFull retrieves a status by ID using the Notes service
func (h *Handler) HandleGetStatusFull(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Optional authentication for privacy
	var viewerUsername string
	token := h.getBearerTokenLift(ctx)
	if token != "" {
		oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.repos)
		if claims, err := oauthSvc.ValidateAccessToken(token); err == nil {
			viewerUsername = claims.Username
		}
	}

	// Call Notes service
	note, err := h.registry.Notes().GetNote(ctx.Context, statusID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "Internal server error"})
	}

	// TODO: Apply privacy filtering based on viewerUsername
	return ctx.JSON(note)
}
```

## Task 3B.3: DELETE /api/v1/statuses/:id Integration  

**File**: `cmd/api/lift/statuses_full.go`
**Replace lines 346-458 with**:

```go
// HandleDeleteStatusFull deletes a status using the Notes service
func (h *Handler) HandleDeleteStatusFull(ctx *lift.Context) error {
	statusID := ctx.Param("id")
	if statusID == "" {
		return ctx.Status(http.StatusBadRequest).JSON(map[string]string{"error": "missing status id"})
	}

	// Authenticate with write scope
	claims, err := h.authenticateWithScope(ctx, auth.ScopeWrite)
	if err != nil {
		return err
	}

	// Call Notes service
	result, err := h.registry.Notes().DeleteNote(ctx.Context, &notes.DeleteNoteCommand{
		NoteID:   statusID,
		AuthorID: claims.Username,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ctx.Status(http.StatusNotFound).JSON(map[string]string{"error": "status not found"})
		}
		if strings.Contains(err.Error(), "not authorized") {
			return ctx.Status(http.StatusForbidden).JSON(map[string]string{"error": "not authorized to delete this status"})
		}
		return ctx.Status(http.StatusInternalServerError).JSON(map[string]string{"error": "failed to delete status"})
	}

	// Return deleted status
	return ctx.JSON(result.Note)
}
```

## Validation Commands

After each task, run these checks:

```bash
# 1. Compilation check
go build cmd/api/lift/

# 2. No business logic violations
grep -n "activitypub\." cmd/api/lift/statuses_full.go | wc -l  # Should be 0
grep -n "\.repos\." cmd/api/lift/statuses_full.go | wc -l     # Should be 0  
grep -n "federation\." cmd/api/lift/statuses_full.go | wc -l  # Should be 0

# 3. Line count check
wc -l cmd/api/lift/statuses_full.go  # Should be significantly reduced

# 4. Service usage verification  
grep -n "registry\." cmd/api/lift/statuses_full.go  # Should show service calls
```

## Anti-Patterns (NEVER DO THIS)

❌ **Creating ActivityPub objects in handlers**:
```go
note := &activitypub.Note{...}  // WRONG - services handle this
```

❌ **Direct repository calls**:  
```go
h.repos.Object().CreateObject(...)  // WRONG - use services
```

❌ **Manual federation**:
```go  
federationStorage := federation.New...  // WRONG - services handle this
```

❌ **Manual event publishing**:
```go
h.streamQueue.QueueEvent...  // WRONG - services handle this
```

✅ **Correct pattern**:
```go
result, err := h.registry.ServiceName().Method(ctx, command)  // RIGHT
```

## Next Steps After 3B.1-3B.3

Once status endpoints are successfully integrated:

1. **Verify service integration works**: Test create/get/delete flow
2. **Apply same pattern** to account endpoints (3B.4, 3B.5)
3. **Continue methodically** through remaining endpoints
4. **Delete old logic** from `*_full.go` files once replaced

## Key Success Metrics

- **Compilation**: All tasks must compile
- **Line reduction**: 80%+ reduction in handler code
- **Architecture compliance**: Zero business logic in handlers  
- **Service usage**: 100% of operations via registry.ServiceName()
- **Testing**: Existing service tests continue to pass