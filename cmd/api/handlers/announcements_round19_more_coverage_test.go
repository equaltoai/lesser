package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAnnouncementsRound19_HandleGetAnnouncementsLift_ErrorBranches(t *testing.T) {
	t.Run("repository errors return 500", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{
			allErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/announcements", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleGetAnnouncementsLift(ctx))
	})

	t.Run("dismissal lookup errors do not block listing", func(t *testing.T) {
		cfg := round11TestConfig()
		now := time.Now()

		state := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]*models.AnnouncementDismissal": errors.New("boom"),
			},
			announcementByID: map[string]storagemodels.Announcement{
				"a1": {ID: "a1", Content: "Hello", Text: "Hello", PublishedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), AllDay: false},
				"a2": {ID: "a2", Content: "Other", Text: "Other", PublishedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), AllDay: false},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/announcements", headers, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAnnouncementsLift(ctx))
		var out []apimodels.Announcement
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 2)
	})
}

func TestAnnouncementsRound19_HandleGetAnnouncementsLift_AnonymousOptionalDatesAndReactionMerge(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	startsAt := now.Add(-1 * time.Hour)
	endsAt := now.Add(1 * time.Hour)

	state := &round10QueryState{
		announcementByID: map[string]storagemodels.Announcement{
			"a1": {
				ID:          "a1",
				Content:     "Hello :party:",
				Text:        "Hello",
				PublishedAt: now.Add(-3 * time.Hour),
				UpdatedAt:   now.Add(-2 * time.Hour),
				AllDay:      false,
				StartsAt:    &startsAt,
				EndsAt:      &endsAt,
				Reactions: []storagemodels.Reaction{
					{Name: ":party:", Count: 0, Me: false},
				},
			},
		},
		announcementReactionsByID: map[string][]storagemodels.AnnouncementReaction{
			"a1": {{AnnouncementID: "a1", Username: "bob", EmojiName: ":party:", ReactedAt: now.Add(-30 * time.Minute)}},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/announcements", nil, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(h.HandleGetAnnouncementsLift(ctx))
	var out []apimodels.Announcement
	require.NoError(t, json.Unmarshal(resp.Body, &out))
	require.Len(t, out, 1)
	require.NotNil(t, out[0].StartsAt)
	require.NotNil(t, out[0].EndsAt)

	found := false
	for _, reaction := range out[0].Reactions {
		if reaction.Name == ":party:" {
			found = true
			require.Equal(t, 1, reaction.Count)
			require.False(t, reaction.Me)
		}
	}
	require.True(t, found, "expected merged :party: reaction to be present")
}

func TestAnnouncementsRound19_HandleDismissAnnouncementLift_ErrorBranches(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	t.Run("missing announcement ID returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/announcements//dismiss", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleDismissAnnouncementLift(ctx))
	})

	t.Run("missing token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/announcements/a1/dismiss", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "a1"

		requireStatus(t, http.StatusUnauthorized)(h.HandleDismissAnnouncementLift(ctx))
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		headers := map[string]string{"Authorization": "Bearer not-a-token"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/announcements/a1/dismiss", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "a1"

		requireStatus(t, http.StatusUnauthorized)(h.HandleDismissAnnouncementLift(ctx))
	})

	t.Run("announcement not found returns 404", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"ANNOUNCEMENT#missing#ANNOUNCEMENT": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/announcements/missing/dismiss", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"

		requireStatus(t, http.StatusNotFound)(h.HandleDismissAnnouncementLift(ctx))
	})

	t.Run("dismissal store errors return 500", func(t *testing.T) {
		state := &round10QueryState{
			announcementByID: map[string]storagemodels.Announcement{
				"a1": {ID: "a1", Content: "Hello", Text: "Hello", PublishedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), AllDay: false},
			},
			createErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/announcements/a1/dismiss", headers, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "a1"

		requireStatus(t, http.StatusInternalServerError)(h.HandleDismissAnnouncementLift(ctx))
	})
}

func TestAnnouncementsRound19_HandleAnnouncementReaction_RemoveErrorsReturn500(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	state := &round10QueryState{
		announcementByID: map[string]storagemodels.Announcement{
			"a1": {ID: "a1", Content: "Hello", Text: "Hello", PublishedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), AllDay: false},
		},
		deleteErrorOnce: errors.New("boom"),
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/announcements/a1/reactions/party", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "a1"
	ctx.Params["name"] = ":party:"

	requireStatus(t, http.StatusInternalServerError)(h.HandleRemoveAnnouncementReactionLift(ctx))
}

func TestAnnouncementsRound19_CreateAnnouncement_ErrorBranches(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	t.Run("missing text returns 422", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/announcements", nil, nil, apimodels.CreateAnnouncementRequest{})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateAnnouncementLift(ctx))
	})

	t.Run("non-admin returns 403", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/announcements", headers, nil, apimodels.CreateAnnouncementRequest{Text: "hello"})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleCreateAnnouncementLift(ctx))
	})

	t.Run("invalid starts_at and ends_at return 422", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/announcements", headers, nil, apimodels.CreateAnnouncementRequest{Text: "hello", StartsAt: "nope"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateAnnouncementLift(ctx))

		ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/admin/announcements", headers, nil, apimodels.CreateAnnouncementRequest{Text: "hello", EndsAt: "nope"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateAnnouncementLift(ctx))
	})

	t.Run("create announcement errors return 500", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
			},
			createErrorOnce: errors.New("boom"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/announcements", headers, nil, apimodels.CreateAnnouncementRequest{Text: "hello"})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleCreateAnnouncementLift(ctx))
	})
}

func TestAnnouncementsRound19_ExtractMentionsAndStatuses_NotFoundBranches(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		notFoundPKSK: map[string]bool{
			"USER#missing#METADATA":           true,
			"status#missing-status#status#missing-status": true,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	mentions := h.extractAnnouncementMentions(context.Background(), "Hello @missing!")
	require.Empty(t, mentions)

	statuses := h.extractAnnouncementStatuses(context.Background(), "see https://example.com/statuses/missing-status")
	require.Empty(t, statuses)
}
