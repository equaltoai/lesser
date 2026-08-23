package cms

import (
	"context"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type editorialMediaMemRepo struct {
	items map[string]*models.Media
}

func (r *editorialMediaMemRepo) GetMedia(_ context.Context, mediaID string) (*models.Media, error) {
	media := r.items[mediaID]
	if media == nil {
		return nil, storage.ErrNotFound
	}
	copy := *media
	return &copy, nil
}

func internalEditorialMedia(id, owner string) *models.Media {
	hash := "sha256:" + strings.Repeat("a", 64)
	return &models.Media{
		MediaID: id, UserID: owner, Visibility: models.MediaVisibilityInternal, ContentHash: hash,
		Provenance: &models.MediaProvenance{
			Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: owner, ContentIntegrity: hash,
		},
	}
}

func TestDraftEditorialMediaEnforcesOwnershipAndExactGrantAccess(t *testing.T) {
	svc, repo := newReviewService(t)
	mediaRepo := &editorialMediaMemRepo{items: map[string]*models.Media{
		"hero":    internalEditorialMedia("hero", "owner"),
		"foreign": internalEditorialMedia("foreign", "someone-else"),
		"public":  {MediaID: "public", UserID: "owner", Visibility: models.MediaVisibilityPublic},
	}}
	svc.SetEditorialMediaRepository(mediaRepo)
	ctx := context.Background()

	position := 1
	draft, err := svc.SetEditorialMedia(ctx, "owner", "d1", []models.DraftMediaUsage{
		{MediaID: "hero", Role: models.EditorialMediaRoleHero, Caption: "Launch", AltText: "A launch illustration"},
		{MediaID: "missing-later", Role: models.EditorialMediaRoleInline, InlinePosition: &position},
	})
	require.Error(t, err, "assets must exist when first attached")
	require.Nil(t, draft)

	draft, err = svc.SetEditorialMedia(ctx, "owner", "d1", []models.DraftMediaUsage{
		{MediaID: "hero", Role: models.EditorialMediaRoleHero, Caption: "Launch", AltText: "A launch illustration"},
	})
	require.NoError(t, err)
	require.Len(t, draft.EditorialMedia, 1)

	_, err = svc.SetEditorialMedia(ctx, "owner", "d1", []models.DraftMediaUsage{{MediaID: "foreign", Role: models.EditorialMediaRoleHero}})
	require.ErrorContains(t, err, "does not belong")
	_, err = svc.SetEditorialMedia(ctx, "owner", "d1", []models.DraftMediaUsage{{MediaID: "public", Role: models.EditorialMediaRoleHero}})
	require.ErrorContains(t, err, "integrity-bound internal")

	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)
	bound, err := svc.BoundEditorialMediaForCaller(ctx, "reviewer", "d1", "hero")
	require.NoError(t, err)
	require.Equal(t, "hero", bound.MediaID)
	_, err = svc.BoundEditorialMediaForCaller(ctx, "reviewer", "d1", "foreign")
	require.ErrorContains(t, err, "not bound")
	_, err = svc.BoundEditorialMediaForCaller(ctx, "stranger", "d1", "hero")
	require.Error(t, err)

	require.NoError(t, svc.RevokeDraftReview(ctx, "owner", "d1", "reviewer"))
	_, err = svc.BoundEditorialMediaForCaller(ctx, "reviewer", "d1", "hero")
	require.Error(t, err)

	// A previously bound object can disappear asynchronously; preview preserves
	// the association as an explicit missing state rather than dropping it.
	require.NoError(t, func() error {
		_, shareErr := svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
		return shareErr
	}())
	delete(mediaRepo.items, "hero")
	_, bindings, err := svc.DraftEditorialMediaForCaller(ctx, "reviewer", "d1")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Nil(t, bindings[0].Media)
	require.Equal(t, "hero", bindings[0].Usage.MediaID)

	_ = repo // review repository remains part of the active-grant exercise above
}
