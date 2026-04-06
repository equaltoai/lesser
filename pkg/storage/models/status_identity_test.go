package models

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCanonicalStatusID(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	t.Run("preserves local status identifiers", func(t *testing.T) {
		assert.Equal(t, "status-123", CanonicalStatusID("status-123"))
		assert.Equal(t, "123", CanonicalStatusID("https://example.com/users/alice/statuses/123"))
		assert.Equal(t, "123", CanonicalStatusID("https://example.com/users/alice/statuses/123/"))
	})

	t.Run("hashes remote status urls into collision safe ids", func(t *testing.T) {
		first := CanonicalStatusID("https://remote.one/users/alice/statuses/123")
		second := CanonicalStatusID("https://remote.two/users/alice/statuses/123")

		assert.NotEmpty(t, first)
		assert.NotEqual(t, first, second)
		assert.True(t, IsCanonicalRemoteStatusID(first))
		assert.True(t, IsCanonicalRemoteStatusID(second))
	})

	t.Run("normalizes remote url host case and trailing slash", func(t *testing.T) {
		expectedHash := sha256.Sum256([]byte("https://remote.example/users/alice/statuses/123"))
		expected := remoteStatusIDPrefix + hex.EncodeToString(expectedHash[:])

		assert.Equal(t, expected, CanonicalStatusID("HTTPS://REMOTE.EXAMPLE/users/alice/statuses/123/"))
		assert.Equal(t, expected, CanonicalStatusID(expected))
	})
}

func TestIsCanonicalRemoteStatusID(t *testing.T) {
	assert.False(t, IsCanonicalRemoteStatusID(""))
	assert.False(t, IsCanonicalRemoteStatusID("status-123"))
	assert.False(t, IsCanonicalRemoteStatusID("remote_short"))
	assert.True(t, IsCanonicalRemoteStatusID("remote_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
}
