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
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

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

func TestMiscRound22_SearchStatusByURLIgnoresObjectOnlyArtifacts(t *testing.T) {
	cfg := round11TestConfig()
	statusURL := "https://remote.example/objects/search-only"

	repos := &MockRepositoryStorage{}
	repos.On("Status").Return(nil).Maybe()

	handler := &Handler{cfg: cfg, repos: repos}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, nil, nil)
	require.NoError(t, err)

	result := apimodels.SearchResult{Statuses: []apimodels.Status{}}
	handler.searchStatusByURL(ctx, statusURL, "", &result)

	require.Empty(t, result.Statuses)
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

func TestMiscRound22_ResolveStatusBySearchURLUsesCanonicalRemoteStatusID(t *testing.T) {
	cfg := round11TestConfig()
	remoteURL := "https://remote.example/users/bob/statuses/abc-123"
	statusID := storagemodels.CanonicalStatusID(remoteURL)

	state := &round10QueryState{
		notFoundPKs: map[string]bool{
			"status#abc-123": true,
		},
		statusByID: map[string]storagemodels.Status{
			statusID: {
				StatusID:       statusID,
				AuthorID:       "https://remote.example/users/bob",
				AuthorUsername: "bob@remote.example",
				Content:        "remote canonical status",
				Visibility:     storagemodels.VisibilityPublic,
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	status, err := handler.resolveStatusBySearchURL(context.Background(), remoteURL)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, statusID, status.StatusID)
}

func TestMiscRound22_SearchHelperPureFunctions(t *testing.T) {
	handler := &Handler{}
	remoteURL := "https://remote.example/users/bob/statuses/abc-123/"
	canonicalRemoteID := storagemodels.CanonicalStatusID(remoteURL)

	require.Nil(t, handler.statusLookupCandidates("   "))
	require.Equal(t, []string{"abc-123", canonicalRemoteID}, handler.statusLookupCandidates(remoteURL))
	require.Equal(t, []string{"status-1"}, handler.statusLookupCandidates(" status-1 "))

	require.Equal(t, "", deriveSearchStatusID(""))
	require.Equal(t, "status-1", deriveSearchStatusID("status-1"))
	require.Equal(t, "abc-123", deriveSearchStatusID(remoteURL))
	require.Equal(t, "statuses", deriveSearchStatusID("https://remote.example/users/bob/statuses/"))

	require.True(t, shouldAugmentStatusSearchByAuthor("simulacrum"))
	require.False(t, shouldAugmentStatusSearchByAuthor(""))
	require.False(t, shouldAugmentStatusSearchByAuthor("two words"))
	require.False(t, shouldAugmentStatusSearchByAuthor("#simulacrum"))
	require.False(t, shouldAugmentStatusSearchByAuthor("https://remote.example/@simulacrum"))

	require.False(t, statusVisibleInSearch(nil, ""))
	require.True(t, statusVisibleInSearch(&storagemodels.Status{Visibility: storagemodels.VisibilityPublic}, ""))
	require.True(t, statusVisibleInSearch(&storagemodels.Status{Visibility: storagemodels.VisibilityUnlisted}, ""))
	require.True(t, statusVisibleInSearch(&storagemodels.Status{
		Visibility:     storagemodels.VisibilityPrivate,
		AuthorUsername: "Simulacrum",
	}, "simulacrum"))
	require.False(t, statusVisibleInSearch(&storagemodels.Status{
		Visibility:     storagemodels.VisibilityPrivate,
		AuthorUsername: "simulacrum",
	}, "other"))
}

func TestMiscRound22_ResolveStatusFromSearchResultHandlesFallbacks(t *testing.T) {
	_, err := (*Handler)(nil).resolveStatusFromSearchResult(context.Background(), nil)
	require.EqualError(t, err, "status repository unavailable")

	cfg := round11TestConfig()
	remoteURL := "https://remote.example/users/bob/statuses/abc-123"
	canonicalID := storagemodels.CanonicalStatusID(remoteURL)
	state := &round10QueryState{
		notFoundPKs: map[string]bool{
			"status#abc-123": true,
		},
		statusByID: map[string]storagemodels.Status{
			canonicalID: {
				StatusID:       canonicalID,
				AuthorID:       "https://remote.example/users/bob",
				AuthorUsername: "bob@remote.example",
				Content:        "remote canonical status",
				Visibility:     storagemodels.VisibilityPublic,
				URLs:           []string{remoteURL},
				Note: &activitypub.Note{
					BaseObject: activitypub.BaseObject{ID: remoteURL, Type: activitypub.NoteType},
					Content:    "remote canonical status",
				},
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	status, err := handler.resolveStatusFromSearchResult(context.Background(), &storage.StatusSearchResult{
		StatusID: remoteURL,
		URL:      remoteURL,
	})
	require.NoError(t, err)
	require.Equal(t, canonicalID, status.StatusID)
}

func TestMiscRound22_ExecuteHashtagSearchAddsPlaceholderWhenNoMatches(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, nil, nil)
	require.NoError(t, err)

	result := apimodels.SearchResult{Hashtags: []apimodels.Tag{}}
	handler.executeHashtagSearch(ctx, &SearchParams{Query: "#lesser", Limit: 5}, &result)

	require.Len(t, result.Hashtags, 1)
	require.Equal(t, "lesser", result.Hashtags[0].Name)
	require.Contains(t, result.Hashtags[0].URL, "/tags/lesser")
}

func TestMiscRound22_SearchStatusByContentAugmentsAuthorTimeline(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)
	authorID := cfg.ActorURL("simulacrum")
	statusID := "timeline-status-2"

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
	handler.searchStatusByContent(ctx, &SearchParams{Query: "simulacrum", Limit: 2}, "", &result)

	require.NotEmpty(t, result.Statuses)
	require.Equal(t, statusID, result.Statuses[0].ID)
}
