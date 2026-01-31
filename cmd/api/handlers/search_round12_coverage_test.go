package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type round12RemoteSearchStub struct {
	results []*federation.SearchResult
	err     error
}

func (s *round12RemoteSearchStub) SearchRemoteActors(ctx context.Context, query string, limit int) ([]*federation.SearchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}

func TestSearch_Round12Coverage(t *testing.T) {
	now := time.Now().UTC()

	state := &round10QueryState{
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			},
			"bob": {
				Username: "bob",
				Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/bob", Type: "Person"},
					PreferredUsername: "bob",
					Name:              "Bob",
				},
			},
		},
		objectList: []storagemodels.Object{
			{ID: "obj-1", Type: activitypub.NoteType, Content: "hello search", Published: now, AttributedTo: "https://example.com/users/alice"},
		},
	}

	t.Run("addRemoteSearchResults branches", func(t *testing.T) {
		cfg := round10TestConfig()
		handler, _, _ := round11NewHandler(t, cfg, state)

		t.Run("invalid handle returns early", func(t *testing.T) {
			actors := []*activitypub.Actor{{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}}
			handler.addRemoteSearchResults(context.Background(), &actors, "alice", 10)
			require.Len(t, actors, 1)
		})

		t.Run("nil service returns early", func(t *testing.T) {
			handler.remoteSearch = func(store core.RepositoryStorage) remoteSearchService {
				return nil
			}
			actors := []*activitypub.Actor{}
			handler.addRemoteSearchResults(context.Background(), &actors, "@user@example.com", 10)
			require.Empty(t, actors)
		})

		t.Run("remote error returns early", func(t *testing.T) {
			handler.remoteSearch = func(store core.RepositoryStorage) remoteSearchService {
				return &round12RemoteSearchStub{err: errors.New("boom")}
			}
			actors := []*activitypub.Actor{}
			handler.addRemoteSearchResults(context.Background(), &actors, "@user@example.com", 10)
			require.Empty(t, actors)
		})

		t.Run("remote results append non-nil actors", func(t *testing.T) {
			handler.remoteSearch = func(store core.RepositoryStorage) remoteSearchService {
				return &round12RemoteSearchStub{
					results: []*federation.SearchResult{
						{Actor: nil},
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/remote"}, PreferredUsername: "remote"}},
					},
				}
			}
			actors := []*activitypub.Actor{}
			handler.addRemoteSearchResults(context.Background(), &actors, "@user@example.com", 10)
			require.Len(t, actors, 1)
		})

		require.True(t, isValidHandle("@user@example.com"))
		require.True(t, isValidHandle("@user"))
		require.False(t, isValidHandle("nope"))
	})

	t.Run("account search validation and auth branches", func(t *testing.T) {
		cfg := round10TestConfig()
		handler, _, _ := round11NewHandler(t, cfg, state)
		handler.registry = &RegistryStub{}

		t.Run("missing query is validation error", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleAccountSearchLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("following filter requires auth", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", nil, map[string]string{"q": "alice", "following": "true"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleAccountSearchLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("optional auth ignores invalid header", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", map[string]string{"Authorization": "Bearer invalid"}, map[string]string{"q": "alice"}, nil)
			require.NoError(t, err)
			require.Equal(t, "", handler.authenticateFromSearchHeader(ctx, false))
		})

		t.Run("authenticated search uses privacy-aware path", func(t *testing.T) {
			readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
			headers := map[string]string{"Authorization": "Bearer " + readToken}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", headers, map[string]string{"q": "alice", "offset": "-1", "limit": "nope"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleAccountSearchLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("valid offset is applied", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", nil, map[string]string{"q": "alice", "offset": "5"}, nil)
			require.NoError(t, err)
			params, err := handler.parseAccountSearchParams(ctx)
			require.NoError(t, err)
			require.NotNil(t, params)
			require.Equal(t, 5, params.offset)
		})

		t.Run("resolve adds remote results and analytics errors are non-fatal", func(t *testing.T) {
			state := &round10QueryState{
				actorsByUser:    state.actorsByUser,
				objectList:      state.objectList,
				createErrorOnce: errors.New("boom"),
			}
			handler, _, _ := round11NewHandler(t, cfg, state)
			handler.remoteSearch = func(store core.RepositoryStorage) remoteSearchService {
				return &round12RemoteSearchStub{
					results: []*federation.SearchResult{
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://remote.example/users/remote"}, PreferredUsername: "remote"}},
					},
				}
			}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search", nil, map[string]string{"q": "@user@example.com", "resolve": "true"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleAccountSearchLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})
	})

	t.Run("search suggestions branches", func(t *testing.T) {
		cfg := round10TestConfig()
		handler, _, _ := round11NewHandler(t, cfg, state)

		t.Run("short prefix returns empty", func(t *testing.T) {
			handler.registry = &RegistryStub{}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search/suggestions", nil, map[string]string{"q": "h"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleGetSearchSuggestionsLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("service error returns 500", func(t *testing.T) {
			handler.registry = &RegistryStub{
				NotesSvc: &NotesServiceStub{
					GetSearchSuggestionsFunc: func(ctx context.Context, query *notes.GetSearchSuggestionsQuery) (*notes.GetSearchSuggestionsResult, error) {
						return nil, errors.New("boom")
					},
				},
			}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search/suggestions", nil, map[string]string{"q": "he"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleGetSearchSuggestionsLift(ctx))
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("success converts results", func(t *testing.T) {
			handler.registry = &RegistryStub{
				NotesSvc: &NotesServiceStub{
					GetSearchSuggestionsFunc: func(ctx context.Context, query *notes.GetSearchSuggestionsQuery) (*notes.GetSearchSuggestionsResult, error) {
						return &notes.GetSearchSuggestionsResult{
							Suggestions: []*storage.SearchSuggestion{{Type: "hashtag", Value: "#hello", Score: 0.5}},
						}, nil
					},
				},
			}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/search/suggestions", nil, map[string]string{"q": "he"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleGetSearchSuggestionsLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})
	})

	t.Run("status search auth and error branches", func(t *testing.T) {
		cfg := round10TestConfig()

		t.Run("missing auth is unauthorized", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", nil, map[string]string{"q": "hello"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleStatusSearchLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("invalid auth header is unauthorized", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", map[string]string{"Authorization": "nope"}, map[string]string{"q": "hello"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleStatusSearchLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("missing query is validation error before auth", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleStatusSearchLift(ctx))
			require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
		})

		t.Run("invalid token is unauthorized", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", map[string]string{"Authorization": "Bearer invalid"}, map[string]string{"q": "hello"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleStatusSearchLift(ctx))
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("token without read scope is insufficient scope", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, state)
			token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", map[string]string{"Authorization": "Bearer " + token}, map[string]string{"q": "hello"}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleStatusSearchLift(ctx))
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("success parses params and records analytics", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
				actorsByUser:    state.actorsByUser,
				objectList:      state.objectList,
				createErrorOnce: errors.New("boom"),
			})

			token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
			headers := map[string]string{"Authorization": "Bearer " + token}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/search/statuses", headers, map[string]string{
				"q":          "hello",
				"limit":      "100",
				"max_id":     "m1",
				"min_id":     "m0",
				"account_id": "acct-1",
				"local":      "true",
			}, nil)
			require.NoError(t, err)
			require.NoError(t, handler.HandleStatusSearchLift(ctx))
			require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		})

		t.Run("performStatusSearch handles empty queries", func(t *testing.T) {
			handler, _, _ := round11NewHandler(t, cfg, state)
			statuses, err := handler.performStatusSearch(context.Background(), &statusSearchParams{query: "", limit: 20}, "alice")
			require.NoError(t, err)
			require.Empty(t, statuses)
		})
	})
}
