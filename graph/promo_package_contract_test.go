package graph

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
	testinginmemory "github.com/equaltoai/lesser/pkg/testing/inmemory"
	"github.com/stretchr/testify/require"
)

func graphPromoDigest(label string) string {
	return "sha256:" + strings.Repeat(label, 64/len(label)+1)[:64]
}

// recordingPromoGraphCreator records the exact release command so contract
// tests can assert the outbound post carries the reviewed content, the exact
// PUBLISHED asset set, and the AI-authorship disclosure.
type recordingPromoGraphCreator struct {
	commands []*notes.CreateNoteCommand
	refSets  [][]notes.PromoPublishedMediaRef
}

func (c *recordingPromoGraphCreator) CreatePromoNote(_ context.Context, cmd *notes.CreateNoteCommand, refs []notes.PromoPublishedMediaRef) (*notes.NoteResult, error) {
	c.commands = append(c.commands, cmd)
	c.refSets = append(c.refSets, refs)
	now := time.Now().UTC()
	statusID := fmt.Sprintf("status-%d", len(c.commands))
	status := &models.Status{
		StatusID:       statusID,
		AuthorID:       cmd.AuthorID,
		AuthorUsername: cmd.AuthorID,
		Content:        cmd.Content,
		Visibility:     cmd.Visibility,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "https://localhost/users/" + cmd.AuthorID + "/statuses/" + statusID,
				Type: activitypub.NoteType,
			},
			Content:          cmd.Content,
			AttributedTo:     "https://localhost/users/" + cmd.AuthorID,
			Visibility:       cmd.Visibility,
			AgentAttribution: cmd.AgentAttribution,
		},
		PublishedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
		ModifiedAt:  now,
	}
	return &notes.NoteResult{Note: status}, nil
}

// newPromoGraphHarness wires the round12 resolver with a promo package
// repository, the instance principal provider, a recording status creator, and
// a published article to promote.
func newPromoGraphHarness(t *testing.T) (*Resolver, *round12GraphStorage, *round12PermissiveQueryState, *recordingPromoGraphCreator) {
	t.Helper()
	resolver, storage, _, _, state := newRound12GraphResolverWithMocks(t)
	state.seededMedia = make(map[string]*models.Media)
	svc := resolver.Registry.Drafts()
	require.NotNil(t, svc, "the round12 harness must wire the draft service")
	svc.SetPromoPackageRepository(testinginmemory.NewPromoPackageRepository())
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "principal", nil })
	creator := &recordingPromoGraphCreator{}
	svc.SetPromoStatusCreator(creator)

	now := time.Now().UTC()
	article := &models.Article{
		Object: models.Object{
			ID:        "https://localhost/articles/hello",
			Type:      activitypub.ArticleType,
			Name:      "Hello",
			Published: now,
		},
		Slug:      "hello",
		UpdatedAt: now,
	}
	require.NoError(t, article.UpdateKeys())
	require.NoError(t, storage.Article().CreateArticle(context.Background(), article))
	return resolver, storage, state, creator
}

func graphPublishedPromoMedia(id, owner, digest string, origin models.EditorialMediaOrigin) *models.Media {
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
		Provenance:     &models.MediaProvenance{Origin: origin, Tool: "image tool", ContentIntegrity: digest, ResponsibleActor: owner, RecordedAt: now},
		EditorialState: models.EditorialLifecycleAvailable,
		PublishedS3Key: "published/" + id + ".png",
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

// Scenario E at the GraphQL contract: compose -> internal review (hash-bound
// verdict) -> public release BLOCKED before authorization -> explicit
// instance-principal authorization (AI-origin asset) -> release with the exact
// approved assets attached and AI-authorship disclosure intact.
func TestPromoPackageScenarioE_BlockedReleaseThenAuthorizedRelease(t *testing.T) {
	resolver, _, state, creator := newPromoGraphHarness(t)
	mut := resolver.Mutation()
	alice := round12AuthContext("alice")
	reviewer := round12AuthContext("reviewer")
	principal := round12AuthContext("principal")

	digest := graphPromoDigest("aa")
	state.seededMedia["m1"] = graphPublishedPromoMedia("m1", "alice", digest, models.EditorialMediaOriginAIGenerated)

	// Compose the promo package referencing the published article.
	pkg, err := mut.ComposePromoPackage(alice, model.ComposePromoPackageInput{
		ArticleID:     "https://localhost/articles/hello",
		PostText:      "Read our launch article",
		Visibility:    model.PromoPackageVisibilityPublic,
		AssetMediaIds: []string{"m1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, pkg.ID)
	require.Equal(t, "https://localhost/articles/hello", pkg.ArticleID)
	require.Equal(t, model.PromoPackageVisibilityPublic, pkg.Visibility)
	require.Equal(t, model.PromoPackageStatusDraft, pkg.Status)
	require.Len(t, pkg.Assets, 1)
	require.Equal(t, model.PromoPackageAssetStatePublished, pkg.Assets[0].State)
	require.Equal(t, digest, *pkg.Assets[0].ContentHash, "the digest is bound at compose time")
	require.NotNil(t, pkg.Review)
	require.False(t, pkg.Review.ReleaseEligible, "unreviewed package cannot release")

	// Internal review: the reviewer approves the exact reviewed content.
	review, err := mut.SharePromoPackageForReview(alice, pkg.ID, "reviewer")
	require.NoError(t, err)
	require.False(t, review.ReviewersApproved)
	review, err = mut.SubmitPromoPackageReview(reviewer, pkg.ID, model.PromoPackageReviewVerdictApproved, nil)
	require.NoError(t, err)
	require.True(t, review.ReviewersApproved)
	require.True(t, review.PrincipalApprovalRequired, "the AI-origin asset requires the instance principal")
	require.False(t, review.ReleaseEligible)

	// Public release BLOCKED before authorization: no status is created.
	_, err = mut.ReleasePromoPackage(alice, pkg.ID)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "principal")
	require.Len(t, creator.commands, 0, "no outbound post before authorization")

	// Explicit authorization: the instance principal reviews and approves the
	// same package content.
	review, err = mut.SharePromoPackageForReview(alice, pkg.ID, "principal")
	require.NoError(t, err)
	review, err = mut.SubmitPromoPackageReview(principal, pkg.ID, model.PromoPackageReviewVerdictApproved, nil)
	require.NoError(t, err)
	require.True(t, review.ReleaseEligible, "after principal authorization the package may release")

	// Release: the post is created with the exact approved assets and the
	// disclosure intact.
	released, err := mut.ReleasePromoPackage(alice, pkg.ID)
	require.NoError(t, err)
	require.NotEmpty(t, released.StatusID)
	require.NotEmpty(t, released.URL)
	require.Equal(t, model.PromoPackageStatusReleased, released.Package.Status)
	require.NotNil(t, released.Package.ReleasedStatusID)
	require.Equal(t, released.StatusID, *released.Package.ReleasedStatusID)

	require.Len(t, creator.commands, 1)
	cmd := creator.commands[0]
	require.Equal(t, "alice", cmd.AuthorID)
	require.Equal(t, "Read our launch article", cmd.Content, "the exact reviewed post text is what releases")
	require.Equal(t, "public", cmd.Visibility)
	require.Len(t, creator.refSets[0], 1)
	require.Equal(t, notes.PromoPublishedMediaRef{MediaID: "m1", ContentHash: digest}, creator.refSets[0][0])
	require.NotNil(t, cmd.AgentAttribution, "AI-origin assets disclose on the outbound post")
	require.Equal(t, "manual", cmd.AgentAttribution.TriggerType)
	require.Equal(t, "https://localhost/users/principal", cmd.AgentAttribution.ApprovedBy)

	// Re-release is refused; no second post is created.
	_, err = mut.ReleasePromoPackage(alice, pkg.ID)
	require.Error(t, err)
	require.Len(t, creator.commands, 1)
}

// TestPromoPackageContract_StaleOnChangeReBlocksAndUnpublishedAssetsRejected
// covers the two load-bearing guards at the contract surface: content changes
// after approval re-block release, and non-PUBLISHED assets are structurally
// rejected at compose.
func TestPromoPackageContract_StaleOnChangeReBlocksAndUnpublishedAssetsRejected(t *testing.T) {
	resolver, _, state, creator := newPromoGraphHarness(t)
	mut := resolver.Mutation()
	alice := round12AuthContext("alice")
	reviewer := round12AuthContext("reviewer")

	digest := graphPromoDigest("bb")
	state.seededMedia["m1"] = graphPublishedPromoMedia("m1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := mut.ComposePromoPackage(alice, model.ComposePromoPackageInput{
		ArticleID:     "https://localhost/articles/hello",
		PostText:      "Read our launch article",
		Visibility:    model.PromoPackageVisibilityPublic,
		AssetMediaIds: []string{"m1"},
	})
	require.NoError(t, err)
	_, err = mut.SharePromoPackageForReview(alice, pkg.ID, "reviewer")
	require.NoError(t, err)
	_, err = mut.SubmitPromoPackageReview(reviewer, pkg.ID, model.PromoPackageReviewVerdictApproved, nil)
	require.NoError(t, err)

	// Any content edit after approval re-hashes and stales the verdict.
	edited, err := mut.ComposePromoPackage(alice, model.ComposePromoPackageInput{
		PackageID:     &pkg.ID,
		ArticleID:     "https://localhost/articles/hello",
		PostText:      "Read our launch article now",
		Visibility:    model.PromoPackageVisibilityPublic,
		AssetMediaIds: []string{"m1"},
	})
	require.NoError(t, err)
	require.NotEqual(t, pkg.ContentHash, edited.ContentHash, "content change re-hashes the package")

	_, err = mut.ReleasePromoPackage(alice, pkg.ID)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "approval")
	require.Len(t, creator.commands, 0, "a stale approval never releases")

	// Non-PUBLISHED assets are structurally rejected at compose: an internal
	// asset that never crossed the M2 publish transition cannot bind.
	unpublished := graphPublishedPromoMedia("m2", "alice", digest, models.EditorialMediaOriginSupplied)
	unpublished.PublishedURL = ""
	unpublished.PublishedS3Key = ""
	unpublished.PublishedAt = nil
	state.seededMedia["m2"] = unpublished
	_, err = mut.ComposePromoPackage(alice, model.ComposePromoPackageInput{
		ArticleID:     "https://localhost/articles/hello",
		PostText:      "cannot attach unpublished bytes",
		Visibility:    model.PromoPackageVisibilityPublic,
		AssetMediaIds: []string{"m2"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PUBLISHED")

	// Private/direct visibility is structurally rejected at compose.
	_, err = mut.ComposePromoPackage(alice, model.ComposePromoPackageInput{
		ArticleID:     "https://localhost/articles/hello",
		PostText:      "private promo",
		Visibility:    model.PromoPackageVisibilityUnlisted, // valid enum; service normalizes
		AssetMediaIds: []string{"m1"},
	})
	require.NoError(t, err)
}

// TestPromoPackageContract_ActorIsolationAndReviewerQueue proves pre-release
// packages are never world-readable and the reviewer queue is grant-scoped.
func TestPromoPackageContract_ActorIsolationAndReviewerQueue(t *testing.T) {
	resolver, _, state, _ := newPromoGraphHarness(t)
	mut := resolver.Mutation()
	qry := resolver.Query()
	alice := round12AuthContext("alice")
	reviewer := round12AuthContext("reviewer")
	mallory := round12AuthContext("mallory")

	digest := graphPromoDigest("cc")
	state.seededMedia["m1"] = graphPublishedPromoMedia("m1", "alice", digest, models.EditorialMediaOriginSupplied)

	pkg, err := mut.ComposePromoPackage(alice, model.ComposePromoPackageInput{
		ArticleID:     "https://localhost/articles/hello",
		PostText:      "promote",
		Visibility:    model.PromoPackageVisibilityPublic,
		AssetMediaIds: []string{"m1"},
	})
	require.NoError(t, err)
	_, err = mut.SharePromoPackageForReview(alice, pkg.ID, "reviewer")
	require.NoError(t, err)

	// Owner and active reviewer resolve the package; an unrelated caller does not.
	got, err := qry.PromoPackage(alice, pkg.ID)
	require.NoError(t, err)
	require.Equal(t, pkg.ID, got.ID)
	got, err = qry.PromoPackage(reviewer, pkg.ID)
	require.NoError(t, err)
	require.Equal(t, pkg.ID, got.ID)
	_, err = qry.PromoPackage(mallory, pkg.ID)
	require.Error(t, err)

	// The reviewer queue surfaces the shared package with its review state.
	queue, err := qry.SharedPromoPackageReviews(reviewer, nil, nil)
	require.NoError(t, err)
	require.Len(t, queue.Edges, 1)
	require.Equal(t, pkg.ID, queue.Edges[0].Node.PackageID)
	require.False(t, queue.Edges[0].Node.ReviewersApproved)

	// Owner list surfaces the package.
	list, err := qry.PromoPackages(alice, nil, nil)
	require.NoError(t, err)
	require.Len(t, list.Edges, 1)
	require.Equal(t, model.PromoPackageStatusDraft, list.Edges[0].Node.Status)

	// Revoking the grant removes the reviewer's queue entry and their access.
	ok, err := mut.RevokePromoPackageReview(alice, pkg.ID, "reviewer")
	require.NoError(t, err)
	require.True(t, ok)
	queue, err = qry.SharedPromoPackageReviews(reviewer, nil, nil)
	require.NoError(t, err)
	require.Empty(t, queue.Edges)
	_, err = qry.PromoPackage(reviewer, pkg.ID)
	require.Error(t, err, "a revoked grant stops authorizing reads")
}
