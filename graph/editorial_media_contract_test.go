package graph

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage/models"
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

	preview, err := qry.DraftPreview(ctx, draft.ID)
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
