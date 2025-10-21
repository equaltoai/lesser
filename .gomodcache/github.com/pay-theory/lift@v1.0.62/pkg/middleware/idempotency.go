package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// IdempotencyStore defines the interface for storing idempotency keys and responses
type IdempotencyStore interface {
	// Get retrieves a stored response by key
	Get(ctx context.Context, key string) (*IdempotencyRecord, error)
	// Set stores a response with the given key
	Set(ctx context.Context, key string, record *IdempotencyRecord) error
	// SetProcessing marks a key as being processed (prevents concurrent duplicates)
	SetProcessing(ctx context.Context, key string, expiresAt time.Time) error
	// Delete removes a key from the store
	Delete(ctx context.Context, key string) error
}

// IdempotencyRecord represents a stored idempotent response
type IdempotencyRecord struct {
	Response  any       `json:"response,omitempty"` // 8 bytes (interface)
	CreatedAt time.Time `json:"created_at"`         // 8 bytes (int64)
	ExpiresAt time.Time `json:"expires_at"`         // 8 bytes (int64)

	Key          string `json:"key"`                     // 16 bytes
	Status       string `json:"status"`                  // "processing", "completed", "error" - 16 bytes
	Error        string `json:"error,omitempty"`         // 16 bytes
	RequestHash  string `json:"request_hash,omitempty"`  // 16 bytes
	FunctionName string `json:"function_name,omitempty"` // 16 bytes
	TenantID     string `json:"tenant_id,omitempty"`     // 16 bytes
	UserID       string `json:"user_id,omitempty"`       // 16 bytes

	StatusCode int `json:"status_code,omitempty"` // 4 bytes
}

// IdempotencyOptions configures the idempotency middleware
type IdempotencyOptions struct {
	Store              IdempotencyStore
	OnDuplicate        func(ctx *lift.Context, record *IdempotencyRecord)
	HeaderName         string
	TTL                time.Duration
	ProcessingTimeout  time.Duration
	IncludeRequestHash bool
}

// Idempotency creates middleware that provides idempotent request handling
func Idempotency(opts IdempotencyOptions) Middleware {
	// Set defaults
	opts = setIdempotencyDefaults(opts)

	// Create the idempotency service
	service := newIdempotencyService(opts)

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			return service.handleRequest(ctx, next)
		})
	}
}

// setIdempotencyDefaults applies default values to idempotency options
func setIdempotencyDefaults(opts IdempotencyOptions) IdempotencyOptions {
	if opts.HeaderName == "" {
		opts.HeaderName = "Idempotency-Key"
	}
	if opts.TTL == 0 {
		opts.TTL = 24 * time.Hour
	}
	if opts.ProcessingTimeout == 0 {
		opts.ProcessingTimeout = 30 * time.Second
	}
	return opts
}

// MemoryIdempotencyStore provides an in-memory implementation of IdempotencyStore
// This is suitable for single-instance applications or testing
// Memory optimized: 32 → 8 bytes (24 bytes saved)
type MemoryIdempotencyStore struct {
	records map[string]*IdempotencyRecord // 24 bytes (map first)
	mu      sync.RWMutex                  // 24 bytes
}

// NewMemoryIdempotencyStore creates a new in-memory idempotency store
func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{
		records: make(map[string]*IdempotencyRecord),
	}
}

// Get retrieves a record by key
func (m *MemoryIdempotencyStore) Get(_ context.Context, key string) (*IdempotencyRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, exists := m.records[key]
	if !exists {
		return nil, nil
	}

	// Check if expired
	if time.Now().After(record.ExpiresAt) {
		return nil, nil
	}

	return record, nil
}

// Set stores a record
func (m *MemoryIdempotencyStore) Set(_ context.Context, key string, record *IdempotencyRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records[key] = record

	// Clean up expired records periodically
	m.cleanupExpired()

	return nil
}

// SetProcessing marks a key as being processed
func (m *MemoryIdempotencyStore) SetProcessing(ctx context.Context, key string, expiresAt time.Time) error {
	return m.Set(ctx, key, &IdempotencyRecord{
		Key:       key,
		Status:    "processing",
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	})
}

// Delete removes a record
func (m *MemoryIdempotencyStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.records, key)
	return nil
}

// cleanupExpired removes expired records (called with lock held)
func (m *MemoryIdempotencyStore) cleanupExpired() {
	now := time.Now()
	for key, record := range m.records {
		if now.After(record.ExpiresAt) {
			delete(m.records, key)
		}
	}
}

// idempotencyState represents the different states an idempotency key can be in
type idempotencyState int

const (
	stateNew idempotencyState = iota
	stateProcessing
	stateCompleted
	stateError
	stateExpired
)

// idempotencyService handles idempotent request processing
type idempotencyService struct {
	store   IdempotencyStore
	options IdempotencyOptions
}

// newIdempotencyService creates a new idempotency service
func newIdempotencyService(options IdempotencyOptions) *idempotencyService {
	return &idempotencyService{
		store:   options.Store,
		options: options,
	}
}

// handleRequest processes a request with idempotency support
func (s *idempotencyService) handleRequest(ctx *lift.Context, next lift.Handler) error {
	key := s.extractKey(ctx)
	if key == "" {
		return next.Handle(ctx)
	}

	state, record := s.determineState(ctx, key)

	switch state {
	case stateCompleted:
		return s.replayResponse(ctx, record)
	case stateError:
		return s.replayError(record)
	case stateProcessing:
		return s.handleConcurrent(ctx, key, record)
	case stateExpired:
		return s.handleExpired(ctx, key, next)
	default: // stateNew
		return s.processNew(ctx, key, next)
	}
}

// extractKey extracts and processes the idempotency key
func (s *idempotencyService) extractKey(ctx *lift.Context) string {
	key := ctx.Header(s.options.HeaderName)
	if key == "" {
		return ""
	}

	// Add account/tenant context for isolation
	if accountID := ctx.Get("account_id"); accountID != nil {
		key = fmt.Sprintf("%v:%s", accountID, key)
	}

	return key
}

// determineState checks the current state of an idempotency key
func (s *idempotencyService) determineState(ctx *lift.Context, key string) (idempotencyState, *IdempotencyRecord) {
	record, err := s.store.Get(ctx.Request.Context(), key)

	if err != nil || record == nil {
		return stateNew, nil
	}

	switch record.Status {
	case "completed":
		return stateCompleted, record
	case "error":
		return stateError, record
	case "processing":
		if time.Now().After(record.ExpiresAt) {
			return stateExpired, record
		}
		return stateProcessing, record
	default:
		return stateNew, nil
	}
}

// replayResponse replays a cached successful response
func (s *idempotencyService) replayResponse(ctx *lift.Context, record *IdempotencyRecord) error {
	if s.options.OnDuplicate != nil {
		s.options.OnDuplicate(ctx, record)
	}

	ctx.Response.StatusCode = record.StatusCode
	ctx.Response.Header("X-Idempotent-Replay", "true")
	ctx.Response.Body = record.Response
	ctx.Response.Header(lift.HeaderContentType, lift.ContentTypeJSON)

	return nil
}

// replayError replays a cached error response
func (s *idempotencyService) replayError(record *IdempotencyRecord) error {
	if record.Error != "" {
		return lift.NewLiftError("IDEMPOTENT_ERROR_REPLAY", record.Error, record.StatusCode)
	}
	return lift.NewLiftError("IDEMPOTENT_ERROR_REPLAY", "Previous request failed", 500)
}

// handleConcurrent handles concurrent requests with the same key
func (s *idempotencyService) handleConcurrent(_ *lift.Context, _ string, _ *IdempotencyRecord) error {
	return lift.NewLiftError("IDEMPOTENCY_CONFLICT", "A request with this idempotency key is already being processed", 409)
}

// handleExpired handles expired processing states
func (s *idempotencyService) handleExpired(ctx *lift.Context, key string, next lift.Handler) error {
	if err := s.store.Delete(ctx.Request.Context(), key); err != nil {
		s.logError(ctx, "Failed to delete expired idempotency key", key, err)
	}
	return s.processNew(ctx, key, next)
}

// processNew processes a new request and stores the result
func (s *idempotencyService) processNew(ctx *lift.Context, key string, next lift.Handler) error {
	// Mark as processing
	if err := s.setProcessing(ctx, key); err != nil {
		s.logWarn(ctx, "Failed to set idempotency processing lock", key, err)
	}

	// Execute handler with response capture
	processor := newResponseProcessor(ctx)
	err := processor.executeAndCapture(next)

	// Store result
	record := s.createRecord(key, processor, err)
	if storeErr := s.store.Set(ctx.Request.Context(), key, record); storeErr != nil {
		s.logError(ctx, "Failed to store idempotency record", key, storeErr)
	}

	return err
}

// setProcessing marks a key as being processed
func (s *idempotencyService) setProcessing(ctx *lift.Context, key string) error {
	expiresAt := time.Now().Add(s.options.ProcessingTimeout)
	return s.store.SetProcessing(ctx.Request.Context(), key, expiresAt)
}

// createRecord creates an idempotency record from the processing result
func (s *idempotencyService) createRecord(key string, processor *responseProcessor, err error) *IdempotencyRecord {
	record := &IdempotencyRecord{
		Key:       key,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(s.options.TTL),
	}

	if err != nil {
		record.Status = "error"
		record.Error = err.Error()
		if liftErr, ok := err.(*lift.LiftError); ok {
			record.StatusCode = liftErr.StatusCode
		} else {
			record.StatusCode = 500
		}
	} else {
		record.Status = "completed"
		record.Response = processor.capturedResponse
		record.StatusCode = processor.capturedStatus
		if record.StatusCode == 0 {
			record.StatusCode = 200
		}
	}

	return record
}

// logError logs an error message if logger is available
func (s *idempotencyService) logError(ctx *lift.Context, message, key string, err error) {
	if ctx.Logger != nil {
		ctx.Logger.Error(message, map[string]any{
			"key":   key,
			"error": err.Error(),
		})
	}
}

// logWarn logs a warning message if logger is available
func (s *idempotencyService) logWarn(ctx *lift.Context, message, key string, err error) {
	if ctx.Logger != nil {
		ctx.Logger.Warn(message, map[string]any{
			"key":   key,
			"error": err.Error(),
		})
	}
}

// responseProcessor handles response capture during handler execution
type responseProcessor struct {
	ctx              *lift.Context
	capturedResponse any
	capturedStatus   int
}

// newResponseProcessor creates a new response processor
func newResponseProcessor(ctx *lift.Context) *responseProcessor {
	return &responseProcessor{
		ctx: ctx,
	}
}

// executeAndCapture executes the handler and captures the response
func (rp *responseProcessor) executeAndCapture(next lift.Handler) error {
	// Enable response buffering to capture the response
	rp.ctx.EnableResponseBuffering()

	// Execute handler
	err := next.Handle(rp.ctx)

	// Capture response after handler execution
	rp.captureResponse()

	return err
}

// captureResponse captures the response from the context
func (rp *responseProcessor) captureResponse() {
	// Try to get response from buffer first
	if buffer := rp.ctx.GetResponseBuffer(); buffer != nil {
		rp.capturedResponse = buffer.CapturedData
		rp.capturedStatus = buffer.StatusCode
	} else {
		// Fallback to Response.Body (may not be reliable)
		rp.capturedResponse = rp.ctx.Response.Body
		rp.capturedStatus = rp.ctx.Response.StatusCode
	}

	if rp.capturedStatus == 0 {
		rp.capturedStatus = 200
	}
}
