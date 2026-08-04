package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/quotes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	testinginmemory "github.com/equaltoai/lesser/pkg/testing/inmemory"
	testingmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestQuotePostRESTCreateRoundTrip(t *testing.T) {
	cfg := round11TestConfig()
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
	headers := map[string]string{"Authorization": "Bearer " + writeToken}
	target := &storagemodels.Status{
		StatusID:       "target-1",
		AuthorID:       cfg.ActorURL("bob"),
		AuthorUsername: "bob",
		Visibility:     storagemodels.VisibilityPublic,
	}
	created := &storagemodels.Status{
		StatusID:       "quote-1",
		AuthorID:       cfg.ActorURL("alice"),
		AuthorUsername: "alice",
		Content:        "Quote text",
		Visibility:     storagemodels.VisibilityPublic,
		CreatedAt:      time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC),
	}

	t.Run("happy path uses notes creation and quote attachment", func(t *testing.T) {
		var createCalls, attachCalls int
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(_ context.Context, viewerID, rawTarget string) (*storagemodels.Status, error) {
					require.Equal(t, "alice", viewerID)
					require.Equal(t, "target-1", rawTarget)
					return target, nil
				},
				CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					createCalls++
					require.Equal(t, "alice", cmd.AuthorID)
					require.Equal(t, "Quote text", cmd.Content)
					require.Equal(t, storagemodels.VisibilityPublic, cmd.Visibility)
					return &notes.NoteResult{Note: created}, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{
				CheckQuotePermissionsFunc: func(_ context.Context, quoter string, gotTarget *storagemodels.Status) (bool, error) {
					require.Equal(t, "alice", quoter)
					require.Same(t, target, gotTarget)
					return true, nil
				},
				AttachQuoteToStatusFunc: func(_ context.Context, quoteStatus *storagemodels.Status, targetID string) (*quotes.QuotePostResult, error) {
					attachCalls++
					require.Same(t, created, quoteStatus)
					require.Equal(t, "target-1", targetID)
					return &quotes.QuotePostResult{QuoteStatus: created, TargetStatus: target}, nil
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "Quote text"})
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"

		resp := requireStatus(t, http.StatusOK)(handler.HandleCreateQuotePostLift(ctx))
		require.Equal(t, 1, createCalls)
		require.Equal(t, 1, attachCalls)
		var body apimodels.QuoteStatusSummary
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "quote-1", body.ID)
		require.Equal(t, "Quote text", body.Content)
		require.Equal(t, cfg.ActorURL("alice"), body.Account.ID)
	})

	t.Run("missing and invisible targets have one 404 shape and no write", func(t *testing.T) {
		var expectedBody []byte
		for _, test := range []struct {
			name          string
			seedInvisible bool
		}{
			{name: "missing"},
			{name: "invisible", seedInvisible: true},
		} {
			t.Run(test.name, func(t *testing.T) {
				statusRepo := testinginmemory.NewStatusRepository()
				relationshipRepo := testingmocks.NewMockRelationshipRepository()
				if test.seedInvisible {
					now := time.Now().UTC()
					relationshipRepo.On("IsFollowing", mock.Anything, "alice", "bob").Return(false, nil).Once()
					require.NoError(t, statusRepo.CreateStatus(context.Background(), &storagemodels.Status{
						StatusID:       "target-1",
						AuthorID:       cfg.ActorURL("bob"),
						AuthorUsername: "bob",
						Visibility:     storagemodels.VisibilityPrivate,
						ToRecipients:   []string{cfg.ActorURL("bob") + "/followers"},
						PublishedAt:    now,
						CreatedAt:      now,
						UpdatedAt:      now,
						ModifiedAt:     now,
						Note: &activitypub.Note{
							BaseObject: activitypub.BaseObject{
								ID:   cfg.ActorURL("bob") + "/statuses/target-1",
								Type: activitypub.NoteType,
							},
							AttributedTo: cfg.ActorURL("bob"),
							Visibility:   storagemodels.VisibilityPrivate,
						},
					}))
				}
				notesService := notes.NewService(
					statusRepo,
					nil,
					nil,
					relationshipRepo,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					nil,
					zap.NewNop(),
					cfg.Domain,
				)
				reg := &RegistryStub{
					NotesSvc:  notesService,
					QuotesSvc: &QuotesServiceStub{},
				}
				handler, _, _ := round11NewHandler(t, cfg, reg)
				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "Quote text"})
				require.NoError(t, err)
				ctx.Params["id"] = "target-1"

				resp := requireStatus(t, http.StatusNotFound)(handler.HandleCreateQuotePostLift(ctx))
				if expectedBody == nil {
					expectedBody = append([]byte(nil), resp.Body...)
				} else {
					require.Equal(t, expectedBody, resp.Body)
				}
				createdCount, countErr := statusRepo.CountStatusesByAuthor(context.Background(), "alice")
				require.NoError(t, countErr)
				require.Zero(t, createdCount)
				relationshipRepo.AssertExpectations(t)
			})
		}
	})

	t.Run("quoteability denial happens before note creation", func(t *testing.T) {
		createCalls := 0
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(context.Context, string, string) (*storagemodels.Status, error) { return target, nil },
				CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					createCalls++
					return nil, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{
				CheckQuotePermissionsFunc: func(context.Context, string, *storagemodels.Status) (bool, error) { return false, nil },
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "Quote text"})
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"
		requireStatus(t, http.StatusForbidden)(handler.HandleCreateQuotePostLift(ctx))
		require.Zero(t, createCalls)
	})

	t.Run("non-public target fails with 422 before note creation", func(t *testing.T) {
		createCalls := 0
		permissionCalls := 0
		followersOnlyTarget := *target
		followersOnlyTarget.Visibility = storagemodels.VisibilityPrivate
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(context.Context, string, string) (*storagemodels.Status, error) {
					return &followersOnlyTarget, nil
				},
				CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					createCalls++
					return nil, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{
				CheckQuotePermissionsFunc: func(context.Context, string, *storagemodels.Status) (bool, error) {
					permissionCalls++
					return true, nil
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", headers, nil, apimodels.CreateQuotePostRequest{
			Status:     "Quote text",
			Visibility: storagemodels.VisibilityPrivate,
		})
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"

		resp := requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleCreateQuotePostLift(ctx))
		require.Contains(t, string(resp.Body), string(commonerrors.CodeUnprocessableEntity))
		require.Zero(t, permissionCalls)
		require.Zero(t, createCalls)
	})

	t.Run("quoteability storage errors fail closed before note creation", func(t *testing.T) {
		createCalls := 0
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(context.Context, string, string) (*storagemodels.Status, error) { return target, nil },
				CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					createCalls++
					return nil, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{
				CheckQuotePermissionsFunc: func(context.Context, string, *storagemodels.Status) (bool, error) {
					return false, commonerrors.Internal("permission storage failed")
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "Quote text"})
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateQuotePostLift(ctx))
		require.Zero(t, createCalls)
	})

	t.Run("target storage errors fail closed before note creation", func(t *testing.T) {
		createCalls := 0
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(context.Context, string, string) (*storagemodels.Status, error) {
					return nil, commonerrors.Internal("target storage failed")
				},
				CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					createCalls++
					return nil, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{},
		}
		handler, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", headers, nil, apimodels.CreateQuotePostRequest{Status: "Quote text"})
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateQuotePostLift(ctx))
		require.Zero(t, createCalls)
	})
}

func TestQuotePostRESTListRoundTrip(t *testing.T) {
	cfg := round11TestConfig()
	target := &storagemodels.Status{StatusID: "target-1", Visibility: storagemodels.VisibilityPublic}
	visibleOne := &storagemodels.Status{StatusID: "quote-1", AuthorID: cfg.ActorURL("alice"), AuthorUsername: "alice", Content: "one", CreatedAt: time.Now()}
	visibleTwo := &storagemodels.Status{StatusID: "quote-2", AuthorID: cfg.ActorURL("carol"), AuthorUsername: "carol", Content: "two", CreatedAt: time.Now()}

	reg := &RegistryStub{
		NotesSvc: &NotesServiceStub{
			GetNoteWithViewerFunc: func(_ context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
				switch query.StatusID {
				case "target-1":
					return target, nil
				case "quote-hidden":
					return nil, notes.ErrStatusNotFound
				case "quote-1":
					return visibleOne, nil
				case "quote-2":
					return visibleTwo, nil
				default:
					return nil, notes.ErrStatusNotFound
				}
			},
		},
		QuotesSvc: &QuotesServiceStub{
			GetQuoteRelationshipsForStatusFunc: func(_ context.Context, statusID string, limit int, cursor string) (*quotes.QuoteRelationshipPage, error) {
				require.Equal(t, "target-1", statusID)
				require.Equal(t, quoteListPageSize, limit)
				require.Empty(t, cursor)
				return &quotes.QuoteRelationshipPage{Relationships: []*storagemodels.QuoteRelationship{
					{QuoterNoteID: "quote-hidden"},
					{QuoterNoteID: "quote-1"},
					{QuoterNoteID: "quote-2"},
				}}, nil
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, reg)
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, map[string]string{"limit": "1", "offset": "1"}, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "target-1"

	resp := requireStatus(t, http.StatusOK)(handler.HandleGetQuotesOfStatusLift(ctx))
	var body []apimodels.QuoteStatusSummary
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Len(t, body, 1)
	require.Equal(t, "quote-2", body[0].ID)

	t.Run("offset beyond the servable scan bound is rejected", func(t *testing.T) {
		boundedCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, map[string]string{"limit": "1", "offset": "4"}, nil)
		require.NoError(t, err)
		boundedCtx.Params["id"] = "target-1"

		boundedResponse := requireStatus(t, http.StatusUnprocessableEntity)(handler.HandleGetQuotesOfStatusLift(boundedCtx))
		var errorBody common.StandardErrorResponse
		require.NoError(t, json.Unmarshal(boundedResponse.Body, &errorBody))
		require.Contains(t, errorBody.Error, "3")
		require.Equal(t, string(commonerrors.CodeUnprocessableEntity), errorBody.Code)
	})

	t.Run("missing and invisible targets have one 404 shape", func(t *testing.T) {
		var expectedBody []byte
		for _, name := range []string{"missing", "invisible"} {
			t.Run(name, func(t *testing.T) {
				listCalls := 0
				deniedRegistry := &RegistryStub{
					NotesSvc: &NotesServiceStub{
						GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
							return nil, notes.ErrStatusNotFound
						},
					},
					QuotesSvc: &QuotesServiceStub{
						GetQuoteRelationshipsForStatusFunc: func(context.Context, string, int, string) (*quotes.QuoteRelationshipPage, error) {
							listCalls++
							return nil, nil
						},
					},
				}
				deniedHandler, _, _ := round11NewHandler(t, cfg, deniedRegistry)
				deniedCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, nil, nil)
				require.NoError(t, err)
				deniedCtx.Params["id"] = "target-1"
				deniedResponse := requireStatus(t, http.StatusNotFound)(deniedHandler.HandleGetQuotesOfStatusLift(deniedCtx))
				require.Zero(t, listCalls)
				if expectedBody == nil {
					expectedBody = append([]byte(nil), deniedResponse.Body...)
				} else {
					require.Equal(t, expectedBody, deniedResponse.Body)
				}
			})
		}
	})

	t.Run("scan stops at bounded multiple when all quotes are hidden", func(t *testing.T) {
		const requestedLimit = 2
		quoteReads := 0
		listCalls := 0
		boundedRegistry := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
					if query.StatusID == "target-1" {
						return target, nil
					}
					quoteReads++
					return nil, notes.ErrStatusNotFound
				},
			},
			QuotesSvc: &QuotesServiceStub{
				GetQuoteRelationshipsForStatusFunc: func(_ context.Context, statusID string, limit int, cursor string) (*quotes.QuoteRelationshipPage, error) {
					listCalls++
					require.Equal(t, "target-1", statusID)
					require.Equal(t, quoteListPageSize, limit)
					require.Empty(t, cursor)
					relationships := make([]*storagemodels.QuoteRelationship, requestedLimit*quoteListScanMultiplier)
					for i := range relationships {
						relationships[i] = &storagemodels.QuoteRelationship{QuoterNoteID: fmt.Sprintf("hidden-%d", i)}
					}
					return &quotes.QuoteRelationshipPage{Relationships: relationships, NextCursor: "more-relationships-exist"}, nil
				},
			},
		}
		boundedHandler, _, _ := round11NewHandler(t, cfg, boundedRegistry)
		boundedCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, map[string]string{"limit": "2"}, nil)
		require.NoError(t, err)
		boundedCtx.Params["id"] = "target-1"

		boundedResponse := requireStatus(t, http.StatusOK)(boundedHandler.HandleGetQuotesOfStatusLift(boundedCtx))
		var boundedBody []apimodels.QuoteStatusSummary
		require.NoError(t, json.Unmarshal(boundedResponse.Body, &boundedBody))
		require.Empty(t, boundedBody)
		require.Equal(t, requestedLimit*quoteListScanMultiplier, quoteReads)
		require.Equal(t, 1, listCalls)
	})

	t.Run("withdrawn relationship pages terminate at the fetch cap", func(t *testing.T) {
		const requestedLimit = 80
		fetchCap := (requestedLimit*quoteListScanMultiplier + quoteListPageSize - 1) / quoteListPageSize
		listCalls := 0
		withdrawnRegistry := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteWithViewerFunc: func(_ context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
					require.Empty(t, query.ViewerID, "the termination path must remain safe for unauthenticated callers")
					if query.StatusID == "target-1" {
						return target, nil
					}
					t.Fatalf("withdrawn relationships must not reach visibility projection: %s", query.StatusID)
					return nil, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{
				GetQuoteRelationshipsForStatusFunc: func(_ context.Context, statusID string, limit int, cursor string) (*quotes.QuoteRelationshipPage, error) {
					listCalls++
					require.Equal(t, "target-1", statusID)
					require.Equal(t, quoteListPageSize, limit)
					if listCalls == 1 {
						require.Empty(t, cursor)
					} else {
						require.Equal(t, fmt.Sprintf("withdrawn-page-%d", listCalls-1), cursor)
					}
					// QuoteRepository filters withdrawn rows after each raw storage page,
					// so the handler sees no survivors alongside a live cursor.
					return &quotes.QuoteRelationshipPage{NextCursor: fmt.Sprintf("withdrawn-page-%d", listCalls)}, nil
				},
			},
		}
		withdrawnHandler, _, _ := round11NewHandler(t, cfg, withdrawnRegistry)
		withdrawnCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, map[string]string{"limit": "80"}, nil)
		require.NoError(t, err)
		withdrawnCtx.Params["id"] = "target-1"

		withdrawnResponse := requireStatus(t, http.StatusOK)(withdrawnHandler.HandleGetQuotesOfStatusLift(withdrawnCtx))
		var withdrawnBody []apimodels.QuoteStatusSummary
		require.NoError(t, json.Unmarshal(withdrawnResponse.Body, &withdrawnBody))
		require.Empty(t, withdrawnBody)
		require.Equal(t, fetchCap, listCalls)
	})

	t.Run("relationship storage errors fail closed", func(t *testing.T) {
		failedRegistry := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
					return target, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{
				GetQuoteRelationshipsForStatusFunc: func(context.Context, string, int, string) (*quotes.QuoteRelationshipPage, error) {
					return nil, commonerrors.Internal("relationship storage failed")
				},
			},
		}
		failedHandler, _, _ := round11NewHandler(t, cfg, failedRegistry)
		failedCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, nil, nil)
		require.NoError(t, err)
		failedCtx.Params["id"] = "target-1"
		requireStatus(t, http.StatusInternalServerError)(failedHandler.HandleGetQuotesOfStatusLift(failedCtx))
	})
}

func TestQuotePermissionsRESTRoundTrips(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:accounts"})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:accounts"})
	actors := map[string]*activitypub.Actor{
		"alice": {BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("alice")}, PreferredUsername: "alice"},
		"bob":   {BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("bob")}, PreferredUsername: "bob"},
	}
	stored := map[string]*storagemodels.QuotePermissions{
		"alice": {Username: "alice", AllowPublic: true, AllowFollowers: true, AllowMentioned: true, BlockList: []string{}},
		"bob":   {Username: "bob", AllowFollowers: true, BlockList: []string{"mallory"}},
	}
	var updated *storagemodels.QuotePermissions
	var readUsernames []string
	reg := &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
				actor := actors[username]
				if actor == nil {
					return nil, commonerrors.NotFound("account")
				}
				return &storage.Account{Actor: actor}, nil
			},
		},
		QuotesSvc: &QuotesServiceStub{
			GetQuotePermissionsFunc: func(_ context.Context, username string) (*storagemodels.QuotePermissions, error) {
				readUsernames = append(readUsernames, username)
				return stored[username], nil
			},
			UpdateQuotePermissionsFunc: func(_ context.Context, permissions *storagemodels.QuotePermissions) error {
				copy := *permissions
				copy.BlockList = append([]string(nil), permissions.BlockList...)
				updated = &copy
				return nil
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, reg)

	t.Run("authenticated self read uses persisted policy", func(t *testing.T) {
		readUsernames = nil
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"
		resp := requireStatus(t, http.StatusOK)(handler.HandleGetQuotePermissionsLift(ctx))
		var body apimodels.QuotePermissionsResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, stored["alice"].AllowFollowers, body.AllowFollowers)
		require.Equal(t, stored["alice"].BlockList, body.BlockList)
		require.Equal(t, []string{"alice"}, readUsernames)
	})

	t.Run("other and missing accounts have one 404 shape without policy reads", func(t *testing.T) {
		readUsernames = nil
		var expectedBody []byte
		for _, accountID := range []string{"bob", "missing"} {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/"+accountID+"/quote_permissions", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
			require.NoError(t, err)
			ctx.Params["id"] = accountID
			resp := requireStatus(t, http.StatusNotFound)(handler.HandleGetQuotePermissionsLift(ctx))
			if expectedBody == nil {
				expectedBody = append([]byte(nil), resp.Body...)
			} else {
				require.Equal(t, expectedBody, resp.Body)
			}
		}
		require.Empty(t, readUsernames)
	})

	t.Run("permission read requires authentication and read scope", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetQuotePermissionsLift(ctx))

		onlyWrite := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		ctx, err = round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", map[string]string{"Authorization": "Bearer " + onlyWrite}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"
		requireStatus(t, http.StatusForbidden)(handler.HandleGetQuotePermissionsLift(ctx))
	})

	t.Run("update validates and persists the full arm set", func(t *testing.T) {
		allowPublic := false
		allowFollowers := true
		allowMentioned := true
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.UpdateQuotePermissionsRequest{
			AllowPublic:    &allowPublic,
			AllowFollowers: &allowFollowers,
			AllowMentioned: &allowMentioned,
			BlockList:      []string{" @bob ", "bob", "carol"},
		})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleUpdateQuotePermissionsLift(ctx))
		require.NotNil(t, updated)
		require.False(t, updated.AllowPublic)
		require.True(t, updated.AllowFollowers)
		require.True(t, updated.AllowMentioned)
		require.Equal(t, []string{"bob", "carol"}, updated.BlockList)
		var body apimodels.QuotePermissionsResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, updated.BlockList, body.BlockList)
	})

	t.Run("invalid block-list member fails before persistence", func(t *testing.T) {
		updated = nil
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.UpdateQuotePermissionsRequest{BlockList: []string{"../../root"}})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleUpdateQuotePermissionsLift(ctx))
		require.Nil(t, updated)
	})

	t.Run("permission storage errors fail closed", func(t *testing.T) {
		failedRegistry := &RegistryStub{
			AccountsSvc: reg.AccountsSvc,
			QuotesSvc: &QuotesServiceStub{
				GetQuotePermissionsFunc: func(context.Context, string) (*storagemodels.QuotePermissions, error) {
					return nil, commonerrors.Internal("permission storage failed")
				},
			},
		}
		failedHandler, _, _ := round11NewHandler(t, cfg, failedRegistry)

		readCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
		require.NoError(t, err)
		readCtx.Params["id"] = "alice"
		requireStatus(t, http.StatusInternalServerError)(failedHandler.HandleGetQuotePermissionsLift(readCtx))

		updateCtx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.UpdateQuotePermissionsRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(failedHandler.HandleUpdateQuotePermissionsLift(updateCtx))
	})

	t.Run("permission save errors fail closed", func(t *testing.T) {
		failedRegistry := &RegistryStub{
			QuotesSvc: &QuotesServiceStub{
				GetQuotePermissionsFunc: func(context.Context, string) (*storagemodels.QuotePermissions, error) {
					return stored["alice"], nil
				},
				UpdateQuotePermissionsFunc: func(context.Context, *storagemodels.QuotePermissions) error {
					return commonerrors.Internal("permission save failed")
				},
			},
		}
		failedHandler, _, _ := round11NewHandler(t, cfg, failedRegistry)
		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + writeToken}, nil, apimodels.UpdateQuotePermissionsRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(failedHandler.HandleUpdateQuotePermissionsLift(ctx))
	})
}

func TestQuoteRESTUnavailableServicesFailClosed(t *testing.T) {
	cfg := round11TestConfig()
	writeStatuses := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
	readAccounts := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read:accounts"})
	writeAccounts := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:accounts"})
	handler, _, _ := round11NewHandler(t, cfg, &RegistryStub{})
	handler.registry = &RegistryStub{}

	createCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses/target-1/quote", map[string]string{"Authorization": "Bearer " + writeStatuses}, nil, apimodels.CreateQuotePostRequest{Status: "quote"})
	require.NoError(t, err)
	createCtx.Params["id"] = "target-1"
	requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateQuotePostLift(createCtx))

	listCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, nil, nil)
	require.NoError(t, err)
	listCtx.Params["id"] = "target-1"
	requireStatus(t, http.StatusInternalServerError)(handler.HandleGetQuotesOfStatusLift(listCtx))

	readCtx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/quote_permissions", map[string]string{"Authorization": "Bearer " + readAccounts}, nil, nil)
	require.NoError(t, err)
	readCtx.Params["id"] = "alice"
	requireStatus(t, http.StatusInternalServerError)(handler.HandleGetQuotePermissionsLift(readCtx))

	updateCtx, err := round10NewLiftContext(http.MethodPut, "/api/v1/accounts/quote_permissions", map[string]string{"Authorization": "Bearer " + writeAccounts}, nil, apimodels.UpdateQuotePermissionsRequest{})
	require.NoError(t, err)
	requireStatus(t, http.StatusInternalServerError)(handler.HandleUpdateQuotePermissionsLift(updateCtx))
}
