package inmemory

import (
	"context"
	"testing"

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
