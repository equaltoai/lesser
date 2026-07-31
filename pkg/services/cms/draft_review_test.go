package cms

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type reviewMemRepo struct {
	*memDraftRepo
	grants                  map[string]*models.DraftReviewGrant
	verdicts                []*models.DraftReviewVerdict
	includeRevokedQueueRows bool
	createGrantCalls        int
	regrantGrantCalls       int
	revokeGrantCalls        int
	getGrantErr             error
	callLog                 []string
}

func newReviewMemRepo() *reviewMemRepo {
	return &reviewMemRepo{memDraftRepo: newMemDraftRepo(), grants: map[string]*models.DraftReviewGrant{}}
}
func reviewKey(owner, draft, reviewer string) string { return owner + "|" + draft + "|" + reviewer }
func (r *reviewMemRepo) storeGrant(g *models.DraftReviewGrant) error {
	if err := g.UpdateKeys(); err != nil {
		return err
	}
	// This logical service double replaces the full row and does not exercise
	// TableTheory's create/update conditions. Repository tests use the real
	// TableTheory builders to cover those production clauses.
	copy := *g
	r.grants[reviewKey(g.OwnerID, g.DraftID, g.Reviewer)] = &copy
	return nil
}
func (r *reviewMemRepo) CreateDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	r.callLog = append(r.callLog, "create")
	r.createGrantCalls++
	return r.storeGrant(g)
}
func (r *reviewMemRepo) RegrantDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	r.callLog = append(r.callLog, "regrant")
	r.regrantGrantCalls++
	return r.storeGrant(g)
}
func (r *reviewMemRepo) RevokeDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	r.callLog = append(r.callLog, "revoke")
	r.revokeGrantCalls++
	return r.storeGrant(g)
}
func (r *reviewMemRepo) GetDraftReviewGrant(_ context.Context, owner, draft, reviewer string) (*models.DraftReviewGrant, error) {
	r.callLog = append(r.callLog, "get")
	if r.getGrantErr != nil {
		return nil, r.getGrantErr
	}
	g, ok := r.grants[reviewKey(owner, draft, reviewer)]
	if !ok {
		return nil, storage.ErrNotFound
	}
	copy := *g
	return &copy, nil
}
func (r *reviewMemRepo) ListActiveDraftReviewGrants(_ context.Context, reviewer string, limit int, cursor string) ([]*models.DraftReviewGrant, string, error) {
	// Production filters RevokedAt at the repository and service boundaries.
	// includeRevokedQueueRows deliberately simulates a stale sparse-index row so
	// service tests prove that index membership alone cannot restore access.
	out := []*models.DraftReviewGrant{}
	for _, g := range r.grants {
		if g.Reviewer == reviewer && (g.RevokedAt == nil || r.includeRevokedQueueRows) && (cursor == "" || g.GSI2SK < cursor) {
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
func (r *reviewMemRepo) CountActiveDraftReviewGrants(_ context.Context, reviewer string) (int, error) {
	count := 0
	for _, g := range r.grants {
		if g.Reviewer == reviewer && g.RevokedAt == nil {
			count++
		}
	}
	return count, nil
}
func (r *reviewMemRepo) ListDraftReviewGrants(_ context.Context, owner, draft string) ([]*models.DraftReviewGrant, error) {
	out := []*models.DraftReviewGrant{}
	for _, g := range r.grants {
		if g.OwnerID == owner && g.DraftID == draft {
			copy := *g
			out = append(out, &copy)
		}
	}
	return out, nil
}
func (r *reviewMemRepo) CreateDraftReviewVerdict(_ context.Context, v *models.DraftReviewVerdict) error {
	if err := v.UpdateKeys(); err != nil {
		return err
	}
	copy := *v
	r.verdicts = append(r.verdicts, &copy)
	return nil
}
func (r *reviewMemRepo) ListDraftReviewVerdicts(_ context.Context, owner, draft string) ([]*models.DraftReviewVerdict, error) {
	out := []*models.DraftReviewVerdict{}
	for _, v := range r.verdicts {
		if v.OwnerID == owner && v.DraftID == draft {
			copy := *v
			out = append(out, &copy)
		}
	}
	return out, nil
}

func newReviewService(t *testing.T) (*DraftService, *reviewMemRepo) {
	t.Helper()
	repo := newReviewMemRepo()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: newMemArticleService(),
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.NewNop(),
	}
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	draft := &models.Draft{ID: "d1", AuthorID: "owner", ContentType: activitypub.ArticleType, Title: "Review draft", Slug: "review-draft", Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	return svc, repo
}

func TestDraftReviewGateRequiresAllActiveReviewersAndPrincipal(t *testing.T) {
	svc, _ := newReviewService(t)
	ctx := context.Background()
	require.NoError(t, func() error { _, err := svc.ShareDraftForReview(ctx, "owner", "d1", "principal"); return err }())
	require.NoError(t, func() error { _, err := svc.ShareDraftForReview(ctx, "owner", "d1", "other"); return err }())
	_, err := svc.PublishDraft(ctx, "owner", "d1")
	require.Error(t, err, "all active reviewers require a verdict")
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "ok")
	require.NoError(t, err)
	approved, err := svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.False(t, approved, "other active reviewer is still required")
	_, err = svc.SubmitDraftReview(ctx, "other", "owner", "d1", DraftReviewApproved, "ok")
	require.NoError(t, err)
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.True(t, approved)
	_, err = svc.SubmitDraftReview(ctx, "other", "owner", "d1", DraftReviewChangesRequested, "revise")
	require.NoError(t, err)
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.False(t, approved, "latest verdict supersedes per reviewer")
}

func TestHumanDraftWithActiveReviewsRequiresUnanimousApproval(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	draft, err := repo.GetDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	draft.GeneratedBy = ""
	require.NoError(t, repo.UpdateDraft(ctx, "owner", draft))

	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer-a")
	require.NoError(t, err)
	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer-b")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "reviewer-a", "owner", "d1", DraftReviewApproved, "ready")
	require.NoError(t, err)

	err = svc.ScheduleDraft(ctx, "owner", "d1", time.Now().Add(time.Hour))
	require.ErrorContains(t, err, "every active reviewer", "a missing current verdict must block")

	_, err = svc.SubmitDraftReview(ctx, "reviewer-b", "owner", "d1", DraftReviewChangesRequested, "revise")
	require.NoError(t, err)
	_, err = svc.PublishDraft(ctx, "owner", "d1")
	require.ErrorContains(t, err, "every active reviewer", "changes requested must block")

	_, err = svc.SubmitDraftReview(ctx, "reviewer-b", "owner", "d1", DraftReviewApproved, "ready")
	require.NoError(t, err)
	require.NoError(t, svc.ScheduleDraft(ctx, "owner", "d1", time.Now().Add(time.Hour)))
	article, err := svc.PublishDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	require.NotNil(t, article)
}

func TestAgentDraftRequiresPrincipalApprovalAfterReviewerConsensus(t *testing.T) {
	svc, _ := newReviewService(t)
	ctx := context.Background()

	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewApproved, "ready")
	require.NoError(t, err)

	_, err = svc.PublishDraft(ctx, "owner", "d1")
	require.ErrorContains(t, err, "instance principal")

	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)
	article, err := svc.PublishDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	require.NotNil(t, article)
}

func TestDraftReviewRevocationAndRegrantInvalidateApproval(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	require.Equal(t, 1, repo.createGrantCalls)
	require.Zero(t, repo.regrantGrantCalls)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "ok")
	require.NoError(t, err)
	approved, err := svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.True(t, approved)
	require.NoError(t, svc.RevokeDraftReview(ctx, "owner", "d1", "principal"))
	require.Equal(t, 1, repo.revokeGrantCalls)
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.False(t, approved)
	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	require.Equal(t, 1, repo.createGrantCalls, "re-share after revoke must not use the first-write path")
	require.Equal(t, 1, repo.regrantGrantCalls, "re-share after revoke must use the explicit re-grant path")
	draft, grant, err := svc.DraftReviewForCaller(ctx, "principal", "d1")
	require.NoError(t, err, "re-invited reviewer can read the draft")
	require.Equal(t, "d1", draft.ID)
	require.Nil(t, grant.RevokedAt)
	approved, err = svc.HasUnanimousActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.False(t, approved, "re-invited reviewer is counted and requires a fresh verdict")
	time.Sleep(time.Millisecond) // grant and fresh verdict must have distinct timestamps.
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "again")
	require.NoError(t, err)
	approved, err = svc.HasUnanimousActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.True(t, approved, "fresh verdict restores reviewer unanimity")
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.True(t, approved)
}

func TestShareDraftForReviewFailsClosedOnGrantLookupError(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	revokedAt := time.Now().UTC().Add(-time.Hour)
	key := reviewKey("owner", "d1", "principal")
	repo.grants[key] = &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "d1",
		Reviewer:  "principal",
		RevokedAt: &revokedAt,
		Version:   7,
	}
	transientErr := errors.New("transient grant lookup failure")
	repo.getGrantErr = transientErr
	repo.callLog = nil

	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.ErrorIs(t, err, transientErr)
	require.Equal(t, []string{"get"}, repo.callLog, "a failed read must not be followed by any grant write")
	require.Zero(t, repo.createGrantCalls)
	require.Zero(t, repo.regrantGrantCalls)
	require.Equal(t, 7, repo.grants[key].Version)
	require.Equal(t, revokedAt, *repo.grants[key].RevokedAt)
	require.Empty(t, repo.grants[key].GSI2PK)
	require.Empty(t, repo.grants[key].GSI2SK)
}

func TestDraftReviewQueueRejectsStaleRevokedIndexRow(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	grant, err := svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)

	revokedAt := time.Now().UTC()
	stored := repo.grants[reviewKey("owner", "d1", "principal")]
	stored.RevokedAt = &revokedAt
	stored.GSI2PK = grant.GSI2PK
	stored.GSI2SK = grant.GSI2SK
	repo.includeRevokedQueueRows = true

	queue, _, err := svc.SharedDraftReviews(ctx, "principal", 25, "")
	require.NoError(t, err)
	require.Empty(t, queue, "a stale sparse-index row must not restore queue access")

	_, _, err = svc.DraftReviewForCaller(ctx, "principal", "d1")
	require.Error(t, err, "a stale sparse-index row must not restore draft access")
}

func TestSharedDraftReviewsPaginationRoundTrip(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	for i, draftID := range []string{"older", "middle", "newer"} {
		draft := &models.Draft{ID: draftID, AuthorID: "owner", ContentType: activitypub.ArticleType, Title: draftID, Slug: draftID, Content: "draft", ContentFormat: "markdown"}
		require.NoError(t, svc.CreateDraft(ctx, draft))
		grant := &models.DraftReviewGrant{OwnerID: "owner", DraftID: draftID, Reviewer: "reviewer", GrantedAt: base.Add(time.Duration(i) * time.Minute)}
		require.NoError(t, repo.CreateDraftReviewGrant(ctx, grant))
	}

	first, next, err := svc.SharedDraftReviews(ctx, "reviewer", 2, "")
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.Equal(t, []string{"newer", "middle"}, []string{first[0].DraftID, first[1].DraftID})
	require.Equal(t, first[1].GSI2SK, next)

	second, finalCursor, err := svc.SharedDraftReviews(ctx, "reviewer", 2, next)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, "older", second[0].DraftID)
	require.Empty(t, finalCursor)

	total, err := svc.CountSharedDraftReviews(ctx, "reviewer")
	require.NoError(t, err)
	require.Equal(t, 3, total)
}

func TestDraftReviewForCallerPagesPastFormerTwoHundredGrantCap(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 201; i++ {
		draftID := fmt.Sprintf("paged-%03d", i)
		draft := &models.Draft{ID: draftID, AuthorID: "owner", ContentType: activitypub.ArticleType, Title: draftID, Slug: draftID, Content: "draft", ContentFormat: "markdown"}
		require.NoError(t, svc.CreateDraft(ctx, draft))
		grant := &models.DraftReviewGrant{OwnerID: "owner", DraftID: draftID, Reviewer: "reviewer", GrantedAt: base.Add(time.Duration(i) * time.Second)}
		require.NoError(t, repo.CreateDraftReviewGrant(ctx, grant))
	}

	draft, grant, err := svc.DraftReviewForCaller(ctx, "reviewer", "paged-000")
	require.NoError(t, err)
	require.Equal(t, "paged-000", draft.ID)
	require.Equal(t, "paged-000", grant.DraftID)
}

func TestDraftReviewRejectsUngrantAndOwner(t *testing.T) {
	svc, _ := newReviewService(t)
	ctx := context.Background()
	_, err := svc.SubmitDraftReview(ctx, "stranger", "owner", "d1", DraftReviewApproved, "")
	require.Error(t, err)
	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "owner")
	require.Error(t, err)
}

func TestDraftReviewPrincipalOwnerUsesExplicitSelfGrant(t *testing.T) {
	repo := newReviewMemRepo()
	svc := NewDraftService(repo, nil, "example.test", true, zap.NewNop())
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	draft := &models.Draft{ID: "d1", AuthorID: "principal", ContentType: activitypub.ArticleType, Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	_, err := svc.ShareDraftForReview(context.Background(), "principal", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(context.Background(), "principal", "principal", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)
	approved, err := svc.HasActiveApproval(context.Background(), "principal", "d1")
	require.NoError(t, err)
	require.True(t, approved)
}
