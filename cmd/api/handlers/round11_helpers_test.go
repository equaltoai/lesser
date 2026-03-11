package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	storage "github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func round11SignAccessToken(t *testing.T, secret, username string, scopes []string) string {
	t.Helper()

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
		},
		Username: username,
		ClientID: "test-client",
		Scopes:   scopes,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestHelperParsingAndAuth(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/alice",
						Type: "Person",
					},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext("GET", "/test", headers, map[string]string{"flag": "true"}, nil)
	require.NoError(t, err)

	username, err := handler.authenticateUser(ctx, []string{auth.ScopeWrite})
	require.NoError(t, err)
	require.Equal(t, "alice", username)

	ctxBool, err := round10NewLiftContext("GET", "/test", nil, map[string]string{"flag": "true"}, nil)
	require.NoError(t, err)
	require.True(t, handler.parseBoolParam(ctxBool, "flag"))

	ctxArray, err := round10NewLiftContext("GET", "/test", nil, map[string]string{"id[0]": "a", "id[1]": "b"}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a", "b"}, handler.parseArrayParam(ctxArray, "id"))

	params := &PaginationParams{Limit: 10, MaxID: "max", MinID: "min"}
	linkHeader := buildLinkHeader("/items", params, true, true)
	require.Contains(t, linkHeader, "max_id")
	require.Contains(t, linkHeader, "min_id")
}

func TestHelperErrorsAndAuthService(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusBadRequest)(handler.respondBadRequest(ctx, "bad"))

	ctxForbidden, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(handler.respondForbidden(ctxForbidden, ""))

	ctxAuthErr, err := round10NewLiftContext("GET", "/test", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusForbidden)(handler.handleAuthServiceError(ctxAuthErr, auth.ErrUserSuspended, "login"))

	body := []byte(`{invalid}`)
	ctxBody := round10NewLiftContextWithBodyBytes("POST", "/parse", nil, nil, body)
	var payload map[string]string
	err = handler.parseRequestBody(ctxBody, &payload)
	require.Error(t, err)

	ctxLimit, err := round10NewLiftContext("GET", "/limit", nil, map[string]string{"limit": "bad"}, nil)
	require.NoError(t, err)
	require.Equal(t, 7, handler.parseLimitParam(ctxLimit, 7, 10))
}

func TestResolveAccountID(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/alice",
						Type: "Person",
					},
					PreferredUsername: "alice",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	actor, err := handler.resolveAccountID(context.Background(), "https://example.com/users/alice")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/users/alice", actor.ID)

	actor, err = handler.resolveAccountID(context.Background(), "@alice@example.com")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/users/alice", actor.ID)

	_, err = handler.resolveAccountID(context.Background(), "http://")
	require.Error(t, err)
}

func TestConvertStorageStatusToAPI(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	statusNote := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:   cfg.BaseURL() + "/objects/s1",
			Type: activitypub.NoteType,
			To:   []string{activitypub.PublicAddress},
		},
		AttributedTo: cfg.ActorURL("alice"),
		Content:      "hello",
		Attachment: []activitypub.Attachment{
			{URL: "https://example.com/media/1", MediaType: "image/png", Name: "pic"},
		},
		Tag: []activitypub.Tag{{Type: "Hashtag", Name: "#Go"}},
	}

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
			"bob":   {PK: "USER#bob", SK: storagemodels.SKMetadata, Username: "bob", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   cfg.ActorURL("alice"),
						Type: "Person",
					},
					PreferredUsername: "alice",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
			"bob": {
				Username: "bob",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   cfg.ActorURL("bob"),
						Type: "Person",
					},
					PreferredUsername: "bob",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
		statusByID: map[string]storagemodels.Status{
			"parent": {
				StatusID:       "parent",
				AuthorUsername: "bob",
				AuthorID:       cfg.ActorURL("bob"),
				PublishedAt:    now.Add(-2 * time.Hour),
				CreatedAt:      now.Add(-2 * time.Hour),
			},
			"reblog": {
				StatusID:       "reblog",
				AuthorUsername: "bob",
				AuthorID:       cfg.ActorURL("bob"),
				PublishedAt:    now.Add(-3 * time.Hour),
				CreatedAt:      now.Add(-3 * time.Hour),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	status := &storagemodels.Status{
		StatusID:       "s1",
		AuthorUsername: "alice",
		AuthorID:       cfg.ActorURL("alice"),
		Content:        "hello",
		InReplyToID:    "parent",
		PublishedAt:    now.Add(-1 * time.Hour),
		CreatedAt:      now.Add(-1 * time.Hour),
		UpdatedAt:      now.Add(-30 * time.Minute),
		Hashtags:       []string{"Go"},
		Mentions:       []string{"bob"},
		Note:           statusNote,
		ReblogOfID:     "reblog",
		LikeCount:      4,
		ReblogCount:    3,
	}

	apiStatus, err := handler.convertStorageStatusToAPI(status, "alice")
	require.NoError(t, err)
	require.True(t, strings.Contains(apiStatus.URI, "/statuses/s1"))
	require.True(t, strings.Contains(apiStatus.URL, "/@alice/s1"))
	require.NotNil(t, apiStatus.InReplyToID)
	require.Len(t, apiStatus.MediaAttachments, 1)
	require.Len(t, apiStatus.Tags, 1)
	require.NotNil(t, apiStatus.Reblog)
	require.Equal(t, 4, apiStatus.FavouritesCount)
	require.Equal(t, 3, apiStatus.ReblogsCount)

	serialized, err := json.Marshal(apiStatus)
	require.NoError(t, err)
	require.Contains(t, string(serialized), "s1")

	// cover tag extraction helpers via map input
	tags := handler.extractHashtagsFromObject(map[string]any{
		"tag": []any{
			map[string]any{"type": "Hashtag", "name": "#GoLang"},
		},
	})
	require.Equal(t, []string{"golang"}, tags)
}

func TestHelperResolveAccountAndAction(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	ctx, err := round10NewLiftContext("POST", "/status", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "1"

	resp := requireStatus(t, http.StatusOK)(handler.statusActionHandler(ctx, auth.ScopeRead, func(_, _ string) (*apimodels.Status, error) {
		return &apimodels.Status{ID: "1"}, nil
	}))

	var body apimodels.Status
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "1", body.ID)
}

func TestHelperResponseHelpers(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx, err := round10NewLiftContext("GET", "/resp", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusUnauthorized)(handler.respondUnauthorized(ctx))

	ctxInternal, err := round10NewLiftContext("GET", "/resp", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusInternalServerError)(handler.respondInternalError(ctxInternal, ""))

	ctxUnprocessable, err := round10NewLiftContext("GET", "/resp", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusUnprocessableEntity)(handler.respondUnprocessableEntity(ctxUnprocessable, ""))

	ctxNotFound, err := round10NewLiftContext("GET", "/resp", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusNotFound)(handler.respondNotFound(ctxNotFound, "thing"))

	ctxOK, err := round10NewLiftContext("GET", "/resp", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.respondOK(ctxOK, storage.Account{User: &storage.User{Username: "alice"}}))
}
