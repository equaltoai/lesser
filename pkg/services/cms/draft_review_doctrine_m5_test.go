package cms

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// The OPERATOR CONTENT DOCTRINE matrix (2026-08-24) pinned as tests on the
// article surface, mirroring the promo surface's M4 tests:
//
//	principal releaser + zero ever-granted reviewers -> allowed (implicit)
//	principal releaser + granted reviewers -> all required
//	non-principal releaser -> principal required, regardless of provenance
//
// plus the requested = required rule: every reviewer who ever recorded a
// verdict must hold a current approving verdict even after their grant is
// revoked or expires — revocation cannot delete a required approval.

// doctrineReviewService builds a review-wired service whose draft is owned by
// owner. generatedBy mirrors newReviewService's seed so doctrine tests can
// toggle draft provenance explicitly.
func doctrineReviewService(t *testing.T, owner, generatedBy string) (*DraftService, *reviewMemRepo) {
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
	draft := &models.Draft{ID: "d1", AuthorID: owner, ContentType: activitypub.ArticleType, Title: "Doctrine", Slug: "doctrine", Content: "draft", ContentFormat: "markdown", GeneratedBy: generatedBy}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	return svc, repo
}

// TestDraftDoctrine_PrincipalReleaserImplicitApproval pins the first matrix
// row: the principal releasing their own draft is the implicit approval — with
// zero ever-granted reviewers the draft releases without any verdict, and this
// holds regardless of draft provenance (including agent-generated content).
func TestDraftDoctrine_PrincipalReleaserImplicitApproval(t *testing.T) {
	for _, tc := range []struct {
		name        string
		generatedBy string
	}{
		{name: "human-written", generatedBy: ""},
		{name: "agent-generated", generatedBy: "agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := doctrineReviewService(t, "principal", tc.generatedBy)
			ctx := context.Background()
			state, err := svc.DraftReviewState(ctx, "principal", "d1", nil)
			require.NoError(t, err)
			require.False(t, state.PrincipalApprovalRequired,
				"the read state renders the principal not required for the principal's own draft")
			require.True(t, state.PublishEligible, "zero ever-granted reviewers; the principal's release is eligible")

			article, err := svc.PublishDraft(ctx, "principal", "d1")
			require.NoError(t, err, "the gate must agree with the read state and publish the principal's own draft")
			require.NotNil(t, article)
		})
	}
}

// TestDraftDoctrine_PrincipalReleaserGrantedReviewersAllRequired pins the
// second matrix row: a granted reviewer without a current approval blocks even
// the principal's own release.
func TestDraftDoctrine_PrincipalReleaserGrantedReviewersAllRequired(t *testing.T) {
	svc, _ := doctrineReviewService(t, "principal", "agent")
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "principal", "d1", "reviewer")
	require.NoError(t, err)

	_, err = svc.PublishDraft(ctx, "principal", "d1")
	require.ErrorIs(t, err, ErrDraftReviewApprovalRequired,
		"a granted reviewer without a current approval blocks even the principal")

	_, err = svc.SubmitDraftReview(ctx, "reviewer", "principal", "d1", DraftReviewApproved, "ready")
	require.NoError(t, err)
	article, err := svc.PublishDraft(ctx, "principal", "d1")
	require.NoError(t, err)
	require.NotNil(t, article)
}

// TestDraftDoctrine_NonPrincipalReleaserPrincipalFloor pins the third matrix
// row: the releasing owner is not the instance principal, so the principal
// floor demands a current principal approval REGARDLESS of draft provenance —
// including a human-written draft with an empty GeneratedBy.
func TestDraftDoctrine_NonPrincipalReleaserPrincipalFloor(t *testing.T) {
	svc, _ := doctrineReviewService(t, "owner", "")
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewApproved, "ready")
	require.NoError(t, err)

	// Reviewer approval alone is not enough: owner is not the instance
	// principal, so the principal floor demands a current principal approval.
	_, err = svc.PublishDraft(ctx, "owner", "d1")
	require.ErrorIs(t, err, ErrDraftReviewPrincipalApprovalRequired,
		"an agent releasing human-written content still requires principal approval")

	state, err := svc.DraftReviewState(ctx, "owner", "d1", nil)
	require.NoError(t, err)
	require.True(t, state.PrincipalApprovalRequired)
	require.False(t, state.PrincipalApproved)
	require.True(t, state.ReviewersApproved, "the reviewer requirement is met; only the floor blocks")

	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)
	article, err := svc.PublishDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	require.NotNil(t, article)
}

// TestDraftDoctrine_RevokedGrantCannotDeleteRequiredApproval pins the
// requested = required rule: revoking a dissenting reviewer does not delete
// their requirement — their ever-recorded verdict still binds until they hold
// a current approving verdict.
func TestDraftDoctrine_RevokedGrantCannotDeleteRequiredApproval(t *testing.T) {
	svc, _ := doctrineReviewService(t, "owner", "agent")
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)
	// The reviewer demands changes; the owner revokes the grant to be rid of
	// them, then the principal approves. The changes-requested verdict binds:
	// revocation cannot delete the required approval.
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewChangesRequested, "fix the copy")
	require.NoError(t, err)
	require.NoError(t, svc.RevokeDraftReview(ctx, "owner", "d1", "reviewer"))
	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)

	_, err = svc.PublishDraft(ctx, "owner", "d1")
	require.ErrorIs(t, err, ErrDraftReviewApprovalRequired,
		"the ever-recorded-verdict reviewer stays required even after revocation")

	// The owner re-grants and the reviewer approves; the draft can publish.
	_, err = svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewApproved, "ready now")
	require.NoError(t, err)
	article, err := svc.PublishDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	require.NotNil(t, article)
}

// TestDraftDoctrine_StalePrincipalVerdictBlocksAgentRelease pins the
// hash-current requirement of the principal floor: a principal approval goes
// stale when the reviewed content changes, and the non-principal release stays
// blocked until the principal re-approves the exact bytes.
func TestDraftDoctrine_StalePrincipalVerdictBlocksAgentRelease(t *testing.T) {
	svc, repo := doctrineReviewService(t, "owner", "agent")
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)

	draft, err := repo.GetDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	draft.Content = "edited after principal approval"
	require.NoError(t, repo.UpdateDraft(ctx, "owner", draft))

	state, err := svc.DraftReviewState(ctx, "owner", "d1", nil)
	require.NoError(t, err)
	require.False(t, state.PrincipalApproved, "a stale principal verdict no longer satisfies the floor")
	require.False(t, state.PublishEligible)

	_, err = svc.PublishDraft(ctx, "owner", "d1")
	require.Error(t, err, "the stale principal approval must block the agent release")

	_, err = svc.SubmitDraftReview(ctx, "principal", "owner", "d1", DraftReviewApproved, "re-approval")
	require.NoError(t, err)
	article, err := svc.PublishDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	require.NotNil(t, article)
}

// TestDraftReviewSubmitBindsToInspectedContentHash pins the submit-time
// expected-hash binding: a submit carrying an inspected hash that no longer
// matches the stored draft is rejected with a conflict signal and records no
// verdict, so a verdict can never bless content the reviewer did not see.
func TestDraftReviewSubmitBindsToInspectedContentHash(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)

	draft, err := repo.GetDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	inspected := DraftReviewContentHash(draft)

	// The reviewer's client inspected the draft (hash H1); the owner then edits.
	draft.Content = "changed after the reviewer read it"
	require.NoError(t, repo.UpdateDraft(ctx, "owner", draft))

	// Submit with the inspected hash -> explicit conflict, no verdict recorded.
	_, err = svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewApproved, "", inspected)
	require.ErrorIs(t, err, ErrDraftReviewReviewContentChanged)
	require.ErrorIs(t, err, ErrDraftReviewConflict, "the mismatch surfaces the additive conflict signal")
	verdicts, err := svc.DraftReviewVerdicts(ctx, "owner", "d1")
	require.NoError(t, err)
	require.Empty(t, verdicts, "no verdict is recorded for unseen content")

	// A submit carrying the current hash records the verdict as before.
	stored, err := repo.GetDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	current := DraftReviewContentHash(stored)
	require.NotEqual(t, inspected, current)
	v, err := svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewApproved, "", current)
	require.NoError(t, err)
	require.Equal(t, current, v.ContentHash, "the recorded verdict binds the reviewed hash")
}

// TestDraftReviewSubmitLegacyNoHashKeepsWorking pins the compatibility path:
// deployed consumers submit without a content hash and the legacy
// no-constraint path keeps recording the verdict bound to the current hash.
func TestDraftReviewSubmitLegacyNoHashKeepsWorking(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	_, err := svc.ShareDraftForReview(ctx, "owner", "d1", "reviewer")
	require.NoError(t, err)

	v, err := svc.SubmitDraftReview(ctx, "reviewer", "owner", "d1", DraftReviewApproved, "legacy submit")
	require.NoError(t, err)
	require.NotEmpty(t, v.ContentHash, "the verdict still binds the current hash")

	draft, err := repo.GetDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	require.Equal(t, DraftReviewContentHash(draft), v.ContentHash)
	require.Equal(t, "reviewer", draft.ReviewedBy)
	require.Equal(t, DraftReviewApproved, draft.ReviewStatus)
}

// TestDraftDoctrine_PrincipalIdentityMixedCaseAgreesAcrossGateAndReadState
// pins the sameAccount choke point on the article surface: a mixed-case
// principal string agrees across the read state, the gate, and the
// principal-owner self-grant, mirroring the promo surface.
func TestDraftDoctrine_PrincipalIdentityMixedCaseAgreesAcrossGateAndReadState(t *testing.T) {
	repo := newReviewMemRepo()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: newMemArticleService(),
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.NewNop(),
	}
	// The stored primary-admin username keeps its original casing.
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "PriNcIpAl", nil })
	draft := &models.Draft{ID: "d1", AuthorID: "principal", ContentType: activitypub.ArticleType, Title: "Case", Slug: "case", Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	ctx := context.Background()

	state, err := svc.DraftReviewState(ctx, "principal", "d1", nil)
	require.NoError(t, err)
	require.False(t, state.PrincipalApprovalRequired,
		"the read state renders the principal not required despite the casing divergence")
	require.True(t, state.PublishEligible)

	article, err := svc.PublishDraft(ctx, "principal", "d1")
	require.NoError(t, err, "the gate must agree with the read state despite the casing divergence")
	require.NotNil(t, article)

	// Self-grant: the principal may share a draft with themselves (owner ==
	// reviewer); the mixed-case identity must not refuse it.
	selfDraft := &models.Draft{ID: "d2", AuthorID: "principal", ContentType: activitypub.ArticleType, Title: "Case2", Slug: "case2", Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
	require.NoError(t, svc.CreateDraft(ctx, selfDraft))
	_, err = svc.ShareDraftForReview(ctx, "principal", "d2", "principal")
	require.NoError(t, err, "the principal may self-grant despite the casing divergence")
	_, err = svc.SubmitDraftReview(ctx, "principal", "principal", "d2", DraftReviewApproved, "self-approval")
	require.NoError(t, err, "the principal may self-review despite the casing divergence")
}

// TestDraftDoctrine_PrincipalFloorResolvesMixedCaseExternalApprover pins the
// F1 floor-resolution lane on the article surface. The principal floor's
// state.active[principal] / state.latest[principal] access is byte-keyed
// against the reviewer strings recorded at share/submit time, while
// principal-hood is case-insensitive (sameAccount). The principal string is
// canonicalized to lowercase at the instancePrincipal choke point, so a
// mixed-case principal config still resolves a real approval recorded under
// the canonical lowercase share text — a non-principal release proceeds
// instead of being blocked despite the recorded principal approval.
func TestDraftDoctrine_PrincipalFloorResolvesMixedCaseExternalApprover(t *testing.T) {
	repo := newReviewMemRepo()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: newMemArticleService(),
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.NewNop(),
	}
	// The stored primary-admin username keeps its original casing.
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "PriNcIpAl", nil })
	draft := &models.Draft{ID: "d1", AuthorID: "alice", ContentType: activitypub.ArticleType, Title: "Floor", Slug: "floor", Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	ctx := context.Background()

	// The non-principal owner shares to the principal under the canonical
	// lowercase text and the principal approves.
	_, err := svc.ShareDraftForReview(ctx, "alice", "d1", "principal")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "principal", "alice", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)

	state, err := svc.DraftReviewState(ctx, "alice", "d1", nil)
	require.NoError(t, err)
	require.True(t, state.PrincipalApproved,
		"the floor must see the principal approval recorded under the canonical lowercase share text")
	require.True(t, state.PublishEligible)

	article, err := svc.PublishDraft(ctx, "alice", "d1")
	require.NoError(t, err, "a non-principal release proceeds once the floor resolves the recorded principal approval")
	require.NotNil(t, article)
}

// TestDraftDoctrine_PrincipalCaseGrantWithoutResolutionFailsClosed pins the
// F3 denial direction on the article surface: a grant recorded under a
// principal-case text that the canonical floor cannot resolve (the share text
// carries the configured casing rather than the canonical lowercase form) must
// fail closed with the principal-required denial — never a silent pass.
func TestDraftDoctrine_PrincipalCaseGrantWithoutResolutionFailsClosed(t *testing.T) {
	repo := newReviewMemRepo()
	svc := &DraftService{
		draftRepo:      repo,
		articleService: newMemArticleService(),
		domain:         "example.test",
		scheduling:     true,
		logger:         zap.NewNop(),
	}
	// The stored primary-admin username keeps its original casing.
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "PriNcIpAl", nil })
	draft := &models.Draft{ID: "d1", AuthorID: "alice", ContentType: activitypub.ArticleType, Title: "Floor", Slug: "floor", Content: "draft", ContentFormat: "markdown", GeneratedBy: "agent"}
	require.NoError(t, svc.CreateDraft(context.Background(), draft))
	ctx := context.Background()

	// The owner shares to the principal under the configured casing; the
	// recorded grant/verdict text never equals the canonical form, so the
	// byte-keyed floor cannot resolve an approval.
	_, err := svc.ShareDraftForReview(ctx, "alice", "d1", "PriNcIpAl")
	require.NoError(t, err)
	_, err = svc.SubmitDraftReview(ctx, "PriNcIpAl", "alice", "d1", DraftReviewApproved, "operator approval")
	require.NoError(t, err)

	state, err := svc.DraftReviewState(ctx, "alice", "d1", nil)
	require.NoError(t, err)
	require.False(t, state.PrincipalApproved, "the byte-keyed floor cannot resolve the recorded principal-case grant")
	require.True(t, state.ReviewersApproved, "the reviewer requirement is met; only the floor blocks")
	require.False(t, state.PublishEligible)
	require.Contains(t, state.BlockingReasons, "PRINCIPAL_APPROVAL_REQUIRED")

	_, err = svc.PublishDraft(ctx, "alice", "d1")
	require.ErrorIs(t, err, ErrDraftReviewPrincipalApprovalRequired,
		"the unresolvable principal-case grant denies with the principal-required error, never a silent pass")
}
