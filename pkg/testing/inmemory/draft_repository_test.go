package inmemory

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestDraftRepositoryEditorialMediaUsesFieldScopedWriter(t *testing.T) {
	ctx := context.Background()
	repo := NewDraftRepository()
	stored := &models.Draft{
		AuthorID: "owner",
		ID:       "draft-1",
		Content:  "original content",
		EditorialMedia: []models.DraftMediaUsage{
			{MediaID: "original", Role: models.EditorialMediaRoleHero},
		},
	}
	require.NoError(t, repo.CreateDraft(ctx, stored))

	incoming := &models.Draft{
		AuthorID: "owner",
		ID:       "draft-1",
		Content:  "updated content",
		EditorialMedia: []models.DraftMediaUsage{
			{MediaID: "stale", Role: models.EditorialMediaRoleSocialCard},
		},
	}
	require.NoError(t, repo.UpdateDraft(ctx, "owner", incoming))

	updated, err := repo.GetDraft(ctx, "owner", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "updated content", updated.Content)
	require.Equal(t, stored.EditorialMedia, updated.EditorialMedia,
		"full-model updates must preserve the stored editorial-media association")

	replacement := []models.DraftMediaUsage{
		{MediaID: "replacement", Role: models.EditorialMediaRoleSocialCard},
	}
	incoming.EditorialMedia = replacement
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", incoming))
	require.Equal(t, replacement, updated.EditorialMedia)

	incoming.EditorialMedia = nil
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", incoming))
	require.Empty(t, updated.EditorialMedia, "an empty field-scoped update must clear the association")
}

// TestDraftRepositoryStatusPaginationPastEndCursorTerminates proves the
// in-memory doubles terminate on a past-end cursor: when no draft's GSI4SK
// exceeds the cursor, the page must be empty with an empty next cursor
// (matching the production repository), not a re-emitted first page that would
// loop the caller forever.
func TestDraftRepositoryStatusPaginationPastEndCursorTerminates(t *testing.T) {
	ctx := context.Background()
	repo := NewDraftRepository()
	base := time.Now().UTC().Add(-25 * time.Hour)

	for i, id := range []string{"f1", "f2", "f3"} {
		draft := &models.Draft{
			AuthorID: "owner", ID: id, Status: "failed", Content: "c", ContentFormat: "markdown",
			UpdatedAt: base.Add(time.Duration(i+1) * time.Hour),
		}
		require.NoError(t, draft.UpdateKeys())
		require.NoError(t, repo.CreateDraft(ctx, draft))
	}
	scheduledAt := time.Now().UTC().Add(30 * time.Minute)
	scheduled := &models.Draft{
		AuthorID: "owner", ID: "s1", Status: "scheduled", Content: "c", ContentFormat: "markdown",
		ScheduledAt: &scheduledAt, UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, scheduled.UpdateKeys())
	require.NoError(t, repo.CreateDraft(ctx, scheduled))

	// Page one yields a next cursor; page two consumes it and terminates.
	page1, next1, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next1, "a full first page must emit a next cursor")

	page2, next2, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 2, next1)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, next2, "the final page must not emit a next cursor")

	// A past-end cursor returns an empty page with an empty next cursor.
	page3, next3, err := repo.ListDraftsByStatusPaginated(ctx, "failed", 2, "zzz-past-end")
	require.NoError(t, err)
	require.Empty(t, page3, "a past-end cursor must return an empty page")
	require.Empty(t, next3, "a past-end cursor must not re-emit a next cursor")

	// The pre-existing scheduled-drafts double has the same termination fix.
	past, nextPast, err := repo.ListScheduledDraftsDuePaginated(ctx, time.Now().UTC().Add(time.Hour), 2, "zzz-past-end")
	require.NoError(t, err)
	require.Empty(t, past, "a past-end cursor must return an empty page")
	require.Empty(t, nextPast, "a past-end cursor must not re-emit a next cursor")
}

// TestDraftRepositoryEditorialMediaCASEnforced proves the in-memory mirror
// enforces the same version-conditioned CAS as the production writer: a stale
// media-set snapshot conflicts instead of silently losing its update, and the
// winning bump is carried back to the caller's snapshot.
func TestDraftRepositoryEditorialMediaCASEnforced(t *testing.T) {
	ctx := context.Background()
	repo := NewDraftRepository()

	stored := &models.Draft{
		AuthorID: "owner",
		ID:       "draft-cas",
		Content:  "body",
		EditorialMedia: []models.DraftMediaUsage{
			{MediaID: "original", Role: models.EditorialMediaRoleHero},
		},
	}
	require.NoError(t, repo.CreateDraft(ctx, stored))

	first := &models.Draft{AuthorID: "owner", ID: "draft-cas"}
	first.EditorialMedia = []models.DraftMediaUsage{{MediaID: "alice", Role: models.EditorialMediaRoleHero}}
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", first))
	require.Equal(t, 1, first.ModelVersion, "the winning snapshot carries the bumped version")

	// A stale snapshot (version 0) must conflict.
	stale := &models.Draft{AuthorID: "owner", ID: "draft-cas"}
	stale.EditorialMedia = []models.DraftMediaUsage{{MediaID: "bob", Role: models.EditorialMediaRoleHero}}
	err := repo.UpdateDraftEditorialMedia(ctx, "owner", stale)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict), "stale media-set surfaces CONFLICT: %v", err)

	updated, err := repo.GetDraft(ctx, "owner", "draft-cas")
	require.NoError(t, err)
	require.Equal(t, "alice", updated.EditorialMedia[0].MediaID, "the losing write must not land")
	require.Equal(t, 1, updated.ModelVersion)
}
