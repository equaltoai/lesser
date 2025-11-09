package models

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
)

func TestNoteFieldMarshalToMap(t *testing.T) {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.org/users/test/statuses/123",
			Type: "Note",
		},
		Content:      "hello world",
		AttributedTo: "https://example.org/users/test",
		QuoteURL:     "https://example.org/users/test/statuses/original",
		QuoteContext: &activitypub.QuoteContext{OriginalNoteID: "original", OriginalAuthor: "https://example.org/users/test"},
	}

	av, err := (NoteField{Note: note}).MarshalDynamoDBAttributeValue()
	require.NoError(t, err)
	mapped, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok, "expected AttributeValueMemberM")

	contentAttr, ok := mapped.Value["Content"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "hello world", contentAttr.Value)

	typeAttr, ok := mapped.Value["Type"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "Note", typeAttr.Value)
}

func TestNoteFieldUnmarshalFromMap(t *testing.T) {
	mapped := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"ID":       &types.AttributeValueMemberS{Value: "https://example.org/users/test/statuses/abc"},
		"Type":     &types.AttributeValueMemberS{Value: "Note"},
		"Content":  &types.AttributeValueMemberS{Value: "example"},
		"QuoteURL": &types.AttributeValueMemberS{Value: "https://example.org/users/test/statuses/original"},
		"QuoteContext": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"OriginalNoteID":  &types.AttributeValueMemberS{Value: "original"},
			"OriginalAuthor":  &types.AttributeValueMemberS{Value: "https://example.org/users/test"},
			"AllowWithdrawal": &types.AttributeValueMemberBOOL{Value: true},
		}},
		"AttributedTo": &types.AttributeValueMemberS{Value: "https://example.org/users/test"},
	}}

	var nf NoteField
	require.NoError(t, nf.UnmarshalDynamoDBAttributeValue(mapped))
	require.NotNil(t, nf.Note)
	require.Equal(t, "example", nf.Note.Content)
	require.Equal(t, "https://example.org/users/test", nf.Note.AttributedTo)
	require.Equal(t, "https://example.org/users/test/statuses/original", nf.Note.QuoteURL)
	require.NotNil(t, nf.Note.QuoteContext)
	require.Equal(t, "original", nf.Note.QuoteContext.OriginalNoteID)
}
