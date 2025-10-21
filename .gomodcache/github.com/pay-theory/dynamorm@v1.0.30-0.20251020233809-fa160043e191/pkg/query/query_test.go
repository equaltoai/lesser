package query_test

import (
	"testing"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock types for testing
type mockMetadata struct {
	mock.Mock
}

func (m *mockMetadata) TableName() string {
	return "test-table"
}

func (m *mockMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: "id",
		SortKey:      "timestamp",
	}
}

func (m *mockMetadata) Indexes() []core.IndexSchema {
	return []core.IndexSchema{
		{
			Name:           "status-index",
			Type:           "GSI",
			PartitionKey:   "status",
			SortKey:        "timestamp",
			ProjectionType: "ALL",
		},
		{
			Name:           "user-index",
			Type:           "GSI",
			PartitionKey:   "userId",
			SortKey:        "createdAt",
			ProjectionType: "KEYS_ONLY",
		},
	}
}

func (m *mockMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	return &core.AttributeMetadata{
		Name:         field,
		Type:         "S",
		DynamoDBName: field,
	}
}

type mockExecutor struct {
	mock.Mock
}

func (m *mockExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	args := m.Called(input, dest)
	return args.Error(0)
}

func (m *mockExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	args := m.Called(input, dest)
	return args.Error(0)
}

// Test model
type TestItem struct {
	ID        string `dynamodb:"id"`
	Timestamp int64  `dynamodb:"timestamp"`
	Status    string `dynamodb:"status"`
	UserID    string `dynamodb:"userId"`
	CreatedAt int64  `dynamodb:"createdAt"`
	Data      string `dynamodb:"data"`
}

func TestQuery_BasicQuery(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Test basic query with partition key
	q.Where("id", "=", "test-123")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Equal(t, "test-table", compiled.TableName)
	assert.NotEmpty(t, compiled.KeyConditionExpression)
	assert.Contains(t, compiled.KeyConditionExpression, "#n1 = :v1")
}

func TestQuery_QueryWithSortKey(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with partition and sort key
	q.Where("id", "=", "test-123").
		Where("timestamp", ">", 1000)

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Contains(t, compiled.KeyConditionExpression, "AND")
}

func TestQuery_IndexSelection(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query that should use status-index
	q.Where("status", "=", "active").
		Where("timestamp", ">", 1000)

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Query", compiled.Operation)
	assert.Equal(t, "status-index", compiled.IndexName)
}

func TestQuery_ScanFallback(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query without partition key should fall back to scan
	q.Where("data", "=", "some-value")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.Equal(t, "Scan", compiled.Operation)
	assert.NotEmpty(t, compiled.FilterExpression)
}

func TestQuery_ComplexFilters(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Complex query with multiple conditions
	q.Where("id", "=", "test-123").
		Where("timestamp", "BETWEEN", []any{1000, 2000}).
		Filter("status", "IN", []string{"active", "pending"})

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotEmpty(t, compiled.FilterExpression)
	assert.Contains(t, compiled.FilterExpression, "IN")
}

func TestQuery_Projection(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with projection
	q.Where("id", "=", "test-123").
		Select("id", "status", "data")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotEmpty(t, compiled.ProjectionExpression)
	assert.Contains(t, compiled.ProjectionExpression, "#n")
}

func TestQuery_Pagination(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with limit
	q.Where("id", "=", "test-123").
		Limit(10)

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotNil(t, compiled.Limit)
	assert.Equal(t, int32(10), *compiled.Limit)
}

func TestQuery_OrderBy(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Query with descending order
	q.Where("id", "=", "test-123").
		OrderBy("timestamp", "desc")

	compiled, err := q.Compile()
	assert.NoError(t, err)
	assert.NotNil(t, compiled.ScanIndexForward)
	assert.False(t, *compiled.ScanIndexForward)
}

func TestQuery_BatchGet(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockBatchExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Batch get with composite keys
	keys := []any{
		TestItem{ID: "test-1", Timestamp: 1000},
		TestItem{ID: "test-2", Timestamp: 2000},
	}

	var results []TestItem
	err := q.BatchGet(keys, &results)
	assert.NoError(t, err)
}

func TestQuery_BatchCreate(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockBatchExecutor{}

	q := query.New(&TestItem{}, metadata, executor)

	// Batch create
	items := []TestItem{
		{ID: "test-1", Timestamp: 1000, Status: "active"},
		{ID: "test-2", Timestamp: 2000, Status: "pending"},
	}

	err := q.BatchCreate(items)
	assert.NoError(t, err)
}

func TestQuery_ReservedWords(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	// Add a field with reserved word
	type ItemWithReserved struct {
		ID     string `dynamodb:"id"`
		Status string `dynamodb:"status"` // "STATUS" is a reserved word
		Size   int    `dynamodb:"size"`   // "SIZE" is a reserved word
	}

	q := query.New(&ItemWithReserved{}, metadata, executor)

	q.Where("status", "=", "active").
		Where("size", ">", 10)

	compiled, err := q.Compile()
	assert.NoError(t, err)

	// Check that reserved words are properly escaped
	assert.Contains(t, compiled.ExpressionAttributeNames, "#STATUS")
	assert.Contains(t, compiled.ExpressionAttributeNames, "#SIZE")
}

func TestQuery_ComplexExpressions(t *testing.T) {
	metadata := &mockMetadata{}
	executor := &mockExecutor{}

	// Test various operators
	testCases := []struct {
		name     string
		setup    func(*query.Query)
		validate func(*testing.T, *core.CompiledQuery)
	}{
		{
			name: "IN operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("status", "IN", []string{"active", "pending", "completed"})
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "IN")
			},
		},
		{
			name: "BEGINS_WITH operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("data", "BEGINS_WITH", "prefix")
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "begins_with")
			},
		},
		{
			name: "CONTAINS operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("data", "CONTAINS", "substring")
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "contains")
			},
		},
		{
			name: "EXISTS operator",
			setup: func(q *query.Query) {
				q.Where("id", "=", "test").
					Where("data", "EXISTS", nil)
			},
			validate: func(t *testing.T, c *core.CompiledQuery) {
				assert.Contains(t, c.FilterExpression, "attribute_exists")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := query.New(&TestItem{}, metadata, executor)
			tc.setup(q)
			compiled, err := q.Compile()
			assert.NoError(t, err)
			tc.validate(t, compiled)
		})
	}
}

// Mock batch executor for testing batch operations
type mockBatchExecutor struct {
	mockExecutor
}

func (m *mockBatchExecutor) ExecuteBatchGet(input *query.CompiledBatchGet, dest any) error {
	return nil
}

func (m *mockBatchExecutor) ExecuteBatchWrite(input *query.CompiledBatchWrite) error {
	return nil
}
