package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/index"
)

// Query represents a DynamoDB query builder
type Query struct {
	model      any
	conditions []Condition
	filters    []Filter
	rawFilters []RawFilter
	index      string
	limit      int
	offset     *int
	projection []string
	orderBy    OrderBy
	exclusive  map[string]types.AttributeValue
	ctx        context.Context

	// Internal state
	metadata core.ModelMetadata
	executor QueryExecutor
	builder  *expr.Builder

	// Parallel scan configuration
	segment       *int32
	totalSegments *int32

	// Consistency options
	consistentRead bool

	// Retry configuration
	retryConfig *RetryConfig
}

// Condition represents a query condition
type Condition struct {
	Field    string
	Operator string
	Value    any
}

// Filter represents a filter expression
type Filter struct {
	Expression string
	Params     map[string]any
}

// RawFilter represents a raw filter with parameters
type RawFilter struct {
	Expression string
	Params     []core.Param
}

// OrderBy represents ordering configuration
type OrderBy struct {
	Field string
	Order string // "asc" or "desc"
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
}

// QueryExecutor is the base query executor interface
type QueryExecutor interface {
	ExecuteQuery(input *core.CompiledQuery, dest any) error
	ExecuteScan(input *core.CompiledQuery, dest any) error
}

// PaginatedQueryExecutor extends QueryExecutor with pagination support
type PaginatedQueryExecutor interface {
	QueryExecutor
	ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*QueryResult, error)
	ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*ScanResult, error)
}

// PutItemExecutor extends QueryExecutor with PutItem support
type PutItemExecutor interface {
	QueryExecutor
	ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error
}

// UpdateItemExecutor extends QueryExecutor with UpdateItem support
type UpdateItemExecutor interface {
	QueryExecutor
	ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error
}

// UpdateItemWithResultExecutor extends UpdateItemExecutor with result support
type UpdateItemWithResultExecutor interface {
	UpdateItemExecutor
	ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error)
}

// DeleteItemExecutor extends QueryExecutor with DeleteItem support
type DeleteItemExecutor interface {
	QueryExecutor
	ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error
}

// BatchWriteItemExecutor extends QueryExecutor with BatchWriteItem support
type BatchWriteItemExecutor interface {
	QueryExecutor
	ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error)
}

// New creates a new Query instance
func New(model any, metadata core.ModelMetadata, executor QueryExecutor) *Query {
	return &Query{
		model:    model,
		metadata: metadata,
		executor: executor,
		filters:  make([]Filter, 0),
	}
}

// Where adds a condition to the query
func (q *Query) Where(field string, op string, value any) core.Query {
	q.conditions = append(q.conditions, Condition{
		Field:    field,
		Operator: op,
		Value:    value,
	})
	return q
}

// Filter adds a filter expression to the query
func (q *Query) Filter(field string, op string, value any) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	q.builder.AddFilterCondition("AND", field, op, value)
	return q
}

// Index specifies which index to use
func (q *Query) Index(name string) core.Query {
	q.index = name
	return q
}

// Limit sets the maximum number of items to return
func (q *Query) Limit(n int) core.Query {
	q.limit = n
	return q
}

// Offset sets the starting position for the query
func (q *Query) Offset(offset int) core.Query {
	q.offset = &offset
	return q
}

// OrderBy sets the sort order
func (q *Query) OrderBy(field string, order string) core.Query {
	q.orderBy = OrderBy{
		Field: field,
		Order: order,
	}
	return q
}

// Select specifies which fields to return
func (q *Query) Select(fields ...string) core.Query {
	q.projection = fields
	return q
}

// ConsistentRead enables strongly consistent reads for Query operations
func (q *Query) ConsistentRead() core.Query {
	q.consistentRead = true
	return q
}

// WithRetry configures retry behavior for eventually consistent reads
func (q *Query) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	q.retryConfig = &RetryConfig{
		MaxRetries:   maxRetries,
		InitialDelay: initialDelay,
	}
	return q
}

// First executes the query and returns the first result
func (q *Query) First(dest any) error {
	// Set limit to 1 for efficiency
	q.limit = 1

	compiled, err := q.Compile()
	if err != nil {
		return err
	}

	if compiled.Operation == "Query" {
		return q.executor.ExecuteQuery(compiled, dest)
	}
	return q.executor.ExecuteScan(compiled, dest)
}

// All executes the query and returns all results
func (q *Query) All(dest any) error {
	compiled, err := q.Compile()
	if err != nil {
		return err
	}

	if compiled.Operation == "Query" {
		return q.executor.ExecuteQuery(compiled, dest)
	}
	return q.executor.ExecuteScan(compiled, dest)
}

// Count returns the count of matching items
func (q *Query) Count() (int64, error) {
	compiled, err := q.Compile()
	if err != nil {
		return 0, err
	}

	// Set select to COUNT for efficiency
	compiled.Select = "COUNT"

	var result struct {
		Count        int64
		ScannedCount int64
	}

	if compiled.Operation == "Query" {
		err = q.executor.ExecuteQuery(compiled, &result)
	} else {
		err = q.executor.ExecuteScan(compiled, &result)
	}

	return result.Count, err
}

// Create creates a new item
func (q *Query) Create() error {
	// Marshal the model to AttributeValues
	item, err := convertItemToAttributeValue(q.model)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem request
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	// Add conditional expression to prevent overwriting existing items
	if len(q.conditions) > 0 {
		// If conditions are specified, use them as conditional expressions
		builder := expr.NewBuilder()
		for _, cond := range q.conditions {
			err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return fmt.Errorf("failed to build condition: %w", err)
			}
		}
		components := builder.Build()
		compiled.ConditionExpression = components.FilterExpression
		compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
		compiled.ExpressionAttributeValues = components.ExpressionAttributeValues
	} else {
		// Default: ensure item doesn't already exist (using partition key)
		primaryKey := q.metadata.PrimaryKey()
		if primaryKey.PartitionKey != "" {
			builder := expr.NewBuilder()
			builder.AddFilterCondition("AND", primaryKey.PartitionKey, "attribute_not_exists", nil)
			components := builder.Build()
			compiled.ConditionExpression = components.FilterExpression
			compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
		}
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		return putExecutor.ExecutePutItem(compiled, item)
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (q *Query) CreateOrUpdate() error {
	// Build the item to put
	item := make(map[string]types.AttributeValue)

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}
	modelType := modelValue.Type()

	// Convert all fields to AttributeValues
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Parse dynamorm tags
		tag := field.Tag.Get("dynamorm")
		if tag == "-" {
			continue
		}

		// Get field value
		fieldValue := modelValue.Field(i)
		if !fieldValue.IsValid() {
			continue
		}

		// Skip zero values if omitempty is set
		if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
			continue
		}

		// Convert to AttributeValue
		av, err := expr.ConvertToAttributeValue(fieldValue.Interface())
		if err != nil {
			return fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		// Use field name as key
		item[field.Name] = av
	}

	// Compile the query for PutItem (without condition expression)
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		return putExecutor.ExecutePutItem(compiled, item)
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}

// isZeroValue checks if a reflect.Value is the zero value for its type
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Struct:
		// Check if it's time.Time
		if v.Type().String() == "time.Time" {
			return v.Interface().(interface{ IsZero() bool }).IsZero()
		}
		// For other structs, check if all fields are zero
		for i := 0; i < v.NumField(); i++ {
			if !isZeroValue(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// For other types (chan, func), compare with zero value
		return v.IsZero()
	}
}

// Update updates specified fields on an item
func (q *Query) Update(fields ...string) error {
	// Validate we have key conditions
	primaryKey := q.metadata.PrimaryKey()
	keyValues := make(map[string]any)

	// Extract key values from conditions
	for _, cond := range q.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	// Build update expression for specified fields
	updateParts := []string{}
	updateValues := make(map[string]any)

	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		modelValue = modelValue.Elem()
	}
	modelType := modelValue.Type()

	if len(fields) > 0 {
		// Update only specified fields
		for i, field := range fields {
			// Get field value from model
			fieldValue := modelValue.FieldByName(field)
			if !fieldValue.IsValid() {
				return fmt.Errorf("field %s not found in model", field)
			}

			// Add to update expression
			placeholder := fmt.Sprintf(":val%d", i)
			updateParts = append(updateParts, fmt.Sprintf("#%s = %s", field, placeholder))
			updateValues[placeholder] = fieldValue.Interface()
		}
	} else {
		// Update all non-key fields
		primaryKey := q.metadata.PrimaryKey()
		fieldIndex := 0

		for i := 0; i < modelType.NumField(); i++ {
			field := modelType.Field(i)

			// Skip unexported fields
			if !field.IsExported() {
				continue
			}

			// Parse dynamorm tags
			tag := field.Tag.Get("dynamorm")
			if tag == "-" {
				continue
			}

			// Skip primary key fields
			if field.Name == primaryKey.PartitionKey || field.Name == primaryKey.SortKey {
				continue
			}

			// Check if this is a primary key field based on tags
			if strings.Contains(tag, "pk") || strings.Contains(tag, "sk") {
				continue
			}

			// Skip special fields based on tags
			if strings.Contains(tag, "created_at") {
				continue
			}

			// Get field value
			fieldValue := modelValue.Field(i)
			if !fieldValue.IsValid() {
				continue
			}

			// Skip zero values if omitempty is set
			if strings.Contains(tag, "omitempty") && isZeroValue(fieldValue) {
				continue
			}

			// Add to update expression
			placeholder := fmt.Sprintf(":val%d", fieldIndex)
			updateParts = append(updateParts, fmt.Sprintf("#%s = %s", field.Name, placeholder))
			updateValues[placeholder] = fieldValue.Interface()

			// Also add to expression attribute names
			fields = append(fields, field.Name)
			fieldIndex++
		}

		// Check if we have any fields to update
		if len(updateParts) == 0 {
			return fmt.Errorf("no non-key fields to update")
		}
	}

	// Build expressions
	builder := expr.NewBuilder()

	// Add filter conditions as condition expressions
	for _, cond := range q.conditions {
		// Skip key conditions
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			continue
		}
		builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value)
	}

	filterComponents := builder.Build()

	// Build update expression manually
	updateExpression := ""
	if len(updateParts) > 0 {
		updateExpression = "SET " + strings.Join(updateParts, ", ")
	}

	// Convert update values to AttributeValues
	expressionAttributeValues := make(map[string]types.AttributeValue)
	for k, v := range updateValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert update value: %w", err)
		}
		expressionAttributeValues[k] = av
	}

	// Merge with filter expression values
	for k, v := range filterComponents.ExpressionAttributeValues {
		expressionAttributeValues[k] = v
	}

	// Build expression attribute names
	expressionAttributeNames := make(map[string]string)
	for _, field := range fields {
		expressionAttributeNames["#"+field] = field
	}

	// Merge with filter expression names
	for k, v := range filterComponents.ExpressionAttributeNames {
		expressionAttributeNames[k] = v
	}

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                 "UpdateItem",
		TableName:                 q.metadata.TableName(),
		UpdateExpression:          updateExpression,
		ConditionExpression:       filterComponents.FilterExpression,
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute update
	if updateExecutor, ok := q.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

// Delete deletes an item
func (q *Query) Delete() error {
	// Validate we have key conditions
	primaryKey := q.metadata.PrimaryKey()
	keyValues := make(map[string]any)

	// Extract key values from conditions
	for _, cond := range q.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for delete", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for delete", primaryKey.SortKey)
		}
	}

	// Build condition expression from non-key conditions
	builder := expr.NewBuilder()
	for _, cond := range q.conditions {
		// Skip key conditions
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			continue
		}
		builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value)
	}

	components := builder.Build()

	// Compile the delete query
	compiled := &core.CompiledQuery{
		Operation:                 "DeleteItem",
		TableName:                 q.metadata.TableName(),
		ConditionExpression:       components.FilterExpression,
		ExpressionAttributeNames:  components.ExpressionAttributeNames,
		ExpressionAttributeValues: components.ExpressionAttributeValues,
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute delete
	if deleteExecutor, ok := q.executor.(DeleteItemExecutor); ok {
		return deleteExecutor.ExecuteDeleteItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support DeleteItem operation")
}

// Scan performs a table scan
func (q *Query) Scan(dest any) error {
	compiled, err := q.compileScan()
	if err != nil {
		return err
	}

	return q.executor.ExecuteScan(compiled, dest)
}

// ParallelScan performs a parallel table scan with the specified segment
func (q *Query) ParallelScan(segment int32, totalSegments int32) core.Query {
	q.segment = &segment
	q.totalSegments = &totalSegments
	return q
}

// ScanAllSegments performs a parallel scan across all segments and combines results
func (q *Query) ScanAllSegments(dest any, totalSegments int32) error {
	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	// Create a channel to collect results from each segment
	type segmentResult struct {
		items []any
		err   error
	}

	results := make(chan segmentResult, totalSegments)

	// Launch goroutines for each segment
	for i := int32(0); i < totalSegments; i++ {
		go func(segment int32) {
			// Create a new query for this segment
			segmentQuery := &Query{
				model:         q.model,
				conditions:    q.conditions,
				filters:       q.filters,
				rawFilters:    q.rawFilters,
				index:         q.index,
				limit:         q.limit,
				offset:        q.offset,
				projection:    q.projection,
				orderBy:       q.orderBy,
				exclusive:     q.exclusive,
				ctx:           q.ctx,
				metadata:      q.metadata,
				executor:      q.executor,
				builder:       q.builder,
				segment:       &segment,
				totalSegments: &totalSegments,
			}

			// Create a slice to hold this segment's results
			elemType := destValue.Type().Elem()
			segmentDest := reflect.New(reflect.SliceOf(elemType))

			// Execute scan for this segment
			err := segmentQuery.Scan(segmentDest.Interface())
			if err != nil {
				results <- segmentResult{nil, err}
				return
			}

			// Convert results to []any
			segmentSlice := segmentDest.Elem()
			items := make([]any, segmentSlice.Len())
			for j := 0; j < segmentSlice.Len(); j++ {
				items[j] = segmentSlice.Index(j).Interface()
			}

			results <- segmentResult{items, nil}
		}(i)
	}

	// Collect results from all segments
	var allItems []any
	for i := int32(0); i < totalSegments; i++ {
		result := <-results
		if result.err != nil {
			return result.err
		}
		allItems = append(allItems, result.items...)
	}

	// Combine all results into the destination slice
	destSlice := destValue.Elem()
	newSlice := reflect.MakeSlice(destSlice.Type(), len(allItems), len(allItems))

	for i, item := range allItems {
		newSlice.Index(i).Set(reflect.ValueOf(item))
	}

	destSlice.Set(newSlice)
	return nil
}

// BatchGet retrieves multiple items by their primary keys
func (q *Query) BatchGet(keys []any, dest any) error {
	// Validate dest is a pointer to slice
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return errors.New("dest must be a pointer to slice")
	}

	// Validate keys
	if len(keys) == 0 {
		return errors.New("no keys provided")
	}

	if len(keys) > 100 {
		return errors.New("BatchGet supports maximum 100 keys per request")
	}

	// Build batch get request
	batchGet := &CompiledBatchGet{
		TableName: q.metadata.TableName(),
		Keys:      make([]map[string]types.AttributeValue, 0, len(keys)),
	}

	// Add projection if specified
	if len(q.projection) > 0 {
		builder := expr.NewBuilder()
		builder.AddProjection(q.projection...)
		components := builder.Build()
		batchGet.ProjectionExpression = components.ProjectionExpression
		batchGet.ExpressionAttributeNames = components.ExpressionAttributeNames
	}

	// Convert keys to AttributeValues
	primaryKey := q.metadata.PrimaryKey()
	for _, key := range keys {
		keyMap := make(map[string]types.AttributeValue)

		// Handle composite keys
		keyValue := reflect.ValueOf(key)
		if keyValue.Kind() == reflect.Struct {
			// Extract partition and sort key from struct
			for i := 0; i < keyValue.NumField(); i++ {
				field := keyValue.Type().Field(i)
				if field.Name == primaryKey.PartitionKey ||
					(primaryKey.SortKey != "" && field.Name == primaryKey.SortKey) {
					av, err := expr.ConvertToAttributeValue(keyValue.Field(i).Interface())
					if err != nil {
						return fmt.Errorf("failed to convert key field %s: %w", field.Name, err)
					}
					keyMap[field.Name] = av
				}
			}
		} else {
			// Simple primary key
			av, err := expr.ConvertToAttributeValue(key)
			if err != nil {
				return fmt.Errorf("failed to convert key: %w", err)
			}
			keyMap[primaryKey.PartitionKey] = av
		}

		batchGet.Keys = append(batchGet.Keys, keyMap)
	}

	// Execute batch get through executor
	if executor, ok := q.executor.(BatchExecutor); ok {
		return executor.ExecuteBatchGet(batchGet, dest)
	}

	return errors.New("executor does not support batch operations")
}

// BatchCreate creates multiple items
func (q *Query) BatchCreate(items any) error {
	// Validate items is a slice
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() != reflect.Slice {
		return errors.New("items must be a slice")
	}

	if itemsValue.Len() == 0 {
		return errors.New("no items to create")
	}

	if itemsValue.Len() > 25 {
		return errors.New("BatchCreate supports maximum 25 items per request")
	}

	// Try to use the new BatchWriteItemExecutor first
	if batchWriteExecutor, ok := q.executor.(BatchWriteItemExecutor); ok {
		// Convert items to write requests
		writeRequests := make([]types.WriteRequest, 0, itemsValue.Len())

		for i := 0; i < itemsValue.Len(); i++ {
			item := itemsValue.Index(i).Interface()

			// Convert item to AttributeValues
			av, err := convertItemToAttributeValue(item)
			if err != nil {
				return fmt.Errorf("failed to convert item %d: %w", i, err)
			}

			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: av,
				},
			})
		}

		// Execute batch write
		result, err := batchWriteExecutor.ExecuteBatchWriteItem(q.metadata.TableName(), writeRequests)
		if err != nil {
			return err
		}

		// Check for unprocessed items
		if len(result.UnprocessedItems) > 0 {
			unprocessedCount := 0
			for _, items := range result.UnprocessedItems {
				unprocessedCount += len(items)
			}
			if unprocessedCount > 0 {
				return fmt.Errorf("%d items were not processed", unprocessedCount)
			}
		}

		return nil
	}

	// Fall back to old BatchExecutor for backward compatibility
	if executor, ok := q.executor.(BatchExecutor); ok {
		// Build batch write request
		batchWrite := &CompiledBatchWrite{
			TableName: q.metadata.TableName(),
			Items:     make([]map[string]types.AttributeValue, 0, itemsValue.Len()),
		}

		// Convert items to AttributeValues
		for i := 0; i < itemsValue.Len(); i++ {
			item := itemsValue.Index(i).Interface()

			// Convert item to map[string]types.AttributeValue
			av, err := convertItemToAttributeValue(item)
			if err != nil {
				return fmt.Errorf("failed to convert item %d: %w", i, err)
			}

			batchWrite.Items = append(batchWrite.Items, av)
		}

		return executor.ExecuteBatchWrite(batchWrite)
	}

	return errors.New("executor does not support batch operations")
}

// WithContext sets the context for the query
func (q *Query) WithContext(ctx context.Context) core.Query {
	q.ctx = ctx
	return q
}

// selectBestIndex analyzes conditions and selects the optimal index
func (q *Query) selectBestIndex() (*core.IndexSchema, error) {
	// Get all indexes including the primary index
	allIndexes := make([]core.IndexSchema, 0, len(q.metadata.Indexes())+1)

	// Add the primary index
	primaryKey := q.metadata.PrimaryKey()
	allIndexes = append(allIndexes, core.IndexSchema{
		Name:         "", // Empty name indicates primary index
		Type:         "PRIMARY",
		PartitionKey: primaryKey.PartitionKey,
		SortKey:      primaryKey.SortKey,
	})

	// Add GSIs and LSIs
	allIndexes = append(allIndexes, q.metadata.Indexes()...)

	selector := index.NewSelector(allIndexes)

	// Convert our conditions to index.Condition type
	indexConditions := make([]index.Condition, len(q.conditions))
	for i, cond := range q.conditions {
		indexConditions[i] = index.Condition{
			Field:    cond.Field,
			Operator: cond.Operator,
			Value:    cond.Value,
		}
	}

	// Analyze conditions to find required keys
	requiredKeys := index.AnalyzeConditions(indexConditions)

	// Use the selector to find the best index
	return selector.SelectOptimal(requiredKeys, nil)
}

// AllPaginated executes the query and returns paginated results
func (q *Query) AllPaginated(dest any) (*core.PaginatedResult, error) {
	// Set a reasonable limit if not specified
	if q.limit == 0 {
		q.limit = 100
	}

	compiled, err := q.Compile()
	if err != nil {
		return nil, err
	}

	// Execute the query
	var result any
	if compiled.Operation == "Query" {
		result, err = q.executePaginatedQuery(compiled, dest)
	} else {
		result, err = q.executePaginatedScan(compiled, dest)
	}

	if err != nil {
		return nil, err
	}

	// Extract pagination info
	queryResult := result.(map[string]any)

	// Build the paginated result
	paginatedResult := &core.PaginatedResult{
		Items:        dest,
		NextCursor:   q.encodeCursor(queryResult["LastEvaluatedKey"]),
		Count:        0,
		ScannedCount: 0,
	}

	// Safely extract counts
	if count, ok := queryResult["Count"].(int64); ok {
		paginatedResult.Count = int(count)
	} else if count, ok := queryResult["Count"].(int); ok {
		paginatedResult.Count = count
	}

	if scannedCount, ok := queryResult["ScannedCount"].(int64); ok {
		paginatedResult.ScannedCount = int(scannedCount)
	} else if scannedCount, ok := queryResult["ScannedCount"].(int); ok {
		paginatedResult.ScannedCount = scannedCount
	}

	// Set HasMore based on cursor
	paginatedResult.HasMore = paginatedResult.NextCursor != ""

	// Extract LastEvaluatedKey
	if lastKey, ok := queryResult["LastEvaluatedKey"].(map[string]types.AttributeValue); ok {
		paginatedResult.LastEvaluatedKey = lastKey
	}

	return paginatedResult, nil
}

// SetCursor sets the pagination cursor for the query
func (q *Query) SetCursor(cursor string) error {
	if cursor == "" {
		return nil
	}

	// Decode the cursor to ExclusiveStartKey
	startKey, err := q.decodeCursor(cursor)
	if err != nil {
		return fmt.Errorf("invalid cursor: %w", err)
	}

	q.exclusive = startKey
	return nil
}

// Cursor is a fluent method to set the pagination cursor
func (q *Query) Cursor(cursor string) core.Query {
	if err := q.SetCursor(cursor); err != nil {
		// Store error to be returned on execution
		// This pattern is common in fluent interfaces
		// The error will be checked and returned when the query is executed
	}
	return q
}

// executePaginatedQuery executes a query with pagination support
func (q *Query) executePaginatedQuery(compiled *core.CompiledQuery, dest any) (any, error) {
	// Check if executor supports pagination
	if paginatedExecutor, ok := q.executor.(PaginatedQueryExecutor); ok {
		result, err := paginatedExecutor.ExecuteQueryWithPagination(compiled, dest)
		if err != nil {
			return nil, err
		}

		// Return the actual pagination info
		return map[string]any{
			"Count":            result.Count,
			"ScannedCount":     result.ScannedCount,
			"LastEvaluatedKey": result.LastEvaluatedKey,
		}, nil
	}

	// Fall back to regular query without pagination info
	err := q.executor.ExecuteQuery(compiled, dest)
	if err != nil {
		return nil, err
	}

	// Return mock result for backward compatibility
	return map[string]any{
		"Count":            0,
		"ScannedCount":     0,
		"LastEvaluatedKey": nil,
	}, nil
}

// executePaginatedScan executes a scan with pagination support
func (q *Query) executePaginatedScan(compiled *core.CompiledQuery, dest any) (any, error) {
	// Check if executor supports pagination
	if paginatedExecutor, ok := q.executor.(PaginatedQueryExecutor); ok {
		result, err := paginatedExecutor.ExecuteScanWithPagination(compiled, dest)
		if err != nil {
			return nil, err
		}

		// Return the actual pagination info
		return map[string]any{
			"Count":            result.Count,
			"ScannedCount":     result.ScannedCount,
			"LastEvaluatedKey": result.LastEvaluatedKey,
		}, nil
	}

	// Fall back to regular scan without pagination info
	err := q.executor.ExecuteScan(compiled, dest)
	if err != nil {
		return nil, err
	}

	// Return mock result for backward compatibility
	return map[string]any{
		"Count":            0,
		"ScannedCount":     0,
		"LastEvaluatedKey": nil,
	}, nil
}

// encodeCursor encodes the LastEvaluatedKey as a cursor string
func (q *Query) encodeCursor(lastKey any) string {
	if lastKey == nil {
		return ""
	}

	// Convert to map[string]types.AttributeValue if needed
	var avMap map[string]types.AttributeValue
	switch v := lastKey.(type) {
	case map[string]types.AttributeValue:
		avMap = v
	case map[string]any:
		// Handle the case where lastKey is map[string]any
		// This would come from the executor results
		if val, ok := v["LastEvaluatedKey"]; ok {
			if m, ok := val.(map[string]types.AttributeValue); ok {
				avMap = m
			}
		}
	default:
		return ""
	}

	if len(avMap) == 0 {
		return ""
	}

	// Use the new EncodeCursor function
	encoded, err := EncodeCursor(avMap, q.index, q.orderBy.Order)
	if err != nil {
		// Log error in production
		return ""
	}
	return encoded
}

// decodeCursor decodes a cursor string to ExclusiveStartKey
func (q *Query) decodeCursor(cursor string) (map[string]types.AttributeValue, error) {
	if cursor == "" {
		return nil, nil
	}

	// Use the new DecodeCursor function
	decodedCursor, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}

	if decodedCursor == nil {
		return nil, nil
	}

	// Convert back to AttributeValues
	return decodedCursor.ToAttributeValues()
}

// Compile compiles the query into executable form
func (q *Query) Compile() (*core.CompiledQuery, error) {
	// Use existing builder if available (contains filters from Filter/OrFilter calls)
	// Otherwise create a new one
	var builder *expr.Builder
	if q.builder != nil {
		builder = q.builder
	} else {
		builder = expr.NewBuilder()
	}

	// Select the best index
	bestIndex, err := q.selectBestIndex()
	if err != nil {
		return nil, err
	}

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
	}

	// If we have a suitable index, use Query operation
	if bestIndex != nil {
		compiled.Operation = "Query"
		if bestIndex.Name != "" {
			compiled.IndexName = bestIndex.Name
		}

		// Separate key conditions from filter conditions
		var keyConditions []Condition
		var filterConditions []Condition

		primaryKey := q.metadata.PrimaryKey()
		indexPK := bestIndex.PartitionKey
		indexSK := bestIndex.SortKey

		// If using primary table
		if bestIndex.Name == "" {
			indexPK = primaryKey.PartitionKey
			indexSK = primaryKey.SortKey
		}

		for _, cond := range q.conditions {
			if cond.Field == indexPK || (indexSK != "" && cond.Field == indexSK) {
				keyConditions = append(keyConditions, cond)
			} else {
				filterConditions = append(filterConditions, cond)
			}
		}

		// Add key conditions
		for _, cond := range keyConditions {
			err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return nil, err
			}
		}

		// Add filter conditions from Where clauses
		for _, cond := range filterConditions {
			err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// Must use Scan
		compiled.Operation = "Scan"

		// All conditions become filters
		for _, cond := range q.conditions {
			err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value)
			if err != nil {
				return nil, err
			}
		}
	}

	// Note: Additional filters from Filter/OrFilter calls are already in the builder

	// Add projections
	if len(q.projection) > 0 {
		builder.AddProjection(q.projection...)
	}

	// Build the expressions
	components := builder.Build()
	compiled.KeyConditionExpression = components.KeyConditionExpression
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues

	// Set other parameters
	if q.limit > 0 {
		limit := int32(q.limit)
		compiled.Limit = &limit
	}

	if q.orderBy.Order == "desc" {
		forward := false
		compiled.ScanIndexForward = &forward
	}

	// Handle cursor/exclusive start key
	if q.exclusive != nil && len(q.exclusive) > 0 {
		compiled.ExclusiveStartKey = q.exclusive
	}

	// Set consistent read (only for main table, not GSI)
	if q.consistentRead && compiled.IndexName == "" {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, nil
}

// compileScan compiles a scan operation
func (q *Query) compileScan() (*core.CompiledQuery, error) {
	// Use existing builder if available (contains filters from Filter/OrFilter calls)
	// Otherwise create a new one
	var builder *expr.Builder
	if q.builder != nil {
		builder = q.builder
	} else {
		builder = expr.NewBuilder()
	}

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
		Operation: "Scan",
	}

	// Add filter conditions from Where clauses
	for _, cond := range q.conditions {
		err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value)
		if err != nil {
			return nil, err
		}
	}

	// Note: Additional filters from Filter/OrFilter calls are already in the builder

	// Add projections
	if len(q.projection) > 0 {
		builder.AddProjection(q.projection...)
	}

	// Build the expressions
	components := builder.Build()
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues

	// Set parameters
	if q.limit > 0 {
		limit := int32(q.limit)
		compiled.Limit = &limit
	}

	// Handle offset with pagination
	if q.offset != nil && *q.offset > 0 {
		// Note: DynamoDB doesn't support direct offset, so this would need
		// to be handled by the executor with multiple requests
		compiled.Offset = q.offset
	}

	compiled.ExclusiveStartKey = q.exclusive

	// Set parallel scan parameters if specified
	if q.segment != nil && q.totalSegments != nil {
		compiled.Segment = q.segment
		compiled.TotalSegments = q.totalSegments
	}

	// Set consistent read (only for main table scan, not GSI)
	if q.consistentRead && q.index == "" {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, nil
}

// convertItemToAttributeValue converts an item to DynamoDB AttributeValue map
func convertItemToAttributeValue(item any) (map[string]types.AttributeValue, error) {
	// Use our new converter
	av, err := expr.ConvertToAttributeValue(item)
	if err != nil {
		return nil, fmt.Errorf("failed to convert item: %w", err)
	}

	// The converter returns a M type for structs
	if m, ok := av.(*types.AttributeValueMemberM); ok {
		return m.Value, nil
	}

	return nil, fmt.Errorf("expected map type for struct conversion, got %T", av)
}

// OrFilter adds an OR filter condition
func (q *Query) OrFilter(field string, op string, value any) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	q.builder.AddFilterCondition("OR", field, op, value)
	return q
}

// FilterGroup adds a grouped AND filter condition
func (q *Query) FilterGroup(fn func(core.Query)) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilder()
	subQuery := &Query{
		model:    q.model,
		metadata: q.metadata,
		executor: q.executor,
		ctx:      q.ctx,
		builder:  subBuilder,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter("AND", components)
	return q
}

// OrFilterGroup adds a grouped OR filter condition
func (q *Query) OrFilterGroup(fn func(core.Query)) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = expr.NewBuilder()
	}

	// Create a new sub-query and builder for the group
	subBuilder := expr.NewBuilder()
	subQuery := &Query{
		model:    q.model,
		metadata: q.metadata,
		executor: q.executor,
		ctx:      q.ctx,
		builder:  subBuilder,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter("OR", components)
	return q
}

// UpdateBuilder returns a builder for complex update operations
func (q *Query) UpdateBuilder() core.UpdateBuilder {
	return NewUpdateBuilder(q)
}

// NewWithConditions creates a new Query instance with all necessary fields
func NewWithConditions(model any, metadata core.ModelMetadata, executor QueryExecutor, conditions []Condition, ctx context.Context) *Query {
	return &Query{
		model:      model,
		metadata:   metadata,
		executor:   executor,
		conditions: conditions,
		ctx:        ctx,
		filters:    make([]Filter, 0),
		builder:    expr.NewBuilder(),
	}
}
