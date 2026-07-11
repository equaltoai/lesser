package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	theorydberrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

// ============================================================================
// DynamORM Mock Tests: searchExactUsername
// ============================================================================

func TestSearchExactUsername_ValidQuery(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"

	// Set up expectations for the exact match query shape
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "exact-match-id"},
			PreferredUsername: "alice",
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	assert.NoError(t, repo.searchExactUsername(ctx, query, &results, seen))

	assert.Len(t, results, 1)
	assert.Equal(t, "exact-match-id", results[0].ID)
	assert.True(t, seen["exact-match-id"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchExactUsername_QueryTooShort(t *testing.T) {
	mockDB := new(mocks.MockDB)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "ab" // Less than 3 characters required by ValidateRepositorySearchQuery

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	// Should return early without making any DB calls
	assert.NoError(t, repo.searchExactUsername(ctx, query, &results, seen))

	assert.Empty(t, results)
	assert.Empty(t, seen)

	// No mock expectations set, so no DB calls should have been made
	mockDB.AssertNotCalled(t, "WithContext", mock.Anything)
}

func TestSearchExactUsername_NotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "notfound"

	// Set up expectations for a query that returns an error (not found)
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#notfound").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(theorydberrors.ErrItemNotFound)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	assert.NoError(t, repo.searchExactUsername(ctx, query, &results, seen))

	assert.Empty(t, results)
	assert.Empty(t, seen)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchExactUsername_NilActorInResult(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"

	// Set up expectations - Actor field is nil
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		// Don't set actor.Actor - leave it nil
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	assert.Error(t, repo.searchExactUsername(ctx, query, &results, seen))

	// Should not add nil actor to results
	assert.Empty(t, results)
	assert.Empty(t, seen)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchExactUsername_DeduplicatesAlreadySeen(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#alice").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "already-seen-id"},
			PreferredUsername: "alice",
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := map[string]bool{"already-seen-id": true} // Already seen

	assert.NoError(t, repo.searchExactUsername(ctx, query, &results, seen))

	// Should not add duplicate
	assert.Empty(t, results)
	assert.True(t, seen["already-seen-id"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// DynamORM Mock Tests: searchUsernamePrefix
// ============================================================================

func TestSearchUsernamePrefix_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10
	offset := 0

	// Set up expectations for username prefix search using gsi1
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit+offset).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "alice-1"},
					PreferredUsername: "alice",
				},
			},
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "alice-2"},
					PreferredUsername: "alicewonderland",
				},
			},
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	_, err := repo.searchUsernamePrefix(ctx, query, limit, offset, &results, seen)
	assert.NoError(t, err)

	assert.Len(t, results, 2)
	assert.True(t, seen["alice-1"])
	assert.True(t, seen["alice-2"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchUsernamePrefix_DeduplicatesByActorID(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10
	offset := 0

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit+offset).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "alice-new"},
					PreferredUsername: "alicenew",
				},
			},
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	results = append(results, &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "alice-existing"},
		PreferredUsername: "aliceexisting",
	})
	seen := map[string]bool{"alice-existing": true}

	_, err := repo.searchUsernamePrefix(ctx, query, limit, offset, &results, seen)
	assert.NoError(t, err)

	// Should have 2: existing + new
	assert.Len(t, results, 2)
	assert.True(t, seen["alice-existing"])
	assert.True(t, seen["alice-new"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchUsernamePrefix_SkipsNilActors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10
	offset := 0

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit+offset).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{Actor: nil}, // nil Actor
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "alice-valid"},
					PreferredUsername: "alicevalid",
				},
			},
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	_, err := repo.searchUsernamePrefix(ctx, query, limit, offset, &results, seen)
	assert.NoError(t, err)

	// Only one valid result
	assert.Len(t, results, 1)
	assert.Equal(t, "alice-valid", results[0].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchUsernamePrefix_ErrorDoesNotPanic(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", zap.NewNop(), nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10
	offset := 0

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi1").Return(mockQuery)
	mockQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit+offset).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Return(ErrTestMockError)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	// Should not panic on error
	_, err := repo.searchUsernamePrefix(ctx, query, limit, offset, &results, seen)
	assert.Error(t, err)

	assert.Empty(t, results)
	assert.Empty(t, seen)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// DynamORM Mock Tests: searchDisplayName
// ============================================================================

func TestSearchDisplayName_Success(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10

	// Set up expectations for display name search using gsi2
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "name-match-1"},
					PreferredUsername: "user1",
					Name:              "Alice Smith",
				},
			},
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	_, err := repo.searchDisplayName(ctx, query, limit, &results, seen)
	assert.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "name-match-1", results[0].ID)
	assert.True(t, seen["name-match-1"])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDisplayName_QueryTooShort(t *testing.T) {
	mockDB := new(mocks.MockDB)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "a" // Less than 2 characters
	limit := 10

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	// Should return early without making any DB calls
	_, err := repo.searchDisplayName(ctx, query, limit, &results, seen)
	assert.NoError(t, err)

	assert.Empty(t, results)
	assert.Empty(t, seen)

	// No mock expectations set, so no DB calls should have been made
	mockDB.AssertNotCalled(t, "WithContext", mock.Anything)
}

func TestSearchDisplayName_DeduplicatesByActorID(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "already-seen"},
					PreferredUsername: "user1",
				},
			},
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "new-match"},
					PreferredUsername: "user2",
				},
			},
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := map[string]bool{"already-seen": true}

	_, err := repo.searchDisplayName(ctx, query, limit, &results, seen)
	assert.NoError(t, err)

	// Only new match should be added
	assert.Len(t, results, 1)
	assert.Equal(t, "new-match", results[0].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDisplayName_SkipsNilActors(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{Actor: nil}, // nil Actor
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "valid-match"},
					PreferredUsername: "valid",
				},
			},
		}
	}).Return(nil)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	_, err := repo.searchDisplayName(ctx, query, limit, &results, seen)
	assert.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "valid-match", results[0].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestSearchDisplayName_ErrorDoesNotPanic(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", zap.NewNop(), nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Index", "gsi2").Return(mockQuery)
	mockQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQuery)
	mockQuery.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQuery)
	mockQuery.On("Limit", limit).Return(mockQuery)
	mockQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Return(ErrTestMockError)

	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	// Should not panic on error
	_, err := repo.searchDisplayName(ctx, query, limit, &results, seen)
	assert.Error(t, err)

	assert.Empty(t, results)
	assert.Empty(t, seen)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ============================================================================
// Integration Test: All Search Strategies Together
// ============================================================================

func TestExecuteSearchStrategies_CallsAllThreeStrategies(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQueryExact := new(mocks.MockQuery)
	mockQueryPrefix := new(mocks.MockQuery)
	mockQueryName := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10
	offset := 0

	// Strategy 1: Exact username match
	mockDB.On("WithContext", ctx).Return(mockDB).Times(3)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQueryExact).Once()
	mockQueryExact.On("Where", "PK", "=", "ACTOR#alice").Return(mockQueryExact)
	mockQueryExact.On("Where", "SK", "=", "PROFILE").Return(mockQueryExact)
	mockQueryExact.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "exact-match"},
			PreferredUsername: "alice",
		}
	}).Return(nil)

	// Strategy 2: Username prefix search
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQueryPrefix).Once()
	mockQueryPrefix.On("Index", "gsi1").Return(mockQueryPrefix)
	mockQueryPrefix.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQueryPrefix)
	mockQueryPrefix.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQueryPrefix)
	mockQueryPrefix.On("Limit", limit+offset).Return(mockQueryPrefix)
	mockQueryPrefix.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "prefix-match"},
					PreferredUsername: "alicewonderland",
				},
			},
		}
	}).Return(nil)

	// Strategy 3: Display name search
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQueryName).Once()
	mockQueryName.On("Index", "gsi2").Return(mockQueryName)
	mockQueryName.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQueryName)
	mockQueryName.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQueryName)
	mockQueryName.On("Limit", limit).Return(mockQueryName)
	mockQueryName.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "name-match"},
					PreferredUsername: "user_alice",
					Name:              "Alice Smith",
				},
			},
		}
	}).Return(nil)

	results, seen, err := repo.executeSearchStrategies(ctx, query, limit, offset)
	assert.NoError(t, err)

	// Should have 3 unique results
	assert.Len(t, results, 3)
	assert.Len(t, seen, 3)
	assert.True(t, seen["exact-match"])
	assert.True(t, seen["prefix-match"])
	assert.True(t, seen["name-match"])

	mockDB.AssertExpectations(t)
	mockQueryExact.AssertExpectations(t)
	mockQueryPrefix.AssertExpectations(t)
	mockQueryName.AssertExpectations(t)
}

func TestExecuteSearchStrategies_DeduplicatesAcrossStrategies(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQueryExact := new(mocks.MockQuery)
	mockQueryPrefix := new(mocks.MockQuery)
	mockQueryName := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	query := "alice"
	limit := 10
	offset := 0

	// All strategies return the same actor ID
	mockDB.On("WithContext", ctx).Return(mockDB).Times(3)

	// Strategy 1
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQueryExact).Once()
	mockQueryExact.On("Where", "PK", "=", "ACTOR#alice").Return(mockQueryExact)
	mockQueryExact.On("Where", "SK", "=", "PROFILE").Return(mockQueryExact)
	mockQueryExact.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "same-id"},
			PreferredUsername: "alice",
		}
	}).Return(nil)

	// Strategy 2 - returns same ID
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQueryPrefix).Once()
	mockQueryPrefix.On("Index", "gsi1").Return(mockQueryPrefix)
	mockQueryPrefix.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#al").Return(mockQueryPrefix)
	mockQueryPrefix.On("Where", "gsi1SK", "BEGINS_WITH", "alice").Return(mockQueryPrefix)
	mockQueryPrefix.On("Limit", limit+offset).Return(mockQueryPrefix)
	mockQueryPrefix.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "same-id"}, // Same ID
					PreferredUsername: "alice",
				},
			},
		}
	}).Return(nil)

	// Strategy 3 - returns same ID
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQueryName).Once()
	mockQueryName.On("Index", "gsi2").Return(mockQueryName)
	mockQueryName.On("Where", "gsi2PK", "=", "NAME_SEARCH#al").Return(mockQueryName)
	mockQueryName.On("Where", "gsi2SK", "BEGINS_WITH", "alice").Return(mockQueryName)
	mockQueryName.On("Limit", limit).Return(mockQueryName)
	mockQueryName.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		actors := args.Get(0).(*[]models.Actor)
		*actors = []models.Actor{
			{
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "same-id"}, // Same ID
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
		}
	}).Return(nil)

	results, seen, err := repo.executeSearchStrategies(ctx, query, limit, offset)
	assert.NoError(t, err)

	// Should only have 1 unique result despite 3 matches
	assert.Len(t, results, 1)
	assert.Len(t, seen, 1)
	assert.Equal(t, "same-id", results[0].ID)

	mockDB.AssertExpectations(t)
}
