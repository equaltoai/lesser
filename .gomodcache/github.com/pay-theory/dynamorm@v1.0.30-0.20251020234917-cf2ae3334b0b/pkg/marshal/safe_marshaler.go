// Package marshal provides safe marshaling for DynamoDB without unsafe operations
package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
)

// SafeMarshaler provides memory-safe marshaling implementation without unsafe operations
// This is the default marshaler and should be used in production environments
type SafeMarshaler struct {
	// Cache for reflection metadata to optimize performance
	cache sync.Map // map[reflect.Type]*safeStructMarshaler
}

// safeStructMarshaler contains cached reflection information for a struct type
type safeStructMarshaler struct {
	fields    []safeFieldMarshaler
	minFields int // Pre-calculated number of non-omitempty fields for better allocation
}

// safeFieldMarshaler contains cached information for marshaling a struct field
type safeFieldMarshaler struct {
	fieldIndex  []int // Field index path for nested fields
	dbName      string
	typ         reflect.Type
	omitEmpty   bool
	isSet       bool
	isCreatedAt bool
	isUpdatedAt bool
	isVersion   bool
	isTTL       bool
}

// NewSafeMarshaler creates a new safe marshaler (recommended for production)
func NewSafeMarshaler() *SafeMarshaler {
	return &SafeMarshaler{}
}

// MarshalItem safely marshals a model to DynamoDB AttributeValues using only reflection
// This implementation prioritizes security over performance but is still highly optimized
func (m *SafeMarshaler) MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("cannot marshal nil pointer")
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("model must be a struct or pointer to struct")
	}

	// Get or create cached marshaler
	typ := v.Type()
	cached, ok := m.cache.Load(typ)
	if !ok {
		sm := m.buildSafeStructMarshaler(typ, metadata)
		cached, _ = m.cache.LoadOrStore(typ, sm)
	}

	sm := cached.(*safeStructMarshaler)

	// Pre-allocate result map with estimated size
	result := make(map[string]types.AttributeValue, sm.minFields)

	// Pre-calculate timestamps once if needed
	var nowStr string
	hasTimestamps := false
	for _, fm := range sm.fields {
		if fm.isCreatedAt || fm.isUpdatedAt {
			hasTimestamps = true
			break
		}
	}
	if hasTimestamps {
		nowStr = time.Now().Format(time.RFC3339Nano)
	}

	// Marshal each field using safe reflection
	for _, fm := range sm.fields {
		// Handle special fields that don't require field access
		if fm.isCreatedAt || fm.isUpdatedAt {
			result[fm.dbName] = &types.AttributeValueMemberS{Value: nowStr}
			continue
		}

		// Get field value safely using reflection
		field := v.FieldByIndex(fm.fieldIndex)

		// Handle version field specially
		if fm.isVersion {
			if field.Kind() == reflect.Int64 {
				val := field.Int()
				if val == 0 {
					result[fm.dbName] = &types.AttributeValueMemberN{Value: "0"}
				} else {
					result[fm.dbName] = &types.AttributeValueMemberN{Value: strconv.FormatInt(val, 10)}
				}
			}
			continue
		}

		// Marshal the field value safely
		av, err := m.marshalValue(field, &fm)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", fm.dbName, err)
		}

		// Skip NULL values if omitempty
		if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fm.omitEmpty {
			continue
		}

		result[fm.dbName] = av
	}

	return result, nil
}

// buildSafeStructMarshaler builds a cached marshaler for a struct type using safe reflection
func (m *SafeMarshaler) buildSafeStructMarshaler(typ reflect.Type, metadata *model.Metadata) *safeStructMarshaler {
	sm := &safeStructMarshaler{
		fields:    make([]safeFieldMarshaler, 0, len(metadata.Fields)),
		minFields: 0,
	}

	for _, fieldMeta := range metadata.Fields {
		// Count non-omitempty fields for better allocation
		if !fieldMeta.OmitEmpty || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt || fieldMeta.IsVersion {
			sm.minFields++
		}

		// Get field information safely
		field, ok := typ.FieldByName(fieldMeta.Name)
		if !ok {
			// Try by index if name lookup fails
			if fieldMeta.Index < typ.NumField() {
				field = typ.Field(fieldMeta.Index)
			} else {
				continue // Skip invalid fields
			}
		}

		fm := safeFieldMarshaler{
			fieldIndex:  fieldMeta.IndexPath,
			dbName:      fieldMeta.DBName,
			typ:         field.Type,
			omitEmpty:   fieldMeta.OmitEmpty,
			isSet:       fieldMeta.IsSet,
			isCreatedAt: fieldMeta.IsCreatedAt,
			isUpdatedAt: fieldMeta.IsUpdatedAt,
			isVersion:   fieldMeta.IsVersion,
			isTTL:       fieldMeta.IsTTL,
		}

		sm.fields = append(sm.fields, fm)
	}

	return sm
}

// marshalValue safely marshals a reflect.Value to AttributeValue
func (m *SafeMarshaler) marshalValue(v reflect.Value, fieldMeta *safeFieldMarshaler) (types.AttributeValue, error) {
	// Handle nil pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		v = v.Elem()
	}

	// Check for zero values with omitempty
	if fieldMeta.omitEmpty && v.IsZero() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v.Uint(), 10)}, nil

	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 64)}, nil

	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil

	case reflect.Struct:
		// Special handling for time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t := v.Interface().(time.Time)
			if fieldMeta.isTTL {
				return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}, nil
			}
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}
		// For other structs, marshal as a map
		return m.marshalStruct(v)

	case reflect.Slice:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		// Handle string sets
		if v.Type().Elem().Kind() == reflect.String && fieldMeta.isSet {
			strings := make([]string, v.Len())
			for i := 0; i < v.Len(); i++ {
				strings[i] = v.Index(i).String()
			}
			return &types.AttributeValueMemberSS{Value: strings}, nil
		}
		// Handle regular lists
		return m.marshalSlice(v)

	case reflect.Map:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return m.marshalMap(v)

	case reflect.Interface:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return m.marshalValue(v.Elem(), fieldMeta)

	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

// marshalSlice safely marshals a slice
func (m *SafeMarshaler) marshalSlice(v reflect.Value) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		elem, err := m.marshalValue(v.Index(i), &safeFieldMarshaler{})
		if err != nil {
			return nil, fmt.Errorf("slice index %d: %w", i, err)
		}
		list[i] = elem
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

// marshalMap safely marshals a map
func (m *SafeMarshaler) marshalMap(v reflect.Value) (types.AttributeValue, error) {
	avMap := make(map[string]types.AttributeValue, v.Len())
	for _, key := range v.MapKeys() {
		keyStr := fmt.Sprintf("%v", key.Interface())
		val, err := m.marshalValue(v.MapIndex(key), &safeFieldMarshaler{})
		if err != nil {
			return nil, fmt.Errorf("map key %s: %w", keyStr, err)
		}
		avMap[keyStr] = val
	}
	return &types.AttributeValueMemberM{Value: avMap}, nil
}

// marshalStruct safely marshals a struct as a map
func (m *SafeMarshaler) marshalStruct(v reflect.Value) (types.AttributeValue, error) {
	structMap := make(map[string]types.AttributeValue)
	typ := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		// Skip unexported fields for security
		if !field.IsExported() {
			continue
		}

		fieldValue := v.Field(i)
		// Skip zero values for omitempty behavior
		if fieldValue.IsZero() {
			continue
		}

		av, err := m.marshalValue(fieldValue, &safeFieldMarshaler{})
		if err != nil {
			return nil, fmt.Errorf("struct field %s: %w", field.Name, err)
		}

		structMap[field.Name] = av
	}
	return &types.AttributeValueMemberM{Value: structMap}, nil
}
