package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestBookmarkRepository_Round08_HelperUtilities(t *testing.T) {
	require.Equal(t, "BOOKMARK#alice", buildBookmarkPK("alice"))
	require.Equal(t, "OBJECT#status-1", buildObjectSK("status-1"))

	require.Equal(t, 20, sanitizeLimit(0, 20, 100))
	require.Equal(t, 100, sanitizeLimit(200, 20, 100))
	require.Equal(t, 33, sanitizeLimit(33, 20, 100))

	require.Equal(t, []string{"a", "b"}, deduplicate([]string{"a", "a", "b"}))
}

func TestBookmarkRepository_Round08_CountAndQueryUnlockedTimeBookmarks(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	base := time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC)
	pk := buildBookmarkPK("alice")
	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]models.Bookmark)
		if allCalls == 1 {
			*dest = []models.Bookmark{
				{PK: pk, SK: "TIME#" + base.Add(2*time.Minute).Format(time.RFC3339Nano) + "#o3", ObjectID: "o3", CreatedAt: base.Add(2 * time.Minute), Locked: false},
				{PK: pk, SK: "TIME#" + base.Add(time.Minute).Format(time.RFC3339Nano) + "#o2", ObjectID: "o2", CreatedAt: base.Add(time.Minute), Locked: false},
				{PK: pk, SK: "OBJECT#o1", ObjectID: "o1", CreatedAt: base, RecordType: models.BookmarkRecordTypeObject},
				{PK: pk, SK: legacyBookmarkSK(base.Add(-time.Minute), "legacy"), ObjectID: "legacy", CreatedAt: base.Add(-time.Minute), Locked: false},
			}
			return
		}
		if allCalls == 2 {
			*dest = []models.Bookmark{
				{PK: pk, SK: "TIME#" + base.Add(2*time.Minute).Format(time.RFC3339Nano) + "#o3", ObjectID: "o3", CreatedAt: base.Add(2 * time.Minute), Locked: false},
				{PK: pk, SK: "TIME#" + base.Add(time.Minute).Format(time.RFC3339Nano) + "#o2", ObjectID: "o2", CreatedAt: base.Add(time.Minute), Locked: false},
			}
			return
		}
		*dest = []models.Bookmark{
			{PK: pk, SK: legacyBookmarkSK(base.Add(-time.Minute), "legacy"), ObjectID: "legacy", CreatedAt: base.Add(-time.Minute), Locked: false},
		}
	}).Return(nil)

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())

	count, err := repo.CountUserBookmarks(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	items, nextCursor, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 2, "")
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "o3", items[0].ObjectID)
	require.Equal(t, "o2", items[1].ObjectID)
	pageCursor, err := parseBookmarkPageCursor(nextCursor)
	require.NoError(t, err)
	require.Equal(t, items[1].SK, pageCursor.TimeSK)
	require.Empty(t, pageCursor.LegacySK)
}

func TestBookmarkRepository_Round08_CascadeDeleteUserBookmarks_FallbackDeletes(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Query() is implemented with query.All; feed a batch once, then empty to stop.
	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]*models.Bookmark)
		if allCalls == 1 {
			*dest = []*models.Bookmark{
				{PK: buildBookmarkPK("alice"), SK: "OBJECT#o1"},
				{PK: buildBookmarkPK("alice"), SK: "TIME#t1"},
			}
			return
		}
		*dest = []*models.Bookmark{}
	}).Return(nil).Maybe()

	// BatchDelete fails so the repository falls back to individual Delete calls.
	mockQuery.On("BatchDelete", mock.Anything).Return(errors.New("batch delete failed")).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.CascadeDeleteUserBookmarks(ctx, "alice"))
}

func TestBookmarkRepository_Round08_CascadeDeleteObjectBookmarks_ScanAndBatchFallback(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Bookmark)
		*dest = []models.Bookmark{
			{PK: buildBookmarkPK("alice"), SK: "TIME#t1#obj-1", ObjectID: "obj-1"},
			{PK: buildBookmarkPK("bob"), SK: "TIME#t2#obj-1x", ObjectID: "obj-1x"}, // filtered out (partial match)
			{PK: buildBookmarkPK("carol"), SK: "OBJECT#obj-1", ObjectID: "obj-1"},
		}
	}).Return(nil).Once()

	// BatchDelete fails so it falls back to individual Delete calls.
	mockQuery.On("BatchDelete", mock.Anything).Return(errors.New("batch delete failed")).Once()

	// Force one Delete failure to hit both branches; permissive mocks handle the rest.
	mockQuery.On("Delete").Return(errors.New("delete failed")).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	require.NoError(t, repo.CascadeDeleteObjectBookmarks(ctx, "obj-1"))
}
