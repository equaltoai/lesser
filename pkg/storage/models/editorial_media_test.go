package models

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMediaInternalEditorialValidationBindsProvenanceAndSuppressesCDN(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	media := &Media{
		MediaID:       "media-1",
		UserID:        "alice",
		FileName:      "hero.png",
		ContentType:   "image/png",
		FileSize:      12,
		ContentHash:   digest,
		CDNUrl:        "https://cdn.example/media/hero.png",
		Visibility:    MediaVisibilityInternal,
		MediaCategory: MediaCategoryImage,
		Provenance: &MediaProvenance{
			Origin:           EditorialMediaOriginAIGenerated,
			Tool:             "  image tool  ",
			SourceReferences: []string{" ref-a ", "ref-a", ""},
		},
	}

	require.NoError(t, media.BeforeCreate())
	require.Empty(t, media.CDNUrl)
	require.Equal(t, "alice", media.Provenance.ResponsibleActor)
	require.Equal(t, digest, media.Provenance.ContentIntegrity)
	require.Equal(t, "image tool", media.Provenance.Tool)
	require.Equal(t, []string{"ref-a"}, media.Provenance.SourceReferences)
	require.False(t, media.Provenance.RecordedAt.IsZero())
	require.True(t, media.IsInternalEditorial())
}

func TestMediaInternalEditorialValidationRejectsMissingProvenance(t *testing.T) {
	media := &Media{
		MediaID: "media-1", UserID: "alice", FileName: "hero.png", ContentType: "image/png", FileSize: 12,
		ContentHash: "sha256:" + strings.Repeat("a", 64), Visibility: MediaVisibilityInternal,
	}
	require.ErrorContains(t, media.BeforeCreate(), "requires provenance")
}

func TestMediaEditorialLifecycleValidation(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	internal := func() *Media {
		return &Media{
			MediaID: "media-1", UserID: "alice", FileName: "hero.png", ContentType: "image/png", FileSize: 12,
			ContentHash: digest, Visibility: MediaVisibilityInternal,
			Provenance: &MediaProvenance{Origin: EditorialMediaOriginSupplied, ContentIntegrity: digest},
		}
	}

	available := internal()
	available.EditorialState = EditorialLifecycleAvailable
	require.NoError(t, available.BeforeCreate())
	require.True(t, available.EditorialLifecycleAvailableForPublish())

	withdrawn := internal()
	withdrawn.EditorialState = "  WITHDRAWN "
	require.NoError(t, withdrawn.BeforeCreate())
	require.Equal(t, EditorialLifecycleWithdrawn, withdrawn.EditorialState)
	require.False(t, withdrawn.EditorialLifecycleAvailableForPublish())

	superseded := internal()
	superseded.EditorialState = EditorialLifecycleSuperseded
	require.ErrorContains(t, superseded.BeforeCreate(), "must name the superseding asset")
	superseded.SupersededByMediaID = "media-2"
	require.NoError(t, superseded.BeforeCreate())
	require.False(t, superseded.EditorialLifecycleAvailableForPublish())

	unavailable := internal()
	unavailable.EditorialState = EditorialLifecycleUnavailable
	require.NoError(t, unavailable.BeforeCreate())
	require.False(t, unavailable.EditorialLifecycleAvailableForPublish())

	unknown := internal()
	unknown.EditorialState = EditorialLifecycle("hidden")
	require.ErrorContains(t, unknown.BeforeCreate(), "invalid editorial lifecycle")

	orphan := internal()
	orphan.SupersededByMediaID = "media-2"
	require.ErrorContains(t, orphan.BeforeCreate(), "requires the superseded lifecycle")

	publicWithLifecycle := internal()
	publicWithLifecycle.Visibility = MediaVisibilityPublic
	publicWithLifecycle.EditorialState = EditorialLifecycleWithdrawn
	require.ErrorContains(t, publicWithLifecycle.BeforeCreate(), "require internal editorial media")

	publishedOnPublic := internal()
	publishedOnPublic.Visibility = MediaVisibilityPublic
	publishedAt := time.Now().UTC()
	publishedOnPublic.PublishedURL = "https://cdn.example/published.png"
	publishedOnPublic.PublishedAt = &publishedAt
	require.ErrorContains(t, publishedOnPublic.BeforeCreate(), "require internal editorial media")

	published := internal()
	publishedAt = time.Now().UTC()
	published.PublishedURL = "https://cdn.example/published.png"
	published.PublishedS3Key = "published/media/hero.png"
	published.PublishedAt = &publishedAt
	require.NoError(t, published.BeforeCreate())
	require.True(t, published.IsPublished())
	require.False(t, internal().IsPublished(), "pre-publish internal assets are not durably served")
}

func TestNormalizeDraftMediaUsagesEnforcesModeledRoles(t *testing.T) {
	position := 2
	got, err := NormalizeDraftMediaUsages([]DraftMediaUsage{
		{MediaID: " hero ", Role: "HERO", Caption: " Cover ", CreditLine: " Credit ", AltText: " Alt "},
		{MediaID: "inline", Role: "INLINE", InlinePosition: &position},
		{MediaID: "card", Role: "SOCIAL_CARD"},
	})
	require.NoError(t, err)
	require.Equal(t, EditorialMediaRoleHero, got[0].Role)
	require.Equal(t, "hero", got[0].MediaID)
	require.Equal(t, "Cover", got[0].Caption)
	require.Equal(t, 2, *got[1].InlinePosition)

	_, err = NormalizeDraftMediaUsages([]DraftMediaUsage{{MediaID: "a", Role: EditorialMediaRoleInline}})
	require.ErrorContains(t, err, "inline position")
	_, err = NormalizeDraftMediaUsages([]DraftMediaUsage{
		{MediaID: "a", Role: EditorialMediaRoleHero},
		{MediaID: "b", Role: EditorialMediaRoleHero},
	})
	require.ErrorContains(t, err, "only one")
	_, err = NormalizeDraftMediaUsages([]DraftMediaUsage{
		{MediaID: "same", Role: EditorialMediaRoleHero},
		{MediaID: "same", Role: EditorialMediaRoleSocialCard},
	})
	require.ErrorContains(t, err, "bound more than once")
}

func TestMediaProvenancePreservesSourceTimestampsAndRecordedTime(t *testing.T) {
	created := time.Date(2026, 8, 1, 2, 3, 4, 0, time.FixedZone("offset", 3600))
	recorded := time.Date(2026, 8, 2, 3, 4, 5, 0, time.UTC)
	p := &MediaProvenance{
		Origin: EditorialMediaOriginPhotographed, ResponsibleActor: "alice", CreatedAt: &created, RecordedAt: recorded,
	}
	require.NoError(t, p.Normalize("alice", "sha256:"+strings.Repeat("b", 64), time.Now()))
	require.Equal(t, recorded, p.RecordedAt)
	require.Equal(t, created.UTC(), *p.CreatedAt)
}

func TestMediaProvenanceRejectsContentIntegrityRebinding(t *testing.T) {
	p := &MediaProvenance{
		Origin: EditorialMediaOriginSupplied, ResponsibleActor: "alice",
		ContentIntegrity: "sha256:" + strings.Repeat("a", 64),
	}
	err := p.Normalize("alice", "sha256:"+strings.Repeat("b", 64), time.Now())
	require.ErrorContains(t, err, "does not match")
}
