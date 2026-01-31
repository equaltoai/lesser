package lift

import (
	stdErrors "errors"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestExports_Round12_ListExports_TokenAuth(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	expires := now.Add(2 * time.Hour)
	state := &round10QueryState{
		exportList: []storagemodels.Export{
			{
				ID:          "exp-1",
				Username:    "alice",
				Type:        "archive",
				Format:      "activitypub",
				Status:      ExportStatusCompleted,
				DownloadURL: "https://example.com/download",
				ExpiresAt:   &expires,
				FileSize:    2048,
				RecordCount: 12,
				CreatedAt:   now.Add(-2 * time.Hour),
			},
			{
				ID:        "exp-2",
				Username:  "alice",
				Type:      "followers",
				Format:    "csv",
				Status:    ExportStatusFailed,
				Error:     "failed",
				CreatedAt: now.Add(-1 * time.Hour),
			},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"authorization": "Bearer " + token}

	t.Run("ok_lowercase_header", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", headers, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleListExportsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		jobs, ok := ctx.Response.Body.([]apimodels.ExportJob)
		require.True(t, ok)
		require.Len(t, jobs, 2)
		var sawFailed bool
		var sawCompleted bool
		for _, job := range jobs {
			if job.Status == ExportStatusFailed && job.Error != nil {
				sawFailed = true
			}
			if job.Status == ExportStatusCompleted && job.DownloadURL != nil {
				sawCompleted = true
			}
		}
		require.True(t, sawFailed)
		require.True(t, sawCompleted)
	})

	t.Run("ok_direct_header_fallback", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", headers, nil, nil)
		require.NoError(t, err)

		ctx.Request.Headers = nil

		require.NoError(t, handler.HandleListExportsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("missing_header_unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleListExportsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})
}

func TestExports_Round12_CreateExport_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}

	t.Run("insufficient_scope", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + writeToken}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", headers, nil, apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("invalid_body", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/exports", readHeaders, nil, []byte(`{invalid}`))
		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("invalid_type", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: "wat", Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("invalid_format", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "xml"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("csv_archive_not_allowed", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: ExportTypeArchive, Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("invalid_date_range", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{
			Type:   ExportTypeFollowing,
			Format: "csv",
			DateRange: &apimodels.ExportDateRange{
				Start: "not-a-date",
				End:   "2025-01-01",
			},
		})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("conflict_existing_export", func(t *testing.T) {
		state := &round10QueryState{
			exportList: []storagemodels.Export{
				{
					ID:        "exp-pending",
					Username:  "alice",
					Type:      ExportTypeFollowers,
					Format:    "csv",
					Status:    "pending",
					CreatedAt: time.Now().Add(-2 * time.Hour),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusConflict, ctx.Response.StatusCode)
	})

	t.Run("rate_limit_after_existing_check_errors", func(t *testing.T) {
		allErrType := reflect.TypeOf(&[]*storagemodels.Export{}).String()
		state := &round10QueryState{
			allErrorByType: map[string]error{
				allErrType: stdErrors.New("query error"),
			},
			exportList: []storagemodels.Export{
				{
					ID:        "exp-recent",
					Username:  "alice",
					Type:      ExportTypeFollowers,
					Format:    "csv",
					Status:    "pending",
					CreatedAt: time.Now().Add(-10 * time.Minute),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusTooManyRequests, ctx.Response.StatusCode)
	})

	t.Run("budget_limit_exceeded", func(t *testing.T) {
		now := time.Now()
		budget := storagemodels.ImportBudget{
			Username:              "alice",
			Period:                "daily",
			IsActive:              true,
			ExportLimitMicroCents: 1,
			NextResetAt:           now.Add(24 * time.Hour),
		}
		budget.UpdateKeys()

		state := &round10QueryState{
			importBudgetsByPKSK: map[string]storagemodels.ImportBudget{
				budget.PK + "#" + budget.SK: budget,
			},
			exportList: []storagemodels.Export{
				{
					ID:        "exp-previous",
					Username:  "alice",
					Type:      ExportTypeFollowers,
					Format:    "csv",
					Status:    ExportStatusCompleted,
					CreatedAt: now.Add(-2 * time.Hour),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: ExportTypeArchive, Format: "activitypub"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusPaymentRequired, ctx.Response.StatusCode)
	})

	t.Run("create_export_fails", func(t *testing.T) {
		state := &round10QueryState{
			createErrorOnce: stdErrors.New("create error"),
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", readHeaders, nil, apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}

func TestExports_Round12_StatusAndDownload_Branches(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()
	expired := now.Add(-1 * time.Hour)

	state := &round10QueryState{
		exportsByID: map[string]storagemodels.Export{
			"exp-failed": {
				ID:        "exp-failed",
				Username:  "alice",
				Type:      ExportTypeFollowers,
				Format:    "csv",
				Status:    ExportStatusFailed,
				Error:     "nope",
				CreatedAt: now.Add(-3 * time.Hour),
			},
			"exp-otheruser": {
				ID:        "exp-otheruser",
				Username:  "bob",
				Type:      ExportTypeArchive,
				Format:    "activitypub",
				Status:    ExportStatusCompleted,
				CreatedAt: now.Add(-3 * time.Hour),
			},
			"exp-processing": {
				ID:        "exp-processing",
				Username:  "alice",
				Type:      ExportTypeArchive,
				Format:    "activitypub",
				Status:    "processing",
				CreatedAt: now.Add(-10 * time.Minute),
			},
			"exp-nourl": {
				ID:        "exp-nourl",
				Username:  "alice",
				Type:      ExportTypeArchive,
				Format:    "activitypub",
				Status:    ExportStatusCompleted,
				CreatedAt: now.Add(-3 * time.Hour),
			},
			"exp-expired": {
				ID:          "exp-expired",
				Username:    "alice",
				Type:        ExportTypeArchive,
				Format:      "activitypub",
				Status:      ExportStatusCompleted,
				DownloadURL: "https://example.com/download",
				ExpiresAt:   &expired,
				CreatedAt:   now.Add(-3 * time.Hour),
			},
			"exp-ready": {
				ID:          "exp-ready",
				Username:    "alice",
				Type:        ExportTypeArchive,
				Format:      "activitypub",
				Status:      ExportStatusCompleted,
				DownloadURL: "https://example.com/download2",
				CreatedAt:   now.Add(-3 * time.Hour),
			},
		},
		notFoundPKs: map[string]bool{
			"EXPORT#missing": true,
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	headers := map[string]string{"authorization": "Bearer " + token}

	t.Run("status_unauthorized", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-failed", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-failed")

		require.NoError(t, handler.HandleGetExportStatusLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("status_missing_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/", headers, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleGetExportStatusLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("status_not_found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "missing")

		require.NoError(t, handler.HandleGetExportStatusLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("status_forbidden", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-otheruser", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-otheruser")

		require.NoError(t, handler.HandleGetExportStatusLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("status_failed_includes_error", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-failed", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-failed")

		require.NoError(t, handler.HandleGetExportStatusLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

		job, ok := ctx.Response.Body.(apimodels.ExportJob)
		require.True(t, ok)
		require.NotNil(t, job.Error)
	})

	t.Run("download_not_ready", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-processing/download", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-processing")

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusConflict, ctx.Response.StatusCode)
	})

	t.Run("download_missing_id", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports//download", headers, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("download_not_found", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/missing/download", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "missing")

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("download_forbidden", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-otheruser/download", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-otheruser")

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
	})

	t.Run("download_gone_missing_url", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-nourl/download", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-nourl")

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusGone, ctx.Response.StatusCode)
	})

	t.Run("download_gone_expired", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-expired/download", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-expired")

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusGone, ctx.Response.StatusCode)
	})

	t.Run("download_success_no_expires_at", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports/exp-ready/download", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "exp-ready")

		require.NoError(t, handler.HandleDownloadExportLift(ctx))
		require.Equal(t, http.StatusFound, ctx.Response.StatusCode)

		resp, ok := ctx.Response.Body.(apimodels.ExportDownloadResponse)
		require.True(t, ok)
		require.Equal(t, "https://example.com/download2", resp.DownloadURL)
		require.Nil(t, resp.ExpiresAt)
		require.Equal(t, "https://example.com/download2", ctx.Get("Location"))
	})
}

func TestExports_Round12_HelperBranches(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})

	t.Run("extract_export_auth_header_direct_fallback", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", map[string]string{"authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)

		ctx.Request.Headers = nil

		require.Equal(t, "Bearer "+token, handler.extractExportAuthHeader(ctx))
	})

	t.Run("validate_export_token_missing", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", nil, nil, nil)
		require.NoError(t, err)

		_, handled, err := handler.validateExportToken(ctx, "")
		require.NoError(t, err)
		require.True(t, handled)
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("validate_export_params_missing_fields", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", nil, nil, nil)
		require.NoError(t, err)

		handled, err := handler.validateExportParams(ctx, &apimodels.ExportRequest{})
		require.NoError(t, err)
		require.True(t, handled)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("parse_export_request_missing_body", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", nil, nil, nil)
		require.NoError(t, err)

		ctx.Request = nil

		_, handled, err := handler.parseExportRequest(ctx)
		require.NoError(t, err)
		require.True(t, handled)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("process_export_date_range_partial_blank", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", nil, nil, nil)
		require.NoError(t, err)

		dateRange, handled, err := handler.processExportDateRange(ctx, &apimodels.ExportDateRange{Start: "2025-01-01", End: ""})
		require.NoError(t, err)
		require.False(t, handled)
		require.Nil(t, dateRange)
	})

	t.Run("create_export_invalid_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", map[string]string{"Authorization": "Bearer not-a-jwt"}, nil, apimodels.ExportRequest{Type: ExportTypeFollowers, Format: "csv"})
		require.NoError(t, err)

		require.NoError(t, handler.HandleCreateExportLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("list_exports_invalid_token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", map[string]string{"Authorization": "Bearer not-a-jwt"}, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.HandleListExportsLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})
}

func TestExports_Round12_ListExports_RepoError(t *testing.T) {
	cfg := round11TestConfig()

	allErrType := reflect.TypeOf(&[]*storagemodels.Export{}).String()
	state := &round10QueryState{
		allErrorByType: map[string]error{
			allErrType: stdErrors.New("query error"),
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/exports", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)

	require.NoError(t, handler.HandleListExportsLift(ctx))
	require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
}

func TestExports_Round12_QueueSQS_DateParseBranches(t *testing.T) {
	setEnv := func(key, value string) {
		old, ok := os.LookupEnv(key)
		_ = os.Setenv(key, value)
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(key, old)
				return
			}
			_ = os.Unsetenv(key)
		})
	}

	setEnv("AWS_EC2_METADATA_DISABLED", "true")
	setEnv("AWS_REGION", "us-east-1")
	setEnv("AWS_ACCESS_KEY_ID", "test")
	setEnv("AWS_SECRET_ACCESS_KEY", "test")

	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/exports", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
	require.NoError(t, err)

	err = handler.queueExportJobSQS(ctx, "exp-1", "alice", &apimodels.ExportRequest{
		Type:   ExportTypeFollowers,
		Format: "csv",
		DateRange: &apimodels.ExportDateRange{
			Start: "2025-01-01",
			End:   "not-a-date",
		},
	})
	require.Error(t, err)

	err = handler.queueExportJobSQS(ctx, "exp-2", "alice", &apimodels.ExportRequest{
		Type:   ExportTypeFollowers,
		Format: "csv",
		DateRange: &apimodels.ExportDateRange{
			Start: "not-a-date",
			End:   "2025-01-01",
		},
	})
	require.Error(t, err)

	err = handler.queueExportJobSQS(ctx, "exp-3", "alice", &apimodels.ExportRequest{
		Type:   ExportTypeFollowers,
		Format: "csv",
	})
	require.NoError(t, err)
}
