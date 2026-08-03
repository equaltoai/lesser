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

func TestBuildCanonicalRemoteStatus_InvalidInputs(t *testing.T) {
	assert.Nil(t, BuildCanonicalRemoteStatus(nil, "local.example"))
	assert.Nil(t, BuildCanonicalRemoteStatus(&activitypub.Note{}, "local.example"))
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
