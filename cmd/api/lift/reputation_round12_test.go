package lift

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func round12StoredReputationModel(t *testing.T, actorID string, totalScore float64) storagemodels.Reputation {
	t.Helper()

	rep := storage.Reputation{
		ActorID:       actorID,
		InstanceURL:   "https://example.com",
		TotalScore:    totalScore,
		TrustScore:    totalScore,
		ActivityScore: totalScore,
		CalculatedAt:  time.Now().UTC().Truncate(time.Second),
		Version:       1,
	}

	repJSON, err := json.Marshal(rep)
	require.NoError(t, err)

	model := storagemodels.Reputation{}
	require.NoError(t, model.UpdateKeys(actorID, rep))
	model.ReputationData = string(repJSON)
	return model
}

func round12VouchModel(t *testing.T, vouch storage.Vouch) *storagemodels.Vouch {
	t.Helper()

	vouchJSON, err := json.Marshal(vouch)
	require.NoError(t, err)

	model := &storagemodels.Vouch{
		VouchData: string(vouchJSON),
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if vouch.ExpiresAt != nil {
		expiresAt = *vouch.ExpiresAt
	}
	model.UpdateKeys(vouch.ID, vouch.From, vouch.To, vouch.Active, vouch.CreatedAt, expiresAt)
	return model
}

func TestReputationHandlersRound12(t *testing.T) {
	cfg := round11TestConfig()

	aliceActorID := "https://example.com/users/alice"
	alicePK := "ACTOR#alice"

	repModel := round12StoredReputationModel(t, aliceActorID, 600)

	vouch := storage.Vouch{
		ID:                "vouch-1",
		From:              "https://example.com/users/bob",
		To:                aliceActorID,
		Active:            true,
		Revoked:           false,
		CreatedAt:         time.Now().Add(-2 * time.Hour),
		ExpiresAt:         ptrTime(time.Now().Add(24 * time.Hour).Truncate(time.Second)),
		Confidence:        0.9,
		Context:           "context",
		VoucherReputation: 900,
	}
	vouchModel := round12VouchModel(t, vouch)

	state := &round10QueryState{
		reputationsByPK: map[string][]storagemodels.Reputation{
			alicePK: {repModel},
		},
		vouchModels: []*storagemodels.Vouch{
			vouchModel,
		},
		vouchModelsByID: map[string]*storagemodels.Vouch{
			vouch.ID: vouchModel,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)

	userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + userToken}

	t.Run("get reputation requires actor id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get reputation unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/alice", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", "alice")
		require.NoError(t, h.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get reputation success (username normalized)", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/alice", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", "alice")
		require.NoError(t, h.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get reputation invalid token", func(t *testing.T) {
		badHeaders := map[string]string{"Authorization": "Bearer bad-token"}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/alice", badHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", aliceActorID)
		require.NoError(t, h.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get reputation returns 500 when signer misconfigured", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.ReputationPrivateKey = "not a pem"
		badHandler, _, _ := round11NewHandler(t, badCfg, state)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/alice", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", aliceActorID)
		require.NoError(t, badHandler.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get reputation internal error returns 500", func(t *testing.T) {
		errState := &round10QueryState{
			allErrorOnce: errors.New("boom"),
		}
		errHandler, _, _ := round11NewHandler(t, cfg, errState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/alice", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", aliceActorID)
		require.NoError(t, errHandler.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get reputation actor not found returns 404", func(t *testing.T) {
		notFoundState := &round10QueryState{
			allErrorOnce: errors.New("actor not found"),
		}
		notFoundHandler, _, _ := round11NewHandler(t, cfg, notFoundState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/reputation/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", "missing")
		require.NoError(t, notFoundHandler.HandleGetReputationLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("export reputation sets content type", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/export", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleExportReputationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		require.Contains(t, ctx.Response.Headers["Content-Type"], "application/ld+json")
	})

	t.Run("export reputation accepts lowercase authorization header", func(t *testing.T) {
		lowerHeaders := map[string]string{"authorization": "Bearer " + userToken}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/export", lowerHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleExportReputationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("export reputation returns 500 on storage failure", func(t *testing.T) {
		failState := &round10QueryState{allErrorOnce: errors.New("export failed")}
		failHandler, _, _ := round11NewHandler(t, cfg, failState)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/export", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, failHandler.HandleExportReputationLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("export reputation unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/export", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleExportReputationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("export reputation invalid token", func(t *testing.T) {
		badHeaders := map[string]string{"Authorization": "Bearer bad-token"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/export", badHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleExportReputationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("import reputation invalid request body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/reputation/import", headers, nil, []byte("{"))
		require.NoError(t, h.HandleImportReputationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("import reputation missing body returns bad request", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/import", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleImportReputationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("import reputation unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/import", nil, nil, apimodels.ReputationDocumentRequest{Document: "{}"})
		require.NoError(t, err)
		require.NoError(t, h.HandleImportReputationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("import reputation invalid token", func(t *testing.T) {
		badHeaders := map[string]string{"Authorization": "Bearer bad-token"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/import", badHeaders, nil, apimodels.ReputationDocumentRequest{Document: "{}"})
		require.NoError(t, err)
		require.NoError(t, h.HandleImportReputationLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("import reputation invalid document returns result", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/import", headers, nil, apimodels.ReputationDocumentRequest{Document: "{"})
		require.NoError(t, err)
		require.NoError(t, h.HandleImportReputationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("create vouch rejects missing to", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", headers, nil, apimodels.CreateVouchRequest{Confidence: 0.5})
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create vouch rejects invalid confidence", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", headers, nil, apimodels.CreateVouchRequest{To: "https://example.com/users/bob", Confidence: 2})
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create vouch unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", nil, nil, apimodels.CreateVouchRequest{To: "https://example.com/users/bob", Confidence: 0.5})
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("create vouch invalid request body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/vouches", headers, nil, []byte("{"))
		require.NoError(t, h.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create vouch monthly limit reached", func(t *testing.T) {
		t.Skip("Test requires mock infrastructure update to properly simulate GetMonthlyVouchCount")
		now := time.Now().UTC()
		limitState := &round10QueryState{
			reputationsByPK: map[string][]storagemodels.Reputation{
				alicePK: {repModel},
			},
			vouchModels: []*storagemodels.Vouch{
				{GSI1PK: "VOUCHER#" + aliceActorID, CreatedAt: now},
				{GSI1PK: "VOUCHER#" + aliceActorID, CreatedAt: now},
				{GSI1PK: "VOUCHER#" + aliceActorID, CreatedAt: now},
				{GSI1PK: "VOUCHER#" + aliceActorID, CreatedAt: now},
				{GSI1PK: "VOUCHER#" + aliceActorID, CreatedAt: now},
			},
		}

		limitHandler, _, _ := round11NewHandler(t, cfg, limitState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", headers, nil, apimodels.CreateVouchRequest{To: "https://example.com/users/bob", Confidence: 0.5})
		require.NoError(t, err)
		require.NoError(t, limitHandler.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create vouch insufficient reputation", func(t *testing.T) {
		lowRepModel := round12StoredReputationModel(t, aliceActorID, 100)
		lowState := &round10QueryState{
			reputationsByPK: map[string][]storagemodels.Reputation{
				alicePK: {lowRepModel},
			},
		}

		lowHandler, _, _ := round11NewHandler(t, cfg, lowState)
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", headers, nil, apimodels.CreateVouchRequest{To: "https://example.com/users/bob", Confidence: 0.5})
		require.NoError(t, err)
		require.NoError(t, lowHandler.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create vouch success", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", headers, nil, apimodels.CreateVouchRequest{To: "https://example.com/users/bob", Confidence: 0.5})
		require.NoError(t, err)
		require.NoError(t, h.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusCreated, ctx.Response.StatusCode)
	})

	t.Run("create vouch storage failure returns 500", func(t *testing.T) {
		failState := &round10QueryState{
			reputationsByPK: map[string][]storagemodels.Reputation{
				alicePK: {repModel},
			},
			createErrorOnce: errors.New("create failed"),
		}
		failHandler, _, _ := round11NewHandler(t, cfg, failState)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/vouches", headers, nil, apimodels.CreateVouchRequest{To: "https://example.com/users/bob", Confidence: 0.5})
		require.NoError(t, err)
		require.NoError(t, failHandler.HandleCreateVouchLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get vouches requires actor id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/vouches/", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetVouchesLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get vouches unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/vouches/alice", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", "alice")
		require.NoError(t, h.HandleGetVouchesLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("get vouches lists items", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/vouches/alice", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", "alice")
		require.NoError(t, h.HandleGetVouchesLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get vouches scan failure returns 500", func(t *testing.T) {
		failState := &round10QueryState{
			scanErrorOnce: errors.New("scan failed"),
		}
		failHandler, _, _ := round11NewHandler(t, cfg, failState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/vouches/alice", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("actor_id", "alice")
		require.NoError(t, failHandler.HandleGetVouchesLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("revoke vouch forbids non-voucher", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/vouches/"+vouch.ID, headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("vouch_id", vouch.ID)
		require.NoError(t, h.HandleRevokeVouchLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("revoke vouch success", func(t *testing.T) {
		bobToken := round11SignAccessToken(t, cfg.JWTSecret, "bob", []string{auth.ScopeWrite})
		bobHeaders := map[string]string{"Authorization": "Bearer " + bobToken}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/vouches/"+vouch.ID, bobHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("vouch_id", vouch.ID)
		require.NoError(t, h.HandleRevokeVouchLift(ctx))
		require.Equal(t, http.StatusNoContent, ctx.Response.StatusCode)
	})

	t.Run("revoke vouch requires vouch id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/vouches/", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleRevokeVouchLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("revoke vouch unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/vouches/"+vouch.ID, nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("vouch_id", vouch.ID)
		require.NoError(t, h.HandleRevokeVouchLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("revoke vouch missing returns 500", func(t *testing.T) {
		bobToken := round11SignAccessToken(t, cfg.JWTSecret, "bob", []string{auth.ScopeWrite})
		bobHeaders := map[string]string{"Authorization": "Bearer " + bobToken}
		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/vouches/missing", bobHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("vouch_id", "missing")
		require.NoError(t, h.HandleRevokeVouchLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("verify reputation invalid body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/reputation/verify", headers, nil, []byte("{"))
		require.NoError(t, h.HandleVerifyReputationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("verify reputation missing body returns bad request", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/verify", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleVerifyReputationLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("verify reputation invalid document returns result", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/verify", headers, nil, apimodels.ReputationDocumentRequest{Document: "{"})
		require.NoError(t, err)
		require.NoError(t, h.HandleVerifyReputationLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("verify reputation returns 500 when signer misconfigured", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.ReputationPrivateKey = "not a pem"
		badHandler, _, _ := round11NewHandler(t, badCfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/reputation/verify", nil, nil, apimodels.ReputationDocumentRequest{Document: "{"})
		require.NoError(t, err)
		require.NoError(t, badHandler.HandleVerifyReputationLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get reputation keys", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/reputation-keys", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetReputationKeysLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get reputation keys returns 500 when signer misconfigured", func(t *testing.T) {
		badCfg := round11TestConfig()
		badCfg.ReputationPrivateKey = "not a pem"
		badHandler, _, _ := round11NewHandler(t, badCfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodGet, "/.well-known/reputation-keys", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, badHandler.HandleGetReputationKeysLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestErrorChainContainsRound12(t *testing.T) {
	base := errors.New("actor not found")
	require.True(t, errorChainContains(base, "actor not found"))

	wrapped := fmt.Errorf("wrapped: %w", base)
	require.True(t, errorChainContains(wrapped, "actor not found"))

	joined := errors.Join(errors.New("other"), wrapped)
	require.True(t, errorChainContains(joined, "actor not found"))
	require.False(t, errorChainContains(joined, "missing"))
}
