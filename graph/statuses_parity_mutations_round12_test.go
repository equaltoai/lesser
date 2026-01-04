package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_StatusesParity_UpdateMuteUnmute(t *testing.T) {
	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	mutations := &mutationResolver{resolver}

	// Seed a status in the in-memory status repository for Notes service to load.
	statusRepo := storage.Status()
	require.NotNil(t, statusRepo)

	statusID := "status-1"
	now := time.Now()
	require.NoError(t, statusRepo.CreateStatus(context.Background(), &models.Status{
		StatusID:       statusID,
		AuthorID:       "https://localhost/users/alice",
		AuthorUsername: "alice",
		Content:        "old content",
		Sensitive:      false,
		Language:       "en",
		Visibility:     models.VisibilityPublic,
		PublishedAt:    now.Add(-time.Hour),
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now.Add(-time.Hour),
		ModifiedAt:     now.Add(-time.Hour),
		Note: &models.NoteField{Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:        "https://localhost/users/alice/statuses/status-1",
				Type:      activitypub.NoteType,
				Summary:   "old spoiler",
				Sensitive: false,
			},
			Content: "old content",
		}},
	}))

	// Auth required.
	_, err := mutations.UpdateStatus(context.Background(), statusID, model.UpdateStatusInput{Content: "new"})
	require.Error(t, err)

	ctx := round12AuthContext("alice")

	// Validation required.
	_, err = mutations.UpdateStatus(ctx, "   ", model.UpdateStatusInput{Content: "new"})
	require.Error(t, err)

	// Update succeeds and returns an object.
	newSpoiler := "new spoiler"
	newLang := "fr"
	updated, err := mutations.UpdateStatus(ctx, statusID, model.UpdateStatusInput{
		Content:     "new content",
		Sensitive:   ptrBool(true),
		SpoilerText: &newSpoiler,
		Language:    &newLang,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	// Mute/unmute succeeds.
	dur := 60
	ok, err := mutations.MuteStatus(ctx, statusID, &dur)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = mutations.UnmuteStatus(ctx, statusID)
	require.NoError(t, err)
	require.True(t, ok)
}
