package notecontract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalAndNormalize_PublicFixture(t *testing.T) {
	raw, err := Marshal(PublicFixtureNote())
	require.NoError(t, err)
	require.NotNil(t, raw["BaseObject"])
	require.NotNil(t, raw["Attachment"])
	require.NotNil(t, raw["Tag"])
	require.NotNil(t, raw["QuoteContext"])
	require.NotNil(t, raw["AgentAttribution"])

	normalized, err := Normalize(PublicFixtureNote())
	require.NoError(t, err)
	require.NotNil(t, normalized)
	require.Equal(t, PublicFixtureNote().ConversationID, normalized.ConversationID)
	require.Len(t, normalized.Attachment, 1)
	require.Len(t, normalized.Tag, 2)
	require.NotNil(t, normalized.QuoteContext)
	require.NotNil(t, normalized.AgentAttribution)
}

func TestMarshalAndNormalize_DirectFixture(t *testing.T) {
	raw, err := Marshal(DirectFixtureNote())
	require.NoError(t, err)
	require.NotNil(t, raw["BaseObject"])
	require.NotNil(t, raw["Tag"])
	require.Nil(t, raw["Attachment"])

	normalized, err := Normalize(DirectFixtureNote())
	require.NoError(t, err)
	require.NotNil(t, normalized)
	require.Equal(t, DirectFixtureNote().ConversationID, normalized.ConversationID)
	require.Len(t, normalized.Tag, 2)
	require.Equal(t, []string{
		"https://remote.example/users/bob",
		"https://lesser.example/users/carol",
	}, normalized.To)
}

func TestUnmarshal_AcceptsActivityPubJSONKeyAliases(t *testing.T) {
	note, err := Unmarshal(map[string]any{
		"@context": []any{"https://www.w3.org/ns/activitystreams"},
		"id":       "https://example.com/users/alice/statuses/1",
		"type":     "Note",
		"to":       []any{"https://www.w3.org/ns/activitystreams#Public"},
		"content":  "hello world",
		"tag": []any{
			map[string]any{"type": "Mention", "href": "https://remote.example/users/bob", "name": "@bob"},
		},
		"conversationId": "conv-1",
		"_:visibility":   "direct",
		"agentAttribution": map[string]any{
			"trigger_type":   "assistant",
			"schema_version": "1.0",
			"model_id":       "gpt-5.4",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, note)
	require.Equal(t, "https://example.com/users/alice/statuses/1", note.ID)
	require.Equal(t, "hello world", note.Content)
	require.Equal(t, "conv-1", note.ConversationID)
	require.Equal(t, "direct", note.Visibility)
	require.NotNil(t, note.AgentAttribution)
	require.Equal(t, "gpt-5.4", note.AgentAttribution.ModelID)
}
