package quotes

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type statusRepoWithErrors struct {
	statuses map[string]*models.Status
	getErr   map[string]error
}

func (r *statusRepoWithErrors) CreateStatus(ctx context.Context, status *models.Status) error {
	if r.statuses == nil {
		r.statuses = map[string]*models.Status{}
	}
	r.statuses[status.StatusID] = status
	return nil
}

func (r *statusRepoWithErrors) GetStatus(ctx context.Context, statusID string) (*models.Status, error) {
	if r.getErr != nil {
		if err, ok := r.getErr[statusID]; ok {
			return nil, err
		}
	}
	if r.statuses == nil {
		return nil, nil
	}
	return r.statuses[statusID], nil
}

func (r *statusRepoWithErrors) UpdateStatus(ctx context.Context, status *models.Status) error {
	if r.statuses == nil {
		r.statuses = map[string]*models.Status{}
	}
	r.statuses[status.StatusID] = status
	return nil
}

func TestQuoteService_GetQuotePermissions_ReturnsDefaultsWhenMissing(t *testing.T) {
	t.Parallel()

	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	perms, err := qs.GetQuotePermissions(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, perms)
	require.Equal(t, "alice", perms.Username)
	require.True(t, perms.AllowPublic)
}

func TestQuoteService_UpdateQuotePermissions_CreatesOrUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	perms := &models.QuotePermissions{Username: "alice"}
	require.NoError(t, qs.UpdateQuotePermissions(ctx, perms))
	require.Equal(t, 1, quoteRepo.createPermissionsCall)
	require.Equal(t, 0, quoteRepo.updatePermissionsCall)

	quoteRepo.permissions = map[string]*models.QuotePermissions{"alice": {Username: "alice"}}
	perms2 := &models.QuotePermissions{Username: "alice", AllowPublic: false}
	require.NoError(t, qs.UpdateQuotePermissions(ctx, perms2))
	require.Equal(t, 1, quoteRepo.updatePermissionsCall)
}

func TestQuoteService_UpdateQuotePermissions_ReturnsErrCheckExistingPermissions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	quoteRepo := &fakeQuoteRepo{getPermissionsErr: errors.New("boom")}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	err := qs.UpdateQuotePermissions(ctx, &models.QuotePermissions{Username: "alice"})
	require.Error(t, err)
}

func TestQuoteService_DeleteQuotePost_HandlesMissingOwnershipAndWithdrawErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	quoteRepo := &fakeQuoteRepo{relationships: map[string]*models.QuoteRelationship{}}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	require.ErrorIs(t, qs.DeleteQuotePost(ctx, "q1", "t1", "alice"), ErrQuoteRelationshipNotFound)

	quoteRepo.relationships[quoteRelKey("q1", "t1")] = &models.QuoteRelationship{
		QuoterNoteID: "q1",
		TargetNoteID: "t1",
		QuoterID:     "bob",
	}
	require.ErrorIs(t, qs.DeleteQuotePost(ctx, "q1", "t1", "alice"), ErrNotAuthorizedToDeleteQuote)

	quoteRepo.relationships[quoteRelKey("q1", "t1")] = &models.QuoteRelationship{
		QuoterNoteID: "q1",
		TargetNoteID: "t1",
		QuoterID:     "alice",
	}
	quoteRepo.updateRelErr = errors.New("update failed")
	require.ErrorIs(t, qs.DeleteQuotePost(ctx, "q1", "t1", "alice"), ErrWithdrawQuoteRelationship)
}

func TestQuoteService_GetQuotesForStatus_SkipsWithdrawnAndMissingOrErroredStatuses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	statusRepo := &statusRepoWithErrors{
		statuses: map[string]*models.Status{
			"q1": {StatusID: "q1", Content: "one"},
			"q2": {StatusID: "q2", Content: "two"},
		},
		getErr: map[string]error{
			"q3": errors.New("boom"),
		},
	}

	quoteRepo := &fakeQuoteRepo{
		quotesForStatus: map[string][]*models.QuoteRelationship{
			"t1": {
				{QuoterNoteID: "q1", TargetNoteID: "t1", Withdrawn: false},
				{QuoterNoteID: "q2", TargetNoteID: "t1", Withdrawn: true},
				{QuoterNoteID: "q3", TargetNoteID: "t1", Withdrawn: false},
				{QuoterNoteID: "missing", TargetNoteID: "t1", Withdrawn: false},
			},
		},
	}

	qs := &QuoteService{
		storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	out, err := qs.GetQuotesForStatus(ctx, "t1", 10, 0)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "q1", out[0].StatusID)
}

func TestQuoteService_GetQuotesForStatus_ReturnsErrGetQuoteRelationshipsOnRepoError(t *testing.T) {
	t.Parallel()

	quoteRepo := &fakeQuoteRepo{getQuotesErr: errors.New("boom")}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	_, err := qs.GetQuotesForStatus(context.Background(), "t1", 10, 0)
	require.Error(t, err)
}

func TestQuoteService_WithdrawFromQuotes_CoversErrorsAndSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{
		statuses: map[string]*models.Status{
			"note-1": {StatusID: "note-1", AuthorUsername: "bob", Visibility: "public"},
		},
	}
	quoteRepo := &fakeQuoteRepo{withdrawCount: 2}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	_, _, err := qs.WithdrawFromQuotes(ctx, "", "user")
	require.Error(t, err)

	statusRepo.getErr = errors.New("boom")
	_, _, err = qs.WithdrawFromQuotes(ctx, "note-1", "user")
	require.Error(t, err)
	statusRepo.getErr = nil

	quoteRepo.withdrawErr = errors.New("withdraw failed")
	_, _, err = qs.WithdrawFromQuotes(ctx, "note-1", "user")
	require.Error(t, err)
	quoteRepo.withdrawErr = nil

	note, count, err := qs.WithdrawFromQuotes(ctx, "note-1", "user")
	require.NoError(t, err)
	require.NotNil(t, note)
	require.Equal(t, 2, count)
}

func TestQuoteService_setQuoteReference_WarnsOnUpdateError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{updateErr: errors.New("update failed")}
	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	target := &models.Status{StatusID: "t1", AuthorID: "author-id"}
	quote := &models.Status{StatusID: "q1", AuthorUsername: "alice"}

	qs.setQuoteReference(ctx, quote, target)
	require.Equal(t, 1, statusRepo.updates)
}

func TestQuoteService_AttachQuoteToStatus_ValidationAndMissingTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{statuses: map[string]*models.Status{}}
	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	_, err := qs.AttachQuoteToStatus(ctx, nil, "target")
	require.Error(t, err)

	_, err = qs.AttachQuoteToStatus(ctx, &models.Status{StatusID: "q1", AuthorUsername: "alice"}, "")
	require.Error(t, err)

	_, err = qs.AttachQuoteToStatus(ctx, &models.Status{StatusID: "q1", AuthorUsername: "alice"}, "missing")
	require.Error(t, err)
}

func TestQuoteService_getQuoteRelationships_UsesLimit(t *testing.T) {
	t.Parallel()

	quoteRepo := &fakeQuoteRepo{
		quotesForStatus: map[string][]*models.QuoteRelationship{
			"t1": {
				{QuoterNoteID: "q1"},
				{QuoterNoteID: "q2"},
			},
		},
	}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: &fakeQuoteStatusRepo{}, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	rels, err := qs.getQuoteRelationships(context.Background(), "t1", 1, 0)
	require.NoError(t, err)
	require.Len(t, rels, 1)
}

func TestQuoteService_PlaceholderHelpers_Coverage(t *testing.T) {
	t.Parallel()

	qs := &QuoteService{logger: zap.NewNop()}
	require.False(t, qs.isStatusQuotable(&models.Status{Visibility: "private"}))
	require.NoError(t, qs.updateQuoteCounts(context.Background(), "t1", 1))
	require.NoError(t, qs.createQuoteNotification(context.Background(), &models.Status{AuthorUsername: "alice"}, &models.Status{AuthorUsername: "bob"}))
	ok, err := qs.checkFollowRelationship(context.Background(), "alice", "bob")
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, qs.checkMentioned(&models.Status{Content: "hi"}, "alice"))

	_ = generateStatusID()
	_ = interfaces.PaginationOptions{Limit: 1}
}
