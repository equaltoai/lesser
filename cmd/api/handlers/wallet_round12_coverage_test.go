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
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestWalletHandlers_Round12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("create_challenge_username_required", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/challenge", nil, nil, apimodels.WalletChallengeRequest{
			Address:  "0xabc",
			Username: "",
			ChainID:  1,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleCreateChallengeLift(ctx))
	})

	t.Run("create_challenge_defaults_chain_id_to_1", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/challenge", nil, nil, apimodels.WalletChallengeRequest{
			Address:  "0xabc",
			Username: "alice",
			ChainID:  0,
		})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleCreateChallengeLift(ctx))

		var challenge storage.WalletChallenge
		require.NoError(t, json.Unmarshal(resp.Body, &challenge))
		require.Equal(t, 1, challenge.ChainID)
	})

	t.Run("create_challenge_storage_error_returns_500", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: errors.New("create failed")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/challenge", nil, nil, apimodels.WalletChallengeRequest{
			Address:  "0xabc",
			Username: "alice",
			ChainID:  1,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleCreateChallengeLift(ctx))
	})

	t.Run("verify_signature_required_fields", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctxMissingAddress, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/verify", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleVerifySignatureLift(ctxMissingAddress))

		ctxMissingSignature, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/verify", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "0xabc",
			Signature:   "",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleVerifySignatureLift(ctxMissingSignature))

		ctxMissingMessage, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/verify", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "0xabc",
			Signature:   "sig",
			Message:     "",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleVerifySignatureLift(ctxMissingMessage))
	})

	t.Run("verify_signature_error_paths_call_handleAuthServiceError", func(t *testing.T) {
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
					ExpiresAt: now.Add(5 * time.Minute),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/verify", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "0xabc",
			Signature:   "not-hex",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleVerifySignatureLift(ctx))
	})

	t.Run("login_wallet_missing_challenge_id_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/login", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "",
			Address:     "0xabc",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleLoginWalletLift(ctx))
	})

	t.Run("login_wallet_missing_address_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/login", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleLoginWalletLift(ctx))
	})

	t.Run("login_wallet_error_calls_handleAuthServiceError", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"WALLET_CHALLENGE#missing": true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/login", nil, nil, auth.WalletVerifyRequest{
			ChallengeID: "missing",
			Address:     "0xabc",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleLoginWalletLift(ctx))
	})

	t.Run("link_wallet_address_required", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:    "",
			Username:   "alice",
			WalletType: "ethereum",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_get_challenge_error_returns_401", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"WALLET_CHALLENGE#missing": true},
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     "0xabc",
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "missing",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_username_mismatch_returns_401", func(t *testing.T) {
		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "bob", Address: "0xabc", ChainID: 1, Nonce: "n", Message: "msg", IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute)},
			},
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     "0xabc",
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_challenge_spent_returns_401", func(t *testing.T) {
		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: "0xabc", ChainID: 1, Nonce: "n", Message: "msg", IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), Spent: true},
			},
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     "0xabc",
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   "sig",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_unauthed_requires_typed_registration_completion", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		message := "hello"
		signature := round11SignWalletMessage(t, key, message)

		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: address, ChainID: 1, Nonce: "n", Message: message, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute)},
			},
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     address,
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   signature,
			Message:     message,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_signature_verification_failed_returns_401", func(t *testing.T) {
		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: "0xabc", ChainID: 1, Nonce: "n", Message: "msg", IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), RegistrationCompleted: true},
			},
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     "0xabc",
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   "not-hex",
			Message:     "msg",
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_mark_challenge_spent_failure_returns_500", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		message := "hello"
		signature := round11SignWalletMessage(t, key, message)

		state := &round10QueryState{
			updateErrorOnce: errors.New("update failed"),
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: address, ChainID: 1, Nonce: "n", Message: message, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), RegistrationCompleted: true},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     address,
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   signature,
			Message:     message,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleLinkWalletLift(ctx))
	})

	t.Run("link_wallet_link_wallet_error_restores_challenge_for_retry", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		message := "hello"
		signature := round11SignWalletMessage(t, key, message)

		state := &round10QueryState{
			createErrorOnce: errors.New("create failed"),
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: address, ChainID: 1, Nonce: "n", Message: message, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), RegistrationCompleted: true},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     address,
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   signature,
			Message:     message,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleLinkWalletLift(ctx))

		challenge := state.walletChallengesByID["c1"]
		require.False(t, challenge.Spent)
		require.Empty(t, state.walletCredentialsByAddress)

		ctxRetry, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     address,
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   signature,
			Message:     message,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(handler.HandleLinkWalletLift(ctxRetry))
		require.True(t, state.walletChallengesByID["c1"].Spent)
	})

	t.Run("link_wallet_session_creation_error_restores_retry_for_new_link", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		message := "hello"
		signature := round11SignWalletMessage(t, key, message)

		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Approved: false, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
			},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: address, ChainID: 1, Nonce: "n", Message: message, IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), RegistrationCompleted: true},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     address,
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   signature,
			Message:     message,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleLinkWalletLift(ctx))

		challenge := state.walletChallengesByID["c1"]
		require.False(t, challenge.Spent)
		require.Empty(t, state.walletCredentialsByAddress)

		user := state.usersByUsername["alice"]
		user.Approved = true
		state.usersByUsername["alice"] = user

		ctxRetry, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{
			Address:     address,
			Username:    "alice",
			WalletType:  "ethereum",
			ChallengeID: "c1",
			Signature:   signature,
			Message:     message,
		})
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(handler.HandleLinkWalletLift(ctxRetry))
		require.True(t, state.walletChallengesByID["c1"].Spent)
	})

	t.Run("unlink_wallet_unauthorized_returns_401", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/auth/wallet/unlink/0xabc", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["address"] = "0xabc"
		requireStatus(t, http.StatusUnauthorized)(handler.HandleUnlinkWalletLift(ctx))
	})

	t.Run("unlink_wallet_missing_address_param_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/auth/wallet/unlink/", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleUnlinkWalletLift(ctx))
	})

	t.Run("unlink_wallet_last_authenticator_returns_400", func(t *testing.T) {
		state := &round10QueryState{
			webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
				"alice": {},
			},
			walletCredentialsByUser: map[string][]storagemodels.WalletCredential{
				"alice": {
					{Username: "alice", Address: "0xabc", ChainID: 1, Type: "ethereum"},
				},
			},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authService, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)
		require.ErrorIs(t, authService.UnlinkWallet(context.Background(), "alice", "0xabc"), auth.ErrLastAuthMethodDelete)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/auth/wallet/unlink/0xabc", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["address"] = "0xabc"
		requireStatus(t, http.StatusBadRequest)(handler.HandleUnlinkWalletLift(ctx))
	})

	t.Run("unlink_wallet_delete_error_returns_500", func(t *testing.T) {
		state := &round10QueryState{deleteErrorOnce: errors.New("delete failed")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodDelete, "/auth/wallet/unlink/0xabc", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.Params["address"] = "0xabc"
		requireStatus(t, http.StatusInternalServerError)(handler.HandleUnlinkWalletLift(ctx))
	})

	t.Run("get_wallets_unauthorized_returns_401", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/wallet/list", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleGetWalletsLift(ctx))
	})

	t.Run("get_wallets_error_returns_500", func(t *testing.T) {
		state := &round10QueryState{
			allErrorByType: map[string]error{
				"*[]models.WalletCredential": errors.New("boom"),
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/auth/wallet/list", writeHeaders, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleGetWalletsLift(ctx))
	})

	t.Run("getAuthenticatedUserLift_invalid_token_returns_empty", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/any", map[string]string{"Authorization": "Bearer not-a-real-jwt"}, nil, nil)
		require.NoError(t, err)
		require.Empty(t, handler.getAuthenticatedUserLift(ctx))
	})

	t.Run("getAuthenticatedUserLift_auth_service_init_failure_returns_empty", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.JWTSecret = "short"
		handler, _, _ := round11NewHandler(t, badCfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/any", map[string]string{"Authorization": "Bearer any"}, nil, nil)
		require.NoError(t, err)
		require.Empty(t, handler.getAuthenticatedUserLift(ctx))
	})
}
