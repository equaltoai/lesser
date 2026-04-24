package models

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
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

func TestCanonicalActivityPubObjectIDForStatus(t *testing.T) {
	t.Run("nil status has no object id", func(t *testing.T) {
		assert.Empty(t, CanonicalActivityPubObjectIDForStatus(nil, "example.com"))
	})

	t.Run("prefers remote note id over local remote status id", func(t *testing.T) {
		remoteURL := "https://remote.example/users/alice/statuses/123"
		status := &Status{
			StatusID:       CanonicalStatusIDForDomain(remoteURL, "example.com"),
			AuthorID:       "https://remote.example/users/alice",
			AuthorUsername: "alice@remote.example",
			Note:           &activitypub.Note{BaseObject: activitypub.BaseObject{ID: remoteURL}},
		}

		assert.Equal(t, remoteURL, CanonicalActivityPubObjectIDForStatus(status, "example.com"))
	})

	t.Run("uses explicit url when note id is absent", func(t *testing.T) {
		remoteURL := "https://remote.example/users/alice/statuses/456"
		status := &Status{
			StatusID:       CanonicalStatusIDForDomain(remoteURL, "example.com"),
			AuthorID:       "https://remote.example/users/alice",
			AuthorUsername: "alice@remote.example",
			URLs:           []string{remoteURL},
		}

		assert.Equal(t, remoteURL, CanonicalActivityPubObjectIDForStatus(status, "example.com"))
	})

	t.Run("builds local canonical fallback only for local statuses", func(t *testing.T) {
		status := &Status{
			StatusID:       "local-1",
			AuthorID:       "https://example.com/users/alice",
			AuthorUsername: "alice",
		}

		assert.Equal(t,
			"https://example.com/users/alice/statuses/local-1",
			CanonicalActivityPubObjectIDForStatus(status, "https://example.com"),
		)
	})

	t.Run("does not synthesize a local object id for remote statuses", func(t *testing.T) {
		status := &Status{
			StatusID:       "remote_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			AuthorID:       "https://remote.example/users/alice",
			AuthorUsername: "alice@remote.example",
		}

		assert.Empty(t, CanonicalActivityPubObjectIDForStatus(status, "example.com"))
	})
}

func TestStatusInteractionIdentityHelpers(t *testing.T) {
	t.Run("status appears remote from canonical remote id", func(t *testing.T) {
		status := &Status{StatusID: "remote_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
		assert.True(t, statusAppearsRemote(status, "example.com"))
	})

	t.Run("status appears remote from acct author", func(t *testing.T) {
		status := &Status{StatusID: "status-1", AuthorUsername: "alice@remote.example"}
		assert.True(t, statusAppearsRemote(status, "example.com"))
	})

	t.Run("status appears remote from actor host", func(t *testing.T) {
		status := &Status{StatusID: "status-1", AuthorID: "https://remote.example/users/alice"}
		assert.True(t, statusAppearsRemote(status, "https://example.com"))
	})

	t.Run("status appears local from actor host", func(t *testing.T) {
		status := &Status{StatusID: "status-1", AuthorID: "https://example.com/users/alice"}
		assert.False(t, statusAppearsRemote(status, "https://example.com"))
	})

	t.Run("local object id derives author from note attribution", func(t *testing.T) {
		status := &Status{
			StatusID: "status-1",
			Note: &activitypub.Note{
				AttributedTo: "https://example.com/users/alice",
			},
		}
		assert.Equal(t,
			"https://example.com/users/alice/statuses/status-1",
			localActivityPubObjectIDForStatus(status, "example.com"),
		)
	})

	t.Run("local object id refuses acct-style remote author", func(t *testing.T) {
		status := &Status{StatusID: "status-1", AuthorUsername: "alice@remote.example"}
		assert.Empty(t, localActivityPubObjectIDForStatus(status, "example.com"))
	})

	t.Run("local object id handles missing essentials", func(t *testing.T) {
		assert.Empty(t, localActivityPubObjectIDForStatus(nil, "example.com"))
		assert.Empty(t, localActivityPubObjectIDForStatus(&Status{AuthorUsername: "alice"}, "example.com"))
		assert.Empty(t, localActivityPubObjectIDForStatus(&Status{StatusID: "status-1", AuthorUsername: "alice"}, ""))
		assert.Empty(t, localActivityPubObjectIDForStatus(&Status{StatusID: "status-1"}, "example.com"))
	})

	t.Run("username extraction handles urls and raw identifiers", func(t *testing.T) {
		assert.Empty(t, usernameFromActorIDForStatusIdentity(""))
		assert.Equal(t, "alice", usernameFromActorIDForStatusIdentity("https://example.com/users/alice"))
		assert.Equal(t, "urn:actor:alice", usernameFromActorIDForStatusIdentity("urn:actor:alice"))
	})

	t.Run("http status url detection", func(t *testing.T) {
		assert.True(t, isHTTPStatusURL("https://example.com/users/alice/statuses/1"))
		assert.True(t, isHTTPStatusURL("http://example.com/users/alice/statuses/1"))
		assert.False(t, isHTTPStatusURL("mailto:alice@example.com"))
		assert.False(t, isHTTPStatusURL("not a url"))
	})

	t.Run("normalized local domain strips scheme port and path", func(t *testing.T) {
		assert.Equal(t, "example.com", normalizedLocalDomain("https://Example.com:443/base"))
		assert.Equal(t, "example.com", normalizedLocalDomain("http://Example.com:80"))
	})
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
