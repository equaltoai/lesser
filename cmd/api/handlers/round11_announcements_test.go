package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAnnouncements_ListDismissAndReactions(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"bob":   {PK: "USER#bob", SK: storagemodels.SKMetadata, Username: "bob", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
		statusByID: map[string]storagemodels.Status{
			"123": {StatusID: "123", AuthorUsername: "bob", Content: "hello", PublishedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour)},
		},
		announcementByID: map[string]storagemodels.Announcement{
			"a1": {ID: "a1", Content: "Hello @bob #Go :party: https://example.com/statuses/123", Text: "Hello", PublishedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), AllDay: false},
			"a2": {ID: "a2", Content: "Other announcement", Text: "Other", PublishedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour), AllDay: false},
		},
		announcementDismissalsByID: map[string][]storagemodels.AnnouncementDismissal{
			"alice": {{Username: "alice", AnnouncementID: "a2", DismissedAt: now.Add(-1 * time.Hour)}},
		},
		announcementReactionsByID: map[string][]storagemodels.AnnouncementReaction{
			"a1": {{AnnouncementID: "a1", Username: "alice", EmojiName: ":party:", ReactedAt: now.Add(-30 * time.Minute)}},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/announcements", headers, nil, nil)
	require.NoError(t, err)
	resp := requireStatus(t, http.StatusOK)(handler.HandleGetAnnouncementsLift(ctx))
	var body []apimodels.Announcement
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Len(t, body, 1)
	require.Equal(t, "a1", body[0].ID)
	require.NotEmpty(t, body[0].Tags)
	require.NotEmpty(t, body[0].Mentions)
	require.NotEmpty(t, body[0].Statuses)
	require.NotEmpty(t, body[0].Emojis)

	ctxDismiss, err := round10NewLiftContext(http.MethodPost, "/api/v1/announcements/a1/dismiss", headers, nil, nil)
	require.NoError(t, err)
	ctxDismiss.Params["id"] = "a1"
	requireStatus(t, http.StatusOK)(handler.HandleDismissAnnouncementLift(ctxDismiss))

	ctxReact, err := round10NewLiftContext(http.MethodPut, "/api/v1/announcements/a1/reactions/party", headers, nil, nil)
	require.NoError(t, err)
	ctxReact.Params["id"] = "a1"
	ctxReact.Params["name"] = ":party:"
	requireStatus(t, http.StatusOK)(handler.HandleAddAnnouncementReactionLift(ctxReact))
}

func TestAnnouncements_CreateAnnouncement(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/admin/announcements", headers, nil, []byte(`{"text":`))
	requireStatus(t, http.StatusBadRequest)(handler.HandleCreateAnnouncementLift(ctxBad))

	body := apimodels.CreateAnnouncementRequest{Text: "hello", StartsAt: "2024-01-02T15:04:05Z", EndsAt: "2024-01-03T15:04:05Z", AllDay: true}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/announcements", headers, nil, body)
	require.NoError(t, err)
	requireStatus(t, http.StatusCreated)(handler.HandleCreateAnnouncementLift(ctx))
}
