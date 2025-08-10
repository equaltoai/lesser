# API Alignment Architecture - Service-First Design

## Core Principle: Single Service, Multiple Interfaces

All business logic lives in domain services. REST handlers, GraphQL resolvers, and WebSocket handlers are thin adapters that:
1. Parse and validate input
2. Authenticate/authorize the request
3. Call the appropriate service method
4. Return the result and emit events

## Architecture Layers

### 1. Domain Services (`pkg/services/`)

Each service encapsulates all business logic for a domain:

```go
// pkg/services/notes/service.go
type Service struct {
    repo      storage.NoteRepository
    publisher streaming.Publisher
    federation federation.Service
}

type CreateNoteCommand struct {
    ActorID     string
    Content     string
    Visibility  string
    InReplyToID *string
    MediaIDs    []string
}

type NoteResult struct {
    Note   *models.Note
    Events []streaming.Event
}

func (s *Service) CreateNote(ctx context.Context, cmd CreateNoteCommand) (*NoteResult, error) {
    // Validate
    // Create note in storage
    // Queue federation
    // Emit events
    // Return result
}
```

### 2. Unified Event Publisher (`pkg/streaming/publisher.go`)

Single publisher used by all services to emit events:

```go
type Publisher interface {
    // Core publishing methods
    PublishToUser(ctx context.Context, username string, event Event) error
    PublishToStream(ctx context.Context, stream string, event Event) error
    PublishToConversation(ctx context.Context, convID string, event Event) error
    
    // Batch operations
    PublishBatch(ctx context.Context, events []Event) error
}

type Event struct {
    Type      string      // "status.created", "notification.new", etc.
    Stream    string      // "user", "public", "hashtag:golang", etc.
    Payload   interface{} // The actual data
    Timestamp time.Time
}
```

### 3. API Adapters (Thin Layers)

#### REST Handler Example
```go
// cmd/api/lift/statuses.go
func (h *StatusesHandler) Create(c *lift.Context) error {
    // Parse request
    var req CreateStatusRequest
    if err := c.Bind(&req); err != nil {
        return lift.BadRequest(err)
    }
    
    // Call service
    result, err := h.noteService.CreateNote(c.Context(), notes.CreateNoteCommand{
        ActorID:    c.Actor.ID,
        Content:    req.Status,
        Visibility: req.Visibility,
        MediaIDs:   req.MediaIDs,
    })
    if err != nil {
        return lift.Error(err)
    }
    
    // Return response
    return c.JSON(200, h.converter.NoteToStatus(result.Note))
}
```

#### GraphQL Resolver Example
```go
// graph/schema.resolvers.go
func (r *mutationResolver) CreateStatus(ctx context.Context, input model.CreateStatusInput) (*model.Status, error) {
    // Get actor from context
    actor := auth.ActorFromContext(ctx)
    
    // Call service
    result, err := r.noteService.CreateNote(ctx, notes.CreateNoteCommand{
        ActorID:    actor.ID,
        Content:    input.Content,
        Visibility: input.Visibility,
        MediaIDs:   input.MediaIDs,
    })
    if err != nil {
        return nil, err
    }
    
    // Return response
    return r.converter.NoteToGraphQLStatus(result.Note), nil
}
```

#### WebSocket Handler Example
```go
// cmd/streaming/handlers/commands.go
func (h *CommandHandler) HandleCreateStatus(ctx context.Context, msg WSMessage) error {
    // Parse command
    var cmd CreateStatusCommand
    if err := json.Unmarshal(msg.Data, &cmd); err != nil {
        return h.sendError(msg.ConnectionID, err)
    }
    
    // Call service
    result, err := h.noteService.CreateNote(ctx, notes.CreateNoteCommand{
        ActorID:    msg.Actor.ID,
        Content:    cmd.Content,
        Visibility: cmd.Visibility,
        MediaIDs:   cmd.MediaIDs,
    })
    if err != nil {
        return h.sendError(msg.ConnectionID, err)
    }
    
    // Send response
    return h.sendResponse(msg.ConnectionID, "status.created", result.Note)
}
```

## Command + Event Pattern for Long-Running Operations

For operations that may exceed API Gateway timeouts (29s for REST/GraphQL, 10min idle for WebSocket):

### 1. Immediate Command Acceptance
```go
func (s *ImportService) StartImport(ctx context.Context, cmd StartImportCommand) (*ImportJob, error) {
    // Create job record
    job := &models.ImportJob{
        ID:        uuid.New().String(),
        ActorID:   cmd.ActorID,
        Type:      cmd.Type,
        Status:    "pending",
        CreatedAt: time.Now(),
    }
    
    // Save to DynamoDB (triggers stream)
    if err := s.repo.CreateImportJob(job); err != nil {
        return nil, err
    }
    
    // Emit job.started event
    s.publisher.PublishToUser(ctx, cmd.ActorID, Event{
        Type:    "import.started",
        Payload: job,
    })
    
    return job, nil
}
```

### 2. Background Processing via DynamoDB Streams
```go
// cmd/import-processor/main.go (triggered by DynamoDB stream)
func handleImportJob(record events.DynamoDBEventRecord) error {
    job := parseImportJob(record)
    
    // Process import (can run up to 15 minutes)
    for i, item := range importData {
        // Process item
        processItem(item)
        
        // Emit progress event every 100 items
        if i % 100 == 0 {
            publisher.PublishToUser(ctx, job.ActorID, Event{
                Type: "import.progress",
                Payload: map[string]interface{}{
                    "jobId":    job.ID,
                    "progress": float64(i) / float64(len(importData)),
                },
            })
        }
    }
    
    // Emit completion
    publisher.PublishToUser(ctx, job.ActorID, Event{
        Type: "import.completed",
        Payload: job,
    })
}
```

### 3. WebSocket Subscription to Progress
```javascript
// Client-side
ws.send(JSON.stringify({
    type: "subscribe",
    streams: ["user", "import.progress", "import.completed"]
}));

ws.send(JSON.stringify({
    type: "command",
    action: "import.start",
    data: { type: "mastodon", file: "..." }
}));

// Receive immediate acknowledgment
// { type: "import.started", data: { jobId: "...", status: "pending" } }

// Receive progress updates
// { type: "import.progress", data: { jobId: "...", progress: 0.25 } }

// Receive completion
// { type: "import.completed", data: { jobId: "...", status: "completed", itemsImported: 1000 } }
```

## Service Registry Pattern

To avoid circular dependencies and enable testing:

```go
// pkg/services/registry.go
type Registry struct {
    Notes         notes.Service
    Accounts      accounts.Service
    Relationships relationships.Service
    Media         media.Service
    Federation    federation.Service
    Publisher     streaming.Publisher
    // ... other services
}

// Dependency injection in handlers
func NewHandlers(reg *services.Registry) *Handlers {
    return &Handlers{
        statuses: &StatusesHandler{
            noteService: reg.Notes,
            converter:   mastodon.NewConverter(),
        },
        accounts: &AccountsHandler{
            accountService: reg.Accounts,
            converter:      mastodon.NewConverter(),
        },
        // ...
    }
}
```

## Repository Pattern for Storage

Each service uses a repository interface, not direct DynamoDB/S3 access:

```go
// pkg/storage/interfaces.go
type NoteRepository interface {
    Create(ctx context.Context, note *models.Note) error
    Get(ctx context.Context, id string) (*models.Note, error)
    Update(ctx context.Context, note *models.Note) error
    Delete(ctx context.Context, id string) error
    ListByActor(ctx context.Context, actorID string, opts ListOptions) ([]*models.Note, error)
}

// pkg/storage/dynamodb/notes.go
type noteRepository struct {
    table *dynamorm.Table[models.Note]
}

func (r *noteRepository) Create(ctx context.Context, note *models.Note) error {
    return r.table.Put(note).Execute(ctx)
}
```

## Event-Driven Federation

Federation happens asynchronously via events:

```go
// In service
func (s *NoteService) CreateNote(ctx context.Context, cmd CreateNoteCommand) (*NoteResult, error) {
    // ... create note ...
    
    // Queue federation delivery
    s.publisher.PublishToStream(ctx, "federation.outbox", Event{
        Type: "activity.created",
        Payload: map[string]interface{}{
            "activityID": activity.ID,
            "actorID":    cmd.ActorID,
            "type":       "Create",
            "object":     note,
        },
    })
    
    return result, nil
}

// cmd/federation-delivery/main.go listens to federation.outbox stream
```

## Benefits of This Architecture

1. **No Duplication**: Business logic exists once in services
2. **Testability**: Services can be unit tested without HTTP/GraphQL/WS
3. **Consistency**: All APIs behave identically because they use the same service
4. **Scalability**: Long operations handled via queues/streams
5. **Real-time**: Events flow naturally to WebSocket subscribers
6. **Federation**: Decoupled via event streams
7. **Flexibility**: Easy to add new API interfaces (gRPC, etc.)

## Implementation Strategy (Pre-Release Advantage)

Since Lesser has no existing user base, we can:

1. **Phase 1**: Build service layer from scratch
   - Design ideal service interfaces without legacy constraints
   - Use latest Go patterns and best practices
   - No need to maintain compatibility

2. **Phase 2**: Replace existing handlers entirely
   - Delete old handler logic
   - Implement clean handlers that only call services
   - No migration path needed

3. **Phase 3**: Implement all three APIs simultaneously
   - REST, GraphQL, and WebSocket all use same services from day one
   - No incremental rollout needed
   - Can break existing endpoints if needed for consistency

4. **Phase 4**: Delete legacy code
   - Remove any old implementations
   - Clean up technical debt
   - No deprecation period required

## Example: Complete Note Creation Flow

1. **Client** sends request (REST/GraphQL/WebSocket)
2. **Handler/Resolver** validates and calls `NoteService.CreateNote`
3. **NoteService**:
   - Validates business rules
   - Saves to repository
   - Emits events:
     - To user stream (for timeline)
     - To public stream (if public)
     - To federation queue (for delivery)
   - Returns result
4. **Handler/Resolver** returns response
5. **Background processors**:
   - Federation delivery sends to remote instances
   - Search indexer updates search index
   - Notification processor creates notifications
6. **WebSocket** subscribers receive real-time updates

This ensures complete feature parity across all API interfaces with minimal code duplication.
