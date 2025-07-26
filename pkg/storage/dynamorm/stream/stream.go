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
	case "INSERT", "MODIFY":
		image = record.Change.NewImage
	case "REMOVE":
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
		if record.EventName != "INSERT" && record.EventName != "MODIFY" && record.EventName != "REMOVE" {
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

	fieldType := field.Type()
	valueType := reflect.TypeOf(value)

	// Direct assignment if types match
	if valueType.AssignableTo(fieldType) {
		field.Set(reflect.ValueOf(value))
		return nil
	}

	// Handle type conversions
	switch field.Kind() {
	case reflect.String:
		if str, ok := value.(string); ok {
			field.SetString(str)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
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
	case reflect.Float32, reflect.Float64:
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
	case reflect.Bool:
		if b, ok := value.(bool); ok {
			field.SetBool(b)
			return nil
		}
	case reflect.Slice:
		// Handle slice types
		if list, ok := value.([]any); ok {
			sliceVal := reflect.MakeSlice(fieldType, len(list), len(list))
			for i, item := range list {
				if err := setFieldValue(sliceVal.Index(i), item); err != nil {
					return err
				}
			}
			field.Set(sliceVal)
			return nil
		}
	case reflect.Map:
		// Handle map types
		if m, ok := value.(map[string]any); ok {
			mapVal := reflect.MakeMap(fieldType)
			for k, v := range m {
				keyVal := reflect.ValueOf(k)
				valVal := reflect.New(fieldType.Elem()).Elem()
				if err := setFieldValue(valVal, v); err != nil {
					return err
				}
				mapVal.SetMapIndex(keyVal, valVal)
			}
			field.Set(mapVal)
			return nil
		}
	}

	return fmt.Errorf("cannot convert %v (type %T) to %v", value, value, fieldType)
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
