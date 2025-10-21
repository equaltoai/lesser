package mocks

import (
	"context"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/mock"
)

// MockExtendedDB is a complete mock implementation of core.ExtendedDB
type MockExtendedDB struct {
	mocks.MockDB // Embed MockDB to inherit base DB methods and mock.Mock
}

// Ensure MockExtendedDB implements ExtendedDB
var _ core.ExtendedDB = (*MockExtendedDB)(nil)

// AutoMigrateWithOptions performs enhanced auto-migration with options
func (m *MockExtendedDB) AutoMigrateWithOptions(model any, opts ...any) error {
	args := m.Called(model, opts)
	return args.Error(0)
}

// CreateTable creates a DynamoDB table for the given model
func (m *MockExtendedDB) CreateTable(model any, opts ...any) error {
	args := m.Called(model, opts)
	return args.Error(0)
}

// EnsureTable checks if a table exists and creates it if not
func (m *MockExtendedDB) EnsureTable(model any) error {
	args := m.Called(model)
	return args.Error(0)
}

// DeleteTable deletes the DynamoDB table for the given model
func (m *MockExtendedDB) DeleteTable(model any) error {
	args := m.Called(model)
	return args.Error(0)
}

// DescribeTable returns the table description for the given model
func (m *MockExtendedDB) DescribeTable(model any) (any, error) {
	args := m.Called(model)
	return args.Get(0), args.Error(1)
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (m *MockExtendedDB) WithLambdaTimeout(ctx context.Context) core.DB {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil
	}
	if db, ok := args.Get(0).(core.DB); ok {
		return db
	}
	return nil
}

// WithLambdaTimeoutBuffer sets a custom timeout buffer
func (m *MockExtendedDB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
	args := m.Called(buffer)
	if args.Get(0) == nil {
		return nil
	}
	if db, ok := args.Get(0).(core.DB); ok {
		return db
	}
	return nil
}

// TransactionFunc executes a function within a full transaction context
func (m *MockExtendedDB) TransactionFunc(fn func(tx any) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

// NewMockExtendedDB creates a new MockExtendedDB with sensible defaults
func NewMockExtendedDB() *MockExtendedDB {
	m := &MockExtendedDB{}

	// Set up default expectations for schema operations
	// These are rarely used in unit tests
	m.On("AutoMigrateWithOptions", mock.Anything, mock.Anything).
		Return(nil).Maybe()
	m.On("CreateTable", mock.Anything, mock.Anything).
		Return(nil).Maybe()
	m.On("EnsureTable", mock.Anything).
		Return(nil).Maybe()
	m.On("DeleteTable", mock.Anything).
		Return(nil).Maybe()
	m.On("DescribeTable", mock.Anything).
		Return(nil, nil).Maybe()

	// Lambda-specific methods typically return self
	m.On("WithLambdaTimeout", mock.Anything).
		Return(m).Maybe()
	m.On("WithLambdaTimeoutBuffer", mock.Anything).
		Return(m).Maybe()

	return m
}
