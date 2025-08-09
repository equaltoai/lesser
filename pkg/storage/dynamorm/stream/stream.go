package stream

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
)

// UnmarshalItem unmarshals a single DynamoDB stream record into a struct
func UnmarshalItem(record events.DynamoDBEventRecord, out any) error {
	// Check if we're dealing with a NewImage (INSERT/MODIFY) or OldImage (REMOVE)
	var image map[string]events.DynamoDBAttributeValue
	switch record.EventName {
	case eventNameInsert, eventNameModify:
		image = record.Change.NewImage
	case eventNameRemove:
		image = record.Change.OldImage
	default:
		return fmt.Errorf("unknown event type: %s", record.EventName)
	}

	// Convert DynamoDB stream attribute values to DynamORM compatible format
	item, err := convertStreamImageToItem(image)
	if err != nil {
		return fmt.Errorf("failed to convert stream image: %w", err)
	}

	// Use reflection to manually set fields
	// This is a simplified approach - in a real implementation, you would need
	// to convert the map to the appropriate DynamoDB attribute value types
	outVal := reflect.ValueOf(out).Elem()
	outType := outVal.Type()

	for i := 0; i < outType.NumField(); i++ {
		field := outType.Field(i)
		fieldName := field.Name

		// Check if the field exists in the item map
		if val, ok := item[fieldName]; ok {
			fieldVal := outVal.Field(i)
			if fieldVal.CanSet() {
				// Convert and set the value based on field type
				if err := setFieldValue(fieldVal, val); err != nil {
					common.Logger().Warn("Failed to set field value",
						zap.String("field", fieldName),
						zap.Error(err),
					)
				}
			}
		}
	}

	return nil
}

// UnmarshalItems unmarshals multiple DynamoDB stream records into a slice of structs
func UnmarshalItems(records []events.DynamoDBEventRecord, outType any) (any, error) {
	// Create a slice of the correct type to hold results
	sliceType := reflect.SliceOf(reflect.TypeOf(outType))
	results := reflect.MakeSlice(sliceType, 0, len(records))

	// Process each record
	for _, record := range records {
		// Create a new instance of the output type
		item := reflect.New(reflect.TypeOf(outType)).Interface()

		// Unmarshal the record
		if err := UnmarshalItem(record, item); err != nil {
			common.Logger().Warn("Failed to unmarshal stream record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			continue
		}

		// Add to results
		results = reflect.Append(results, reflect.ValueOf(item).Elem())
	}

	return results.Interface(), nil
}

// ProcessStreamRecords processes DynamoDB stream records with the provided handler function
func ProcessStreamRecords[T any](ctx context.Context, records []events.DynamoDBEventRecord, handler func(ctx context.Context, record events.DynamoDBEventRecord, item T) error) error {
	for _, record := range records {
		// Skip records we don't care about
		if record.EventName != eventNameInsert && record.EventName != eventNameModify && record.EventName != eventNameRemove {
			continue
		}

		// Create a new instance of T
		var item T

		// Unmarshal the record
		if err := UnmarshalItem(record, &item); err != nil {
			common.Logger().Warn("Failed to unmarshal stream record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			continue
		}

		// Process the item with the handler
		if err := handler(ctx, record, item); err != nil {
			common.Logger().Error("Failed to process stream record",
				zap.String("eventID", record.EventID),
				zap.String("eventName", record.EventName),
				zap.Error(err),
			)
			// Continue processing other records even if one fails
			continue
		}
	}

	return nil
}

// Helper function to convert DynamoDB stream attribute values to DynamORM compatible format
func convertStreamImageToItem(image map[string]events.DynamoDBAttributeValue) (map[string]any, error) {
	item := make(map[string]any)

	for key, value := range image {
		converted, err := convertAttributeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to convert attribute %s: %w", key, err)
		}
		item[key] = converted
	}

	return item, nil
}

// setFieldValue sets a field value using reflection, handling type conversions
func setFieldValue(field reflect.Value, value any) error {
	if value == nil {
		return nil
	}

	// Try direct assignment first
	if tryDirectAssignment(field, value) {
		return nil
	}

	// Handle type conversions based on field kind
	switch field.Kind() {
	case reflect.String:
		return setStringField(field, value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return setIntField(field, value)
	case reflect.Float32, reflect.Float64:
		return setFloatField(field, value)
	case reflect.Bool:
		return setBoolField(field, value)
	case reflect.Slice:
		return setSliceField(field, value)
	case reflect.Map:
		return setMapField(field, value)
	default:
		return fmt.Errorf("cannot convert %v (type %T) to %v", value, value, field.Type())
	}
}

// tryDirectAssignment attempts to directly assign the value if types match
func tryDirectAssignment(field reflect.Value, value any) bool {
	valueType := reflect.TypeOf(value)
	if valueType.AssignableTo(field.Type()) {
		field.Set(reflect.ValueOf(value))
		return true
	}
	return false
}

// setStringField sets a string field value
func setStringField(field reflect.Value, value any) error {
	if str, ok := value.(string); ok {
		field.SetString(str)
		return nil
	}
	return fmt.Errorf("cannot convert %v to string", value)
}

// setIntField sets an integer field value
func setIntField(field reflect.Value, value any) error {
	switch v := value.(type) {
	case string:
		if num, err := strconv.ParseInt(v, 10, 64); err == nil {
			field.SetInt(num)
			return nil
		}
	case float64:
		field.SetInt(int64(v))
		return nil
	}
	return fmt.Errorf("cannot convert %v to int", value)
}

// setFloatField sets a float field value
func setFloatField(field reflect.Value, value any) error {
	switch v := value.(type) {
	case string:
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			field.SetFloat(num)
			return nil
		}
	case float64:
		field.SetFloat(v)
		return nil
	}
	return fmt.Errorf("cannot convert %v to float", value)
}

// setBoolField sets a boolean field value
func setBoolField(field reflect.Value, value any) error {
	if b, ok := value.(bool); ok {
		field.SetBool(b)
		return nil
	}
	return fmt.Errorf("cannot convert %v to bool", value)
}

// setSliceField sets a slice field value
func setSliceField(field reflect.Value, value any) error {
	list, ok := value.([]any)
	if !ok {
		return fmt.Errorf("cannot convert %v to slice", value)
	}

	sliceVal := reflect.MakeSlice(field.Type(), len(list), len(list))
	for i, item := range list {
		if err := setFieldValue(sliceVal.Index(i), item); err != nil {
			return err
		}
	}
	field.Set(sliceVal)
	return nil
}

// setMapField sets a map field value
func setMapField(field reflect.Value, value any) error {
	m, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("cannot convert %v to map", value)
	}

	mapVal := reflect.MakeMap(field.Type())
	for k, v := range m {
		if err := setMapEntry(mapVal, field.Type(), k, v); err != nil {
			return err
		}
	}
	field.Set(mapVal)
	return nil
}

// setMapEntry sets a single map entry
func setMapEntry(mapVal reflect.Value, fieldType reflect.Type, key string, value any) error {
	keyVal := reflect.ValueOf(key)
	valVal := reflect.New(fieldType.Elem()).Elem()
	if err := setFieldValue(valVal, value); err != nil {
		return err
	}
	mapVal.SetMapIndex(keyVal, valVal)
	return nil
}

// Helper function to convert a single DynamoDB attribute value to a Go type
func convertAttributeValue(attr events.DynamoDBAttributeValue) (any, error) {
	switch attr.DataType() {
	case events.DataTypeString:
		return attr.String(), nil
	case events.DataTypeNumber:
		return attr.Number(), nil
	case events.DataTypeBoolean:
		return attr.Boolean(), nil
	case events.DataTypeMap:
		m := make(map[string]any)
		for k, v := range attr.Map() {
			converted, err := convertAttributeValue(v)
			if err != nil {
				return nil, err
			}
			m[k] = converted
		}
		return m, nil
	case events.DataTypeList:
		list := make([]any, len(attr.List()))
		for i, v := range attr.List() {
			converted, err := convertAttributeValue(v)
			if err != nil {
				return nil, err
			}
			list[i] = converted
		}
		return list, nil
	case events.DataTypeNull:
		return nil, nil
	case events.DataTypeBinary:
		return attr.Binary(), nil
	default:
		return nil, fmt.Errorf("unsupported attribute type: %v", attr.DataType())
	}
}
