package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type miscRound22ObjectRepo struct {
	interfaces.ObjectRepository
	object any
}

func (r *miscRound22ObjectRepo) GetObject(_ context.Context, _ string) (any, error) {
	return r.object, nil
}

func TestMiscRound22_ConvertThinStatusResultToAPIAddsResolvedAccount(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)
	authorID := cfg.ActorURL("simulacrum")

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"simulacrum": {
				Username:     "simulacrum",
				DisplayName:  "Simulacrum",
				CreatedAt:    now.Add(-time.Hour),
				UpdatedAt:    now.Add(-time.Hour),
				Discoverable: true,
				Role:         "user",
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"simulacrum": {
				Username: "simulacrum",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: authorID, Type: "Service"},
					PreferredUsername: "simulacrum",
					Name:              "Simulacrum",
					Discoverable:      true,
				},
				NumericID: common.GenerateNumericID("simulacrum"),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, nil, nil)
	require.NoError(t, err)

	got := handler.convertThinStatusResultToAPI(ctx, &storage.StatusSearchResult{
		StatusID:  "status-1",
		Content:   "hello from search",
		URL:       cfg.BaseURL() + "/@simulacrum/status-1",
		AuthorID:  authorID,
		Published: now,
	})

	require.Equal(t, "status-1", got.ID)
	require.Equal(t, "hello from search", got.Content)
	require.Equal(t, "simulacrum", got.Account.Username)
	require.Equal(t, common.GenerateNumericID("simulacrum"), got.Account.ID)
	require.Nil(t, handler.getActorFromAuthorID(ctx, ""))
}

func TestSearchRound22_ConvertStatusSearchResultsHydratesFullStatus(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)
	authorID := cfg.ActorURL("simulacrum")
	statusID := "3f796ab5-9242-412f-80ef-473a6672c1e8"
	statusURL := authorID + "/statuses/" + statusID

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"simulacrum": {
				Username:     "simulacrum",
				DisplayName:  "Simulacrum",
				CreatedAt:    now.Add(-24 * time.Hour),
				UpdatedAt:    now.Add(-24 * time.Hour),
				Discoverable: true,
				Role:         "user",
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"simulacrum": {
				Username: "simulacrum",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: authorID, Type: "Service"},
					PreferredUsername: "simulacrum",
					Name:              "Simulacrum",
					Discoverable:      true,
				},
				NumericID: common.GenerateNumericID("simulacrum"),
			},
		},
		statusByID: map[string]storagemodels.Status{
			statusID: {
				StatusID:       statusID,
				AuthorID:       authorID,
				AuthorUsername: "simulacrum",
				Content:        "hydrated content",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
				LikeCount:      7,
				ReblogCount:    2,
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType},
					Content:      "hydrated content",
					AttributedTo: authorID,
				},
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	results := handler.convertStatusSearchResults(context.Background(), []storage.StatusSearchResult{{
		StatusID:       statusID,
		URL:            statusURL,
		Content:        "thin content",
		AuthorID:       authorID,
		AuthorUsername: "simulacrum",
		Published:      now,
		Score:          0.75,
		Highlights:     []string{"hydrated"},
	}}, "")

	require.Len(t, results, 1)
	require.Equal(t, statusID, results[0].ID)
	require.Equal(t, "hydrated content", results[0].Content)
	require.Equal(t, cfg.BaseURL()+"/@simulacrum/"+statusID, results[0].URL)
	require.Equal(t, common.GenerateNumericID("simulacrum"), results[0].AccountID)
	require.Equal(t, "simulacrum", results[0].AccountUsername)
	require.Equal(t, []string{"hydrated"}, results[0].Highlights)
	require.Equal(t, 0.75, results[0].Score)
}

func TestMiscRound22_SearchStatusByURLFallsBackToObjectLookup(t *testing.T) {
	cfg := round11TestConfig()
	statusURL := "https://remote.example/objects/search-only"

	repos := &MockRepositoryStorage{}
	repos.On("Status").Return(nil).Maybe()
	repos.On("Object").Return(&miscRound22ObjectRepo{
		object: &activitypub.Note{
			BaseObject: activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType},
			Content:    "fallback object content",
		},
	}).Once()

	handler := &Handler{cfg: cfg, repos: repos}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, nil, nil)
	require.NoError(t, err)

	result := apimodels.SearchResult{Statuses: []apimodels.Status{}}
	handler.searchStatusByURL(ctx, statusURL, "", &result)

	require.Len(t, result.Statuses, 1)
	require.Equal(t, statusURL, result.Statuses[0].ID)
	require.Equal(t, "fallback object content", result.Statuses[0].Content)
	repos.AssertExpectations(t)
}

func TestMiscRound22_AddAuthorMatchedStatusesAppendsVisibleTimelineStatuses(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)
	authorID := cfg.ActorURL("simulacrum")
	statusID := "timeline-status-1"

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"simulacrum": {
				Username:     "simulacrum",
				DisplayName:  "Simulacrum",
				CreatedAt:    now.Add(-24 * time.Hour),
				UpdatedAt:    now.Add(-24 * time.Hour),
				Discoverable: true,
				Role:         "user",
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"simulacrum": {
				Username: "simulacrum",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: authorID, Type: "Service"},
					PreferredUsername: "simulacrum",
					Name:              "Simulacrum",
					Discoverable:      true,
				},
				NumericID: common.GenerateNumericID("simulacrum"),
			},
		},
		statusByID: map[string]storagemodels.Status{
			statusID: {
				StatusID:       statusID,
				AuthorID:       authorID,
				AuthorUsername: "simulacrum",
				Content:        "timeline fallback content",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: authorID + "/statuses/" + statusID, Type: activitypub.NoteType},
					Content:      "timeline fallback content",
					AttributedTo: authorID,
				},
			},
		},
		statusList: []storagemodels.Status{{
			StatusID:       statusID,
			AuthorID:       authorID,
			AuthorUsername: "simulacrum",
			Content:        "timeline fallback content",
			Visibility:     storagemodels.VisibilityPublic,
			PublishedAt:    now,
			CreatedAt:      now,
			UpdatedAt:      now,
			Note: &activitypub.Note{
				BaseObject:   activitypub.BaseObject{ID: authorID + "/statuses/" + statusID, Type: activitypub.NoteType},
				Content:      "timeline fallback content",
				AttributedTo: authorID,
			},
		}},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, nil, nil)
	require.NoError(t, err)

	result := apimodels.SearchResult{Statuses: []apimodels.Status{}}
	seen := map[string]struct{}{}
	handler.addAuthorMatchedStatuses(ctx, &SearchParams{Query: "simulacrum", Limit: 2}, "", seen, &result)

	require.Len(t, result.Statuses, 1)
	require.Equal(t, statusID, result.Statuses[0].ID)
	require.Contains(t, seen, statusID)
	status := state.statusByID[statusID]
	require.True(t, statusVisibleInSearch(&status, ""))
}

func TestMiscRound22_SearchStatusByURLUsesStoredStatusWhenAvailable(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)
	authorID := cfg.ActorURL("simulacrum")
	statusID := "stored-status-1"
	statusURL := authorID + "/statuses/" + statusID

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"simulacrum": {
				Username:     "simulacrum",
				DisplayName:  "Simulacrum",
				CreatedAt:    now.Add(-24 * time.Hour),
				UpdatedAt:    now.Add(-24 * time.Hour),
				Discoverable: true,
				Role:         "user",
			},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"simulacrum": {
				Username: "simulacrum",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: authorID, Type: "Service"},
					PreferredUsername: "simulacrum",
					Name:              "Simulacrum",
					Discoverable:      true,
				},
				NumericID: common.GenerateNumericID("simulacrum"),
			},
		},
		statusByID: map[string]storagemodels.Status{
			statusID: {
				StatusID:       statusID,
				AuthorID:       authorID,
				AuthorUsername: "simulacrum",
				Content:        "stored status content",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType},
					Content:      "stored status content",
					AttributedTo: authorID,
				},
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, nil, nil)
	require.NoError(t, err)

	result := apimodels.SearchResult{Statuses: []apimodels.Status{}}
	handler.searchStatusByURL(ctx, statusURL, "", &result)

	require.Len(t, result.Statuses, 1)
	require.Equal(t, statusID, result.Statuses[0].ID)
	require.Equal(t, "stored status content", result.Statuses[0].Content)
	require.Equal(t, cfg.BaseURL()+"/@simulacrum/"+statusID, result.Statuses[0].URL)
}
