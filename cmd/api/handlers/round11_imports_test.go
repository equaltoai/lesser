package handlers

import (
	"encoding/base64"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestImportStatusAndList(t *testing.T) {
	state := &round10QueryState{
		importsByID: map[string]storagemodels.Import{
			"import-1": {ID: "import-1", Username: "alice", Type: "followers", Mode: "merge", Status: "pending", CreatedAt: time.Now().Add(-1 * time.Hour)},
		},
		importsByUser: map[string][]storagemodels.Import{
			"alice": {{ID: "import-1", Username: "alice", Type: "followers", Mode: "merge", Status: "pending", CreatedAt: time.Now().Add(-1 * time.Hour)}},
		},
	}
	h, _, _ := round11NewHandlerSliceC(t, state)

	token := round11SignToken(t, h.cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxStatus, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports/import-1", headers, nil, nil)
	require.NoError(t, err)
	ctxStatus.Params["id"] = "import-1"
	requireStatus(t, http.StatusOK)(h.HandleGetImportStatusLift(ctxStatus))

	ctxList, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", headers, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(h.HandleListImportsLift(ctxList))

	ctxCancel, err := round10NewLiftContext(http.MethodDelete, "/api/v1/imports/import-1", headers, nil, nil)
	require.NoError(t, err)
	ctxCancel.Params["id"] = "import-1"
	requireStatus(t, http.StatusOK)(h.HandleCancelImportLift(ctxCancel))
}

func TestImportHelpers(t *testing.T) {
	h, _, _ := round11NewHandlerSliceC(t, nil)

	require.Equal(t, "application/json", h.detectContentType([]byte("{}")))
	require.True(t, h.isValidImportFormat("text/csv"))

	encoded := base64.StdEncoding.EncodeToString([]byte("{}"))
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
	require.NoError(t, err)
	data, resp, err := h.processImportFileData(ctx, encoded)
	require.NoError(t, err)
	require.Nil(t, resp)
	require.NotEmpty(t, data)
}
