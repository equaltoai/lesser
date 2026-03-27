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

func TestMarshalUnmarshalNormalize_NilNote(t *testing.T) {
	raw, err := Marshal(nil)
	require.NoError(t, err)
	require.Nil(t, raw)

	note, err := Unmarshal(nil)
	require.NoError(t, err)
	require.Nil(t, note)

	normalized, err := Normalize(nil)
	require.NoError(t, err)
	require.Nil(t, normalized)
}

func TestUnmarshal_NormalizesAttachmentAndQuoteAliases(t *testing.T) {
	note, err := Unmarshal(map[string]any{
		"@context": []any{"https://www.w3.org/ns/activitystreams"},
		"id":       "https://example.com/users/alice/statuses/2",
		"type":     "Note",
		"content":  "hello with aliases",
		"attachment": []any{
			map[string]any{
				"type":      "Document",
				"mediaType": "image/png",
				"url":       "https://cdn.example.com/alias.png",
				"name":      "alias-image",
				"value":     "preview",
				"width":     640,
				"height":    480,
			},
		},
		"quoteContext": map[string]any{
			"originalNoteId":         "https://example.com/users/bob/statuses/3",
			"originalAuthor":         "https://example.com/users/bob",
			"originalAuthorUsername": "bob",
			"quoteCount":             4,
			"allowWithdrawal":        true,
			"quoteAllowed":           true,
			"withdrawn":              false,
		},
		"agentAttribution": map[string]any{
			"trigger_type":  "assistant",
			"model_version": "gpt-5.4-fallback",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, note)
	require.Len(t, note.Attachment, 1)
	require.Equal(t, "image/png", note.Attachment[0].MediaType)
	require.Equal(t, 640, note.Attachment[0].Width)
	require.NotNil(t, note.QuoteContext)
	require.Equal(t, "bob", note.QuoteContext.OriginalAuthorUsername)
	require.True(t, note.QuoteContext.AllowWithdrawal)
	require.NotNil(t, note.AgentAttribution)
	require.Equal(t, "gpt-5.4-fallback", note.AgentAttribution.ModelID)
}
