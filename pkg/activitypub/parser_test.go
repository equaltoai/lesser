package activitypub

import (
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseActivity(t *testing.T) {
	t.Run("invalid JSON rejected", func(t *testing.T) {
		_, err := ParseActivity([]byte("{"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("invalid activity rejected", func(t *testing.T) {
		_, err := ParseActivity([]byte(`{"id":"https://example.com/a/1","type":"not-a-type","actor":"https://example.com/users/alice"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid activity")
	})

	t.Run("valid activity parsed", func(t *testing.T) {
		activity, err := ParseActivity([]byte(`{"id":"https://example.com/a/1","type":"Create","actor":"https://example.com/users/alice","to":["https://www.w3.org/ns/activitystreams#Public"]}`))
		require.NoError(t, err)
		require.NotNil(t, activity)
		assert.Equal(t, "Create", activity.Type)
	})

	t.Run("unmarshal error surfaces as parse failure", func(t *testing.T) {
		_, err := ParseActivity([]byte(`{"id":123,"type":"Create","actor":"https://example.com/users/alice"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse activity")
	})

	t.Run("json safety limits rejected", func(t *testing.T) {
		body := `{"id":"https://example.com/a/bomb","type":"Create","actor":"https://example.com/users/alice","object":` + strings.Repeat(`{"nested":`, common.MaxJSONDepth+2) + `"leaf"` + strings.Repeat(`}`, common.MaxJSONDepth+2) + `}`
		_, err := ParseActivity([]byte(body))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse activity")
	})
}

func TestParseActor(t *testing.T) {
	t.Run("valid actor parsed", func(t *testing.T) {
		actor, err := ParseActor([]byte(`{"id":"https://example.com/users/alice","type":"Person","preferredUsername":"alice","inbox":"https://example.com/users/alice/inbox","outbox":"https://example.com/users/alice/outbox"}`))
		require.NoError(t, err)
		require.NotNil(t, actor)
		assert.Equal(t, "alice", actor.PreferredUsername)
	})

	t.Run("invalid actor rejected", func(t *testing.T) {
		_, err := ParseActor([]byte(`{"id":"https://example.com/users/alice","type":"Person"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid actor")
	})

	t.Run("unmarshal error surfaces as parse failure", func(t *testing.T) {
		_, err := ParseActor([]byte(`{"id":true,"type":"Person","preferredUsername":"alice","inbox":"https://example.com/users/alice/inbox","outbox":"https://example.com/users/alice/outbox"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse actor")
	})
}

func TestParseNote(t *testing.T) {
	t.Run("valid note parsed", func(t *testing.T) {
		note, err := ParseNote([]byte(`{"id":"https://example.com/notes/1","type":"Note","content":"hello","attributedTo":"https://example.com/users/alice"}`))
		require.NoError(t, err)
		require.NotNil(t, note)
		assert.Equal(t, "hello", note.Content)
	})

	t.Run("invalid note rejected", func(t *testing.T) {
		_, err := ParseNote([]byte(`{"id":"https://example.com/notes/1","type":"Note","content":"","attributedTo":"https://example.com/users/alice"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid note")
	})

	t.Run("unmarshal error surfaces as parse failure", func(t *testing.T) {
		_, err := ParseNote([]byte(`{"id":"https://example.com/notes/1","type":"Note","content":true,"attributedTo":"https://example.com/users/alice"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse note")
	})
}
