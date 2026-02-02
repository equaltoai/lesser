package repositories

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type usernameRelationshipRepo struct {
	usernames []string
	err       error
}

func (r usernameRelationshipRepo) GetFollowing(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return r.usernames, "", r.err
}

type fullRelationshipRepo struct {
	following *interfaces.PaginatedResult[*storage.Account]
	err       error
}

func (r fullRelationshipRepo) CreateFollowRequest(context.Context, string, string) error { return nil }
func (r fullRelationshipRepo) AcceptFollowRequest(context.Context, string, string) error { return nil }
func (r fullRelationshipRepo) RejectFollowRequest(context.Context, string, string) error { return nil }
func (r fullRelationshipRepo) Unfollow(context.Context, string, string) error            { return nil }

func (r fullRelationshipRepo) IsFollowing(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r fullRelationshipRepo) GetFollowStatus(context.Context, string, string) (string, error) {
	return "", nil
}
func (r fullRelationshipRepo) GetFollowRelationship(context.Context, string, string) (*models.RelationshipRecord, error) {
	return nil, nil
}

func (r fullRelationshipRepo) GetFollowers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}
func (r fullRelationshipRepo) GetFollowing(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	if r.following != nil {
		return r.following, r.err
	}
	return &interfaces.PaginatedResult[*storage.Account]{}, r.err
}
func (r fullRelationshipRepo) GetFollowRequests(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}
func (r fullRelationshipRepo) GetPendingFollowRequests(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}
func (r fullRelationshipRepo) GetMutualFollows(context.Context, string, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}

func (r fullRelationshipRepo) BlockUser(context.Context, string, string) error   { return nil }
func (r fullRelationshipRepo) UnblockUser(context.Context, string, string) error { return nil }
func (r fullRelationshipRepo) IsBlocked(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r fullRelationshipRepo) GetBlockedUsers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}

func (r fullRelationshipRepo) MuteUser(context.Context, string, string) error   { return nil }
func (r fullRelationshipRepo) UnmuteUser(context.Context, string, string) error { return nil }
func (r fullRelationshipRepo) IsMuted(context.Context, string, string) (bool, error) {
	return false, nil
}
func (r fullRelationshipRepo) GetMutedUsers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}

func (r fullRelationshipRepo) GetFollowerCount(context.Context, string) (int64, error) { return 0, nil }
func (r fullRelationshipRepo) GetFollowingCount(context.Context, string) (int64, error) {
	return 0, nil
}
func (r fullRelationshipRepo) GetMutualFollowCount(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (r fullRelationshipRepo) GetRelationships(context.Context, string, []string) (map[string]*models.RelationshipRecord, error) {
	return map[string]*models.RelationshipRecord{}, nil
}

type noopTransactionBuilder struct{}

func (noopTransactionBuilder) Put(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Create(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Update(_ any, _ []string, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) UpdateWithBuilder(_ any, _ func(core.UpdateBuilder) error, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Delete(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) ConditionCheck(_ any, _ ...core.TransactCondition) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) WithContext(_ context.Context) core.TransactionBuilder {
	return noopTransactionBuilder{}
}
func (noopTransactionBuilder) Execute() error                             { return nil }
func (noopTransactionBuilder) ExecuteWithContext(_ context.Context) error { return nil }

func setupPermissiveStatusRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("Select", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateStatusRepositorySliceForCoverage(args.Get(0))
	}).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateStatusRepositorySliceForCoverage(args.Get(0))
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateStatusRepositoryStructForCoverage(args.Get(0))
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()

	if mockUpdateBuilder != nil {
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Execute").Return(nil).Maybe()
	}
}

func populateStatusRepositorySliceForCoverage(target any) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	sliceValue := value.Elem()
	elemType := sliceValue.Type().Elem()
	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}

	// Avoid interface slices to prevent type assertion pitfalls.
	if baseElemType.Kind() == reflect.Interface {
		return
	}

	count := 2
	for i := range count {
		now := time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC)

		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateStatusRepositoryStructForCoverageWithTime(element.Interface(), now)
		} else {
			elemPtr := reflect.New(baseElemType)
			populateStatusRepositoryStructForCoverageWithTime(elemPtr.Interface(), now)
			element = elemPtr.Elem()
		}

		sliceValue = reflect.Append(sliceValue, element)
	}

	value.Elem().Set(sliceValue)
}

func populateStatusRepositoryStructForCoverage(target any) {
	populateStatusRepositoryStructForCoverageWithTime(target, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
}

func populateStatusRepositoryStructForCoverageWithTime(target any, publishedAt time.Time) {
	switch model := target.(type) {
	case *models.Status:
		model.StatusID = "status-1"
		model.AuthorID = "https://localhost/users/alice"
		model.AuthorUsername = "alice"
		model.Visibility = models.VisibilityPublic
		model.PublishedAt = publishedAt
		model.Content = "hello"
		_ = model.UpdateKeys()

	case *models.StatusEngagement:
		model.PK = "STATUS_ENGAGEMENT#status-1"
		model.SK = fmt.Sprintf("like#%d#user-1", publishedAt.UnixNano())
		model.StatusID = "status-1"
		model.UserID = "user-1"
		model.EngagementType = "like"
		model.EngagedAt = publishedAt
	}
}

func TestStatusRepository_create_update_delete_and_counts(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	assert.Error(t, repo.CreateBoostStatus(ctx, nil))
	assert.Error(t, repo.CreateBoostStatus(ctx, &models.Status{StatusID: "s1", AuthorID: "a1"}))
	assert.Error(t, repo.CreateBoostStatus(ctx, &models.Status{StatusID: "s1", BoostOfStatusID: "t1"}))

	status := &models.Status{
		StatusID:        "boost1",
		AuthorID:        "actor1",
		AuthorUsername:  "alice",
		BoostOfStatusID: "target1",
		Visibility:      models.VisibilityPublic,
		Hashtags:        []string{"#Go", " ", "#"},
		Language:        "en",
	}
	status.Note = &activitypub.Note{BaseObject: activitypub.BaseObject{Type: "Note", ID: "https://example.com/status/boost1"}}

	assert.NoError(t, repo.CreateBoostStatus(ctx, status))

	status.Content = "updated"
	status.DeletedAt = ptr(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	status.ToRecipients = []string{"https://example.com/users/alice"}
	status.CcRecipients = []string{"https://example.com/users/bob"}
	status.QuoteTargetStatusID = "quote1"
	status.QuoteTargetAuthorID = "author1"
	assert.NoError(t, repo.UpdateStatus(ctx, status))

	_, _ = repo.CountStatusesByAuthor(ctx, "actor1")
	_, _ = repo.CountReplies(ctx, "root1")
	assert.NoError(t, repo.UpdateEngagementMetrics(ctx, "boost1", 1, 2, 3, 4))
	_, _ = repo.GetTotalStatusCount(ctx)
}

func TestStatusRepository_boost_deletion_not_found_and_found(t *testing.T) {
	ctx := context.Background()

	t.Run("not_found_is_nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		// findBoostStatus -> All returns empty -> storage.ErrNotFound
		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		status, err := repo.DeleteBoostStatus(ctx, "booster1", "target1")
		assert.NoError(t, err)
		assert.Nil(t, status)
	})

	t.Run("found_deletes", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		out, err := repo.DeleteBoostStatus(ctx, "booster1", "target1")
		assert.NoError(t, err)
		assert.NotNil(t, out)
		assert.NotEmpty(t, out.StatusID)
	})
}

func TestStatusRepository_url_queries(t *testing.T) {
	ctx := context.Background()

	t.Run("GetStatusesByURL_filters_and_sorts", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		targetURL := "https://example.com/x"
		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "s1", URLs: []string{targetURL}, PublishedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
				{StatusID: "s2", URLs: []string{"https://example.com/other"}, PublishedAt: time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		out, err := repo.GetStatusesByURL(ctx, targetURL, -1)
		assert.NoError(t, err)
		assert.Len(t, out, 1)
		assert.Equal(t, "s1", out[0].StatusID)
	})

	t.Run("GetStatusByURL_matches_note_or_urls", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		url := "https://example.com/status/1"
		mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{
					StatusID: "note-match",
					Note:     &activitypub.Note{BaseObject: activitypub.BaseObject{Type: "Note", ID: url}},
				},
				{
					StatusID: "url-match",
					URLs:     []string{url},
				},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		found, err := repo.GetStatusByURL(ctx, url)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, "note-match", found.StatusID)
	})
}

func TestStatusRepository_home_timeline_branches(t *testing.T) {
	ctx := context.Background()

	t.Run("relationship_repo_nil_falls_back_public", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "p1", PublishedAt: time.Now()},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		page, err := repo.GetHomeTimeline(ctx, "user1", interfaces.PaginationOptions{Limit: 1})
		assert.NoError(t, err)
		assert.Len(t, page.Items, 1)
	})

	t.Run("relationship_repo_usernames_and_pagination", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		call := 0
		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			call++
			*dest = []models.Status{
				{StatusID: fmt.Sprintf("s%d", call), PublishedAt: time.Date(2025, 1, call, 0, 0, 0, 0, time.UTC)},
				{StatusID: fmt.Sprintf("s%db", call), PublishedAt: time.Date(2025, 1, call, 0, 0, 0, 0, time.UTC).Add(-time.Minute)},
			}
		}).Return(nil)

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetRelationshipRepository(usernameRelationshipRepo{usernames: []string{"alice", "", "bob"}})

		page, err := repo.GetHomeTimeline(ctx, "user1", interfaces.PaginationOptions{Limit: 1})
		assert.NoError(t, err)
		assert.Len(t, page.Items, 1)
		assert.True(t, page.HasMore)
		assert.NotEmpty(t, page.NextCursor)
	})

	t.Run("relationship_repo_empty_following_returns_empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetRelationshipRepository(usernameRelationshipRepo{usernames: []string{}})

		page, err := repo.GetHomeTimeline(ctx, "user1", interfaces.PaginationOptions{Limit: 10})
		assert.NoError(t, err)
		assert.Empty(t, page.Items)
		assert.False(t, page.HasMore)
	})
}

func TestStatusRepository_filters_and_counts(t *testing.T) {
	ctx := context.Background()

	origDomain := config.Get().Domain
	defer func() { config.Get().Domain = origDomain }()

	t.Run("remote_filtering_with_empty_domain_scans", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		config.Get().Domain = ""

		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{{StatusID: "s1"}}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)
		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		out, err := repo.applyRemoteFiltering(ctx, mockQuery, &interfaces.StatusFilter{Remote: boolPtr(true)}, 2)
		assert.NoError(t, err)
		assert.Len(t, out, 1)
	})

	t.Run("remote_filtering_with_domain_postprocesses", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		config.Get().Domain = DefaultDomain

		mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "local", AuthorID: "https://localhost/users/a"},
				{StatusID: "remote", AuthorID: "https://remote.example/users/b"},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)
		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		out, err := repo.applyRemoteFiltering(ctx, mockQuery, &interfaces.StatusFilter{Remote: boolPtr(true)}, 1)
		assert.NoError(t, err)
		assert.Len(t, out, 1)
		assert.Equal(t, "remote", out[0].StatusID)
	})

	t.Run("count_remote_statuses", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		config.Get().Domain = DefaultDomain

		filter := &interfaces.StatusFilter{Remote: boolPtr(true)}
		mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{AuthorID: "https://localhost/users/a"},
				{AuthorID: "https://remote.example/users/b"},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)
		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		count, err := repo.CountStatusesForAdmin(ctx, filter)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})
}

func TestStatusRepository_engagements_bookmarks_and_helpers(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

	bookmarkRepo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
	bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
		return nil, dynamormErrors.ErrItemNotFound
	}
	bookmarkRepo.findTimeBookmarkFn = func(_ context.Context, username, objectID string) (*models.Bookmark, error) {
		now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		bm, _ := models.NewTimeOrderedBookmark(username, objectID, now)
		bm.Locked = false
		return bm, nil
	}
	bookmarkRepo.transactWriteFn = func(_ context.Context, fn func(core.TransactionBuilder) error) error {
		_ = fn(noopTransactionBuilder{})
		return nil
	}
	repo.SetBookmarkRepository(bookmarkRepo)

	assert.NoError(t, repo.LikeStatus(ctx, "user1", "s1"))

	mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.StatusEngagement)
		*dest = []models.StatusEngagement{
			{PK: "STATUS_ENGAGEMENT#s1", SK: "like#1#user1"},
		}
	}).Return(nil).Once()
	assert.NoError(t, repo.UnlikeStatus(ctx, "user1", "s1"))

	assert.NoError(t, repo.BookmarkStatus(ctx, "user1", "s1"))

	bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, username, objectID string) (*models.Bookmark, error) {
		now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		timeRecord, _ := models.NewTimeOrderedBookmark(username, objectID, now)
		objectRecord, _ := models.NewObjectIndexedBookmark(username, objectID, timeRecord.CreatedAt, timeRecord.SK)
		return objectRecord, nil
	}
	assert.NoError(t, repo.UnbookmarkStatus(ctx, "user1", "s1"))

	liked, reblogged, bookmarked, err := repo.GetStatusEngagement(ctx, "s1", "user1")
	assert.NoError(t, err)
	_ = liked
	_ = reblogged
	assert.True(t, bookmarked)

	// Small helper coverage: validate canonical hashtag errors
	_, err = repo.GetStatusesByHashtag(ctx, "", interfaces.PaginationOptions{Limit: 10})
	assert.Error(t, err)
	_, err = repo.GetStatusesByHashtag(ctx, "#Test", interfaces.PaginationOptions{Limit: 10})
	assert.Error(t, err)
	_, err = repo.GetStatusesByHashtag(ctx, "Test", interfaces.PaginationOptions{Limit: 10})
	assert.Error(t, err)

	// IDs helper path: empty/duplicate are ignored -> BatchGet short-circuits.
	out, err := repo.GetStatusesByIDs(ctx, []string{"", ""})
	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestStatusRepository_query_helpers_and_flags(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

	_, _ = repo.GetUserTimeline(ctx, "user1", interfaces.PaginationOptions{Limit: 2, Cursor: "c1"})
	_, _ = repo.GetConversationThread(ctx, "conv1", interfaces.PaginationOptions{Limit: 2, Cursor: "c1"})
	_, _ = repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 2})
	_, _ = repo.GetTrendingStatuses(ctx, interfaces.PaginationOptions{Limit: 2})
	_, _ = repo.GetFlaggedStatuses(ctx, interfaces.PaginationOptions{Limit: 2})

	assert.NoError(t, repo.FlagStatus(ctx, "s1", "reason", "by"))
	assert.NoError(t, repo.UnflagStatus(ctx, "s1"))

	// Public timeline wrapper uses queryPublicTimelineDirect.
	_, _ = repo.GetPublicTimeline(ctx, interfaces.PaginationOptions{Limit: 2})

	// Replies query wrapper.
	_, _ = repo.GetReplies(ctx, "parent1", interfaces.PaginationOptions{Limit: 2})

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestStatusRepository_admin_filtering_and_cursor(t *testing.T) {
	ctx := context.Background()

	origDomain := config.Get().Domain
	config.Get().Domain = DefaultDomain
	defer func() { config.Get().Domain = origDomain }()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

	out, cursor, err := repo.ListStatusesForAdmin(ctx, &interfaces.StatusFilter{}, 2, "")
	assert.NoError(t, err)
	assert.Len(t, out, 2)
	assert.NotEmpty(t, cursor)

	out, _, err = repo.ListStatusesForAdmin(ctx, &interfaces.StatusFilter{
		Local:      boolPtr(true),
		Visibility: models.VisibilityPublic,
		WithMedia:  boolPtr(false),
		Sensitive:  boolPtr(false),
	}, 2, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, out)

	minDate := time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
	maxDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	out, _, err = repo.ListStatusesForAdmin(ctx, &interfaces.StatusFilter{
		ByDomain:  "remote.example",
		MinDate:   &minDate,
		MaxDate:   &maxDate,
		Flagged:   boolPtr(true),
		WithMedia: boolPtr(true),
	}, 2, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, out)

	_, err = repo.CountStatusesForAdmin(ctx, &interfaces.StatusFilter{Local: boolPtr(true)})
	assert.NoError(t, err)
	_, err = repo.CountStatusesForAdmin(ctx, &interfaces.StatusFilter{Visibility: models.VisibilityPublic})
	assert.NoError(t, err)
}

func TestStatusRepository_hashtag_queries(t *testing.T) {
	ctx := context.Background()

	t.Run("not_found_returns_empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		page, err := repo.GetStatusesByHashtag(ctx, "go", interfaces.PaginationOptions{Limit: 10})
		assert.NoError(t, err)
		assert.Empty(t, page.Items)
	})

	t.Run("success", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		page, err := repo.GetStatusesByHashtag(ctx, "go", interfaces.PaginationOptions{Limit: 1})
		assert.NoError(t, err)
		assert.NotEmpty(t, page.Items)
	})
}

func TestStatusRepository_engagement_edge_cases(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.StatusEngagement)
		*dest = []models.StatusEngagement{}
	}).Return(nil).Once()

	setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	assert.NoError(t, repo.UnlikeStatus(ctx, "user1", "status-1"))
}

func TestStatusRepository_canonicalize_and_hashtag_index_branches(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	// createHashtagTimelineIndexes creates two records; first is condition-failed and ignored.
	mockQuery.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
	mockQuery.On("Create").Return(nil).Maybe()

	setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	status := &models.Status{
		StatusID:       "s1",
		AuthorID:       "https://localhost/users/alice",
		AuthorUsername: "alice",
		Visibility:     models.VisibilityPublic,
		PublishedAt:    time.Time{}, // force timestamp fallback
		Hashtags:       []string{"Go", "Rust"},
	}
	status.Note = &activitypub.Note{BaseObject: activitypub.BaseObject{Type: "Note", ID: "https://example.com/status/1"}}

	assert.NoError(t, repo.createHashtagTimelineIndexes(ctx, status))
	assert.NoError(t, repo.canonicalizeStatusIndexes(ctx, status))

	status.Visibility = models.VisibilityPrivate
	assert.NoError(t, repo.canonicalizeStatusIndexes(ctx, status))
}

func TestStatusRepository_home_timeline_relationship_repo_interface(t *testing.T) {
	ctx := context.Background()

	origDomain := config.Get().Domain
	config.Get().Domain = "example.com"
	defer func() { config.Get().Domain = origDomain }()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

	repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetRelationshipRepository(fullRelationshipRepo{
		following: &interfaces.PaginatedResult[*storage.Account]{
			Items: []*storage.Account{
				{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}},
				{User: &storage.User{Username: "bob"}},
			},
		},
	})

	page, err := repo.GetHomeTimeline(ctx, "user1", interfaces.PaginationOptions{Limit: 1})
	assert.NoError(t, err)
	assert.NotEmpty(t, page.Items)
}

func TestStatusRepository_missing_wrappers_and_context_counts(t *testing.T) {
	ctx := context.Background()

	t.Run("ReblogStatus_and_UnreblogStatus_hit_wrappers", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		assert.NoError(t, repo.ReblogStatus(ctx, "user1", "status-1", ""))
		assert.NoError(t, repo.UnreblogStatus(ctx, "user1", "status-1"))
	})

	t.Run("GetStatusCounts_and_GetStatusContext", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		firstCall := 0
		mockQuery.On("First", mock.AnythingOfType("*models.Status")).Run(func(args mock.Arguments) {
			firstCall++
			status := args.Get(0).(*models.Status)
			switch firstCall {
			case 1:
				*status = models.Status{StatusID: "child", InReplyToID: "parent1"}
			case 2:
				*status = models.Status{StatusID: "parent1", InReplyToID: "root"}
			default:
				*status = models.Status{StatusID: "root"}
			}
			_ = status.UpdateKeys()
		}).Return(nil).Maybe()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		likes, reblogs, replies, err := repo.GetStatusCounts(ctx, "child")
		assert.NoError(t, err)
		assert.Equal(t, 0, likes)
		assert.Equal(t, 0, reblogs)
		assert.Equal(t, 0, replies)

		firstCall = 0
		ancestors, descendants, err := repo.GetStatusContext(ctx, "child")
		assert.NoError(t, err)
		assert.Len(t, ancestors, 2)
		assert.NotNil(t, descendants)
	})
}

func TestStatusRepository_queryStatusesByGSI_cursor_and_next_cursor(t *testing.T) {
	ctx := context.Background()

	t.Run("DESC_uses_lt_and_gsi1_next_cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Where", "gsi1SK", "<", "cursor-val").Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "s1", GSI1SK: "c1"},
				{StatusID: "s2", GSI1SK: "c2"},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		page, err := repo.GetUserTimeline(ctx, "alice", interfaces.PaginationOptions{Limit: 1, Cursor: "cursor-val"})
		assert.NoError(t, err)
		assert.True(t, page.HasMore)
		assert.Equal(t, "c1", page.NextCursor)
		assert.Len(t, page.Items, 1)
	})

	t.Run("ASC_uses_gt_and_gsi3_next_cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Where", "gsi3SK", ">", "cursor-val").Return(mockQuery).Once()
		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "s1", GSI3SK: "c1"},
				{StatusID: "s2", GSI3SK: "c2"},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		page, err := repo.GetConversationThread(ctx, "conv1", interfaces.PaginationOptions{Limit: 1, Cursor: "cursor-val"})
		assert.NoError(t, err)
		assert.True(t, page.HasMore)
		assert.Equal(t, "c1", page.NextCursor)
		assert.Len(t, page.Items, 1)
	})
}

func TestStatusRepository_public_timeline_and_batch_get_ids(t *testing.T) {
	ctx := context.Background()

	t.Run("GetPublicTimeline_error_path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetPublicTimeline(ctx, interfaces.PaginationOptions{Limit: 2})
		assert.Error(t, err)
	})

	t.Run("GetPublicTimeline_has_more_when_len_equals_limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "s1"},
				{StatusID: "s2"},
			}
		}).Return(nil).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		page, err := repo.GetPublicTimeline(ctx, interfaces.PaginationOptions{Limit: 2})
		assert.NoError(t, err)
		assert.True(t, page.HasMore)
	})

	t.Run("GetStatusesByIDs_dedupes_and_orders", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			dest := args.Get(1).(*[]*models.Status)
			*dest = []*models.Status{
				nil,
				{StatusID: "b"},
				{StatusID: "a"},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		out, err := repo.GetStatusesByIDs(ctx, []string{"a", "", "b", "a"})
		assert.NoError(t, err)
		if assert.Len(t, out, 3) {
			assert.Equal(t, "a", out[0].StatusID)
			assert.Equal(t, "b", out[1].StatusID)
			assert.Equal(t, "a", out[2].StatusID)
		}

		out, err = repo.GetStatusesByIDs(ctx, nil)
		assert.NoError(t, err)
		assert.Empty(t, out)
	})
}

func TestStatusRepository_more_error_branches_and_helpers(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateStatus_nil_payload_errors", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		assert.Panics(t, func() {
			_ = repo.CreateStatus(ctx, nil)
		})
	})

	t.Run("CreateStatus_warns_on_index_failures", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		// First Create is the status record; second Create is the hashtag index record.
		mockQuery.On("Create").Return(nil).Once()
		mockQuery.On("Create").Return(assert.AnError).Once()

		// canonicalizeStatusIndexes builder.Execute failure should be logged and ignored.
		mockUpdateBuilder.On("Execute").Return(assert.AnError).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		repo.SetValidationService(nil)
		repo.SetPermissionService(nil)
		repo.SetCachingService(nil)
		repo.SetEventService(nil)

		status := &models.Status{
			StatusID:       "s1",
			AuthorID:       "https://localhost/users/alice",
			AuthorUsername: "alice",
			Visibility:     models.VisibilityPublic,
			Hashtags:       []string{"Go"},
			PublishedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		}
		status.Note = &activitypub.Note{BaseObject: activitypub.BaseObject{Type: "Note", ID: "https://example.com/status/1"}}

		assert.NoError(t, repo.CreateStatus(ctx, status))
	})

	t.Run("GetStatusByURL_hits_url_fallback_and_not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		url := "https://example.com/status/1"

		mockQuery.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "url-match", URLs: []string{url}},
			}
		}).Return(nil).Once()

		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)
		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		found, err := repo.GetStatusByURL(ctx, url)
		assert.NoError(t, err)
		assert.Equal(t, "url-match", found.StatusID)

		mockDB2 := new(mocks.MockDB)
		mockQuery2 := new(mocks.MockQuery)
		mockQuery2.On("Scan", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.Status)
			*dest = []models.Status{
				{StatusID: "nope", URLs: []string{"https://example.com/other"}},
			}
		}).Return(nil).Once()
		setupPermissiveStatusRepoMocks(mockDB2, mockQuery2, nil)

		repo2 := NewStatusRepository(mockDB2, "test-table", zap.NewNop(), nil)
		_, err = repo2.GetStatusByURL(ctx, url)
		assert.Error(t, err)
	})

	t.Run("fetchFollowingActorIDs_and_fetchStatusesForActor_error_path", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		origDomain := config.Get().Domain
		config.Get().Domain = "example.com"
		defer func() { config.Get().Domain = origDomain }()

		repo.SetRelationshipRepository(fullRelationshipRepo{
			following: &interfaces.PaginatedResult[*storage.Account]{
				Items: []*storage.Account{
					nil,
					{},
					{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}},
					{User: &storage.User{Username: ""}},
					{User: &storage.User{Username: "bob"}},
				},
			},
		})

		actorIDs, err := repo.fetchFollowingActorIDs(ctx, "user1")
		assert.NoError(t, err)
		assert.Equal(t, []string{"https://example.com/users/alice", "https://example.com/users/bob"}, actorIDs)

		out := repo.fetchStatusesForActor(ctx, "user1", "https://example.com/users/alice")
		assert.Nil(t, out)
	})

	t.Run("DeleteStatus_update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockUpdateBuilder.On("Execute").Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		assert.Error(t, repo.DeleteStatus(ctx, "status-1"))
	})
}

func TestStatusRepository_more_error_paths(t *testing.T) {
	ctx := context.Background()

	t.Run("GetTotalStatusCount_count_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetTotalStatusCount(ctx)
		assert.Error(t, err)
	})

	t.Run("removeEngagement_query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.StatusEngagement")).Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		assert.Error(t, repo.UnreblogStatus(ctx, "user1", "status-1"))
	})

	t.Run("DeleteBoostStatus_propagates_query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.DeleteBoostStatus(ctx, "booster1", "target1")
		assert.Error(t, err)
	})

	t.Run("getBookmarkRepository_returns_existing", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		existing := &BookmarkRepository{}
		repo.SetBookmarkRepository(existing)
		assert.Same(t, existing, repo.getBookmarkRepository())
	})
}

func TestStatusRepository_more_branch_errors_for_counts_and_bookmarks(t *testing.T) {
	ctx := context.Background()

	t.Run("CountStatusesByAuthor_count_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.CountStatusesByAuthor(ctx, "author1")
		assert.Error(t, err)
	})

	t.Run("CountReplies_count_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Count").Return(int64(0), assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.CountReplies(ctx, "status-1")
		assert.Error(t, err)
	})

	t.Run("GetStatusCounts_not_found", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("First", mock.AnythingOfType("*models.Status")).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, _, _, err := repo.GetStatusCounts(ctx, "missing")
		assert.Error(t, err)
	})

	t.Run("GetStatusContext_replies_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, _, err := repo.GetStatusContext(ctx, "status-1")
		assert.Error(t, err)
	})

	t.Run("BookmarkStatus_and_UnbookmarkStatus_error_paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)

		bookmarkRepo := NewBookmarkRepository(mockDB, "test-table", zap.NewNop())
		bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
			return nil, dynamormErrors.ErrItemNotFound
		}
		bookmarkRepo.findTimeBookmarkFn = func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
			return nil, dynamormErrors.ErrItemNotFound
		}
		bookmarkRepo.transactWriteFn = func(_ context.Context, _ func(core.TransactionBuilder) error) error {
			return assert.AnError
		}
		repo.SetBookmarkRepository(bookmarkRepo)

		assert.Error(t, repo.BookmarkStatus(ctx, "user1", "s1"))

		now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		timeRecord, _ := models.NewTimeOrderedBookmark("user1", "s1", now)
		objectRecord, _ := models.NewObjectIndexedBookmark("user1", "s1", timeRecord.CreatedAt, timeRecord.SK)
		bookmarkRepo.getObjectBookmarkFn = func(_ context.Context, _ string, _ string) (*models.Bookmark, error) {
			return objectRecord, nil
		}
		assert.Error(t, repo.UnbookmarkStatus(ctx, "user1", "s1"))
	})

	t.Run("DeleteBoostStatus_invalid_params", func(t *testing.T) {
		repo := NewStatusRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
		_, err := repo.DeleteBoostStatus(ctx, "", "target")
		assert.Error(t, err)
	})
}

func TestStatusRepository_more_error_paths_for_queries_and_engagements(t *testing.T) {
	ctx := context.Background()

	t.Run("GetStatusesByURL_query_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		_, err := repo.GetStatusesByURL(ctx, "https://example.com/x", 5)
		assert.Error(t, err)
	})

	t.Run("createEngagementAndIncrement_create_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.On("Create").Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		assert.Error(t, repo.ReblogStatus(ctx, "user1", "status-1", ""))
	})

	t.Run("createEngagementAndIncrement_update_error", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockUpdateBuilder := new(mocks.MockUpdateBuilder)

		mockQuery.On("Create").Return(nil).Once()
		mockUpdateBuilder.On("Execute").Return(assert.AnError).Once()
		setupPermissiveStatusRepoMocks(mockDB, mockQuery, mockUpdateBuilder)

		repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
		assert.Error(t, repo.ReblogStatus(ctx, "user1", "status-1", ""))
	})
}

func TestStatusRepository_queryStatusesByGSI_switch_cases(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name       string
		gsiSKField string
		expect     string
	}{
		{name: "gsi2", gsiSKField: gsi2SKField, expect: "gsi2-c1"},
		{name: "gsi4", gsiSKField: "gsi4SK", expect: "gsi4-c1"},
		{name: "gsi5", gsiSKField: "gsi5SK", expect: "gsi5-c1"},
		{name: "gsi6", gsiSKField: "gsi6SK", expect: "gsi6-c1"},
		{name: "gsi7", gsiSKField: "gsi7SK", expect: "gsi7-c1"},
		{name: "default", gsiSKField: "customSK", expect: "sk-c1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)

			mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
				dest := args.Get(0).(*[]models.Status)
				*dest = []models.Status{
					{
						StatusID: "s1",
						SK:       "sk-c1",
						GSI2SK:   "gsi2-c1",
						GSI4SK:   "gsi4-c1",
						GSI5SK:   "gsi5-c1",
						GSI6SK:   "gsi6-c1",
						GSI7SK:   "gsi7-c1",
					},
					{
						StatusID: "s2",
						SK:       "sk-c2",
						GSI2SK:   "gsi2-c2",
						GSI4SK:   "gsi4-c2",
						GSI5SK:   "gsi5-c2",
						GSI6SK:   "gsi6-c2",
						GSI7SK:   "gsi7-c2",
					},
				}
			}).Return(nil).Once()

			setupPermissiveStatusRepoMocks(mockDB, mockQuery, nil)

			repo := NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
			page, err := repo.queryStatusesByGSI(ctx, "gsi2", "gsi2PK", "PUBLIC_TIMELINE", tc.gsiSKField, "DESC", interfaces.PaginationOptions{Limit: 1}, "switch test")
			assert.NoError(t, err)
			assert.True(t, page.HasMore)
			assert.Equal(t, tc.expect, page.NextCursor)
		})
	}
}

func ptr[T any](v T) *T { return &v }
