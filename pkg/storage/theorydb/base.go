package theorydb

import (
	"context"
	"time"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

// BaseModel provides common fields for all DynamORM models
type BaseModel struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate is a hook that sets CreatedAt and UpdatedAt before creating a record
func (m *BaseModel) BeforeCreate() error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	return nil
}

// BeforeUpdate is a hook that updates the UpdatedAt timestamp before updating a record
func (m *BaseModel) BeforeUpdate() error {
	m.UpdatedAt = time.Now()
	return nil
}

// Repository defines the interface for DynamoDB operations
// This is the base interface that all repositories should implement
type Repository interface {
	// GetTableName returns the name of the DynamoDB table
	GetTableName() string

	// GetDB returns the DynamORM client
	GetDB() core.DB
}

// BaseRepository provides common functionality for all repositories
type BaseRepository struct {
	db        core.DB
	tableName string
}

// NewBaseRepository creates a new BaseRepository
func NewBaseRepository(db core.DB, tableName string) *BaseRepository {
	return &BaseRepository{
		db:        db,
		tableName: tableName,
	}
}

// GetTableName returns the name of the DynamoDB table
func (r *BaseRepository) GetTableName() string {
	return r.tableName
}

// GetDB returns the DynamORM client
func (r *BaseRepository) GetDB() core.DB {
	return r.db
}

// NewLambdaOptimizedClient creates a new DynamORM client optimized for Lambda functions
// This should be used in the init() function of Lambda handlers to ensure connection reuse
func NewLambdaOptimizedClient(ctx context.Context, region string) (core.DB, error) {
	lambdaClient, err := newConfiguredLambdaOptimizedClient(lambdaOptimizedClientOptionsFor(region))
	if err != nil {
		return nil, err
	}

	if ctx != nil {
		return lambdaClient.WithLambdaTimeout(ctx), nil
	}

	return lambdaClient, nil
}

// PreRegisterModels pre-registers models with the DynamORM client to reduce cold start time
// Note: The latest version of DynamORM doesn't have a PreRegisterModels method
// This is a placeholder for compatibility
func PreRegisterModels(_ core.DB, _ ...any) error {
	// In the latest version, models are registered automatically when used
	// So this is essentially a no-op
	return nil
}
