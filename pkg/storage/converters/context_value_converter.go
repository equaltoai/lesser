package converters

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/equaltoai/lesser/pkg/activitypub"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	pkgtypes "github.com/pay-theory/dynamorm/pkg/types"
)

var contextValueType = reflect.TypeOf(activitypub.ContextValue{})

// ContextValueConverter normalizes ActivityPub context serialization in DynamoDB.
type ContextValueConverter struct{}

// Ensure ContextValueConverter satisfies the CustomConverter interface.
var _ pkgtypes.CustomConverter = ContextValueConverter{}

// RegisterContextConverters wires the context value converter into the provided DB instance.
func RegisterContextConverters(db dynamormCore.DB) error {
	if db == nil {
		return fmt.Errorf("dynamorm DB is nil")
	}

	extended, ok := db.(dynamormCore.ExtendedDB)
	if !ok {
		return fmt.Errorf("dynamorm DB does not expose converter registration")
	}

	return extended.RegisterTypeConverter(contextValueType, ContextValueConverter{})
}

// ToAttributeValue persists the context as a DynamoDB list while tolerating nil/legacy inputs.
func (ContextValueConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	ctx, err := coerceContextValue(value)
	if err != nil {
		return nil, err
	}

	if len(ctx) == 0 {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	items := make([]types.AttributeValue, 0, len(ctx))
	for idx, entry := range ctx {
		av, err := attributevalue.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("marshal context entry %d: %w", idx, err)
		}
		items = append(items, av)
	}

	return &types.AttributeValueMemberL{Value: items}, nil
}

// FromAttributeValue hydrates the context, accepting both the canonical list encoding and legacy string values.
func (ContextValueConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	elem, err := ensureContextValueTarget(target)
	if err != nil {
		return err
	}

	var ctx activitypub.ContextValue

	switch attr := av.(type) {
	case *types.AttributeValueMemberNULL:
		// leave ctx nil
	case *types.AttributeValueMemberS:
		ctx = activitypub.ContextValue{attr.Value}
	case *types.AttributeValueMemberL:
		out := make(activitypub.ContextValue, 0, len(attr.Value))
		for idx, item := range attr.Value {
			var decoded any
			if err := attributevalue.Unmarshal(item, &decoded); err != nil {
				return fmt.Errorf("unmarshal context entry %d: %w", idx, err)
			}
			out = append(out, decoded)
		}
		ctx = out
	default:
		var decoded any
		if err := attributevalue.Unmarshal(av, &decoded); err != nil {
			return fmt.Errorf("unmarshal context attribute: %w", err)
		}

		switch v := decoded.(type) {
		case []any:
			ctx = activitypub.ContextValue(v)
		case string:
			ctx = activitypub.ContextValue{v}
		case nil:
			// keep nil
		default:
			ctx = activitypub.ContextValue{v}
		}
	}

	elem.Set(reflect.ValueOf(ctx))
	return nil
}

func coerceContextValue(value any) (activitypub.ContextValue, error) {
	if value == nil {
		return nil, nil
	}

	switch v := value.(type) {
	case activitypub.ContextValue:
		return v, nil
	case *activitypub.ContextValue:
		if v == nil {
			return nil, nil
		}
		return *v, nil
	default:
		rv := reflect.ValueOf(value)
		if !rv.IsValid() {
			return nil, nil
		}

		switch rv.Kind() {
		case reflect.Ptr:
			if rv.IsNil() {
				return nil, nil
			}
			if rv.Elem().Type() == contextValueType {
				typed := rv.Elem().Interface().(activitypub.ContextValue)
				return typed, nil
			}
		case reflect.Slice:
			if rv.Type() == contextValueType {
				return rv.Interface().(activitypub.ContextValue), nil
			}
		}
	}

	return nil, fmt.Errorf("unsupported context value type %T", value)
}

func ensureContextValueTarget(target any) (reflect.Value, error) {
	if target == nil {
		return reflect.Value{}, fmt.Errorf("context target is nil")
	}

	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return reflect.Value{}, fmt.Errorf("context target must be non-nil pointer, got %T", target)
	}

	switch rv.Elem().Kind() {
	case reflect.Ptr:
		if rv.Elem().Type().Elem() != contextValueType {
			break
		}
		if rv.Elem().IsNil() {
			rv.Elem().Set(reflect.New(contextValueType))
		}
		return rv.Elem().Elem(), nil
	default:
		if rv.Elem().Type() == contextValueType {
			return rv.Elem(), nil
		}
	}

	return reflect.Value{}, fmt.Errorf("context target must point to ContextValue, got %T", target)
}
