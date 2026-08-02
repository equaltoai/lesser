package handlers

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/quotes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRESTQuoteBoostMatchesGraphQLAccountPermissionEnforcement(t *testing.T) {
	cfg := round11TestConfig()
	targetID := cfg.BaseURL() + "/objects/status-1"
	target := quotePermissionTarget("status-1", targetID)
	comment := "permission-checked quote"

	newHarness := func(t *testing.T, permissions *storagemodels.QuotePermissions, permissionErr error) (*Handler, *quotes.QuoteService) {
		t.Helper()
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"mallory": {Username: "mallory", Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("mallory"), Type: "Person"},
					PreferredUsername: "mallory",
					Followers:         cfg.ActorURL("mallory") + "/followers",
				}},
			},
			statusByID: map[string]storagemodels.Status{"status-1": *target},
			objectsByID: map[string]storagemodels.Object{
				targetID: {
					ID:           targetID,
					Type:         activitypub.NoteType,
					Content:      target.Content,
					AttributedTo: target.AuthorID,
					Published:    target.PublishedAt,
				},
			},
		}
		if permissionErr != nil {
			state.firstErrorPK = map[string]error{"USER#alice": permissionErr}
		}

		registry := &RegistryStub{NotesSvc: &NotesServiceStub{
			ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawQuoteTarget string) (*storagemodels.Status, error) {
				require.Equal(t, "mallory", viewerID)
				require.Equal(t, "status-1", rawQuoteTarget)
				return target, nil
			},
		}}
		handler, repos, _ := round11NewHandler(t, cfg, state, registry)
		quoteService, ok := registry.QuotesSvc.(*quotes.QuoteService)
		require.True(t, ok)

		if permissions != nil {
			permissions.Username = "alice"
			require.NoError(t, permissions.UpdateKeys())
			// This is the same QuoteRepository persistence seam used by updateAccountQuotePermissions.
			require.NoError(t, repos.Quote().CreateQuotePermissions(context.Background(), permissions))
		}

		return handler, quoteService
	}

	type restQuoteResult struct {
		status int
		body   *common.StandardErrorResponse
	}
	requestRESTQuote := func(t *testing.T, handler *Handler) restQuoteResult {
		t.Helper()
		token := round11SignAccessToken(t, cfg.JWTSecret, "mallory", []string{"write"})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/status-1/reblog",
			map[string]string{"Authorization": "Bearer " + token}, nil, models.ReblogRequest{Comment: &comment})
		require.NoError(t, err)
		ctx.Params["id"] = "status-1"
		resp, err := handler.HandleReblogLift(ctx)
		require.NoError(t, err)
		if resp.Status == http.StatusOK {
			return restQuoteResult{status: resp.Status}
		}
		var body common.StandardErrorResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		return restQuoteResult{status: resp.Status, body: &body}
	}

	t.Run("blocked viewer receives the GraphQL denial class", func(t *testing.T) {
		handler, quoteService := newHarness(t, &storagemodels.QuotePermissions{
			AllowPublic: true,
			BlockList:   []string{"mallory"},
		}, nil)

		_, graphQLErr := quoteService.AttachQuoteToStatus(context.Background(), &storagemodels.Status{
			StatusID:       "mallory-quote",
			AuthorUsername: "mallory",
		}, "status-1")
		require.ErrorIs(t, graphQLErr, quotes.ErrNotAuthorizedToQuote)
		graphQLAppErr, ok := commonerrors.AsAppError(graphQLErr)
		require.True(t, ok)

		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusForbidden, rest.status)
		require.NotNil(t, rest.body)
		require.Equal(t, graphQLAppErr.Message, rest.body.Error)
		require.Equal(t, string(graphQLAppErr.Code), rest.body.Code)
	})

	t.Run("allowed viewer creates the REST quote unchanged", func(t *testing.T) {
		handler, quoteService := newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil)
		allowed, err := quoteService.CheckQuotePermissions(context.Background(), "mallory", target)
		require.NoError(t, err)
		require.True(t, allowed)
		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusOK, rest.status)
		require.Nil(t, rest.body)
	})

	t.Run("missing permissions row uses the GraphQL permissive default", func(t *testing.T) {
		handler, quoteService := newHarness(t, nil, nil)
		allowed, err := quoteService.CheckQuotePermissions(context.Background(), "mallory", target)
		require.NoError(t, err)
		require.True(t, allowed)
		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusOK, rest.status)
		require.Nil(t, rest.body)
	})

	t.Run("storage failure is fail closed on both surfaces", func(t *testing.T) {
		storageErr := stdErrors.New("quote permissions unavailable")
		handler, quoteService := newHarness(t, nil, storageErr)

		_, graphQLErr := quoteService.AttachQuoteToStatus(context.Background(), &storagemodels.Status{
			StatusID:       "mallory-quote",
			AuthorUsername: "mallory",
		}, "status-1")
		require.Error(t, graphQLErr)
		graphQLAppErr, ok := commonerrors.AsAppError(graphQLErr)
		require.True(t, ok)

		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusInternalServerError, rest.status)
		require.NotNil(t, rest.body)
		require.Equal(t, graphQLAppErr.Message, rest.body.Error)
		require.Equal(t, string(graphQLAppErr.Code), rest.body.Code)
	})
}

func quotePermissionTarget(statusID, objectID string) *storagemodels.Status {
	now := time.Now().UTC()
	return &storagemodels.Status{
		StatusID:       statusID,
		AuthorID:       "alice",
		AuthorUsername: "alice",
		Content:        "original",
		Visibility:     storagemodels.VisibilityPublic,
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: objectID, Type: activitypub.NoteType},
			AttributedTo: "alice",
			Content:      "original",
			Visibility:   storagemodels.VisibilityPublic,
		},
	}
}
