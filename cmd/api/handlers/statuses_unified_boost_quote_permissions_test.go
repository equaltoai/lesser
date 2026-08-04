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

	newHarness := func(
		t *testing.T,
		permissions *storagemodels.QuotePermissions,
		permissionErr error,
		configure func(*round10QueryState, *storagemodels.Status),
	) (*Handler, *quotes.QuoteService, *round10QueryState) {
		t.Helper()
		targetForHarness := *target
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"mallory": {Username: "mallory", Actor: &activitypub.Actor{
					BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("mallory"), Type: "Person"},
					PreferredUsername: "mallory",
					Followers:         cfg.ActorURL("mallory") + "/followers",
				}},
			},
			statusByID: map[string]storagemodels.Status{"status-1": targetForHarness},
			objectsByID: map[string]storagemodels.Object{
				targetID: {
					ID:           targetID,
					Type:         activitypub.NoteType,
					Content:      targetForHarness.Content,
					AttributedTo: targetForHarness.AuthorID,
					Published:    targetForHarness.PublishedAt,
				},
			},
		}
		if configure != nil {
			configure(state, &targetForHarness)
			state.statusByID["status-1"] = targetForHarness
		}
		if permissionErr != nil {
			state.firstErrorPK = map[string]error{"USER#alice": permissionErr}
		}

		registry := &RegistryStub{NotesSvc: &NotesServiceStub{
			ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawQuoteTarget string) (*storagemodels.Status, error) {
				require.Equal(t, "mallory", viewerID)
				require.Equal(t, "status-1", rawQuoteTarget)
				return &targetForHarness, nil
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

		return handler, quoteService, state
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
	assertDeniedParity := func(t *testing.T, handler *Handler, quoteService *quotes.QuoteService) {
		t.Helper()
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
	}

	t.Run("blocked viewer receives the GraphQL denial class", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{
			AllowPublic: true,
			BlockList:   []string{"mallory"},
		}, nil, nil)

		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("followers-only target is rejected before any write", func(t *testing.T) {
		handler, _, state := newHarness(t, nil, nil,
			func(state *round10QueryState, status *storagemodels.Status) {
				status.Visibility = storagemodels.VisibilityPrivate
				state.createErrorOnce = stdErrors.New("quote boost persistence must not run")
			})

		result := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusUnprocessableEntity, result.status)
		require.Empty(t, state.quoteRelationships, "denial must not persist a quote relationship")
		require.Empty(t, state.activitiesByID, "denial must not persist a federation activity")
		require.Len(t, state.objectsByID, 1, "denial must not persist a quote note object")
	})

	t.Run("follower arm allows both REST and GraphQL quote creation", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowFollowers: true}, nil,
			func(state *round10QueryState, _ *storagemodels.Status) {
				state.relationshipRecords = []storagemodels.RelationshipRecord{
					{PK: "FOLLOW#mallory", SK: "FOLLOWING#alice", State: storagemodels.RelationshipAccepted},
				}
			})
		_, err := quoteService.AttachQuoteToStatus(context.Background(), &storagemodels.Status{
			StatusID: "mallory-follower-quote", AuthorUsername: "mallory",
		}, "status-1")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, requestRESTQuote(t, handler).status)
	})

	t.Run("follower arm denies non-followers identically", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowFollowers: true}, nil, nil)
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("follower lookup error fails closed as forbidden on both surfaces", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowFollowers: true}, nil,
			func(state *round10QueryState, _ *storagemodels.Status) {
				state.firstErrorPK = map[string]error{"FOLLOW#mallory": stdErrors.New("relationship store unavailable")}
			})
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("mentioned arm allows both REST and GraphQL quote creation", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowMentioned: true}, nil,
			func(_ *round10QueryState, status *storagemodels.Status) {
				status.Mentions = []string{cfg.ActorURL("mallory")}
			})
		_, err := quoteService.AttachQuoteToStatus(context.Background(), &storagemodels.Status{
			StatusID: "mallory-mentioned-quote", AuthorUsername: "mallory",
		}, "status-1")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, requestRESTQuote(t, handler).status)
	})

	t.Run("mentioned arm denies unmentioned quoters identically", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowMentioned: true}, nil, nil)
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("mentioned status read error fails closed as REST forbidden", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowMentioned: true}, nil,
			func(state *round10QueryState, _ *storagemodels.Status) {
				state.firstErrorPK = map[string]error{"status#status-1": stdErrors.New("status store unavailable")}
			})
		allowed, err := quoteService.CheckQuotePermissions(context.Background(), "mallory", target)
		require.NoError(t, err)
		require.False(t, allowed)

		graphQLAppErr, ok := commonerrors.AsAppError(quotes.ErrNotAuthorizedToQuote)
		require.True(t, ok)
		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusForbidden, rest.status)
		require.Equal(t, graphQLAppErr.Message, rest.body.Error)
		require.Equal(t, string(graphQLAppErr.Code), rest.body.Code)
	})

	t.Run("per-note none denies both surfaces after account allow", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil,
			func(state *round10QueryState, _ *storagemodels.Status) {
				state.statusMetadataByStatus = map[string]storagemodels.StatusMetadata{
					"status-1": {StatusID: "status-1", QuoteType: "disabled", AllowQuotes: false},
				}
			})
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("per-note followers allows follower and denies non-follower on both surfaces", func(t *testing.T) {
		permissions := &storagemodels.QuotePermissions{AllowPublic: true}
		withFollowersControl := func(follows bool) func(*round10QueryState, *storagemodels.Status) {
			return func(state *round10QueryState, _ *storagemodels.Status) {
				state.statusMetadataByStatus = map[string]storagemodels.StatusMetadata{
					"status-1": {StatusID: "status-1", QuoteType: "followers", AllowQuotes: true},
				}
				if follows {
					state.relationshipRecords = []storagemodels.RelationshipRecord{
						{PK: "FOLLOW#mallory", SK: "FOLLOWING#alice", State: storagemodels.RelationshipAccepted},
					}
				}
			}
		}

		handler, quoteService, _ := newHarness(t, permissions, nil, withFollowersControl(true))
		_, err := quoteService.AttachQuoteToStatus(context.Background(), &storagemodels.Status{
			StatusID: "mallory-note-follower-quote", AuthorUsername: "mallory",
		}, "status-1")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, requestRESTQuote(t, handler).status)

		handler, quoteService, _ = newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil, withFollowersControl(false))
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("per-note mentioned allows mentioned and denies unmentioned on both surfaces", func(t *testing.T) {
		withMentionedControl := func(mentioned bool) func(*round10QueryState, *storagemodels.Status) {
			return func(state *round10QueryState, status *storagemodels.Status) {
				state.statusMetadataByStatus = map[string]storagemodels.StatusMetadata{
					"status-1": {StatusID: "status-1", QuoteType: "mentioned", AllowQuotes: true},
				}
				if mentioned {
					status.Mentions = []string{cfg.ActorURL("mallory")}
				}
			}
		}

		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil, withMentionedControl(true))
		_, err := quoteService.AttachQuoteToStatus(context.Background(), &storagemodels.Status{
			StatusID: "mallory-note-mentioned-quote", AuthorUsername: "mallory",
		}, "status-1")
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, requestRESTQuote(t, handler).status)

		handler, quoteService, _ = newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil, withMentionedControl(false))
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("per-note storage error fails closed as forbidden on both surfaces", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil,
			func(state *round10QueryState, _ *storagemodels.Status) {
				state.firstErrorPK = map[string]error{"STATUS_META#status-1": stdErrors.New("metadata unavailable")}
			})
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("per-note public cannot widen account denial", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{}, nil,
			func(state *round10QueryState, _ *storagemodels.Status) {
				state.statusMetadataByStatus = map[string]storagemodels.StatusMetadata{
					"status-1": {StatusID: "status-1", QuoteType: storagemodels.VisibilityPublic, AllowQuotes: true},
				}
			})
		assertDeniedParity(t, handler, quoteService)
	})

	t.Run("allowed viewer creates the REST quote unchanged", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, &storagemodels.QuotePermissions{AllowPublic: true}, nil, nil)
		allowed, err := quoteService.CheckQuotePermissions(context.Background(), "mallory", target)
		require.NoError(t, err)
		require.True(t, allowed)
		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusOK, rest.status)
		require.Nil(t, rest.body)
	})

	t.Run("missing permissions row uses the GraphQL permissive default", func(t *testing.T) {
		handler, quoteService, _ := newHarness(t, nil, nil, nil)
		allowed, err := quoteService.CheckQuotePermissions(context.Background(), "mallory", target)
		require.NoError(t, err)
		require.True(t, allowed)
		rest := requestRESTQuote(t, handler)
		require.Equal(t, http.StatusOK, rest.status)
		require.Nil(t, rest.body)
	})

	t.Run("storage failure is fail closed on both surfaces", func(t *testing.T) {
		storageErr := stdErrors.New("quote permissions unavailable")
		handler, quoteService, _ := newHarness(t, nil, storageErr, nil)

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
		AuthorID:       "https://example.com/users/alice",
		AuthorUsername: "alice",
		Content:        "original",
		Visibility:     storagemodels.VisibilityPublic,
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject:   activitypub.BaseObject{ID: objectID, Type: activitypub.NoteType},
			AttributedTo: "https://example.com/users/alice",
			Content:      "original",
			Visibility:   storagemodels.VisibilityPublic,
		},
	}
}
