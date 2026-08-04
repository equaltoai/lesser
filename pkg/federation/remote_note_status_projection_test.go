package federation

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCanonicalRemoteStatus(t *testing.T) {
	publishedAt := time.Date(2025, 2, 1, 2, 3, 4, 0, time.UTC)
	updatedAt := publishedAt.Add(5 * time.Minute)
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://remote.example/users/bob/statuses/1",
			Type:      activitypub.NoteType,
			Published: &publishedAt,
			Updated:   &updatedAt,
		},
		Content:      "hello world",
		AttributedTo: "https://remote.example/users/bob",
	}

	status := BuildCanonicalRemoteStatus(note, "local.example")
	require.NotNil(t, status)
	assert.Equal(t, models.CanonicalStatusIDForDomain(note.ID, "local.example"), status.StatusID)
	assert.Equal(t, note.AttributedTo, status.AuthorID)
	assert.Equal(t, "bob@remote.example", status.AuthorUsername)
	assert.Equal(t, []string{note.ID}, status.URLs)
	assert.Equal(t, publishedAt, status.PublishedAt)
	assert.Equal(t, updatedAt, status.UpdatedAt)
	require.NotNil(t, status.Note)
	assert.NotSame(t, note, status.Note)

	status.Note.Content = "changed copy"
	assert.Equal(t, "hello world", note.Content)
}

func TestBuildCanonicalRemoteStatus_RejectsRemoteProjectionOfLocalAuthors(t *testing.T) {
	for _, attributedTo := range []string{
		"https://local.example/users/alice",
		"https://LOCAL.EXAMPLE/users/Alice/",
		"https://local.example:443/@alice",
		"https://local.example/users/alice?x=1",
		"https://local.example/users/alice#fragment",
		"https://user@local.example/users/alice",
		"https://local.example/foo/alice",
		"https://local.example/users/admin/alice",
		"https://local.example/api/v1/accounts/alice",
		"https://local.example/alice",
		"HTTP://local.example/users/alice",
		"HTTPS://local.example/users/alice",
		"HttPs://local.example/users/alice",
	} {
		t.Run(attributedTo, func(t *testing.T) {
			note := &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/users/mallory/statuses/2",
					Type: activitypub.NoteType,
				},
				AttributedTo: attributedTo,
			}

			assert.Nil(t, BuildCanonicalRemoteStatus(note, "https://local.example"))
		})
	}
}

func TestBuildCanonicalRemoteStatus_AllowsHonestRemoteActorPaths(t *testing.T) {
	for _, test := range []struct {
		name           string
		attributedTo   string
		authorUsername string
	}{
		{name: "bob", attributedTo: "https://remote.example/users/bob", authorUsername: "bob@remote.example"},
		{name: "remote username collision", attributedTo: "https://remote.example/users/alice", authorUsername: "alice@remote.example"},
		{name: "profile path", attributedTo: "https://remote.example/profile/carol", authorUsername: "carol@remote.example"},
		{name: "actor path", attributedTo: "https://remote.example/actor", authorUsername: "actor@remote.example"},
		{name: "api account path", attributedTo: "https://remote.example/api/v1/accounts/dave", authorUsername: "dave@remote.example"},
		{name: "IPv6 actor", attributedTo: "https://[2001:db8::1]/users/alice", authorUsername: "alice@2001:db8::1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			note := &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/objects/remote-path-control",
					Type: activitypub.NoteType,
				},
				AttributedTo: test.attributedTo,
			}

			status := BuildCanonicalRemoteStatus(note, "https://local.example")
			require.NotNil(t, status)
			assert.Equal(t, test.attributedTo, status.AuthorID)
			assert.Equal(t, test.authorUsername, status.AuthorUsername)
		})
	}
}

func TestBuildCanonicalRemoteStatus_RejectsAttributionWithoutUsableDomainAnchor(t *testing.T) {
	for _, attributedTo := range []string{
		"https://[::1]/users/alice",
		"http://:8443/users/alice",
	} {
		t.Run(attributedTo, func(t *testing.T) {
			note := &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://remote.example/objects/unanchored-attribution",
					Type: activitypub.NoteType,
				},
				AttributedTo: attributedTo,
			}
			assert.Nil(t, BuildCanonicalRemoteStatus(note, "local.example"))
		})
	}
}

func TestBuildCanonicalRemoteStatus_InvalidInputs(t *testing.T) {
	assert.Nil(t, BuildCanonicalRemoteStatus(nil, "local.example"))
	assert.Nil(t, BuildCanonicalRemoteStatus(&activitypub.Note{}, "local.example"))

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   "https://remote.example/objects/missing-domain-anchor",
			Type: activitypub.NoteType,
		},
		AttributedTo: "https://remote.example/users/bob",
	}
	for _, localDomain := range []string{"", " ", "://", "not a domain"} {
		t.Run("local domain "+localDomain, func(t *testing.T) {
			assert.Nil(t, BuildCanonicalRemoteStatus(note, localDomain))
		})
	}
}

func TestRemoteNoteStatusProjectionHelpers(t *testing.T) {
	assert.Equal(t, "", remoteStatusAuthorUsername(nil, "local.example"))
	assert.Nil(t, remoteStatusProjectionURLs(nil))
	assert.Empty(t, remoteStatusProjectionURLs(&activitypub.Note{}))

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{ID: " https://remote.example/users/casey/statuses/3 "},
	}
	assert.Equal(t, []string{"https://remote.example/users/casey/statuses/3"}, remoteStatusProjectionURLs(note))
}
