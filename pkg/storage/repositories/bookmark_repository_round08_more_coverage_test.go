package repositories

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestBookmarkRepository_Round08_GetBookmarkIsBookmarkedAndWrappers(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	created, err := repo.CreateBookmark(ctx, "alice", "status-1")
	require.NoError(t, err)

	got, err := repo.GetBookmark(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.Equal(t, created.ObjectID, got.ObjectID)

	ok, err := repo.IsBookmarked(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.True(t, ok)

	legacy, err := models.NewTimeOrderedBookmark("bob", "status-2", time.Now().UTC())
	require.NoError(t, err)
	legacy.Locked = false
	repo.store[repo.makeKey(legacy.PK, legacy.SK)] = legacy

	got, err = repo.GetBookmark(ctx, "bob", "status-2")
	require.NoError(t, err)
	require.Equal(t, legacy.ObjectID, got.ObjectID)

	ok, err = repo.IsBookmarked(ctx, "bob", "status-2")
	require.NoError(t, err)
	require.True(t, ok)

	lockedLegacy, err := models.NewTimeOrderedBookmark("carol", "status-3", time.Now().UTC())
	require.NoError(t, err)
	lockedLegacy.Locked = true
	repo.store[repo.makeKey(lockedLegacy.PK, lockedLegacy.SK)] = lockedLegacy

	_, err = repo.GetBookmark(ctx, "carol", "status-3")
	require.Error(t, err)

	ok, err = repo.IsBookmarked(ctx, "carol", "status-3")
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, repo.AddBookmark(ctx, "alice", "status-4"))
	require.NoError(t, repo.RemoveBookmark(ctx, "alice", "status-4"))

	bookmarks, _, err := repo.GetBookmarks(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.NotEmpty(t, bookmarks)
}

func TestBookmarkRepository_Round08_RepairAndDeleteLegacyHelpers(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	legacy, err := models.NewTimeOrderedBookmark("dana", "status-1", time.Now().UTC())
	require.NoError(t, err)
	legacy.Locked = true
	repo.store[repo.makeKey(legacy.PK, legacy.SK)] = legacy

	repaired, err := repo.repairLegacyBookmark(ctx, "dana", "status-1")
	require.NoError(t, err)
	require.NotNil(t, repaired)
	require.Equal(t, buildObjectSK("status-1"), repaired.SK)

	timeRecord := repo.store[repo.makeKey(legacy.PK, legacy.SK)]
	require.False(t, timeRecord.Locked)

	// deleteLegacyTimeBookmark: not found error is swallowed.
	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	require.NoError(t, repo.deleteLegacyTimeBookmark(ctx, "dana", "missing"))

	// deleteLegacyTimeBookmark: non-not-found error is returned.
	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, stdErrors.New("boom")
	}
	require.Error(t, repo.deleteLegacyTimeBookmark(ctx, "dana", "missing"))

	// deleteLegacyTimeBookmark: nil legacy is a no-op.
	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return nil, nil
	}
	require.NoError(t, repo.deleteLegacyTimeBookmark(ctx, "dana", "missing"))

	// deleteLegacyTimeBookmark: delete not-found is swallowed.
	repo.findTimeBookmarkFn = func(context.Context, string, string) (*models.Bookmark, error) {
		return legacy, nil
	}
	repo.transactWriteFn = func(context.Context, func(core.TransactionBuilder) error) error {
		return dynamormerrors.ErrItemNotFound
	}
	require.NoError(t, repo.deleteLegacyTimeBookmark(ctx, "dana", "status-1"))
}

type fakeTransactionalDB struct{}

func (fakeTransactionalDB) Model(any) core.Query                   { panic("unused") }
func (fakeTransactionalDB) Transaction(func(*core.Tx) error) error { return nil }
func (fakeTransactionalDB) Migrate() error                         { return nil }
func (fakeTransactionalDB) AutoMigrate(...any) error               { return nil }
func (fakeTransactionalDB) Close() error                           { return nil }
func (fakeTransactionalDB) WithContext(context.Context) core.DB    { return fakeTransactionalDB{} }
func (fakeTransactionalDB) TransactWrite(_ context.Context, fn func(core.TransactionBuilder) error) error {
	return fn(nil)
}

func TestBookmarkRepository_Round08_DynamoHelpers_TransactWriteBatchGetAndLookups(t *testing.T) {
	ctx := context.Background()

	// transactionalDB error path.
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	require.Error(t, repo.transactWrite(ctx, func(core.TransactionBuilder) error { return nil }))
	_, err := repo.transactionalDB()
	require.Error(t, err)

	_ = NewBookmarkRepositoryWithCostTracking(mockDB, "test-table", zap.NewNop(), nil)

	// transactWrite success path.
	repoTx := NewBookmarkRepository(fakeTransactionalDB{}, "test-table", zap.NewNop())
	require.NoError(t, repoTx.transactWrite(ctx, func(core.TransactionBuilder) error { return nil }))

	// batchGetBookmarks: empty keys short-circuit.
	got, err := repo.batchGetBookmarks(ctx, nil)
	require.NoError(t, err)
	require.Nil(t, got)

	// batchGetBookmarks: builder workflow.
	builder := new(mocks.MockBatchGetBuilder)
	builder.On("Keys", mock.Anything).Return(builder)
	builder.On("Parallel", 4).Return(builder)
	builder.On("WithRetry", mock.AnythingOfType("*core.RetryPolicy")).Return(builder)
	builder.On("OnError", mock.Anything).Run(func(args mock.Arguments) {
		handler := args.Get(0).(core.BatchChunkErrorHandler)
		_ = handler([]any{"k1"}, stdErrors.New("chunk"))
	}).Return(builder)
	builder.On("Execute", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		*dest = []models.Bookmark{
			{PK: "pk1", SK: "sk1", ObjectID: "o1"},
			{PK: "pk2", SK: "sk2", ObjectID: "o2"},
		}
	}).Return(nil)

	mockQuery.On("BatchGetBuilder").Return(builder)

	keys := []any{
		core.NewKeyPair("pk1", "sk1"),
		core.NewKeyPair("pk2", "sk2"),
	}
	out, err := repo.batchGetBookmarks(ctx, keys)
	require.NoError(t, err)
	require.Len(t, out, 2)

	// logTransactionError should include TransactionError metadata when present.
	repo.logTransactionError("plain", stdErrors.New("boom"))
	repo.logTransactionError("tx", &dynamormerrors.TransactionError{
		OperationIndex: 1,
		Operation:      "Update",
		Reason:         "ConditionalCheckFailed",
	})

	// dynamoGetObjectBookmark + dynamoFindTimeBookmarkByObject.
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Bookmark)
		*dest = models.Bookmark{PK: buildBookmarkPK("alice"), SK: buildObjectSK("status-1"), ObjectID: "status-1"}
	}).Return(nil).Once()
	found, err := repo.dynamoGetObjectBookmark(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.Equal(t, "status-1", found.ObjectID)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		*dest = []models.Bookmark{{PK: buildBookmarkPK("alice"), SK: "TIME#t1", ObjectID: "status-1"}}
	}).Return(nil).Once()
	found, err = repo.dynamoFindTimeBookmarkByObject(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.NotNil(t, found)

	mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Twice()
	found, err = repo.dynamoFindTimeBookmarkByObject(ctx, "alice", "missing")
	require.NoError(t, err)
	require.Nil(t, found)
}
