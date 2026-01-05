package lift

import (
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestExportHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	expires := now.Add(2 * time.Hour)
	state := &round10QueryState{
		exportsByID: map[string]storagemodels.Export{
			"exp-1": {
				ID:          "exp-1",
				Username:    "alice",
				Type:        "archive",
				Format:      "activitypub",
				Status:      ExportStatusCompleted,
				DownloadURL: "https://example.com/download",
				ExpiresAt:   &expires,
				FileSize:    2048,
				RecordCount: 12,
				CreatedAt:   now.Add(-1 * time.Hour),
			},
		},
		exportList: []storagemodels.Export{
			{
				ID:        "exp-1",
				Username:  "testuser_with_exports",
				Type:      "archive",
				Format:    "activitypub",
				Status:    ExportStatusCompleted,
				CreatedAt: now.Add(-1 * time.Hour),
				FileSize:  2048,
			},
			{
				ID:        "exp-2",
				Username:  "testuser_with_exports",
				Type:      "followers",
				Format:    "csv",
				Status:    ExportStatusFailed,
				Error:     "failed",
				CreatedAt: now.Add(-2 * time.Hour),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"read"})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	createReq := apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "csv"}
	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, createReq)
	require.NoError(t, err)
	require.NoError(t, handler.HandleCreateExportLift(ctxCreate))
	require.Equal(t, http.StatusAccepted, ctxCreate.Response.StatusCode)

	ctxStatus, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-1", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxStatus.SetParam("id", "exp-1")
	require.NoError(t, handler.HandleGetExportStatusLift(ctxStatus))
	require.Equal(t, http.StatusOK, ctxStatus.Response.StatusCode)

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", map[string]string{"X-Test-Username": "testuser_with_exports"}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, handler.HandleListExportsLift(ctxList))
	require.Equal(t, http.StatusOK, ctxList.Response.StatusCode)

	ctxDownload, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-1/download", readHeaders, nil, nil)
	require.NoError(t, err)
	ctxDownload.SetParam("id", "exp-1")
	require.NoError(t, handler.HandleDownloadExportLift(ctxDownload))
	require.Equal(t, http.StatusFound, ctxDownload.Response.StatusCode)

	job := handler.convertSingleExportToResponse(&state.exportList[0])
	require.Equal(t, ExportStatusCompleted, job.Status)

	cost := handler.estimateExportCost(&apimodels.ExportRequest{Type: ExportTypeArchive, Format: "activitypub", IncludeMedia: true})
	require.Greater(t, cost, int64(0))
}
