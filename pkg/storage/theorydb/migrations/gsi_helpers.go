package migrations

import (
	"context"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// Projection type constants
const (
	projectionTypeInclude  = "INCLUDE"
	projectionTypeAll      = "ALL"
	projectionTypeKeysOnly = "KEYS_ONLY"
)

const (
	gsiMigrationPKPrefix = "GSI_MIGRATION#"
	gsiMigrationSKPrefix = "GSI#"

	gsiStatusCreated = "CREATED"
	gsiStatusDeleted = "DELETED"
)

// GSIMigrationRecord tracks GSI migration operations
type GSIMigrationRecord struct {
	PK             string    `theorydb:"pk"`
	SK             string    `theorydb:"sk"`
	TableName      string    `json:"table_name"`
	GSIName        string    `json:"gsi_name"`
	HashKey        string    `json:"hash_key"`
	HashKeyType    string    `json:"hash_key_type"`
	RangeKey       string    `json:"range_key,omitempty"`
	RangeKeyType   string    `json:"range_key_type,omitempty"`
	ProjectionType string    `json:"projection_type"`
	IncludeFields  []string  `json:"include_fields,omitempty"`
	ReadCapacity   int64     `json:"read_capacity"`
	WriteCapacity  int64     `json:"write_capacity"`
	Status         string    `json:"status"` // CREATED, DELETED, FAILED
	CreatedAt      time.Time `json:"created_at"`
	Error          string    `json:"error,omitempty"`
}

// GSIHelper provides utilities for managing Global Secondary Indexes using DynamORM
type GSIHelper struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewGSIHelper creates a new GSI helper using DynamORM
func NewGSIHelper(db core.DB, tableName string, logger *zap.Logger) (*GSIHelper, error) {
	if db == nil {
		return nil, fmt.Errorf("database instance is required")
	}
	if tableName == "" {
		return nil, fmt.Errorf("table name is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &GSIHelper{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}, nil
}

// GSIDefinition defines a Global Secondary Index
type GSIDefinition struct {
	Name           string
	HashKey        string
	HashKeyType    string // "S", "N", "B"
	RangeKey       string
	RangeKeyType   string // "S", "N", "B"
	ProjectionType string // "ALL", "KEYS_ONLY", "INCLUDE"
	IncludeFields  []string
	ReadCapacity   int64
	WriteCapacity  int64
}

// CreateGSI creates a new Global Secondary Index using DynamORM patterns
func (h *GSIHelper) CreateGSI(ctx context.Context, gsi GSIDefinition) error {
	h.logger.Info("Creating GSI with DynamORM",
		zap.String("table", h.tableName),
		zap.String("gsi", gsi.Name))

	// Note: DynamORM focuses on data operations, not schema management.
	// GSI creation should be handled via CDK/Terraform during deployment.
	// This method validates the GSI definition and records it for migration tracking.

	if err := h.validateGSIDefinition(gsi); err != nil {
		return fmt.Errorf("invalid GSI definition: %w", err)
	}

	if gsi.ProjectionType == "" {
		gsi.ProjectionType = projectionTypeAll
	}

	record := &GSIMigrationRecord{
		PK:             h.partitionKey(),
		SK:             h.sortKey(gsi.Name),
		TableName:      h.tableName,
		GSIName:        gsi.Name,
		HashKey:        gsi.HashKey,
		HashKeyType:    gsi.HashKeyType,
		RangeKey:       gsi.RangeKey,
		RangeKeyType:   gsi.RangeKeyType,
		ProjectionType: gsi.ProjectionType,
		IncludeFields:  append([]string(nil), gsi.IncludeFields...),
		ReadCapacity:   gsi.ReadCapacity,
		WriteCapacity:  gsi.WriteCapacity,
		Status:         gsiStatusCreated,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.db.WithContext(ctx).Model(record).Create(); err != nil {
		return fmt.Errorf("failed to persist GSI migration record: %w", err)
	}

	h.logger.Info("GSI migration record created",
		zap.String("table", h.tableName),
		zap.String("gsi", gsi.Name),
		zap.String("status", gsiStatusCreated))

	return nil
}

// DeleteGSI deletes a Global Secondary Index using DynamORM patterns
func (h *GSIHelper) DeleteGSI(ctx context.Context, gsiName string) error {
	if gsiName == "" {
		return fmt.Errorf("GSI name is required")
	}

	h.logger.Info("Deleting GSI with DynamORM",
		zap.String("table", h.tableName),
		zap.String("gsi", gsiName))

	// Note: DynamORM focuses on data operations, not schema management.
	// GSI deletion should be handled via CDK/Terraform during deployment.
	// This method records the deletion for migration tracking.

	record := &GSIMigrationRecord{}
	query := h.db.WithContext(ctx).Model(record).
		Where("PK", "=", h.partitionKey()).
		Where("SK", "=", h.sortKey(gsiName))

	if err := query.First(record); err != nil {
		return fmt.Errorf("failed to load GSI migration record: %w", err)
	}

	record.Status = gsiStatusDeleted
	record.Error = ""
	if record.TableName == "" {
		record.TableName = h.tableName
	}
	record.PK = h.partitionKey()
	record.SK = h.sortKey(gsiName)

	if err := query.Update(); err != nil {
		return fmt.Errorf("failed to update GSI migration record: %w", err)
	}

	h.logger.Info("GSI migration record updated for deletion",
		zap.String("table", h.tableName),
		zap.String("gsi", gsiName),
		zap.String("status", gsiStatusDeleted))

	return nil
}

// validateGSIDefinition validates a GSI definition
func (h *GSIHelper) validateGSIDefinition(gsi GSIDefinition) error {
	if gsi.Name == "" {
		return fmt.Errorf("GSI name is required")
	}
	if gsi.HashKey == "" {
		return fmt.Errorf("GSI hash key is required")
	}
	if gsi.HashKeyType == "" {
		return fmt.Errorf("GSI hash key type is required")
	}

	// Validate hash key type
	if gsi.HashKeyType != "S" && gsi.HashKeyType != "N" && gsi.HashKeyType != "B" {
		return fmt.Errorf("invalid hash key type: %s (must be S, N, or B)", gsi.HashKeyType)
	}

	// Validate range key type if provided
	if gsi.RangeKey != "" {
		if gsi.RangeKeyType == "" {
			return fmt.Errorf("range key type is required when range key is provided")
		}
		if gsi.RangeKeyType != "S" && gsi.RangeKeyType != "N" && gsi.RangeKeyType != "B" {
			return fmt.Errorf("invalid range key type: %s (must be S, N, or B)", gsi.RangeKeyType)
		}
	}

	// Validate projection type
	if gsi.ProjectionType == "" {
		gsi.ProjectionType = projectionTypeAll // Default
	}
	if gsi.ProjectionType != projectionTypeAll && gsi.ProjectionType != projectionTypeKeysOnly && gsi.ProjectionType != projectionTypeInclude {
		return fmt.Errorf("invalid projection type: %s", gsi.ProjectionType)
	}

	// Validate include fields for INCLUDE projection
	if gsi.ProjectionType == projectionTypeInclude && len(gsi.IncludeFields) == 0 {
		return fmt.Errorf("include fields are required for INCLUDE projection type")
	}

	return nil
}

// GetGSIStatus retrieves the status of a GSI migration
func (h *GSIHelper) GetGSIStatus(ctx context.Context, gsiName string) (*GSIMigrationRecord, error) {
	if gsiName == "" {
		return nil, fmt.Errorf("GSI name is required")
	}

	record := &GSIMigrationRecord{}
	query := h.db.WithContext(ctx).Model(record).
		Where("PK", "=", h.partitionKey()).
		Where("SK", "=", h.sortKey(gsiName))

	if err := query.First(record); err != nil {
		return nil, fmt.Errorf("failed to load GSI migration record: %w", err)
	}

	return record, nil
}

// ListGSIMigrations lists all GSI migration records
func (h *GSIHelper) ListGSIMigrations(ctx context.Context) ([]*GSIMigrationRecord, error) {
	h.logger.Info("Listing GSI migrations",
		zap.String("table", h.tableName))

	var records []*GSIMigrationRecord
	err := h.db.WithContext(ctx).Model(&GSIMigrationRecord{}).
		Where("PK", "=", h.partitionKey()).
		All(&records)
	if err != nil {
		return nil, fmt.Errorf("failed to list GSI migration records: %w", err)
	}

	return records, nil
}

// GSIMigration is a helper base for GSI-related migrations
type GSIMigration struct {
	BaseMigration
	TableName string
	GSI       GSIDefinition
}

// NewGSIMigration creates a new GSI migration
func NewGSIMigration(id string, version int64, description string, tableName string, gsi GSIDefinition, dependencies ...string) GSIMigration {
	return GSIMigration{
		BaseMigration: NewBaseMigration(id, version, description, dependencies...),
		TableName:     tableName,
		GSI:           gsi,
	}
}

// Up creates the GSI
func (m GSIMigration) Up(ctx context.Context, db core.DB) error {
	logger, _ := zap.NewProduction()
	helper, err := NewGSIHelper(db, m.TableName, logger)
	if err != nil {
		return err
	}

	return helper.CreateGSI(ctx, m.GSI)
}

// Down removes the GSI
func (m GSIMigration) Down(ctx context.Context, db core.DB) error {
	logger, _ := zap.NewProduction()
	helper, err := NewGSIHelper(db, m.TableName, logger)
	if err != nil {
		return err
	}

	return helper.DeleteGSI(ctx, m.GSI.Name)
}

func (h *GSIHelper) partitionKey() string {
	return fmt.Sprintf("%s%s", gsiMigrationPKPrefix, h.tableName)
}

func (h *GSIHelper) sortKey(gsiName string) string {
	return fmt.Sprintf("%s%s", gsiMigrationSKPrefix, gsiName)
}
