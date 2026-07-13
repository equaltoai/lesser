package handlers

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apiModels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAccountsFull_Round12_HandleGetAccountFull_Errors(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
		Icon:              &activitypub.Image{URL: "https://example.com/avatar.png"},
		Image:             &activitypub.Image{URL: "https://example.com/header.png"},
		LastStatusAt:      func() *time.Time { t := now.Add(-24 * time.Hour); return &t }(),
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				switch username {
				case "missing":
					return nil, stdErrors.New("not found")
				case "boom":
					return nil, stdErrors.New("boom")
				default:
					return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
				}
			},
		},
		NotesSvc: &NotesServiceStub{
			CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) {
				require.Equal(t, actor.ID, authorID)
				return 3, nil
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			CountFollowersFunc: func(ctx context.Context, username string) (int64, error) { return 5, nil },
			GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{{Actor: actor}}, "", nil
			},
		},
	}

	t.Run("missing_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleGetAccountFull(ctx))
	})

	t.Run("not_found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/missing", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"

		requireStatus(t, http.StatusNotFound)(handler.HandleGetAccountFull(ctx))
	})

	t.Run("internal_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/boom", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "boom"

		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetAccountFull(ctx))
	})
}

func TestAccountsFull_Round12_AccountStatusesFallback(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	t.Run("account_not_found", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
					if username == "missing" {
						return nil, stdErrors.New("not found")
					}
					return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
				},
			},
			NotesSvc: &NotesServiceStub{
				GetUserTimelineFunc: func(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
					return &notes.GetUserTimelineResult{Items: []*storagemodels.Status{}, NextCursor: ""}, nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/missing/statuses", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusNotFound)(handler.handleAccountStatusesFallback(ctx, "missing", 20, "", ""))
	})

	t.Run("timeline_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
					return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
				},
			},
			NotesSvc: &NotesServiceStub{
				GetUserTimelineFunc: func(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
					return nil, stdErrors.New("timeline error")
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(handler.handleAccountStatusesFallback(ctx, "alice", 20, "", ""))
	})

	t.Run("success_sets_link_header", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
					return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
				},
			},
			NotesSvc: &NotesServiceStub{
				GetUserTimelineFunc: func(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
					items := []*storagemodels.Status{
						{
							StatusID:       "status-1",
							AuthorUsername: "alice",
							AuthorID:       actorID,
							Content:        "hello",
							Visibility:     "public",
							CreatedAt:      now.Add(-2 * time.Hour),
							UpdatedAt:      now.Add(-10 * time.Minute),
						},
					}
					return &notes.GetUserTimelineResult{Items: items, NextCursor: "cursor"}, nil
				},
				CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) { return 3, nil },
				CountRepliesFunc:       func(ctx context.Context, statusID string) (int, error) { return 1, nil },
				GetBoostCountFunc:      func(ctx context.Context, statusID string) (int64, error) { return 2, nil },
				GetLikeCountFunc:       func(ctx context.Context, statusID string) (int64, error) { return 4, nil },
			},
			RelationshipsSvc: &RelationshipsServiceStub{
				CountFollowersFunc: func(ctx context.Context, username string) (int64, error) { return 5, nil },
				GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
					return []*storage.Account{{Actor: actor}}, "", nil
				},
			},
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(handler.handleAccountStatusesFallback(ctx, "alice", 20, "", ""))
		require.NotEmpty(t, resp.Headers["link"])
	})
}

func TestAccountsFull_Round12_ConvertStatusToMastodonAPI_HelperBranches(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				switch username {
				case "nil_actor":
					return &storage.Account{User: &storage.User{Username: username}}, nil
				case "err":
					return nil, stdErrors.New("boom")
				default:
					return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
				}
			},
			IsAccountPinnedFunc: func(ctx context.Context, userID, pinnedActorID string) (bool, error) {
				if pinnedActorID == "pin-error" {
					return false, stdErrors.New("pin error")
				}
				return true, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) { return 3, nil },
			CountRepliesFunc: func(ctx context.Context, statusID string) (int, error) {
				if statusID == "reply-error" {
					return 0, stdErrors.New("err")
				}
				return 1, nil
			},
			GetBoostCountFunc: func(ctx context.Context, statusID string) (int64, error) {
				if statusID == "boost-error" {
					return 0, stdErrors.New("err")
				}
				return 2, nil
			},
			GetLikeCountFunc: func(ctx context.Context, statusID string) (int64, error) {
				if statusID == "like-error" {
					return 0, stdErrors.New("err")
				}
				return 4, nil
			},
			HasLikedFunc: func(ctx context.Context, userID, statusID string) (bool, error) {
				if statusID == "fav-error" {
					return false, stdErrors.New("err")
				}
				return true, nil
			},
			HasRebloggedFunc: func(ctx context.Context, userID, statusID string) (bool, error) {
				if statusID == "reblog-error" {
					return false, stdErrors.New("err")
				}
				return true, nil
			},
			IsBookmarkedFunc: func(ctx context.Context, userID, statusID string) (bool, error) {
				if statusID == "bookmark-error" {
					return false, stdErrors.New("err")
				}
				return true, nil
			},
			GetNoteWithViewerFunc: func(ctx context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
				statusID := ""
				if query != nil {
					statusID = query.StatusID
				}
				switch statusID {
				case "mute-note-error", "pin-note-error", "reply-note-error":
					return nil, stdErrors.New("err")
				case "reply-to":
					return &storagemodels.Status{AuthorID: "reply-author"}, nil
				case "pin-status":
					return &storagemodels.Status{AuthorID: "pin-error", AuthorUsername: "alice"}, nil
				default:
					return &storagemodels.Status{AuthorID: "author", AuthorUsername: "alice"}, nil
				}
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			CountFollowersFunc: func(ctx context.Context, username string) (int64, error) { return 5, nil },
			GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{{Actor: actor}}, "", nil
			},
			IsMutedFunc: func(ctx context.Context, userID, targetID string) (bool, error) {
				if targetID == "mute-error" {
					return false, stdErrors.New("err")
				}
				return true, nil
			},
		},
	}

	t.Run("viewer_missing_short_circuits_actions", func(t *testing.T) {
		status := &storagemodels.Status{
			StatusID:       "status-1",
			AuthorUsername: "nil_actor",
			AuthorID:       "author",
			Content:        "hello",
			Visibility:     "public",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
		}

		out := handler.convertStatusToMastodonAPI(status, "")
		require.Equal(t, false, out["favourited"])
		require.Equal(t, false, out["bookmarked"])
	})

	t.Run("viewer_actions_and_error_branches", func(t *testing.T) {
		edited := &storagemodels.Status{
			StatusID:       "fav-error",
			AuthorUsername: "alice",
			AuthorID:       "author",
			Content:        "hello",
			Visibility:     "public",
			InReplyToID:    "reply-to",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now.Add(-2 * time.Minute),
		}

		out := handler.convertStatusToMastodonAPI(edited, "viewer")
		require.Equal(t, edited.StatusID, out["id"])
		require.NotNil(t, out["edited_at"])
		require.Equal(t, "reply-author", *(out["in_reply_to_account_id"].(*string)))

		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "reply-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "boost-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "like-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "reblog-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "bookmark-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")

		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "mute-note-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "pin-note-error", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
		_ = handler.convertStatusToMastodonAPI(&storagemodels.Status{StatusID: "pin-status", AuthorUsername: "alice", AuthorID: "author", CreatedAt: now, UpdatedAt: now}, "viewer")
	})

	t.Run("minimal_account_on_lookup_error", func(t *testing.T) {
		status := &storagemodels.Status{
			StatusID:       "status-3",
			AuthorUsername: "err",
			AuthorID:       "author",
			Content:        "hello",
			Visibility:     "public",
			CreatedAt:      now.Add(-2 * time.Hour),
			UpdatedAt:      now,
		}

		out := handler.convertStatusToMastodonAPI(status, "viewer")
		acct := out["account"].(map[string]interface{})
		require.Equal(t, "author", acct["id"])
	})
}

func TestAccountsFull_Round12_ConvertStatusToMastodonAPIDoesNotHydrateAccountStatusCount(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()
	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: cfg.ActorURL("alice"), Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               cfg.BaseURL() + "/@alice",
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
				return &storage.Account{
					User:  &storage.User{Username: "alice", DisplayName: "Alice"},
					Actor: actor,
				}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			CountNotesByAuthorFunc: func(context.Context, string) (int64, error) {
				t.Fatalf("status rendering must not hydrate embedded account statuses_count")
				return 0, nil
			},
			CountRepliesFunc:  func(context.Context, string) (int, error) { return 0, nil },
			GetBoostCountFunc: func(context.Context, string) (int64, error) { return 0, nil },
			GetLikeCountFunc:  func(context.Context, string) (int64, error) { return 0, nil },
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			CountFollowersFunc: func(context.Context, string) (int64, error) { return 0, nil },
			GetFollowingFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
				return nil, "", nil
			},
		},
	}

	out := handler.convertStatusToMastodonAPI(&storagemodels.Status{
		StatusID:       "status-1",
		AuthorUsername: "alice",
		AuthorID:       cfg.ActorURL("alice"),
		Content:        "hello",
		Visibility:     "public",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, "")

	account := out["account"].(map[string]interface{})
	require.Equal(t, int64(0), account["statuses_count"])
}

func TestAccountsFull_Round12_FollowersFollowing_EdgeCases(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) {
				if authorID != actor.ID {
					return 0, nil
				}
				return 1, nil
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			GetFollowersFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{nil, &storage.Account{}, &storage.Account{Actor: actor}}, "", nil
			},
			GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{&storage.Account{}, &storage.Account{Actor: actor}}, "next", nil
			},
			CountFollowersFunc: func(ctx context.Context, username string) (int64, error) { return 5, nil },
		},
	}

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("followers_skip_nil_entries", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/followers", readHeaders, map[string]string{"limit": "bad"}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"

		requireStatus(t, http.StatusOK)(handler.HandleGetAccountFollowersFull(ctx))
	})

	t.Run("following_sets_pagination_link", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/following", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"

		resp := requireStatus(t, http.StatusOK)(handler.HandleGetAccountFollowingFull(ctx))
		require.NotEmpty(t, resp.Headers["link"])
		require.Contains(t, resp.Headers["link"][0], "max_id=next")
	})

	t.Run("build_account_response_includes_optional_fields", func(t *testing.T) {
		withMedia := &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Service"},
			PreferredUsername: "alice",
			Name:              "Alice",
			URL:               "https://example.com/@alice",
			Icon:              &activitypub.Image{URL: "https://example.com/avatar.png"},
			Image:             &activitypub.Image{URL: "https://example.com/header.png"},
			LastStatusAt:      func() *time.Time { t := now.Add(-24 * time.Hour); return &t }(),
		}

		account := handler.buildAccountResponseFull(context.Background(), withMedia)
		require.Equal(t, "https://example.com/avatar.png", account["avatar"])
		require.Equal(t, "https://example.com/header.png", account["header"])
		require.NotNil(t, account["last_status_at"])
		require.Equal(t, int64(1), account["statuses_count"])
	})
}

func TestAccountsFull_Round12_BuildAccountResponseFullSkipsRemoteStatusCounts(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CountNotesByAuthorFunc: func(context.Context, string) (int64, error) {
				t.Fatalf("remote actor must not inherit same-named local user's statuses_count")
				return 0, nil
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			CountFollowersFunc: func(context.Context, string) (int64, error) { return 0, nil },
			GetFollowingFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
				return nil, "", nil
			},
		},
	}

	account := handler.buildAccountResponseFull(context.Background(), &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://remote.example/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Remote Alice",
		URL:               "https://remote.example/@alice",
	})

	require.Equal(t, int64(0), account["statuses_count"])
}

func TestAccountsFull_Round12_VerifyCredentialsAuthError(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", nil, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusUnauthorized)(handler.HandleVerifyCredentialsFull(ctx))
}

func TestAccountsFull_Round12_UpdateCredentials_ParseError(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil, []byte(`{invalid}`))

	requireStatus(t, http.StatusBadRequest)(handler.HandleUpdateCredentialsFull(ctx))
}

func TestAccountsFull_Round12_GetAccountStatuses_ServiceError(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		NotesSvc: &NotesServiceStub{
			ListNotesFunc: func(ctx context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
				return nil, stdErrors.New("boom")
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", headers, map[string]string{"limit": "bad", "only_media": boolTrue}, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "alice"

	requireStatus(t, http.StatusInternalServerError)(handler.HandleGetAccountStatusesFull(ctx))
}

func TestAccountsFull_Round12_VerifyCredentials_ServiceError(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return nil, stdErrors.New("boom")
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", headers, nil, nil)
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(handler.HandleVerifyCredentialsFull(ctx))
}

func TestAccountsFull_Round12_UpdateCredentials_ServiceError(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					User:  &storage.User{Username: username},
					Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + username}, PreferredUsername: username},
				}, nil
			},
			UpdateProfileFunc: func(ctx context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
				return nil, stdErrors.New("boom")
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil, apiModels.UpdateCredentialsRequest{DisplayName: "Alice"})
	require.NoError(t, err)

	requireStatus(t, http.StatusInternalServerError)(handler.HandleUpdateCredentialsFull(ctx))
}

func TestAccountsFull_Round12_UpdateCredentials_CommandAndTransformErrors(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("account_load_error_returns_500", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
					return nil, stdErrors.New("missing account row")
				},
			},
		}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil, []byte(`{"display_name":"Della"}`))

		requireStatus(t, http.StatusInternalServerError)(handler.HandleUpdateCredentialsFull(ctx))
	})

	t.Run("untransformable_update_result_returns_500", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		handler.registry = &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
					return &storage.Account{
						User: &storage.User{Username: "alice"},
						Actor: &activitypub.Actor{
							BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice", Type: "Person"},
							PreferredUsername: "alice",
						},
					}, nil
				},
				UpdateProfileFunc: func(context.Context, *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
					return &accounts.AccountResult{Account: nil}, nil
				},
			},
		}

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil, []byte(`{"display_name":"Della"}`))

		requireStatus(t, http.StatusInternalServerError)(handler.HandleUpdateCredentialsFull(ctx))
	})
}

func TestAccountsFull_Round12_UpdateCredentials_SuccessReturnsMastodonAccount(t *testing.T) {
	cfg := round11TestConfig()
	expectedActorID := cfg.BaseURL() + "/users/alice"

	existingAccount := &storage.Account{
		User: &storage.User{
			Username:     "alice",
			DisplayName:  "Della",
			Note:         "old bio",
			Locked:       true,
			Discoverable: true,
			IsAgent:      true,
		},
		Actor: &activitypub.Actor{
			BaseObject:                activitypub.BaseObject{ID: expectedActorID, Type: activitypub.ServiceType},
			PreferredUsername:         "alice",
			Name:                      "Della",
			Summary:                   "old bio",
			ManuallyApprovesFollowers: true,
			Discoverable:              true,
		},
	}
	updatedAccount := &storage.Account{
		User: &storage.User{
			Username:     "alice",
			DisplayName:  "Della",
			Note:         "same bio",
			Locked:       true,
			Discoverable: true,
			IsAgent:      true,
		},
		Actor: &activitypub.Actor{
			BaseObject:                activitypub.BaseObject{ID: expectedActorID, Type: activitypub.ServiceType},
			PreferredUsername:         "alice",
			Name:                      "Della",
			Summary:                   "same bio",
			ManuallyApprovesFollowers: true,
			Discoverable:              true,
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				require.Equal(t, "alice", username)
				return existingAccount, nil
			},
			UpdateProfileFunc: func(ctx context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
				require.Equal(t, "alice", cmd.Username)
				require.Equal(t, "alice", cmd.UpdaterID)
				require.Equal(t, "Della", cmd.DisplayName)
				require.Equal(t, "same bio", cmd.Bio)
				require.True(t, cmd.Locked)
				require.True(t, cmd.Discoverable)
				require.True(t, cmd.Bot)
				return &accounts.AccountResult{Account: updatedAccount}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) {
				require.Equal(t, expectedActorID, authorID)
				return 9, nil
			},
		},
	}

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/accounts/update_credentials", headers, nil, []byte(`{"display_name":"Della","note":"same bio"}`))

	resp := requireStatus(t, http.StatusOK)(handler.HandleUpdateCredentialsFull(ctx))

	var got apiModels.Account
	require.NoError(t, json.Unmarshal(resp.Body, &got))
	require.Equal(t, "alice", got.Username)
	require.Equal(t, "Della", got.DisplayName)
	require.Equal(t, "same bio", got.Note)
	require.True(t, got.Locked)
	require.True(t, got.Discoverable)
	require.True(t, got.Bot)
	require.Equal(t, 9, got.StatusesCount)
}

func TestAccountsFull_Round12_HasUserMutedStatus_IsMutedError(t *testing.T) {
	cfg := round11TestConfig()

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		NotesSvc: &NotesServiceStub{
			GetNoteFunc: func(ctx context.Context, statusID string) (*storagemodels.Status, error) {
				return &storagemodels.Status{AuthorUsername: "mute-error"}, nil
			},
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			IsMutedFunc: func(ctx context.Context, userID, targetID string) (bool, error) {
				return false, stdErrors.New("boom")
			},
		},
	}

	require.False(t, handler.hasUserMutedStatus(context.Background(), "viewer", "status-1"))
}

func TestAccountsFull_Round12_ResolveAccountIDFull_AtHandleFormat(t *testing.T) {
	cfg := round11TestConfig()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
			},
		},
	}

	resolved, err := handler.resolveAccountIDFull(context.Background(), "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, "alice", resolved.PreferredUsername)
}

func TestAccountsFull_Round12_GetReplyToAccountID_EdgeCases(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		NotesSvc: &NotesServiceStub{
			GetNoteWithViewerFunc: func(ctx context.Context, query *notes.GetNoteQuery) (*storagemodels.Status, error) {
				return nil, stdErrors.New("not found")
			},
		},
	}

	t.Run("missing_in_reply_to", func(t *testing.T) {
		status := &storagemodels.Status{}
		require.Nil(t, handler.getReplyToAccountID(context.Background(), "", status))
	})

	t.Run("reply_lookup_error", func(t *testing.T) {
		status := &storagemodels.Status{InReplyToID: "missing"}
		require.Nil(t, handler.getReplyToAccountID(context.Background(), "alice", status))
	})
}

func TestAccountsFull_Round12_AccountStatusesFallback_NoNextCursor(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now().UTC()

	actor := &activitypub.Actor{
		BaseObject:        activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"},
		PreferredUsername: "alice",
		Name:              "Alice",
		URL:               "https://example.com/@alice",
	}

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	handler.registry = &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{Actor: actor, User: &storage.User{Username: username}}, nil
			},
		},
		NotesSvc: &NotesServiceStub{
			GetUserTimelineFunc: func(ctx context.Context, actorID string, opts interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
				items := []*storagemodels.Status{
					{
						StatusID:       "status-1",
						AuthorUsername: "alice",
						AuthorID:       actorID,
						Content:        "hello",
						Visibility:     "public",
						CreatedAt:      now.Add(-2 * time.Hour),
						UpdatedAt:      now,
					},
				}
				return &notes.GetUserTimelineResult{Items: items, NextCursor: ""}, nil
			},
			CountNotesByAuthorFunc: func(ctx context.Context, authorID string) (int64, error) { return 3, nil },
			CountRepliesFunc:       func(ctx context.Context, statusID string) (int, error) { return 1, nil },
			GetBoostCountFunc:      func(ctx context.Context, statusID string) (int64, error) { return 2, nil },
			GetLikeCountFunc:       func(ctx context.Context, statusID string) (int64, error) { return 4, nil },
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			CountFollowersFunc: func(ctx context.Context, username string) (int64, error) { return 5, nil },
			GetFollowingFunc: func(ctx context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
				return []*storage.Account{{Actor: actor}}, "", nil
			},
		},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", nil, nil, nil)
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusOK)(handler.handleAccountStatusesFallback(ctx, "alice", 20, "", ""))
	require.Empty(t, resp.Headers["link"])
}
