package activitypub

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextValue_JSONRoundTrip(t *testing.T) {
	t.Run("single string marshals as string", func(t *testing.T) {
		data, err := json.Marshal(ContextValue{"https://www.w3.org/ns/activitystreams"})
		require.NoError(t, err)
		assert.Equal(t, `"https://www.w3.org/ns/activitystreams"`, string(data))

		var decoded ContextValue
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, ContextValue{"https://www.w3.org/ns/activitystreams"}, decoded)
	})

	t.Run("array marshals as array", func(t *testing.T) {
		data, err := json.Marshal(ContextValue{"https://www.w3.org/ns/activitystreams", map[string]any{"toot": "http://joinmastodon.org/ns#"}})
		require.NoError(t, err)
		assert.Contains(t, string(data), "[")

		var decoded ContextValue
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Len(t, decoded, 2)
	})

	t.Run("null unmarshals to nil", func(t *testing.T) {
		var decoded ContextValue
		require.NoError(t, json.Unmarshal([]byte("null"), &decoded))
		assert.Nil(t, decoded)
	})

	t.Run("non-string single value marshals as array", func(t *testing.T) {
		data, err := json.Marshal(ContextValue{map[string]any{"k": "v"}})
		require.NoError(t, err)
		assert.Contains(t, string(data), "[")
	})

	t.Run("non-array non-string unmarshals to single entry", func(t *testing.T) {
		var decoded ContextValue
		require.NoError(t, json.Unmarshal([]byte(`{"k":"v"}`), &decoded))
		require.Len(t, decoded, 1)
		_, ok := decoded[0].(map[string]any)
		assert.True(t, ok)
	})
}

func TestContextValue_CloneAndWith(t *testing.T) {
	original := ContextValue{"a", "b"}
	cloned := original.Clone()
	require.Equal(t, original, cloned)

	cloned[0] = "changed"
	assert.Equal(t, ContextValue{"a", "b"}, original)

	with := original.With("c")
	assert.Equal(t, ContextValue{"a", "b", "c"}, with)
	assert.Equal(t, ContextValue{"a", "b"}, original)
}

func TestNewActorAndActivityHelpers(t *testing.T) {
	actor := NewActor(PersonType, "https://example.com/users/alice", "alice")
	require.NotNil(t, actor)
	assert.Equal(t, "https://example.com/users/alice", actor.ID)
	assert.Equal(t, PersonType, actor.Type)
	assert.Equal(t, "alice", actor.PreferredUsername)
	assert.Equal(t, "https://example.com/users/alice/inbox", actor.Inbox)
	assert.Equal(t, "https://example.com/users/alice/outbox", actor.Outbox)
	assert.Equal(t, "https://example.com/users/alice/followers", actor.Followers)

	activity := NewActivity(CreateType, "https://example.com/activities/1", actor.ID, map[string]any{"type": "Note"})
	require.NotNil(t, activity)
	assert.Equal(t, CreateType, activity.Type)
	require.NotNil(t, activity.Published)
}

func TestActor_MarshalJSON_UsesContextValue(t *testing.T) {
	actor := NewActor(PersonType, "https://example.com/users/alice", "alice")
	data, err := json.Marshal(actor)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Contains(t, decoded, "@context")
	_, ok := decoded["@context"].([]any)
	assert.True(t, ok)

	contextEntries, ok := decoded["@context"].([]any)
	require.True(t, ok)
	require.Len(t, contextEntries, 2)

	contextMap, ok := contextEntries[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://spec.lessersoul.ai/ns/agent-attribution/v1#", contextMap["lessersoul"])

	agentAttribution, ok := contextMap["agentAttribution"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "lessersoul:agentAttribution", agentAttribution["@id"])
	assert.Equal(t, "@json", agentAttribution["@type"])
}

func TestNoteAgentAttributionJSONCompatibility(t *testing.T) {
	t.Run("legacy key unmarshals", func(t *testing.T) {
		var note Note
		err := json.Unmarshal([]byte(`{
			"id":"https://example.com/notes/1",
			"type":"Note",
			"content":"hi",
			"attributedTo":"https://example.com/users/alice",
			"_:agentAttribution":{"delegated_by":"https://example.com/users/owner","model_version":"claude-3"}
		}`), &note)
		require.NoError(t, err)
		require.NotNil(t, note.AgentAttribution)
		require.Equal(t, "https://example.com/users/owner", note.AgentAttribution.DelegatedBy)
		require.Equal(t, legacyAgentAttributionSchemaVersion, note.AgentAttribution.SchemaVersion)
		require.Equal(t, "claude-3", note.AgentAttribution.ModelID)
	})

	t.Run("new key unmarshals", func(t *testing.T) {
		var note Note
		err := json.Unmarshal([]byte(`{
			"id":"https://example.com/notes/1",
			"type":"Note",
			"content":"hi",
			"attributedTo":"https://example.com/users/alice",
			"agentAttribution":{"delegated_by":"https://example.com/users/owner","schema_version":"1.0","model_id":"claude-3"}
		}`), &note)
		require.NoError(t, err)
		require.NotNil(t, note.AgentAttribution)
		require.Equal(t, AgentAttributionSchemaVersion, note.AgentAttribution.SchemaVersion)
		require.Equal(t, "claude-3", note.AgentAttribution.ModelID)
	})

	t.Run("marshal uses new key", func(t *testing.T) {
		data, err := json.Marshal(Note{
			BaseObject:   BaseObject{ID: "https://example.com/notes/1", Type: NoteType},
			Content:      "hi",
			AttributedTo: "https://example.com/users/alice",
			AgentAttribution: &AgentPostAttribution{
				SchemaVersion: AgentAttributionSchemaVersion,
				ModelID:       "claude-3",
			},
		})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"agentAttribution"`)
		assert.NotContains(t, string(data), `"_:agentAttribution"`)
	})

	t.Run("marshal normalizes short delegated_by to actor URI", func(t *testing.T) {
		data, err := json.Marshal(Note{
			BaseObject:   BaseObject{ID: "https://example.com/notes/1", Type: NoteType},
			Content:      "hi",
			AttributedTo: "https://example.com/users/simulacrum",
			AgentAttribution: &AgentPostAttribution{
				DelegatedBy:   "@owner",
				ModelID:       "claude-3",
				SchemaVersion: AgentAttributionSchemaVersion,
			},
		})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"delegated_by":"https://example.com/users/owner"`)
	})

	t.Run("article marshal normalizes short delegated_by to actor URI", func(t *testing.T) {
		data, err := json.Marshal(Article{
			Note: Note{
				BaseObject:   BaseObject{ID: "https://example.com/articles/1", Type: ArticleType},
				Content:      "hi",
				AttributedTo: "https://example.com/users/simulacrum",
				AgentAttribution: &AgentPostAttribution{
					DelegatedBy:   "owner",
					ModelID:       "claude-3",
					SchemaVersion: AgentAttributionSchemaVersion,
				},
			},
			Name: "Title",
		})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"name":"Title"`)
		assert.Contains(t, string(data), `"delegated_by":"https://example.com/users/owner"`)
	})

	t.Run("quote note marshal normalizes short delegated_by to actor URI", func(t *testing.T) {
		data, err := json.Marshal(QuoteNote{
			Note: Note{
				BaseObject:   BaseObject{ID: "https://example.com/notes/quote-1", Type: NoteType},
				Content:      "hi",
				AttributedTo: "https://example.com/users/simulacrum",
				AgentAttribution: &AgentPostAttribution{
					DelegatedBy:   "@owner",
					ModelID:       "claude-3",
					SchemaVersion: AgentAttributionSchemaVersion,
				},
			},
		})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"delegated_by":"https://example.com/users/owner"`)
	})

	t.Run("marshal leaves delegated_by unchanged when actor URI is unavailable", func(t *testing.T) {
		data, err := json.Marshal(Note{
			BaseObject:   BaseObject{ID: "https://example.com/notes/1", Type: NoteType},
			Content:      "hi",
			AttributedTo: "not-a-uri",
			AgentAttribution: &AgentPostAttribution{
				DelegatedBy:   "@owner",
				ModelID:       "claude-3",
				SchemaVersion: AgentAttributionSchemaVersion,
			},
		})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"delegated_by":"@owner"`)
	})

	t.Run("note without attribution still works", func(t *testing.T) {
		var note Note
		err := json.Unmarshal([]byte(`{
			"id":"https://example.com/notes/1",
			"type":"Note",
			"content":"hi",
			"attributedTo":"https://example.com/users/alice"
		}`), &note)
		require.NoError(t, err)
		require.Nil(t, note.AgentAttribution)
	})
}

func TestNormalizeAgentPostAttributionHelpers(t *testing.T) {
	t.Run("normalize delegated_by keeps full URI unchanged", func(t *testing.T) {
		value := "HTTPS://remote.example/users/owner"
		require.Equal(t, value, normalizeDelegatedByActorURI(value, "https://example.com/users/alice"))
	})

	t.Run("normalize attribution returns nil for nil input", func(t *testing.T) {
		require.Nil(t, normalizeAgentPostAttributionForActor(nil, "https://example.com/users/alice"))
	})

	t.Run("normalize attribution returns copy without mutating original", func(t *testing.T) {
		original := &AgentPostAttribution{
			DelegatedBy:   "@owner",
			SchemaVersion: AgentAttributionSchemaVersion,
			ModelID:       "claude-3",
		}

		normalized := normalizeAgentPostAttributionForActor(original, "https://example.com/users/simulacrum")
		require.NotNil(t, normalized)
		require.NotSame(t, original, normalized)
		require.Equal(t, "@owner", original.DelegatedBy)
		require.Equal(t, "https://example.com/users/owner", normalized.DelegatedBy)
	})
}
