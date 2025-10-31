package models

import (
	"encoding/json"
	"fmt"

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

	// Marshal the Note as JSON string - this is the canonical format
	// Expected format: {"@context":[...],"id":"...","type":"Note","content":"...","attributedTo":"..."}
	data, err := json.Marshal(nf.Note)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Note to JSON: %w", err)
	}

	return &types.AttributeValueMemberS{Value: string(data)}, nil
}

// UnmarshalDynamoDBAttributeValue implements DynamORM's Unmarshaler interface
func (nf *NoteField) UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error {
	// Handle NULL attribute value
	if nullVal, ok := av.(*types.AttributeValueMemberNULL); ok && nullVal.Value {
		nf.Note = nil
		return nil
	}

	// Handle string attribute value (JSON string)
	strVal, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("invalid NoteField format: expected string AttributeValue, got %T", av)
	}

	jsonStr := strVal.Value

	// Handle the case where the database contains the literal string "null"
	// This should not happen with proper usage, but we handle it gracefully
	if jsonStr == "" || jsonStr == "null" {
		nf.Note = nil
		return nil
	}

	// Unmarshal the JSON string into activitypub.Note
	// Expected format: {"@context":[...],"id":"...","type":"Note","content":"...","attributedTo":"..."}
	var note activitypub.Note
	if err := json.Unmarshal([]byte(jsonStr), &note); err != nil {
		return fmt.Errorf("failed to unmarshal Note JSON: %w", err)
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

