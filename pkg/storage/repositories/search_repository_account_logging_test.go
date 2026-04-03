package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestExecuteSearchStrategies_LogsSummaryWhenIndexedQueriesMiss(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	mockDB := new(mocks.MockDB)
	prefixQuery := new(mocks.MockQuery)
	displayQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", logger, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(prefixQuery).Once()
	prefixQuery.On("Index", "gsi1").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#ag").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1SK", "BEGINS_WITH", "ag").Return(prefixQuery)
	prefixQuery.On("Limit", 10).Return(prefixQuery)
	prefixQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(displayQuery).Once()
	displayQuery.On("Index", "gsi2").Return(displayQuery)
	displayQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#ag").Return(displayQuery)
	displayQuery.On("Where", "gsi2SK", "BEGINS_WITH", "ag").Return(displayQuery)
	displayQuery.On("Limit", 10).Return(displayQuery)
	displayQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{}
	}).Return(nil)

	results, _ := repo.executeSearchStrategies(ctx, "ag", 10, 0)
	require.Empty(t, results)

	entries := recorded.FilterMessage("account search hydration summary").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, int64(0), fields["result_count"])
	require.Equal(t, int64(0), fields["username_prefix_raw_matches"])
	require.Equal(t, int64(0), fields["display_name_raw_matches"])
}

func TestExecuteSearchStrategies_LogsDiscardReasonsForIndexedMatches(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	mockDB := new(mocks.MockDB)
	prefixQuery := new(mocks.MockQuery)
	displayQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", logger, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(prefixQuery).Once()
	prefixQuery.On("Index", "gsi1").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#ag").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1SK", "BEGINS_WITH", "ag").Return(prefixQuery)
	prefixQuery.On("Limit", 10).Return(prefixQuery)
	prefixQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{{GSI1SK: ""}}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(displayQuery).Once()
	displayQuery.On("Index", "gsi2").Return(displayQuery)
	displayQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#ag").Return(displayQuery)
	displayQuery.On("Where", "gsi2SK", "BEGINS_WITH", "ag").Return(displayQuery)
	displayQuery.On("Limit", 10).Return(displayQuery)
	displayQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{}
	}).Return(nil)

	results, _ := repo.executeSearchStrategies(ctx, "ag", 10, 0)
	require.Empty(t, results)

	entries := recorded.FilterMessage("account search hydration summary").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, int64(1), fields["username_prefix_raw_matches"])
	require.Equal(t, int64(1), fields["username_prefix_discarded"])
	require.Equal(t, map[string]int{"missing_match_username": 1}, fields["username_prefix_discard_reasons"])
}

func TestSearchUsernamePrefix_LogsHydrationWarningWhenLookupFails(t *testing.T) {
	core, recorded := observer.New(zapcore.WarnLevel)
	logger := zap.New(core)

	mockDB := new(mocks.MockDB)
	prefixQuery := new(mocks.MockQuery)
	fullActorQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", logger, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(prefixQuery).Once()
	prefixQuery.On("Index", "gsi1").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#ar").Return(prefixQuery)
	prefixQuery.On("Where", "gsi1SK", "BEGINS_WITH", "ar").Return(prefixQuery)
	prefixQuery.On("Limit", 10).Return(prefixQuery)
	prefixQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{{PK: "ACTOR#arch", Username: "arch", GSI1SK: "arch"}}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(fullActorQuery).Once()
	fullActorQuery.On("Where", "PK", "=", "ACTOR#arch").Return(fullActorQuery)
	fullActorQuery.On("Where", "SK", "=", "PROFILE").Return(fullActorQuery)
	fullActorQuery.On("First", mock.AnythingOfType("*models.Actor")).Return(assertiveLookupError{})

	repo.searchUsernamePrefix(ctx, "ar", 10, 0, &results, seen)

	entries := recorded.FilterMessage("indexed account search hydration failed").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "username_prefix", fields["strategy"])
	require.Equal(t, "arch", fields["username"])
}

type assertiveLookupError struct{}

func (assertiveLookupError) Error() string {
	return "lookup failed"
}
