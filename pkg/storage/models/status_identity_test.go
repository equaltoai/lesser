package models

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
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

func TestStatusLookupCandidatesForDomain(t *testing.T) {
	assert.Nil(t, StatusLookupCandidatesForDomain("   ", "example.com"))
	assert.Equal(t, []string{"status-123"}, StatusLookupCandidatesForDomain(" status-123 ", "example.com"))

	localURL := "https://example.com/users/alice/statuses/123/"
	assert.Equal(t, []string{"123"}, StatusLookupCandidatesForDomain(localURL, "example.com"))

	remoteURL := "https://remote.example/users/alice/statuses/123/"
	assert.Equal(t,
		[]string{CanonicalStatusIDForDomain(remoteURL, "example.com"), "123"},
		StatusLookupCandidatesForDomain(remoteURL, "example.com"),
	)

	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)
	assert.Equal(t, []string{"123"}, StatusLookupCandidates(localURL))
}

func TestIsCanonicalRemoteStatusID(t *testing.T) {
	assert.False(t, IsCanonicalRemoteStatusID(""))
	assert.False(t, IsCanonicalRemoteStatusID("status-123"))
	assert.False(t, IsCanonicalRemoteStatusID("remote_short"))
	assert.True(t, IsCanonicalRemoteStatusID("remote_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
}

func TestCanonicalStatusIDForDomain_InvalidInputFallsBackToRaw(t *testing.T) {
	assert.Equal(t, "status-123", CanonicalStatusIDForDomain("status-123", "example.com"))
	assert.Equal(t, "mailto:alice@example.com", CanonicalStatusIDForDomain("mailto:alice@example.com", "example.com"))
}

func TestNormalizeStatusIdentifierURL(t *testing.T) {
	normalized, parsed, ok := normalizeStatusIdentifierURL(" HTTPS://Example.com:443/users/alice/statuses/123/#frag ")
	assert.True(t, ok)
	assert.Equal(t, "https://example.com/users/alice/statuses/123", normalized)
	assert.Equal(t, "example.com", parsed.Hostname())
	assert.Equal(t, "/users/alice/statuses/123", parsed.Path)

	normalized, parsed, ok = normalizeStatusIdentifierURL("http://example.com:80#frag")
	assert.True(t, ok)
	assert.Equal(t, "http://example.com/", normalized)
	assert.Equal(t, "/", parsed.Path)

	_, _, ok = normalizeStatusIdentifierURL("https:///statuses/123")
	assert.False(t, ok)

	_, _, ok = normalizeStatusIdentifierURL("status-123")
	assert.False(t, ok)
}

func TestLocalStatusIDFromNormalizedURL(t *testing.T) {
	assert.Equal(t, "", localStatusIDFromNormalizedURL(nil))

	parsed := &url.URL{Path: "/"}
	assert.Equal(t, "", localStatusIDFromNormalizedURL(parsed))

	parsed.Path = "/users/alice/statuses/123"
	assert.Equal(t, "123", localStatusIDFromNormalizedURL(parsed))
}

func TestIsLocalStatusIdentifierHostForDomain(t *testing.T) {
	assert.True(t, isLocalStatusIdentifierHostForDomain("example.com", "https://example.com:443/app"))
	assert.False(t, isLocalStatusIdentifierHostForDomain("remote.example", "https://example.com"))
	assert.False(t, isLocalStatusIdentifierHostForDomain("", "https://example.com"))
	assert.False(t, isLocalStatusIdentifierHostForDomain("example.com", ""))
}
