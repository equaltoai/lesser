package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeClaims struct {
	username string
}

func (f fakeClaims) HasScope(string) bool { return true }
func (f fakeClaims) GetUsername() string  { return f.username }

type fakeNotesService struct {
	noteByID        map[string]*models.Status
	boostStates     map[string]bool
	reblogErr       error
	unreblogErr     error
	getNoteErr      error
	hasRebloggedErr error
	reblogCalls     int
	unreblogCalls   int
	lastStatusID    string
	lastUser        string
}

func (f *fakeNotesService) ReblogNote(_ context.Context, cmd *notes.ReblogNoteCommand) (*notes.LikeResult, error) {
	f.reblogCalls++
	f.lastStatusID = cmd.StatusID
	f.lastUser = cmd.RebloggerID
	return &notes.LikeResult{Status: f.noteByID[cmd.StatusID]}, f.reblogErr
}

func (f *fakeNotesService) UnreblogNote(_ context.Context, cmd *notes.UnreblogNoteCommand) (*notes.LikeResult, error) {
	f.unreblogCalls++
	f.lastStatusID = cmd.StatusID
	f.lastUser = cmd.UnrebloggerID
	return &notes.LikeResult{Status: f.noteByID[cmd.StatusID]}, f.unreblogErr
}

func (f *fakeNotesService) GetNote(_ context.Context, id string) (*models.Status, error) {
	return f.noteByID[id], f.getNoteErr
}

func (f *fakeNotesService) HasReblogged(_ context.Context, _, statusID string) (bool, error) {
	if f.hasRebloggedErr != nil {
		return false, f.hasRebloggedErr
	}
	return f.boostStates[statusID], nil
}

func TestShareObjectReturnsUpdatedObject(t *testing.T) {
	now := time.Now()
	status := &models.Status{
		StatusID:    "status-share",
		Content:     "hello world",
		CreatedAt:   now,
		UpdatedAt:   now,
		Note:        &models.NoteField{Note: &activitypub.Note{}},
		ReblogCount: 42,
	}

	fakeService := &fakeNotesService{
		noteByID: map[string]*models.Status{
			status.StatusID: status,
		},
		boostStates: map[string]bool{
			status.StatusID: true,
		},
	}

	resolver := &Resolver{Logger: zap.NewNop()}
	resolver.notesClient = fakeService
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, fakeClaims{username: "viewer"})

	result, err := (&mutationResolver{resolver}).ShareObject(ctx, status.StatusID)
	require.NoError(t, err)
	require.Equal(t, status.ReblogCount, result.SharesCount)
	require.True(t, result.Boosted)
	require.Equal(t, 1, fakeService.reblogCalls)
	require.Equal(t, status.StatusID, fakeService.lastStatusID)
}

func TestUnshareObjectReturnsUpdatedObject(t *testing.T) {
	now := time.Now()
	status := &models.Status{
		StatusID:    "status-unshare",
		Content:     "goodbye world",
		CreatedAt:   now,
		UpdatedAt:   now,
		Note:        &models.NoteField{Note: &activitypub.Note{}},
		ReblogCount: 7,
	}

	fakeService := &fakeNotesService{
		noteByID: map[string]*models.Status{
			status.StatusID: status,
		},
		boostStates: map[string]bool{
			status.StatusID: false,
		},
	}

	resolver := &Resolver{Logger: zap.NewNop()}
	resolver.notesClient = fakeService
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, fakeClaims{username: "viewer"})

	result, err := (&mutationResolver{resolver}).UnshareObject(ctx, status.StatusID)
	require.NoError(t, err)
	require.Equal(t, status.ReblogCount, result.SharesCount)
	require.False(t, result.Boosted)
	require.Equal(t, 1, fakeService.unreblogCalls)
	require.Equal(t, status.StatusID, fakeService.lastStatusID)
}

func TestConvertStatusToObjectSetsBoosted(t *testing.T) {
	resolver := &Resolver{Logger: zap.NewNop()}
	now := time.Now()
	status := &models.Status{
		StatusID:  "status-boosted",
		Content:   "content",
		CreatedAt: now,
		UpdatedAt: now,
	}

	withViewerBoostResolverStub(t, func(ctx context.Context, r *Resolver, viewerID, statusID string) (bool, error) {
		require.Equal(t, "viewer", viewerID)
		require.Equal(t, status.StatusID, statusID)
		return true, nil
	})

	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, fakeClaims{username: "viewer"})

	result := resolver.convertStatusToObject(ctx, status)
	require.NotNil(t, result)
	require.True(t, result.Boosted)
}

func withViewerBoostResolverStub(t *testing.T, fn func(context.Context, *Resolver, string, string) (bool, error)) {
	t.Helper()
	original := viewerBoostStateResolverFunc
	viewerBoostStateResolverFunc = fn
	t.Cleanup(func() {
		viewerBoostStateResolverFunc = original
	})
}
