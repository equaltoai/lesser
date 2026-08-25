package graph

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestEditorialMediaLifecycleStatesAreConspicuous(t *testing.T) {
	resolver := &Resolver{}
	position := 0
	hash := "sha256:" + strings.Repeat("a", 64)
	usage := models.DraftMediaUsage{MediaID: "inline", Role: models.EditorialMediaRoleInline, InlinePosition: &position}
	media := func(lifecycle models.EditorialLifecycle) *models.Media {
		return &models.Media{
			MediaID: "inline", UserID: "alice", Visibility: models.MediaVisibilityInternal,
			Status: models.StatusReady, ContentHash: hash,
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "alice",
				RecordedAt: time.Now(), ContentIntegrity: hash,
			},
			EditorialState: lifecycle,
		}
	}

	cases := []struct {
		lifecycle models.EditorialLifecycle
		state     model.EditorialMediaState
	}{
		{models.EditorialLifecycleWithdrawn, model.EditorialMediaStateWithdrawn},
		{models.EditorialLifecycleSuperseded, model.EditorialMediaStateSuperseded},
		{models.EditorialLifecycleUnavailable, model.EditorialMediaStateUnavailable},
	}
	for _, tc := range cases {
		converted := resolver.convertCMSEditorialMediaBinding(context.Background(), cms.DraftEditorialMediaBinding{
			Usage: usage,
			Media: media(tc.lifecycle),
		}, false, nil)
		require.Equal(t, tc.state, converted.State, "lifecycle %q must be inspectable", tc.lifecycle)
	}

	// The durable published serving is exposed on the usage once minted.
	published := media("")
	publishedAt := time.Now().UTC()
	published.PublishedURL = "https://cdn.example.test/published/media/inline.png"
	published.PublishedS3Key = "published/media/inline.png"
	published.PublishedAt = &publishedAt
	converted := resolver.convertCMSEditorialMediaBinding(context.Background(), cms.DraftEditorialMediaBinding{Usage: usage, Media: published}, false, nil)
	require.Equal(t, model.EditorialMediaStateReady, converted.State)
	require.NotNil(t, converted.PublishedURL)
	require.Equal(t, published.PublishedURL, *converted.PublishedURL)
	require.NotNil(t, converted.PublishedAt)
}

func TestDraftReviewGrantExpirySurface(t *testing.T) {
	resolver := &Resolver{}
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	active := resolver.convertCMSDraftReviewGrant(context.Background(), &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: "d", Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &future,
	})
	require.Equal(t, model.DraftReviewGrantStatusActive, active.Status)
	require.NotNil(t, active.ExpiresAt)

	expired := resolver.convertCMSDraftReviewGrant(context.Background(), &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: "d", Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &past,
	})
	require.Equal(t, model.DraftReviewGrantStatusExpired, expired.Status)
	require.NotNil(t, expired.ExpiresAt)

	revokedAt := now.Add(-time.Minute)
	revoked := resolver.convertCMSDraftReviewGrant(context.Background(), &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: "d", Reviewer: "reviewer", GrantedAt: now, ExpiresAt: &future, RevokedAt: &revokedAt,
	})
	require.Equal(t, model.DraftReviewGrantStatusRevoked, revoked.Status, "revocation dominates expiry classification")
}

// m2FutureExpiry returns an expiry safely in the future for seeded review
// grants, keeping fail-closed expiry semantics honest in fixtures.
func m2FutureExpiry() *time.Time {
	value := time.Now().UTC().Add(2 * time.Hour)
	return &value
}

func TestUpdateEditorialMediaLifecycleMutationWiresFieldScopedWrite(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	now := time.Now().UTC()
	digest := "sha256:" + strings.Repeat("e", 64)
	state.seededMedia = map[string]*models.Media{
		"m1": {
			MediaID: "m1", UserID: "alice", ContentType: "image/png", FileSize: 12,
			ContentHash: digest, Status: "ready", Visibility: models.MediaVisibilityInternal,
			S3Bucket: "media-private", S3Key: "alice/m1.png",
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "alice",
				RecordedAt: now, ContentIntegrity: digest,
			},
		},
	}

	payload, err := resolver.Mutation().UpdateEditorialMediaLifecycle(
		round12AuthContext("alice"), "m1", model.EditorialMediaLifecycleWithdrawn, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, "m1", payload.MediaID)
	require.Equal(t, model.EditorialMediaLifecycleWithdrawn, payload.Lifecycle,
		"the mutation must drive the field-scoped lifecycle writer and read the state back")

	// A non-owner cannot withdraw an asset.
	_, err = resolver.Mutation().UpdateEditorialMediaLifecycle(
		round12AuthContext("mallory"), "m1", model.EditorialMediaLifecycleWithdrawn, nil,
	)
	require.Error(t, err)
}

func TestBuildCMSDraftReviewActiveReviewerIDsExcludeExpired(t *testing.T) {
	resolver, drafts := newDraftReviewCursorResolver(t)
	ctx := round12AuthContext("owner")
	now := time.Now().UTC()
	draft := &models.Draft{
		ID: "active-ids", AuthorID: "owner", ContentType: "Article", Content: "body", ContentFormat: "markdown",
		Status: "draft", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, drafts.CreateDraft(ctx, draft))

	active := &models.DraftReviewGrant{OwnerID: "owner", DraftID: draft.ID, Reviewer: "active-reviewer", GrantedAt: now, ExpiresAt: m2FutureExpiry()}
	past := now.Add(-time.Hour)
	expired := &models.DraftReviewGrant{OwnerID: "owner", DraftID: draft.ID, Reviewer: "expired-reviewer", GrantedAt: now, ExpiresAt: &past}
	drafts.ownedDraftReviews = []*models.DraftReviewGrant{active, expired}
	drafts.sharedDraftReviews = []*models.DraftReviewGrant{active, expired}

	review, err := resolver.buildCMSDraftReview(ctx, draft, nil, nil, false)
	require.NoError(t, err)
	require.Equal(t, []string{"active-reviewer"}, review.ActiveReviewerIds,
		"expired grants must not be classified as active reviewers")
	require.Len(t, review.Grants, 2, "expired grants remain visible on the grant list")
}

func TestUpdateEditorialMediaLifecycleMutationValidatesSupersedingAsset(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	now := time.Now().UTC()
	digest := "sha256:" + strings.Repeat("h", 64)
	internal := func(id, owner string) *models.Media {
		return &models.Media{
			MediaID: id, UserID: owner, ContentType: "image/png", FileSize: 12,
			ContentHash: digest, Status: "ready", Visibility: models.MediaVisibilityInternal,
			S3Bucket: "media-private", S3Key: "alice/" + id + ".png", ModelVersion: 1,
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: owner,
				RecordedAt: now, ContentIntegrity: digest,
			},
		}
	}
	state.seededMedia = map[string]*models.Media{
		"m1": internal("m1", "alice"),
		"m2": internal("m2", "alice"),
		"public": {
			MediaID: "public", UserID: "alice", ContentType: "image/png", FileSize: 12,
			ContentHash: digest, Status: "ready", Visibility: models.MediaVisibilityPublic,
			S3Bucket: "media-private", S3Key: "alice/public.png",
		},
	}

	// A non-internal successor is rejected through the mutation surface.
	_, err := resolver.Mutation().UpdateEditorialMediaLifecycle(
		round12AuthContext("alice"), "m1", model.EditorialMediaLifecycleSuperseded, ptrString("public"),
	)
	require.Error(t, err)

	// A valid internal successor succeeds and is echoed back.
	payload, err := resolver.Mutation().UpdateEditorialMediaLifecycle(
		round12AuthContext("alice"), "m1", model.EditorialMediaLifecycleSuperseded, ptrString("m2"),
	)
	require.NoError(t, err)
	require.NotNil(t, payload)
	require.Equal(t, model.EditorialMediaLifecycleSuperseded, payload.Lifecycle)
	require.NotNil(t, payload.SupersededByMediaID)
	require.Equal(t, "m2", *payload.SupersededByMediaID)
}

func TestUpdateEditorialMediaLifecycleMutationRejectsSuccessorOutsideSuperseded(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	now := time.Now().UTC()
	digest := "sha256:" + strings.Repeat("j", 64)
	internal := func(id string) *models.Media {
		return &models.Media{
			MediaID: id, UserID: "alice", ContentType: "image/png", FileSize: 12,
			ContentHash: digest, Status: "ready", Visibility: models.MediaVisibilityInternal,
			S3Bucket: "media-private", S3Key: "alice/" + id + ".png", ModelVersion: 1,
			Provenance: &models.MediaProvenance{
				Origin: models.EditorialMediaOriginSupplied, ResponsibleActor: "alice",
				RecordedAt: now, ContentIntegrity: digest,
			},
		}
	}
	state.seededMedia = map[string]*models.Media{
		"m1": internal("m1"),
		"m2": internal("m2"),
	}

	for _, tc := range []struct {
		name      string
		lifecycle model.EditorialMediaLifecycle
	}{
		{name: "withdrawn", lifecycle: model.EditorialMediaLifecycleWithdrawn},
		{name: "unavailable", lifecycle: model.EditorialMediaLifecycleUnavailable},
	} {
		t.Run(tc.name+" with a successor is rejected at the mutation surface", func(t *testing.T) {
			_, err := resolver.Mutation().UpdateEditorialMediaLifecycle(
				round12AuthContext("alice"), "m1", tc.lifecycle, ptrString("m2"),
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), "superseded-by media ID requires the superseded lifecycle")
		})
	}

	// The same non-superseded lifecycle without a successor remains writable, so
	// a withdrawn asset can still receive plain metadata updates afterwards.
	payload, err := resolver.Mutation().UpdateEditorialMediaLifecycle(
		round12AuthContext("alice"), "m1", model.EditorialMediaLifecycleWithdrawn, nil,
	)
	require.NoError(t, err)
	require.Equal(t, model.EditorialMediaLifecycleWithdrawn, payload.Lifecycle)
	require.Nil(t, payload.SupersededByMediaID)
}
