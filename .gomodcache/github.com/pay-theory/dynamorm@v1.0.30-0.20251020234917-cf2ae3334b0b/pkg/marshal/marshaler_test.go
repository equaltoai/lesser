package marshal

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test structures
type SimpleStruct struct {
	ID     string  `dynamodb:"id"`
	Name   string  `dynamodb:"name"`
	Age    int     `dynamodb:"age"`
	Score  float64 `dynamodb:"score"`
	Active bool    `dynamodb:"active"`
}

type ComplexStruct struct {
	ID            string            `dynamodb:"id"`
	Tags          []string          `dynamodb:"tags"`
	Attributes    map[string]string `dynamodb:"attributes"`
	CreatedAt     time.Time         `dynamodb:"created_at,createdAt"`
	UpdatedAt     time.Time         `dynamodb:"updated_at,updatedAt"`
	Version       int64             `dynamodb:"version,version"`
	TTL           time.Time         `dynamodb:"ttl,ttl"`
	OptionalField *string           `dynamodb:"optional,omitempty"`
	StringSet     []string          `dynamodb:"string_set,set"`
}

type PointerStruct struct {
	StringPtr  *string  `dynamodb:"string_ptr"`
	IntPtr     *int     `dynamodb:"int_ptr"`
	Float64Ptr *float64 `dynamodb:"float64_ptr"`
	BoolPtr    *bool    `dynamodb:"bool_ptr"`
}

type OmitEmptyStruct struct {
	Required string            `dynamodb:"required"`
	Optional string            `dynamodb:"optional,omitempty"`
	Number   int               `dynamodb:"number,omitempty"`
	Float    float64           `dynamodb:"float,omitempty"`
	SliceOE  []string          `dynamodb:"slice_oe,omitempty"`
	MapOE    map[string]string `dynamodb:"map_oe,omitempty"`
}

type AllTypesStruct struct {
	String   string            `dynamodb:"string"`
	Int      int               `dynamodb:"int"`
	Int64    int64             `dynamodb:"int64"`
	Float64  float64           `dynamodb:"float64"`
	Bool     bool              `dynamodb:"bool"`
	Time     time.Time         `dynamodb:"time"`
	StrSlice []string          `dynamodb:"str_slice"`
	StrMap   map[string]string `dynamodb:"str_map"`
}

type VersionedStruct struct {
	ID      string `dynamodb:"id"`
	Version int64  `dynamodb:"version,version"`
}

// Helper function to create field metadata
func createFieldMetadata(name, dbName string, index int, typ reflect.Type, opts ...func(*model.FieldMetadata)) *model.FieldMetadata {
	fm := &model.FieldMetadata{
		Name:      name,
		DBName:    dbName,
		Index:     index,
		IndexPath: []int{index}, // Add IndexPath for embedded struct support
		Type:      typ,
	}
	for _, opt := range opts {
		opt(fm)
	}
	return fm
}

// Helper options for field metadata
func withCreatedAt() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsCreatedAt = true }
}

func withUpdatedAt() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsUpdatedAt = true }
}

func withVersion() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsVersion = true }
}

func withTTL() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsTTL = true }
}

func withSet() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.IsSet = true }
}

func withOmitEmpty() func(*model.FieldMetadata) {
	return func(fm *model.FieldMetadata) { fm.OmitEmpty = true }
}

// Helper to create metadata
func createMetadata(fields ...*model.FieldMetadata) *model.Metadata {
	metadata := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
	}

	for _, f := range fields {
		metadata.Fields[f.Name] = f
		metadata.FieldsByDBName[f.DBName] = f
	}

	return metadata
}

func TestNew(t *testing.T) {
	marshaler := New()
	assert.NotNil(t, marshaler)
}

func TestMarshalItem_SimpleTypes(t *testing.T) {
	marshaler := New()

	tests := []struct {
		name     string
		input    interface{}
		metadata *model.Metadata
		expected map[string]types.AttributeValue
	}{
		{
			name: "simple struct with all fields",
			input: SimpleStruct{
				ID:     "test-id",
				Name:   "Test Name",
				Age:    30,
				Score:  98.5,
				Active: true,
			},
			metadata: createMetadata(
				createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
				createFieldMetadata("Name", "name", 1, reflect.TypeOf("")),
				createFieldMetadata("Age", "age", 2, reflect.TypeOf(0)),
				createFieldMetadata("Score", "score", 3, reflect.TypeOf(0.0)),
				createFieldMetadata("Active", "active", 4, reflect.TypeOf(false)),
			),
			expected: map[string]types.AttributeValue{
				"id":     &types.AttributeValueMemberS{Value: "test-id"},
				"name":   &types.AttributeValueMemberS{Value: "Test Name"},
				"age":    &types.AttributeValueMemberN{Value: "30"},
				"score":  &types.AttributeValueMemberN{Value: "98.5"},
				"active": &types.AttributeValueMemberBOOL{Value: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshaler.MarshalItem(tt.input, tt.metadata)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMarshalItem_ComplexTypes(t *testing.T) {
	marshaler := New()

	now := time.Now()
	optional := "optional-value"

	input := ComplexStruct{
		ID:            "complex-id",
		Tags:          []string{"tag1", "tag2", "tag3"},
		Attributes:    map[string]string{"key1": "value1", "key2": "value2"},
		CreatedAt:     now,
		UpdatedAt:     now,
		Version:       1,
		TTL:           now.Add(24 * time.Hour),
		OptionalField: &optional,
		StringSet:     []string{"set1", "set2"},
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Tags", "tags", 1, reflect.TypeOf([]string{})),
		createFieldMetadata("Attributes", "attributes", 2, reflect.TypeOf(map[string]string{})),
		createFieldMetadata("CreatedAt", "created_at", 3, reflect.TypeOf(time.Time{}), withCreatedAt()),
		createFieldMetadata("UpdatedAt", "updated_at", 4, reflect.TypeOf(time.Time{}), withUpdatedAt()),
		createFieldMetadata("Version", "version", 5, reflect.TypeOf(int64(0)), withVersion()),
		createFieldMetadata("TTL", "ttl", 6, reflect.TypeOf(time.Time{}), withTTL()),
		createFieldMetadata("OptionalField", "optional", 7, reflect.TypeOf(&optional), withOmitEmpty()),
		createFieldMetadata("StringSet", "string_set", 8, reflect.TypeOf([]string{}), withSet()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	// Check regular fields
	assert.Equal(t, "complex-id", result["id"].(*types.AttributeValueMemberS).Value)

	// Check list
	tagsList := result["tags"].(*types.AttributeValueMemberL).Value
	assert.Len(t, tagsList, 3)
	assert.Equal(t, "tag1", tagsList[0].(*types.AttributeValueMemberS).Value)

	// Check map
	attrMap := result["attributes"].(*types.AttributeValueMemberM).Value
	assert.Len(t, attrMap, 2)
	assert.Equal(t, "value1", attrMap["key1"].(*types.AttributeValueMemberS).Value)

	// Check timestamps (should be current time)
	createdAt := result["created_at"].(*types.AttributeValueMemberS).Value
	updatedAt := result["updated_at"].(*types.AttributeValueMemberS).Value
	assert.NotEmpty(t, createdAt)
	assert.NotEmpty(t, updatedAt)

	// Check version
	assert.Equal(t, "1", result["version"].(*types.AttributeValueMemberN).Value)

	// Check TTL (should be Unix timestamp)
	ttl := result["ttl"].(*types.AttributeValueMemberN).Value
	assert.NotEmpty(t, ttl)

	// Check optional field
	assert.Equal(t, "optional-value", result["optional"].(*types.AttributeValueMemberS).Value)

	// Check string set
	stringSet := result["string_set"].(*types.AttributeValueMemberSS).Value
	assert.ElementsMatch(t, []string{"set1", "set2"}, stringSet)
}

func TestMarshalItem_PointerTypes(t *testing.T) {
	marshaler := New()

	// Test with non-nil pointers
	str := "test-string"
	num := 42
	flt := 3.14
	bl := true

	input := PointerStruct{
		StringPtr:  &str,
		IntPtr:     &num,
		Float64Ptr: &flt,
		BoolPtr:    &bl,
	}

	metadata := createMetadata(
		createFieldMetadata("StringPtr", "string_ptr", 0, reflect.TypeOf(&str)),
		createFieldMetadata("IntPtr", "int_ptr", 1, reflect.TypeOf(&num)),
		createFieldMetadata("Float64Ptr", "float64_ptr", 2, reflect.TypeOf(&flt)),
		createFieldMetadata("BoolPtr", "bool_ptr", 3, reflect.TypeOf(&bl)),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	assert.Equal(t, "test-string", result["string_ptr"].(*types.AttributeValueMemberS).Value)
	assert.Equal(t, "42", result["int_ptr"].(*types.AttributeValueMemberN).Value)
	assert.Equal(t, "3.14", result["float64_ptr"].(*types.AttributeValueMemberN).Value)
	assert.Equal(t, true, result["bool_ptr"].(*types.AttributeValueMemberBOOL).Value)

	// Test with nil pointers
	input2 := PointerStruct{}

	result2, err := marshaler.MarshalItem(input2, metadata)
	require.NoError(t, err)

	for _, key := range []string{"string_ptr", "int_ptr", "float64_ptr", "bool_ptr"} {
		assert.IsType(t, &types.AttributeValueMemberNULL{}, result2[key])
		assert.True(t, result2[key].(*types.AttributeValueMemberNULL).Value)
	}
}

func TestMarshalItem_OmitEmpty(t *testing.T) {
	marshaler := New()

	// Test with empty values
	input := OmitEmptyStruct{
		Required: "required-value",
		// All other fields are zero values
	}

	metadata := createMetadata(
		createFieldMetadata("Required", "required", 0, reflect.TypeOf("")),
		createFieldMetadata("Optional", "optional", 1, reflect.TypeOf(""), withOmitEmpty()),
		createFieldMetadata("Number", "number", 2, reflect.TypeOf(0), withOmitEmpty()),
		createFieldMetadata("Float", "float", 3, reflect.TypeOf(0.0), withOmitEmpty()),
		createFieldMetadata("SliceOE", "slice_oe", 4, reflect.TypeOf([]string{}), withOmitEmpty()),
		createFieldMetadata("MapOE", "map_oe", 5, reflect.TypeOf(map[string]string{}), withOmitEmpty()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	// Required field should be present
	assert.Equal(t, "required-value", result["required"].(*types.AttributeValueMemberS).Value)

	// OmitEmpty fields should not be present
	assert.Len(t, result, 1) // Only required field should be in result
}

func TestMarshalItem_Errors(t *testing.T) {
	marshaler := New()

	tests := []struct {
		name     string
		input    interface{}
		metadata *model.Metadata
		wantErr  string
	}{
		{
			name:     "nil pointer",
			input:    (*SimpleStruct)(nil),
			metadata: &model.Metadata{},
			wantErr:  "cannot marshal nil pointer",
		},
		{
			name:     "non-struct type",
			input:    "not-a-struct",
			metadata: &model.Metadata{},
			wantErr:  "model must be a struct or pointer to struct",
		},
		{
			name:     "non-struct pointer",
			input:    new(string),
			metadata: &model.Metadata{},
			wantErr:  "model must be a struct or pointer to struct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := marshaler.MarshalItem(tt.input, tt.metadata)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMarshalItem_AllTypesSupport(t *testing.T) {
	marshaler := New()

	now := time.Now()
	input := AllTypesStruct{
		String:   "test",
		Int:      42,
		Int64:    int64(9223372036854775807),
		Float64:  3.14159,
		Bool:     true,
		Time:     now,
		StrSlice: []string{"a", "b", "c"},
		StrMap:   map[string]string{"key": "value"},
	}

	metadata := createMetadata(
		createFieldMetadata("String", "string", 0, reflect.TypeOf("")),
		createFieldMetadata("Int", "int", 1, reflect.TypeOf(0)),
		createFieldMetadata("Int64", "int64", 2, reflect.TypeOf(int64(0))),
		createFieldMetadata("Float64", "float64", 3, reflect.TypeOf(0.0)),
		createFieldMetadata("Bool", "bool", 4, reflect.TypeOf(false)),
		createFieldMetadata("Time", "time", 5, reflect.TypeOf(time.Time{})),
		createFieldMetadata("StrSlice", "str_slice", 6, reflect.TypeOf([]string{})),
		createFieldMetadata("StrMap", "str_map", 7, reflect.TypeOf(map[string]string{})),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	assert.Equal(t, "test", result["string"].(*types.AttributeValueMemberS).Value)
	assert.Equal(t, "42", result["int"].(*types.AttributeValueMemberN).Value)
	assert.Equal(t, "9223372036854775807", result["int64"].(*types.AttributeValueMemberN).Value)
	assert.Equal(t, "3.14159", result["float64"].(*types.AttributeValueMemberN).Value)
	assert.Equal(t, true, result["bool"].(*types.AttributeValueMemberBOOL).Value)
	assert.Equal(t, now.Format(time.RFC3339Nano), result["time"].(*types.AttributeValueMemberS).Value)

	// Check slice
	sliceVal := result["str_slice"].(*types.AttributeValueMemberL).Value
	assert.Len(t, sliceVal, 3)
	assert.Equal(t, "a", sliceVal[0].(*types.AttributeValueMemberS).Value)

	// Check map
	mapVal := result["str_map"].(*types.AttributeValueMemberM).Value
	assert.Len(t, mapVal, 1)
	assert.Equal(t, "value", mapVal["key"].(*types.AttributeValueMemberS).Value)
}

func TestMarshalItem_VersionField(t *testing.T) {
	marshaler := New()

	// Test with zero version
	input1 := VersionedStruct{ID: "test-id", Version: 0}
	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Version", "version", 1, reflect.TypeOf(int64(0)), withVersion()),
	)

	result1, err := marshaler.MarshalItem(input1, metadata)
	require.NoError(t, err)
	assert.Equal(t, "0", result1["version"].(*types.AttributeValueMemberN).Value)

	// Test with non-zero version
	input2 := VersionedStruct{ID: "test-id", Version: 5}
	result2, err := marshaler.MarshalItem(input2, metadata)
	require.NoError(t, err)
	assert.Equal(t, "5", result2["version"].(*types.AttributeValueMemberN).Value)
}

func TestMarshalItem_ConcurrentAccess(t *testing.T) {
	marshaler := New()

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Name", "name", 1, reflect.TypeOf("")),
		createFieldMetadata("Age", "age", 2, reflect.TypeOf(0)),
		createFieldMetadata("Score", "score", 3, reflect.TypeOf(0.0)),
		createFieldMetadata("Active", "active", 4, reflect.TypeOf(false)),
	)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Run 100 concurrent marshal operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			input := SimpleStruct{
				ID:     fmt.Sprintf("id-%d", id),
				Name:   fmt.Sprintf("name-%d", id),
				Age:    id,
				Score:  float64(id) * 1.5,
				Active: id%2 == 0,
			}
			_, err := marshaler.MarshalItem(input, metadata)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check no errors occurred
	for err := range errors {
		t.Errorf("Concurrent marshal error: %v", err)
	}
}

func TestMarshalItem_CacheReuse(t *testing.T) {
	marshaler := New()

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Name", "name", 1, reflect.TypeOf("")),
	)

	// First marshal should populate cache
	input1 := SimpleStruct{ID: "1", Name: "First"}
	_, err := marshaler.MarshalItem(input1, metadata)
	require.NoError(t, err)

	// Second marshal should use cached marshaler
	input2 := SimpleStruct{ID: "2", Name: "Second"}
	_, err = marshaler.MarshalItem(input2, metadata)
	require.NoError(t, err)

	// Verify cache was used (we can't directly test this, but ensure no errors)
	assert.NoError(t, err)
}

func TestMarshalComplexValue_EdgeCases(t *testing.T) {
	marshaler := New()

	// Test nil slice
	var nilSlice []string
	v1 := reflect.ValueOf(nilSlice)
	result1, err := marshaler.marshalComplexValue(v1)
	require.NoError(t, err)
	assert.IsType(t, &types.AttributeValueMemberNULL{}, result1)

	// Test nil map
	var nilMap map[string]string
	v2 := reflect.ValueOf(nilMap)
	result2, err := marshaler.marshalComplexValue(v2)
	require.NoError(t, err)
	assert.IsType(t, &types.AttributeValueMemberNULL{}, result2)

	// Test empty slice
	emptySlice := []string{}
	v3 := reflect.ValueOf(emptySlice)
	result3, err := marshaler.marshalComplexValue(v3)
	require.NoError(t, err)
	list := result3.(*types.AttributeValueMemberL).Value
	assert.Len(t, list, 0)

	// Test empty map
	emptyMap := map[string]string{}
	v4 := reflect.ValueOf(emptyMap)
	result4, err := marshaler.marshalComplexValue(v4)
	require.NoError(t, err)
	mapVal := result4.(*types.AttributeValueMemberM).Value
	assert.Len(t, mapVal, 0)
}

func TestMarshalValue_AllNumericTypes(t *testing.T) {
	marshaler := New()

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"int8", int8(127), "127"},
		{"int16", int16(32767), "32767"},
		{"int32", int32(2147483647), "2147483647"},
		{"uint", uint(42), "42"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float32", float32(3.14), "3.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := reflect.ValueOf(tt.value)
			result, err := marshaler.marshalValue(v)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.(*types.AttributeValueMemberN).Value)
		})
	}
}

func BenchmarkMarshalItem_Simple(b *testing.B) {
	marshaler := New()

	input := SimpleStruct{
		ID:     "bench-id",
		Name:   "Benchmark Test",
		Age:    30,
		Score:  98.5,
		Active: true,
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Name", "name", 1, reflect.TypeOf("")),
		createFieldMetadata("Age", "age", 2, reflect.TypeOf(0)),
		createFieldMetadata("Score", "score", 3, reflect.TypeOf(0.0)),
		createFieldMetadata("Active", "active", 4, reflect.TypeOf(false)),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = marshaler.MarshalItem(input, metadata)
	}
}

func BenchmarkMarshalItem_Complex(b *testing.B) {
	marshaler := New()

	optional := "optional"
	input := ComplexStruct{
		ID:            "bench-id",
		Tags:          []string{"tag1", "tag2", "tag3"},
		Attributes:    map[string]string{"key1": "value1", "key2": "value2"},
		Version:       1,
		TTL:           time.Now().Add(24 * time.Hour),
		OptionalField: &optional,
		StringSet:     []string{"set1", "set2"},
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Tags", "tags", 1, reflect.TypeOf([]string{})),
		createFieldMetadata("Attributes", "attributes", 2, reflect.TypeOf(map[string]string{})),
		createFieldMetadata("CreatedAt", "created_at", 3, reflect.TypeOf(time.Time{}), withCreatedAt()),
		createFieldMetadata("UpdatedAt", "updated_at", 4, reflect.TypeOf(time.Time{}), withUpdatedAt()),
		createFieldMetadata("Version", "version", 5, reflect.TypeOf(int64(0)), withVersion()),
		createFieldMetadata("TTL", "ttl", 6, reflect.TypeOf(time.Time{}), withTTL()),
		createFieldMetadata("OptionalField", "optional", 7, reflect.TypeOf(&optional), withOmitEmpty()),
		createFieldMetadata("StringSet", "string_set", 8, reflect.TypeOf([]string{}), withSet()),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = marshaler.MarshalItem(input, metadata)
	}
}

// Additional tests for edge cases and special scenarios
func TestMarshalItem_SpecialStringSetHandling(t *testing.T) {
	marshaler := New()

	// Test empty string set with omitempty
	type StringSetStruct struct {
		ID   string   `dynamodb:"id"`
		Tags []string `dynamodb:"tags,set,omitempty"`
	}

	input := StringSetStruct{
		ID:   "test-id",
		Tags: []string{}, // Empty set
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("Tags", "tags", 1, reflect.TypeOf([]string{}), withSet(), withOmitEmpty()),
	)

	result, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)

	// Empty set with omitempty should not be in result
	_, exists := result["tags"]
	assert.False(t, exists)
}

func TestMarshalItem_DeepNestedStructures(t *testing.T) {
	marshaler := New()

	type NestedMap struct {
		ID      string                       `dynamodb:"id"`
		DeepMap map[string]map[string]string `dynamodb:"deep_map"`
	}

	input := NestedMap{
		ID: "nested-id",
		DeepMap: map[string]map[string]string{
			"level1": {
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	metadata := createMetadata(
		createFieldMetadata("ID", "id", 0, reflect.TypeOf("")),
		createFieldMetadata("DeepMap", "deep_map", 1, reflect.TypeOf(map[string]map[string]string{})),
	)

	_, err := marshaler.MarshalItem(input, metadata)
	require.NoError(t, err)
}
