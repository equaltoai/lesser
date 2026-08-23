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
