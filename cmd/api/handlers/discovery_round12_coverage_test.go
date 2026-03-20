package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/search"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryRound12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	published := now.Add(-24 * time.Hour)

	actorWithPublishedAndIcon := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:        "acct:alice",
			Type:      actorTypeService,
			Published: &published,
		},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
		Icon:              &activitypub.Image{URL: "https://example.com/avatar.png"},
	}

	t.Run("directory: search service nil", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: nil})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/directory", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetDirectoryLift(ctx))
	})

	t.Run("directory: service error", func(t *testing.T) {
		searchStub := &SearchServiceStub{
			GetDirectoryFunc: func(_ context.Context, _ *search.DirectoryQuery) (*search.DirectoryResult, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: searchStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/directory", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetDirectoryLift(ctx))
	})

	t.Run("directory: query fallback + parse defaults", func(t *testing.T) {
		searchStub := &SearchServiceStub{
			GetDirectoryFunc: func(_ context.Context, query *search.DirectoryQuery) (*search.DirectoryResult, error) {
				require.True(t, query.Local)
				require.Equal(t, 2, query.Limit)
				require.Equal(t, 10, query.Offset)
				require.Equal(t, "", query.Order)

				return &search.DirectoryResult{
					Accounts: []search.AccountResult{
						{
							Actor:          actorWithPublishedAndIcon,
							FollowersCount: 1,
							FollowingCount: 2,
							StatusesCount:  3,
							LastStatusAt:   now.Format(time.RFC3339),
						},
					},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: searchStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/directory?limit=2&offset=10&local=true", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(h.HandleGetDirectoryLift(ctx))

		// cover extractFromPathQuery edge cases
		require.Equal(t, "", h.extractFromPathQuery("/api/v1/directory", "limit"))
		require.Equal(t, "", h.extractFromPathQuery("/api/v1/directory?foo=bar", "limit"))

		// cover Parse* error defaults (limit=bad, offset=bad)
		ctxBad, err := round10NewLiftContext(http.MethodGet, "/api/v1/directory?limit=bad&offset=bad", nil, nil, nil)
		require.NoError(t, err)
		params := h.parseDirectoryParams(ctxBad)
		require.Equal(t, 40, params.limit)
		require.Equal(t, 0, params.offset)

		// cover getAccountAcctLift fallback when no scheme
		require.Equal(t, "alice", h.getAccountAcctLift(actorWithPublishedAndIcon))
	})

	t.Run("suggestions v1: auth + errors", func(t *testing.T) {
		searchStub := &SearchServiceStub{
			GetSuggestionsFunc: func(_ context.Context, _ *search.SuggestionsQuery) (*search.SuggestionsResult, error) {
				return nil, errors.New("boom")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: searchStub})

		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/suggestions", nil, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusUnauthorized)(h.HandleGetSuggestionsV1Lift(ctxMissing))
		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_token", body["error"])

		ctxInvalid, err := round10NewLiftContext(http.MethodGet, "/api/v1/suggestions", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
		require.NoError(t, err)
		resp = requireStatus(t, http.StatusUnauthorized)(h.HandleGetSuggestionsV1Lift(ctxInvalid))
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_token", body["error"])

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxNoSvc, err := round10NewLiftContext(http.MethodGet, "/api/v1/suggestions", headers, nil, nil)
		require.NoError(t, err)
		hNoSvc, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: nil})
		requireStatus(t, http.StatusInternalServerError)(hNoSvc.HandleGetSuggestionsV1Lift(ctxNoSvc))

		ctxErr, err := round10NewLiftContext(http.MethodGet, "/api/v1/suggestions", headers, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleGetSuggestionsV1Lift(ctxErr))
	})

	t.Run("suggestions v2: success + service nil", func(t *testing.T) {
		searchStub := &SearchServiceStub{
			GetSuggestionsFunc: func(_ context.Context, query *search.SuggestionsQuery) (*search.SuggestionsResult, error) {
				require.Equal(t, 2, query.Version)
				return &search.SuggestionsResult{
					Suggestions: []search.SuggestionItem{
						{
							Account: search.AccountResult{Actor: actorWithPublishedAndIcon},
							Source:  "staff",
						},
					},
				}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: searchStub})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxOK, err := round10NewLiftContext(http.MethodGet, "/api/v2/suggestions", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleGetSuggestionsV2Lift(ctxOK))

		ctxNoSvc, err := round10NewLiftContext(http.MethodGet, "/api/v2/suggestions", headers, nil, nil)
		require.NoError(t, err)
		hNoSvc, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: nil})
		requireStatus(t, http.StatusInternalServerError)(hNoSvc.HandleGetSuggestionsV2Lift(ctxNoSvc))
	})

	t.Run("remove suggestion: path fallback + errors", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctxMissing, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions/bob", nil, nil, nil)
		require.NoError(t, err)
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: &SearchServiceStub{}})
		resp := requireStatus(t, http.StatusUnauthorized)(h.HandleRemoveSuggestionLift(ctxMissing))
		var body map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_token", body["error"])

		ctxInvalid, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions/bob", map[string]string{"Authorization": "Bearer invalid"}, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(h.HandleRemoveSuggestionLift(ctxInvalid))

		ctxBadPath, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(h.HandleRemoveSuggestionLift(ctxBadPath))

		hNoSvc, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: nil})
		ctxNoSvc, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions/bob", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(hNoSvc.HandleRemoveSuggestionLift(ctxNoSvc))

		searchStub := &SearchServiceStub{
			RemoveSuggestionFunc: func(_ context.Context, _ *search.RemoveSuggestionCommand) error {
				return errors.New("boom")
			},
		}
		hErr, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: searchStub})
		ctxErr, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions/bob", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(hErr.HandleRemoveSuggestionLift(ctxErr))

		searchOK := &SearchServiceStub{
			RemoveSuggestionFunc: func(_ context.Context, cmd *search.RemoveSuggestionCommand) error {
				require.Equal(t, "alice", cmd.Username)
				require.Equal(t, "bob", cmd.AccountID)
				return nil
			},
		}
		hOK, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{SearchSvc: searchOK})
		ctxOK, err := round10NewLiftContext(http.MethodDelete, "/api/v1/suggestions/bob", headers, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(hOK.HandleRemoveSuggestionLift(ctxOK))
	})
}
