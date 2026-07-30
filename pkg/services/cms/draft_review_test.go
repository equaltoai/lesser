package cms

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type reviewMemRepo struct {
	*memDraftRepo
	grants   map[string]*models.DraftReviewGrant
	verdicts []*models.DraftReviewVerdict
}

func newReviewMemRepo() *reviewMemRepo {
	return &reviewMemRepo{memDraftRepo: newMemDraftRepo(), grants: map[string]*models.DraftReviewGrant{}}
}
func reviewKey(owner, draft, reviewer string) string { return owner + "|" + draft + "|" + reviewer }
func (r *reviewMemRepo) PutDraftReviewGrant(_ context.Context, g *models.DraftReviewGrant) error {
	if err := g.UpdateKeys(); err != nil {
		return err
	}
	copy := *g
	r.grants[reviewKey(g.OwnerID, g.DraftID, g.Reviewer)] = &copy
	return nil
}
func (r *reviewMemRepo) GetDraftReviewGrant(_ context.Context, owner, draft, reviewer string) (*models.DraftReviewGrant, error) {
	g, ok := r.grants[reviewKey(owner, draft, reviewer)]
	if !ok {
		return nil, errReviewNotFound{}
	}
	copy := *g
	return &copy, nil
}
func (r *reviewMemRepo) ListActiveDraftReviewGrants(_ context.Context, reviewer string, _ int) ([]*models.DraftReviewGrant, error) {
	out := []*models.DraftReviewGrant{}
	for _, g := range r.grants {
		if g.Reviewer == reviewer && g.RevokedAt == nil {
			copy := *g
			out = append(out, &copy)
		}
	}
	return out, nil
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

type errReviewNotFound struct{}

func (errReviewNotFound) Error() string { return "not found" }

func newReviewService(t *testing.T) (*DraftService, *reviewMemRepo) {
	t.Helper()
	repo := newReviewMemRepo()
	svc := NewDraftService(repo, nil, "example.test", true, zap.NewNop())
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	draft := &models.Draft{ID: "d1", AuthorID: "owner", ContentType: activitypub.ArticleType, Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
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

func TestDraftReviewRevocationAndRegrantInvalidateApproval(t *testing.T) {
	svc, _ := newReviewService(t)
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "ok")
	require.NoError(t, err)
	approved, err := svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.True(t, approved)
	require.NoError(t, svc.RevokeDraftReview(ctx, "owner", "d1", "principal"))
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.False(t, approved)
	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.False(t, approved, "regrant requires a fresh verdict")
	time.Sleep(time.Millisecond) // grant and fresh verdict must have distinct timestamps.
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "again")
	require.NoError(t, err)
	approved, err = svc.HasActiveApproval(ctx, "owner", "d1")
	require.NoError(t, err)
	require.True(t, approved)
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
