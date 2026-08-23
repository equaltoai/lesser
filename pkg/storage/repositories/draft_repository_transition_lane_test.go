package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// F1 remediation repository proving suite: TransitionDraftToPublishing is the
// only field-scoped lane that writes PublishAttemptedAt (and it also moves the
// GSI4 status index), while the content lane (UpdateDraft) never selects the
// attribute, so a stored publish-attempt stamp survives ordinary content
// writes and the stale-publishing sweep horizon cannot be re-armed by editing.

func newTransitionLaneRepo(t *testing.T) *DraftRepository {
	t.Helper()
	client := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	// The fakedb Create does not auto-run model key hooks; seed rows must call
	// UpdateKeys explicitly.
	return repo
}

func TestTransitionDraftToPublishingFieldScopedLane(t *testing.T) {
	ctx := context.Background()
	repo := newTransitionLaneRepo(t)
	base := time.Now().UTC().Add(-2 * time.Hour)

	scheduled := &models.Draft{
		AuthorID: "owner", ID: "draft-1", Content: "body", ContentFormat: "markdown",
		Status: "scheduled", ScheduledAt: &base, CreatedAt: base.Add(-time.Hour), UpdatedAt: base,
	}
	require.NoError(t, scheduled.UpdateKeys())
	require.NoError(t, repo.CreateDraft(ctx, scheduled))

	loaded, err := repo.GetDraft(ctx, "owner", "draft-1")
	require.NoError(t, err)

	attempt := time.Now().UTC()
	loaded.Status = "publishing"
	loaded.ScheduledAt = nil
	loaded.UpdatedAt = attempt
	loaded.PublishAttemptedAt = &attempt
	require.NoError(t, repo.TransitionDraftToPublishing(ctx, "owner", loaded))

	stored, err := repo.GetDraft(ctx, "owner", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "publishing", stored.Status)
	require.Equal(t, attempt, *stored.PublishAttemptedAt,
		"the transition lane is the only writer of the publish-attempt stamp")
	require.Equal(t, attempt, stored.UpdatedAt)
	require.Nil(t, stored.ScheduledAt, "the transition clears the schedule")
	require.Equal(t, "DRAFT#STATUS#publishing", stored.GSI4PK,
		"the status change must move the row into the publishing status partition")
	require.True(t, strings.HasPrefix(stored.GSI4SK, "TIME#"+attempt.UTC().Format(time.RFC3339Nano)),
		"GSI4SK must reflect the transition timestamp")

	publishing, _, err := repo.ListDraftsByStatusPaginated(ctx, "publishing", 10, "")
	require.NoError(t, err)
	require.Len(t, publishing, 1, "the transitioned draft must be enumerable as publishing")
	scheduledNow, _, err := repo.ListDraftsByStatusPaginated(ctx, "scheduled", 10, "")
	require.NoError(t, err)
	require.Empty(t, scheduledNow, "the transitioned draft must leave the scheduled status partition")
}

func TestContentUpdateDraftDoesNotAlterStoredPublishAttempt(t *testing.T) {
	ctx := context.Background()
	repo := newTransitionLaneRepo(t)
	attempt := time.Now().UTC().Add(-25 * time.Hour)
	base := attempt.Add(-time.Hour)

	seed := &models.Draft{
		AuthorID: "owner", ID: "draft-1", Content: "before", ContentFormat: "markdown",
		Status: "publishing", CreatedAt: base, UpdatedAt: attempt, PublishAttemptedAt: &attempt,
	}
	require.NoError(t, seed.UpdateKeys())
	require.NoError(t, repo.CreateDraft(ctx, seed))

	loaded, err := repo.GetDraft(ctx, "owner", "draft-1")
	require.NoError(t, err)
	loaded.Content = "edited while publishing"
	require.NoError(t, repo.UpdateDraft(ctx, "owner", loaded))

	stored, err := repo.GetDraft(ctx, "owner", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "edited while publishing", stored.Content)
	require.Equal(t, attempt, *stored.PublishAttemptedAt,
		"the content lane never selects PublishAttemptedAt; the stored stamp must survive the edit")
}

func TestTransitionDraftToPublishingMissingRowFailsClosed(t *testing.T) {
	ctx := context.Background()
	repo := newTransitionLaneRepo(t)

	now := time.Now().UTC()
	ghost := &models.Draft{
		AuthorID: "owner", ID: "missing", Content: "x", ContentFormat: "markdown",
		Status: "publishing", UpdatedAt: now, PublishAttemptedAt: &now,
	}
	err := repo.TransitionDraftToPublishing(ctx, "owner", ghost)
	require.Error(t, err, "a transition onto a missing row must fail closed via the PK condition")
	require.True(t, strings.Contains(strings.ToLower(err.Error()), "not found") ||
		strings.Contains(strings.ToLower(err.Error()), "condition"),
		"the failure must surface as the mapped not-found/condition error, got: %v", err)
}
