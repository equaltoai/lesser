package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func TestEditorialMediaGraphQLDraftExercise(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.persistMedia = true
	ctx := round12AuthContext("alice")
	mut := resolver.Mutation()
	qry := resolver.Query()

	tool := "illustration-suite"
	rights := "commissioned for this article"
	description := "global description"
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	upload, err := mut.UploadMedia(ctx, model.UploadMediaInput{
		File: graphql.Upload{
			File:     &round12ReadSeekCloser{Reader: bytes.NewReader(pngBytes)},
			Filename: "hero.png",
		},
		Description: &description,
		EditorialProvenance: &model.EditorialMediaProvenanceInput{
			Origin:             model.EditorialMediaOriginIllustrated,
			Tool:               &tool,
			SourceReferences:   []string{"https://example.com/reference"},
			RightsLicenseNotes: &rights,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, upload)
	require.NotNil(t, upload.Media)
	require.Empty(t, upload.Media.URL, "internal media must not receive an unsigned CDN URL")

	title := "Media draft"
	draft, err := mut.CreateDraft(ctx, model.CreateDraftInput{
		ContentType: model.ObjectTypeArticle,
		Title:       &title,
		Content:     "# Media draft",
	})
	require.NoError(t, err)

	caption := "Launch artwork"
	credit := "Illustration by Alice"
	alt := "A rocket leaving a violet planet"
	draft, err = mut.SetDraftEditorialMedia(ctx, draft.ID, []*model.EditorialMediaUsageInput{{
		MediaID:    upload.UploadID,
		Role:       model.EditorialMediaRoleHero,
		Caption:    &caption,
		CreditLine: &credit,
		AltText:    &alt,
	}})
	require.NoError(t, err)
	require.Len(t, draft.EditorialMedia, 1)
	require.Equal(t, model.EditorialMediaRoleHero, draft.EditorialMedia[0].Role)
	require.Equal(t, alt, *draft.EditorialMedia[0].EffectiveAltText)
	require.NotNil(t, draft.EditorialMedia[0].Provenance)

	preview, err := qry.DraftPreview(ctx, draft.ID, boolPtr(true))
	require.NoError(t, err)
	require.Len(t, preview.EditorialMedia, 1)
	hero := preview.EditorialMedia[0]
	require.Equal(t, model.EditorialMediaRoleHero, hero.Role)
	require.Equal(t, model.EditorialMediaStateReady, hero.State)
	require.Equal(t, caption, *hero.Caption)
	require.Equal(t, credit, *hero.CreditLine)
	require.Equal(t, alt, *hero.EffectiveAltText)
	require.NotNil(t, hero.AccessURL)
	require.Contains(t, *hero.AccessURL, "signature=review")
	require.True(t, strings.HasPrefix(*hero.ContentHash, "sha256:"))
	require.Equal(t, *hero.ContentHash, hero.Provenance.ContentIntegrity)

	access, err := qry.DraftEditorialMediaAccess(ctx, draft.ID, upload.UploadID)
	require.NoError(t, err)
	require.Equal(t, upload.UploadID, access.MediaID)
	require.Equal(t, *hero.ContentHash, access.ContentHash)

	publicRead, err := qry.Media(context.Background(), upload.UploadID)
	require.Error(t, err)
	require.Nil(t, publicRead)
}

func TestEditorialMediaPreviewStatesAreConspicuous(t *testing.T) {
	resolver := &Resolver{}
	position := 0
	hash := "sha256:" + strings.Repeat("a", 64)
	usage := models.DraftMediaUsage{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position}

	missing := resolver.convertCMSEditorialMediaBinding(context.Background(), cms.DraftEditorialMediaBinding{Usage: usage}, false, nil)
	require.Equal(t, model.EditorialMediaStateMissing, missing.State)

	processing := resolver.convertCMSEditorialMediaBinding(context.Background(), cms.DraftEditorialMediaBinding{
		Usage: usage,
		Media: &models.Media{
			MediaID: "inline", UserID: "alice", Visibility: models.MediaVisibilityInternal,
			Status: models.StatusPending, ContentHash: hash,
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "alice",
				RecordedAt: time.Now(), ContentIntegrity: hash,
			},
		},
	}, false, nil)
	require.Equal(t, model.EditorialMediaStateProcessing, processing.State)

	rejected := resolver.convertCMSEditorialMediaBinding(context.Background(), cms.DraftEditorialMediaBinding{
		Usage: usage,
		Media: &models.Media{
			MediaID: "inline", UserID: "alice", Visibility: models.MediaVisibilityInternal,
			Status: models.StatusFailed, ContentHash: hash,
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "alice",
				RecordedAt: time.Now(), ContentIntegrity: hash,
			},
		},
	}, false, nil)
	require.Equal(t, model.EditorialMediaStateRejected, rejected.State)
}

func TestEditorialMediaMintingGranularityForListAndSingleDraftContexts(t *testing.T) {
	resolver, drafts, state := newEditorialMediaReviewResolver(t)
	now := time.Now().UTC()
	hash := "sha256:" + strings.Repeat("b", 64)
	state.seededMedia = map[string]*models.Media{
		"ready": {
			MediaID: "ready", UserID: "owner", Visibility: models.MediaVisibilityInternal,
			Status: models.StatusReady, ContentType: "image/png", ContentHash: hash,
			S3Bucket: "media-private", S3Key: "owner/ready.png",
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "owner",
				RecordedAt: now, ContentIntegrity: hash,
			},
		},
	}
	draft := &models.Draft{
		ID: "mint-granularity", AuthorID: "owner", ContentType: "Article",
		Title: "Mint granularity", Content: "# Body", ContentFormat: "markdown", Status: "draft",
		EditorialMedia: []models.DraftMediaUsage{
			{MediaID: "ready", Role: models.EditorialMediaRoleHero},
			{MediaID: "gone", Role: models.EditorialMediaRoleSocialCard},
		},
		CreatedAt: now, UpdatedAt: now, LastSavedAt: now,
	}
	require.NoError(t, drafts.CreateDraft(context.Background(), draft))
	expiresAt := now.Add(time.Hour)
	grant := &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: draft.ID, Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &expiresAt,
		SK:     "GRANT#mint-granularity#REVIEWER#reviewer",
		GSI2SK: "TIME#2026-08-23T00:00:00Z#OWNER#owner#DRAFT#mint-granularity",
	}
	drafts.ownedDraftReviews = []*models.DraftReviewGrant{grant}
	drafts.sharedDraftReviews = []*models.DraftReviewGrant{grant}
	state.presignErr = errors.New("presigner unavailable")

	owned, err := resolver.Query().MyDraftReviews(round12AuthContext("owner"), nil, nil)
	require.NoError(t, err)
	require.Len(t, owned.Edges, 1)
	requireEditorialMediaWithoutAccess(t, owned.Edges[0].Node.EditorialMedia)
	require.Zero(t, state.presignCalls, "owned review lists must not mint bearer URLs")

	shared, err := resolver.Query().SharedDraftReviews(round12AuthContext("reviewer"), nil, nil)
	require.NoError(t, err)
	require.Len(t, shared.Edges, 1)
	requireEditorialMediaWithoutAccess(t, shared.Edges[0].Node.EditorialMedia)
	require.Zero(t, state.presignCalls, "shared review lists must not mint bearer URLs")

	// Default reads do NOT mint per-asset URLs (fold-in a): the projection is
	// URL-free unless the caller explicitly requests includeAccessUrls.
	defaultReview, err := resolver.Query().DraftReview(round12AuthContext("owner"), draft.ID, nil)
	require.NoError(t, err)
	requireEditorialMediaWithoutAccess(t, defaultReview.EditorialMedia)
	require.Zero(t, state.presignCalls, "a default draftReview read must not mint bearer URLs")

	defaultPreview, err := resolver.Query().DraftPreview(round12AuthContext("owner"), draft.ID, nil)
	require.NoError(t, err)
	requireEditorialMediaWithoutAccess(t, defaultPreview.EditorialMedia)
	require.Zero(t, state.presignCalls, "a default draftPreview read must not mint bearer URLs")

	// An explicit includeAccessUrls=true read mints only for the present
	// internal bindings (one per read), never for the missing binding.
	single, err := resolver.Query().DraftReview(round12AuthContext("owner"), draft.ID, boolPtr(true))
	require.NoError(t, err, "one failed binding mint must not fail a single-draft review")
	requireEditorialMediaWithoutAccess(t, single.EditorialMedia)
	require.Equal(t, 1, state.presignCalls, "only the present internal binding should attempt a mint")

	preview, err := resolver.Query().DraftPreview(round12AuthContext("owner"), draft.ID, boolPtr(true))
	require.NoError(t, err, "one failed binding mint must not fail a draft preview")
	requireEditorialMediaWithoutAccess(t, preview.EditorialMedia)
	require.Equal(t, 2, state.presignCalls)
}

func newEditorialMediaReviewResolver(
	t *testing.T,
) (*Resolver, *cursorRecordingDraftRepository, *round12PermissiveQueryState) {
	t.Helper()

	base, storage, _, _, state := newRound12GraphResolverWithMocks(t)
	drafts := &cursorRecordingDraftRepository{DraftRepository: inmemory.NewDraftRepository()}
	wrapped := &cursorResolverStorage{RepositoryStorage: storage, draft: drafts}
	registry, err := services.NewRegistry(
		services.WithStorage(wrapped),
		services.WithPublisher(base.Registry.GetPublisher()),
		services.WithLogger(base.Registry.GetLogger()),
		services.WithMediaS3Service(round12MediaS3Service{state: state}),
		services.WithConfig(base.Registry.GetConfig()),
	)
	require.NoError(t, err)

	return &Resolver{
		Registry: registry,
		Config:   base.Config,
		Storage:  wrapped,
		Logger:   base.Logger,
	}, drafts, state
}

func requireEditorialMediaWithoutAccess(t *testing.T, usages []*model.EditorialMediaUsage) {
	t.Helper()
	require.Len(t, usages, 2)
	require.Equal(t, model.EditorialMediaStateReady, usages[0].State)
	require.Nil(t, usages[0].AccessURL)
	require.Nil(t, usages[0].AccessExpiresAt)
	require.Equal(t, model.EditorialMediaStateMissing, usages[1].State)
	require.Nil(t, usages[1].AccessURL)
	require.Nil(t, usages[1].AccessExpiresAt)
}

// boolPtr converts a bool to a pointer for resolver argument calls.
func boolPtr(value bool) *bool {
	return &value
}
