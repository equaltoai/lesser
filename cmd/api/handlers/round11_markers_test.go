package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestMarkersHandlers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	markers := map[string]*storage.Marker{
		"home":          {Timeline: "home", LastReadID: "1", UpdatedAt: time.Now(), Version: 1},
		"notifications": {Timeline: "notifications", LastReadID: "2", UpdatedAt: time.Now(), Version: 2},
	}
	accountsSvc := &AccountsServiceStub{
		GetMarkersFunc: func(ctx context.Context, query *accounts.GetMarkersQuery) (*accounts.GetMarkersResult, error) {
			return &accounts.GetMarkersResult{Markers: markers}, nil
		},
		SaveMarkerFunc: func(ctx context.Context, cmd *accounts.SaveMarkerCommand) (*accounts.SaveMarkerResult, error) {
			markers[cmd.Timeline] = &storage.Marker{Timeline: cmd.Timeline, LastReadID: cmd.LastReadID, UpdatedAt: time.Now(), Version: cmd.Version}
			return &accounts.SaveMarkerResult{}, nil
		},
	}

	h.registry = &RegistryStub{AccountsSvc: accountsSvc}

	readToken := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-1")
	writeToken := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/markers", map[string]string{"Authorization": "Bearer " + readToken}, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleGetMarkersLift(ctxGet))

	body := map[string]map[string]string{
		"home":          {"last_read_id": "10"},
		"notifications": {"last_read_id": "20"},
	}
	ctxSave := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/markers", map[string]string{"Authorization": "Bearer " + writeToken}, nil, round11JSONBody(t, body))
	requireStatus(t, http.StatusOK)(h.HandleSaveMarkersLift(ctxSave))
}
