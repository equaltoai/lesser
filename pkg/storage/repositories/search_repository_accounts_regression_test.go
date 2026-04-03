package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestSearchExactUsername_HyphenatedServiceActor(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "ACTOR#agent-0").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery)
	mockQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/agent-0", Type: activitypub.ServiceType},
			PreferredUsername: "agent-0",
			Name:              "Agent 0",
		}
	}).Return(nil)

	repo.searchExactUsername(ctx, "agent-0", &results, seen)

	assert.Len(t, results, 1)
	assert.Equal(t, "agent-0", results[0].PreferredUsername)
	assert.Equal(t, activitypub.ServiceType, results[0].Type)
	assert.True(t, seen["https://example.com/users/agent-0"])
}

func TestSearchUsernamePrefix_HydratesPartialServiceActorWithHyphenatedUsername(t *testing.T) {
	mockDB := new(mocks.MockDB)
	indexQuery := new(mocks.MockQuery)
	fullActorQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(indexQuery).Once()
	indexQuery.On("Index", "gsi1").Return(indexQuery)
	indexQuery.On("Where", "gsi1PK", "=", "USERNAME_SEARCH#ag").Return(indexQuery)
	indexQuery.On("Where", "gsi1SK", "BEGINS_WITH", "ag").Return(indexQuery)
	indexQuery.On("Limit", 10).Return(indexQuery)
	indexQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{{
			PK:       "ACTOR#agent-0",
			Username: "agent-0",
			GSI1SK:   "agent-0",
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{Type: activitypub.ServiceType},
				PreferredUsername: "agent-0",
				Name:              "Agent 0",
			},
		}}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(fullActorQuery).Once()
	fullActorQuery.On("Where", "PK", "=", "ACTOR#agent-0").Return(fullActorQuery)
	fullActorQuery.On("Where", "SK", "=", "PROFILE").Return(fullActorQuery)
	fullActorQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/agent-0", Type: activitypub.ServiceType},
			PreferredUsername: "agent-0",
			Name:              "Agent 0",
		}
	}).Return(nil)

	repo.searchUsernamePrefix(ctx, "ag", 10, 0, &results, seen)

	assert.Len(t, results, 1)
	assert.Equal(t, "agent-0", results[0].PreferredUsername)
	assert.Equal(t, activitypub.ServiceType, results[0].Type)
	assert.Equal(t, "https://example.com/users/agent-0", results[0].ID)
}

func TestSearchDisplayName_HydratesPartialServiceActorFromIndexedMatch(t *testing.T) {
	mockDB := new(mocks.MockDB)
	indexQuery := new(mocks.MockQuery)
	fullActorQuery := new(mocks.MockQuery)

	repo := &SearchRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.SearchSuggestion](
			mockDB, "test-table", nil, nil, "SearchRepository", "search",
		),
	}

	ctx := context.Background()
	results := make([]*activitypub.Actor, 0)
	seen := make(map[string]bool)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(indexQuery).Once()
	indexQuery.On("Index", "gsi2").Return(indexQuery)
	indexQuery.On("Where", "gsi2PK", "=", "NAME_SEARCH#pi").Return(indexQuery)
	indexQuery.On("Where", "gsi2SK", "BEGINS_WITH", "pi").Return(indexQuery)
	indexQuery.On("Limit", 10).Return(indexQuery)
	indexQuery.On("All", mock.AnythingOfType("*[]models.Actor")).Run(func(args mock.Arguments) {
		matches := args.Get(0).(*[]models.Actor)
		*matches = []models.Actor{{
			PK:       "ACTOR#pilot",
			Username: "pilot",
			GSI2SK:   "pilot#pilot",
			Actor: &activitypub.Actor{
				BaseObject:        activitypub.BaseObject{Type: activitypub.ServiceType},
				PreferredUsername: "pilot",
				Name:              "Pilot",
			},
		}}
	}).Return(nil)

	mockDB.On("Model", mock.AnythingOfType("*models.Actor")).Return(fullActorQuery).Once()
	fullActorQuery.On("Where", "PK", "=", "ACTOR#pilot").Return(fullActorQuery)
	fullActorQuery.On("Where", "SK", "=", "PROFILE").Return(fullActorQuery)
	fullActorQuery.On("First", mock.AnythingOfType("*models.Actor")).Run(func(args mock.Arguments) {
		actor := args.Get(0).(*models.Actor)
		actor.Actor = &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/pilot", Type: activitypub.ServiceType},
			PreferredUsername: "pilot",
			Name:              "Pilot",
		}
	}).Return(nil)

	repo.searchDisplayName(ctx, "pi", 10, &results, seen)

	assert.Len(t, results, 1)
	assert.Equal(t, "pilot", results[0].PreferredUsername)
	assert.Equal(t, activitypub.ServiceType, results[0].Type)
	assert.Equal(t, "https://example.com/users/pilot", results[0].ID)
}
