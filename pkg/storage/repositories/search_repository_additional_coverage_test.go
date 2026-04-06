package repositories

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type fakeSearchRepositoryDeps struct {
	following       []string
	blockedPairs    map[string]bool
	blockCheckError bool
}

func (d *fakeSearchRepositoryDeps) GetFollowing(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return d.following, "", nil
}

func (d *fakeSearchRepositoryDeps) IsBlocked(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func (d *fakeSearchRepositoryDeps) IsBlockedBidirectional(_ context.Context, actor1, actor2 string) (bool, error) {
	if d.blockCheckError {
		return false, assert.AnError
	}
	if d.blockedPairs == nil {
		return false, nil
	}
	return d.blockedPairs[actor1+"|"+actor2] || d.blockedPairs[actor2+"|"+actor1], nil
}

func (d *fakeSearchRepositoryDeps) GetFollowers(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return nil, "", nil
}

type statusPrivacyDeps struct {
	blockedTargets map[string]bool
	errorTargets   map[string]bool
}

func (d *statusPrivacyDeps) GetFollowing(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return nil, "", nil
}
func (d *statusPrivacyDeps) IsBlocked(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (d *statusPrivacyDeps) IsBlockedBidirectional(_ context.Context, actor1, actor2 string) (bool, error) {
	if d.errorTargets != nil && d.errorTargets[actor1+"|"+actor2] {
		return false, assert.AnError
	}
	if d.blockedTargets == nil {
		return false, nil
	}
	return d.blockedTargets[actor1+"|"+actor2], nil
}
func (d *statusPrivacyDeps) GetFollowers(_ context.Context, _ string, _ int, _ string) ([]string, string, error) {
	return nil, "", nil
}

func setupPermissiveSearchRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Offset", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateSearchRepositorySliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateSearchRepositoryStructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Delete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("BatchDelete", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(0), nil).Maybe()
}

func populateSearchRepositorySliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	if elemType.Kind() == reflect.Interface {
		return
	}

	baseElemType := elemType
	if baseElemType.Kind() == reflect.Ptr {
		baseElemType = baseElemType.Elem()
	}

	count := 2
	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseElemType)
			populateSearchRepositoryStructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseElemType)
			populateSearchRepositoryStructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateSearchRepositoryStructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute)

	switch model := target.(type) {
	case *models.Actor:
		model.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "actor-1"},
			PreferredUsername: "alice",
			Name:              "Alice",
		}
	case *models.Object:
		model.ID = "status-1"
		model.Type = ActivityTypeNote
		model.Content = "hello world #tag"
		model.URL = "https://example.com/status/1"
		model.AttributedTo = "https://example.com/users/alice"
		model.Published = now
	case *models.Hashtag:
		model.Name = "tag"
		model.UsageCount = 7
		model.FirstSeen = baseTime.Add(-24 * time.Hour)
		model.LastUsed = now
	case *models.Status:
		model.StatusID = "status-2"
		model.Content = "hello #tag"
		model.AuthorID = "https://example.com/users/alice"
		model.AuthorUsername = "alice"
		model.PublishedAt = now
		model.Hashtags = []string{"tag"}
		model.Visibility = "public"
		model.Language = "en"
	case *models.SearchSuggestion:
		model.Type = "username"
		model.Term = "al"
		model.Score = 1.0
		model.LastUsed = now
		model.UseCount = 2
		model.CreatedAt = baseTime
		model.UpdatedAt = now
	case *models.SearchAnalytics:
		model.Query = "alice"
		model.SearchType = "accounts"
		model.ResultCount = 1
		model.SearchTime = 10
		model.Timestamp = now
	case *models.SearchQueryStats:
		model.QueryHash = "al_hash_3"
		model.QueryCount = 10
		model.LastQueried = now
		model.QueryType = "search_suggestion"
	case *models.SearchEmbedding:
		model.ContentID = "content-1"
		model.ContentType = "status"
		model.Embedding = []float32{1, 0, 0}
		model.CreatedAt = now
	case *models.TrendingStatus:
		model.PK = "TRENDING#STATUS#status-1"
		model.SK = "TRENDING"
	case *models.SearchCache:
		model.PK = "SEARCH_CACHE#query"
		model.SK = "RESULTS"
		model.Query = "query"
		model.Results = map[string]interface{}{
			"status_ids": []string{"status-1", "status-2"},
		}
		model.CreatedAt = now
	default:
		// Best-effort: do nothing for unhandled types.
	}
}

func TestSearchRepository_CoverageSweep(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	setupPermissiveSearchRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetDependencies(&fakeSearchRepositoryDeps{
		following: []string{"alice"},
		blockedPairs: map[string]bool{
			"searcher|blocked": true,
		},
	})

	ctx := context.Background()

	// Search embeddings
	assert.Error(t, repo.IndexContentEmbedding(ctx, nil))
	require.NoError(t, repo.IndexContentEmbedding(ctx, &models.SearchEmbedding{
		ContentID:   "content-1",
		ContentType: "status",
		Embedding:   []float32{1, 0, 0},
	}))
	_, _ = repo.SearchByEmbedding(ctx, []float32{1, 0, 0}, 1, 2.0) // invalid threshold triggers default
	_, _, _ = repo.SearchByEmbeddingPaginated(ctx, []float32{1, 0, 0}, 0.7, &PaginationOptions{Limit: 1, SortOrder: SearchSortRelevance})
	_ = repo.UpdateEmbedding(ctx, "content-1", []float32{1, 0, 0})
	_ = repo.DeleteEmbedding(ctx, "content-1")

	// Account search
	_, _ = repo.SearchAccounts(ctx, "alice", 10, false, 0)
	_, _ = repo.SearchAccountsWithPrivacy(ctx, "alice", 10, false, 0, "searcher")
	_, _ = repo.SearchAccountsAdvanced(ctx, "alice", false, 10, 0, true, "searcher")

	// Status search
	_, _ = repo.SearchStatuses(ctx, "world", 10)
	_, _, _ = repo.SearchStatusesWithOptionsPaginated(ctx, "https://example.com/status/1", storage.StatusSearchOptions{Limit: 10})
	_, _, _ = repo.SearchStatusesWithOptionsPaginated(ctx, "#tag", storage.StatusSearchOptions{Limit: 10, LocalOnly: true})
	_, _ = repo.SearchStatusesWithPrivacy(ctx, "world", storage.StatusSearchOptions{Limit: 10}, "searcher")
	_, _, _ = repo.SearchStatusesWithPrivacyPaginated(ctx, "world", storage.StatusSearchOptions{Limit: 2}, "searcher")

	maxID := "status-z"
	minID := "status-a"
	_, _ = repo.SearchStatusesAdvanced(ctx, "world", 10, &maxID, &minID, "https://example.com/users/alice")

	// Comprehensive search
	_, _ = repo.SearchAll(ctx, "alice", 10, "")
	_, _, _ = repo.SearchAllPaginated(ctx, "alice", &PaginationOptions{Limit: 10, SortOrder: SearchSortRelevance})

	// Hashtags
	_, _ = repo.SearchHashtags(ctx, "#tag", 10)
	_, _, _ = repo.SearchHashtagsAdvancedPaginated(ctx, "#tag", &PaginationOptions{Limit: 1, SortOrder: SearchSortRelevance})

	// Suggestions
	assert.Error(t, repo.CreateSearchSuggestion(ctx, nil))
	_ = repo.CreateSearchSuggestion(ctx, &models.SearchSuggestion{Type: "username", Term: "al"})
	require.NoError(t, repo.UpdateSearchSuggestion(ctx, "username", "al", nil))
	_ = repo.UpdateSearchSuggestion(ctx, "username", "al", map[string]interface{}{"score": 1.5})
	_, _ = repo.GetSearchSuggestions(ctx, "al", 5)
	_ = repo.IncrementSuggestionUse(ctx, "username", "al")
	_ = repo.PruneOldSuggestions(ctx, time.Now().Add(-24*time.Hour))

	// Indexing
	_ = repo.IndexStatus(ctx, nil)
	_ = repo.IndexStatus(ctx, &models.Object{ID: "status-1", Type: ActivityTypeNote, Content: "hello #tag"})
	_ = repo.UnindexStatus(ctx, "status-1")

	_, _ = repo.SearchStatusesByHashtag(ctx, "#tag", 10)
	_, _ = repo.SearchStatusesByAuthor(ctx, "https://example.com/users/alice", 10)

	// Analytics
	assert.Error(t, repo.RecordSearch(ctx, nil))
	_ = repo.RecordSearch(ctx, &models.SearchAnalytics{Query: "alice", SearchType: "accounts"})
	_ = repo.RecordSearchWithPrivacy(ctx, "alice", "accounts", 1, 10, nil)
	_, _ = repo.GetSearchAnalytics(ctx, baseTime, baseTime.AddDate(0, 0, 1))
	_, _ = repo.GetPopularSearches(ctx, 10, 24*time.Hour)
	_, _ = repo.GetSearchTrends(ctx, 1)

	// Pure helpers
	t.Setenv("DOMAIN", "example.com")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)
	assert.True(t, repo.isURL("https://example.com"))
	assert.False(t, repo.isURL("not-a-url"))
	assert.Equal(t, []string{"tag"}, repo.extractHashtags("hello #Tag"))
	assert.Equal(t, "alice", extractUsernameFromActor("https://example.com/users/alice"))
	localStatusID, ok := repo.extractStatusIDFromURL("https://example.com/users/alice/statuses/123")
	assert.True(t, ok)
	assert.Equal(t, "123", localStatusID)
	remoteStatusID, ok := repo.extractStatusIDFromURL("https://remote.example/users/alice/statuses/123")
	assert.True(t, ok)
	assert.True(t, models.IsCanonicalRemoteStatusID(remoteStatusID))
	assert.True(t, repo.isLocalStatus("status-1"))
	assert.False(t, repo.isLocalStatus("https://example.com/status/1"))
	assert.False(t, repo.isLocalStatus(models.CanonicalStatusID("https://remote.example/users/alice/statuses/123")))
	assert.Greater(t, repo.calculateContentScore("hello world", "world"), 0.0)
	assert.Equal(t, 0.0, repo.cosineSimilarity([]float32{1}, []float32{1, 0}))
	assert.Equal(t, "[empty]", repo.hashQuery(""))
	assert.Equal(t, "[short]", repo.hashQuery("abc"))
	assert.Contains(t, repo.hashQuery("alice"), "al_hash_")
}

func TestSearchRepository_SearchByEmbeddingPaginated_InvalidCursor(t *testing.T) {
	repo := NewSearchRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	_, _, err := repo.SearchByEmbeddingPaginated(context.Background(), []float32{1, 0}, 0.7, &PaginationOptions{Limit: 10, Cursor: "not-a-cursor"})
	assert.Error(t, err)
}

func TestSearchRepository_SearchByEmbeddingPaginated_EmptyEmbeddingReturnsError(t *testing.T) {
	repo := NewSearchRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	_, _, err := repo.SearchByEmbeddingPaginated(context.Background(), nil, 0.7, &PaginationOptions{Limit: 10})
	assert.Error(t, err)
}

func TestSearchRepository_PrivacyFiltering_FailOpenOnBlockCheckError(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)
	repo.SetDependencies(&fakeSearchRepositoryDeps{blockCheckError: true})

	results := repo.filterAccountsByPrivacy(context.Background(), []*activitypub.Actor{
		nil,
		{BaseObject: activitypub.BaseObject{ID: "searcher"}, PreferredUsername: "searcher"},
		{BaseObject: activitypub.BaseObject{ID: "other"}, PreferredUsername: "other"},
	}, "searcher")
	require.Len(t, results, 2)
}

func TestSearchRepository_Suggestions_CreatePathOnNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	setupPermissiveSearchRepoMocks(mockDB, mockQuery, baseTime)

	// Make the Get() inside IncrementSuggestionUse behave like a "not found" case.
	mockQuery.On("First", mock.AnythingOfType("*models.SearchSuggestion")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.IncrementSuggestionUse(context.Background(), "username", "al")
	assert.NoError(t, err)
}

func TestSearchRepository_CompareRelevance_AndSortCoverage(t *testing.T) {
	repo := NewSearchRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

	a := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "a"}, PreferredUsername: "alice"}
	b := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "b"}, PreferredUsername: "alicewonder"}
	assert.True(t, repo.compareRelevance(a, b, "alice"))
	assert.False(t, repo.compareRelevance(b, a, "alice"))

	results := []*activitypub.Actor{b, a}
	repo.sortAccountResults(results, "alice")
	assert.Equal(t, "a", results[0].ID)
}

func TestSearchRepository_FilterStatusesByPrivacy_BlockedAndFailOpen(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)
	repo.SetDependencies(&statusPrivacyDeps{
		blockedTargets: map[string]bool{"searcher|blocked-author": true},
		errorTargets:   map[string]bool{"searcher|error-author": true},
	})

	results := repo.filterStatusesByPrivacy(context.Background(), []*storage.StatusSearchResult{
		{StatusID: "s1", AuthorID: "blocked-author"},
		{StatusID: "s2", AuthorID: "error-author"},
		{StatusID: "s3", AuthorID: "ok-author"},
	}, "searcher")

	require.Len(t, results, 2)
	assert.Equal(t, "s2", results[0].StatusID)
	assert.Equal(t, "s3", results[1].StatusID)
}

func TestSearchRepository_UpdateSearchSuggestion_AppliesAllUpdateFields(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	ctx := context.Background()
	lastUsed := time.Now().Add(-time.Hour)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchSuggestion")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchSuggestion")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.SearchSuggestion)
		out.Type = "username"
		out.Term = "al"
		out.Score = 0.1
		out.UseCount = 1
		out.LastUsed = lastUsed
	}).Return(nil)

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Update", mock.Anything).Return(nil)

	updates := map[string]interface{}{
		"score":     float64(9.9),
		"last_used": time.Now(),
		"use_count": 42,
	}
	err := repo.UpdateSearchSuggestion(ctx, "username", "al", updates)
	assert.NoError(t, err)
}

func TestSearchRepository_PrivacyFilterAnalyticsEvent_TruncatesAndRedactsUser(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	userID := "user-1"
	event := &models.SearchAnalytics{
		Query:  "dm " + strings.Repeat("x", 200),
		UserID: &userID,
	}

	repo.privacyFilterAnalyticsEvent(event)
	assert.Nil(t, event.UserID)
	assert.LessOrEqual(t, len(event.Query), 100)
	assert.Contains(t, event.Query, "...")
}

func TestSearchRepository_PrivacyFilterAnalyticsEvent_HashesSensitiveQuery(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	userID := "user-1"
	event := &models.SearchAnalytics{
		Query:  "test@example.com",
		UserID: &userID,
	}

	repo.privacyFilterAnalyticsEvent(event)
	assert.Contains(t, event.Query, "_hash_")
}

func TestSearchRepository_ProcessBatch_ErrorsAndEmptyBreaks(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchEmbedding")).Return(mockQuery)

	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Offset", mock.Anything).Return(mockQuery)

	baseQuery := repo.buildBaseQuery(context.Background(), &CursorData{})

	t.Run("not_found_breaks", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]models.SearchEmbedding")).Return(dynamormerrors.ErrItemNotFound).Once()
		emb, shouldBreak, err := repo.processBatch(baseQuery, 0, 100)
		assert.NoError(t, err)
		assert.True(t, shouldBreak)
		assert.Nil(t, emb)
	})

	t.Run("query_error_returns_error", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]models.SearchEmbedding")).Return(ErrTestMockError).Once()
		_, shouldBreak, err := repo.processBatch(baseQuery, 0, 100)
		assert.Error(t, err)
		assert.False(t, shouldBreak)
	})

	t.Run("empty_slice_breaks", func(t *testing.T) {
		mockQuery.On("All", mock.AnythingOfType("*[]models.SearchEmbedding")).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]models.SearchEmbedding)
			*dest = nil
		}).Return(nil).Once()
		emb, shouldBreak, err := repo.processBatch(baseQuery, 0, 100)
		assert.NoError(t, err)
		assert.True(t, shouldBreak)
		assert.Nil(t, emb)
	})
}

func TestSearchRepository_SearchStatusesWithOptionsPaginated_SortOrderMapping(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	setupPermissiveSearchRepoMocks(mockDB, mockQuery, baseTime)

	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	_, _, err := repo.SearchStatusesWithOptionsPaginated(context.Background(), "hello", storage.StatusSearchOptions{
		Limit:     10,
		SortOrder: "time_asc",
	})
	require.NoError(t, err)

	_, _, err = repo.SearchStatusesWithOptionsPaginated(context.Background(), "hello", storage.StatusSearchOptions{
		Limit:     10,
		SortOrder: "time_desc",
	})
	require.NoError(t, err)
}

func TestSearchRepository_ExtractUsername_Branches(t *testing.T) {
	assert.Equal(t, "alice", extractUsernameFromActor("https://example.com/users/alice"))
	assert.Equal(t, "alice", extractUsernameFromActor("alice"))
	assert.Equal(t, "b", extractUsernameFromActor("https://example.com/users/a/b"))
	assert.Equal(t, "bob", extractUsernameFromActor("https://example.com/users/alice/users/bob"))
}

func TestSearchRepository_StatusModelToSearchResult_ReturnsNilForNilInput(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	assert.Nil(t, repo.statusModelToSearchResult(nil, 1, "s"))
}

func TestSearchRepository_CompareRelevance_CoversLengthTieBreaker(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	a := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "a"}, PreferredUsername: "bob"}
	b := &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "b"}, PreferredUsername: "charlie"}
	assert.True(t, repo.compareRelevance(a, b, "alice"))
}

func TestSearchRepository_IncrementSuggestionUse_PropagatesNonNotFoundError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchSuggestion")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchSuggestion")).Return(ErrTestMockError)

	err := repo.IncrementSuggestionUse(context.Background(), "username", "al")
	assert.Error(t, err)
}

func TestSearchRepository_EmbeddingCRUD_ErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)

	t.Run("create_error", func(t *testing.T) {
		mockQuery.On("Create").Return(ErrTestMockError).Once()
		err := repo.createSearchEmbedding(context.Background(), &models.SearchEmbedding{ContentID: "x", Embedding: []float32{1}})
		assert.Error(t, err)
	})

	t.Run("get_not_found", func(t *testing.T) {
		mockQuery.On("First", mock.AnythingOfType("*models.SearchEmbedding")).Return(dynamormerrors.ErrItemNotFound).Once()
		emb, err := repo.getSearchEmbedding(context.Background(), "missing")
		assert.Error(t, err)
		assert.Nil(t, emb)
	})

	t.Run("update_error", func(t *testing.T) {
		mockQuery.On("Update", mock.Anything).Return(ErrTestMockError).Once()
		err := repo.updateSearchEmbedding(context.Background(), &models.SearchEmbedding{ContentID: "x", Embedding: []float32{1}})
		assert.Error(t, err)
	})

	t.Run("delete_error", func(t *testing.T) {
		mockQuery.On("Delete").Return(ErrTestMockError).Once()
		err := repo.deleteSearchEmbedding(context.Background(), "x")
		assert.Error(t, err)
	})
}

func TestSearchRepository_IsValidEmbedding_Branches(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	assert.True(t, repo.isValidEmbedding(models.SearchEmbedding{Embedding: []float32{1, 0}}, []float32{1, 0}))
	assert.False(t, repo.isValidEmbedding(models.SearchEmbedding{ContentID: "x", Embedding: []float32{1}}, []float32{1, 0}))
}

func TestSearchRepository_SearchHashtagsAdvancedPaginated_InvalidCursorReturnsError(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	_, _, err := repo.SearchHashtagsAdvancedPaginated(context.Background(), "tag", &PaginationOptions{
		Limit:  10,
		Cursor: "not-a-cursor",
	})
	assert.Error(t, err)
}

func TestSearchRepository_ValidateSearchParams_InvalidOptionsReturnsError(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	threshold := 0.7
	opts := &PaginationOptions{Limit: 10, SortOrder: "bad"}
	optPtr := opts

	err := repo.validateSearchParams([]float32{1, 0}, &threshold, &optPtr)
	assert.Error(t, err)
}

func TestSearchRepository_BuildBaseQuery_AppliesCursorFilters(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	ctx := context.Background()
	ts := time.Date(2025, 1, 1, 2, 3, 4, 0, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchEmbedding")).Return(mockQuery)
	mockQuery.On("OrderBy", "CreatedAt", "DESC").Return(mockQuery)
	mockQuery.On("Filter", "ContentID", ">", "content-1").Return(mockQuery)
	mockQuery.On("Filter", "CreatedAt", "<", ts.Format(time.RFC3339)).Return(mockQuery)

	_ = repo.buildBaseQuery(ctx, &CursorData{LastID: "content-1", LastTimestamp: ts})
	mockQuery.AssertExpectations(t)
}

func TestSearchRepository_SearchStatusesByHashtag_NotFoundAndErrorBranches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.HashtagStatusIndex")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	mockQuery.On("All", mock.AnythingOfType("*[]models.HashtagStatusIndex")).Return(dynamormerrors.ErrItemNotFound).Once()
	results, err := repo.SearchStatusesByHashtag(context.Background(), "#tag", 5)
	require.NoError(t, err)
	assert.Empty(t, results)

	mockQuery.On("All", mock.AnythingOfType("*[]models.HashtagStatusIndex")).Return(ErrTestMockError).Once()
	_, err = repo.SearchStatusesByHashtag(context.Background(), "#tag", 5)
	assert.Error(t, err)
}

func TestSearchRepository_GetPopularSearches_AggregatesAboveMinCount(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	now := time.Date(2025, 1, 1, 1, 2, 3, 0, time.UTC)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchAnalytics")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.SearchAnalytics")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchAnalytics)
		events := make([]models.SearchAnalytics, 0, 6)
		for i := range 6 {
			events = append(events, models.SearchAnalytics{
				Query:      "hello",
				SearchType: "accounts",
				Timestamp:  now.Add(time.Duration(i) * time.Minute),
			})
		}
		events = append(events, models.SearchAnalytics{
			Query:      "test@example.com",
			SearchType: "accounts",
			Timestamp:  now,
		})
		*dest = events
	}).Return(nil).Once()

	stats, err := repo.GetPopularSearches(context.Background(), 10, 0)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, "he_hash_5", stats[0].QueryHash)
}

func TestSearchRepository_UpdateEmbedding_ReturnsErrorWhenEmbeddingMissing(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchEmbedding")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.SearchEmbedding")).Return(dynamormerrors.ErrItemNotFound).Once()

	err := repo.UpdateEmbedding(context.Background(), "content-1", []float32{1, 0})
	assert.Error(t, err)
}

func TestSearchRepository_GetRecentStatuses_ErrorReturnsEmpty(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Status")).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(ErrTestMockError).Once()

	recent := repo.getRecentStatuses(context.Background())
	assert.Empty(t, recent)
}

func TestSearchRepository_StatusMatchesQuery_EmptyContentReturnsFalse(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)
	assert.False(t, repo.statusMatchesQuery(models.Status{Content: ""}, "x"))
}

func TestSearchRepository_GetSearchSuggestions_DedupSortAndTrim(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	ctx := context.Background()
	now := time.Now()

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.SearchSuggestion")).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// 1) error branch for one type
	mockQuery.On("All", mock.AnythingOfType("*[]models.SearchSuggestion")).Return(ErrTestMockError).Once()
	// 2) success with duplicates within a type
	mockQuery.On("All", mock.AnythingOfType("*[]models.SearchSuggestion")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchSuggestion)
		*dest = []models.SearchSuggestion{
			{Type: "hashtag", Term: "al", Score: 1.0, LastUsed: now.Add(-time.Minute)},
			{Type: "hashtag", Term: "al", Score: 0.5, LastUsed: now},
		}
	}).Return(nil).Once()
	// 3) includes a duplicate key and a higher-score alternative
	mockQuery.On("All", mock.AnythingOfType("*[]models.SearchSuggestion")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.SearchSuggestion)
		*dest = []models.SearchSuggestion{
			{Type: "hashtag", Term: "al", Score: 1.0, LastUsed: now},
			{Type: "display_name", Term: "al", Score: 2.0, LastUsed: now},
		}
	}).Return(nil).Once()

	suggestions, err := repo.GetSearchSuggestions(ctx, "al", 1)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, "display_name", suggestions[0].Type)
}

func TestSearchRepository_SearchStatusesWithOptionsPaginated_EmptyQueryReturnsEmpty(t *testing.T) {
	repo := NewSearchRepository(nil, "test-table", zap.NewNop(), nil)

	results, page, err := repo.SearchStatusesWithOptionsPaginated(context.Background(), "   ", storage.StatusSearchOptions{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, results)
	require.NotNil(t, page)
	assert.False(t, page.HasNextPage)
	assert.Equal(t, 0, page.TotalScanned)
}

func TestSearchRepository_SearchStatusesWithPrivacyPaginated_SetsNextCursorWhenMoreResults(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewSearchRepository(mockDB, "test-table", zap.NewNop(), nil)

	ctx := context.Background()

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()

	// Populate content search with enough matches to create a next cursor.
	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.Status)
		now := time.Now()
		out := make([]models.Status, 0, 7)
		for i := range 7 {
			id := fmt.Sprintf("status-%d", i)
			out = append(out, models.Status{
				StatusID:       id,
				Content:        "hello world",
				AuthorID:       "https://example.com/users/alice",
				AuthorUsername: "alice",
				PublishedAt:    now.Add(time.Duration(-i) * time.Minute),
			})
		}
		*dest = out
	}).Return(nil).Once()

	// No hashtag matches.
	mockQuery.On("All", mock.AnythingOfType("*[]models.Status")).Return(nil).Maybe()

	results, page, err := repo.SearchStatusesWithPrivacyPaginated(ctx, "hello", storage.StatusSearchOptions{Limit: 2}, "searcher")
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.NotNil(t, page)
	assert.True(t, page.HasNextPage)
	assert.NotEmpty(t, page.NextCursor)
}
