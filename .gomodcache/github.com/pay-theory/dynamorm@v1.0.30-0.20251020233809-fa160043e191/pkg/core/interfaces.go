// Package core defines the core interfaces and types for DynamORM
package core

import (
	"context"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

// DB represents the main database connection interface
type DB interface {
	// Model returns a new query builder for the given model
	Model(model any) Query

	// Transaction executes a function within a database transaction
	Transaction(fn func(tx *Tx) error) error

	// Migrate runs all pending migrations
	Migrate() error

	// AutoMigrate creates or updates tables based on the given models
	AutoMigrate(models ...any) error

	// Close closes the database connection
	Close() error

	// WithContext returns a new DB instance with the given context
	WithContext(ctx context.Context) DB
}

// ExtendedDB represents the full database interface with all available methods
// This interface includes schema management and Lambda-specific features
type ExtendedDB interface {
	DB

	// AutoMigrateWithOptions performs enhanced auto-migration with data copy support
	// opts should be of type schema.AutoMigrateOption
	AutoMigrateWithOptions(model any, opts ...any) error

	// RegisterTypeConverter registers a custom converter for a specific Go type, allowing
	// callers to override how values are marshaled to and unmarshaled from DynamoDB.
	RegisterTypeConverter(typ reflect.Type, converter pkgTypes.CustomConverter) error

	// CreateTable creates a DynamoDB table for the given model
	// opts should be of type schema.TableOption
	CreateTable(model any, opts ...any) error

	// EnsureTable checks if a table exists for the model and creates it if not
	EnsureTable(model any) error

	// DeleteTable deletes the DynamoDB table for the given model
	DeleteTable(model any) error

	// DescribeTable returns the table description for the given model
	// Returns *types.TableDescription
	DescribeTable(model any) (any, error)

	// WithLambdaTimeout sets a deadline based on Lambda context
	WithLambdaTimeout(ctx context.Context) DB

	// WithLambdaTimeoutBuffer sets a custom timeout buffer for Lambda execution
	WithLambdaTimeoutBuffer(buffer time.Duration) DB

	// TransactionFunc executes a function within a full transaction context
	// tx should be of type *transaction.Transaction
	TransactionFunc(fn func(tx any) error) error
}

// Query represents a chainable query builder interface
type Query interface {
	// Query construction
	Where(field string, op string, value any) Query
	Index(indexName string) Query
	Filter(field string, op string, value any) Query
	OrFilter(field string, op string, value any) Query
	FilterGroup(func(Query)) Query
	OrFilterGroup(func(Query)) Query
	OrderBy(field string, order string) Query
	Limit(limit int) Query

	// Offset sets the starting position for the query
	Offset(offset int) Query

	// Select specifies which fields to retrieve
	Select(fields ...string) Query

	// ConsistentRead enables strongly consistent reads for Query operations
	// Note: This only works on main table queries, not GSI queries
	ConsistentRead() Query

	// WithRetry configures retry behavior for eventually consistent reads
	// Useful for GSI queries where you need read-after-write consistency
	WithRetry(maxRetries int, initialDelay time.Duration) Query

	// First retrieves the first matching item
	First(dest any) error

	// All retrieves all matching items
	All(dest any) error

	// AllPaginated retrieves all matching items with pagination metadata
	AllPaginated(dest any) (*PaginatedResult, error)

	// Count returns the number of matching items
	Count() (int64, error)

	// Create creates a new item
	Create() error

	// CreateOrUpdate creates a new item or updates an existing one (upsert)
	CreateOrUpdate() error

	// Update updates the matching items
	Update(fields ...string) error

	// UpdateBuilder returns a builder for complex update operations
	UpdateBuilder() UpdateBuilder

	// Delete deletes the matching items
	Delete() error

	// Scan performs a table scan
	Scan(dest any) error

	// ParallelScan configures parallel scanning with segment and total segments
	ParallelScan(segment int32, totalSegments int32) Query

	// ScanAllSegments performs parallel scan across all segments automatically
	ScanAllSegments(dest any, totalSegments int32) error

	// BatchGet retrieves multiple items by their primary keys
	BatchGet(keys []any, dest any) error

	// BatchCreate creates multiple items
	BatchCreate(items any) error

	// BatchDelete deletes multiple items by their primary keys
	BatchDelete(keys []any) error

	// BatchWrite performs mixed batch write operations (puts and deletes)
	BatchWrite(putItems []any, deleteKeys []any) error

	// BatchUpdateWithOptions performs batch update operations with custom options
	BatchUpdateWithOptions(items []any, fields []string, options ...any) error

	// Cursor sets the pagination cursor for the query
	Cursor(cursor string) Query

	// SetCursor sets the cursor from a string (alternative to Cursor)
	SetCursor(cursor string) error

	// WithContext sets the context for the query
	WithContext(ctx context.Context) Query
}

// UpdateBuilder represents a fluent interface for building update operations
type UpdateBuilder interface {
	// Set updates a field to a new value
	Set(field string, value any) UpdateBuilder

	// SetIfNotExists sets a field value only if it doesn't already exist
	SetIfNotExists(field string, value any, defaultValue any) UpdateBuilder

	// Add performs atomic addition (for numbers) or adds to a set
	Add(field string, value any) UpdateBuilder

	// Increment increments a numeric field by 1
	Increment(field string) UpdateBuilder

	// Decrement decrements a numeric field by 1
	Decrement(field string) UpdateBuilder

	// Remove removes an attribute from the item
	Remove(field string) UpdateBuilder

	// Delete removes values from a set
	Delete(field string, value any) UpdateBuilder

	// AppendToList appends values to the end of a list
	AppendToList(field string, values any) UpdateBuilder

	// PrependToList prepends values to the beginning of a list
	PrependToList(field string, values any) UpdateBuilder

	// RemoveFromListAt removes an element at a specific index from a list
	RemoveFromListAt(field string, index int) UpdateBuilder

	// SetListElement sets a specific element in a list
	SetListElement(field string, index int, value any) UpdateBuilder

	// Condition adds a condition that must be met for the update to succeed
	Condition(field string, operator string, value any) UpdateBuilder

	// OrCondition adds a condition with OR logic
	OrCondition(field string, operator string, value any) UpdateBuilder

	// ConditionExists adds a condition that the field must exist
	ConditionExists(field string) UpdateBuilder

	// ConditionNotExists adds a condition that the field must not exist
	ConditionNotExists(field string) UpdateBuilder

	// ConditionVersion adds optimistic locking based on version
	ConditionVersion(currentVersion int64) UpdateBuilder

	// ReturnValues specifies what values to return after the update
	ReturnValues(option string) UpdateBuilder

	// Execute performs the update operation
	Execute() error

	// ExecuteWithResult performs the update and returns the result
	ExecuteWithResult(result any) error
}

// PaginatedResult contains the results and pagination metadata
type PaginatedResult struct {
	// Items contains the retrieved items
	Items any

	// Count is the number of items returned
	Count int

	// ScannedCount is the number of items examined
	ScannedCount int

	// LastEvaluatedKey is the key of the last item evaluated
	LastEvaluatedKey map[string]types.AttributeValue

	// NextCursor is a base64-encoded cursor for the next page
	NextCursor string

	// HasMore indicates if there are more results
	HasMore bool
}

// Tx represents a database transaction
type Tx struct {
	db DB
}

// SetDB sets the database reference for the transaction
func (tx *Tx) SetDB(db DB) {
	tx.db = db
}

// Model returns a new query builder for the given model within the transaction
func (tx *Tx) Model(model any) Query {
	return tx.db.Model(model)
}

// Create creates a new item within the transaction
func (tx *Tx) Create(model any) error {
	return tx.db.Model(model).Create()
}

// Update updates an item within the transaction
func (tx *Tx) Update(model any, fields ...string) error {
	return tx.db.Model(model).Update(fields...)
}

// Delete deletes an item within the transaction
func (tx *Tx) Delete(model any) error {
	return tx.db.Model(model).Delete()
}

// Param represents a parameter for expressions
type Param struct {
	Name  string
	Value any
}

// CompiledQuery represents a compiled query ready for execution
type CompiledQuery struct {
	Operation string // "Query", "Scan", "GetItem", etc.
	TableName string
	IndexName string

	// Expression components
	KeyConditionExpression string
	FilterExpression       string
	ProjectionExpression   string
	UpdateExpression       string
	ConditionExpression    string

	// Expression mappings
	ExpressionAttributeNames  map[string]string
	ExpressionAttributeValues map[string]types.AttributeValue

	// Other query parameters
	Limit             *int32
	ExclusiveStartKey map[string]types.AttributeValue
	ScanIndexForward  *bool
	Select            string // "ALL_ATTRIBUTES", "COUNT", etc.
	Offset            *int   // For pagination handling
	ReturnValues      string // "NONE", "ALL_OLD", "UPDATED_OLD", "ALL_NEW", "UPDATED_NEW"
	ConsistentRead    *bool  // For strongly consistent reads

	// Parallel scan parameters
	Segment       *int32 // The segment number for parallel scan
	TotalSegments *int32 // Total number of segments for parallel scan
}

// ModelMetadata provides metadata about a model
type ModelMetadata interface {
	TableName() string
	PrimaryKey() KeySchema
	Indexes() []IndexSchema
	AttributeMetadata(field string) *AttributeMetadata
}

// KeySchema represents a primary key or index key schema
type KeySchema struct {
	PartitionKey string
	SortKey      string // optional
}

// IndexSchema represents a GSI or LSI schema
type IndexSchema struct {
	Name            string
	Type            string // "GSI" or "LSI"
	PartitionKey    string
	SortKey         string
	ProjectionType  string
	ProjectedFields []string
}

// AttributeMetadata provides metadata about a model attribute
type AttributeMetadata struct {
	Name         string
	Type         string
	DynamoDBName string
	Tags         map[string]string
}
