package theorydb

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

var newDynamormClient = func(cfg session.Config) (core.DB, error) {
	db, err := tabletheory.New(cfg)
	if err != nil {
		return nil, err
	}
	return db, nil
}

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
func NewLambdaOptimizedClient(_ context.Context, region string) (core.DB, error) {
	trimmed := strings.TrimSpace(region)
	if trimmed == "" {
		if envRegion := strings.TrimSpace(os.Getenv("AWS_REGION")); envRegion != "" {
			trimmed = envRegion
		} else if envDefault := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); envDefault != "" {
			trimmed = envDefault
		} else {
			trimmed = "us-east-1"
		}
	}

	config := session.Config{
		Region: trimmed,
	}

	// Use the standard client creation method with the latest DynamORM version
	client, err := newDynamormClient(config)
	if err != nil {
		return nil, err
	}

	if err := registerDefaultTypeConverters(client); err != nil {
		return nil, err
	}

	// Return the client as a core.DB interface
	return client, nil
}

// PreRegisterModels pre-registers models with the DynamORM client to reduce cold start time
// Note: The latest version of DynamORM doesn't have a PreRegisterModels method
// This is a placeholder for compatibility
func PreRegisterModels(_ core.DB, _ ...any) error {
	// In the latest version, models are registered automatically when used
	// So this is essentially a no-op
	return nil
}
