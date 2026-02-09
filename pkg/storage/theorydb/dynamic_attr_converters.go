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
)

var (
	mapStringAnyType = reflect.TypeOf(map[string]any{})
	sliceAnyType     = reflect.TypeOf([]any{})
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

func toAttributeValueDynamic(value any) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface:
		if rv.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
	}

	switch v := value.(type) {
	case types.AttributeValue:
		return v, nil
	case string:
		return &types.AttributeValueMemberS{Value: v}, nil
	case []byte:
		return &types.AttributeValueMemberB{Value: v}, nil
	case bool:
		return &types.AttributeValueMemberBOOL{Value: v}, nil
	case time.Time:
		if v.IsZero() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		return &types.AttributeValueMemberS{Value: v.UTC().Format(time.RFC3339Nano)}, nil
	case json.Number:
		return &types.AttributeValueMemberN{Value: v.String()}, nil
	case int:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(int64(v), 10)}, nil
	case int8:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(int64(v), 10)}, nil
	case int16:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(int64(v), 10)}, nil
	case int32:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(int64(v), 10)}, nil
	case int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}, nil
	case uint:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(uint64(v), 10)}, nil
	case uint8:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(uint64(v), 10)}, nil
	case uint16:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(uint64(v), 10)}, nil
	case uint32:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(uint64(v), 10)}, nil
	case uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v, 10)}, nil
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("invalid float32: %v", v)
		}
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(f, 'f', -1, 32)}, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("invalid float64: %v", v)
		}
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v, 'f', -1, 64)}, nil
	}

	// Handle map types with string keys.
	if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
		out := make(map[string]types.AttributeValue, rv.Len())
		for _, key := range rv.MapKeys() {
			keyStr := key.String()
			val := rv.MapIndex(key)
			av, err := toAttributeValueDynamic(val.Interface())
			if err != nil {
				return nil, fmt.Errorf("map key %s: %w", keyStr, err)
			}
			out[keyStr] = av
		}
		return &types.AttributeValueMemberM{Value: out}, nil
	}

	// Handle slices/arrays (non-[]byte).
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			return &types.AttributeValueMemberB{Value: rv.Bytes()}, nil
		}
		out := make([]types.AttributeValue, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			av, err := toAttributeValueDynamic(rv.Index(i).Interface())
			if err != nil {
				return nil, fmt.Errorf("slice index %d: %w", i, err)
			}
			out = append(out, av)
		}
		return &types.AttributeValueMemberL{Value: out}, nil
	}

	// Fall back to JSON string for unknown types.
	blob, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("unsupported type %T", value)
	}
	return &types.AttributeValueMemberS{Value: string(blob)}, nil
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
