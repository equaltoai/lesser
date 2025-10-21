package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pay-theory/lift/pkg/dynamorm"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/models"
)

const (
	statusCompleted  = "completed"
	statusProcessing = "processing"
)

// DynamORMIdempotencyStore implements IdempotencyStore using DynamORM
type DynamORMIdempotencyStore struct {
	wrapper *dynamorm.DynamORMWrapper
}

// NewDynamORMIdempotencyStore creates a new DynamORM-based idempotency store
// This assumes the DynamORM middleware has been configured in the Lift app
func NewDynamORMIdempotencyStore() *DynamORMIdempotencyStore {
	return &DynamORMIdempotencyStore{}
}

// NewDynamORMIdempotencyStoreWithWrapper creates a store with a specific DynamORM wrapper
func NewDynamORMIdempotencyStoreWithWrapper(wrapper *dynamorm.DynamORMWrapper) *DynamORMIdempotencyStore {
	return &DynamORMIdempotencyStore{
		wrapper: wrapper,
	}
}

// getDB gets the DynamORM wrapper from context or uses the configured one
func (d *DynamORMIdempotencyStore) getDB(ctx context.Context) (*dynamorm.DynamORMWrapper, error) {
	if d.wrapper != nil {
		return d.wrapper, nil
	}

	// Try to get from Lift context if available
	if liftCtx, ok := ctx.(*lift.Context); ok {
		return dynamorm.TenantDB(liftCtx)
	}

	return nil, fmt.Errorf("DynamORM not available - ensure DynamORM middleware is configured")
}

// Get retrieves a stored response by key
func (d *DynamORMIdempotencyStore) Get(ctx context.Context, key string) (*IdempotencyRecord, error) {
	db, err := d.getDB(ctx)
	if err != nil {
		return nil, err
	}

	// Create DynamORM model instance with key
	record := &models.IdempotencyRecord{
		IdempotencyKey: key,
		SK:             "IDEMPOTENCY",
	}

	// Use DynamORM to get the record
	err = db.Get(ctx, key, record)
	if err != nil {
		return nil, fmt.Errorf("record not found: %s", key)
	}

	// Convert DynamORM model to middleware record
	middlewareRecord := &IdempotencyRecord{
		Key:          record.IdempotencyKey,
		Status:       record.Status,
		StatusCode:   record.StatusCode,
		Error:        record.ErrorMessage,
		CreatedAt:    record.CreatedAt,
		ExpiresAt:    record.ExpiresAt,
		RequestHash:  record.RequestHash,
		FunctionName: record.FunctionName,
		TenantID:     record.TenantID,
	}

	// Unmarshal response if present
	if record.Response != "" {
		if err := json.Unmarshal([]byte(record.Response), &middlewareRecord.Response); err != nil {
			// If unmarshal fails, store as string
			middlewareRecord.Response = record.Response
		}
	}

	return middlewareRecord, nil
}

// Set stores a response with the given key
func (d *DynamORMIdempotencyStore) Set(ctx context.Context, key string, record *IdempotencyRecord) error {
	db, err := d.getDB(ctx)
	if err != nil {
		return err
	}

	// Convert middleware record to DynamORM model
	dynamormRecord := &models.IdempotencyRecord{
		IdempotencyKey: key,
		SK:             "IDEMPOTENCY",
		FunctionName:   record.FunctionName,
		TenantID:       record.TenantID,
		Status:         record.Status,
		Timestamp:      record.CreatedAt,
		RequestHash:    record.RequestHash,
		StatusCode:     record.StatusCode,
		ErrorMessage:   record.Error,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      time.Now(),
		ExpiresAt:      record.ExpiresAt,
	}

	// Marshal response to JSON if present
	if record.Response != nil {
		data, err := json.Marshal(record.Response)
		if err != nil {
			return err
		}
		dynamormRecord.Response = string(data)
	}

	// If this is a completed record, set completion time
	if record.Status == statusCompleted {
		dynamormRecord.CompletedAt = time.Now()
	}

	// Use DynamORM to save the record
	return db.Put(ctx, dynamormRecord)
}

// SetProcessing marks a key as being processed
func (d *DynamORMIdempotencyStore) SetProcessing(ctx context.Context, key string, expiresAt time.Time) error {
	db, err := d.getDB(ctx)
	if err != nil {
		return err
	}

	record := &models.IdempotencyRecord{
		IdempotencyKey: key,
		SK:             "IDEMPOTENCY",
		Status:         statusProcessing,
		Timestamp:      time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
		LockedUntil:    expiresAt,
	}

	return db.Put(ctx, record)
}

// Delete removes a key from the store
func (d *DynamORMIdempotencyStore) Delete(ctx context.Context, key string) error {
	db, err := d.getDB(ctx)
	if err != nil {
		return err
	}

	return db.Delete(ctx, key)
}
