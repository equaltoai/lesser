package theorydb

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/notecontract"
)

var activityPubNoteType = reflect.TypeOf(activitypub.Note{})

type activityPubNoteConverter struct{}

func (activityPubNoteConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	note, err := noteValueFromAny(value)
	if err != nil {
		return nil, err
	}
	if note == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	raw, err := notecontract.Marshal(note)
	if err != nil {
		return nil, fmt.Errorf("activityPubNoteConverter: marshal note contract: %w", err)
	}

	return (mapStringAnyConverter{}).ToAttributeValue(raw)
}

func (activityPubNoteConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	dest, ok := target.(*activitypub.Note)
	if !ok {
		return fmt.Errorf("activityPubNoteConverter: target must be *activitypub.Note, got %T", target)
	}
	if dest == nil {
		return fmt.Errorf("activityPubNoteConverter: target is nil")
	}

	switch av.(type) {
	case nil, *types.AttributeValueMemberNULL:
		*dest = activitypub.Note{}
		return nil
	}

	var raw map[string]any
	if err := (mapStringAnyConverter{}).FromAttributeValue(av, &raw); err != nil {
		return fmt.Errorf("activityPubNoteConverter: decode raw note: %w", err)
	}

	note, err := notecontract.Unmarshal(raw)
	if err != nil {
		return fmt.Errorf("activityPubNoteConverter: unmarshal note contract: %w", err)
	}
	if note == nil {
		*dest = activitypub.Note{}
		return nil
	}

	*dest = *note
	return nil
}

func noteValueFromAny(value any) (*activitypub.Note, error) {
	if value == nil {
		return nil, nil
	}

	switch typed := value.(type) {
	case activitypub.Note:
		note := typed
		return &note, nil
	case *activitypub.Note:
		return typed, nil
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Kind() == reflect.Ptr && rv.IsNil() {
			return nil, nil
		}
		return nil, fmt.Errorf("activityPubNoteConverter: expected activitypub.Note, got %T", value)
	}
}
