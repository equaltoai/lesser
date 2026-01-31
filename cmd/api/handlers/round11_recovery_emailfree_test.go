package lift

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestEmailFreeRecoveryOptionsAndSocialFlow(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
		webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
			"alice": {{ID: "cred-1", UserID: "alice", Name: "Test Key", CreatedAt: now.Add(-2 * time.Hour)}},
		},
		walletCredentialsByUser: map[string][]storagemodels.WalletCredential{
			"alice": {{Username: "alice", Address: "0xabc", ChainID: 1, Type: "ethereum", LinkedAt: now.Add(-2 * time.Hour)}},
		},
		trusteesByUser: map[string][]storagemodels.Trustee{
			"alice": {
				{Username: "alice", ActorID: "@trustee1@example.com", AddedAt: now.Add(-2 * time.Hour), Confirmed: true},
				{Username: "alice", ActorID: "@trustee2@example.com", AddedAt: now.Add(-2 * time.Hour), Confirmed: true},
			},
		},
		recoveryCodesByUser: map[string][]storagemodels.RecoveryCode{
			"alice": {{Username: "alice", CodeHash: "hash", CreatedAt: now.Add(-2 * time.Hour), Position: 0}},
		},
	}

	_, repos, _ := round11NewHandler(t, cfg, state)
	authService, err := auth.NewAuthService(cfg, repos)
	require.NoError(t, err)
	handler := NewEmailFreeRecoveryHandler(authService)

	ctxOptions, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/options", nil, map[string]string{"username": "alice"}, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleGetRecoveryOptionsLift(ctxOptions))
	require.Equal(t, http.StatusOK, ctxOptions.Response.StatusCode)

	ctxInit, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/social/initiate", nil, nil, map[string]string{"username": "alice"})
	require.NoError(t, err)
	require.NoError(t, handler.HandleInitiateSocialRecoveryLift(ctxInit))

	state.recoveryRequestsByID = map[string]storagemodels.RecoveryRequest{
		"req-1": {
			ID:            "req-1",
			Username:      "alice",
			InitiatedAt:   now.Add(-1 * time.Hour),
			ExpiresAt:     now.Add(24 * time.Hour),
			RequiredVotes: 2,
			ReceivedVotes: map[string]bool{},
			Status:        "pending",
		},
	}
	ctxConfirm, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/social/confirm", nil, nil, map[string]string{"request_id": "req-1", "trustee_id": "@trustee1@example.com"})
	require.NoError(t, err)
	require.NoError(t, handler.HandleConfirmSocialRecoveryLift(ctxConfirm))
	require.Equal(t, http.StatusOK, ctxConfirm.Response.StatusCode)
}

func TestEmailFreeRecoveryCodesAndDevices(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	code := "ABCD-EFGH-IJKL-MNOP"
	normalized := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	hash, err := auth.HashPassword(normalized)
	require.NoError(t, err)

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
		recoveryCodesByUser: map[string][]storagemodels.RecoveryCode{
			"alice": {{Username: "alice", CodeHash: hash, CreatedAt: now.Add(-2 * time.Hour), Position: 0}},
		},
		devicesByID: map[string]storagemodels.Device{
			"device-1": {DeviceID: "device-1", Username: "alice", DeviceName: "iPhone", TrustLevel: "trusted"},
		},
	}

	_, repos, _ := round11NewHandler(t, cfg, state)
	authService, err := auth.NewAuthService(cfg, repos)
	require.NoError(t, err)
	handler := NewEmailFreeRecoveryHandler(authService)

	ctxGenerate, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/generate", nil, nil, nil)
	require.NoError(t, err)
	ctxGenerate.Context = context.WithValue(ctxGenerate.Context, "jwt_claims", map[string]any{"sub": "alice"})
	require.NoError(t, handler.HandleGenerateRecoveryCodesLift(ctxGenerate))
	require.Equal(t, http.StatusOK, ctxGenerate.Response.StatusCode)

	ctxUse, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/codes/use", nil, nil, map[string]string{"username": "alice", "code": code})
	require.NoError(t, err)
	require.NoError(t, handler.HandleUseRecoveryCodeLift(ctxUse))
	require.Equal(t, http.StatusOK, ctxUse.Response.StatusCode)

	ctxTrusteeAdd, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/trustees/add", nil, nil, map[string]string{"trustee_actor_id": "@bob@example.com"})
	require.NoError(t, err)
	ctxTrusteeAdd.Context = context.WithValue(ctxTrusteeAdd.Context, "jwt_claims", map[string]any{"sub": "alice"})
	require.NoError(t, handler.HandleAddTrusteeLift(ctxTrusteeAdd))

	ctxTrustees, err := round10NewLiftContext(http.MethodGet, "/auth/recovery/trustees", nil, nil, nil)
	require.NoError(t, err)
	ctxTrustees.Context = context.WithValue(ctxTrustees.Context, "jwt_claims", map[string]any{"sub": "alice"})
	require.NoError(t, handler.HandleListTrusteesLift(ctxTrustees))

	ctxRemove, err := round10NewLiftContext(http.MethodDelete, "/auth/recovery/trustees/trustee-1", nil, nil, nil)
	require.NoError(t, err)
	ctxRemove.Context = context.WithValue(ctxRemove.Context, "jwt_claims", map[string]any{"sub": "alice"})
	ctxRemove.SetParam("trustee_id", "@bob@example.com")
	require.NoError(t, handler.HandleRemoveTrusteeLift(ctxRemove))

	ctxDevice, err := round10NewLiftContext(http.MethodPost, "/auth/recovery/device", nil, nil, map[string]string{"username": "alice", "device_id": "device-1"})
	require.NoError(t, err)
	require.NoError(t, handler.HandleDeviceRecoveryLift(ctxDevice))
	require.Equal(t, http.StatusOK, ctxDevice.Response.StatusCode)
}
