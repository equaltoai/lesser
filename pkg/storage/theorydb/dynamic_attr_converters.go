package theorydb

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
)

var (
	mapStringAnyType            = reflect.TypeOf(map[string]any{})
	sliceAnyType                = reflect.TypeOf([]any{})
	activityPubContextValueType = reflect.TypeOf(activitypub.ContextValue{})
)

type mapStringAnyConverter struct{}

func (mapStringAnyConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mapStringAnyConverter: expected map[string]any, got %T", value)
	}
	if m == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	out := make(map[string]types.AttributeValue, len(m))
	for k, v := range m {
		av, err := toAttributeValueDynamic(v)
		if err != nil {
			return nil, fmt.Errorf("mapStringAnyConverter: key %s: %w", k, err)
		}
		out[k] = av
	}
	return &types.AttributeValueMemberM{Value: out}, nil
}

func (mapStringAnyConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	dest, ok := target.(*map[string]any)
	if !ok {
		return fmt.Errorf("mapStringAnyConverter: target must be *map[string]any, got %T", target)
	}

	switch v := av.(type) {
	case nil:
		*dest = nil
		return nil
	case *types.AttributeValueMemberNULL:
		*dest = nil
		return nil
	case *types.AttributeValueMemberM:
		out := make(map[string]any, len(v.Value))
		for k, vv := range v.Value {
			decoded, err := fromAttributeValueDynamic(vv)
			if err != nil {
				return fmt.Errorf("mapStringAnyConverter: key %s: %w", k, err)
			}
			out[k] = decoded
		}
		*dest = out
		return nil
	case *types.AttributeValueMemberS:
		var out map[string]any
		if err := json.Unmarshal([]byte(v.Value), &out); err != nil {
			return fmt.Errorf("mapStringAnyConverter: expected map attribute or JSON string, got string: %w", err)
		}
		*dest = out
		return nil
	default:
		return fmt.Errorf("mapStringAnyConverter: expected map attribute or JSON string, got %T", av)
	}
}

type sliceAnyConverter struct{}

type activityPubContextValueConverter struct{}

func (sliceAnyConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	s, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("sliceAnyConverter: expected []any, got %T", value)
	}
	if s == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	out := make([]types.AttributeValue, 0, len(s))
	for i, v := range s {
		av, err := toAttributeValueDynamic(v)
		if err != nil {
			return nil, fmt.Errorf("sliceAnyConverter: index %d: %w", i, err)
		}
		out = append(out, av)
	}
	return &types.AttributeValueMemberL{Value: out}, nil
}

func (sliceAnyConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	dest, ok := target.(*[]any)
	if !ok {
		return fmt.Errorf("sliceAnyConverter: target must be *[]any, got %T", target)
	}

	switch v := av.(type) {
	case nil:
		*dest = nil
		return nil
	case *types.AttributeValueMemberNULL:
		*dest = nil
		return nil
	case *types.AttributeValueMemberL:
		out := make([]any, 0, len(v.Value))
		for i, vv := range v.Value {
			decoded, err := fromAttributeValueDynamic(vv)
			if err != nil {
				return fmt.Errorf("sliceAnyConverter: index %d: %w", i, err)
			}
			out = append(out, decoded)
		}
		*dest = out
		return nil
	case *types.AttributeValueMemberS:
		var out []any
		if err := json.Unmarshal([]byte(v.Value), &out); err != nil {
			return fmt.Errorf("sliceAnyConverter: expected list attribute or JSON string, got string: %w", err)
		}
		*dest = out
		return nil
	default:
		return fmt.Errorf("sliceAnyConverter: expected list attribute or JSON string, got %T", av)
	}
}

func (activityPubContextValueConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	contextValue, ok := value.(activitypub.ContextValue)
	if !ok {
		return nil, fmt.Errorf("activityPubContextValueConverter: expected activitypub.ContextValue, got %T", value)
	}

	return (sliceAnyConverter{}).ToAttributeValue([]any(contextValue))
}

func (activityPubContextValueConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	dest, ok := target.(*activitypub.ContextValue)
	if !ok {
		return fmt.Errorf("activityPubContextValueConverter: target must be *activitypub.ContextValue, got %T", target)
	}

	var out []any
	if err := (sliceAnyConverter{}).FromAttributeValue(av, &out); err != nil {
		return err
	}

	*dest = activitypub.ContextValue(out)
	return nil
}

func toAttributeValueDynamic(value any) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	rv := reflect.ValueOf(value)
	if isNilReflectValue(rv) {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if av, ok := value.(types.AttributeValue); ok {
		return av, nil
	}

	switch v := value.(type) {
	case time.Time:
		if v.IsZero() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return &types.AttributeValueMemberS{Value: v.UTC().Format(time.RFC3339Nano)}, nil
	case json.Number:
		return &types.AttributeValueMemberN{Value: v.String()}, nil
	}

	if av, ok, err := toAttributeValueScalar(rv); ok {
		return av, err
	}

	if av, ok, err := toAttributeValueStringKeyMap(rv); ok {
		return av, err
	}

	if av, ok, err := toAttributeValueSliceOrArray(rv); ok {
		return av, err
	}

	// Fall back to JSON string for unknown types.
	blob, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("unsupported type %T", value)
	}
	return &types.AttributeValueMemberS{Value: string(blob)}, nil
}

func isNilReflectValue(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

func toAttributeValueScalar(rv reflect.Value) (types.AttributeValue, bool, error) {
	switch rv.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: rv.String()}, true, nil
	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: rv.Bool()}, true, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(rv.Int(), 10)}, true, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(rv.Uint(), 10)}, true, nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, true, fmt.Errorf("invalid float: %v", f)
		}
		bitSize := 64
		if rv.Kind() == reflect.Float32 {
			bitSize = 32
		}
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(f, 'f', -1, bitSize)}, true, nil
	default:
		return nil, false, nil
	}
}

func toAttributeValueStringKeyMap(rv reflect.Value) (types.AttributeValue, bool, error) {
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false, nil
	}

	out := make(map[string]types.AttributeValue, rv.Len())
	for _, key := range rv.MapKeys() {
		keyStr := key.String()
		val := rv.MapIndex(key)
		av, err := toAttributeValueDynamic(val.Interface())
		if err != nil {
			return nil, true, fmt.Errorf("map key %s: %w", keyStr, err)
		}
		out[keyStr] = av
	}
	return &types.AttributeValueMemberM{Value: out}, true, nil
}

func toAttributeValueSliceOrArray(rv reflect.Value) (types.AttributeValue, bool, error) {
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false, nil
	}

	// Treat any slice/array of bytes as a binary DynamoDB attribute.
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		if rv.Kind() == reflect.Slice {
			return &types.AttributeValueMemberB{Value: rv.Bytes()}, true, nil
		}

		out := make([]byte, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = byte(rv.Index(i).Uint())
		}
		return &types.AttributeValueMemberB{Value: out}, true, nil
	}

	out := make([]types.AttributeValue, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		av, err := toAttributeValueDynamic(rv.Index(i).Interface())
		if err != nil {
			return nil, true, fmt.Errorf("slice index %d: %w", i, err)
		}
		out = append(out, av)
	}
	return &types.AttributeValueMemberL{Value: out}, true, nil
}

func fromAttributeValueDynamic(av types.AttributeValue) (any, error) {
	switch v := av.(type) {
	case nil:
		return nil, nil
	case *types.AttributeValueMemberNULL:
		return nil, nil
	case *types.AttributeValueMemberS:
		return v.Value, nil
	case *types.AttributeValueMemberBOOL:
		return v.Value, nil
	case *types.AttributeValueMemberN:
		return parseDynamoNumber(v.Value), nil
	case *types.AttributeValueMemberB:
		return v.Value, nil
	case *types.AttributeValueMemberSS:
		out := make([]string, 0, len(v.Value))
		out = append(out, v.Value...)
		return out, nil
	case *types.AttributeValueMemberNS:
		out := make([]any, 0, len(v.Value))
		for _, raw := range v.Value {
			out = append(out, parseDynamoNumber(raw))
		}
		return out, nil
	case *types.AttributeValueMemberBS:
		return v.Value, nil
	case *types.AttributeValueMemberL:
		out := make([]any, 0, len(v.Value))
		for _, vv := range v.Value {
			decoded, err := fromAttributeValueDynamic(vv)
			if err != nil {
				return nil, err
			}
			out = append(out, decoded)
		}
		return out, nil
	case *types.AttributeValueMemberM:
		out := make(map[string]any, len(v.Value))
		for k, vv := range v.Value {
			decoded, err := fromAttributeValueDynamic(vv)
			if err != nil {
				return nil, fmt.Errorf("key %s: %w", k, err)
			}
			out[k] = decoded
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported AttributeValue type: %T", av)
	}
}

func parseDynamoNumber(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return int64(0)
	}
	if strings.ContainsAny(s, ".eE") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return json.Number(s)
}
