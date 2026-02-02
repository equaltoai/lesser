package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestProcessUserSessionsRound12_Empty(t *testing.T) {
	lastIP, ipHistory := processUserSessions(nil)
	require.Nil(t, lastIP)
	require.Nil(t, ipHistory)
}

func TestHandleAdminGetAccountLiftRound12_Returns500OnRepositoryError(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
		firstErrorPK: map[string]error{
			"USER#target": context.Canceled,
		},
	}

	h, _, _ := round11NewHandler(t, cfg, state)
	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeAdmin})

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts/user-target", map[string]string{
		"Authorization": "Bearer " + adminToken,
	}, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = "user-target"

	requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetAccountLift(ctx))
}
