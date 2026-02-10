package theorydb

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicAttrConverters_ToAttributeValueDynamic_MoreCoverage(t *testing.T) {
	t.Run("nil_and_typed_nil_values_become_NULL", func(t *testing.T) {
		av, err := toAttributeValueDynamic(nil)
		require.NoError(t, err)
		_, ok := av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)

		var p *int
		av, err = toAttributeValueDynamic(p)
		require.NoError(t, err)
		_, ok = av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)

		var m map[string]any
		av, err = toAttributeValueDynamic(m)
		require.NoError(t, err)
		_, ok = av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)

		var s []any
		av, err = toAttributeValueDynamic(s)
		require.NoError(t, err)
		_, ok = av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)
	})

	t.Run("scalar_kinds", func(t *testing.T) {
		type myString string
		type myBool bool

		direct := &types.AttributeValueMemberS{Value: "direct"}
		av, err := toAttributeValueDynamic(direct)
		require.NoError(t, err)
		require.Equal(t, direct, av)

		av, err = toAttributeValueDynamic(myString("hello"))
		require.NoError(t, err)
		require.Equal(t, "hello", av.(*types.AttributeValueMemberS).Value)

		av, err = toAttributeValueDynamic(myBool(true))
		require.NoError(t, err)
		require.Equal(t, true, av.(*types.AttributeValueMemberBOOL).Value)

		av, err = toAttributeValueDynamic(int64(7))
		require.NoError(t, err)
		require.Equal(t, "7", av.(*types.AttributeValueMemberN).Value)

		av, err = toAttributeValueDynamic(uintptr(9))
		require.NoError(t, err)
		require.Equal(t, "9", av.(*types.AttributeValueMemberN).Value)

		av, err = toAttributeValueDynamic(float32(1.25))
		require.NoError(t, err)
		require.Equal(t, "1.25", av.(*types.AttributeValueMemberN).Value)

		av, err = toAttributeValueDynamic(float64(1.5))
		require.NoError(t, err)
		require.Equal(t, "1.5", av.(*types.AttributeValueMemberN).Value)

		_, err = toAttributeValueDynamic(math.NaN())
		require.Error(t, err)
	})

	t.Run("time_and_json_number", func(t *testing.T) {
		now := time.Date(2026, 2, 10, 12, 34, 56, 0, time.UTC)
		av, err := toAttributeValueDynamic(now)
		require.NoError(t, err)
		require.Equal(t, now.Format(time.RFC3339Nano), av.(*types.AttributeValueMemberS).Value)

		av, err = toAttributeValueDynamic(time.Time{})
		require.NoError(t, err)
		_, ok := av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)

		av, err = toAttributeValueDynamic(json.Number("123"))
		require.NoError(t, err)
		require.Equal(t, "123", av.(*types.AttributeValueMemberN).Value)
	})

	t.Run("string_key_maps_become_M_and_non_string_key_maps_fall_back_to_JSON", func(t *testing.T) {
		av, err := toAttributeValueDynamic(map[string]int{"a": 1})
		require.NoError(t, err)
		m := av.(*types.AttributeValueMemberM).Value
		require.Equal(t, "1", m["a"].(*types.AttributeValueMemberN).Value)

		av, err = toAttributeValueDynamic(map[int]string{1: "a"})
		require.NoError(t, err)
		s := av.(*types.AttributeValueMemberS).Value
		require.Contains(t, s, `"1"`)
		require.Contains(t, s, `"a"`)
	})

	t.Run("map_key_errors_are_wrapped", func(t *testing.T) {
		_, err := toAttributeValueDynamic(map[string]any{"bad": math.NaN()})
		require.Error(t, err)
		require.Contains(t, err.Error(), "map key bad")
	})

	t.Run("lists_and_binary", func(t *testing.T) {
		av, err := toAttributeValueDynamic([]int{1, 2})
		require.NoError(t, err)
		list := av.(*types.AttributeValueMemberL).Value
		require.Equal(t, "1", list[0].(*types.AttributeValueMemberN).Value)
		require.Equal(t, "2", list[1].(*types.AttributeValueMemberN).Value)

		type myBytes []byte
		av, err = toAttributeValueDynamic(myBytes([]byte{0x1, 0x2, 0x3}))
		require.NoError(t, err)
		require.Equal(t, []byte{0x1, 0x2, 0x3}, av.(*types.AttributeValueMemberB).Value)

		av, err = toAttributeValueDynamic([2]byte{0x4, 0x5})
		require.NoError(t, err)
		require.Equal(t, []byte{0x4, 0x5}, av.(*types.AttributeValueMemberB).Value)
	})

	t.Run("slice_index_errors_are_wrapped", func(t *testing.T) {
		_, err := toAttributeValueDynamic([]any{math.NaN()})
		require.Error(t, err)
		require.Contains(t, err.Error(), "slice index 0")
	})

	t.Run("fallback_JSON_string_for_structs", func(t *testing.T) {
		type payload struct {
			A int `json:"a"`
		}
		av, err := toAttributeValueDynamic(payload{A: 7})
		require.NoError(t, err)
		require.Equal(t, `{"a":7}`, av.(*types.AttributeValueMemberS).Value)

		_, err = toAttributeValueDynamic(make(chan int))
		require.Error(t, err)
	})
}

func TestDynamicAttrConverters_FromAttributeValueDynamic_MoreCoverage(t *testing.T) {
	out, err := fromAttributeValueDynamic(nil)
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = fromAttributeValueDynamic(&types.AttributeValueMemberNULL{Value: true})
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = fromAttributeValueDynamic(&types.AttributeValueMemberSS{Value: []string{"a", "b"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, out)

	out, err = fromAttributeValueDynamic(&types.AttributeValueMemberNS{Value: []string{"1", "1.25"}})
	require.NoError(t, err)
	require.IsType(t, []any{}, out)
	assert.Equal(t, int64(1), out.([]any)[0])
	assert.Equal(t, 1.25, out.([]any)[1])

	out, err = fromAttributeValueDynamic(&types.AttributeValueMemberBS{Value: [][]byte{[]byte("x")}})
	require.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("x")}, out)

	out, err = fromAttributeValueDynamic(&types.AttributeValueMemberB{Value: []byte{0x1}})
	require.NoError(t, err)
	assert.Equal(t, []byte{0x1}, out)

	assert.Equal(t, int64(0), parseDynamoNumber(" "))
	assert.Equal(t, json.Number("nope"), parseDynamoNumber("nope"))
	assert.Equal(t, json.Number("1e999"), parseDynamoNumber("1e999"))
}

func TestDynamicAttrConverters_JSONStringFallbacks(t *testing.T) {
	t.Run("mapStringAnyConverter_FromAttributeValue_accepts_JSON_string", func(t *testing.T) {
		conv := mapStringAnyConverter{}

		var out map[string]any
		require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberS{Value: `{"k":"v","n":7}`}, &out))
		require.Equal(t, "v", out["k"])
		require.Equal(t, float64(7), out["n"])
	})

	t.Run("sliceAnyConverter_FromAttributeValue_accepts_JSON_string", func(t *testing.T) {
		conv := sliceAnyConverter{}

		var out []any
		require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberS{Value: `["a",2,true]`}, &out))
		require.Len(t, out, 3)
		require.Equal(t, "a", out[0])
		require.Equal(t, float64(2), out[1])
		require.Equal(t, true, out[2])
	})

	t.Run("mapStringAnyConverter_FromAttributeValue_rejects_invalid_JSON_string", func(t *testing.T) {
		conv := mapStringAnyConverter{}

		var out map[string]any
		err := conv.FromAttributeValue(&types.AttributeValueMemberS{Value: `{"k":`}, &out)
		require.Error(t, err)
	})

	t.Run("sliceAnyConverter_FromAttributeValue_rejects_invalid_JSON_string", func(t *testing.T) {
		conv := sliceAnyConverter{}

		var out []any
		err := conv.FromAttributeValue(&types.AttributeValueMemberS{Value: `{"k":`}, &out)
		require.Error(t, err)
	})
}

func TestDynamicAttrConverters_NilInputsAndTypeErrors(t *testing.T) {
	t.Run("nil_maps_and_slices_round_trip_as_NULL", func(t *testing.T) {
		var nilMap map[string]any
		av, err := (mapStringAnyConverter{}).ToAttributeValue(nilMap)
		require.NoError(t, err)
		_, ok := av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)

		var out map[string]any
		require.NoError(t, (mapStringAnyConverter{}).FromAttributeValue(nil, &out))
		require.NoError(t, (mapStringAnyConverter{}).FromAttributeValue(av, &out))
		require.Nil(t, out)

		var nilSlice []any
		av, err = (sliceAnyConverter{}).ToAttributeValue(nilSlice)
		require.NoError(t, err)
		_, ok = av.(*types.AttributeValueMemberNULL)
		require.True(t, ok)

		var outSlice []any
		require.NoError(t, (sliceAnyConverter{}).FromAttributeValue(nil, &outSlice))
		require.NoError(t, (sliceAnyConverter{}).FromAttributeValue(av, &outSlice))
		require.Nil(t, outSlice)
	})

	t.Run("type_mismatches_return_errors", func(t *testing.T) {
		_, err := (mapStringAnyConverter{}).ToAttributeValue("nope")
		require.Error(t, err)
		_, err = (sliceAnyConverter{}).ToAttributeValue(map[string]any{"k": "v"})
		require.Error(t, err)

		var out map[string]any
		require.Error(t, (mapStringAnyConverter{}).FromAttributeValue(&types.AttributeValueMemberBOOL{Value: true}, &out))

		var outSlice []any
		require.Error(t, (sliceAnyConverter{}).FromAttributeValue(&types.AttributeValueMemberBOOL{Value: true}, &outSlice))
	})

	t.Run("FromAttributeValue_target_type_mismatches_return_errors", func(t *testing.T) {
		var outSlice []any
		require.Error(t, (mapStringAnyConverter{}).FromAttributeValue(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}}, &outSlice))

		var outMap map[string]any
		require.Error(t, (sliceAnyConverter{}).FromAttributeValue(&types.AttributeValueMemberL{Value: []types.AttributeValue{}}, &outMap))
	})

	t.Run("converter_errors_propagate_dynamic_errors", func(t *testing.T) {
		_, err := (mapStringAnyConverter{}).ToAttributeValue(map[string]any{"bad": math.NaN()})
		require.Error(t, err)
		_, err = (sliceAnyConverter{}).ToAttributeValue([]any{math.NaN()})
		require.Error(t, err)
	})
}
