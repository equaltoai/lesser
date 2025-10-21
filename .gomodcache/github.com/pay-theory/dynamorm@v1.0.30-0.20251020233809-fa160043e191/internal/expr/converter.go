package expr

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Marshaler interface for custom marshaling
type Marshaler interface {
	MarshalDynamoDBAttributeValue() (types.AttributeValue, error)
}

// Unmarshaler interface for custom unmarshaling
type Unmarshaler interface {
	UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error
}

// ConvertToAttributeValue converts a Go value to a DynamoDB AttributeValue
func ConvertToAttributeValue(value any) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Check for custom marshaler
	if marshaler, ok := value.(Marshaler); ok {
		return marshaler.MarshalDynamoDBAttributeValue()
	}

	v := reflect.ValueOf(value)

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return ConvertToAttributeValue(v.Elem().Interface())
	}

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Int())}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Uint())}, nil

	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%g", v.Float())}, nil

	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil

	case reflect.Slice:
		// Handle []byte as binary
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return &types.AttributeValueMemberB{Value: v.Bytes()}, nil
		}

		// Handle other slices as lists
		list := make([]types.AttributeValue, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := ConvertToAttributeValue(v.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			list[i] = item
		}
		return &types.AttributeValueMemberL{Value: list}, nil

	case reflect.Map:
		// Handle map[string]any as M type
		if v.Type().Key().Kind() == reflect.String {
			m := make(map[string]types.AttributeValue)
			for _, key := range v.MapKeys() {
				val, err := ConvertToAttributeValue(v.MapIndex(key).Interface())
				if err != nil {
					return nil, err
				}
				m[key.String()] = val
			}
			return &types.AttributeValueMemberM{Value: m}, nil
		}
		return nil, fmt.Errorf("unsupported map type: %v", v.Type())

	case reflect.Struct:
		// Special handling for time.Time
		if t, ok := value.(time.Time); ok {
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}

		// Handle JSON marshaling for structs with json tag
		if hasJSONTag(v.Type()) {
			data, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal struct to JSON: %w", err)
			}
			return &types.AttributeValueMemberS{Value: string(data)}, nil
		}

		// General struct marshaling
		m := make(map[string]types.AttributeValue)
		t := v.Type()

		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			fieldValue := v.Field(i)

			// Skip unexported fields
			if !field.IsExported() {
				continue
			}

			// Check for dynamorm tags
			fieldName := field.Name
			tag := field.Tag.Get("dynamorm")
			if tag == "-" {
				continue // Skip this field
			}
			if tag != "" {
				// Parse the tag
				parts := strings.Split(tag, ",")
				if len(parts) > 0 && parts[0] != "" {
					// First part is the field name unless it contains ":" or is purely a modifier
					firstPart := parts[0]
					if !strings.Contains(firstPart, ":") && !isPureModifierTag(firstPart) {
						fieldName = firstPart
					}
					// Check for attr: tag
					if attrName := parseAttrTag(tag); attrName != "" {
						fieldName = attrName
					}
				}
			}

			// Skip zero values if omitempty is set
			if hasOmitEmpty(tag) && isZeroValue(fieldValue) {
				continue
			}

			// Convert field value
			av, err := ConvertToAttributeValue(fieldValue.Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to convert field %s: %w", field.Name, err)
			}

			m[fieldName] = av
		}

		return &types.AttributeValueMemberM{Value: m}, nil

	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Type())
	}
}

// ConvertFromAttributeValue converts a DynamoDB AttributeValue to a Go value
func ConvertFromAttributeValue(av types.AttributeValue, target any) error {
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}

	// Check for custom unmarshaler
	if unmarshaler, ok := target.(Unmarshaler); ok {
		return unmarshaler.UnmarshalDynamoDBAttributeValue(av)
	}

	targetElem := targetValue.Elem()
	return unmarshalAttributeValue(av, targetElem)
}

// unmarshalAttributeValue unmarshals an AttributeValue into a reflect.Value
func unmarshalAttributeValue(av types.AttributeValue, v reflect.Value) error {
	// Handle any / any types
	if v.Kind() == reflect.Interface && v.Type().NumMethod() == 0 {
		// This is an empty interface (any or any)
		val, err := attributeValueToInterface(av)
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(val))
		return nil
	}

	// Handle pointer types
	if v.Kind() == reflect.Ptr {
		if av == nil || isNullAttributeValue(av) {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		// Create new value if pointer is nil
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return unmarshalAttributeValue(av, v.Elem())
	}

	switch av := av.(type) {
	case *types.AttributeValueMemberS:
		return unmarshalString(av.Value, v)

	case *types.AttributeValueMemberN:
		return unmarshalNumber(av.Value, v)

	case *types.AttributeValueMemberB:
		return unmarshalBinary(av.Value, v)

	case *types.AttributeValueMemberBOOL:
		return unmarshalBool(av.Value, v)

	case *types.AttributeValueMemberNULL:
		v.Set(reflect.Zero(v.Type()))
		return nil

	case *types.AttributeValueMemberL:
		return unmarshalList(av.Value, v)

	case *types.AttributeValueMemberM:
		return unmarshalMap(av.Value, v)

	case *types.AttributeValueMemberSS:
		return unmarshalStringSet(av.Value, v)

	case *types.AttributeValueMemberNS:
		return unmarshalNumberSet(av.Value, v)

	case *types.AttributeValueMemberBS:
		return unmarshalBinarySet(av.Value, v)

	default:
		return fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}

// unmarshalString unmarshals a string value
func unmarshalString(s string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
		return nil

	case reflect.Struct:
		// Special handling for time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				// Try other common formats
				t, err = time.Parse(time.RFC3339, s)
				if err != nil {
					return fmt.Errorf("failed to parse time: %w", err)
				}
			}
			v.Set(reflect.ValueOf(t))
			return nil
		}

		// Handle JSON unmarshaling for structs
		if hasJSONTag(v.Type()) {
			return json.Unmarshal([]byte(s), v.Addr().Interface())
		}

		return fmt.Errorf("cannot unmarshal string into %v", v.Type())

	default:
		return fmt.Errorf("cannot unmarshal string into %v", v.Type())
	}
}

// unmarshalNumber unmarshals a number value
func unmarshalNumber(n string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(n, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
		return nil

	default:
		return fmt.Errorf("cannot unmarshal number into %v", v.Type())
	}
}

// unmarshalBinary unmarshals binary data
func unmarshalBinary(b []byte, v reflect.Value) error {
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
		v.SetBytes(b)
		return nil
	}
	return fmt.Errorf("cannot unmarshal binary into %v", v.Type())
}

// unmarshalBool unmarshals a boolean value
func unmarshalBool(b bool, v reflect.Value) error {
	if v.Kind() == reflect.Bool {
		v.SetBool(b)
		return nil
	}
	return fmt.Errorf("cannot unmarshal bool into %v", v.Type())
}

// unmarshalList unmarshals a list of AttributeValues
func unmarshalList(list []types.AttributeValue, v reflect.Value) error {
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal list into %v", v.Type())
	}

	// Create new slice
	slice := reflect.MakeSlice(v.Type(), len(list), len(list))

	// Unmarshal each element
	for i, item := range list {
		if err := unmarshalAttributeValue(item, slice.Index(i)); err != nil {
			return fmt.Errorf("failed to unmarshal list item %d: %w", i, err)
		}
	}

	v.Set(slice)
	return nil
}

// unmarshalMap unmarshals a map of AttributeValues
func unmarshalMap(m map[string]types.AttributeValue, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Map:
		// Ensure map is string-keyed
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("map must have string keys")
		}

		// Create new map if nil
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}

		// Unmarshal each value
		for key, value := range m {
			mapValue := reflect.New(v.Type().Elem()).Elem()
			if err := unmarshalAttributeValue(value, mapValue); err != nil {
				return fmt.Errorf("failed to unmarshal map value for key %s: %w", key, err)
			}
			v.SetMapIndex(reflect.ValueOf(key), mapValue)
		}
		return nil

	case reflect.Struct:
		// Unmarshal map into struct fields
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}

			// Get field name from tag or use field name
			fieldName := field.Name
			tag := field.Tag.Get("dynamorm")
			if tag != "" && tag != "-" {
				// Parse the tag
				parts := strings.Split(tag, ",")
				if len(parts) > 0 && parts[0] != "" {
					// First part is the field name unless it contains ":" or is purely a modifier
					firstPart := parts[0]
					if !strings.Contains(firstPart, ":") && !isPureModifierTag(firstPart) {
						fieldName = firstPart
					}
					// Check for attr: tag
					if attrName := parseAttrTag(tag); attrName != "" {
						fieldName = attrName
					}
				}
			}

			// Look for matching attribute
			if av, ok := m[fieldName]; ok {
				if err := unmarshalAttributeValue(av, v.Field(i)); err != nil {
					return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("cannot unmarshal map into %v", v.Type())
	}
}

// unmarshalStringSet unmarshals a string set
func unmarshalStringSet(ss []string, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal string set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(ss), len(ss))
	for i, s := range ss {
		slice.Index(i).SetString(s)
	}
	v.Set(slice)
	return nil
}

// unmarshalNumberSet unmarshals a number set
func unmarshalNumberSet(ns []string, v reflect.Value) error {
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal number set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(ns), len(ns))
	for i, n := range ns {
		if err := unmarshalNumber(n, slice.Index(i)); err != nil {
			return fmt.Errorf("failed to unmarshal number set item %d: %w", i, err)
		}
	}
	v.Set(slice)
	return nil
}

// unmarshalBinarySet unmarshals a binary set
func unmarshalBinarySet(bs [][]byte, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal binary set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(bs), len(bs))
	for i, b := range bs {
		slice.Index(i).SetBytes(b)
	}
	v.Set(slice)
	return nil
}

// Helper functions

func isNullAttributeValue(av types.AttributeValue) bool {
	if nullAV, ok := av.(*types.AttributeValueMemberNULL); ok {
		return nullAV.Value
	}
	return false
}

func hasJSONTag(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if tag := t.Field(i).Tag.Get("json"); tag != "" {
			return true
		}
	}
	return false
}

func parseAttrTag(tag string) string {
	// Parse "attr:name" from tag
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "attr:") {
			return strings.TrimPrefix(part, "attr:")
		}
	}
	return ""
}

func hasOmitEmpty(tag string) bool {
	return strings.Contains(tag, "omitempty")
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func isPureModifierTag(tag string) bool {
	// These are tags that are ONLY modifiers and never field names
	modifiers := []string{"pk", "sk", "version", "ttl", "set", "omitempty", "binary", "json", "encrypted"}
	for _, mod := range modifiers {
		if tag == mod {
			return true
		}
	}
	return false
}

// attributeValueToInterface converts an AttributeValue to a native Go type
func attributeValueToInterface(av types.AttributeValue) (any, error) {
	switch av := av.(type) {
	case *types.AttributeValueMemberS:
		return av.Value, nil

	case *types.AttributeValueMemberN:
		// Try to parse as int first, then float
		if i, err := strconv.ParseInt(av.Value, 10, 64); err == nil {
			return i, nil
		}
		if f, err := strconv.ParseFloat(av.Value, 64); err == nil {
			return f, nil
		}
		return nil, fmt.Errorf("cannot parse number: %s", av.Value)

	case *types.AttributeValueMemberB:
		return av.Value, nil

	case *types.AttributeValueMemberBOOL:
		return av.Value, nil

	case *types.AttributeValueMemberNULL:
		return nil, nil

	case *types.AttributeValueMemberL:
		list := make([]any, len(av.Value))
		for i, item := range av.Value {
			val, err := attributeValueToInterface(item)
			if err != nil {
				return nil, fmt.Errorf("failed to convert list item %d: %w", i, err)
			}
			list[i] = val
		}
		return list, nil

	case *types.AttributeValueMemberM:
		m := make(map[string]any)
		for k, v := range av.Value {
			val, err := attributeValueToInterface(v)
			if err != nil {
				return nil, fmt.Errorf("failed to convert map value for key %s: %w", k, err)
			}
			m[k] = val
		}
		return m, nil

	case *types.AttributeValueMemberSS:
		return av.Value, nil

	case *types.AttributeValueMemberNS:
		// Convert number set to slice of numbers
		nums := make([]any, len(av.Value))
		for i, n := range av.Value {
			if intVal, err := strconv.ParseInt(n, 10, 64); err == nil {
				nums[i] = intVal
			} else if f, err := strconv.ParseFloat(n, 64); err == nil {
				nums[i] = f
			} else {
				return nil, fmt.Errorf("cannot parse number in set: %s", n)
			}
		}
		return nums, nil

	case *types.AttributeValueMemberBS:
		// Convert binary set to slice of []byte
		return av.Value, nil

	default:
		return nil, fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}
