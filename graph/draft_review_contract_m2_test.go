package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/services"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

// workingReviewDraftRepository models the full review write semantics over the
// in-memory draft store: grants persist, re-share refreshes, revocation sticks,
// and verdicts are immutable history. The production doubles and the reviewMemRepo
// in pkg/services/cms model the same state.
type workingReviewDraftRepository struct {
	*inmemory.DraftRepository
	mu       sync.Mutex
	grants   map[string]*models.DraftReviewGrant
	verdicts []*models.DraftReviewVerdict
}

func newWorkingReviewDraftRepository() *workingReviewDraftRepository {
	return &workingReviewDraftRepository{
		DraftRepository: inmemory.NewDraftRepository(),
		grants:          map[string]*models.DraftReviewGrant{},
	}
}

func (r *workingReviewDraftRepository) reviewKey(owner, draft, reviewer string) string {
	return owner + "|" + draft + "|" + reviewer
}

func (r *workingReviewDraftRepository) storeGrant(g *models.DraftReviewGrant) error {
	if err := g.UpdateKeys(); err != nil {
		return err
	}
	copy := *g
	r.grants[r.reviewKey(g.OwnerID, g.DraftID, g.Reviewer)] = &copy
	return nil
}

func (r *workingReviewDraftRepository) CreateDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storeGrant(g)
}

func (r *workingReviewDraftRepository) RegrantDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storeGrant(g)
}

func (r *workingReviewDraftRepository) RevokeDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.storeGrant(g)
}

func (r *workingReviewDraftRepository) GetDraftReviewGrant(_ context.Context, owner, draft, reviewer string) (*models.DraftReviewGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.grants[r.reviewKey(owner, draft, reviewer)]
	if !ok {
		return nil, storage.ErrNotFound
	}
	copy := *g
	return &copy, nil
}

func (r *workingReviewDraftRepository) ListActiveDraftReviewGrants(_ context.Context, reviewer string, limit int, cursor string) ([]*models.DraftReviewGrant, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*models.DraftReviewGrant{}
	for _, g := range r.grants {
		if g.Reviewer == reviewer && g.RevokedAt == nil && (cursor == "" || g.GSI2SK < cursor) {
			copy := *g
			out = append(out, &copy)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GSI2SK > out[j].GSI2SK })
	if limit <= 0 {
		limit = 25
	}
	nextCursor := ""
	if len(out) > limit {
		nextCursor = out[limit-1].GSI2SK
		out = out[:limit]
	}
	return out, nextCursor, nil
}

func (r *workingReviewDraftRepository) ListDraftReviewGrants(_ context.Context, owner, draft string) ([]*models.DraftReviewGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*models.DraftReviewGrant{}
	for _, g := range r.grants {
		if g.OwnerID == owner && g.DraftID == draft {
			copy := *g
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (r *workingReviewDraftRepository) ListDraftReviewGrantsByOwner(_ context.Context, owner string) ([]*models.DraftReviewGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*models.DraftReviewGrant{}
	for _, g := range r.grants {
		if g.OwnerID == owner {
			copy := *g
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (r *workingReviewDraftRepository) CreateDraftReviewVerdict(_ context.Context, v *models.DraftReviewVerdict) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := v.UpdateKeys(); err != nil {
		return err
	}
	copy := *v
	r.verdicts = append(r.verdicts, &copy)
	return nil
}

func (r *workingReviewDraftRepository) ListDraftReviewVerdicts(_ context.Context, owner, draft string) ([]*models.DraftReviewVerdict, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []*models.DraftReviewVerdict{}
	for _, v := range r.verdicts {
		if v.OwnerID == owner && v.DraftID == draft {
			copy := *v
			out = append(out, &copy)
		}
	}
	return out, nil
}

// UpdateDraftReviewFields mirrors the production field-scoped writer.
func (r *workingReviewDraftRepository) UpdateDraftReviewFields(_ context.Context, owner string, draft *models.Draft) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, err := r.DraftRepository.GetDraft(context.Background(), owner, draft.ID)
	if err != nil {
		return err
	}
	stored.ReviewedBy = draft.ReviewedBy
	stored.ReviewStatus = draft.ReviewStatus
	stored.EditorNotes = draft.EditorNotes
	return nil
}

// TestDraftReviewScenarioCByteBoundApprovalEndToEnd is the M2 exercisable
// result: approve a revision containing image A, swap to image B, observe the
// prior approval go stale and publish block, re-review and authorize, publish,
// and confirm the durable Lesser URL serves the exact approved B bytes.
func TestDraftReviewScenarioCByteBoundApprovalEndToEnd(t *testing.T) {
	resolver, storage, _, _, state := newRound12GraphResolverWithMocks(t)
	state.persistMedia = true
	drafts := newWorkingReviewDraftRepository()
	wrapped := &cursorResolverStorage{RepositoryStorage: storage, draft: drafts}
	registry, err := services.NewRegistry(
		services.WithStorage(wrapped),
		services.WithPublisher(resolver.Registry.GetPublisher()),
		services.WithLogger(resolver.Registry.GetLogger()),
		services.WithMediaS3Service(round12MediaS3Service{state: state}),
		services.WithConfig(resolver.Registry.GetConfig()),
	)
	require.NoError(t, err)
	// The durable published serving needs a CDN domain; the harness config is
	// built without one, so mint it for this exercise before the media service
	// is lazily initialized by the publish path.
	resolver.Config.CloudFrontDomain = "cdn.example.test"

	resolver.Registry = registry
	resolver.Storage = wrapped
	mut := resolver.Mutation()
	qry := resolver.Query()

	ctx := round12AuthContext("alice")
	reviewerCtx := round12AuthContext("reviewer")

	uploadEditorial := func(filename string, data []byte, description string) *model.UploadMediaPayload {
		t.Helper()
		payload, err := mut.UploadMedia(ctx, model.UploadMediaInput{
			File:        graphql.Upload{File: &round12ReadSeekCloser{Reader: bytes.NewReader(data)}, Filename: filename},
			Description: &description,
			EditorialProvenance: &model.EditorialMediaProvenanceInput{
				Origin: model.EditorialMediaOriginIllustrated,
				Tool:   &description,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, payload)
		require.Empty(t, payload.Media.URL, "internal media must not receive an unsigned CDN URL pre-publish")
		return payload
	}

	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	pngBytesB, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	require.NoError(t, err)
	require.NotEqual(t, pngBytes, pngBytesB, "the two fixtures must be different bytes")

	assetA := uploadEditorial("hero-a.png", pngBytes, "image A")
	assetB := uploadEditorial("hero-b.png", pngBytesB, "image B")
	require.NotEqual(t, assetA.UploadID, assetB.UploadID)

	title := "Byte-bound approval"
	draft, err := mut.CreateDraft(ctx, model.CreateDraftInput{
		ContentType: model.ObjectTypeArticle,
		Title:       &title,
		Content:     "# Launch",
	})
	require.NoError(t, err)

	attach := func(mediaID, caption string) *model.Draft {
		t.Helper()
		updated, err := mut.SetDraftEditorialMedia(ctx, draft.ID, []*model.EditorialMediaUsageInput{{
			MediaID: mediaID,
			Role:    model.EditorialMediaRoleHero,
			Caption: &caption,
			AltText: &caption,
		}})
		require.NoError(t, err)
		require.Len(t, updated.EditorialMedia, 1)
		return updated
	}
	attach(assetA.UploadID, "A caption")

	shared, err := mut.ShareDraftForReview(ctx, draft.ID, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, shared)
	require.NotNil(t, shared.Grant)
	require.NotNil(t, shared.Grant.ExpiresAt, "grants are bounded by an explicit expiry")

	approved, err := mut.SubmitDraftReview(reviewerCtx, draft.ID, model.DraftReviewVerdictApproved, nil)
	require.NoError(t, err)
	require.True(t, approved.PublishEligible, "the A revision is fully approved")

	expectedHashA := "sha256:" + sha256Hex(pngBytes)
	expectedHashB := "sha256:" + sha256Hex(pngBytesB)

	review, err := qry.DraftReview(ctx, draft.ID)
	require.NoError(t, err)
	require.True(t, review.PublishEligible)
	require.Empty(t, review.PublishBlockingReasons)
	require.Len(t, review.EditorialMedia, 1)
	require.Equal(t, model.EditorialMediaStateReady, review.EditorialMedia[0].State)
	require.Equal(t, expectedHashA, *review.EditorialMedia[0].ContentHash)

	// Swap image A for image B.
	attach(assetB.UploadID, "B caption")

	review, err = qry.DraftReview(ctx, draft.ID)
	require.NoError(t, err)
	require.False(t, review.PublishEligible, "the byte swap must stale the prior approval")
	require.Contains(t, review.PublishBlockingReasons, "REVIEW_APPROVAL_REQUIRED")
	require.Equal(t, expectedHashB, *review.EditorialMedia[0].ContentHash, "the review surface reflects the exact B bytes")

	_, err = mut.PublishDraft(ctx, draft.ID)
	require.Error(t, err, "publish must block on the stale approval")
	require.Contains(t, strings.ToLower(err.Error()), "approval")

	// Re-review and authorize the B revision, then publish.
	reapproved, err := mut.SubmitDraftReview(reviewerCtx, draft.ID, model.DraftReviewVerdictApproved, nil)
	require.NoError(t, err)
	require.True(t, reapproved.PublishEligible)

	article, err := mut.PublishDraft(ctx, draft.ID)
	require.NoError(t, err)
	require.NotNil(t, article)
	require.NotNil(t, article.FeaturedImage, "the hero binding must flow into the published featured image")
	require.True(t, strings.HasPrefix(article.FeaturedImage.URL, "https://cdn.example.test/published/"),
		"the published featured image must resolve to the durable Lesser serving, not an expiring presignature")
	require.Contains(t, article.FeaturedImage.URL, assetB.UploadID,
		"the published article must reference the exact approved B asset")
	require.Len(t, state.publishCopies, 1, "exactly one durable copy mint for the bound hero")
	require.True(t, strings.HasPrefix(state.publishCopies[0].destination, "published/media/"),
		"the durable copy must land on the published serving prefix")
	require.True(t, strings.HasSuffix(state.publishCopies[0].destination, assetB.UploadID+".png"),
		"the durable copy must carry the exact approved B bytes")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
