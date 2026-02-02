package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestWebAuthnBeginHandlers(t *testing.T) {
	state := &round10QueryState{
		sessionsByID: map[string]storagemodels.Session{
			"sess-1": {SessionID: "sess-1", UserID: "USER#alice", ExpiresAt: time.Now().Add(1 * time.Hour).Unix()},
		},
		webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
			"alice": {{ID: "Y3JlZA==", UserID: "alice", PublicKey: []byte{0x01, 0x02}, CreatedAt: time.Now()}},
		},
	}
	h, _, _ := round11NewHandlerSliceC(t, state)
	h.cfg.JWTSecret = round11StrongJWTSecret

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"read"}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxBegin, err := round10NewLiftContext(http.MethodPost, "/api/v1/auth/webauthn/register/begin", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleBeginWebAuthnRegistrationLift(ctxBegin))

	ctxBeginLogin := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/login/begin", headers, nil, round11JSONBody(t, models.WebAuthnBeginLoginRequest{Username: "alice"}))
	requireStatus(t, http.StatusOK)(h.HandleBeginWebAuthnLoginLift(ctxBeginLogin))
}

func TestWebAuthnFinishAndManageHandlers(t *testing.T) {
	state := &round10QueryState{
		sessionsByID: map[string]storagemodels.Session{
			"sess-1": {SessionID: "sess-1", UserID: "USER#alice", ExpiresAt: time.Now().Add(1 * time.Hour).Unix()},
		},
		webAuthnCredentialByID: map[string]storagemodels.WebAuthnCredential{
			"Y3JlZA==": {ID: "Y3JlZA==", UserID: "alice", PublicKey: []byte{0x01, 0x02}, CreatedAt: time.Now()},
			"Y3JlZDI=": {ID: "Y3JlZDI=", UserID: "alice", PublicKey: []byte{0x03, 0x04}, CreatedAt: time.Now()},
		},
		webAuthnCredentialsByUser: map[string][]storagemodels.WebAuthnCredential{
			"alice": {
				{ID: "Y3JlZA==", UserID: "alice", PublicKey: []byte{0x01, 0x02}, CreatedAt: time.Now()},
				{ID: "Y3JlZDI=", UserID: "alice", PublicKey: []byte{0x03, 0x04}, CreatedAt: time.Now()},
			},
		},
		notFoundPKs: map[string]bool{
			"CHALLENGE#missing": true,
		},
	}
	h, _, _ := round11NewHandlerSliceC(t, state)
	h.cfg.JWTSecret = round11StrongJWTSecret

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{"read"}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}

	finishReq := models.WebAuthnFinishRegistrationRequest{Challenge: "missing", Response: map[string]any{"foo": "bar"}}
	ctxFinish := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/register/finish", headers, nil, round11JSONBody(t, finishReq))
	requireStatus(t, http.StatusBadRequest)(h.HandleFinishWebAuthnRegistrationLift(ctxFinish))

	finishLoginReq := models.WebAuthnFinishLoginRequest{Username: "alice", Challenge: "missing", Response: map[string]any{"foo": "bar"}}
	ctxFinishLogin := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/auth/webauthn/login/finish", headers, nil, round11JSONBody(t, finishLoginReq))
	requireStatus(t, http.StatusBadRequest)(h.HandleFinishWebAuthnLoginLift(ctxFinishLogin))

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/auth/webauthn/credentials", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleListWebAuthnCredentialsLift(ctxList))

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/auth/webauthn/credentials/Y3JlZA==", headers, nil, nil)
	require.NoError(t, err)
	ctxDelete.Params["credentialId"] = "Y3JlZA=="
	requireStatus(t, http.StatusOK)(h.HandleDeleteWebAuthnCredentialLift(ctxDelete))

	updateReq := models.WebAuthnUpdateCredentialRequest{Name: "Updated"}
	ctxUpdate := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/auth/webauthn/credentials/Y3JlZA==", headers, nil, round11JSONBody(t, updateReq))
	ctxUpdate.Params["credentialId"] = "Y3JlZA=="
	requireStatus(t, http.StatusOK)(h.HandleUpdateWebAuthnCredentialNameLift(ctxUpdate))
}
