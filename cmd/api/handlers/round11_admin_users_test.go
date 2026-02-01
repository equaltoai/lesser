package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestAdminCreateUserLift(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1, CreatedAt: now.Add(-24 * time.Hour)},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)
	token := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	req := apimodels.AdminCreateUserRequest{Username: "newuser", Email: "new@example.com", Password: "password123456", DisplayName: "New User", Role: "user"}
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/users", headers, nil, req)
	require.NoError(t, err)
	resp := requireStatus(t, http.StatusCreated)(handler.HandleAdminCreateUserLift(ctx))
	var created storage.User
	require.NoError(t, json.Unmarshal(resp.Body, &created))
	require.Equal(t, "newuser", created.Username)
	require.Empty(t, created.Email)
	require.Empty(t, created.PasswordHash)
}
