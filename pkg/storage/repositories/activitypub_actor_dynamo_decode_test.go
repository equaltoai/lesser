package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeActivityPubActorFromDynamoValue_mergesEmbeddedBaseObject(t *testing.T) {
	raw := map[string]any{
		"BaseObject": map[string]any{
			"ID":   "https://example.com/users/alice",
			"Type": "Person",
		},
		"PreferredUsername": "alice",
		"Name":              "Alice",
	}

	actor, err := decodeActivityPubActorFromDynamoValue(raw)
	require.NoError(t, err)
	require.NotNil(t, actor)
	assert.Equal(t, "https://example.com/users/alice", actor.ID)
	assert.Equal(t, "Person", actor.Type)
	assert.Equal(t, "alice", actor.PreferredUsername)
	assert.Equal(t, "Alice", actor.Name)
}
