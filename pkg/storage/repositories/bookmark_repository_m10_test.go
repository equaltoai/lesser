package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestBookmarkRepository_M10_DynamoFindTimeBookmarkByObjectReadsLegacyTimestampSK(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	createdAt := time.Date(2025, 12, 28, 12, 30, 0, 123, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]models.Bookmark)
		if allCalls == 1 {
			*dest = []models.Bookmark{}
			return
		}
		*dest = []models.Bookmark{
			{
				PK:       buildBookmarkPK("alice"),
				SK:       createdAt.Format(time.RFC3339Nano),
				Username: "alice",
				ObjectID: "status-1",
				Locked:   false,
			},
		}
	}).Return(nil).Twice()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	found, err := repo.dynamoFindTimeBookmarkByObject(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "status-1", found.ObjectID)
	require.Equal(t, createdAt, found.CreatedAt)
	require.Equal(t, 2, allCalls)
}

func TestBookmarkRepository_M10_QueryUnlockedTimeBookmarksIncludesLegacyTimestampSKs(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	base := time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC)
	pk := buildBookmarkPK("alice")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		*dest = []models.Bookmark{
			{
				PK:         pk,
				SK:         "OBJECT#status-object",
				Username:   "alice",
				ObjectID:   "status-object",
				CreatedAt:  base.Add(4 * time.Minute),
				RecordType: models.BookmarkRecordTypeObject,
			},
			{
				PK:        pk,
				SK:        "TIME#" + base.Add(2*time.Minute).Format(time.RFC3339Nano) + "#status-time",
				Username:  "alice",
				ObjectID:  "status-time",
				CreatedAt: base.Add(2 * time.Minute),
				Locked:    false,
			},
			{
				PK:       pk,
				SK:       base.Add(3 * time.Minute).Format(time.RFC3339Nano),
				Username: "alice",
				ObjectID: "status-legacy",
				Locked:   false,
			},
			{
				PK:        pk,
				SK:        "TIME#" + base.Add(time.Minute).Format(time.RFC3339Nano) + "#status-locked",
				Username:  "alice",
				ObjectID:  "status-locked",
				CreatedAt: base.Add(time.Minute),
				Locked:    true,
			},
		}
	}).Return(nil).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	items, nextCursor, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Empty(t, nextCursor)
	require.Len(t, items, 2)
	require.Equal(t, "status-legacy", items[0].ObjectID)
	require.Equal(t, base.Add(3*time.Minute), bookmarkCreatedAt(items[0]))
	require.Equal(t, "status-time", items[1].ObjectID)
}

func TestBookmarkRepository_M10_CountUserBookmarksCountsLegacyAndTimeRecords(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	base := time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC)
	pk := buildBookmarkPK("alice")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		*dest = []models.Bookmark{
			{PK: pk, SK: "TIME#" + base.Format(time.RFC3339Nano) + "#status-time", ObjectID: "status-time", Locked: false},
			{PK: pk, SK: base.Add(-time.Minute).Format(time.RFC3339Nano), ObjectID: "status-legacy", Locked: false},
			{PK: pk, SK: "OBJECT#status-time", ObjectID: "status-time", RecordType: models.BookmarkRecordTypeObject},
			{PK: pk, SK: "TIME#" + base.Add(-2*time.Minute).Format(time.RFC3339Nano) + "#status-locked", ObjectID: "status-locked", Locked: true},
		}
	}).Return(nil).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.CountUserBookmarks(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}
