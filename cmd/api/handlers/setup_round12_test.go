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
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
)

func TestSetupStageURLsRound12(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	urls := handler.stageURLs()
	require.Contains(t, urls.WS, "wss://ws.example.com")
	require.Contains(t, urls.Media, "https://media.example.com")

	localCfg := &config.Config{
		Domain:          "localhost",
		JWTSecret:       cfg.JWTSecret,
		DynamoTableName: cfg.DynamoTableName,
		Stage:           cfg.Stage,
	}
	localHandler, _, _ := round11NewHandler(t, localCfg, &round10QueryState{})
	localURLs := localHandler.stageURLs()
	require.Contains(t, localURLs.WS, "ws://ws.localhost")
	require.Contains(t, localURLs.Media, "http://media.localhost")
}

func TestSetupStatusLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("success locked and defaults", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked: true,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/status", nil, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleSetupStatusLift(ctx))

		var body apimodels.SetupStatusResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "locked", body.InstanceState)
		require.True(t, body.Locked)
		require.Equal(t, storagemodels.DefaultBootstrapUsername, body.Bootstrap.Username)
		require.Equal(t, storagemodels.DefaultBootstrapUsername, body.BootstrapActor.Username)
	})

	t.Run("success unlocked returns active", func(t *testing.T) {
		now := time.Now().UTC().Add(-1 * time.Hour)
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked:               false,
				PrimaryAdminUsername: "admin",
				ActivatedAt:          &now,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/status", nil, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleSetupStatusLift(ctx))

		var body apimodels.SetupStatusResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "active", body.InstanceState)
		require.False(t, body.Locked)
		require.False(t, body.FinalizeAllowed)
	})

	t.Run("instance repo error returns 500", func(t *testing.T) {
		state := &round10QueryState{
			firstErrorOnce: errors.New("boom"),
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/status", nil, nil, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupStatusLift(ctx))
	})
}

func TestSetupBootstrapChallengeLiftRound12(t *testing.T) {
	cfg := round11TestConfig()
	bootstrapAddr := "0xabc"

	t.Run("instance repo error returns 500", func(t *testing.T) {
		state := &round10QueryState{
			firstErrorOnce: errors.New("boom"),
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{Address: bootstrapAddr})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupBootstrapChallengeLift(ctx))
	})

	t.Run("conflict when already activated", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: false},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{Address: bootstrapAddr})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupBootstrapChallengeLift(ctx))
	})

	t.Run("bad request when address missing", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: bootstrapAddr},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupBootstrapChallengeLift(ctx))
	})

	t.Run("conflict when bootstrap wallet not configured", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{Address: bootstrapAddr})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupBootstrapChallengeLift(ctx))
	})

	t.Run("forbidden when wallet mismatch", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: bootstrapAddr},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{Address: "0xdef"})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleSetupBootstrapChallengeLift(ctx))
	})

	t.Run("auth service create failure returns 500", func(t *testing.T) {
		state := &round10QueryState{
			instanceState:        &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: bootstrapAddr},
			createErrorOnce:      errors.New("create failed"),
			walletChallengesByID: map[string]storagemodels.WalletChallenge{},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{Address: bootstrapAddr})
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupBootstrapChallengeLift(ctx))
	})

	t.Run("success returns challenge", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: bootstrapAddr, BootstrapUsername: ""},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/challenge", nil, nil, apimodels.SetupBootstrapChallengeRequest{Address: bootstrapAddr})
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleSetupBootstrapChallengeLift(ctx))

		var body apimodels.SetupBootstrapChallengeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.ChallengeID)
		require.NotEmpty(t, body.Challenge)
		require.Equal(t, 1, body.ChainID)
		require.Equal(t, strings.ToLower(bootstrapAddr), strings.ToLower(body.Address))
		require.Equal(t, storagemodels.DefaultBootstrapUsername, body.Username)
	})
}

func TestSetupBootstrapVerifyLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("conflict when already activated", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: false},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("bad request missing fields", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{Address: "0xabc"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("bad request when address missing", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Signature: "sig", Message: "msg"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("bad request when signature missing", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Address: "0xabc", Message: "msg"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("bad request when message missing", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Address: "0xabc", Signature: "sig"})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("conflict when bootstrap wallet not configured", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Address: "0xabc", Signature: "sig", Message: "msg"})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("forbidden when wallet mismatch", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Address: "0xdef", Signature: "sig", Message: "msg"})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("unauthorized when challenge missing", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc"},
			notFoundPKs: map[string]bool{
				"WALLET_CHALLENGE#missing": true,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "missing", Address: "0xabc", Signature: "sig", Message: "msg"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("forbidden when challenge username mismatch", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc", BootstrapUsername: "bootstrap"},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: "0xabc", ChainID: 1, Message: "message", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(1 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Address: "0xabc", Signature: "sig", Message: "msg"})
		require.NoError(t, err)
		requireStatus(t, http.StatusForbidden)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("invalid signature handled", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: "0xabc", BootstrapUsername: "bootstrap"},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "bootstrap", Address: "0xabc", ChainID: 1, Message: "message", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(1 * time.Hour)},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, apimodels.SetupBootstrapVerifyRequest{ChallengeID: "c1", Address: "0xabc", Signature: "sig", Message: "message"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("create session failure returns 500", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		tmpHandler, tmpRepos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		tmpAuthSvc, err := auth.NewAuthService(cfg, tmpRepos)
		require.NoError(t, err)

		challenge, err := tmpAuthSvc.CreateWalletChallenge(context.Background(), address, 1, "bootstrap")
		require.NoError(t, err)

		state := &round10QueryState{
			instanceState:   &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: address, BootstrapUsername: "bootstrap"},
			createErrorOnce: errors.New("create failed"),
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}

		sig := round11SignWalletMessage(t, key, challenge.Message)
		req := apimodels.SetupBootstrapVerifyRequest{
			ChallengeIDSnake: challenge.ID,
			Address:          address,
			Signature:        sig,
			Challenge:        challenge.Message,
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, req)
		require.NoError(t, err)
		_ = tmpHandler
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupBootstrapVerifyLift(ctx))
	})

	t.Run("success returns setup token", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapWalletAddress: address, BootstrapUsername: ""},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), address, 1, storagemodels.DefaultBootstrapUsername)
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}

		sig := round11SignWalletMessage(t, key, challenge.Message)
		req := apimodels.SetupBootstrapVerifyRequest{
			ChallengeID: challenge.ID,
			Address:     address,
			Signature:   sig,
			Message:     challenge.Message,
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/bootstrap/verify", nil, nil, req)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(handler.HandleSetupBootstrapVerifyLift(ctx))

		var body apimodels.SetupBootstrapVerifyResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "Bearer", body.TokenType)
		require.NotEmpty(t, body.Token)
		require.NotEmpty(t, body.SetupToken)
	})
}

func TestRequireSetupSessionRound12(t *testing.T) {
	cfg := round11TestConfig()

	baseState := &storagemodels.InstanceState{
		Locked:                 true,
		BootstrapUsername:      "bootstrap",
		BootstrapWalletAddress: "0xabc",
	}

	t.Run("missing auth header unauthorized", func(t *testing.T) {
		state := &round10QueryState{instanceState: baseState}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("expired session unauthorized and deleted", func(t *testing.T) {
		expired := storagemodels.SetupSession{
			ID:           "expired",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:    time.Now().Add(-1 * time.Minute),
			InstanceLock: true,
		}
		require.NoError(t, expired.UpdateKeys())
		state := &round10QueryState{
			instanceState:     baseState,
			setupSessionsByID: map[string]storagemodels.SetupSession{expired.ID: expired},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer " + expired.ID}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("missing session unauthorized", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: baseState,
			notFoundPKSK: map[string]bool{
				"SETUP_SESSION#missing#SESSION": true,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer missing"}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("storage error returns 500", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: baseState,
			firstErrorPK: map[string]error{
				"SETUP_SESSION#token": errors.New("query failed"),
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer token"}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("wrong purpose unauthorized", func(t *testing.T) {
		session := storagemodels.SetupSession{
			ID:           "s1",
			Purpose:      "other",
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			InstanceLock: true,
		}
		require.NoError(t, session.UpdateKeys())
		state := &round10QueryState{
			instanceState:     baseState,
			setupSessionsByID: map[string]storagemodels.SetupSession{session.ID: session},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer " + session.ID}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("wallet mismatch unauthorized", func(t *testing.T) {
		session := storagemodels.SetupSession{
			ID:           "s1",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xdef",
			IssuedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			InstanceLock: true,
		}
		require.NoError(t, session.UpdateKeys())
		state := &round10QueryState{
			instanceState:     baseState,
			setupSessionsByID: map[string]storagemodels.SetupSession{session.ID: session},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer " + session.ID}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("bootstrap username mismatch unauthorized", func(t *testing.T) {
		session := storagemodels.SetupSession{
			ID:           "s1",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			InstanceLock: true,
		}
		require.NoError(t, session.UpdateKeys())
		state := &round10QueryState{
			instanceState:     baseState,
			setupSessionsByID: map[string]storagemodels.SetupSession{session.ID: session},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer " + session.ID}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		_, resp, err := handler.requireSetupSession(ctx, "wrong", baseState)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("success returns session", func(t *testing.T) {
		session := storagemodels.SetupSession{
			ID:           "s1",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-2 * time.Hour),
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			InstanceLock: true,
		}
		require.NoError(t, session.UpdateKeys())
		state := &round10QueryState{
			instanceState:     baseState,
			setupSessionsByID: map[string]storagemodels.SetupSession{session.ID: session},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		headers := map[string]string{"Authorization": "Bearer " + session.ID}
		ctx, err := round10NewLiftContext(http.MethodGet, "/setup/admin", headers, nil, nil)
		require.NoError(t, err)

		got, resp, err := handler.requireSetupSession(ctx, "bootstrap", baseState)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.NotNil(t, got)
		require.Equal(t, session.ID, got.ID)
	})
}

func TestEnsureSetupAdminAccountRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("registry missing returns 500", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)
		_, resp, err := handler.ensureSetupAdminAccount(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("register account success uses actor id", func(t *testing.T) {
		state := &round10QueryState{}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return &accounts.RegisterAccountResult{
					Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/new", Type: "Person"}},
				}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)
		actorID, resp, err := handler.ensureSetupAdminAccount(ctx, "new")
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, "https://example.com/users/new", actorID)
	})

	t.Run("username already taken falls back to actor repo", func(t *testing.T) {
		state := &round10QueryState{
			actorsByUser: map[string]storagemodels.Actor{
				"alice": {Username: "alice", Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice", Type: "Person"}}},
			},
		}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return nil, accounts.ErrUsernameAlreadyTaken
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)
		actorID, resp, err := handler.ensureSetupAdminAccount(ctx, "alice")
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, "https://example.com/users/alice", actorID)
	})

	t.Run("register error returns 422", func(t *testing.T) {
		state := &round10QueryState{}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return nil, errors.New("register failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)
		_, resp, err := handler.ensureSetupAdminAccount(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
	})

	t.Run("username taken but actor lookup fails returns 422", func(t *testing.T) {
		state := &round10QueryState{
			firstErrorPK: map[string]error{
				"ACTOR#alice": errors.New("actor lookup failed"),
			},
		}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return nil, accounts.ErrUsernameAlreadyTaken
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)
		_, resp, err := handler.ensureSetupAdminAccount(ctx, "alice")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnprocessableEntity, resp.Status)
	})

	t.Run("register returns no actor falls back to config URL", func(t *testing.T) {
		state := &round10QueryState{}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return &accounts.RegisterAccountResult{}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)
		actorID, resp, err := handler.ensureSetupAdminAccount(ctx, "alice")
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, cfg.ActorURL("alice"), actorID)
	})
}

func TestVerifySetupCreateAdminWalletRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("missing fields rejected", func(t *testing.T) {
		state := &round10QueryState{}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		authSvc, err := auth.NewAuthService(cfg, handler.repos)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("missing address rejected", func(t *testing.T) {
		state := &round10QueryState{}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{ChallengeID: "c1"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("missing signature rejected", func(t *testing.T) {
		state := &round10QueryState{}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{ChallengeID: "c1", Address: "0xabc"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("missing message rejected", func(t *testing.T) {
		state := &round10QueryState{}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{ChallengeID: "c1", Address: "0xabc", Signature: "sig"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("challenge not found unauthorized", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"WALLET_CHALLENGE#missing": true},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{ChallengeID: "missing", Address: "0xabc", Signature: "sig", Message: "msg"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("challenge username mismatch forbidden", func(t *testing.T) {
		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "bob", Address: "0xabc", ChainID: 1, Message: "message", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(1 * time.Hour)},
			},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "0xabc",
			Signature:   "sig",
			Message:     "message",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("wallet index lookup error returns 500", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		state := &round10QueryState{
			allErrorOnce: errors.New("query failed"),
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), address, 1, "alice")
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}
		sig := round11SignWalletMessage(t, key, challenge.Message)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{
			ChallengeID: challenge.ID,
			Address:     address,
			Signature:   sig,
			Message:     challenge.Message,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("invalid signature returns error response", func(t *testing.T) {
		state := &round10QueryState{
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				"c1": {ID: "c1", Username: "alice", Address: "0xabc", ChainID: 1, Message: "message", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(1 * time.Hour)},
			},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{
			ChallengeID: "c1",
			Address:     "0xabc",
			Signature:   "sig",
			Message:     "message",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("wallet already linked conflict", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		state := &round10QueryState{
			walletCredentialsByAddress: map[string]storagemodels.WalletCredential{
				address: {Username: "someone", Type: "ethereum", Address: address},
			},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), address, 1, "alice")
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}
		sig := round11SignWalletMessage(t, key, challenge.Message)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		_, _, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{
			ChallengeID: challenge.ID,
			Address:     address,
			Signature:   sig,
			Message:     challenge.Message,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusConflict, resp.Status)
	})

	t.Run("success returns chain and address", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		state := &round10QueryState{}
		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), address, 1, "alice")
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}
		sig := round11SignWalletMessage(t, key, challenge.Message)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, nil)
		require.NoError(t, err)

		chainID, addr, resp, err := handler.verifySetupCreateAdminWallet(ctx, authSvc, "alice", auth.WalletVerifyRequest{
			ChallengeID: challenge.ID,
			Address:     address,
			Signature:   sig,
			Message:     challenge.Message,
		})
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, 1, chainID)
		require.Equal(t, strings.ToLower(address), addr)
	})
}

func TestSetupCreateAdminLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("conflict when already activated", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: false},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, apimodels.SetupCreateAdminRequest{Username: "alice"})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("conflict when primary admin already created", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, PrimaryAdminUsername: "admin"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, apimodels.SetupCreateAdminRequest{Username: "alice"})
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("requires setup session token", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapUsername: "bootstrap"},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", nil, nil, apimodels.SetupCreateAdminRequest{Username: "alice"})
		require.NoError(t, err)
		requireStatus(t, http.StatusUnauthorized)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("bad request when username missing", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapUsername: "bootstrap", BootstrapWalletAddress: "0xabc"},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				"token": func() storagemodels.SetupSession {
					sess := storagemodels.SetupSession{
						ID:           "token",
						Purpose:      setupSessionPurposeBootstrap,
						WalletType:   "ethereum",
						WalletAddr:   "0xabc",
						IssuedAt:     time.Now().Add(-1 * time.Minute),
						ExpiresAt:    time.Now().Add(30 * time.Minute),
						InstanceLock: true,
					}
					_ = sess.UpdateKeys()
					return sess
				}(),
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		headers := map[string]string{"Authorization": "Bearer token"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, apimodels.SetupCreateAdminRequest{})
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("bad request when username is reserved", func(t *testing.T) {
		sess := storagemodels.SetupSession{
			ID:           "token",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-1 * time.Minute),
			ExpiresAt:    time.Now().Add(30 * time.Minute),
			InstanceLock: true,
		}
		require.NoError(t, sess.UpdateKeys())

		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapUsername: "bootstrap", BootstrapWalletAddress: "0xabc"},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				sess.ID: sess,
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})

		headers := map[string]string{"Authorization": "Bearer " + sess.ID}
		req := apimodels.SetupCreateAdminRequest{Username: "bootstrap"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, req)
		require.NoError(t, err)
		requireStatus(t, http.StatusBadRequest)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("wallet already linked returns conflict", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapUsername: "bootstrap", BootstrapWalletAddress: "0xabc"},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				"token": func() storagemodels.SetupSession {
					sess := storagemodels.SetupSession{
						ID:           "token",
						Purpose:      setupSessionPurposeBootstrap,
						WalletType:   "ethereum",
						WalletAddr:   "0xabc",
						IssuedAt:     time.Now().Add(-1 * time.Minute),
						ExpiresAt:    time.Now().Add(30 * time.Minute),
						InstanceLock: true,
					}
					_ = sess.UpdateKeys()
					return sess
				}(),
			},
			walletCredentialsByAddress: map[string]storagemodels.WalletCredential{
				"0xdef": {Username: "someone", Type: "ethereum", Address: "0xdef"},
			},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: &AccountsServiceStub{}})
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), "0xdef", 1, "admin")
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}

		headers := map[string]string{"Authorization": "Bearer token"}
		req := apimodels.SetupCreateAdminRequest{
			Username: "admin",
			Wallet: auth.WalletVerifyRequest{
				ChallengeID: challenge.ID,
				Address:     "0xdef",
				Signature:   "sig",
				Message:     challenge.Message,
			},
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, req)
		require.NoError(t, err)
		requireStatus(t, http.StatusConflict)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("update user failure returns 500", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		tmpHandler, tmpRepos, _ := round11NewHandler(t, cfg, &round10QueryState{})
		tmpAuthSvc, err := auth.NewAuthService(cfg, tmpRepos)
		require.NoError(t, err)

		challenge, err := tmpAuthSvc.CreateWalletChallenge(context.Background(), address, 1, "admin")
		require.NoError(t, err)
		sig := round11SignWalletMessage(t, key, challenge.Message)

		sess := storagemodels.SetupSession{
			ID:           "token",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-1 * time.Minute),
			ExpiresAt:    time.Now().Add(30 * time.Minute),
			InstanceLock: true,
		}
		require.NoError(t, sess.UpdateKeys())

		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapUsername: "bootstrap", BootstrapWalletAddress: "0xabc"},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				sess.ID: sess,
			},
			firstErrorPK: map[string]error{
				"USER#admin": errors.New("user lookup failed"),
			},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				challenge.ID: {
					ID:        challenge.ID,
					Username:  challenge.Username,
					Address:   challenge.Address,
					ChainID:   challenge.ChainID,
					Nonce:     challenge.Nonce,
					Message:   challenge.Message,
					IssuedAt:  challenge.IssuedAt,
					ExpiresAt: challenge.ExpiresAt,
				},
			},
		}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return &accounts.RegisterAccountResult{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("admin"), Type: "Person"}}}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		headers := map[string]string{"Authorization": "Bearer " + sess.ID}
		req := apimodels.SetupCreateAdminRequest{
			Username: "admin",
			Wallet: auth.WalletVerifyRequest{
				ChallengeID: challenge.ID,
				Address:     address,
				Signature:   sig,
				Message:     challenge.Message,
			},
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, req)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupCreateAdminLift(ctx))
		_ = tmpHandler
	})

	t.Run("link wallet failure returns 500", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)
		tmpAuthState := &round10QueryState{}
		_, tmpRepos, _ := round11NewHandler(t, cfg, tmpAuthState)
		tmpAuthSvc, err := auth.NewAuthService(cfg, tmpRepos)
		require.NoError(t, err)

		challenge, err := tmpAuthSvc.CreateWalletChallenge(context.Background(), address, 1, "admin")
		require.NoError(t, err)
		sig := round11SignWalletMessage(t, key, challenge.Message)

		sess := storagemodels.SetupSession{
			ID:           "token",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-1 * time.Minute),
			ExpiresAt:    time.Now().Add(30 * time.Minute),
			InstanceLock: true,
		}
		require.NoError(t, sess.UpdateKeys())

		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true, BootstrapUsername: "bootstrap", BootstrapWalletAddress: "0xabc"},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				sess.ID: sess,
			},
			walletChallengesByID: map[string]storagemodels.WalletChallenge{
				challenge.ID: {
					ID:        challenge.ID,
					Username:  challenge.Username,
					Address:   challenge.Address,
					ChainID:   challenge.ChainID,
					Nonce:     challenge.Nonce,
					Message:   challenge.Message,
					IssuedAt:  challenge.IssuedAt,
					ExpiresAt: challenge.ExpiresAt,
				},
			},
			createErrorOnce: errors.New("create failed"),
		}
		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return &accounts.RegisterAccountResult{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("admin"), Type: "Person"}}}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})

		headers := map[string]string{"Authorization": "Bearer " + sess.ID}
		req := apimodels.SetupCreateAdminRequest{
			Username: "admin",
			Wallet: auth.WalletVerifyRequest{
				ChallengeID: challenge.ID,
				Address:     address,
				Signature:   sig,
				Message:     challenge.Message,
			},
		}
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, req)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("registry missing returns 500", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)

		sess := storagemodels.SetupSession{
			ID:           "token",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-1 * time.Minute),
			ExpiresAt:    time.Now().Add(30 * time.Minute),
			InstanceLock: true,
		}
		require.NoError(t, sess.UpdateKeys())

		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked:                 true,
				BootstrapUsername:      "bootstrap",
				BootstrapWalletAddress: "0xabc",
			},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				sess.ID: sess,
			},
		}

		handler, repos, _ := round11NewHandler(t, cfg, state)
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), address, 1, "admin")
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}

		sig := round11SignWalletMessage(t, key, challenge.Message)
		headers := map[string]string{"Authorization": "Bearer " + sess.ID}
		req := apimodels.SetupCreateAdminRequest{
			Username: "admin",
			Wallet: auth.WalletVerifyRequest{
				ChallengeID: challenge.ID,
				Address:     address,
				Signature:   sig,
				Message:     challenge.Message,
			},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, req)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(handler.HandleSetupCreateAdminLift(ctx))
	})

	t.Run("success creates admin", func(t *testing.T) {
		key, address := round11GenerateWalletKey(t)

		sess := storagemodels.SetupSession{
			ID:           "token",
			Purpose:      setupSessionPurposeBootstrap,
			WalletType:   "ethereum",
			WalletAddr:   "0xabc",
			IssuedAt:     time.Now().Add(-1 * time.Minute),
			ExpiresAt:    time.Now().Add(30 * time.Minute),
			InstanceLock: true,
		}
		require.NoError(t, sess.UpdateKeys())

		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked:                 true,
				BootstrapUsername:      "bootstrap",
				BootstrapWalletAddress: "0xabc",
			},
			setupSessionsByID: map[string]storagemodels.SetupSession{
				sess.ID: sess,
			},
		}

		accountSvc := &AccountsServiceStub{
			RegisterAccountFunc: func(context.Context, *accounts.RegisterAccountCommand) (*accounts.RegisterAccountResult, error) {
				return &accounts.RegisterAccountResult{
					Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: cfg.ActorURL("admin"), Type: "Person"}},
				}, nil
			},
		}
		handler, repos, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountSvc})
		authSvc, err := auth.NewAuthService(cfg, repos)
		require.NoError(t, err)

		challenge, err := authSvc.CreateWalletChallenge(context.Background(), address, 1, "admin")
		require.NoError(t, err)
		state.walletChallengesByID = map[string]storagemodels.WalletChallenge{
			challenge.ID: {
				ID:        challenge.ID,
				Username:  challenge.Username,
				Address:   challenge.Address,
				ChainID:   challenge.ChainID,
				Nonce:     challenge.Nonce,
				Message:   challenge.Message,
				IssuedAt:  challenge.IssuedAt,
				ExpiresAt: challenge.ExpiresAt,
			},
		}
		sig := round11SignWalletMessage(t, key, challenge.Message)

		headers := map[string]string{"Authorization": "Bearer " + sess.ID}
		req := apimodels.SetupCreateAdminRequest{
			Username:    "admin",
			DisplayName: "Admin",
			Wallet: auth.WalletVerifyRequest{
				ChallengeID: challenge.ID,
				Address:     address,
				Signature:   sig,
				Message:     challenge.Message,
			},
		}

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/admin", headers, nil, req)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusCreated)(handler.HandleSetupCreateAdminLift(ctx))

		var body apimodels.SetupCreateAdminResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "admin", body.Username)
		require.NotEmpty(t, body.Actor)
	})
}

func TestSetupFinalizeLiftRound12(t *testing.T) {
	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("requires auth (middleware handles)", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/finalize", nil, nil, nil)
		require.NoError(t, err)
		resp := handleWithAPIMiddleware(t, handler.HandleSetupFinalizeLift, ctx)
		require.Equal(t, http.StatusUnauthorized, resp.Status)
	})

	t.Run("forbidden when not admin role", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: true},
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: "user"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/finalize", headers, nil, nil)
		require.NoError(t, err)
		resp := handleWithAPIMiddleware(t, handler.HandleSetupFinalizeLift, ctx)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("conflict when already activated", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{Locked: false},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/finalize", headers, nil, nil)
		require.NoError(t, err)
		resp := handleWithAPIMiddleware(t, handler.HandleSetupFinalizeLift, ctx)
		require.Equal(t, http.StatusConflict, resp.Status)
	})

	t.Run("forbidden when not primary admin", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked:               true,
				PrimaryAdminUsername: "primary",
			},
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: roleAdmin, Approved: true, Version: 1},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/finalize", headers, nil, nil)
		require.NoError(t, err)
		resp := handleWithAPIMiddleware(t, handler.HandleSetupFinalizeLift, ctx)
		require.Equal(t, http.StatusForbidden, resp.Status)
	})

	t.Run("unlock failure returns 500", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked:               true,
				PrimaryAdminUsername: "admin",
			},
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: roleAdmin, Approved: true, Version: 1},
			},
			updateErrorOnce: errors.New("update failed"),
		}
		handler, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/finalize", headers, nil, nil)
		require.NoError(t, err)
		resp := handleWithAPIMiddleware(t, handler.HandleSetupFinalizeLift, ctx)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})

	t.Run("success unlocks instance", func(t *testing.T) {
		state := &round10QueryState{
			instanceState: &storagemodels.InstanceState{
				Locked:                 true,
				BootstrapUsername:      storagemodels.DefaultBootstrapUsername,
				BootstrapWalletAddress: "0xabc",
				PrimaryAdminUsername:   "admin",
			},
			usersByUsername: map[string]storagemodels.User{
				"admin": {Username: "admin", Role: roleAdmin, Approved: true, Version: 1},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/setup/finalize", headers, nil, nil)
		require.NoError(t, err)
		resp := handleWithAPIMiddleware(t, handler.HandleSetupFinalizeLift, ctx)
		require.Equal(t, http.StatusOK, resp.Status)

		var body apimodels.SetupFinalizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "active", body.InstanceState)
		require.False(t, body.Locked)
	})
}

func TestSetupErrorHelpersRound12(t *testing.T) {
	require.True(t, apperrors.HasCode(apperrors.NotFound("x"), apperrors.CodeNotFound))
	require.True(t, dynamormerrors.IsNotFound(dynamormerrors.ErrItemNotFound))
}
