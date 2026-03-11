package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestMiscRound21_HandleSearchLift_UsesStorageBackedStatusSerialization(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)

	authorID := cfg.ActorURL("simulacrum")
	statusID := "3f796ab5-9242-412f-80ef-473a6672c1e8"
	statusURL := authorID + "/statuses/" + statusID

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"simulacrum": {
				Username:     "simulacrum",
				DisplayName:  "simulacrum",
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
					Name:              "simulacrum",
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
				Content:        "trying to make contact",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
				LikeCount:      5,
				ReblogCount:    1,
				Note: &activitypub.Note{
					BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType},
					Content:      "trying to make contact",
					AttributedTo: authorID,
				},
			},
		},
		statusList: []storagemodels.Status{{
			StatusID:       statusID,
			AuthorID:       authorID,
			AuthorUsername: "simulacrum",
			Content:        "trying to make contact",
			Visibility:     storagemodels.VisibilityPublic,
			PublishedAt:    now,
			CreatedAt:      now,
			UpdatedAt:      now,
			LikeCount:      5,
			ReblogCount:    1,
			Note: &activitypub.Note{
				BaseObject:   activitypub.BaseObject{ID: statusURL, Type: activitypub.NoteType},
				Content:      "trying to make contact",
				AttributedTo: authorID,
			},
		}},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{
		"q":    "trying to make contact",
		"type": "statuses",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleSearchV2Lift(ctx))

	var result apimodels.SearchResult
	require.NoError(t, json.Unmarshal(resp.Body, &result))
	require.Len(t, result.Statuses, 1)
	require.Equal(t, statusID, result.Statuses[0].ID)
	require.Equal(t, storagemodels.VisibilityPublic, result.Statuses[0].Visibility)
	require.Equal(t, cfg.BaseURL()+"/@simulacrum/"+statusID, result.Statuses[0].URL)
	require.Equal(t, statusURL, result.Statuses[0].URI)
	require.Equal(t, common.GenerateNumericID("simulacrum"), result.Statuses[0].Account.ID)
	require.Equal(t, "simulacrum", result.Statuses[0].Account.Username)
	require.Equal(t, 5, result.Statuses[0].FavouritesCount)
	require.Equal(t, 1, result.Statuses[0].ReblogsCount)
}

func TestMiscRound21_HandleSearchLift_SupplementsAuthorQueriesWithStatuses(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Date(2026, 3, 11, 12, 10, 19, 0, time.UTC)

	authorID := cfg.ActorURL("simulacrum")
	statusID := "3f796ab5-9242-412f-80ef-473a6672c1e8"

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"simulacrum": {
				Username:     "simulacrum",
				DisplayName:  "simulacrum",
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
					Name:              "simulacrum",
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
				Content:        "trying to make contact",
				Visibility:     storagemodels.VisibilityPublic,
				PublishedAt:    now,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		statusList: []storagemodels.Status{{
			StatusID:       statusID,
			AuthorID:       authorID,
			AuthorUsername: "simulacrum",
			Content:        "trying to make contact",
			Visibility:     storagemodels.VisibilityPublic,
			PublishedAt:    now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v2/search", nil, map[string]string{
		"q":    "simulacrum",
		"type": "statuses",
	}, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.HandleSearchV2Lift(ctx))

	var result apimodels.SearchResult
	require.NoError(t, json.Unmarshal(resp.Body, &result))
	require.Len(t, result.Statuses, 1)
	require.Equal(t, statusID, result.Statuses[0].ID)
	require.Equal(t, "simulacrum", result.Statuses[0].Account.Username)
}
