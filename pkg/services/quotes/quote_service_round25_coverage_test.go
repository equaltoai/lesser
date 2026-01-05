package quotes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeQuoteStatusRepo struct {
	statuses  map[string]*models.Status
	getErr    error
	createErr error
	updateErr error
	updates   int
}

func (f *fakeQuoteStatusRepo) CreateStatus(_ context.Context, status *models.Status) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.statuses == nil {
		f.statuses = map[string]*models.Status{}
	}
	f.statuses[status.StatusID] = status
	return nil
}

func (f *fakeQuoteStatusRepo) GetStatus(_ context.Context, statusID string) (*models.Status, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.statuses == nil {
		return nil, nil
	}
	return f.statuses[statusID], nil
}

func (f *fakeQuoteStatusRepo) UpdateStatus(_ context.Context, status *models.Status) error {
	f.updates++
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.statuses == nil {
		f.statuses = map[string]*models.Status{}
	}
	f.statuses[status.StatusID] = status
	return nil
}

type fakeQuoteRepo struct {
	permissions           map[string]*models.QuotePermissions
	relationships         map[string]*models.QuoteRelationship
	quotesForStatus       map[string][]*models.QuoteRelationship
	createRelErr          error
	getRelErr             error
	updateRelErr          error
	getQuotesErr          error
	withdrawErr           error
	withdrawCount         int
	getPermissionsErr     error
	createPermissionsErr  error
	updatePermissionsErr  error
	createPermissionsCall int
	updatePermissionsCall int
}

func quoteRelKey(quoteStatusID, targetStatusID string) string {
	return quoteStatusID + "->" + targetStatusID
}

func (f *fakeQuoteRepo) CreateQuoteRelationship(_ context.Context, relationship *models.QuoteRelationship) error {
	if f.createRelErr != nil {
		return f.createRelErr
	}
	if f.relationships == nil {
		f.relationships = map[string]*models.QuoteRelationship{}
	}
	f.relationships[quoteRelKey(relationship.QuoterNoteID, relationship.TargetNoteID)] = relationship
	return nil
}

func (f *fakeQuoteRepo) GetQuoteRelationship(_ context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	if f.getRelErr != nil {
		return nil, f.getRelErr
	}
	if f.relationships == nil {
		return nil, nil
	}
	return f.relationships[quoteRelKey(quoteStatusID, targetStatusID)], nil
}

func (f *fakeQuoteRepo) UpdateQuoteRelationship(_ context.Context, relationship *models.QuoteRelationship) error {
	if f.updateRelErr != nil {
		return f.updateRelErr
	}
	if f.relationships == nil {
		f.relationships = map[string]*models.QuoteRelationship{}
	}
	f.relationships[quoteRelKey(relationship.QuoterNoteID, relationship.TargetNoteID)] = relationship
	return nil
}

func (f *fakeQuoteRepo) GetQuotesForStatus(_ context.Context, statusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	if f.getQuotesErr != nil {
		return nil, f.getQuotesErr
	}
	items := f.quotesForStatus[statusID]
	if opts.Limit > 0 && len(items) > opts.Limit {
		items = items[:opts.Limit]
	}
	return &interfaces.PaginatedResult[*models.QuoteRelationship]{Items: items}, nil
}

func (f *fakeQuoteRepo) WithdrawQuotes(_ context.Context, _ string, _ string) (int, error) {
	if f.withdrawErr != nil {
		return 0, f.withdrawErr
	}
	return f.withdrawCount, nil
}

func (f *fakeQuoteRepo) CreateQuotePermissions(_ context.Context, permissions *models.QuotePermissions) error {
	f.createPermissionsCall++
	if f.createPermissionsErr != nil {
		return f.createPermissionsErr
	}
	if f.permissions == nil {
		f.permissions = map[string]*models.QuotePermissions{}
	}
	f.permissions[permissions.Username] = permissions
	return nil
}

func (f *fakeQuoteRepo) GetQuotePermissions(_ context.Context, username string) (*models.QuotePermissions, error) {
	if f.getPermissionsErr != nil {
		return nil, f.getPermissionsErr
	}
	if f.permissions == nil {
		return nil, storage.ErrNotFound
	}
	p, ok := f.permissions[username]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return p, nil
}

func (f *fakeQuoteRepo) UpdateQuotePermissions(_ context.Context, permissions *models.QuotePermissions) error {
	f.updatePermissionsCall++
	if f.updatePermissionsErr != nil {
		return f.updatePermissionsErr
	}
	if f.permissions == nil {
		f.permissions = map[string]*models.QuotePermissions{}
	}
	f.permissions[permissions.Username] = permissions
	return nil
}

type fakeQuoteStorage struct {
	status quoteStatusRepository
	quote  quoteRepository
}

func (f fakeQuoteStorage) Status() quoteStatusRepository { return f.status }
func (f fakeQuoteStorage) Quote() quoteRepository        { return f.quote }

func TestQuoteService_Round25_CreateQuotePost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{
		statuses: map[string]*models.Status{
			"target-1": {
				StatusID:       "target-1",
				AuthorUsername: "bob",
				AuthorID:       "bob-id",
				Visibility:     "public",
				QuoteCount:     0,
			},
		},
	}
	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{
		storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo},
		logger:  zap.NewNop(),
	}

	t.Run("invalid request returns validation error", func(t *testing.T) {
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "", TargetStatusID: ""})
		require.Error(t, err)
	})

	t.Run("target status missing returns not found error", func(t *testing.T) {
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "missing"})
		require.Error(t, err)
	})

	t.Run("target status not quotable returns business rule error", func(t *testing.T) {
		statusRepo.statuses["private"] = &models.Status{StatusID: "private", AuthorUsername: "bob", Visibility: "private"}
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "private"})
		require.Error(t, err)
	})

	t.Run("blocked user is not authorized", func(t *testing.T) {
		quoteRepo.permissions = map[string]*models.QuotePermissions{
			"bob": {
				Username:       "bob",
				AllowPublic:    false,
				AllowFollowers: false,
				AllowMentioned: false,
				BlockList:      []string{"alice"},
			},
		}
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "target-1", Visibility: "public"})
		require.Error(t, err)
	})

	t.Run("relationship creation error is surfaced", func(t *testing.T) {
		quoteRepo.permissions = nil
		quoteRepo.createRelErr = errors.New("boom")
		_, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{QuoterUsername: "alice", TargetStatusID: "target-1", Visibility: "public"})
		require.Error(t, err)
		quoteRepo.createRelErr = nil
	})

	t.Run("happy path sets quote reference and returns result", func(t *testing.T) {
		res, err := qs.CreateQuotePost(ctx, &CreateQuoteRequest{
			QuoterUsername: "alice",
			TargetStatusID: "target-1",
			Content:        "great post",
			Visibility:     "public",
		})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.QuoteStatus)
		require.NotNil(t, res.QuoteRelationship)
		require.NotNil(t, res.TargetStatus)

		assert.Equal(t, "alice", res.QuoteStatus.AuthorUsername)
		assert.NotEmpty(t, res.QuoteStatus.StatusID)
		assert.Equal(t, "target-1", res.QuoteStatus.QuoteTargetStatusID)
		assert.Equal(t, "bob-id", res.QuoteStatus.QuoteTargetAuthorID)
		assert.Equal(t, "target-1", res.TargetStatus.StatusID)
	})

	t.Run("setQuoteReference no-ops when already set", func(t *testing.T) {
		beforeUpdates := statusRepo.updates
		quoteStatus := &models.Status{StatusID: "q1", QuoteTargetStatusID: "target-1", QuoteTargetAuthorID: "bob-id"}
		target := statusRepo.statuses["target-1"]
		qs.setQuoteReference(ctx, quoteStatus, target)
		assert.Equal(t, beforeUpdates, statusRepo.updates)
	})

	t.Run("setQuoteReference tolerates update errors", func(t *testing.T) {
		statusRepo.updateErr = errors.New("update failed")
		quoteStatus := &models.Status{StatusID: "q2"}
		target := statusRepo.statuses["target-1"]
		qs.setQuoteReference(ctx, quoteStatus, target)
		statusRepo.updateErr = nil
	})
}

func TestQuoteService_Round25_GetQuotesForStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{
		statuses: map[string]*models.Status{
			"q1": {StatusID: "q1", Content: "quote"},
		},
		getErr: nil,
	}
	quoteRepo := &fakeQuoteRepo{
		quotesForStatus: map[string][]*models.QuoteRelationship{
			"target": {
				{QuoterNoteID: "q1", Withdrawn: false},
				{QuoterNoteID: "q2", Withdrawn: true},
			},
		},
	}
	qs := &QuoteService{storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo}, logger: zap.NewNop()}

	got, err := qs.GetQuotesForStatus(ctx, "target", 10, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "q1", got[0].StatusID)

	quoteRepo.getQuotesErr = errors.New("boom")
	_, err = qs.GetQuotesForStatus(ctx, "target", 10, 0)
	require.Error(t, err)
}

func TestQuoteService_Round25_DeleteQuotePost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{}
	quoteRepo := &fakeQuoteRepo{
		relationships: map[string]*models.QuoteRelationship{
			quoteRelKey("q1", "t1"): {QuoterNoteID: "q1", TargetNoteID: "t1", QuoterID: "alice", Timestamp: time.Now()},
		},
	}
	qs := &QuoteService{storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo}, logger: zap.NewNop()}

	t.Run("missing relationship returns not found error", func(t *testing.T) {
		err := qs.DeleteQuotePost(ctx, "missing", "t1", "alice")
		require.Error(t, err)
	})

	t.Run("ownership enforced", func(t *testing.T) {
		err := qs.DeleteQuotePost(ctx, "q1", "t1", "bob")
		require.Error(t, err)
	})

	t.Run("withdraw update error surfaces", func(t *testing.T) {
		quoteRepo.updateRelErr = errors.New("boom")
		err := qs.DeleteQuotePost(ctx, "q1", "t1", "alice")
		require.Error(t, err)
		quoteRepo.updateRelErr = nil
	})

	t.Run("success withdraws relationship", func(t *testing.T) {
		err := qs.DeleteQuotePost(ctx, "q1", "t1", "alice")
		require.NoError(t, err)
		rel, _ := quoteRepo.GetQuoteRelationship(ctx, "q1", "t1")
		require.NotNil(t, rel)
		assert.True(t, rel.Withdrawn)
	})
}

func TestQuoteService_Round25_GetAndUpdateQuotePermissions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{}
	quoteRepo := &fakeQuoteRepo{}
	qs := &QuoteService{storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo}, logger: zap.NewNop()}

	perms, err := qs.GetQuotePermissions(ctx, "alice")
	require.NoError(t, err)
	require.NotNil(t, perms)
	assert.True(t, perms.AllowPublic)

	quoteRepo.getPermissionsErr = errors.New("boom")
	_, err = qs.GetQuotePermissions(ctx, "alice")
	require.Error(t, err)
	quoteRepo.getPermissionsErr = nil

	t.Run("creates when missing", func(t *testing.T) {
		quoteRepo.permissions = nil
		quoteRepo.createPermissionsCall = 0
		quoteRepo.updatePermissionsCall = 0
		err := qs.UpdateQuotePermissions(ctx, &models.QuotePermissions{Username: "alice"})
		require.NoError(t, err)
		assert.Equal(t, 1, quoteRepo.createPermissionsCall)
		assert.Equal(t, 0, quoteRepo.updatePermissionsCall)
	})

	t.Run("updates when existing", func(t *testing.T) {
		quoteRepo.permissions = map[string]*models.QuotePermissions{"alice": {Username: "alice", AllowPublic: true}}
		quoteRepo.createPermissionsCall = 0
		quoteRepo.updatePermissionsCall = 0
		err := qs.UpdateQuotePermissions(ctx, &models.QuotePermissions{Username: "alice", AllowPublic: false})
		require.NoError(t, err)
		assert.Equal(t, 0, quoteRepo.createPermissionsCall)
		assert.Equal(t, 1, quoteRepo.updatePermissionsCall)
	})

	t.Run("save error surfaces", func(t *testing.T) {
		quoteRepo.permissions = nil
		quoteRepo.createPermissionsErr = errors.New("boom")
		err := qs.UpdateQuotePermissions(ctx, &models.QuotePermissions{Username: "alice"})
		require.Error(t, err)
		quoteRepo.createPermissionsErr = nil
	})
}

func TestQuoteService_Round25_WithdrawFromQuotes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	statusRepo := &fakeQuoteStatusRepo{
		statuses: map[string]*models.Status{"n1": {StatusID: "n1", Content: "note"}},
	}
	quoteRepo := &fakeQuoteRepo{withdrawCount: 2}
	qs := &QuoteService{storage: fakeQuoteStorage{status: statusRepo, quote: quoteRepo}, logger: zap.NewNop()}

	_, _, err := qs.WithdrawFromQuotes(ctx, "", "alice")
	require.Error(t, err)

	_, _, err = qs.WithdrawFromQuotes(ctx, "n1", "")
	require.Error(t, err)

	quoteRepo.withdrawErr = errors.New("boom")
	_, _, err = qs.WithdrawFromQuotes(ctx, "n1", "alice")
	require.Error(t, err)
	quoteRepo.withdrawErr = nil

	note, count, err := qs.WithdrawFromQuotes(ctx, "n1", "alice")
	require.NoError(t, err)
	require.NotNil(t, note)
	assert.Equal(t, 2, count)
}

