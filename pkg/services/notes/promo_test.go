package notes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	svcErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testinginmemory "github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func digestN(n byte) string {
	return "sha256:" + strings.Repeat(string(rune('a'+n)), 64)
}

func publishedPromoMedia(id, owner, digest string) *models.Media {
	now := time.Now().UTC()
	return &models.Media{
		MediaID:        id,
		Version:        "original",
		UserID:         owner,
		FileName:       "hero.png",
		ContentType:    "image/png",
		FileSize:       1024,
		ContentHash:    digest,
		Status:         "ready",
		Visibility:     models.MediaVisibilityInternal,
		Provenance:     &models.MediaProvenance{Origin: models.EditorialMediaOriginAIGenerated, Tool: "image tool", ContentIntegrity: digest, ResponsibleActor: owner, RecordedAt: now},
		EditorialState: models.EditorialLifecycleAvailable,
		PublishedS3Key: "published/media/" + id + ".png",
		PublishedURL:   "https://cdn.example/published/" + id + ".png",
		PublishedAt:    &now,
		MediaCategory:  models.MediaCategoryImage,
		UploadedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
		Width:          800,
		Height:         600,
	}
}

func newPromoNotesService(t *testing.T) (*Service, *testinginmemory.MediaRepository) {
	t.Helper()
	service, _, _, _, _ := newNotesServiceHarness(t)
	mediaRepo := testinginmemory.NewMediaRepository()
	service.mediaRepo = mediaRepo
	service.logger = zap.NewNop()
	return service, mediaRepo
}

func TestService_CreatePromoNoteReleasesWithExactPublishedAssetsAndDisclosure(t *testing.T) {
	ctx := context.Background()
	service, mediaRepo := newPromoNotesService(t)
	digest := digestN(0)
	require.NoError(t, mediaRepo.CreateMedia(ctx, publishedPromoMedia("media-1", "alice", digest)))

	disclosure := &activitypub.AgentPostAttribution{
		TriggerType:   "manual",
		ApprovedBy:    "https://example.com/users/principal",
		SchemaVersion: activitypub.AgentAttributionSchemaVersion,
	}
	created, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:         "alice",
		Content:          "Read our launch article",
		Visibility:       models.VisibilityPublic,
		AgentAttribution: disclosure,
	}, []PromoPublishedMediaRef{{MediaID: "media-1", ContentHash: digest}})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.Note)
	require.NotEmpty(t, created.Note.StatusID)
	require.NotNil(t, created.Note.Note)

	note := created.Note.Note
	require.Len(t, note.Attachment, 1)
	require.Equal(t, "https://cdn.example/published/media-1.png", note.Attachment[0].URL,
		"the attachment must use the M2 durable published serving, not the CDNUrl or a fallback")
	require.Equal(t, "Image", note.Attachment[0].Type)
	require.Equal(t, "image/png", note.Attachment[0].MediaType)
	require.Equal(t, 800, note.Attachment[0].Width)
	require.Equal(t, "hero.png", note.Attachment[0].Value)

	// AI-authorship disclosure survives to the outbound surface.
	require.NotNil(t, note.AgentAttribution)
	require.Equal(t, "manual", note.AgentAttribution.TriggerType)
	require.Equal(t, "https://example.com/users/principal", note.AgentAttribution.ApprovedBy)

	require.Equal(t, models.VisibilityPublic, created.Note.Visibility)
	require.Equal(t, 1, created.Note.MediaCount)
}

func TestService_CreatePromoNoteSupportsUnlisted(t *testing.T) {
	ctx := context.Background()
	service, mediaRepo := newPromoNotesService(t)
	digest := digestN(1)
	require.NoError(t, mediaRepo.CreateMedia(ctx, publishedPromoMedia("media-1", "alice", digest)))

	created, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "quiet promotion",
		Visibility: models.VisibilityUnlisted,
	}, []PromoPublishedMediaRef{{MediaID: "media-1", ContentHash: digest}})
	require.NoError(t, err)
	require.Equal(t, models.VisibilityUnlisted, created.Note.Visibility)
	require.Len(t, created.Note.Note.Attachment, 1)
}

func TestService_CreatePromoNoteRestrictsVisibility(t *testing.T) {
	ctx := context.Background()
	service, _ := newPromoNotesService(t)
	for _, visibility := range []string{models.VisibilityPrivate, models.VisibilityDirect, "", "followers"} {
		_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
			AuthorID:   "alice",
			Content:    "promo",
			Visibility: visibility,
		}, []PromoPublishedMediaRef{{MediaID: "media-1", ContentHash: digestN(0)}})
		require.ErrorIs(t, err, ErrPromoVisibilityRestricted, "visibility %q must be structurally rejected", visibility)
	}
}

func TestService_CreatePromoNoteRequiresPublishedAssets(t *testing.T) {
	ctx := context.Background()
	service, _ := newPromoNotesService(t)
	_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one published asset")
}

func TestService_CreatePromoNoteRejectsUnpublishedAsset(t *testing.T) {
	ctx := context.Background()
	service, mediaRepo := newPromoNotesService(t)
	digest := digestN(2)
	// Internal editorial asset that never crossed the M2 publish transition:
	// structurally ineligible for outbound attachment.
	unpublished := publishedPromoMedia("media-1", "alice", digest)
	unpublished.PublishedURL = ""
	unpublished.PublishedS3Key = ""
	unpublished.PublishedAt = nil
	require.NoError(t, mediaRepo.CreateMedia(ctx, unpublished))

	_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, []PromoPublishedMediaRef{{MediaID: "media-1", ContentHash: digest}})
	require.ErrorIs(t, err, ErrPromoAssetNotPublished, "internal/unpublished bytes must never attach to an outbound post")
}

func TestService_CreatePromoNoteRejectsDigestMismatch(t *testing.T) {
	ctx := context.Background()
	service, mediaRepo := newPromoNotesService(t)
	digest := digestN(3)
	require.NoError(t, mediaRepo.CreateMedia(ctx, publishedPromoMedia("media-1", "alice", digest)))

	// The reviewed package bound a different digest: the live bytes no longer
	// match the reviewed bytes, so release must fail closed (no substitution).
	_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, []PromoPublishedMediaRef{{MediaID: "media-1", ContentHash: digestN(4)}})
	require.ErrorIs(t, err, ErrPromoAssetDigestMismatch)
}

func TestService_CreatePromoNoteRejectsForeignAsset(t *testing.T) {
	ctx := context.Background()
	service, mediaRepo := newPromoNotesService(t)
	digest := digestN(5)
	require.NoError(t, mediaRepo.CreateMedia(ctx, publishedPromoMedia("media-1", "bob", digest)))

	_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, []PromoPublishedMediaRef{{MediaID: "media-1", ContentHash: digest}})
	require.ErrorIs(t, err, svcErrors.ErrMediaAttachmentNotFound)
}

func TestService_CreatePromoNoteRejectsMissingAndDuplicatedAssets(t *testing.T) {
	ctx := context.Background()
	service, _ := newPromoNotesService(t)
	digest := digestN(6)

	_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, []PromoPublishedMediaRef{{MediaID: "missing", ContentHash: digest}})
	require.Error(t, err)
	require.True(t, errors.Is(err, svcErrors.ErrMediaAttachmentNotFound))

	_, err = service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, []PromoPublishedMediaRef{
		{MediaID: "media-1", ContentHash: digest},
		{MediaID: "media-1", ContentHash: digest},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, svcErrors.ErrMediaAttachmentNotFound))
}

func TestService_CreatePromoNoteCapsAttachmentCount(t *testing.T) {
	ctx := context.Background()
	service, mediaRepo := newPromoNotesService(t)
	refs := make([]PromoPublishedMediaRef, 0, 5)
	for i := 0; i < 5; i++ {
		id := "media-" + string(rune('1'+i))
		digest := digestN(byte(i))
		require.NoError(t, mediaRepo.CreateMedia(ctx, publishedPromoMedia(id, "alice", digest)))
		refs = append(refs, PromoPublishedMediaRef{MediaID: id, ContentHash: digest})
	}
	_, err := service.CreatePromoNote(ctx, &CreateNoteCommand{
		AuthorID:   "alice",
		Content:    "promo",
		Visibility: models.VisibilityPublic,
	}, refs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "promo_media")
}
