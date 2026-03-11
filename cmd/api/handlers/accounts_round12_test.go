package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	repoMocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestAccountsRound12_HandleRegistrationLift(t *testing.T) {
	t.Run("validation_error_returns_422", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, round10TestConfig(), &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
					return nil, errors.New("unexpected service call")
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts", nil, nil, apimodels.AccountRegistrationRequest{
			Username:  "alice",
			Agreement: false,
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleRegistrationLift(ctx))
	})

	t.Run("service_errors_are_mapped", func(t *testing.T) {
		cfg := round10TestConfig()
		now := time.Now()

		tests := []struct {
			name       string
			serviceErr error
			wantStatus int
		}{
			{name: "username_already_taken", serviceErr: accounts.ErrUsernameAlreadyTaken, wantStatus: http.StatusConflict},
			{name: "validation_failed", serviceErr: accounts.ErrValidationFailed, wantStatus: http.StatusUnprocessableEntity},
			{name: "other_error", serviceErr: errors.New("boom"), wantStatus: http.StatusUnprocessableEntity},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				state := &round10QueryState{
					walletChallengesByID: map[string]storagemodels.WalletChallenge{
						"c1": {
							ID:        "c1",
							Username:  "alice",
							Address:   "0xabc",
							ChainID:   1,
							Nonce:     "nonce",
							Message:   "msg",
							IssuedAt:  now.Add(-time.Minute),
							ExpiresAt: now.Add(10 * time.Minute),
							Used:      true,
						},
					},
				}

				h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
							return nil, tt.serviceErr
						},
					},
				})

				ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts", nil, nil, apimodels.AccountRegistrationRequest{
					Username:          "alice",
					Password:          "ignored",
					Agreement:         true,
					WalletChallengeID: "c1",
				})
				require.NoError(t, err)

				requireStatus(t, tt.wantStatus)(h.HandleRegistrationLift(ctx))
			})
		}
	})

	t.Run("success_ignores_password_and_email", func(t *testing.T) {
		cfg := round10TestConfig()
		now := time.Now()

		var gotCmd *accounts.RegisterAccountCommand
		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {
					ID:        "c1",
					Username:  "alice",
					Address:   "0xabc",
					ChainID:   1,
					Nonce:     "nonce",
					Message:   "msg",
					IssuedAt:  now.Add(-time.Minute),
					ExpiresAt: now.Add(10 * time.Minute),
					Used:      true,
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				RegisterAccountFunc: func(_ context.Context, cmd *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
					gotCmd = cmd
					return &accounts.RegisterAccountResult{
						Account: &storage.Account{User: &storage.User{Username: cmd.Username}},
						Actor: &activitypub.Actor{
							BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/" + cmd.Username},
							PreferredUsername: cmd.Username,
						},
					}, nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/accounts", nil, nil, apimodels.AccountRegistrationRequest{
			Username:                 "alice",
			Password:                 "this-should-be-ignored",
			Agreement:                true,
			Locale:                   "en",
			Reason:                   "hello",
			DefaultPostingVisibility: "public",
			WalletChallengeID:        "c1",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusCreated)(h.HandleRegistrationLift(ctx))

		require.NotNil(t, gotCmd)
		require.Equal(t, "alice", gotCmd.Username)
		require.Empty(t, gotCmd.Email)
		require.Empty(t, gotCmd.Password)
		require.Equal(t, "en", gotCmd.Locale)
		require.True(t, gotCmd.Agreement)
		require.Equal(t, "hello", gotCmd.Reason)
		require.Equal(t, "public", gotCmd.DefaultPostingVisibility)
		require.Equal(t, "c1", gotCmd.RegistrationChallengeID)

		var regResp apimodels.AccountRegistrationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &regResp))
		require.Equal(t, cfg.BaseURL()+"/users/alice", regResp.ID)
		require.Equal(t, "alice", regResp.Username)
		require.True(t, regResp.Created)
	})

	t.Run("validateRegistrationRequestLift_allows_empty_visibility", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, round10TestConfig(), &round10QueryState{}, &RegistryStub{})
		require.NoError(t, h.validateRegistrationRequestLift(apimodels.AccountRegistrationRequest{
			Username:          "alice",
			Agreement:         true,
			WalletChallengeID: "c1",
		}))
	})
}

func TestAccountsRound12_HandleVerifyCredentialsLift(t *testing.T) {
	cfg := round10TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	authHeaders := map[string]string{"Authorization": "Bearer " + token}

	t.Run("missing_token_returns_error", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", nil, nil, nil)
		require.NoError(t, err)

		_, err = h.HandleVerifyCredentialsLift(ctx)
		require.Error(t, err)
	})

	t.Run("nil_registry_returns_500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", authHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleVerifyCredentialsLift(ctx))
	})

	t.Run("service_error_returns_500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
					return nil, errors.New("boom")
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", authHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleVerifyCredentialsLift(ctx))
	})

	t.Run("success_returns_200", func(t *testing.T) {
		account := &storage.Account{
			User:  &storage.User{Username: "alice"},
			Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice"}, PreferredUsername: "alice"},
		}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(_ context.Context, username string) (*storage.Account, error) {
					require.Equal(t, "alice", username)
					return account, nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/verify_credentials", authHeaders, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleVerifyCredentialsLift(ctx))

		var got apimodels.Account
		require.NoError(t, json.Unmarshal(resp.Body, &got))
		require.Equal(t, "alice", got.Username)
		require.Equal(t, "alice", got.Acct)
		require.NotEmpty(t, got.ID)
	})
}

func TestAccountsRound12_HandleUpdateCredentialsLift(t *testing.T) {
	cfg := round10TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	authHeaders := map[string]string{"Authorization": "Bearer " + token}

	t.Run("invalid_json_returns_400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPatch, "/api/v1/accounts/update_credentials", nil, nil, []byte("{"))

		requireStatus(t, http.StatusBadRequest)(h.HandleUpdateCredentialsLift(ctx))
	})

	t.Run("invalid_params_returns_400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/accounts/update_credentials", nil, nil, apimodels.UpdateCredentialsRequest{
			DisplayName: strings.Repeat("a", 31),
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(h.HandleUpdateCredentialsLift(ctx))
	})

	t.Run("missing_token_returns_error", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/accounts/update_credentials", nil, nil, apimodels.UpdateCredentialsRequest{
			DisplayName: "ok",
		})
		require.NoError(t, err)

		_, err = h.HandleUpdateCredentialsLift(ctx)
		require.Error(t, err)
	})

	t.Run("service_error_returns_500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				UpdateProfileFunc: func(context.Context, *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
					return nil, errors.New("boom")
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/accounts/update_credentials", authHeaders, nil, apimodels.UpdateCredentialsRequest{
			DisplayName: "ok",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleUpdateCredentialsLift(ctx))
	})

	t.Run("success_returns_200", func(t *testing.T) {
		account := &storage.Account{
			User:  &storage.User{Username: "alice"},
			Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice"}, PreferredUsername: "alice"},
		}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				UpdateProfileFunc: func(_ context.Context, cmd *accounts.UpdateProfileCommand) (*accounts.AccountResult, error) {
					require.Equal(t, "alice", cmd.Username)
					require.Equal(t, "alice", cmd.UpdaterID)
					return &accounts.AccountResult{Account: account}, nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodPatch, "/api/v1/accounts/update_credentials", authHeaders, nil, apimodels.UpdateCredentialsRequest{
			DisplayName: "ok",
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleUpdateCredentialsLift(ctx))
		var got storage.Account
		require.NoError(t, json.Unmarshal(resp.Body, &got))
		require.NotNil(t, got.User)
		require.Equal(t, "alice", got.User.Username)
		require.NotNil(t, got.Actor)
		require.Equal(t, cfg.BaseURL()+"/users/alice", got.Actor.ID)
		require.Equal(t, "alice", got.Actor.PreferredUsername)
	})
}

func TestAccountsRound12_AccountHandlers_ErrorBranches(t *testing.T) {
	cfg := round10TestConfig()

	t.Run("HandleGetAccountLift_invalid_id", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/x", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = strings.Repeat("a", 501)

		requireStatus(t, http.StatusBadRequest)(h.HandleGetAccountLift(ctx))
	})

	t.Run("HandleGetAccountLift_not_found_and_internal", func(t *testing.T) {
		tests := []struct {
			name       string
			err        error
			wantStatus int
		}{
			{name: "not_found", err: errors.New("not found"), wantStatus: http.StatusNotFound},
			{name: "internal", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
							return nil, tt.err
						},
					},
				})

				ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice", nil, nil, nil)
				require.NoError(t, err)
				ctx.Params["id"] = "alice"

				requireStatus(t, tt.wantStatus)(h.HandleGetAccountLift(ctx))
			})
		}
	})

	t.Run("HandleAccountLookupLift_validation_and_errors", func(t *testing.T) {
		t.Run("invalid_acct", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/lookup", nil, map[string]string{"acct": ""}, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusBadRequest)(h.HandleAccountLookupLift(ctx))
		})

		t.Run("not_found_and_internal", func(t *testing.T) {
			tests := []struct {
				name       string
				err        error
				wantStatus int
			}{
				{name: "not_found", err: errors.New("not found"), wantStatus: http.StatusNotFound},
				{name: "internal", err: errors.New("boom"), wantStatus: http.StatusInternalServerError},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
						AccountsSvc: &AccountsServiceStub{
							LookupAccountFunc: func(context.Context, *accounts.LookupAccountQuery) (*storage.Account, error) {
								return nil, tt.err
							},
						},
					})

					ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/lookup", nil, map[string]string{"acct": "alice@example.com"}, nil)
					require.NoError(t, err)

					requireStatus(t, tt.wantStatus)(h.HandleAccountLookupLift(ctx))
				})
			}
		})
	})
}

func TestAccountsRound12_RelationshipsAndActions(t *testing.T) {
	cfg := round10TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	authHeaders := map[string]string{"Authorization": "Bearer " + token}

	actorAccount := &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice"},
			PreferredUsername: "alice",
		},
	}

	t.Run("handleAccountRelationshipsList_errors_and_skips_nil", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) { return actorAccount, nil },
			},
			RelationshipsSvc: &RelationshipsServiceStub{
				GetFollowersFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
					return []*storage.Account{nil, {Actor: nil}, actorAccount}, "", nil
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/followers", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAccountFollowersLift(ctx))

		var body []apimodels.Account
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body, 1)
	})

	t.Run("handleAccountRelationshipsList_min_id_used_as_cursor_and_errors", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) { return actorAccount, nil },
			},
			RelationshipsSvc: &RelationshipsServiceStub{
				GetFollowingFunc: func(_ context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
					require.Equal(t, "alice", username)
					require.Equal(t, "min", cursor)
					return nil, "", errors.New("boom")
				},
			},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/following", nil, map[string]string{"min_id": "min"}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"

		requireStatus(t, http.StatusInternalServerError)(h.HandleGetAccountFollowingLift(ctx))
	})

	t.Run("handleAccountRelationshipsList_resolves_numeric_account_id", func(t *testing.T) {
		const numericID = "446117837104852"

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			AccountsSvc: &AccountsServiceStub{
				GetAccountFunc: func(context.Context, string) (*storage.Account, error) {
					return nil, errors.New("not found")
				},
			},
			RelationshipsSvc: &RelationshipsServiceStub{
				GetFollowersFunc: func(_ context.Context, username string, limit int, cursor string) ([]*storage.Account, string, error) {
					require.Equal(t, "alice", username)
					return []*storage.Account{actorAccount}, "", nil
				},
			},
		})

		actorRepo := repoMocks.NewMockActorRepository()
		actorRepo.On("GetActorByNumericID", mock.Anything, numericID).Return(actorAccount.Actor, nil).Once()

		repos := &MockRepositoryStorage{}
		repos.On("Actor").Return(actorRepo).Maybe()
		h.repos = repos

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/"+numericID+"/followers", authHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = numericID

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAccountFollowersLift(ctx))

		var body []apimodels.Account
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Len(t, body, 1)
		require.Equal(t, "alice", body[0].Username)
		actorRepo.AssertExpectations(t)
	})

	t.Run("HandleGetFamiliarFollowersLift_error_cases", func(t *testing.T) {
		t.Run("missing_token", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/familiar_followers", nil, map[string]string{"id[]": "alice"}, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusUnauthorized)(h.HandleGetFamiliarFollowersLift(ctx))
		})

		t.Run("invalid_token", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/familiar_followers", map[string]string{"Authorization": "Bearer bad"}, map[string]string{"id[]": "alice"}, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusUnauthorized)(h.HandleGetFamiliarFollowersLift(ctx))
		})

		t.Run("missing_account_ids", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/familiar_followers", authHeaders, nil, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusBadRequest)(h.HandleGetFamiliarFollowersLift(ctx))
		})

		t.Run("service_error", func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetFamiliarFollowersFunc: func(context.Context, *accounts.GetFamiliarFollowersQuery) (*accounts.FamiliarFollowersResult, error) {
						return nil, errors.New("boom")
					},
				},
			})

			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/familiar_followers", authHeaders, map[string]string{"id[]": "alice"}, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusInternalServerError)(h.HandleGetFamiliarFollowersLift(ctx))
		})
	})

	t.Run("pin_unpin_note_remove_branches", func(t *testing.T) {
		tests := []struct {
			name       string
			call       func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error)
			stubReg    *RegistryStub
			wantStatus int
		}{
			{
				name: "pin_already_pinned_422",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandlePinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						PinAccountFunc: func(context.Context, *accounts.PinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("already pinned")
						},
					},
				},
				wantStatus: http.StatusUnprocessableEntity,
			},
			{
				name: "pin_not_found_404",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandlePinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						PinAccountFunc: func(context.Context, *accounts.PinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("not found")
						},
					},
				},
				wantStatus: http.StatusNotFound,
			},
			{
				name: "pin_unauthorized_403",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandlePinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						PinAccountFunc: func(context.Context, *accounts.PinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("unauthorized")
						},
					},
				},
				wantStatus: http.StatusForbidden,
			},
			{
				name: "pin_internal_error_500",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandlePinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						PinAccountFunc: func(context.Context, *accounts.PinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("boom")
						},
					},
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name: "unpin_unauthorized_403",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleUnpinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						UnpinAccountFunc: func(context.Context, *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("unauthorized")
						},
					},
				},
				wantStatus: http.StatusForbidden,
			},
			{
				name: "unpin_not_found_404",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleUnpinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						UnpinAccountFunc: func(context.Context, *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("not found")
						},
					},
				},
				wantStatus: http.StatusNotFound,
			},
			{
				name: "unpin_internal_error_500",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleUnpinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						UnpinAccountFunc: func(context.Context, *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("boom")
						},
					},
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name: "unpin_success_200",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleUnpinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						UnpinAccountFunc: func(_ context.Context, cmd *accounts.UnpinAccountCommand) (*accounts.RelationshipResult, error) {
							require.Equal(t, "alice", cmd.Username)
							require.Equal(t, "bob", cmd.TargetAccount)
							return &accounts.RelationshipResult{Relationship: map[string]any{"ok": true}}, nil
						},
					},
				},
				wantStatus: http.StatusOK,
			},
			{
				name: "set_note_invalid_json_400",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleSetAccountNoteLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						SetAccountNoteFunc: func(context.Context, *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("unexpected service call")
						},
					},
				},
				wantStatus: http.StatusBadRequest,
			},
			{
				name: "set_note_unauthorized_403",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleSetAccountNoteLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						SetAccountNoteFunc: func(context.Context, *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("unauthorized")
						},
					},
				},
				wantStatus: http.StatusForbidden,
			},
			{
				name: "set_note_not_found_404",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleSetAccountNoteLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						SetAccountNoteFunc: func(context.Context, *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("not found")
						},
					},
				},
				wantStatus: http.StatusNotFound,
			},
			{
				name: "set_note_internal_error_500",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleSetAccountNoteLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						SetAccountNoteFunc: func(context.Context, *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("boom")
						},
					},
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name: "set_note_success_200",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleSetAccountNoteLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						SetAccountNoteFunc: func(_ context.Context, cmd *accounts.SetAccountNoteCommand) (*accounts.RelationshipResult, error) {
							require.Equal(t, "alice", cmd.Username)
							require.Equal(t, "bob", cmd.TargetAccount)
							require.Equal(t, "note", cmd.Note)
							return &accounts.RelationshipResult{Relationship: map[string]any{"ok": true}}, nil
						},
					},
				},
				wantStatus: http.StatusOK,
			},
			{
				name: "remove_follower_not_found_404",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleRemoveFromFollowersLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						RemoveFollowerFunc: func(context.Context, *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("not found")
						},
					},
				},
				wantStatus: http.StatusNotFound,
			},
			{
				name: "remove_follower_unauthorized_403",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleRemoveFromFollowersLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						RemoveFollowerFunc: func(context.Context, *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("unauthorized")
						},
					},
				},
				wantStatus: http.StatusForbidden,
			},
			{
				name: "remove_follower_internal_error_500",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleRemoveFromFollowersLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						RemoveFollowerFunc: func(context.Context, *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error) {
							return nil, errors.New("boom")
						},
					},
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name: "remove_follower_success_200",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandleRemoveFromFollowersLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						RemoveFollowerFunc: func(_ context.Context, cmd *accounts.RemoveFollowerCommand) (*accounts.RelationshipResult, error) {
							require.Equal(t, "alice", cmd.Username)
							require.Equal(t, "bob", cmd.FollowerID)
							return &accounts.RelationshipResult{Relationship: map[string]any{"ok": true}}, nil
						},
					},
				},
				wantStatus: http.StatusOK,
			},
			{
				name: "pin_success_200",
				call: func(h *Handler, ctx *apptheory.Context) (*apptheory.Response, error) {
					return h.HandlePinAccountLift(ctx)
				},
				stubReg: &RegistryStub{
					AccountsSvc: &AccountsServiceStub{
						PinAccountFunc: func(_ context.Context, cmd *accounts.PinAccountCommand) (*accounts.RelationshipResult, error) {
							require.Equal(t, "alice", cmd.Username)
							require.Equal(t, "bob", cmd.TargetAccount)
							return &accounts.RelationshipResult{Relationship: map[string]any{"ok": true}}, nil
						},
					},
				},
				wantStatus: http.StatusOK,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, tt.stubReg)

				var ctx *apptheory.Context
				switch tt.name {
				case "set_note_invalid_json_400":
					ctx = round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/accounts/bob/note", authHeaders, nil, []byte("{"))
				default:
					var err error
					body := any(nil)
					if strings.HasPrefix(tt.name, "set_note_") {
						body = map[string]any{"comment": "note"}
					}
					ctx, err = round10NewLiftContext(http.MethodPost, "/api/v1/accounts/bob/action", authHeaders, nil, body)
					require.NoError(t, err)
				}
				ctx.Params["id"] = "bob"

				requireStatus(t, tt.wantStatus)(tt.call(h, ctx))
			})
		}
	})
}

func TestAccountsRound12_ActivityPubCollectionHelpers(t *testing.T) {
	cfg := round10TestConfig()
	token := round10SignAccessToken(t, cfg.JWTSecret, "alice")
	authHeaders := map[string]string{"Authorization": "Bearer " + token}

	account := &storage.Account{
		User: &storage.User{Username: "alice"},
		Actor: &activitypub.Actor{
			BaseObject:        activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice"},
			PreferredUsername: "alice",
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(context.Context, string) (*storage.Account, error) { return account, nil },
		},
		RelationshipsSvc: &RelationshipsServiceStub{
			CountFollowersFunc: func(context.Context, string) (int64, error) { return 2, nil },
			CountFollowingFunc: func(context.Context, string) (int64, error) { return 0, nil },
			GetFollowersFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
				return []*storage.Account{
					{User: &storage.User{Username: "bob"}},
					{User: &storage.User{Username: "carol"}},
				}, "next", nil
			},
			GetFollowingFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
				return []*storage.Account{
					{User: &storage.User{Username: "bob"}},
				}, "", nil
			},
		},
	})

	t.Run("getCollectionData_unsupported", func(t *testing.T) {
		_, _, err := h.getCollectionData(nil, account.Actor, "bad", "", 1)
		require.Error(t, err)
	})

	t.Run("extractBoundary_missing_boundary", func(t *testing.T) {
		_, err := h.extractBoundary("multipart/form-data")
		require.Error(t, err)
	})

	t.Run("getFollowersData_and_getFollowingData_convert_usernames", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers", nil, nil, nil)
		require.NoError(t, err)

		usernames, next, err := h.getFollowersData(ctx, "alice", "", 2)
		require.NoError(t, err)
		require.Equal(t, "next", next)
		require.Equal(t, []string{"bob", "carol"}, usernames)

		following, next2, err := h.getFollowingData(ctx, "alice", "", 2)
		require.NoError(t, err)
		require.Empty(t, next2)
		require.Equal(t, []string{"bob"}, following)
	})

	t.Run("buildCollectionPage_builds_next_and_prev", func(t *testing.T) {
		page := h.buildCollectionPage(account.Actor, "followers", "cursor", "next", 2, []string{"bob"})
		require.NotNil(t, page)
		require.NotEmpty(t, page.Next)
		require.NotEmpty(t, page.Prev)
	})

	t.Run("checkCollectionAccess_private_followers_requires_matching_token", func(t *testing.T) {
		privateActor := &activitypub.Actor{
			BaseObject:                activitypub.BaseObject{ID: cfg.BaseURL() + "/users/alice"},
			PreferredUsername:         "alice",
			ManuallyApprovesFollowers: true,
		}

		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers?page=1", nil, map[string]string{"page": "1"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.checkCollectionAccess(ctxMissing, privateActor, "followers"))

		ctxBad, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers?page=1", map[string]string{"Authorization": "Bearer bad"}, map[string]string{"page": "1"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.checkCollectionAccess(ctxBad, privateActor, "followers"))

		ctxOK, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers?page=1", authHeaders, map[string]string{"page": "1"}, nil)
		require.NoError(t, err)
		resp, err := h.checkCollectionAccess(ctxOK, privateActor, "followers")
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("HandleActivityPubFollowersLift_flows", func(t *testing.T) {
		t.Run("missing_username_param", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/users//followers", map[string]string{"Accept": "application/activity+json"}, nil, nil)
			require.NoError(t, err)

			requireStatus(t, http.StatusBadRequest)(h.HandleActivityPubFollowersLift(ctx))
		})

		t.Run("invalid_limit", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers?page=1", map[string]string{"Accept": "application/activity+json"}, map[string]string{"page": "1", "limit": "0"}, nil)
			require.NoError(t, err)
			ctx.Params["username"] = "alice"

			requireStatus(t, http.StatusBadRequest)(h.HandleActivityPubFollowersLift(ctx))
		})

		t.Run("non_activitypub_accept_uses_mastodon_endpoint", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers", map[string]string{"Accept": "text/html"}, nil, nil)
			require.NoError(t, err)
			ctx.Params["username"] = "alice"
			ctx.Params["id"] = "alice"

			requireStatus(t, http.StatusOK)(h.HandleActivityPubFollowersLift(ctx))
		})

		t.Run("activitypub_collection_metadata_and_page", func(t *testing.T) {
			ctxMeta, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers", map[string]string{"Accept": "application/activity+json"}, nil, nil)
			require.NoError(t, err)
			ctxMeta.Params["username"] = "alice"

			respMeta := requireStatus(t, http.StatusOK)(h.HandleActivityPubFollowersLift(ctxMeta))
			var collection activitypub.OrderedCollection
			require.NoError(t, json.Unmarshal(respMeta.Body, &collection))
			require.Equal(t, 2, collection.TotalItems)
			require.NotEmpty(t, collection.First)

			ctxPage, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers?page=1", map[string]string{"Accept": "application/activity+json"}, map[string]string{"page": "1", "cursor": "cursor", "limit": "2"}, nil)
			require.NoError(t, err)
			ctxPage.Params["username"] = "alice"

			respPage := requireStatus(t, http.StatusOK)(h.HandleActivityPubFollowersLift(ctxPage))
			var page activitypub.OrderedCollectionPage
			require.NoError(t, json.Unmarshal(respPage.Body, &page))
			require.NotEmpty(t, page.Prev)
			require.NotEmpty(t, page.Next)
			require.Len(t, page.OrderedItems, 2)
		})
	})

	t.Run("HandleActivityPubFollowingLift_and_error_branches", func(t *testing.T) {
		ctxMeta, err := round10NewLiftContext(http.MethodGet, "/users/alice/following", map[string]string{"Accept": "application/activity+json"}, nil, nil)
		require.NoError(t, err)
		ctxMeta.Params["username"] = "alice"

		respMeta := requireStatus(t, http.StatusOK)(h.HandleActivityPubFollowingLift(ctxMeta))
		var collection activitypub.OrderedCollection
		require.NoError(t, json.Unmarshal(respMeta.Body, &collection))
		require.Equal(t, 0, collection.TotalItems)
		require.Empty(t, collection.First)

		t.Run("page_with_relationship_error_returns_500", func(t *testing.T) {
			errHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) { return account, nil },
				},
				RelationshipsSvc: &RelationshipsServiceStub{
					GetFollowersFunc: func(context.Context, string, int, string) ([]*storage.Account, string, error) {
						return nil, "", errors.New("boom")
					},
				},
			})

			ctx, err := round10NewLiftContext(http.MethodGet, "/users/alice/followers?page=1", map[string]string{"Accept": "application/activity+json"}, map[string]string{"page": "1"}, nil)
			require.NoError(t, err)
			ctx.Params["username"] = "alice"

			requireStatus(t, http.StatusInternalServerError)(errHandler.HandleActivityPubFollowersLift(ctx))
		})

		t.Run("count_error_returns_500", func(t *testing.T) {
			errHandler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
				AccountsSvc: &AccountsServiceStub{
					GetAccountFunc: func(context.Context, string) (*storage.Account, error) { return account, nil },
				},
				RelationshipsSvc: &RelationshipsServiceStub{
					CountFollowingFunc: func(context.Context, string) (int64, error) { return 0, errors.New("boom") },
				},
			})

			ctx, err := round10NewLiftContext(http.MethodGet, "/users/alice/following", map[string]string{"Accept": "application/activity+json"}, nil, nil)
			require.NoError(t, err)
			ctx.Params["username"] = "alice"

			requireStatus(t, http.StatusInternalServerError)(errHandler.HandleActivityPubFollowingLift(ctx))
		})
	})
}
