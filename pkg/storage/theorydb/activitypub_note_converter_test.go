package theorydb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/notecontract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivityPubNoteConverter_RoundTripFixtures(t *testing.T) {
	t.Run("public fixture", func(t *testing.T) {
		conv := activityPubNoteConverter{}
		av, err := conv.ToAttributeValue(notecontract.PublicFixtureNote())
		require.NoError(t, err)

		var out activitypub.Note
		require.NoError(t, conv.FromAttributeValue(av, &out))
		require.Equal(t, notecontract.PublicFixtureNote().ConversationID, out.ConversationID)
		require.Len(t, out.Attachment, 1)
		require.Len(t, out.Tag, 2)
		require.NotNil(t, out.QuoteContext)
		require.NotNil(t, out.AgentAttribution)
	})

	t.Run("direct fixture", func(t *testing.T) {
		conv := activityPubNoteConverter{}
		av, err := conv.ToAttributeValue(notecontract.DirectFixtureNote())
		require.NoError(t, err)

		var out activitypub.Note
		require.NoError(t, conv.FromAttributeValue(av, &out))
		require.Equal(t, notecontract.DirectFixtureNote().ConversationID, out.ConversationID)
		require.Len(t, out.Tag, 2)
		require.Equal(t, []string{
			"https://remote.example/users/bob",
			"https://lesser.example/users/carol",
		}, out.To)
	})
}

func TestActivityPubNoteConverter_NilAndErrors(t *testing.T) {
	conv := activityPubNoteConverter{}

	av, err := conv.ToAttributeValue((*activitypub.Note)(nil))
	require.NoError(t, err)
	_, ok := av.(*types.AttributeValueMemberNULL)
	require.True(t, ok)

	var out activitypub.Note
	require.NoError(t, conv.FromAttributeValue(&types.AttributeValueMemberNULL{Value: true}, &out))
	assert.Equal(t, activitypub.Note{}, out)

	_, err = conv.ToAttributeValue("nope")
	require.Error(t, err)
	require.ErrorContains(t, err, "expected activitypub.Note")

	err = conv.FromAttributeValue(&types.AttributeValueMemberS{Value: "[]"}, &out)
	require.Error(t, err)
	require.ErrorContains(t, err, "decode raw note")
}
