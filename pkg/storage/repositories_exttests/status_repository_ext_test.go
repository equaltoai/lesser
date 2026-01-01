package repositories_exttests

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
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type transactionalMockDB struct {
	*mocks.MockDB
}

func (db transactionalMockDB) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	_ = fn(noopTransactionBuilder{})
	return nil
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

type usernameRelationshipRepo struct {
	usernames []string
	err       error
}

func (r usernameRelationshipRepo) GetFollowing(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return r.usernames, "", r.err
}

type fakeRelationshipRepo struct {
	following *interfaces.PaginatedResult[*storage.Account]
	err       error
}

func (fakeRelationshipRepo) CreateFollowRequest(context.Context, string, string) error { return nil }
func (fakeRelationshipRepo) AcceptFollowRequest(context.Context, string, string) error { return nil }
func (fakeRelationshipRepo) RejectFollowRequest(context.Context, string, string) error { return nil }
func (fakeRelationshipRepo) Unfollow(context.Context, string, string) error            { return nil }

func (fakeRelationshipRepo) IsFollowing(context.Context, string, string) (bool, error) {
	return false, nil
}
func (fakeRelationshipRepo) GetFollowStatus(context.Context, string, string) (string, error) {
	return "", nil
}
func (fakeRelationshipRepo) GetFollowRelationship(context.Context, string, string) (*models.RelationshipRecord, error) {
	return nil, nil
}

func (fakeRelationshipRepo) GetFollowers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}
func (r fakeRelationshipRepo) GetFollowing(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	if r.following == nil {
		return &interfaces.PaginatedResult[*storage.Account]{}, r.err
	}
	return r.following, r.err
}
func (fakeRelationshipRepo) GetFollowRequests(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}
func (fakeRelationshipRepo) GetPendingFollowRequests(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}
func (fakeRelationshipRepo) GetMutualFollows(context.Context, string, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}

func (fakeRelationshipRepo) BlockUser(context.Context, string, string) error   { return nil }
func (fakeRelationshipRepo) UnblockUser(context.Context, string, string) error { return nil }
func (fakeRelationshipRepo) IsBlocked(context.Context, string, string) (bool, error) {
	return false, nil
}
func (fakeRelationshipRepo) GetBlockedUsers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}

func (fakeRelationshipRepo) MuteUser(context.Context, string, string) error   { return nil }
func (fakeRelationshipRepo) UnmuteUser(context.Context, string, string) error { return nil }
func (fakeRelationshipRepo) IsMuted(context.Context, string, string) (bool, error) {
	return false, nil
}
func (fakeRelationshipRepo) GetMutedUsers(context.Context, string, interfaces.PaginationOptions) (*interfaces.PaginatedResult[*storage.Account], error) {
	return &interfaces.PaginatedResult[*storage.Account]{}, nil
}

func (fakeRelationshipRepo) GetFollowerCount(context.Context, string) (int64, error)  { return 0, nil }
func (fakeRelationshipRepo) GetFollowingCount(context.Context, string) (int64, error) { return 0, nil }
func (fakeRelationshipRepo) GetMutualFollowCount(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (fakeRelationshipRepo) GetRelationships(context.Context, string, []string) (map[string]*models.RelationshipRecord, error) {
	return map[string]*models.RelationshipRecord{}, nil
}

// --- permissive mocks ---

func setupPermissiveStatusMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, mockUpdateBuilder *mocks.MockUpdateBuilder) {
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

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0))
	}).Return(nil).Maybe()
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(0))
	}).Return(nil).Maybe()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateStruct(args.Get(0))
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		populateSlice(args.Get(1))
	}).Return(nil).Maybe()

	if mockUpdateBuilder != nil {
		mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Add", mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder).Maybe()
		mockUpdateBuilder.On("Execute").Return(nil).Maybe()
	}
}

func populateSlice(target any) {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
		return
	}

	sliceValue := v.Elem()
	elemType := sliceValue.Type().Elem()
	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}
	if baseElemType.Kind() == reflect.Interface {
		return
	}

	for i := range 2 {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateStructWithTime(element.Interface(), time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC))
		} else {
			ptr := reflect.New(baseElemType)
			populateStructWithTime(ptr.Interface(), time.Date(2025, 1, i+1, 0, 0, 0, 0, time.UTC))
			element = ptr.Elem()
		}
		sliceValue = reflect.Append(sliceValue, element)
	}

	v.Elem().Set(sliceValue)
}

func populateStruct(target any) {
	populateStructWithTime(target, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
}

func populateStructWithTime(target any, publishedAt time.Time) {
	switch model := target.(type) {
	case *models.Status:
		model.StatusID = fmt.Sprintf("status-%d", publishedAt.Day())
		model.AuthorID = "https://localhost/users/alice"
		model.AuthorUsername = "alice"
		model.Visibility = models.VisibilityPublic
		model.PublishedAt = publishedAt
		model.Content = "hello"
		model.Flagged = true
		model.URLs = []string{"https://example.com/x"}
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

func TestStatusRepository_ext_sweep(t *testing.T) {
	ctx := context.Background()

	origDomain := config.Get().Domain
	config.Get().Domain = repositories.DefaultDomain
	defer func() { config.Get().Domain = origDomain }()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveStatusMocks(mockDB, mockQuery, mockUpdateBuilder)

	db := transactionalMockDB{MockDB: mockDB}
	repo := repositories.NewStatusRepository(db, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	// Ensure bookmark paths work without touching internals.
	repo.SetBookmarkRepository(repositories.NewBookmarkRepository(db, "test-table", zap.NewNop()))

	assert.Error(t, repo.CreateBoostStatus(ctx, nil))
	assert.Error(t, repo.CreateBoostStatus(ctx, &models.Status{StatusID: "s1", AuthorID: "a1"}))
	assert.Error(t, repo.CreateBoostStatus(ctx, &models.Status{StatusID: "s1", BoostOfStatusID: "t1"}))

	status := &models.Status{
		StatusID:        "s1",
		AuthorID:        "https://localhost/users/alice",
		AuthorUsername:  "alice",
		Visibility:      models.VisibilityPublic,
		BoostOfStatusID: "t1",
		Hashtags:        []string{"Go", " ", "#"},
		Language:        "en",
	}
	status.Note = &models.NoteField{Note: &activitypub.Note{BaseObject: activitypub.BaseObject{Type: "Note", ID: "https://example.com/status/1"}}}
	assert.NoError(t, repo.CreateStatus(ctx, status))

	_ = repo.UpdateStatus(ctx, status)
	_ = repo.DeleteStatus(ctx, "s1")
	_, _ = repo.DeleteBoostStatus(ctx, "booster", "target")

	_, _ = repo.CountStatusesByAuthor(ctx, "author")
	_, _ = repo.CountReplies(ctx, "parent")
	_ = repo.UpdateEngagementMetrics(ctx, "s1", 1, 2, 3, 4)
	_, _ = repo.GetTotalStatusCount(ctx)

	_, _, _ = repo.ListStatusesForAdmin(ctx, &interfaces.StatusFilter{Local: ptr(true)}, 2, "")
	_, _, _ = repo.ListStatusesForAdmin(ctx, &interfaces.StatusFilter{Remote: ptr(true)}, 2, "")
	_, _, _ = repo.ListStatusesForAdmin(ctx, &interfaces.StatusFilter{ByDomain: "remote.example"}, 2, "")
	_, _ = repo.CountStatusesForAdmin(ctx, &interfaces.StatusFilter{Remote: ptr(true)})

	_, _ = repo.GetStatusByURL(ctx, "https://example.com/status/1")
	_, _ = repo.GetStatusesByURL(ctx, "https://example.com/x", 1)

	_, _ = repo.GetPublicTimeline(ctx, interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetReplies(ctx, "parent", interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetFlaggedStatuses(ctx, interfaces.PaginationOptions{Limit: 1, Cursor: "c1"})
	_ = repo.FlagStatus(ctx, "s1", "", "")
	_ = repo.UnflagStatus(ctx, "s1")

	_, _ = repo.GetUserTimeline(ctx, "alice", interfaces.PaginationOptions{Limit: 1, Cursor: "c"})
	_, _ = repo.GetConversationThread(ctx, "conv", interfaces.PaginationOptions{Limit: 1, Cursor: "c"})
	_, _ = repo.SearchStatuses(ctx, "hello", interfaces.PaginationOptions{Limit: 1})
	_, _ = repo.GetTrendingStatuses(ctx, interfaces.PaginationOptions{Limit: 1})

	_, _ = repo.GetStatusesByHashtag(ctx, "go", interfaces.PaginationOptions{Limit: 1})

	_ = repo.LikeStatus(ctx, "user-1", "status-1")
	_ = repo.UnlikeStatus(ctx, "user-1", "status-1")
	_ = repo.ReblogStatus(ctx, "user-1", "status-1", "")
	_ = repo.UnreblogStatus(ctx, "user-1", "status-1")

	_ = repo.BookmarkStatus(ctx, "user-1", "status-1")
	_ = repo.UnbookmarkStatus(ctx, "user-1", "status-1")
	_, _, _, _ = repo.GetStatusEngagement(ctx, "status-1", "user-1")

	_, _, _, _ = repo.GetStatusCounts(ctx, "status-1")
	_, _, _ = repo.GetStatusContext(ctx, "status-1")
	_, _ = repo.GetStatusesByIDs(ctx, []string{"status-1", "status-1", ""})

	// Home timeline branches.
	repo.SetRelationshipRepository(nil)
	_, _ = repo.GetHomeTimeline(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})

	repo.SetRelationshipRepository(usernameRelationshipRepo{usernames: []string{"alice", "", "bob"}})
	_, _ = repo.GetHomeTimeline(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})

	repo.SetRelationshipRepository(usernameRelationshipRepo{usernames: []string{}})
	_, _ = repo.GetHomeTimeline(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})

	repo.SetRelationshipRepository(struct{}{})
	_, _ = repo.GetHomeTimeline(ctx, "user-1", interfaces.PaginationOptions{Limit: 1})
}

func TestStatusRepository_ext_hashtag_not_found(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)

	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(dynamormErrors.ErrItemNotFound).Once()
	setupPermissiveStatusMocks(mockDB, mockQuery, mockUpdateBuilder)

	repo := repositories.NewStatusRepository(mockDB, "test-table", zap.NewNop(), nil)
	page, err := repo.GetStatusesByHashtag(ctx, "go", interfaces.PaginationOptions{Limit: 10})
	assert.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestStatusRepository_ext_relationship_repo_interface_branch(t *testing.T) {
	ctx := context.Background()

	origDomain := config.Get().Domain
	config.Get().Domain = "example.com"
	defer func() { config.Get().Domain = origDomain }()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	setupPermissiveStatusMocks(mockDB, mockQuery, mockUpdateBuilder)

	db := transactionalMockDB{MockDB: mockDB}
	repo := repositories.NewStatusRepository(db, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	repo.SetRelationshipRepository(fakeRelationshipRepo{
		following: &interfaces.PaginatedResult[*storage.Account]{
			Items: []*storage.Account{
				{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}},
				{User: &storage.User{Username: "bob"}},
				nil,
			},
		},
	})

	page, err := repo.GetHomeTimeline(ctx, "user-1", interfaces.PaginationOptions{Limit: 9999})
	assert.NoError(t, err)
	assert.NotNil(t, page)
}

func ptr[T any](v T) *T { return &v }
