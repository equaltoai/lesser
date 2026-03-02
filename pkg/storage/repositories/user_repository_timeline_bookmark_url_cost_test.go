package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type mockUserRepositoryDeps struct {
	mock.Mock
}

func (m *mockUserRepositoryDeps) GetFollowers(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *mockUserRepositoryDeps) GetListsContainingAccount(ctx context.Context, accountID, username string) ([]*storage.List, error) {
	args := m.Called(ctx, accountID, username)
	if lists, ok := args.Get(0).([]*storage.List); ok {
		return lists, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserRepositoryDeps) CreateTimelineEntries(ctx context.Context, entries []*models.Timeline) error {
	args := m.Called(ctx, entries)
	return args.Error(0)
}

func (m *mockUserRepositoryDeps) GetPendingFollowRequests(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	args := m.Called(ctx, username, limit, cursor)
	return args.Get(0).([]string), args.String(1), args.Error(2)
}

func (m *mockUserRepositoryDeps) RemoveFollow(ctx context.Context, followerUsername, username string) error {
	args := m.Called(ctx, followerUsername, username)
	return args.Error(0)
}

func TestUserRepository_getBookmarkRepository_LazilyInitializes(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	assert.Nil(t, repo.bookmarkRepo)

	br := repo.getBookmarkRepository()
	assert.NotNil(t, br)
	assert.Same(t, br, repo.bookmarkRepo)
}

func TestUserRepository_BookmarkWrappers_UseInjectedBookmarkRepo(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	bookmarkRepo := NewBookmarkRepository(nil, "test-table", zap.NewNop())
	bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _, _ string) (*models.Bookmark, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	bookmarkRepo.findTimeBookmarkFn = func(_ context.Context, _, _ string) (*models.Bookmark, error) {
		return nil, dynamormerrors.ErrItemNotFound
	}
	bookmarkRepo.transactWriteFn = func(_ context.Context, _ func(core.TransactionBuilder) error) error {
		return nil
	}
	bookmarkRepo.queryTimeBookmarksFn = func(_ context.Context, _ string, _ int, _ string) ([]models.Bookmark, string, error) {
		return []models.Bookmark{
			{ObjectID: "obj1"},
			{ObjectID: "obj2"},
		}, "next", nil
	}

	repo.SetBookmarkRepository(bookmarkRepo)

	err := repo.CreateBookmark(context.Background(), "alice", "obj1")
	assert.NoError(t, err)

	err = repo.RemoveBookmark(context.Background(), "alice", "obj1")
	assert.NoError(t, err)

	ids, cursor, err := repo.GetBookmarks(context.Background(), "alice", 1000, "")
	assert.NoError(t, err)
	assert.Equal(t, []string{"obj1", "obj2"}, ids)
	assert.Equal(t, "next", cursor)

	bookmarked, err := repo.IsBookmarked(context.Background(), "alice", "obj1")
	assert.NoError(t, err)
	assert.False(t, bookmarked)
}

func TestUserRepository_GetVouchesForActor_NoActiveFilter(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.AnythingOfType("*models.Vouch")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(nil)

	_, err := repo.GetVouchesForActor(context.Background(), "bob", false)
	assert.NoError(t, err)
}

func TestUserRepository_GetAllPreferences_ReturnsMap(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.UserPreferences")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	prefs, err := repo.GetAllPreferences(context.Background(), "alice")
	assert.NoError(t, err)
	assert.NotEmpty(t, prefs)
	assert.Equal(t, "en", prefs["language"])
}

func TestUserRepository_CacheRemoteActor_Stores(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RemoteActor")).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	err := repo.CacheRemoteActor(
		context.Background(),
		"@bob@example.com",
		&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob", Type: "Person"}},
		time.Hour,
	)
	assert.NoError(t, err)
}

func TestUserRepository_DeleteFromTimeline_NotFoundNoError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	err := repo.DeleteFromTimeline(context.Background(), "HOME", "alice", "entry")
	assert.NoError(t, err)
}

func TestUserRepository_GetTimelines_UsesCursorAndTrims(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Timeline")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Timeline)
		*dest = []*models.Timeline{
			{SK: "sk1", TTL: 0, TimelineType: "DIRECT", TimelineID: "alice", PostID: "p1"},
			{SK: "sk2", TTL: time.Now().Add(time.Hour).Unix(), TimelineType: "DIRECT", TimelineID: "alice", PostID: "p2"},
			{SK: "sk3", TTL: time.Now().Add(time.Hour).Unix(), TimelineType: "DIRECT", TimelineID: "alice", PostID: "p3"},
		}
	}).Return(nil)

	entries, next, err := repo.GetDirectTimeline(context.Background(), "alice", 2, "cursor")
	assert.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "sk2", next)

	_, _, err = repo.GetHashtagTimeline(context.Background(), "tag", true, 1, "")
	assert.NoError(t, err)

	_, _, err = repo.GetListTimeline(context.Background(), "list1", 1, "")
	assert.NoError(t, err)
}

func TestUserRepository_DeleteExpiredTimelineEntries_BatchDeletes(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Timeline)
		*dest = []*models.Timeline{
			{PK: "timeline#DIRECT#alice", SK: "sk1"},
			{PK: "timeline#DIRECT#alice", SK: "sk2"},
		}
	}).Return(nil)
	mockQuery.On("BatchDelete", mock.Anything).Return(nil)

	err := repo.DeleteExpiredTimelineEntries(context.Background(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)
}

func TestUserRepository_GetFollowRequestState_UsesDepsWhenConfigured(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	deps := new(mockUserRepositoryDeps)
	repo.SetDependencies(deps)

	deps.On("GetPendingFollowRequests", mock.Anything, "target", 100, "").Return([]string{"follower"}, "", nil).Once()

	state, err := repo.GetFollowRequestState(context.Background(), "follower", "target")
	assert.NoError(t, err)
	assert.Equal(t, "pending", state)
}

func TestUserRepository_GetFollowRequestState_UsesDBWhenNoDeps(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RelationshipRecord")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.RelationshipRecord)
		out.State = models.RelationshipAccepted
	}).Return(nil)

	state, err := repo.GetFollowRequestState(context.Background(), "alice", "bob")
	assert.NoError(t, err)
	assert.Equal(t, models.RelationshipAccepted, state)
}

func TestUserRepository_GetPendingFollowRequests_DependencyRequired(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	_, _, err := repo.GetPendingFollowRequests(context.Background(), "alice", 10, "")
	assert.Error(t, err)
}

func TestUserRepository_RemoveFromFollowers_DependencyRequired(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	err := repo.RemoveFromFollowers(context.Background(), "alice", "bob")
	assert.Error(t, err)
}

func TestUserRepository_ListUsersByRole_NotFoundReturnsEmpty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	users, err := repo.ListUsersByRole(context.Background(), "admin")
	assert.NoError(t, err)
	assert.Empty(t, users)
}

func TestUserRepository_extractActivityObject_NoteCoversNoteToMap(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	activity := &activitypub.Activity{
		Object: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   "note1",
				Type: "Note",
			},
			Content:      "hello",
			AttributedTo: "https://example.com/users/alice",
		},
	}

	obj, _, err := repo.extractActivityObject(activity, zap.NewNop())
	assert.NoError(t, err)
	assert.Equal(t, "note1", obj["id"])
	assert.Equal(t, "Note", obj["type"])
}

func TestUserRepository_URLHelpersAndValidationWrappers(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	repo.urlValidator = nil

	fields := []map[string]string{{"name": "site", "value": "http://example.tk"}}

	normalized, warnings, err := repo.ValidateAndNormalizeUserFields(context.Background(), fields)
	assert.NoError(t, err)
	assert.Equal(t, fields, normalized)
	assert.Nil(t, warnings)

	_, err = repo.ExtractProfileURLs(context.Background(), fields)
	assert.Error(t, err)

	_, err = repo.ValidateUserURL(context.Background(), "http://example.tk")
	assert.Error(t, err)

	assert.Equal(t, []string{
		"Profile URL uses insecure HTTP protocol",
		"Profile URL uses suspicious domain",
		"Profile URL is a shortened URL",
	}, generateURLWarnings([]string{"insecure_http", "suspicious_tld", "url_shortener"}))
}

func TestUserRepository_UpdateUserWithURLValidation_ValidatesAndUpdates(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	// BaseRepository.Get + Update used by UpdateUser.
	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.User)
		out.Username = "alice"
		out.Email = "alice@example.com"
	}).Return(nil)
	mockQuery.On("Update", mock.Anything).Return(nil)

	updates := map[string]any{
		"url":    "http://example.tk",
		"fields": []map[string]string{{"name": "site", "value": "http://example.tk"}},
	}

	warnings, err := repo.UpdateUserWithURLValidation(context.Background(), "alice", updates)
	assert.NoError(t, err)
	assert.NotEmpty(t, warnings)
}

func TestUserRepository_extractAccountFromReplyAndURLPathFallback(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	repo.urlValidator = nil

	assert.Equal(t, "alice", repo.extractAccountFromReply(context.Background(), "POST#alice#123"))
	assert.Equal(t, "bob", repo.extractAccountFromReply(context.Background(), "https://example.com/users/bob"))
	assert.Equal(t, "", repo.extractUsernameFromURLPath("not-a-url"))
	assert.Equal(t, "carol", repo.extractUsernameFromURLPath("https://example.com/@carol"))
	assert.Equal(t, "dave", repo.extractUsernameFromURLPath("https://example.com/actors/dave"))
	assert.Equal(t, "eve", repo.extractUsernameFromURLPath("https://example.com/users/eve"))
	assert.Equal(t, "frank", repo.extractUsernameFromURLPath("https://example.com/profile/frank"))
	assert.Equal(t, "last", repo.extractUsernameFromURLPath("https://example.com/something/last"))
}

func TestUserRepository_CostTrackersAndUtilityMethods(t *testing.T) {
	central := &centralizedCostTracker{logger: zap.NewNop()}
	central.TrackDynamoWrite(2)
	central.TrackDynamoRead(3)
	assert.Equal(t, int64(2), central.CalculateCost().DynamoDBWrites)
	assert.Equal(t, int64(3), central.CalculateCost().DynamoDBReads)

	legacy := &timelineCostTracker{logger: zap.NewNop()}
	legacy.TrackDynamoWrite(4)
	legacy.TrackDynamoRead(5)
	assert.Equal(t, int64(4), legacy.CalculateCost().DynamoDBWrites)
	assert.Equal(t, int64(5), legacy.CalculateCost().DynamoDBReads)

	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	repo.SetCostService(nil)
	assert.NoError(t, repo.TrackRead(context.Background(), "GetItem", 1))
	assert.NoError(t, repo.TrackWrite(context.Background(), "PutItem", 1))
}

func TestUserRepository_validateAndUpdateProfileURL_CoversWarnings(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	updates := map[string]any{
		"url": "http://example.tk",
	}

	warnings, err := repo.validateAndUpdateProfileURL(context.Background(), updates)
	assert.NoError(t, err)
	assert.NotEmpty(t, warnings)

	updatedURL, ok := updates["url"].(string)
	assert.True(t, ok)
	assert.Equal(t, "http://example.tk", updatedURL)
}

func TestUserRepository_validateAndUpdateProfileFields_Normalizes(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())

	updates := map[string]any{
		"fields": []map[string]string{{"name": "site", "value": "http://example.tk"}},
	}

	warnings, err := repo.validateAndUpdateProfileFields(context.Background(), updates)
	assert.NoError(t, err)
	assert.NotEmpty(t, warnings)

	fields, ok := updates["fields"].([]map[string]string)
	assert.True(t, ok)
	assert.Len(t, fields, 1)
	assert.Equal(t, "http://example.tk", fields[0]["value"])
}

func TestUserRepository_UpdateUserWithURLValidation_InvalidURLReturnsError(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	_, err := repo.UpdateUserWithURLValidation(context.Background(), "alice", map[string]any{"url": "not a url"})
	assert.Error(t, err)
}

func TestUserRepository_GetFollowRequestState_DependencyErrorReturnsNone(t *testing.T) {
	repo := NewUserRepository(nil, "test-table", zap.NewNop())
	deps := new(mockUserRepositoryDeps)
	repo.SetDependencies(deps)

	deps.On("GetPendingFollowRequests", mock.Anything, "target", 100, "").Return([]string{}, "", fmt.Errorf("boom")).Once()

	state, err := repo.GetFollowRequestState(context.Background(), "follower", "target")
	assert.NoError(t, err)
	assert.Equal(t, "none", state)
}

func TestUserRepository_DeleteFromTimeline_SuccessAndErrors(t *testing.T) {
	ctx := context.Background()

	// Success path.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*models.Timeline)
			out.PK = "timeline#HOME#alice"
			out.SK = "sk1"
		}).Return(nil)
		mockQuery.On("Delete").Return(nil)

		err := repo.DeleteFromTimeline(ctx, "HOME", "alice", "entry")
		assert.NoError(t, err)
	}

	// Delete failure propagates.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			out := args.Get(0).(*models.Timeline)
			out.PK = "timeline#HOME#alice"
			out.SK = "sk1"
		}).Return(nil)
		mockQuery.On("Delete").Return(ErrTestMockError)

		err := repo.DeleteFromTimeline(ctx, "HOME", "alice", "entry")
		assert.Error(t, err)
	}

	// Non-not-found lookup error propagates.
	{
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

		mockDB.On("WithContext", mock.Anything).Return(mockDB)
		mockDB.On("Model", mock.Anything).Return(mockQuery)
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
		mockQuery.On("First", mock.Anything).Return(ErrTestMockError)

		err := repo.DeleteFromTimeline(ctx, "HOME", "alice", "entry")
		assert.Error(t, err)
	}
}

func TestUserRepository_ListUsersByRole_SuccessConvertsUsers(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.User")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = []models.User{
			{Username: "alice", Email: "alice@example.com", Role: "admin"},
			{Username: "bob", Email: "bob@example.com", Role: "admin"},
		}
	}).Return(nil)

	users, err := repo.ListUsersByRole(context.Background(), "admin")
	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "alice", users[0].Username)
	assert.Equal(t, "bob", users[1].Username)
}

func TestUserRepository_DeleteExpiredTimelineEntries_TTLNoops(t *testing.T) {
	ctx := context.Background()

	repo := NewUserRepository(new(mocks.MockDB), "test-table", zap.NewNop())
	err := repo.DeleteExpiredTimelineEntries(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
}
