package migrations

import (
	"context"
	"testing"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockDB for testing
type MockDB struct {
	mock.Mock
}

func (m *MockDB) Query(ctx context.Context) core.Query {
	args := m.Called(ctx)
	return args.Get(0).(core.Query)
}

func (m *MockDB) Get(ctx context.Context, key interface{}, result interface{}) error {
	args := m.Called(ctx, key, result)
	return args.Error(0)
}

func (m *MockDB) Put(ctx context.Context, item interface{}) error {
	args := m.Called(ctx, item)
	return args.Error(0)
}

func (m *MockDB) Delete(ctx context.Context, key interface{}) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockDB) Update(ctx context.Context) core.UpdateBuilder {
	args := m.Called(ctx)
	return args.Get(0).(core.UpdateBuilder)
}

func (m *MockDB) WithContext(ctx context.Context) core.DB {
	args := m.Called(ctx)
	return args.Get(0).(core.DB)
}

func (m *MockDB) BeginTransaction() core.DB {
	args := m.Called()
	return args.Get(0).(core.DB)
}

// Add other required methods for core.DB interface...

func TestValidator_ValidateIDFormat(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	migrator := &Migrator{logger: logger}
	validator := NewValidator(migrator, logger)
	
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid ID",
			id:      "20240101_add_user_table",
			wantErr: false,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "ID too long",
			id:      string(make([]byte, 101)),
			wantErr: true,
			errMsg:  "too long",
		},
		{
			name:    "ID with spaces",
			id:      "invalid id with spaces",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "ID with special chars",
			id:      "invalid/id",
			wantErr: true,
			errMsg:  "invalid character",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateIDFormat(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_CheckDuplicateVersions(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	registry := NewRegistry()
	
	// Add existing migrations
	registry.Register(&MockMigration{
		id:      "migration1",
		version: 20240101120000,
	})
	
	registry.Register(&MockMigration{
		id:      "migration2",
		version: 20240102120000,
	})
	
	migrator := &Migrator{
		registry: registry,
		logger:   logger,
	}
	validator := NewValidator(migrator, logger)
	
	// Test migration with duplicate version
	migration := &MockMigration{
		id:      "migration3",
		version: 20240101120000, // Same as migration1
	}
	
	result := &ValidationResult{Valid: true}
	validator.checkDuplicateVersions(migration, result)
	
	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, "duplicate_version", result.Warnings[0].Type)
	assert.Contains(t, result.Warnings[0].Message, "migration1")
}

func TestValidationResult_Format(t *testing.T) {
	result := &ValidationResult{
		Valid: false,
		Errors: []ValidationError{
			{
				MigrationID: "test_migration",
				Type:        "invalid_id",
				Message:     "Invalid migration ID",
			},
			{
				Type:    "dependency_error",
				Message: "Circular dependency detected",
			},
		},
		Warnings: []ValidationWarning{
			{
				MigrationID: "test_migration2",
				Type:        "duplicate_version",
				Message:     "Duplicate version number",
			},
		},
	}
	
	formatted := result.Format()
	
	// Check format contains expected elements
	assert.Contains(t, formatted, "✗ Validation failed")
	assert.Contains(t, formatted, "Errors:")
	assert.Contains(t, formatted, "[test_migration] invalid_id: Invalid migration ID")
	assert.Contains(t, formatted, "dependency_error: Circular dependency detected")
	assert.Contains(t, formatted, "Warnings:")
	assert.Contains(t, formatted, "[test_migration2] duplicate_version: Duplicate version number")
}

func TestValidationResult_Format_Valid(t *testing.T) {
	result := &ValidationResult{Valid: true}
	
	formatted := result.Format()
	assert.Contains(t, formatted, "✓ All validations passed")
}