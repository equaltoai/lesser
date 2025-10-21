// Package mocks provides mock implementations for DynamORM interfaces.
// These mocks are designed to be used with github.com/stretchr/testify/mock
// for unit testing applications that use DynamORM.
package mocks

import (
	"context"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/mock"
)

// MockQuery is a mock implementation of the core.Query interface.
// It can be used for unit testing code that depends on DynamORM queries.
//
// Example usage:
//
//	mockQuery := new(mocks.MockQuery)
//	mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
//	mockQuery.On("First", mock.Anything).Return(nil)
type MockQuery struct {
	mock.Mock
}

// Where adds a condition to the query
func (m *MockQuery) Where(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

// Index specifies which index to use
func (m *MockQuery) Index(indexName string) core.Query {
	args := m.Called(indexName)
	return args.Get(0).(core.Query)
}

// Filter adds a filter expression to the query
func (m *MockQuery) Filter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

// OrFilter adds an OR filter expression to the query
func (m *MockQuery) OrFilter(field string, op string, value any) core.Query {
	args := m.Called(field, op, value)
	return args.Get(0).(core.Query)
}

// FilterGroup adds a group of filters with AND logic
func (m *MockQuery) FilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return args.Get(0).(core.Query)
}

// OrFilterGroup adds a group of filters with OR logic
func (m *MockQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	args := m.Called(fn)
	return args.Get(0).(core.Query)
}

// OrderBy sets the sort order
func (m *MockQuery) OrderBy(field string, order string) core.Query {
	args := m.Called(field, order)
	return args.Get(0).(core.Query)
}

// Limit sets the maximum number of items to return
func (m *MockQuery) Limit(limit int) core.Query {
	args := m.Called(limit)
	return args.Get(0).(core.Query)
}

// Offset sets the starting position for the query
func (m *MockQuery) Offset(offset int) core.Query {
	args := m.Called(offset)
	return args.Get(0).(core.Query)
}

// Select specifies which fields to retrieve
func (m *MockQuery) Select(fields ...string) core.Query {
	args := m.Called(fields)
	return args.Get(0).(core.Query)
}

// First retrieves the first matching item
func (m *MockQuery) First(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// All retrieves all matching items
func (m *MockQuery) All(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// AllPaginated retrieves all matching items with pagination metadata
func (m *MockQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	args := m.Called(dest)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*core.PaginatedResult), args.Error(1)
}

// Count returns the number of matching items
func (m *MockQuery) Count() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

// Create creates a new item
func (m *MockQuery) Create() error {
	args := m.Called()
	return args.Error(0)
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (m *MockQuery) CreateOrUpdate() error {
	args := m.Called()
	return args.Error(0)
}

// Update updates the matching items
func (m *MockQuery) Update(fields ...string) error {
	args := m.Called(fields)
	return args.Error(0)
}

// UpdateBuilder returns a builder for complex update operations
func (m *MockQuery) UpdateBuilder() core.UpdateBuilder {
	args := m.Called()
	return args.Get(0).(core.UpdateBuilder)
}

// Delete deletes the matching items
func (m *MockQuery) Delete() error {
	args := m.Called()
	return args.Error(0)
}

// Scan performs a table scan
func (m *MockQuery) Scan(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

// ParallelScan configures parallel scanning
func (m *MockQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	args := m.Called(segment, totalSegments)
	return args.Get(0).(core.Query)
}

// ScanAllSegments performs parallel scan across all segments
func (m *MockQuery) ScanAllSegments(dest any, totalSegments int32) error {
	args := m.Called(dest, totalSegments)
	return args.Error(0)
}

// BatchGet retrieves multiple items by their primary keys
func (m *MockQuery) BatchGet(keys []any, dest any) error {
	args := m.Called(keys, dest)
	return args.Error(0)
}

// BatchCreate creates multiple items
func (m *MockQuery) BatchCreate(items any) error {
	args := m.Called(items)
	return args.Error(0)
}

// BatchDelete deletes multiple items by their primary keys
func (m *MockQuery) BatchDelete(keys []any) error {
	args := m.Called(keys)
	return args.Error(0)
}

// Cursor sets the pagination cursor
func (m *MockQuery) Cursor(cursor string) core.Query {
	args := m.Called(cursor)
	return args.Get(0).(core.Query)
}

// SetCursor sets the cursor from a string
func (m *MockQuery) SetCursor(cursor string) error {
	args := m.Called(cursor)
	return args.Error(0)
}

// WithContext sets the context for the query
func (m *MockQuery) WithContext(ctx context.Context) core.Query {
	args := m.Called(ctx)
	return args.Get(0).(core.Query)
}

// ConsistentRead enables strongly consistent reads for Query operations
func (m *MockQuery) ConsistentRead() core.Query {
	args := m.Called()
	return args.Get(0).(core.Query)
}

// WithRetry configures retry behavior for eventually consistent reads
func (m *MockQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	args := m.Called(maxRetries, initialDelay)
	return args.Get(0).(core.Query)
}

// BatchWrite performs mixed batch write operations
func (m *MockQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	args := m.Called(putItems, deleteKeys)
	return args.Error(0)
}

// BatchUpdateWithOptions performs batch update operations with custom options
func (m *MockQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	args := m.Called(items, fields, options)
	return args.Error(0)
}
