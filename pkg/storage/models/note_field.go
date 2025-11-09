package models

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
)

// NoteField wraps activitypub.Note to handle DynamORM marshaling/unmarshaling
// properly, including the case where the database contains the string "null"
// instead of a DynamoDB NULL value.
//
// For Mastodon/Twitter-like UI compatibility:
// - Note is always populated when creating user statuses
// - Note contains the full ActivityPub representation as JSON
// - When nil, it's stored as DynamoDB NULL (not the string "null")
type NoteField struct {
	*activitypub.Note
}

// MarshalDynamoDBAttributeValue implements DynamORM's Marshaler interface
func (nf NoteField) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	if nf.Note == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	avMap, err := attributevalue.MarshalMap(nf.Note)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Note to Dynamo map: %w", err)
	}

	return &types.AttributeValueMemberM{Value: avMap}, nil
}

// UnmarshalDynamoDBAttributeValue implements DynamORM's Unmarshaler interface
func (nf *NoteField) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	// Handle NULL attribute value
	if nullVal, ok := av.(*types.AttributeValueMemberNULL); ok && nullVal.Value {
		nf.Note = nil
		return nil
	}

	mVal, ok := av.(*types.AttributeValueMemberM)
	if !ok {
		return fmt.Errorf("invalid NoteField format: expected map AttributeValue, got %T", av)
	}

	var note activitypub.Note
	if err := attributevalue.UnmarshalMap(mVal.Value, &note); err != nil {
		return fmt.Errorf("failed to unmarshal Note map: %w", err)
	}

	nf.Note = &note
	return nil
}

// Get returns the underlying *activitypub.Note for convenience
func (nf *NoteField) Get() *activitypub.Note {
	return nf.Note
}

// Set sets the underlying *activitypub.Note
func (nf *NoteField) Set(note *activitypub.Note) {
	nf.Note = note
}
