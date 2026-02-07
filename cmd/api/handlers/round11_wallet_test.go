package handlers

import (
	"context"
	"crypto/ecdsa"
	"net/http"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func round11GenerateWalletKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	return key, address
}

func round11SignWalletMessage(t *testing.T, key *ecdsa.PrivateKey, message string) string {
	hash := accounts.TextHash([]byte(message))
	sig, err := crypto.Sign(hash, key)
	require.NoError(t, err)
	return hexutil.Encode(sig)
}

func TestWalletHandlers_FullFlow(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	key, address := round11GenerateWalletKey(t)

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
		walletCredentialsByUser: map[string][]storagemodels.WalletCredential{
			"alice": {{Username: "alice", Address: address, ChainID: 1, Type: "ethereum", LinkedAt: now.Add(-2 * time.Hour)}},
		},
	}

	handler, repos, _ := round11NewHandler(t, cfg, state)
	authService, err := auth.NewAuthService(cfg, repos)
	require.NoError(t, err)

	challenge, err := authService.CreateWalletChallenge(context.Background(), address, 1, "alice")
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
			Used:      challenge.Used,
			Spent:     challenge.Spent,
		},
	}

	signature := round11SignWalletMessage(t, key, challenge.Message)

	verifyReq := auth.WalletVerifyRequest{
		ChallengeID: challenge.ID,
		Address:     address,
		Signature:   signature,
		Message:     challenge.Message,
	}

	ctxVerify, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/verify", nil, nil, verifyReq)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleVerifySignatureLift(ctxVerify))

	ctxLogin, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/login", nil, nil, verifyReq)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleLoginWalletLift(ctxLogin))

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite, auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxLink, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", headers, nil, apimodels.WalletLinkRequest{
		Address:     address,
		ChainID:     1,
		WalletType:  "ethereum",
		ChallengeID: challenge.ID,
		Signature:   signature,
		Message:     challenge.Message,
	})
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleLinkWalletLift(ctxLink))

	ctxUnlink, err := round10NewLiftContext(http.MethodDelete, "/auth/wallet/unlink/"+address, headers, nil, nil)
	require.NoError(t, err)
	ctxUnlink.Params["address"] = address
	requireStatus(t, http.StatusOK)(handler.HandleUnlinkWalletLift(ctxUnlink))

	ctxList, err := round10NewLiftContext(http.MethodGet, "/auth/wallet/list", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetWalletsLift(ctxList))
}

func TestWalletHandlers_CreateAndLinkRegistration(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	key, address := round11GenerateWalletKey(t)

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
	}

	handler, repos, _ := round11NewHandler(t, cfg, state)
	authService, err := auth.NewAuthService(cfg, repos)
	require.NoError(t, err)

	challenge, err := authService.CreateWalletChallenge(context.Background(), address, 1, "alice")
	require.NoError(t, err)

	// Registration flow: wallet linking without an auth token must match the
	// challenge ID that registration stored on the user record.
	user := state.usersByUsername["alice"]
	user.Metadata = map[string]interface{}{"registration_challenge_id": challenge.ID}
	state.usersByUsername["alice"] = user

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

	signature := round11SignWalletMessage(t, key, challenge.Message)

	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/challenge", nil, nil, apimodels.WalletChallengeRequest{Address: address, Username: "alice", ChainID: 1})
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleCreateChallengeLift(ctxCreate))

	ctxLink, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{Address: address, Username: "alice", ChainID: 1, WalletType: "ethereum", ChallengeID: challenge.ID, Signature: signature, Message: challenge.Message})
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleLinkWalletLift(ctxLink))
}

func TestWalletHandlers_ValidationErrors(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	ctxMissing, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/challenge", nil, nil, apimodels.WalletChallengeRequest{})
	require.NoError(t, err)
	requireStatus(t, http.StatusBadRequest)(handler.HandleCreateChallengeLift(ctxMissing))

	ctxLink, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/link", nil, nil, apimodels.WalletLinkRequest{Address: "0xabc"})
	require.NoError(t, err)
	requireStatus(t, http.StatusUnauthorized)(handler.HandleLinkWalletLift(ctxLink))

	ctxVerify, err := round10NewLiftContext(http.MethodPost, "/auth/wallet/verify", nil, nil, auth.WalletVerifyRequest{})
	require.NoError(t, err)
	requireStatus(t, http.StatusBadRequest)(handler.HandleVerifySignatureLift(ctxVerify))
}
