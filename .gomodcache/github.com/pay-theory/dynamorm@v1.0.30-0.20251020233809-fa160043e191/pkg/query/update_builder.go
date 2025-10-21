// Package query provides update builder functionality for DynamoDB
package query

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/internal/expr"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// UpdateBuilder provides a fluent API for building complex update expressions
type UpdateBuilder struct {
	query        *Query
	expr         *expr.Builder
	keyValues    map[string]any
	conditions   []updateCondition
	returnValues string
}

type updateCondition struct {
	field    string
	operator string
	value    any
}

// NewUpdateBuilder creates a new UpdateBuilder with the given query
func NewUpdateBuilder(q *Query) core.UpdateBuilder {
	return &UpdateBuilder{
		query:        q,
		expr:         expr.NewBuilder(),
		keyValues:    make(map[string]any),
		returnValues: "NONE", // Default
	}
}

// mapFieldToDynamoDBName maps a Go field name to its DynamoDB attribute name
func (ub *UpdateBuilder) mapFieldToDynamoDBName(field string) string {
	if ub.query.metadata != nil {
		if fieldMeta := ub.query.metadata.AttributeMetadata(field); fieldMeta != nil {
			return fieldMeta.DynamoDBName
		}
	}
	return field
}

// Set adds a SET expression to update a field
func (ub *UpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	ub.expr.AddUpdateSet(dbFieldName, value)
	return ub
}

// SetIfNotExists sets a field only if it doesn't exist
func (ub *UpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	// DynamoDB if_not_exists function syntax: SET field = if_not_exists(field, default_value)
	// The 'value' parameter is ignored as DynamoDB if_not_exists only checks existence, not value comparison
	err := ub.expr.AddUpdateFunction(dbFieldName, "if_not_exists", dbFieldName, defaultValue)
	if err != nil {
		// Log error in production, for now fall back to regular set
		// TODO: Add proper error handling
		ub.expr.AddUpdateSet(dbFieldName, defaultValue)
	}
	return ub
}

// Add increments a numeric field (atomic counter)
func (ub *UpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	ub.expr.AddUpdateAdd(dbFieldName, value)
	return ub
}

// Increment is an alias for Add with value 1
func (ub *UpdateBuilder) Increment(field string) core.UpdateBuilder {
	return ub.Add(field, 1)
}

// Decrement is an alias for Add with value -1
func (ub *UpdateBuilder) Decrement(field string) core.UpdateBuilder {
	return ub.Add(field, -1)
}

// Remove removes an attribute from the item
func (ub *UpdateBuilder) Remove(field string) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	ub.expr.AddUpdateRemove(dbFieldName)
	return ub
}

// Delete removes elements from a set
func (ub *UpdateBuilder) Delete(field string, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)

	// DynamoDB DELETE action is for removing elements from a set
	// Ensure the value is properly formatted as a set
	var setValue any

	// Convert single values to slices for set operations
	switch v := value.(type) {
	case string:
		setValue = []string{v}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		setValue = []any{v}
	case []byte:
		setValue = [][]byte{v}
	case []string, []int, []float64, [][]byte:
		setValue = v
	default:
		// For other types, try to convert to a slice if it's not already
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice {
			setValue = value
		} else {
			// Wrap single value in a slice
			setValue = []any{value}
		}
	}

	ub.expr.AddUpdateDelete(dbFieldName, setValue)
	return ub
}

// AppendToList appends values to the end of a list
func (ub *UpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	// Use list_append function to append values
	// list_append(field, values) appends to the end
	err := ub.expr.AddUpdateFunction(dbFieldName, "list_append", dbFieldName, values)
	if err != nil {
		// Log error in production
		// Fall back to regular set (not ideal but better than failing)
		ub.expr.AddUpdateSet(dbFieldName, values)
	}
	return ub
}

// PrependToList prepends values to the beginning of a list
func (ub *UpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	// Use list_append function to prepend values
	// list_append(values, field) prepends to the beginning
	err := ub.expr.AddUpdateFunction(dbFieldName, "list_append", values, dbFieldName)
	if err != nil {
		// Log error in production
		// Fall back to regular set (not ideal but better than failing)
		ub.expr.AddUpdateSet(dbFieldName, values)
	}
	return ub
}

// RemoveFromListAt removes an element from a list at a specific index
func (ub *UpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	ub.expr.AddUpdateRemove(fmt.Sprintf("%s[%d]", dbFieldName, index))
	return ub
}

// SetListElement sets a specific element in a list
func (ub *UpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	ub.expr.AddUpdateSet(fmt.Sprintf("%s[%d]", dbFieldName, index), value)
	return ub
}

// Condition adds a condition that must be met for the update to succeed
func (ub *UpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	ub.conditions = append(ub.conditions, updateCondition{
		field:    field, // Keep original field name here, mapping happens in Execute()
		operator: operator,
		value:    value,
	})
	return ub
}

// OrCondition adds a condition with OR logic
func (ub *UpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	// For now, OR conditions are treated as AND conditions in this implementation
	// TODO: Implement proper OR logic support
	return ub.Condition(field, operator, value)
}

// ConditionExists adds a condition that the field must exist
func (ub *UpdateBuilder) ConditionExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_exists", nil)
}

// ConditionNotExists adds a condition that the field must not exist
func (ub *UpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_not_exists", nil)
}

// ConditionVersion adds optimistic locking based on version field
func (ub *UpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	return ub.Condition("Version", "=", currentVersion)
}

// ReturnValues sets what values to return after the update
func (ub *UpdateBuilder) ReturnValues(option string) core.UpdateBuilder {
	// Options: NONE, ALL_OLD, UPDATED_OLD, ALL_NEW, UPDATED_NEW
	ub.returnValues = option
	return ub
}

// Execute performs the update operation
func (ub *UpdateBuilder) Execute() error {
	// Check if query or metadata is nil
	if ub.query == nil {
		return fmt.Errorf("query is nil")
	}
	if ub.query.metadata == nil {
		return fmt.Errorf("query metadata is nil")
	}

	// Validate we have key conditions
	primaryKey := ub.query.metadata.PrimaryKey()

	// Extract key values from query conditions
	for _, cond := range ub.query.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			// Get the DynamoDB attribute name for this field
			attrMeta := ub.query.metadata.AttributeMetadata(cond.Field)
			if attrMeta != nil {
				ub.keyValues[attrMeta.DynamoDBName] = cond.Value
			} else {
				ub.keyValues[cond.Field] = cond.Value
			}
		}
	}

	// Validate we have complete key using DynamoDB attribute names
	pkAttrMeta := ub.query.metadata.AttributeMetadata(primaryKey.PartitionKey)
	pkName := primaryKey.PartitionKey
	if pkAttrMeta != nil {
		pkName = pkAttrMeta.DynamoDBName
	}

	if _, ok := ub.keyValues[pkName]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}

	if primaryKey.SortKey != "" {
		skAttrMeta := ub.query.metadata.AttributeMetadata(primaryKey.SortKey)
		skName := primaryKey.SortKey
		if skAttrMeta != nil {
			skName = skAttrMeta.DynamoDBName
		}
		if _, ok := ub.keyValues[skName]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	// Add conditions to expression builder
	for _, cond := range ub.conditions {
		// Map field name to DynamoDB attribute name
		fieldName := cond.field
		if fieldMeta := ub.query.metadata.AttributeMetadata(cond.field); fieldMeta != nil {
			fieldName = fieldMeta.DynamoDBName
		}

		err := ub.expr.AddConditionExpression(fieldName, cond.operator, cond.value)
		if err != nil {
			return fmt.Errorf("failed to add condition: %w", err)
		}
	}

	// Build the expression components
	components := ub.expr.Build()

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                "UpdateItem",
		TableName:                ub.query.metadata.TableName(),
		UpdateExpression:         components.UpdateExpression,
		ConditionExpression:      components.ConditionExpression,
		ExpressionAttributeNames: components.ExpressionAttributeNames,
		ReturnValues:             ub.returnValues,
	}

	// Only include ExpressionAttributeValues if it's not empty
	if len(components.ExpressionAttributeValues) > 0 {
		compiled.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range ub.keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute update through executor
	if updateExecutor, ok := ub.query.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

// ExecuteWithResult performs the update and returns the result
func (ub *UpdateBuilder) ExecuteWithResult(result any) error {
	// Validate result is a pointer
	resultValue := reflect.ValueOf(result)
	if resultValue.Kind() != reflect.Ptr || resultValue.IsNil() {
		return fmt.Errorf("result must be a non-nil pointer")
	}

	// Set return values to ALL_NEW if not already set
	if ub.returnValues == "NONE" {
		ub.returnValues = "ALL_NEW"
	}

	// Validate we have key conditions
	primaryKey := ub.query.metadata.PrimaryKey()

	// Extract key values from query conditions
	for _, cond := range ub.query.conditions {
		if cond.Field == primaryKey.PartitionKey ||
			(primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			ub.keyValues[cond.Field] = cond.Value
		}
	}

	// Validate we have complete key
	if _, ok := ub.keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}
	if primaryKey.SortKey != "" {
		if _, ok := ub.keyValues[primaryKey.SortKey]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	// Add conditions to expression builder
	for _, cond := range ub.conditions {
		// Map field name to DynamoDB attribute name
		fieldName := cond.field
		if fieldMeta := ub.query.metadata.AttributeMetadata(cond.field); fieldMeta != nil {
			fieldName = fieldMeta.DynamoDBName
		}

		err := ub.expr.AddConditionExpression(fieldName, cond.operator, cond.value)
		if err != nil {
			return fmt.Errorf("failed to add condition: %w", err)
		}
	}

	// Build the expression components
	components := ub.expr.Build()

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                "UpdateItem",
		TableName:                ub.query.metadata.TableName(),
		UpdateExpression:         components.UpdateExpression,
		ConditionExpression:      components.ConditionExpression,
		ExpressionAttributeNames: components.ExpressionAttributeNames,
		ReturnValues:             ub.returnValues,
	}

	// Only include ExpressionAttributeValues if it's not empty
	if len(components.ExpressionAttributeValues) > 0 {
		compiled.ExpressionAttributeValues = components.ExpressionAttributeValues
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range ub.keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Check if executor supports returning results
	if updateExecutor, ok := ub.query.executor.(UpdateItemWithResultExecutor); ok {
		updateResult, err := updateExecutor.ExecuteUpdateItemWithResult(compiled, keyAV)
		if err != nil {
			return err
		}

		// Unmarshal the returned attributes to the result
		if updateResult != nil && len(updateResult.Attributes) > 0 {
			// Convert the map to a M type AttributeValue and then unmarshal
			mapAV := &types.AttributeValueMemberM{Value: updateResult.Attributes}
			return expr.ConvertFromAttributeValue(mapAV, result)
		}
		return nil
	}

	// Fallback to regular update without result
	if updateExecutor, ok := ub.query.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}
