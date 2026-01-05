package activitypub

import (
	"encoding/json"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
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

func TestContextValue_DynamoDBAttributeValueRoundTrip(t *testing.T) {
	t.Run("empty marshals to NULL", func(t *testing.T) {
		av, err := (ContextValue{}).MarshalDynamoDBAttributeValue()
		require.NoError(t, err)
		_, ok := av.(*types.AttributeValueMemberNULL)
		assert.True(t, ok)

		var decoded ContextValue
		require.NoError(t, decoded.UnmarshalDynamoDBAttributeValue(av))
		assert.Nil(t, decoded)
	})

	t.Run("string marshals to list and unmarshals back", func(t *testing.T) {
		original := ContextValue{"https://www.w3.org/ns/activitystreams"}
		av, err := original.MarshalDynamoDBAttributeValue()
		require.NoError(t, err)

		var decoded ContextValue
		require.NoError(t, decoded.UnmarshalDynamoDBAttributeValue(av))
		require.Len(t, decoded, 1)
		assert.Equal(t, "https://www.w3.org/ns/activitystreams", decoded[0])
	})

	t.Run("legacy string attribute unmarshals", func(t *testing.T) {
		var decoded ContextValue
		require.NoError(t, decoded.UnmarshalDynamoDBAttributeValue(&types.AttributeValueMemberS{Value: "ctx"}))
		assert.Equal(t, ContextValue{"ctx"}, decoded)
	})

	t.Run("empty list attribute unmarshals to empty slice", func(t *testing.T) {
		var decoded ContextValue
		require.NoError(t, decoded.UnmarshalDynamoDBAttributeValue(&types.AttributeValueMemberL{Value: []types.AttributeValue{}}))
		assert.Equal(t, ContextValue{}, decoded)
	})

	t.Run("generic attribute unmarshals", func(t *testing.T) {
		av := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"key": &types.AttributeValueMemberS{Value: "value"},
			},
		}

		var decoded ContextValue
		require.NoError(t, decoded.UnmarshalDynamoDBAttributeValue(av))
		require.Len(t, decoded, 1)
		_, ok := decoded[0].(map[string]any)
		assert.True(t, ok)
	})

	t.Run("numeric attribute falls back to generic decoding", func(t *testing.T) {
		var decoded ContextValue
		require.NoError(t, decoded.UnmarshalDynamoDBAttributeValue(&types.AttributeValueMemberN{Value: "123"}))
		require.Len(t, decoded, 1)
		_, ok := decoded[0].(float64)
		assert.True(t, ok)
	})
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
}
