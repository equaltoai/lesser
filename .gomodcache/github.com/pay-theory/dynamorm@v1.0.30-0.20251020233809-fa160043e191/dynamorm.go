// Package dynamorm provides a type-safe ORM for Amazon DynamoDB in Go
package dynamorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
	customerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/marshal"
	"github.com/pay-theory/dynamorm/pkg/model"
	queryPkg "github.com/pay-theory/dynamorm/pkg/query"
	"github.com/pay-theory/dynamorm/pkg/schema"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/pkg/transaction"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

// DB is the main DynamORM database instance
type DB struct {
	session             *session.Session
	registry            *model.Registry
	converter           *pkgTypes.Converter
	marshaler           *marshal.Marshaler // Optimized marshaler
	ctx                 context.Context
	mu                  sync.RWMutex
	lambdaDeadline      time.Time // Lambda execution deadline for timeout handling
	lambdaTimeoutBuffer time.Duration
	metadataCache       sync.Map // type -> *model.Metadata
}

// UnmarshalItem unmarshals a DynamoDB AttributeValue map into a Go struct.
// This is the recommended way to unmarshal DynamoDB stream records or any
// DynamoDB items when using DynamORM.
//
// The function respects DynamORM struct tags (dynamorm:"pk", dynamorm:"attr:name", etc.)
// and handles all DynamoDB attribute types correctly.
//
// Example usage with DynamoDB Streams:
//
//	func processDynamoDBStream(record events.DynamoDBEventRecord) (*MyModel, error) {
//	    image := record.Change.NewImage
//	    if image == nil {
//	        return nil, nil
//	    }
//
//	    var model MyModel
//	    if err := dynamorm.UnmarshalItem(image, &model); err != nil {
//	        return nil, fmt.Errorf("failed to unmarshal: %w", err)
//	    }
//
//	    return &model, nil
//	}
func UnmarshalItem(item map[string]types.AttributeValue, dest interface{}) error {
	// Use the internal unmarshalItem function from the query executor
	return queryPkg.UnmarshalItem(item, dest)
}

// UnmarshalItems unmarshals a slice of DynamoDB AttributeValue maps into a slice of Go structs.
// This is useful for batch operations or when processing multiple items from a query result.
func UnmarshalItems(items []map[string]types.AttributeValue, dest interface{}) error {
	// Use the internal unmarshalItems function from the query executor
	return queryPkg.UnmarshalItems(items, dest)
}

// UnmarshalStreamImage unmarshals a DynamoDB stream image (from Lambda events) into a Go struct.
// This function handles the conversion from Lambda's events.DynamoDBAttributeValue to the standard types.AttributeValue
// and then unmarshals into your DynamORM model.
//
// Example usage:
//
//	func handleStream(record events.DynamoDBEventRecord) error {
//	    var order Order
//	    if err := dynamorm.UnmarshalStreamImage(record.Change.NewImage, &order); err != nil {
//	        return err
//	    }
//	    // Process order...
//	}
func UnmarshalStreamImage(streamImage map[string]events.DynamoDBAttributeValue, dest interface{}) error {
	// Convert Lambda event AttributeValues to SDK v2 AttributeValues
	item := make(map[string]types.AttributeValue, len(streamImage))
	for k, v := range streamImage {
		item[k] = convertLambdaAttributeValue(v)
	}

	return UnmarshalItem(item, dest)
}

// convertLambdaAttributeValue converts a Lambda event AttributeValue to SDK v2 AttributeValue
func convertLambdaAttributeValue(attr events.DynamoDBAttributeValue) types.AttributeValue {
	switch attr.DataType() {
	case events.DataTypeString:
		return &types.AttributeValueMemberS{Value: attr.String()}
	case events.DataTypeNumber:
		return &types.AttributeValueMemberN{Value: attr.Number()}
	case events.DataTypeBinary:
		return &types.AttributeValueMemberB{Value: attr.Binary()}
	case events.DataTypeBoolean:
		return &types.AttributeValueMemberBOOL{Value: attr.Boolean()}
	case events.DataTypeNull:
		return &types.AttributeValueMemberNULL{Value: true}
	case events.DataTypeList:
		list := make([]types.AttributeValue, 0, len(attr.List()))
		for _, item := range attr.List() {
			list = append(list, convertLambdaAttributeValue(item))
		}
		return &types.AttributeValueMemberL{Value: list}
	case events.DataTypeMap:
		m := make(map[string]types.AttributeValue)
		for k, v := range attr.Map() {
			m[k] = convertLambdaAttributeValue(v)
		}
		return &types.AttributeValueMemberM{Value: m}
	case events.DataTypeStringSet:
		return &types.AttributeValueMemberSS{Value: attr.StringSet()}
	case events.DataTypeNumberSet:
		return &types.AttributeValueMemberNS{Value: attr.NumberSet()}
	case events.DataTypeBinarySet:
		return &types.AttributeValueMemberBS{Value: attr.BinarySet()}
	default:
		// This shouldn't happen, but return NULL if unknown type
		return &types.AttributeValueMemberNULL{Value: true}
	}
}

// New creates a new DynamORM instance with the given configuration
func New(config session.Config) (core.ExtendedDB, error) {
	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	converter := pkgTypes.NewConverter()

	return &DB{
		session:   sess,
		registry:  model.NewRegistry(),
		converter: converter,
		marshaler: marshal.New(converter),
		ctx:       context.Background(),
	}, nil
}

// NewBasic creates a new DynamORM instance that returns the basic DB interface
// Use this when you only need core functionality and want easier mocking
func NewBasic(config session.Config) (core.DB, error) {
	return New(config)
}

// RegisterTypeConverter registers a custom converter for a specific Go type. This allows
// callers to control how values are marshaled to and unmarshaled from DynamoDB without
// forking the internal marshaler. Registering a converter clears any cached marshalers
// so subsequent operations use the new logic.
func (db *DB) RegisterTypeConverter(typ reflect.Type, converter pkgTypes.CustomConverter) error {
	if typ == nil {
		return fmt.Errorf("converter type cannot be nil")
	}
	if converter == nil {
		return fmt.Errorf("converter implementation cannot be nil")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.converter.RegisterConverter(typ, converter)
	if db.marshaler != nil {
		db.marshaler.ClearCache()
	}
	return nil
}

// Model returns a new query builder for the given model
func (db *DB) Model(model any) core.Query {
	// Ensure model is registered
	if err := db.registry.Register(model); err != nil {
		// Log the error for debugging
		if db.ctx != nil {
			// Include context info if available
			return &errorQuery{err: fmt.Errorf("failed to register model %T: %w", model, err)}
		}
		// Return a query that will error on execution
		return &errorQuery{err: fmt.Errorf("failed to register model %T: %w", model, err)}
	}

	// Fast-path metadata lookup - cache for later use
	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// Check cache first
	if _, ok := db.metadataCache.Load(typ); !ok {
		// Get from registry and cache
		meta, err := db.registry.GetMetadata(model)
		if err != nil {
			return &errorQuery{err: fmt.Errorf("failed to get metadata for model %T: %w", model, err)}
		}
		db.metadataCache.Store(typ, meta)
	}

	// Use the context from the DB if query doesn't have one
	ctx := db.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	return &query{
		db:         db,
		model:      model,
		ctx:        ctx,
		builder:    expr.NewBuilder(),
		conditions: make([]condition, 0, 4), // Pre-allocate for typical use case
	}
}

// Transaction executes a function within a database transaction
func (db *DB) Transaction(fn func(tx *core.Tx) error) error {
	// For now, we'll use a simple wrapper that doesn't support full transaction features
	// Users should use TransactionFunc for full transaction support
	tx := &core.Tx{}
	// Set the db field to avoid nil pointer panic
	tx.SetDB(db)
	return fn(tx)
}

// AutoMigrate creates or updates tables based on the given models
func (db *DB) AutoMigrate(models ...any) error {
	manager := schema.NewManager(db.session, db.registry)

	for _, model := range models {
		if err := db.registry.Register(model); err != nil {
			return fmt.Errorf("failed to register model %T: %w", model, err)
		}

		// Check if table exists, create if not
		metadata, err := db.registry.GetMetadata(model)
		if err != nil {
			return err
		}

		exists, err := manager.TableExists(metadata.TableName)
		if err != nil {
			return fmt.Errorf("failed to check table existence: %w", err)
		}

		if !exists {
			if err := manager.CreateTable(model); err != nil {
				return fmt.Errorf("failed to create table for %T: %w", model, err)
			}
		}
	}

	return nil
}

// AutoMigrateWithOptions performs enhanced auto-migration with data copy support
func (db *DB) AutoMigrateWithOptions(model any, opts ...any) error {
	// Convert opts to the expected type
	var options []schema.AutoMigrateOption
	for _, opt := range opts {
		if option, ok := opt.(schema.AutoMigrateOption); ok {
			options = append(options, option)
		} else {
			return fmt.Errorf("invalid option type: expected schema.AutoMigrateOption, got %T", opt)
		}
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.AutoMigrateWithOptions(model, options...)
}

// CreateTable creates a DynamoDB table for the given model
func (db *DB) CreateTable(model any, opts ...any) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	// Convert opts to the expected type
	var options []schema.TableOption
	for _, opt := range opts {
		if option, ok := opt.(schema.TableOption); ok {
			options = append(options, option)
		} else {
			return fmt.Errorf("invalid option type: expected schema.TableOption, got %T", opt)
		}
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.CreateTable(model, options...)
}

// EnsureTable checks if a table exists for the model and creates it if not
func (db *DB) EnsureTable(model any) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	exists, err := manager.TableExists(metadata.TableName)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !exists {
		return manager.CreateTable(model)
	}

	return nil
}

// DeleteTable deletes the DynamoDB table for the given model
func (db *DB) DeleteTable(model any) error {
	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DeleteTable(metadata.TableName)
}

// DescribeTable returns the table description for the given model
func (db *DB) DescribeTable(model any) (any, error) {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return nil, fmt.Errorf("failed to register model %T: %w", model, err)
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DescribeTable(model)
}

// Close closes the database connection
func (db *DB) Close() error {
	// AWS SDK v2 clients don't need explicit closing
	return nil
}

// Migrate runs all pending migrations
func (db *DB) Migrate() error {
	// DynamORM doesn't support traditional migrations
	// Use infrastructure as code tools like Terraform or CloudFormation instead
	return fmt.Errorf("DynamORM does not support migrations. Use infrastructure as code tools (Terraform, CloudFormation) or AutoMigrate for development")
}

// WithContext returns a new DB instance with the given context
func (db *DB) WithContext(ctx context.Context) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (db *DB) WithLambdaTimeout(ctx context.Context) core.DB {
	deadline, ok := ctx.Deadline()
	if !ok {
		return db
	}

	// Leave a buffer for Lambda cleanup
	buffer := db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 500 * time.Millisecond // Default buffer
	}
	adjustedDeadline := deadline.Add(-buffer)

	db.mu.RLock()
	defer db.mu.RUnlock()

	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      adjustedDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// WithLambdaTimeoutBuffer sets a custom timeout buffer for Lambda execution
func (db *DB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Create new instance instead of modifying existing one to avoid race conditions
	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 db.ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: buffer, // Set the new buffer value
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// query implements the core.Query interface
type query struct {
	db      *DB
	model   any
	ctx     context.Context
	builder *expr.Builder

	// Query conditions
	conditions []condition
	indexName  string
	orderBy    *orderBy
	limit      *int
	offset     *int
	fields     []string

	// Parallel scan fields
	segment       *int32
	totalSegments *int32

	// Pagination fields
	exclusiveStartKey string

	// Consistency options
	consistentRead bool

	// Retry configuration
	retryConfig *retryConfig
}

type condition struct {
	field string
	op    string
	value any
}

type orderBy struct {
	field string
	order string
}

type retryConfig struct {
	maxRetries   int
	initialDelay time.Duration
}

// checkLambdaTimeout checks if Lambda execution is about to timeout
func (q *query) checkLambdaTimeout() error {
	if q.db.lambdaDeadline.IsZero() {
		return nil // No Lambda deadline set
	}

	remaining := time.Until(q.db.lambdaDeadline)
	if remaining <= 0 {
		return fmt.Errorf("lambda timeout exceeded")
	}

	// Use configurable buffer, default to 100ms
	buffer := q.db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 100 * time.Millisecond
	}

	// If we have less than the buffer, consider it too close to timeout
	if remaining < buffer {
		return fmt.Errorf("lambda timeout imminent: only %v remaining", remaining)
	}

	return nil
}

// Where adds a condition to the query
func (q *query) Where(field string, op string, value any) core.Query {
	q.conditions = append(q.conditions, condition{
		field: field,
		op:    op,
		value: value,
	})
	return q
}

// Index specifies which index to use for the query
func (q *query) Index(indexName string) core.Query {
	q.indexName = indexName
	return q
}

// Filter adds an AND filter condition
func (q *query) Filter(field string, op string, value any) core.Query {
	q.builder.AddFilterCondition("AND", field, op, value)
	return q
}

// OrFilter adds an OR filter condition
func (q *query) OrFilter(field string, op string, value any) core.Query {
	q.builder.AddFilterCondition("OR", field, op, value)
	return q
}

// FilterGroup adds a grouped AND filter condition
func (q *query) FilterGroup(fn func(q core.Query)) core.Query {
	q.addGroup("AND", fn)
	return q
}

// OrFilterGroup adds a grouped OR filter condition
func (q *query) OrFilterGroup(fn func(q core.Query)) core.Query {
	q.addGroup("OR", fn)
	return q
}

func (q *query) addGroup(logicalOp string, fn func(q core.Query)) {
	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilder()
	subQuery := &query{
		db:      q.db,
		model:   q.model,
		ctx:     q.ctx,
		builder: subBuilder,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter(logicalOp, components)
}

// OrderBy sets the sort order for the query
func (q *query) OrderBy(field string, order string) core.Query {
	q.orderBy = &orderBy{
		field: field,
		order: order,
	}
	return q
}

// Limit sets the maximum number of items to return
func (q *query) Limit(limit int) core.Query {
	q.limit = &limit
	return q
}

// Offset sets the starting position for the query
func (q *query) Offset(offset int) core.Query {
	q.offset = &offset
	return q
}

// Select specifies which fields to retrieve
func (q *query) Select(fields ...string) core.Query {
	q.fields = fields
	return q
}

// ConsistentRead enables strongly consistent reads for Query operations
func (q *query) ConsistentRead() core.Query {
	q.consistentRead = true
	return q
}

// WithRetry configures retry behavior for eventually consistent reads
func (q *query) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	q.retryConfig = &retryConfig{
		maxRetries:   maxRetries,
		initialDelay: initialDelay,
	}
	return q
}

// First retrieves the first matching item
func (q *query) First(dest any) error {
	if q.retryConfig != nil {
		return q.firstWithRetry(dest)
	}
	return q.firstInternal(dest)
}

// firstInternal is the actual implementation of First
func (q *query) firstInternal(dest any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build GetItem request if we have a primary key condition
	if pk := q.extractPrimaryKey(metadata); pk != nil {
		// Use optimized path when no projections are specified
		if len(q.fields) == 0 {
			return q.getItemDirect(metadata, pk, dest)
		}
		return q.getItem(metadata, pk, dest)
	}

	// Otherwise, use Query with limit 1
	q.limit = new(int)
	*q.limit = 1

	// Create a slice to hold results
	sliceType := reflect.SliceOf(reflect.TypeOf(dest).Elem())
	results := reflect.New(sliceType).Interface()

	if err := q.All(results); err != nil {
		return err
	}

	// Extract first result
	resultsValue := reflect.ValueOf(results).Elem()
	if resultsValue.Len() == 0 {
		return customerrors.ErrItemNotFound
	}

	// Copy first item to dest
	reflect.ValueOf(dest).Elem().Set(resultsValue.Index(0))
	return nil
}

// firstWithRetry executes First with retry logic
func (q *query) firstWithRetry(dest any) error {
	delay := q.retryConfig.initialDelay
	maxDelay := 5 * time.Second
	backoffFactor := 2.0

	for attempt := 0; attempt <= q.retryConfig.maxRetries; attempt++ {
		err := q.firstInternal(dest)

		// If successful or it's not a "not found" error, return
		if err == nil || (err != customerrors.ErrItemNotFound && attempt == q.retryConfig.maxRetries) {
			return err
		}

		// Don't sleep on the last attempt
		if attempt < q.retryConfig.maxRetries {
			time.Sleep(delay)

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	return customerrors.ErrItemNotFound
}

// All retrieves all matching items
func (q *query) All(dest any) error {
	if q.retryConfig != nil {
		return q.allWithRetry(dest)
	}
	return q.allInternal(dest)
}

// allInternal is the actual implementation of All
func (q *query) allInternal(dest any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Determine if we should use Query or Scan
	useQuery := false
	var keyConditions []condition
	var filterConditions []condition

	// Check if we have partition key condition
	for _, cond := range q.conditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if !exists {
			filterConditions = append(filterConditions, cond)
			continue
		}

		if fieldMeta.IsPK || (q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata)) {
			// DynamoDB supports these operators for key conditions:
			// Partition key: = (equality only)
			// Sort key: =, <, <=, >, >=, BETWEEN, BEGINS_WITH
			if cond.op == "=" || cond.op == "BEGINS_WITH" ||
				cond.op == "<" || cond.op == "<=" || cond.op == ">" || cond.op == ">=" ||
				cond.op == "BETWEEN" {
				keyConditions = append(keyConditions, cond)
				useQuery = true
			} else {
				filterConditions = append(filterConditions, cond)
			}
		} else {
			filterConditions = append(filterConditions, cond)
		}
	}

	// Execute Query or Scan
	var items []map[string]types.AttributeValue
	if useQuery {
		items, err = q.executeQuery(metadata, keyConditions, filterConditions)
	} else {
		items, err = q.executeScan(metadata, filterConditions)
	}

	if err != nil {
		return err
	}

	// Unmarshal items to destination slice
	return q.unmarshalItems(items, dest, metadata)
}

// allWithRetry executes All with retry logic
func (q *query) allWithRetry(dest any) error {
	delay := q.retryConfig.initialDelay
	maxDelay := 5 * time.Second
	backoffFactor := 2.0

	// Get the slice value to check if results are empty
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	var lastErr error
	for attempt := 0; attempt <= q.retryConfig.maxRetries; attempt++ {
		// Clear the slice before each attempt
		destValue.Elem().Set(reflect.MakeSlice(destValue.Elem().Type(), 0, 0))

		err := q.allInternal(dest)
		lastErr = err

		// If we have an error (other than no results), return it
		if err != nil {
			// For actual errors, keep retrying unless it's the last attempt
			if attempt < q.retryConfig.maxRetries {
				time.Sleep(delay)
				delay = time.Duration(float64(delay) * backoffFactor)
				if delay > maxDelay {
					delay = maxDelay
				}
				continue
			}
			return err
		}

		// If successful and we have results, return
		if destValue.Elem().Len() > 0 {
			return nil
		}

		// No error but empty results - retry if not last attempt
		if attempt < q.retryConfig.maxRetries {
			time.Sleep(delay)

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	// Return success even if empty after retries (All doesn't error on empty results)
	// This maintains backward compatibility - callers should check if slice is empty
	return lastErr
}

// Count returns the number of matching items
func (q *query) Count() (int64, error) {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return 0, err
	}

	// Determine if we should use Query or Scan
	useQuery := false
	var keyConditions []condition
	var filterConditions []condition

	// Check if we have partition key condition
	for _, cond := range q.conditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if !exists {
			filterConditions = append(filterConditions, cond)
			continue
		}

		if fieldMeta.IsPK || (q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata)) {
			// DynamoDB supports these operators for key conditions:
			// Partition key: = (equality only)
			// Sort key: =, <, <=, >, >=, BETWEEN, BEGINS_WITH
			if cond.op == "=" || cond.op == "BEGINS_WITH" ||
				cond.op == "<" || cond.op == "<=" || cond.op == ">" || cond.op == ">=" ||
				cond.op == "BETWEEN" {
				keyConditions = append(keyConditions, cond)
				useQuery = true
			} else {
				filterConditions = append(filterConditions, cond)
			}
		} else {
			filterConditions = append(filterConditions, cond)
		}
	}

	// Execute count operation
	if useQuery {
		return q.executeQueryCount(metadata, keyConditions, filterConditions)
	}
	return q.executeScanCount(metadata, filterConditions)
}

// Create creates a new item
func (q *query) Create() error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Put item
	err = q.putItem(metadata)
	if err != nil {
		// Provide a more helpful error message for duplicate key errors
		if errors.Is(err, customerrors.ErrConditionFailed) {
			return fmt.Errorf("%w: item with the same key already exists", customerrors.ErrConditionFailed)
		}
		return err
	}

	return nil
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (q *query) CreateOrUpdate() error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Marshal the model to DynamoDB item
	item, err := q.marshalItem(q.model, metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem input without condition expression (allowing overwrites)
	input := &dynamodb.PutItemInput{
		TableName: aws.String(metadata.TableName),
		Item:      item,
	}

	// Execute PutItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for put item: %w", err)
	}

	_, err = client.PutItem(q.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put item: %w", err)
	}

	// Update timestamp fields in the original model
	q.updateTimestampsInModel(metadata)

	return nil
}

// Update updates the matching items
func (q *query) Update(fields ...string) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build UpdateItem request
	return q.updateItem(metadata, fields)
}

// Delete deletes the matching items
func (q *query) Delete() error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Build DeleteItem request
	return q.deleteItem(metadata)
}

// Scan performs a table scan
func (q *query) Scan(dest any) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Convert all conditions to filter conditions for scan
	var filterConditions []condition
	filterConditions = append(filterConditions, q.conditions...)

	// Execute scan
	items, err := q.executeScan(metadata, filterConditions)
	if err != nil {
		return err
	}

	// Unmarshal items to destination slice
	return q.unmarshalItems(items, dest, metadata)
}

// BatchGet retrieves multiple items by their primary keys
func (q *query) BatchGet(keys []any, dest any) error {
	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Build batch get request
	keysAndAttributes := &types.KeysAndAttributes{
		Keys: make([]map[string]types.AttributeValue, 0, len(keys)),
	}

	// Add projection if specified
	if len(q.fields) > 0 {
		builder := expr.NewBuilder()
		builder.AddProjection(q.fields...)
		components := builder.Build()

		if components.ProjectionExpression != "" {
			keysAndAttributes.ProjectionExpression = aws.String(components.ProjectionExpression)
			keysAndAttributes.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
	}

	// Convert keys to DynamoDB format
	for _, key := range keys {
		keyMap := make(map[string]types.AttributeValue)

		// Handle different key formats
		switch k := key.(type) {
		case map[string]any:
			// Key is a map with pk and optional sk
			if pk, hasPK := k["pk"]; hasPK {
				av, err := q.db.converter.ToAttributeValue(pk)
				if err != nil {
					return fmt.Errorf("failed to convert partition key: %w", err)
				}
				keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
			}
			if sk, hasSK := k["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
				av, err := q.db.converter.ToAttributeValue(sk)
				if err != nil {
					return fmt.Errorf("failed to convert sort key: %w", err)
				}
				keyMap[metadata.PrimaryKey.SortKey.DBName] = av
			}
		default:
			// Check if key is a struct with the same type as our model
			keyValue := reflect.ValueOf(key)
			if keyValue.Kind() == reflect.Ptr {
				keyValue = keyValue.Elem()
			}

			if keyValue.Kind() == reflect.Struct {
				// Extract primary key fields from struct
				for _, field := range metadata.Fields {
					if field.IsPK {
						fieldValue := keyValue.FieldByIndex(field.IndexPath)
						av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
						if err != nil {
							return fmt.Errorf("failed to convert partition key: %w", err)
						}
						keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
					} else if field.IsSK && metadata.PrimaryKey.SortKey != nil {
						fieldValue := keyValue.FieldByIndex(field.IndexPath)
						av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
						if err != nil {
							return fmt.Errorf("failed to convert sort key: %w", err)
						}
						keyMap[metadata.PrimaryKey.SortKey.DBName] = av
					}
				}
			} else {
				// Key is just the partition key value
				av, err := q.db.converter.ToAttributeValue(key)
				if err != nil {
					return fmt.Errorf("failed to convert partition key: %w", err)
				}
				keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
			}
		}

		// Validate that we have at least a partition key
		if len(keyMap) == 0 {
			return fmt.Errorf("invalid key: missing partition key")
		}

		keysAndAttributes.Keys = append(keysAndAttributes.Keys, keyMap)
	}

	// Build BatchGetItem input
	input := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			metadata.TableName: *keysAndAttributes,
		},
	}

	// Execute batch get
	var allItems []map[string]types.AttributeValue

	// Get client once for the entire batch operation
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for batch get: %w", err)
	}

	for {
		output, err := client.BatchGetItem(q.ctx, input)
		if err != nil {
			return fmt.Errorf("failed to batch get items: %w", err)
		}

		// Collect items
		if items, exists := output.Responses[metadata.TableName]; exists {
			allItems = append(allItems, items...)
		}

		// Check for unprocessed keys
		if len(output.UnprocessedKeys) == 0 {
			break
		}

		// Retry unprocessed keys
		input.RequestItems = output.UnprocessedKeys
	}

	// Unmarshal items to destination slice
	return q.unmarshalItems(allItems, dest, metadata)
}

// BatchCreate creates multiple items
func (q *query) BatchCreate(items any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Validate items is a slice
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() == reflect.Ptr {
		itemsValue = itemsValue.Elem()
	}
	if itemsValue.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}

	// Process items in batches of 25 (DynamoDB limit)
	const batchSize = 25
	totalItems := itemsValue.Len()

	for i := 0; i < totalItems; i += batchSize {
		end := i + batchSize
		if end > totalItems {
			end = totalItems
		}

		// Build batch write request
		writeRequests := make([]types.WriteRequest, 0, end-i)

		for j := i; j < end; j++ {
			itemValue := itemsValue.Index(j)
			if itemValue.Kind() == reflect.Ptr {
				itemValue = itemValue.Elem()
			}

			// Marshal item
			item, err := q.marshalItem(itemValue.Interface(), metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal item %d: %w", j, err)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: item,
				},
			})
		}

		// Build BatchWriteItem input
		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				metadata.TableName: writeRequests,
			},
		}

		// Execute batch write with retries for unprocessed items
		client, err := q.db.session.Client()
		if err != nil {
			return fmt.Errorf("failed to get client for batch create: %w", err)
		}

		for {
			output, err := client.BatchWriteItem(q.ctx, input)
			if err != nil {
				return fmt.Errorf("failed to batch create items: %w", err)
			}

			// Check for unprocessed items
			if len(output.UnprocessedItems) == 0 {
				break
			}

			// Check if context is cancelled before retrying
			select {
			case <-q.ctx.Done():
				return fmt.Errorf("context cancelled during batch create retry: %w", q.ctx.Err())
			default:
				// Continue with retry
			}

			// Retry unprocessed items
			input.RequestItems = output.UnprocessedItems
		}
	}

	return nil
}

// BatchDelete deletes multiple items by their primary keys
func (q *query) BatchDelete(keys []any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get model metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Process keys in batches of 25 (DynamoDB limit)
	const batchSize = 25

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		// Build batch write request
		writeRequests := make([]types.WriteRequest, 0, end-i)

		for j := i; j < end; j++ {
			key := keys[j]
			keyMap := make(map[string]types.AttributeValue)

			// Handle different key formats
			switch k := key.(type) {
			case map[string]any:
				// Key is a map with pk and optional sk
				if pk, hasPK := k["pk"]; hasPK {
					av, err := q.db.converter.ToAttributeValue(pk)
					if err != nil {
						return fmt.Errorf("failed to convert partition key: %w", err)
					}
					keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
				}
				if sk, hasSK := k["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
					av, err := q.db.converter.ToAttributeValue(sk)
					if err != nil {
						return fmt.Errorf("failed to convert sort key: %w", err)
					}
					keyMap[metadata.PrimaryKey.SortKey.DBName] = av
				}
			default:
				// Check if key is a struct with the same type as our model
				keyValue := reflect.ValueOf(key)
				if keyValue.Kind() == reflect.Ptr {
					keyValue = keyValue.Elem()
				}

				if keyValue.Kind() == reflect.Struct {
					// Extract primary key fields from struct
					for _, field := range metadata.Fields {
						if field.IsPK {
							fieldValue := keyValue.FieldByIndex(field.IndexPath)
							av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
							if err != nil {
								return fmt.Errorf("failed to convert partition key: %w", err)
							}
							keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
						} else if field.IsSK && metadata.PrimaryKey.SortKey != nil {
							fieldValue := keyValue.FieldByIndex(field.IndexPath)
							av, err := q.db.converter.ToAttributeValue(fieldValue.Interface())
							if err != nil {
								return fmt.Errorf("failed to convert sort key: %w", err)
							}
							keyMap[metadata.PrimaryKey.SortKey.DBName] = av
						}
					}
				} else {
					// Key is just the partition key value
					av, err := q.db.converter.ToAttributeValue(key)
					if err != nil {
						return fmt.Errorf("failed to convert partition key: %w", err)
					}
					keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
				}
			}

			// Validate that we have at least a partition key
			if len(keyMap) == 0 {
				return fmt.Errorf("invalid key at index %d: missing partition key", j)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: keyMap,
				},
			})
		}

		// Build BatchWriteItem input
		input := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				metadata.TableName: writeRequests,
			},
		}

		// Execute batch write with retries for unprocessed items
		client, err := q.db.session.Client()
		if err != nil {
			return fmt.Errorf("failed to get client for batch delete: %w", err)
		}

		for {
			output, err := client.BatchWriteItem(q.ctx, input)
			if err != nil {
				return fmt.Errorf("failed to batch delete items: %w", err)
			}

			// Check for unprocessed items
			if len(output.UnprocessedItems) == 0 {
				break
			}

			// Retry unprocessed items
			input.RequestItems = output.UnprocessedItems
		}
	}

	return nil
}

// WithContext sets the context for the query
func (q *query) WithContext(ctx context.Context) core.Query {
	q.ctx = ctx
	return q
}

// Helper methods for basic CRUD operations

// lookupField provides consistent field lookup by checking both Go field names and DynamoDB attribute names
func lookupField(metadata *model.Metadata, fieldName string) (*model.FieldMetadata, bool) {
	// First check by Go field name
	if field, exists := metadata.Fields[fieldName]; exists {
		return field, true
	}

	// Then check by DynamoDB attribute name
	if field, exists := metadata.FieldsByDBName[fieldName]; exists {
		return field, true
	}

	return nil, false
}

func (q *query) extractPrimaryKey(metadata *model.Metadata) map[string]any {
	pk := make(map[string]any)

	// First try to extract from conditions
	for _, cond := range q.conditions {
		if cond.op != "=" {
			continue
		}

		// Check field name using enhanced lookup
		if field, exists := lookupField(metadata, cond.field); exists {
			if field.IsPK {
				pk["pk"] = cond.value
			} else if field.IsSK {
				pk["sk"] = cond.value
			}
		}
	}

	// If no primary key found in conditions, try to extract from model
	if _, hasPK := pk["pk"]; !hasPK && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}

		// Extract primary key from model
		if metadata.PrimaryKey.PartitionKey != nil {
			pkField := modelValue.FieldByIndex(metadata.PrimaryKey.PartitionKey.IndexPath)
			if !pkField.IsZero() {
				pk["pk"] = pkField.Interface()
			}
		}

		// Extract sort key from model if exists
		if metadata.PrimaryKey.SortKey != nil {
			skField := modelValue.FieldByIndex(metadata.PrimaryKey.SortKey.IndexPath)
			if !skField.IsZero() {
				pk["sk"] = skField.Interface()
			}
		}
	}

	// Must have at least partition key
	if _, hasPK := pk["pk"]; !hasPK {
		return nil
	}

	return pk
}

func (q *query) getItem(metadata *model.Metadata, pk map[string]any, dest any) error {
	// Build GetItem input
	tableName := metadata.TableName

	// Convert primary key to DynamoDB attribute values
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build GetItem input
	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       keyMap,
	}

	// Add projection expression if fields are specified
	if len(q.fields) > 0 {
		builder := expr.NewBuilder()
		builder.AddProjection(q.fields...)
		components := builder.Build()

		if components.ProjectionExpression != "" {
			input.ProjectionExpression = aws.String(components.ProjectionExpression)
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
	}

	// Set consistent read
	if q.consistentRead {
		input.ConsistentRead = aws.Bool(true)
	}

	// Execute GetItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for get item: %w", err)
	}

	output, err := client.GetItem(q.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	// Check if item was found
	if output.Item == nil {
		return customerrors.ErrItemNotFound
	}

	// Unmarshal item to destination
	return q.unmarshalItem(output.Item, dest, metadata)
}

// getItemDirect performs a direct GetItem without expression builder overhead
func (q *query) getItemDirect(metadata *model.Metadata, pk map[string]any, dest any) error {
	// Pre-allocate with exact size
	keyMap := make(map[string]types.AttributeValue, 2)

	// Direct conversion without error handling in hot path
	if pkValue, hasPK := pk["pk"]; hasPK {
		if av, err := q.db.converter.ToAttributeValue(pkValue); err == nil {
			keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
		} else {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
	}

	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		if av, err := q.db.converter.ToAttributeValue(skValue); err == nil {
			keyMap[metadata.PrimaryKey.SortKey.DBName] = av
		} else {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
	}

	// Direct API call
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for direct get item: %w", err)
	}

	input := &dynamodb.GetItemInput{
		TableName: aws.String(metadata.TableName),
		Key:       keyMap,
	}

	// Set consistent read
	if q.consistentRead {
		input.ConsistentRead = aws.Bool(true)
	}

	output, err := client.GetItem(q.ctx, input)

	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}

	if output.Item == nil {
		return customerrors.ErrItemNotFound
	}

	return q.unmarshalItem(output.Item, dest, metadata)
}

// unmarshalItem converts DynamoDB item to Go struct
func (q *query) unmarshalItem(item map[string]types.AttributeValue, dest any, metadata *model.Metadata) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}
	destValue = destValue.Elem()

	// Handle map destination (e.g., when ExecuteWithResult is used with a map)
	if destValue.Kind() == reflect.Map {
		// If it's a map, just convert each attribute value
		if destValue.IsNil() {
			destValue.Set(reflect.MakeMap(destValue.Type()))
		}

		for attrName, attrValue := range item {
			// Convert the attribute value to the appropriate Go type
			var val any
			if err := q.db.converter.FromAttributeValue(attrValue, &val); err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", attrName, err)
			}

			// Set the value in the map
			destValue.SetMapIndex(reflect.ValueOf(attrName), reflect.ValueOf(val))
		}

		return nil
	}

	// Handle struct destination (original behavior)
	if destValue.Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct or map")
	}

	// Iterate through the item attributes
	for attrName, attrValue := range item {
		// Find the corresponding field in metadata
		field, exists := metadata.FieldsByDBName[attrName]
		if !exists {
			continue // Skip unknown fields
		}

		// Get the struct field
		structField := destValue.FieldByIndex(field.IndexPath)
		if !structField.CanSet() {
			continue
		}

		// Convert and set the value
		if err := q.db.converter.FromAttributeValue(attrValue, structField.Addr().Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
		}
	}

	return nil
}

func (q *query) putItem(metadata *model.Metadata) error {
	// Marshal the model to DynamoDB item
	item, err := q.marshalItem(q.model, metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem input
	input := &dynamodb.PutItemInput{
		TableName: aws.String(metadata.TableName),
		Item:      item,
	}

	// Add condition to ensure item doesn't already exist
	pkField := metadata.PrimaryKey.PartitionKey
	builder := expr.NewBuilder()
	builder.AddConditionExpression(pkField.DBName, "NOT_EXISTS", nil)

	if metadata.PrimaryKey.SortKey != nil {
		skField := metadata.PrimaryKey.SortKey
		builder.AddConditionExpression(skField.DBName, "NOT_EXISTS", nil)
	}

	components := builder.Build()
	if components.ConditionExpression != "" {
		input.ConditionExpression = aws.String(components.ConditionExpression)
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}

	// Execute PutItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for put item: %w", err)
	}

	_, err = client.PutItem(q.ctx, input)
	if err != nil {
		// Check if it's a conditional check failure
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to put item: %w", err)
	}

	// Update timestamp fields in the original model
	q.updateTimestampsInModel(metadata)

	return nil
}

// updateTimestampsInModel updates the created_at and updated_at fields in the original model
func (q *query) updateTimestampsInModel(metadata *model.Metadata) {
	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	now := time.Now()

	// Update timestamp fields
	for _, fieldMeta := range metadata.Fields {
		if fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
			field := modelValue.FieldByIndex(fieldMeta.IndexPath)
			if field.CanSet() && field.Type() == reflect.TypeOf(time.Time{}) {
				field.Set(reflect.ValueOf(now))
			}
		}
	}
}

// marshalItem converts a Go struct to DynamoDB item
func (q *query) marshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	// Use optimized marshaler if available
	if q.db.marshaler != nil {
		return q.db.marshaler.MarshalItem(model, metadata)
	}

	// Fall back to reflection-based marshaling
	item := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	// Process each field
	for fieldName, fieldMeta := range metadata.Fields {
		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)

		// Skip zero values if omitempty
		if fieldMeta.OmitEmpty && fieldValue.IsZero() {
			continue
		}

		// Handle special fields
		if fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
			// Set to current time
			now := time.Now()
			fieldValue = reflect.ValueOf(now)
		} else if fieldMeta.IsVersion {
			// Initialize version to 0 for new items
			if fieldValue.IsZero() {
				fieldValue = reflect.ValueOf(int64(0))
			}
		} else if fieldMeta.IsTTL {
			// Convert TTL to Unix timestamp if it's a time.Time
			if fieldValue.Type().String() == "time.Time" && !fieldValue.IsZero() {
				ttlTime := fieldValue.Interface().(time.Time)
				fieldValue = reflect.ValueOf(ttlTime.Unix())
			}
		}

		// Convert to AttributeValue
		var av types.AttributeValue
		var err error

		if fieldMeta.IsSet {
			av, err = q.db.converter.ConvertToSet(fieldValue.Interface(), true)
		} else {
			av, err = q.db.converter.ToAttributeValue(fieldValue.Interface())
		}

		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldName, err)
		}

		// Skip NULL values unless explicitly included
		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fieldMeta.OmitEmpty {
			continue
		}

		item[fieldMeta.DBName] = av
	}

	return item, nil
}

// isConditionalCheckFailedException checks if the error is a conditional check failure
func isConditionalCheckFailedException(err error) bool {
	var ccfe *types.ConditionalCheckFailedException
	return errors.As(err, &ccfe)
}

// executeQuery performs a DynamoDB Query operation
func (q *query) executeQuery(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()

	// Add key conditions
	for _, cond := range keyConditions {
		fieldMeta, _ := lookupField(metadata, cond.field)
		builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
	}

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
	}

	// Add projection
	if len(q.fields) > 0 {
		builder.AddProjection(q.fields...)
	}

	// Build expressions
	components := builder.Build()

	// Build Query input
	input := &dynamodb.QueryInput{
		TableName: aws.String(metadata.TableName),
	}

	if components.KeyConditionExpression != "" {
		input.KeyConditionExpression = aws.String(components.KeyConditionExpression)
	}
	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if components.ProjectionExpression != "" {
		input.ProjectionExpression = aws.String(components.ProjectionExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Set scan direction
	if q.orderBy != nil && q.orderBy.order == "DESC" {
		input.ScanIndexForward = aws.Bool(false)
	}

	// Set limit
	if q.limit != nil {
		input.Limit = aws.Int32(int32(*q.limit))
	}

	// Set consistent read (only works for main table queries, not GSI)
	if q.consistentRead && q.indexName == "" {
		input.ConsistentRead = aws.Bool(true)
	}

	// Execute query and collect results
	var items []map[string]types.AttributeValue

	client, err := q.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for query: %w", err)
	}

	paginator := dynamodb.NewQueryPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to query items: %w", err)
		}

		items = append(items, output.Items...)

		// Stop if we have enough items
		if q.limit != nil && len(items) >= *q.limit {
			items = items[:*q.limit]
			break
		}
	}

	return items, nil
}

// executeScan performs a DynamoDB Scan operation
func (q *query) executeScan(metadata *model.Metadata, filterConditions []condition) ([]map[string]types.AttributeValue, error) {
	// Use the existing builder from the query to preserve Filter() conditions
	builder := q.builder
	if builder == nil {
		builder = expr.NewBuilder()
	}

	// Add filter conditions from parameters (these come from Where() conditions)
	for _, cond := range filterConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
	}

	// Add projection
	if len(q.fields) > 0 {
		builder.AddProjection(q.fields...)
	}

	// Build expressions
	components := builder.Build()

	// Build Scan input
	input := &dynamodb.ScanInput{
		TableName: aws.String(metadata.TableName),
	}

	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if components.ProjectionExpression != "" {
		input.ProjectionExpression = aws.String(components.ProjectionExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Set limit
	if q.limit != nil {
		input.Limit = aws.Int32(int32(*q.limit))
	}

	// Set consistent read (only works for main table scans, not GSI)
	if q.consistentRead && q.indexName == "" {
		input.ConsistentRead = aws.Bool(true)
	}

	// Execute scan and collect results
	var items []map[string]types.AttributeValue

	client, err := q.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for scan: %w", err)
	}

	paginator := dynamodb.NewScanPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to scan items: %w", err)
		}

		items = append(items, output.Items...)

		// Stop if we have enough items
		if q.limit != nil && len(items) >= *q.limit {
			items = items[:*q.limit]
			break
		}
	}

	return items, nil
}

// isIndexKey checks if a field is a key in the specified index
func (q *query) isIndexKey(fieldMeta *model.FieldMetadata, indexName string, metadata *model.Metadata) bool {
	for _, index := range metadata.Indexes {
		if index.Name == indexName {
			return (index.PartitionKey != nil && index.PartitionKey.Name == fieldMeta.Name) ||
				(index.SortKey != nil && index.SortKey.Name == fieldMeta.Name)
		}
	}
	return false
}

// unmarshalItems converts DynamoDB items to Go slice
func (q *query) unmarshalItems(items []map[string]types.AttributeValue, dest any, metadata *model.Metadata) error {
	destValue := reflect.ValueOf(dest).Elem()
	elemType := destValue.Type().Elem()

	// Create a new slice with the correct length
	newSlice := reflect.MakeSlice(destValue.Type(), len(items), len(items))

	// Unmarshal each item
	for i, item := range items {
		var elem reflect.Value

		// Check if element type is already a pointer
		if elemType.Kind() == reflect.Ptr {
			// For []*Type, create a new instance of Type (not *Type)
			elem = reflect.New(elemType.Elem())
		} else {
			// For []Type, create a new instance of *Type
			elem = reflect.New(elemType)
		}

		if err := q.unmarshalItem(item, elem.Interface(), metadata); err != nil {
			return fmt.Errorf("failed to unmarshal item %d: %w", i, err)
		}

		// Set the element in the slice
		if elemType.Kind() == reflect.Ptr {
			// For []*Type, elem is already a pointer, just set it
			newSlice.Index(i).Set(elem)
		} else {
			// For []Type, dereference the pointer
			newSlice.Index(i).Set(elem.Elem())
		}
	}

	// Set the destination slice
	destValue.Set(newSlice)
	return nil
}

// executeQueryCount performs a DynamoDB Query operation to count items
func (q *query) executeQueryCount(metadata *model.Metadata, keyConditions []condition, filterConditions []condition) (int64, error) {
	builder := expr.NewBuilder()

	// Add key conditions
	for _, cond := range keyConditions {
		fieldMeta, _ := lookupField(metadata, cond.field)
		builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
	}

	// Add filter conditions
	for _, cond := range filterConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
	}

	// Build expressions
	components := builder.Build()

	// Build Query input with Select = COUNT
	input := &dynamodb.QueryInput{
		TableName: aws.String(metadata.TableName),
		Select:    types.SelectCount,
	}

	if components.KeyConditionExpression != "" {
		input.KeyConditionExpression = aws.String(components.KeyConditionExpression)
	}
	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Execute query and count results
	var totalCount int64

	client, err := q.db.session.Client()
	if err != nil {
		return 0, fmt.Errorf("failed to get client for query count: %w", err)
	}

	paginator := dynamodb.NewQueryPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to count items: %w", err)
		}

		totalCount += int64(output.Count)
	}

	return totalCount, nil
}

// executeScanCount performs a DynamoDB Scan operation to count items
func (q *query) executeScanCount(metadata *model.Metadata, filterConditions []condition) (int64, error) {
	// Use the existing builder from the query to preserve Filter() conditions
	builder := q.builder
	if builder == nil {
		builder = expr.NewBuilder()
	}

	// Add filter conditions from parameters (these come from Where() conditions)
	for _, cond := range filterConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
	}

	// Build expressions
	components := builder.Build()

	// Build Scan input with Select = COUNT
	input := &dynamodb.ScanInput{
		TableName: aws.String(metadata.TableName),
		Select:    types.SelectCount,
	}

	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Execute scan and count results
	var totalCount int64

	client, err := q.db.session.Client()
	if err != nil {
		return 0, fmt.Errorf("failed to get client for scan count: %w", err)
	}

	paginator := dynamodb.NewScanPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to count items: %w", err)
		}

		totalCount += int64(output.Count)
	}

	return totalCount, nil
}

func (q *query) updateItem(metadata *model.Metadata, fields []string) error {
	// Extract primary key from conditions or model
	pk := q.extractPrimaryKey(metadata)
	if pk == nil {
		return fmt.Errorf("update requires primary key")
	}

	// Build key map
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build update expression
	builder := expr.NewBuilder()

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}

	// Determine which fields to update
	fieldsToUpdate := fields
	if len(fieldsToUpdate) == 0 {
		// If no fields specified, update all non-zero fields except primary keys and special fields
		fieldsToUpdate = []string{}
		for fieldName, fieldMeta := range metadata.Fields {
			if fieldMeta.IsPK || fieldMeta.IsSK || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt {
				continue
			}
			fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
			if !fieldValue.IsZero() || !fieldMeta.OmitEmpty {
				fieldsToUpdate = append(fieldsToUpdate, fieldName)
			}
		}
	}

	// Build SET expressions
	for _, fieldName := range fieldsToUpdate {
		fieldMeta, exists := lookupField(metadata, fieldName)
		if !exists {
			continue
		}

		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)

		// Handle special fields
		if fieldMeta.IsUpdatedAt {
			// Always update to current time
			builder.AddUpdateSet(fieldMeta.DBName, time.Now())
		} else if fieldMeta.IsVersion {
			// Increment version
			builder.AddUpdateAdd(fieldMeta.DBName, int64(1))
		} else {
			// Regular field update
			value := fieldValue.Interface()
			builder.AddUpdateSet(fieldMeta.DBName, value)
		}
	}

	// Always update updated_at if it exists
	if metadata.UpdatedAtField != nil {
		builder.AddUpdateSet(metadata.UpdatedAtField.DBName, time.Now())
	}

	// Add version check if version field exists
	if metadata.VersionField != nil {
		currentVersion := modelValue.FieldByIndex(metadata.VersionField.IndexPath).Int()
		builder.AddConditionExpression(metadata.VersionField.DBName, "=", currentVersion)
	}

	// Build the update expression
	components := builder.Build()

	// Build UpdateItem input
	input := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(metadata.TableName),
		Key:                       keyMap,
		UpdateExpression:          aws.String(components.UpdateExpression),
		ExpressionAttributeNames:  components.ExpressionAttributeNames,
		ExpressionAttributeValues: components.ExpressionAttributeValues,
	}

	if components.ConditionExpression != "" {
		input.ConditionExpression = aws.String(components.ConditionExpression)
	}

	// Execute UpdateItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	_, err = client.UpdateItem(q.ctx, input)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

func (q *query) deleteItem(metadata *model.Metadata) error {
	// Extract primary key from conditions
	pk := q.extractPrimaryKey(metadata)
	if pk == nil {
		return fmt.Errorf("delete requires primary key in conditions")
	}

	// Build key map
	keyMap := make(map[string]types.AttributeValue)

	// Add partition key
	if pkValue, hasPK := pk["pk"]; hasPK {
		av, err := q.db.converter.ToAttributeValue(pkValue)
		if err != nil {
			return fmt.Errorf("failed to convert partition key: %w", err)
		}
		keyMap[metadata.PrimaryKey.PartitionKey.DBName] = av
	}

	// Add sort key if present
	if skValue, hasSK := pk["sk"]; hasSK && metadata.PrimaryKey.SortKey != nil {
		av, err := q.db.converter.ToAttributeValue(skValue)
		if err != nil {
			return fmt.Errorf("failed to convert sort key: %w", err)
		}
		keyMap[metadata.PrimaryKey.SortKey.DBName] = av
	}

	// Build DeleteItem input
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(metadata.TableName),
		Key:       keyMap,
	}

	// Add condition expression if we have additional conditions
	builder := expr.NewBuilder()
	hasConditions := false

	// Check for version field condition
	if metadata.VersionField != nil && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr {
			modelValue = modelValue.Elem()
		}
		versionValue := modelValue.FieldByIndex(metadata.VersionField.IndexPath)
		if !versionValue.IsZero() {
			builder.AddConditionExpression(metadata.VersionField.DBName, "=", versionValue.Int())
			hasConditions = true
		}
	}

	// Add any other conditions from the query
	for _, cond := range q.conditions {
		// Skip primary key conditions as they're already in the key
		if fieldMeta, exists := lookupField(metadata, cond.field); exists && (fieldMeta.IsPK || fieldMeta.IsSK) {
			continue
		}

		builder.AddConditionExpression(cond.field, cond.op, cond.value)
		hasConditions = true
	}

	if hasConditions {
		components := builder.Build()
		if components.ConditionExpression != "" {
			input.ConditionExpression = aws.String(components.ConditionExpression)
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
			input.ExpressionAttributeValues = components.ExpressionAttributeValues
		}
	}

	// Execute DeleteItem
	client, err := q.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for delete item: %w", err)
	}

	_, err = client.DeleteItem(q.ctx, input)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

// errorQuery is a query that always returns an error
type errorQuery struct {
	err error
}

func (e *errorQuery) Where(field string, op string, value any) core.Query  { return e }
func (e *errorQuery) Index(indexName string) core.Query                    { return e }
func (e *errorQuery) Filter(field string, op string, value any) core.Query { return e }
func (e *errorQuery) OrFilter(field string, op string, value any) core.Query {
	return e
}
func (e *errorQuery) FilterGroup(fn func(q core.Query)) core.Query { return e }
func (e *errorQuery) OrFilterGroup(fn func(core.Query)) core.Query {
	return e
}
func (e *errorQuery) OrderBy(field string, order string) core.Query                   { return e }
func (e *errorQuery) Limit(limit int) core.Query                                      { return e }
func (e *errorQuery) Offset(offset int) core.Query                                    { return e }
func (e *errorQuery) Select(fields ...string) core.Query                              { return e }
func (e *errorQuery) ConsistentRead() core.Query                                      { return e }
func (e *errorQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query { return e }
func (e *errorQuery) First(dest any) error                                            { return e.err }
func (e *errorQuery) All(dest any) error                                              { return e.err }
func (e *errorQuery) Count() (int64, error)                                           { return 0, e.err }
func (e *errorQuery) Create() error                                                   { return e.err }
func (e *errorQuery) CreateOrUpdate() error                                           { return e.err }
func (e *errorQuery) Update(fields ...string) error                                   { return e.err }
func (e *errorQuery) Delete() error                                                   { return e.err }
func (e *errorQuery) Scan(dest any) error                                             { return e.err }
func (e *errorQuery) BatchGet(keys []any, dest any) error                             { return e.err }
func (e *errorQuery) BatchCreate(items any) error                                     { return e.err }
func (e *errorQuery) BatchDelete(keys []any) error                                    { return e.err }
func (e *errorQuery) BatchWrite(putItems []any, deleteKeys []any) error               { return e.err }
func (e *errorQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	return e.err
}
func (e *errorQuery) WithContext(ctx context.Context) core.Query                 { return e }
func (e *errorQuery) AllPaginated(dest any) (*core.PaginatedResult, error)       { return nil, e.err }
func (e *errorQuery) UpdateBuilder() core.UpdateBuilder                          { return nil }
func (e *errorQuery) ParallelScan(segment int32, totalSegments int32) core.Query { return e }
func (e *errorQuery) ScanAllSegments(dest any, totalSegments int32) error        { return e.err }
func (e *errorQuery) Cursor(cursor string) core.Query                            { return e }
func (e *errorQuery) SetCursor(cursor string) error                              { return e.err }

// Re-export types for convenience
type (
	Config            = session.Config
	AutoMigrateOption = schema.AutoMigrateOption
)

// Re-export AutoMigrate options for convenience
var (
	WithBackupTable = schema.WithBackupTable
	WithDataCopy    = schema.WithDataCopy
	WithTargetModel = schema.WithTargetModel
	WithTransform   = schema.WithTransform
	WithBatchSize   = schema.WithBatchSize
)

// TransactionFunc executes a function within a database transaction
// This is the actual implementation that uses our sophisticated transaction support
func (db *DB) TransactionFunc(fn func(tx any) error) error {
	// Create a new transaction
	tx := transaction.NewTransaction(db.session, db.registry, db.converter)
	tx = tx.WithContext(db.ctx)

	// Execute the transaction function
	if err := fn(tx); err != nil {
		// Rollback on error
		_ = tx.Rollback()
		return err
	}

	// Commit the transaction
	return tx.Commit()
}

// AllPaginated retrieves all matching items with pagination metadata
func (q *query) AllPaginated(dest any) (*core.PaginatedResult, error) {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return nil, err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return nil, err
	}

	// Separate key conditions from filter conditions
	var keyConditions []condition
	var filterConditions []condition

	for _, cond := range q.conditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if !exists {
			return nil, fmt.Errorf("field %s not found in model", cond.field)
		}

		isKey := fieldMeta.IsPK || fieldMeta.IsSK ||
			(q.indexName != "" && q.isIndexKey(fieldMeta, q.indexName, metadata))

		if isKey {
			keyConditions = append(keyConditions, cond)
		} else {
			filterConditions = append(filterConditions, cond)
		}
	}

	// Determine operation type
	var items []map[string]types.AttributeValue
	var scannedCount int
	var lastEvaluatedKey map[string]types.AttributeValue

	if len(keyConditions) > 0 {
		// Use Query operation
		builder := expr.NewBuilder()

		// Add key conditions
		for _, cond := range keyConditions {
			fieldMeta, _ := lookupField(metadata, cond.field)
			builder.AddKeyCondition(fieldMeta.DBName, cond.op, cond.value)
		}

		// Add filter conditions
		for _, cond := range filterConditions {
			fieldMeta, exists := lookupField(metadata, cond.field)
			if exists {
				builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
			} else {
				builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
			}
		}

		// Add projection
		if len(q.fields) > 0 {
			builder.AddProjection(q.fields...)
		}

		// Build expressions
		components := builder.Build()

		// Build Query input
		input := &dynamodb.QueryInput{
			TableName: aws.String(metadata.TableName),
		}

		if components.KeyConditionExpression != "" {
			input.KeyConditionExpression = aws.String(components.KeyConditionExpression)
		}
		if components.FilterExpression != "" {
			input.FilterExpression = aws.String(components.FilterExpression)
		}
		if components.ProjectionExpression != "" {
			input.ProjectionExpression = aws.String(components.ProjectionExpression)
		}
		if len(components.ExpressionAttributeNames) > 0 {
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
		if len(components.ExpressionAttributeValues) > 0 {
			input.ExpressionAttributeValues = components.ExpressionAttributeValues
		}

		// Set index name if specified
		if q.indexName != "" {
			input.IndexName = aws.String(q.indexName)
		}

		// Set scan direction
		if q.orderBy != nil && q.orderBy.order == "DESC" {
			input.ScanIndexForward = aws.Bool(false)
		}

		// Set limit
		if q.limit != nil {
			input.Limit = aws.Int32(int32(*q.limit))
		}

		// Execute query
		client, err := q.db.session.Client()
		if err != nil {
			return nil, fmt.Errorf("failed to get client for paginated query: %w", err)
		}

		output, err := client.Query(q.ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to query items: %w", err)
		}

		items = output.Items
		scannedCount = int(output.ScannedCount)
		lastEvaluatedKey = output.LastEvaluatedKey
	} else {
		// Use Scan operation
		builder := expr.NewBuilder()

		// Add filter conditions
		for _, cond := range filterConditions {
			fieldMeta, exists := lookupField(metadata, cond.field)
			if exists {
				builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
			} else {
				builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
			}
		}

		// Add projection
		if len(q.fields) > 0 {
			builder.AddProjection(q.fields...)
		}

		// Build expressions
		components := builder.Build()

		// Build Scan input
		input := &dynamodb.ScanInput{
			TableName: aws.String(metadata.TableName),
		}

		if components.FilterExpression != "" {
			input.FilterExpression = aws.String(components.FilterExpression)
		}
		if components.ProjectionExpression != "" {
			input.ProjectionExpression = aws.String(components.ProjectionExpression)
		}
		if len(components.ExpressionAttributeNames) > 0 {
			input.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
		if len(components.ExpressionAttributeValues) > 0 {
			input.ExpressionAttributeValues = components.ExpressionAttributeValues
		}

		// Set index name if specified
		if q.indexName != "" {
			input.IndexName = aws.String(q.indexName)
		}

		// Set limit
		if q.limit != nil {
			input.Limit = aws.Int32(int32(*q.limit))
		}

		// Execute scan
		client, err := q.db.session.Client()
		if err != nil {
			return nil, fmt.Errorf("failed to get client for paginated scan: %w", err)
		}

		output, err := client.Scan(q.ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to scan items: %w", err)
		}

		items = output.Items
		scannedCount = int(output.ScannedCount)
		lastEvaluatedKey = output.LastEvaluatedKey
	}

	// Unmarshal items
	if len(items) > 0 {
		if err := q.unmarshalItems(items, dest, metadata); err != nil {
			return nil, err
		}
	}

	// Create result
	result := &core.PaginatedResult{
		Items:            dest,
		Count:            len(items),
		ScannedCount:     scannedCount,
		LastEvaluatedKey: lastEvaluatedKey,
		HasMore:          len(lastEvaluatedKey) > 0,
	}

	// Generate next cursor if there are more results
	if result.HasMore {
		cursor, err := queryPkg.EncodeCursor(lastEvaluatedKey, q.indexName, "")
		if err != nil {
			return nil, fmt.Errorf("failed to encode cursor: %w", err)
		}
		result.NextCursor = cursor
	}

	return result, nil
}

// UpdateBuilder returns a builder for complex update operations
func (q *query) UpdateBuilder() core.UpdateBuilder {
	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		// Return an error-producing UpdateBuilder
		return &errorUpdateBuilder{err: err}
	}

	// Create Query struct for pkg/query using the proper constructor
	adapter := &metadataAdapter{metadata: metadata}
	executor := &queryExecutor{db: q.db}
	conditions := convertConditions(q.conditions)

	pkgQuery := queryPkg.NewWithConditions(q.model, adapter, executor, conditions, q.ctx)

	return queryPkg.NewUpdateBuilder(pkgQuery)
}

// errorUpdateBuilder is a simple error-returning UpdateBuilder
type errorUpdateBuilder struct {
	err error
}

func (e *errorUpdateBuilder) Set(field string, value any) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) Add(field string, value any) core.UpdateBuilder              { return e }
func (e *errorUpdateBuilder) Increment(field string) core.UpdateBuilder                   { return e }
func (e *errorUpdateBuilder) Decrement(field string) core.UpdateBuilder                   { return e }
func (e *errorUpdateBuilder) Remove(field string) core.UpdateBuilder                      { return e }
func (e *errorUpdateBuilder) Delete(field string, value any) core.UpdateBuilder           { return e }
func (e *errorUpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder    { return e }
func (e *errorUpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder   { return e }
func (e *errorUpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) ConditionExists(field string) core.UpdateBuilder          { return e }
func (e *errorUpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder       { return e }
func (e *errorUpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) ReturnValues(option string) core.UpdateBuilder            { return e }
func (e *errorUpdateBuilder) Execute() error                                           { return e.err }
func (e *errorUpdateBuilder) ExecuteWithResult(result any) error                       { return e.err }

// ParallelScan configures parallel scanning with segment and total segments
func (q *query) ParallelScan(segment int32, totalSegments int32) core.Query {
	q.segment = &segment
	q.totalSegments = &totalSegments
	return q
}

// ScanAllSegments performs parallel scan across all segments automatically
func (q *query) ScanAllSegments(dest any, totalSegments int32) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Get metadata
	metadata, err := q.db.registry.GetMetadata(q.model)
	if err != nil {
		return err
	}

	// Ensure dest is a pointer to a slice
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("dest must be a pointer to a slice")
	}

	// Create a channel for results
	type segmentResult struct {
		items []map[string]types.AttributeValue
		err   error
	}
	resultsChan := make(chan segmentResult, totalSegments)

	// Launch parallel scans
	var wg sync.WaitGroup
	for i := int32(0); i < totalSegments; i++ {
		wg.Add(1)
		go func(segment int32) {
			defer func() {
				if r := recover(); r != nil {
					// Log the panic with context
					err := fmt.Errorf("scan segment %d panicked: %v", segment, r)
					resultsChan <- segmentResult{nil, err}
				}
				wg.Done()
			}()

			// Clone the query for this segment
			segmentQuery := &query{
				ctx:           q.ctx,
				db:            q.db,
				model:         q.model,
				builder:       q.builder,
				conditions:    q.conditions,
				indexName:     q.indexName,
				orderBy:       q.orderBy,
				limit:         q.limit,
				offset:        q.offset,
				fields:        q.fields,
				segment:       &segment,
				totalSegments: &totalSegments,
			}

			// Execute scan for this segment using the cloned query
			items, err := segmentQuery.executeScanSegment(metadata, segment, totalSegments)
			if err != nil {
				resultsChan <- segmentResult{nil, fmt.Errorf("segment %d failed: %w", segment, err)}
				return
			}

			resultsChan <- segmentResult{items, nil}
		}(i)
	}

	// Wait for all segments to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var allItems []map[string]types.AttributeValue
	for result := range resultsChan {
		if result.err != nil {
			return result.err
		}
		allItems = append(allItems, result.items...)
	}

	// Unmarshal items
	if len(allItems) > 0 {
		if err := q.unmarshalItems(allItems, dest, metadata); err != nil {
			return err
		}
	}

	return nil
}

// executeScanSegment executes a scan for a specific segment
func (q *query) executeScanSegment(metadata *model.Metadata, segment, totalSegments int32) ([]map[string]types.AttributeValue, error) {
	builder := expr.NewBuilder()

	// Add filter conditions
	var filterConditions []condition
	for _, cond := range q.conditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if !exists || (!fieldMeta.IsPK && !fieldMeta.IsSK) {
			filterConditions = append(filterConditions, cond)
		}
	}

	for _, cond := range filterConditions {
		fieldMeta, exists := lookupField(metadata, cond.field)
		if exists {
			builder.AddFilterCondition("AND", fieldMeta.DBName, cond.op, cond.value)
		} else {
			builder.AddFilterCondition("AND", cond.field, cond.op, cond.value)
		}
	}

	// Add projection
	if len(q.fields) > 0 {
		builder.AddProjection(q.fields...)
	}

	// Build expressions
	components := builder.Build()

	// Build Scan input
	input := &dynamodb.ScanInput{
		TableName:     aws.String(metadata.TableName),
		Segment:       aws.Int32(segment),
		TotalSegments: aws.Int32(totalSegments),
	}

	if components.FilterExpression != "" {
		input.FilterExpression = aws.String(components.FilterExpression)
	}
	if components.ProjectionExpression != "" {
		input.ProjectionExpression = aws.String(components.ProjectionExpression)
	}
	if len(components.ExpressionAttributeNames) > 0 {
		input.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if len(components.ExpressionAttributeValues) > 0 {
		input.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Set index name if specified
	if q.indexName != "" {
		input.IndexName = aws.String(q.indexName)
	}

	// Set limit if specified (but be careful with parallel scans)
	if q.limit != nil && *q.limit > 0 {
		// Distribute limit across segments
		segmentLimit := (*q.limit + int(totalSegments) - 1) / int(totalSegments)
		input.Limit = aws.Int32(int32(segmentLimit))
	}

	// Execute scan and collect results
	var items []map[string]types.AttributeValue

	client, err := q.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for scan segment: %w", err)
	}

	paginator := dynamodb.NewScanPaginator(client, input)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(q.ctx)
		if err != nil {
			return nil, err
		}

		items = append(items, output.Items...)

		// If we have a limit and reached it, stop
		if q.limit != nil && len(items) >= *q.limit/int(totalSegments) {
			break
		}
	}

	return items, nil
}

// Cursor sets the pagination cursor for the query
func (q *query) Cursor(cursor string) core.Query {
	q.exclusiveStartKey = cursor
	return q
}

// SetCursor sets the cursor from a string (alternative to Cursor)
func (q *query) SetCursor(cursor string) error {
	q.exclusiveStartKey = cursor
	return nil
}

// BatchWrite performs mixed batch write operations (puts and deletes)
func (q *query) BatchWrite(putItems []any, deleteKeys []any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// Simply combine puts and deletes into batch operations
	// First, perform all puts using BatchCreate
	if len(putItems) > 0 {
		if err := q.BatchCreate(putItems); err != nil {
			return fmt.Errorf("failed to batch put items: %w", err)
		}
	}

	// Then, perform all deletes using BatchDelete
	if len(deleteKeys) > 0 {
		if err := q.BatchDelete(deleteKeys); err != nil {
			return fmt.Errorf("failed to batch delete items: %w", err)
		}
	}

	return nil
}

// BatchUpdateWithOptions performs batch update operations with custom options
func (q *query) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	// Check Lambda timeout
	if err := q.checkLambdaTimeout(); err != nil {
		return err
	}

	// For now, perform updates sequentially
	// In a full implementation, this would support parallel updates with options
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() == reflect.Ptr {
		itemsValue = itemsValue.Elem()
	}
	if itemsValue.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}

	// Get metadata for the model type
	if itemsValue.Len() == 0 {
		return nil // Nothing to update
	}

	firstItem := itemsValue.Index(0).Interface()
	metadata, err := q.db.registry.GetMetadata(firstItem)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	for i := 0; i < itemsValue.Len(); i++ {
		item := itemsValue.Index(i).Interface()

		// Create a new query with the model
		updateQuery := q.db.Model(item)

		// Extract and set primary key conditions
		itemValue := reflect.ValueOf(item)
		if itemValue.Kind() == reflect.Ptr {
			itemValue = itemValue.Elem()
		}

		// Add partition key condition
		if metadata.PrimaryKey.PartitionKey != nil {
			pkField := itemValue.FieldByIndex(metadata.PrimaryKey.PartitionKey.IndexPath)
			updateQuery = updateQuery.Where(metadata.PrimaryKey.PartitionKey.Name, "=", pkField.Interface())
		}

		// Add sort key condition if present
		if metadata.PrimaryKey.SortKey != nil {
			skField := itemValue.FieldByIndex(metadata.PrimaryKey.SortKey.IndexPath)
			updateQuery = updateQuery.Where(metadata.PrimaryKey.SortKey.Name, "=", skField.Interface())
		}

		// Perform the update
		if err := updateQuery.Update(fields...); err != nil {
			return fmt.Errorf("failed to update item %d: %w", i, err)
		}
	}

	return nil
}

// convertConditions converts dynamorm conditions to pkg/query conditions
func convertConditions(conditions []condition) []queryPkg.Condition {
	result := make([]queryPkg.Condition, len(conditions))
	for i, cond := range conditions {
		result[i] = queryPkg.Condition{
			Field:    cond.field,
			Operator: cond.op,
			Value:    cond.value,
		}
	}
	return result
}

// queryExecutor implements the executor interface for pkg/query
type queryExecutor struct {
	db *DB
}

// ExecuteQuery implements QueryExecutor interface
func (qe *queryExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	// For now, return not implemented
	return fmt.Errorf("ExecuteQuery not implemented")
}

// ExecuteScan implements QueryExecutor interface
func (qe *queryExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	// For now, return not implemented
	return fmt.Errorf("ExecuteScan not implemented")
}

// ExecuteUpdateItem implements UpdateItemExecutor interface
func (qe *queryExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	client, err := qe.db.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: input.ExpressionAttributeValues,
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}

	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}

	_, err = client.UpdateItem(context.Background(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// ExecuteUpdateItemWithResult implements UpdateItemWithResultExecutor interface
func (qe *queryExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	client, err := qe.db.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		UpdateExpression:          aws.String(input.UpdateExpression),
		ExpressionAttributeNames:  input.ExpressionAttributeNames,
		ExpressionAttributeValues: input.ExpressionAttributeValues,
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}

	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}

	output, err := client.UpdateItem(context.Background(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return nil, customerrors.ErrConditionFailed
		}
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	return &core.UpdateResult{
		Attributes: output.Attributes,
	}, nil
}

// metadataAdapter adapts *model.Metadata to core.ModelMetadata interface
type metadataAdapter struct {
	metadata *model.Metadata
}

func (ma *metadataAdapter) TableName() string {
	return ma.metadata.TableName
}

func (ma *metadataAdapter) PrimaryKey() core.KeySchema {
	if ma.metadata.PrimaryKey == nil {
		return core.KeySchema{}
	}

	schema := core.KeySchema{}
	if ma.metadata.PrimaryKey.PartitionKey != nil {
		schema.PartitionKey = ma.metadata.PrimaryKey.PartitionKey.Name
	}
	if ma.metadata.PrimaryKey.SortKey != nil {
		schema.SortKey = ma.metadata.PrimaryKey.SortKey.Name
	}
	return schema
}

func (ma *metadataAdapter) Indexes() []core.IndexSchema {
	indexes := make([]core.IndexSchema, len(ma.metadata.Indexes))
	for i, idx := range ma.metadata.Indexes {
		schema := core.IndexSchema{
			Name:            idx.Name,
			Type:            string(idx.Type),
			ProjectionType:  idx.ProjectionType,
			ProjectedFields: idx.ProjectedFields,
		}
		if idx.PartitionKey != nil {
			schema.PartitionKey = idx.PartitionKey.Name
		}
		if idx.SortKey != nil {
			schema.SortKey = idx.SortKey.Name
		}
		indexes[i] = schema
	}
	return indexes
}

func (ma *metadataAdapter) AttributeMetadata(field string) *core.AttributeMetadata {
	// First check by Go field name
	fieldMeta, exists := ma.metadata.Fields[field]
	if !exists {
		// Then check by DynamoDB attribute name
		fieldMeta, exists = ma.metadata.FieldsByDBName[field]
		if !exists {
			return nil
		}
	}

	return &core.AttributeMetadata{
		Name:         fieldMeta.Name,
		Type:         fieldMeta.Type.String(),
		DynamoDBName: fieldMeta.DBName,
		Tags:         fieldMeta.Tags,
	}
}
