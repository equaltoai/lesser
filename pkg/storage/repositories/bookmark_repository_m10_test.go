package repositories

import (
	"context"
	"encoding/base64"
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
	mockQuery.On("FilterGroup", mock.Anything).Return(mockQuery)
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
				SK:       legacyBookmarkSK(createdAt, "status-1"),
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
	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]models.Bookmark)
		if allCalls == 1 {
			*dest = []models.Bookmark{
				{
					PK:        pk,
					SK:        "TIME#" + base.Add(2*time.Minute).Format(time.RFC3339Nano) + "#status-time",
					Username:  "alice",
					ObjectID:  "status-time",
					CreatedAt: base.Add(2 * time.Minute),
					Locked:    false,
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
			return
		}
		*dest = []models.Bookmark{
			{
				PK:       pk,
				SK:       legacyBookmarkSK(base.Add(3*time.Minute), "status-legacy"),
				Username: "alice",
				ObjectID: "status-legacy",
				Locked:   false,
			},
		}
	}).Return(nil).Twice()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	items, nextCursor, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 10, "")
	require.NoError(t, err)
	require.Empty(t, nextCursor)
	require.Len(t, items, 2)
	require.Equal(t, "status-legacy", items[0].ObjectID)
	require.Equal(t, base.Add(3*time.Minute), bookmarkCreatedAt(items[0]))
	require.Equal(t, "status-time", items[1].ObjectID)
}

func TestBookmarkRepository_M10_QueryUnlockedTimeBookmarksPaginatesMixedNamespaces(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	base := time.Date(2025, 12, 28, 12, 0, 0, 0, time.UTC)
	pk := buildBookmarkPK("alice")

	timeSK4 := "TIME#" + base.Add(4*time.Minute).Format(time.RFC3339Nano) + "#status-time-4"
	timeSK1 := "TIME#" + base.Add(time.Minute).Format(time.RFC3339Nano) + "#status-time-1"
	legacySK3 := legacyBookmarkSK(base.Add(3*time.Minute), "status-legacy-3")
	legacySK2 := legacyBookmarkSK(base.Add(2*time.Minute), "status-legacy-2")

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	allCalls := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		allCalls++
		dest := args.Get(0).(*[]models.Bookmark)
		switch allCalls {
		case 1:
			*dest = []models.Bookmark{
				{PK: pk, SK: timeSK4, Username: "alice", ObjectID: "status-time-4", CreatedAt: base.Add(4 * time.Minute)},
				{PK: pk, SK: timeSK1, Username: "alice", ObjectID: "status-time-1", CreatedAt: base.Add(time.Minute)},
			}
		case 2:
			*dest = []models.Bookmark{
				{PK: pk, SK: legacySK3, Username: "alice", ObjectID: "status-legacy-3"},
				{PK: pk, SK: legacySK2, Username: "alice", ObjectID: "status-legacy-2"},
			}
		case 3:
			*dest = []models.Bookmark{{PK: pk, SK: timeSK1, Username: "alice", ObjectID: "status-time-1", CreatedAt: base.Add(time.Minute)}}
		case 4:
			*dest = []models.Bookmark{{PK: pk, SK: legacySK2, Username: "alice", ObjectID: "status-legacy-2"}}
		default:
			*dest = nil
		}
	}).Return(nil)

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	firstPage, cursor, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 2, "")
	require.NoError(t, err)
	require.NotEmpty(t, cursor)
	require.Equal(t, []string{"status-time-4", "status-legacy-3"}, bookmarkObjectIDs(firstPage))

	pageCursor, err := parseBookmarkPageCursor(cursor)
	require.NoError(t, err)
	require.Equal(t, timeSK4, pageCursor.TimeSK)
	require.Equal(t, legacySK3, pageCursor.LegacySK)

	secondPage, nextCursor, err := repo.queryUnlockedTimeBookmarks(ctx, "alice", 2, cursor)
	require.NoError(t, err)
	require.Empty(t, nextCursor)
	require.Equal(t, []string{"status-legacy-2", "status-time-1"}, bookmarkObjectIDs(secondPage))
	require.Equal(t, 4, allCalls)
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
			{PK: pk, SK: legacyBookmarkSK(base.Add(-time.Minute), "status-legacy"), ObjectID: "status-legacy", Locked: false},
			{PK: pk, SK: "OBJECT#status-time", ObjectID: "status-time", RecordType: models.BookmarkRecordTypeObject},
			{PK: pk, SK: "TIME#" + base.Add(-2*time.Minute).Format(time.RFC3339Nano) + "#status-locked", ObjectID: "status-locked", Locked: true},
		}
	}).Return(nil).Once()

	repo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	count, err := repo.CountUserBookmarks(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestBookmarkRepository_M10_BookmarkPageCursorCompatibility(t *testing.T) {
	createdAt := time.Date(2025, 12, 28, 12, 30, 0, 123, time.UTC)
	timeSK := "TIME#" + createdAt.Format(time.RFC3339Nano) + "#status-time"
	legacySK := legacyBookmarkSK(createdAt, "status-legacy")
	legacyTimeUpperBound := createdAt.Format(time.RFC3339Nano) + "\xff"

	fromTime, err := parseBookmarkPageCursor(timeSK)
	require.NoError(t, err)
	require.Equal(t, timeSK, fromTime.TimeSK)
	require.Equal(t, legacyTimeUpperBound, fromTime.LegacySK)

	fromLegacy, err := parseBookmarkPageCursor(legacySK)
	require.NoError(t, err)
	require.Equal(t, legacySK, fromLegacy.LegacySK)
	require.Equal(t, "TIME#"+createdAt.Format(time.RFC3339Nano), fromLegacy.TimeSK)

	encoded := encodeBookmarkPageCursor(bookmarkPageCursor{TimeSK: timeSK, LegacySK: legacySK})
	decoded, err := parseBookmarkPageCursor(encoded)
	require.NoError(t, err)
	require.Equal(t, 2, decoded.Version)
	require.Equal(t, timeSK, decoded.TimeSK)
	require.Equal(t, legacySK, decoded.LegacySK)

	_, err = parseBookmarkPageCursor(bookmarkPageCursorPrefix + "not-base64!")
	require.Error(t, err)

	invalidLegacy, err := parseBookmarkPageCursor("zzzz-attacker-cursor")
	require.NoError(t, err)
	require.Equal(t, 2, invalidLegacy.Version)
	require.Empty(t, invalidLegacy.TimeSK)
	require.Empty(t, invalidLegacy.LegacySK)

	malicious := bookmarkPageCursorPrefix + base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"t":"zzzz-attacker-cursor","l":"zzzz-attacker-cursor"}`))
	_, err = parseBookmarkPageCursor(malicious)
	require.Error(t, err)
}

func bookmarkObjectIDs(bookmarks []models.Bookmark) []string {
	objectIDs := make([]string, len(bookmarks))
	for i := range bookmarks {
		objectIDs[i] = bookmarks[i].ObjectID
	}
	return objectIDs
}
