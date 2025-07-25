package stream

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/common"
)

// UnmarshalItem unmarshals a single DynamoDB stream record into a struct
func UnmarshalItem(record events.DynamoDBEventRecord, out interface{}) error {
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
				switch fieldVal.Kind() {
				case reflect.String:
					if strVal, ok := val.(string); ok {
						fieldVal.SetString(strVal)
					}
				case reflect.Int, reflect.Int64:
					if numStr, ok := val.(string); ok {
						if num, err := strconv.ParseInt(numStr, 10, 64); err == nil {
							fieldVal.SetInt(num)
						}
					}
				case reflect.Bool:
					if boolVal, ok := val.(bool); ok {
						fieldVal.SetBool(boolVal)
					}
				}
			}
		}
	}

	return nil
}

// UnmarshalItems unmarshals multiple DynamoDB stream records into a slice of structs
func UnmarshalItems(records []events.DynamoDBEventRecord, outType interface{}) (interface{}, error) {
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
func convertStreamImageToItem(image map[string]events.DynamoDBAttributeValue) (map[string]interface{}, error) {
	item := make(map[string]interface{})

	for key, value := range image {
		converted, err := convertAttributeValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to convert attribute %s: %w", key, err)
		}
		item[key] = converted
	}

	return item, nil
}

// Helper function to convert a single DynamoDB attribute value to a Go type
func convertAttributeValue(attr events.DynamoDBAttributeValue) (interface{}, error) {
	switch attr.DataType() {
	case events.DataTypeString:
		return attr.String(), nil
	case events.DataTypeNumber:
		return attr.Number(), nil
	case events.DataTypeBoolean:
		return attr.Boolean(), nil
	case events.DataTypeMap:
		m := make(map[string]interface{})
		for k, v := range attr.Map() {
			converted, err := convertAttributeValue(v)
			if err != nil {
				return nil, err
			}
			m[k] = converted
		}
		return m, nil
	case events.DataTypeList:
		list := make([]interface{}, len(attr.List()))
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
