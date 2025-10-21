// Package marshal provides optimized marshaling for DynamoDB
package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
)

// Marshaler provides high-performance marshaling to DynamoDB AttributeValues
type Marshaler struct {
	// Cache for struct marshalers
	cache sync.Map // map[reflect.Type]*structMarshaler
	// Optional custom converter registry shared with DB
	converter *pkgTypes.Converter
}

// structMarshaler contains cached information for marshaling a specific struct type
type structMarshaler struct {
	fields []fieldMarshaler
	// Pre-calculated number of non-omitempty fields for better allocation
	minFields int
}

// fieldMarshaler contains cached information for marshaling a struct field
type fieldMarshaler struct {
	index       int
	dbName      string
	offset      uintptr
	typ         reflect.Type
	omitEmpty   bool
	isSet       bool
	isCreatedAt bool
	isUpdatedAt bool
	isVersion   bool
	isTTL       bool
	marshalFunc func(unsafe.Pointer) (types.AttributeValue, error)
}

// New creates a new optimized marshaler. If a converter is provided it will
// be consulted for custom type conversions during marshaling.
func New(converter *pkgTypes.Converter) *Marshaler {
	return &Marshaler{
		converter: converter,
	}
}

// ClearCache removes all cached struct marshalers. This is useful when new
// custom converters are registered and previously compiled functions need to
// be rebuilt.
func (m *Marshaler) ClearCache() {
	if m == nil {
		return
	}

	m.cache.Range(func(key, _ any) bool {
		m.cache.Delete(key)
		return true
	})
}

// MarshalItem marshals a model to DynamoDB AttributeValues using cached reflection
func (m *Marshaler) MarshalItem(model any, metadata *model.Metadata) (map[string]types.AttributeValue, error) {
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
		sm := m.buildStructMarshaler(typ, metadata)
		cached, _ = m.cache.LoadOrStore(typ, sm)
	}

	sm := cached.(*structMarshaler)

	// Pre-allocate result map with estimated size
	result := make(map[string]types.AttributeValue, sm.minFields)

	// Get pointer to struct data
	// If the value is not addressable, we need to make a copy
	var ptr unsafe.Pointer
	if v.CanAddr() {
		ptr = unsafe.Pointer(v.UnsafeAddr())
	} else {
		// Create an addressable copy
		vcopy := reflect.New(v.Type()).Elem()
		vcopy.Set(v)
		ptr = unsafe.Pointer(vcopy.UnsafeAddr())
	}

	// Pre-calculate timestamps once
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

	// Marshal each field using cached information
	for _, fm := range sm.fields {
		// Handle special fields
		if fm.isCreatedAt || fm.isUpdatedAt {
			result[fm.dbName] = &types.AttributeValueMemberS{Value: nowStr}
			continue
		}

		if fm.isVersion {
			// Get current value
			fieldPtr := unsafe.Add(ptr, fm.offset)
			val := *(*int64)(fieldPtr)
			if val == 0 {
				result[fm.dbName] = &types.AttributeValueMemberN{Value: "0"}
			} else {
				result[fm.dbName] = &types.AttributeValueMemberN{Value: strconv.FormatInt(val, 10)}
			}
			continue
		}

		// Use the pre-compiled marshal function
		if fm.marshalFunc != nil {
			fieldPtr := unsafe.Add(ptr, fm.offset)
			av, err := fm.marshalFunc(fieldPtr)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", fm.dbName, err)
			}

			// Skip NULL values if omitempty
			if _, isNull := av.(*types.AttributeValueMemberNULL); isNull && fm.omitEmpty {
				continue
			}

			result[fm.dbName] = av
		}
	}

	return result, nil
}

// buildStructMarshaler builds a cached marshaler for a struct type
func (m *Marshaler) buildStructMarshaler(typ reflect.Type, metadata *model.Metadata) *structMarshaler {
	sm := &structMarshaler{
		fields:    make([]fieldMarshaler, 0, len(metadata.Fields)),
		minFields: 0,
	}

	for _, fieldMeta := range metadata.Fields {
		field := typ.FieldByIndex(fieldMeta.IndexPath)

		// Count non-omitempty fields
		if !fieldMeta.OmitEmpty || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt || fieldMeta.IsVersion {
			sm.minFields++
		}

		fm := fieldMarshaler{
			index:       fieldMeta.Index,
			dbName:      fieldMeta.DBName,
			offset:      field.Offset,
			typ:         field.Type,
			omitEmpty:   fieldMeta.OmitEmpty,
			isSet:       fieldMeta.IsSet,
			isCreatedAt: fieldMeta.IsCreatedAt,
			isUpdatedAt: fieldMeta.IsUpdatedAt,
			isVersion:   fieldMeta.IsVersion,
			isTTL:       fieldMeta.IsTTL,
		}

		// Prefer registered custom converters when available so callers can
		// override marshalling behavior for specific types.
		if m.hasCustomConverter(field.Type) {
			fm.marshalFunc = m.buildCustomConverterMarshalFunc(field.Type)
			sm.fields = append(sm.fields, fm)
			continue
		}

		// Build type-specific marshal function
		fm.marshalFunc = m.buildMarshalFunc(field.Type, fieldMeta)

		sm.fields = append(sm.fields, fm)
	}

	return sm
}

// buildMarshalFunc builds a type-specific marshal function
func (m *Marshaler) buildMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	// Handle pointer types
	if typ.Kind() == reflect.Ptr {
		elemFunc := m.buildMarshalFunc(typ.Elem(), fieldMeta)
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			p := *(*unsafe.Pointer)(ptr)
			if p == nil {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return elemFunc(p)
		}
	}

	// Fast paths for common types
	switch typ.Kind() {
	case reflect.String:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			s := *(*string)(ptr)
			if s == "" && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberS{Value: s}, nil
		}

	case reflect.Int:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			i := *(*int)(ptr)
			if i == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberN{Value: strconv.Itoa(i)}, nil
		}

	case reflect.Int64:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			i := *(*int64)(ptr)
			if i == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberN{Value: strconv.FormatInt(i, 10)}, nil
		}

	case reflect.Float64:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			f := *(*float64)(ptr)
			if f == 0 && fieldMeta.OmitEmpty {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberN{Value: strconv.FormatFloat(f, 'f', -1, 64)}, nil
		}

	case reflect.Bool:
		return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
			b := *(*bool)(ptr)
			return &types.AttributeValueMemberBOOL{Value: b}, nil
		}

	case reflect.Struct:
		// Special handling for time.Time
		if typ == reflect.TypeOf(time.Time{}) {
			return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
				t := *(*time.Time)(ptr)
				if t.IsZero() && fieldMeta.OmitEmpty {
					return &types.AttributeValueMemberNULL{Value: true}, nil
				}
				if fieldMeta.IsTTL {
					return &types.AttributeValueMemberN{Value: strconv.FormatInt(t.Unix(), 10)}, nil
				}
				return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
			}
		}
		// For other structs, fall back to reflection
		return m.buildReflectMarshalFunc(typ, fieldMeta)

	case reflect.Slice:
		// Handle []string specially for sets
		if typ.Elem().Kind() == reflect.String && fieldMeta.IsSet {
			return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
				// Use reflection for slices as unsafe doesn't work well with them
				slice := (*[]string)(ptr)
				if len(*slice) == 0 && fieldMeta.OmitEmpty {
					return &types.AttributeValueMemberNULL{Value: true}, nil
				}
				return &types.AttributeValueMemberSS{Value: *slice}, nil
			}
		}
		// Handle regular []string as list
		if typ.Elem().Kind() == reflect.String && !fieldMeta.IsSet {
			return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
				// Direct access to string slice
				slice := *(*[]string)(ptr)
				if len(slice) == 0 && fieldMeta.OmitEmpty {
					return &types.AttributeValueMemberNULL{Value: true}, nil
				}
				// Pre-allocate list
				list := make([]types.AttributeValue, len(slice))
				for i, s := range slice {
					list[i] = &types.AttributeValueMemberS{Value: s}
				}
				return &types.AttributeValueMemberL{Value: list}, nil
			}
		}
		// Fall back to reflection for other slices
		return m.buildReflectMarshalFunc(typ, fieldMeta)

	case reflect.Map:
		// Handle map[string]string specially
		if typ.Key().Kind() == reflect.String && typ.Elem().Kind() == reflect.String {
			return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
				// Use reflection for maps as it's complex with unsafe
				v := reflect.NewAt(typ, ptr).Elem()
				if v.IsNil() && fieldMeta.OmitEmpty {
					return &types.AttributeValueMemberNULL{Value: true}, nil
				}

				// Pre-allocate map
				avMap := make(map[string]types.AttributeValue, v.Len())
				for _, key := range v.MapKeys() {
					keyStr := key.String()
					val := v.MapIndex(key).String()
					avMap[keyStr] = &types.AttributeValueMemberS{Value: val}
				}
				return &types.AttributeValueMemberM{Value: avMap}, nil
			}
		}
		// Fall back to reflection for other maps
		return m.buildReflectMarshalFunc(typ, fieldMeta)

	default:
		// Fall back to reflection for complex types
		return m.buildReflectMarshalFunc(typ, fieldMeta)
	}
}

// buildReflectMarshalFunc builds a reflection-based marshal function for complex types
func (m *Marshaler) buildReflectMarshalFunc(typ reflect.Type, fieldMeta *model.FieldMetadata) func(unsafe.Pointer) (types.AttributeValue, error) {
	return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
		// Convert unsafe pointer back to reflect.Value
		v := reflect.NewAt(typ, ptr).Elem()

		if fieldMeta.OmitEmpty && v.IsZero() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}

		// Handle complex types with optimized paths
		return m.marshalComplexValue(v)
	}
}

// marshalComplexValue handles complex types that can't use unsafe optimizations
func (m *Marshaler) marshalComplexValue(v reflect.Value) (types.AttributeValue, error) {
	if av, ok, err := m.marshalUsingCustomConverter(v); ok {
		return av, err
	}

	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}

		// Pre-allocate list
		list := make([]types.AttributeValue, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			av, err := m.marshalValue(elem)
			if err != nil {
				return nil, fmt.Errorf("slice index %d: %w", i, err)
			}
			list[i] = av
		}
		return &types.AttributeValueMemberL{Value: list}, nil

	case reflect.Map:
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}

		// Pre-allocate map
		avMap := make(map[string]types.AttributeValue, v.Len())
		for _, key := range v.MapKeys() {
			keyStr := key.String()
			val := v.MapIndex(key)
			av, err := m.marshalValue(val)
			if err != nil {
				return nil, fmt.Errorf("map key %s: %w", keyStr, err)
			}
			avMap[keyStr] = av
		}
		return &types.AttributeValueMemberM{Value: avMap}, nil

	case reflect.Struct:
		// Special handling for time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t := v.Interface().(time.Time)
			if t.IsZero() {
				return &types.AttributeValueMemberNULL{Value: true}, nil
			}
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}

		// For other structs, marshal as a map
		structMap := make(map[string]types.AttributeValue)
		typ := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := typ.Field(i)
			// Skip unexported fields
			if field.PkgPath != "" {
				continue
			}

			fieldValue := v.Field(i)
			// Skip zero values for omitempty behavior
			if fieldValue.IsZero() {
				continue
			}

			av, err := m.marshalValue(fieldValue)
			if err != nil {
				return nil, fmt.Errorf("struct field %s: %w", field.Name, err)
			}

			// Use field name as key (could be enhanced with tag parsing)
			structMap[field.Name] = av
		}
		return &types.AttributeValueMemberM{Value: structMap}, nil

	default:
		// For other types, use basic marshaling
		return m.marshalValue(v)
	}
}

// marshalValue marshals a single reflect.Value
func (m *Marshaler) marshalValue(v reflect.Value) (types.AttributeValue, error) {
	if !v.IsValid() {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	// Handle nil pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		if av, ok, err := m.marshalUsingCustomConverter(v); ok {
			return av, err
		}
		v = v.Elem()
	}

	if av, ok, err := m.marshalUsingCustomConverter(v); ok {
		return av, err
	}

	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil
	case reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v.Uint(), 10)}, nil
	case reflect.Float32:
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 32)}, nil
	case reflect.Float64:
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 64)}, nil
	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil
	case reflect.Struct:
		// Handle structs through marshalComplexValue which has struct handling
		return m.marshalComplexValue(v)
	case reflect.Slice, reflect.Map:
		// Handle slices and maps through marshalComplexValue
		return m.marshalComplexValue(v)
	case reflect.Interface:
		// Handle interface by getting the concrete value
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		if av, ok, err := m.marshalUsingCustomConverter(v.Elem()); ok {
			return av, err
		}
		return m.marshalValue(v.Elem())
	default:
		// For unsupported types, return an error instead of recursing
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func (m *Marshaler) buildCustomConverterMarshalFunc(typ reflect.Type) func(unsafe.Pointer) (types.AttributeValue, error) {
	return func(ptr unsafe.Pointer) (types.AttributeValue, error) {
		if m.converter == nil {
			return nil, fmt.Errorf("no converter configured for type %s", typ)
		}
		value := reflect.NewAt(typ, ptr).Elem()
		return m.converter.ToAttributeValue(value.Interface())
	}
}

func (m *Marshaler) hasCustomConverter(typ reflect.Type) bool {
	if m == nil || m.converter == nil {
		return false
	}
	return m.converter.HasCustomConverter(typ)
}

func (m *Marshaler) marshalUsingCustomConverter(v reflect.Value) (types.AttributeValue, bool, error) {
	if m == nil || m.converter == nil {
		return nil, false, nil
	}
	if !m.converter.HasCustomConverter(v.Type()) {
		return nil, false, nil
	}
	av, err := m.converter.ToAttributeValue(v.Interface())
	if err != nil {
		return nil, false, err
	}
	return av, true, nil
}
