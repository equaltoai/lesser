package lift

import (
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestImportsRound12_HandleCreateImportLift_Branches(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")

	cfg := round10TestConfig()

	t.Run("missing_auth_returns_401", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.HandleCreateImportLift(ctx))
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("invalid_type_returns_400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}
		data := base64.StdEncoding.EncodeToString([]byte(`{"a":1}`))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", headers, nil, apimodels.ImportRequest{
			Type: "bogus",
			Mode: "merge",
			Data: data,
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreateImportLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("existing_import_conflict_returns_409", func(t *testing.T) {
		state := &round10QueryState{
			importsByUser: map[string][]storagemodels.Import{
				"alice": {{
					ID:        "import-1",
					Username:  "alice",
					Type:      "followers",
					Status:    "pending",
					CreatedAt: time.Now().Add(-1 * time.Hour),
				}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}
		data := base64.StdEncoding.EncodeToString([]byte(`{"a":1}`))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", headers, nil, apimodels.ImportRequest{
			Type: "followers",
			Mode: "merge",
			Data: data,
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreateImportLift(ctx))
		require.Equal(t, http.StatusConflict, ctx.Response.StatusCode)
	})

	t.Run("budget_limit_exceeded_returns_402", func(t *testing.T) {
		state := &round10QueryState{
			importsByUser:       map[string][]storagemodels.Import{"alice": {}},
			importBudgetsByPKSK: map[string]storagemodels.ImportBudget{},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		req := &apimodels.ImportRequest{
			Type: "followers",
			Mode: "merge",
			Data: base64.StdEncoding.EncodeToString([]byte(`{"a":1}`)),
		}
		estimated := h.estimateImportCost(req, len([]byte(`{"a":1}`)))

		budget := storagemodels.ImportBudget{
			Username:              "alice",
			Period:                "daily",
			IsActive:              true,
			ImportLimitMicroCents: estimated - 1,
			NextResetAt:           time.Now().Add(24 * time.Hour),
		}
		budget.UpdateKeys()
		state.importBudgetsByPKSK[budget.PK+"#"+budget.SK] = budget

		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", headers, nil, *req)
		require.NoError(t, err)

		require.NoError(t, h.HandleCreateImportLift(ctx))
		require.Equal(t, http.StatusPaymentRequired, ctx.Response.StatusCode)
	})

	t.Run("storeImportFile_bucket_missing_returns_500_without_double_write", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{importsByUser: map[string][]storagemodels.Import{"alice": {}}})

		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}
		data := base64.StdEncoding.EncodeToString([]byte(`{"a":1}`))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", headers, nil, apimodels.ImportRequest{
			Type: "followers",
			Mode: "merge",
			Data: data,
		})
		require.NoError(t, err)

		require.NoError(t, h.HandleCreateImportLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})
}

func TestImportsRound12_AuthAndParsingHelpers(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("extractImportAuthHeader_checks_multiple_sources", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", map[string]string{"authorization": "Bearer x"}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "Bearer x", h.extractImportAuthHeader(ctx))

		ctx2, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		ctx2.Request.Request.Headers = map[string]string{"Authorization": "Bearer y"}
		require.Equal(t, "Bearer y", h.extractImportAuthHeader(ctx2))
	})

	t.Run("validateImportToken_errors_and_success", func(t *testing.T) {
		t.Run("missing_token", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)

			username, err := h.validateImportToken(ctx, "")
			require.NoError(t, err)
			require.Empty(t, username)
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("invalid_token", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)

			username, err := h.validateImportToken(ctx, "bad")
			require.NoError(t, err)
			require.Empty(t, username)
			require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
		})

		t.Run("insufficient_scope", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)

			token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-1")
			username, err := h.validateImportToken(ctx, token)
			require.NoError(t, err)
			require.Empty(t, username)
			require.Equal(t, http.StatusForbidden, ctx.Response.StatusCode)
		})

		t.Run("success", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)

			token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
			username, err := h.validateImportToken(ctx, token)
			require.NoError(t, err)
			require.Equal(t, "alice", username)
		})
	})

	t.Run("parseImportRequest_default_mode_and_invalid_body", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, apimodels.ImportRequest{
			Type: "followers",
			Data: base64.StdEncoding.EncodeToString([]byte("x")),
		})
		require.NoError(t, err)

		parsed, err := h.parseImportRequest(ctx)
		require.NoError(t, err)
		require.Equal(t, "merge", parsed.Mode)

		ctxMode, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, apimodels.ImportRequest{
			Type: "followers",
			Mode: "overwrite",
			Data: base64.StdEncoding.EncodeToString([]byte("x")),
		})
		require.NoError(t, err)
		parsedMode, err := h.parseImportRequest(ctxMode)
		require.NoError(t, err)
		require.Equal(t, "overwrite", parsedMode.Mode)

		ctxBad := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/imports", nil, nil, []byte("{"))
		parsed2, err := h.parseImportRequest(ctxBad)
		require.NoError(t, err)
		require.Nil(t, parsed2)
		require.Equal(t, http.StatusBadRequest, ctxBad.Response.StatusCode)

		ctxEmpty, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		parsed3, err := h.parseImportRequest(ctxEmpty)
		require.NoError(t, err)
		require.Nil(t, parsed3)
		require.Equal(t, http.StatusBadRequest, ctxEmpty.Response.StatusCode)
	})
}

func TestImportsRound12_ValidationAndCostHelpers(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")

	h, _, _ := round11NewHandler(t, round10TestConfig(), &round10QueryState{})

	t.Run("validateImportParams", func(t *testing.T) {
		require.Error(t, h.validateImportParams(&apimodels.ImportRequest{}))
		require.Error(t, h.validateImportParams(&apimodels.ImportRequest{Type: "followers"}))
		require.Error(t, h.validateImportParams(&apimodels.ImportRequest{Type: "bogus", Mode: "merge"}))
		require.Error(t, h.validateImportParams(&apimodels.ImportRequest{Type: "followers", Mode: "bad"}))
		require.NoError(t, h.validateImportParams(&apimodels.ImportRequest{Type: "followers", Mode: "merge"}))
	})

	t.Run("processImportFileData_errors", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		_, err = h.processImportFileData(ctx, "")
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		_, err = h.processImportFileData(ctx2, "not-base64")
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, ctx2.Response.StatusCode)

		tooLarge := base64.StdEncoding.EncodeToString(make([]byte, 10*1024*1024+1))
		ctx3, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		_, err = h.processImportFileData(ctx3, tooLarge)
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, ctx3.Response.StatusCode)
	})

	t.Run("detectContentType_and_isValidImportFormat", func(t *testing.T) {
		require.Equal(t, "application/octet-stream", h.detectContentType(nil))
		require.Equal(t, "application/json", h.detectContentType([]byte(`{"a":1}`)))
		require.Equal(t, "text/csv", h.detectContentType([]byte("a,b\nc,d\n")))

		require.True(t, h.isValidImportFormat("application/json; charset=utf-8"))
		require.False(t, h.isValidImportFormat("image/png"))
	})

	t.Run("basicFileValidation", func(t *testing.T) {
		require.Error(t, h.basicFileValidation([]byte("no-commas-and-not-json")))
		require.NoError(t, h.basicFileValidation([]byte("a,b")))
	})

	t.Run("estimateImportCost", func(t *testing.T) {
		baseReq := &apimodels.ImportRequest{Type: ExportTypeFollowers, Mode: "merge"}
		mergeCost := h.estimateImportCost(baseReq, 1)
		overwriteCost := h.estimateImportCost(&apimodels.ImportRequest{Type: ExportTypeFollowers, Mode: "overwrite"}, 1)
		require.Greater(t, overwriteCost, mergeCost)

		listsCost := h.estimateImportCost(&apimodels.ImportRequest{Type: "lists", Mode: "merge"}, 1)
		require.Greater(t, listsCost, int64(0))
	})
}

func TestImportsRound12_RepositoryAndQueueHelpers(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")

	cfg := round10TestConfig()

	t.Run("checkExistingImports_handles_repo_error_and_conflict", func(t *testing.T) {
		state := &round10QueryState{allErrorOnce: errors.New("boom")}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.checkExistingImports(ctx, "alice", "followers"))

		state2 := &round10QueryState{
			importsByUser: map[string][]storagemodels.Import{
				"alice": {{ID: "import-1", Username: "alice", Type: "followers", Status: "processing", CreatedAt: time.Now()}},
			},
		}
		h2, _, _ := round11NewHandler(t, cfg, state2)
		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h2.checkExistingImports(ctx2, "alice", "followers"))
		require.Equal(t, http.StatusConflict, ctx2.Response.StatusCode)
	})

	t.Run("checkImportRateLimit", func(t *testing.T) {
		errorState := &round10QueryState{allErrorOnce: errors.New("boom")}
		errorHandler, _, _ := round11NewHandler(t, cfg, errorState)
		errorCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, errorHandler.checkImportRateLimit(errorCtx, "alice", "followers"))
		require.False(t, errorCtx.Response.IsWritten())

		state := &round10QueryState{
			importsByUser: map[string][]storagemodels.Import{
				"alice": {{ID: "import-1", Username: "alice", Type: "followers", Status: "pending", CreatedAt: time.Now().Add(-1 * time.Hour)}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.checkImportRateLimit(ctx, "alice", "followers"))
		require.Equal(t, http.StatusTooManyRequests, ctx.Response.StatusCode)
	})

	t.Run("checkImportBudgetLimits_import_and_combined", func(t *testing.T) {
		state := &round10QueryState{
			importBudgetsByPKSK: map[string]storagemodels.ImportBudget{},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		importReq := &apimodels.ImportRequest{Type: "followers", Mode: "merge"}
		estimated := h.estimateImportCost(importReq, 1024)

		importBudget := storagemodels.ImportBudget{
			Username:              "alice",
			Period:                "daily",
			IsActive:              true,
			ImportLimitMicroCents: estimated - 1,
			NextResetAt:           time.Now().Add(24 * time.Hour),
		}
		importBudget.UpdateKeys()
		state.importBudgetsByPKSK[importBudget.PK+"#"+importBudget.SK] = importBudget

		require.NoError(t, h.checkImportBudgetLimits(ctx, "alice", importReq, 1024))
		require.Equal(t, http.StatusPaymentRequired, ctx.Response.StatusCode)

		combinedCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		state.importBudgetsByPKSK = map[string]storagemodels.ImportBudget{}

		combinedBudget := storagemodels.ImportBudget{
			Username:                "alice",
			Period:                  "daily",
			IsActive:                true,
			ImportLimitMicroCents:   estimated * 10,
			CombinedLimitMicroCents: estimated - 1,
			CurrentCombinedCost:     0,
			NextResetAt:             time.Now().Add(24 * time.Hour),
		}
		combinedBudget.UpdateKeys()
		state.importBudgetsByPKSK[combinedBudget.PK+"#"+combinedBudget.SK] = combinedBudget

		require.NoError(t, h.checkImportBudgetLimits(combinedCtx, "alice", importReq, 1024))
		require.Equal(t, http.StatusPaymentRequired, combinedCtx.Response.StatusCode)
	})

	t.Run("storeImportFile_and_createImportRecord_and_queueImportJobSQS", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		baseCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		// storeImportFile should fail when bucket is not configured (no double write)
		key, err := h.storeImportFile(baseCtx, "alice", uuid.New().String(), "followers", []byte("x"))
		require.NoError(t, err)
		require.Empty(t, key)
		require.Equal(t, http.StatusInternalServerError, baseCtx.Response.StatusCode)

		t.Run("aws_config_load_error_returns_500", func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config")
			require.NoError(t, os.WriteFile(configPath, []byte("[default]\nregion=us-east-1\n"), 0o600))

			t.Setenv("AWS_REGION", "")
			t.Setenv("AWS_DEFAULT_REGION", "")
			t.Setenv("AWS_PROFILE", "missing-profile")
			t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
			t.Setenv("AWS_CONFIG_FILE", configPath)
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)

			key, err := h.storeImportFile(ctx, "alice", "import-1", "followers", []byte("x"))
			require.NoError(t, err)
			require.Empty(t, key)
			require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
		})

		t.Run("createImportRecord_success_queues_without_error", func(t *testing.T) {
			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)

			req := &apimodels.ImportRequest{Type: "followers", Mode: "merge"}
			require.NoError(t, h.createImportRecord(ctx, "import-2", "alice", req, "s3key"))
			require.False(t, ctx.Response.IsWritten())
		})

		// createImportRecord fails on CreateImport error
		errorState := &round10QueryState{createErrorOnce: errors.New("boom")}
		errorHandler, _, _ := round11NewHandler(t, cfg, errorState)
		createCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)
		require.NoError(t, errorHandler.createImportRecord(createCtx, "import-1", "alice", &apimodels.ImportRequest{Type: "followers", Mode: "merge"}, "s3key"))
		require.Equal(t, http.StatusInternalServerError, createCtx.Response.StatusCode)

		// queueImportJobSQS errors when AWS config can't load
		t.Run("queue_error", func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config")
			require.NoError(t, os.WriteFile(configPath, []byte("[default]\nregion=us-east-1\n"), 0o600))

			t.Setenv("AWS_REGION", "")
			t.Setenv("AWS_DEFAULT_REGION", "")
			t.Setenv("AWS_PROFILE", "missing-profile")
			t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
			t.Setenv("AWS_CONFIG_FILE", configPath)
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
			queueCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)
			require.Error(t, h.queueImportJobSQS(queueCtx, "import-1", "alice", &apimodels.ImportRequest{Type: "followers", Mode: "merge"}, "s3key"))
		})

		t.Run("queue_success_skips_when_url_missing", func(t *testing.T) {
			t.Setenv("AWS_REGION", "us-east-1")
			queueCtx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
			require.NoError(t, err)
			require.NoError(t, h.queueImportJobSQS(queueCtx, "import-1", "alice", &apimodels.ImportRequest{Type: "followers", Mode: "merge"}, "s3key"))
		})
	})
}

func TestImportsRound12_StatusListCancelHandlers(t *testing.T) {
	cfg := round10TestConfig()
	state := &round10QueryState{
		importsByID: map[string]storagemodels.Import{
			"import-1": {
				ID:           "import-1",
				Username:     "alice",
				Type:         "followers",
				Mode:         "merge",
				Status:       statusCompleted,
				Progress:     10,
				Total:        10,
				Errors:       []string{"e1"},
				SuccessCount: 9,
				SkipCount:    0,
				ErrorCount:   1,
				CreatedAt:    time.Now().Add(-1 * time.Hour),
			},
		},
		importsByUser: map[string][]storagemodels.Import{
			"alice": {{
				ID:        "import-1",
				Username:  "alice",
				Type:      "followers",
				Mode:      "merge",
				Status:    statusCompleted,
				CreatedAt: time.Now().Add(-1 * time.Hour),
			}},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)

	token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite}, "sess-1")
	headers := map[string]string{"Authorization": "Bearer " + token}

	t.Run("HandleGetImportStatusLift_requires_id_and_ownership", func(t *testing.T) {
		ctxMissing, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports/", headers, nil, nil)
		require.NoError(t, err)
		require.NoError(t, h.HandleGetImportStatusLift(ctxMissing))
		require.Equal(t, http.StatusBadRequest, ctxMissing.Response.StatusCode)

		ctxForbidden, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports/import-1", headers, nil, nil)
		require.NoError(t, err)
		ctxForbidden.SetParam("id", "import-1")
		// Force ownership mismatch
		state.importsByID["import-1"] = storagemodels.Import{ID: "import-1", Username: "bob", CreatedAt: time.Now()}
		require.NoError(t, h.HandleGetImportStatusLift(ctxForbidden))
		require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)
	})

	t.Run("HandleGetImportStatusLift_success_includes_results", func(t *testing.T) {
		state.importsByID["import-1"] = storagemodels.Import{
			ID:           "import-1",
			Username:     "alice",
			Type:         "followers",
			Mode:         "merge",
			Status:       statusCompleted,
			Progress:     10,
			Total:        10,
			Errors:       []string{"e1"},
			SuccessCount: 9,
			SkipCount:    0,
			ErrorCount:   1,
			CreatedAt:    time.Now().Add(-1 * time.Hour),
		}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports/import-1", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "import-1")

		require.NoError(t, h.HandleGetImportStatusLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("HandleListImportsLift_repo_error", func(t *testing.T) {
		errState := &round10QueryState{allErrorOnce: errors.New("boom")}
		errHandler, _, _ := round11NewHandler(t, cfg, errState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", headers, nil, nil)
		require.NoError(t, err)

		require.NoError(t, errHandler.HandleListImportsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("HandleListImportsLift_success", func(t *testing.T) {
		listState := &round10QueryState{
			importsByUser: map[string][]storagemodels.Import{
				"alice": {{
					ID:           "import-1",
					Username:     "alice",
					Type:         "followers",
					Mode:         "merge",
					Status:       statusCompleted,
					Progress:     3,
					Total:        3,
					Errors:       []string{"e1"},
					SuccessCount: 2,
					SkipCount:    0,
					ErrorCount:   1,
					CreatedAt:    time.Now().Add(-1 * time.Hour),
				}},
			},
		}
		listHandler, _, _ := round11NewHandler(t, cfg, listState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", headers, nil, nil)
		require.NoError(t, err)

		require.NoError(t, listHandler.HandleListImportsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
		jobs := ctx.Response.Body.([]apimodels.ImportJob)
		require.Len(t, jobs, 1)
		require.NotNil(t, jobs[0].Results)
		require.NotNil(t, jobs[0].Total)
		require.NotEmpty(t, jobs[0].Errors)
	})

	t.Run("HandleGetImportStatusLift_not_found", func(t *testing.T) {
		notFoundState := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"IMPORT#missing#IMPORT#missing": true,
			},
		}
		notFoundHandler, _, _ := round11NewHandler(t, cfg, notFoundState)

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports/missing", headers, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "missing")

		require.NoError(t, notFoundHandler.HandleGetImportStatusLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("HandleCancelImportLift_conflicts_and_update_error", func(t *testing.T) {
		type testCase struct {
			status     string
			wantStatus int
		}

		for _, tc := range []testCase{
			{status: statusCompleted, wantStatus: http.StatusConflict},
			{status: ImportStatusFailed, wantStatus: http.StatusConflict},
			{status: ImportStatusCancelled, wantStatus: http.StatusConflict},
		} {
			t.Run(tc.status, func(t *testing.T) {
				localState := &round10QueryState{
					importsByID: map[string]storagemodels.Import{
						"import-1": {ID: "import-1", Username: "alice", Status: tc.status, CreatedAt: time.Now().Add(-1 * time.Hour)},
					},
				}
				localHandler, _, _ := round11NewHandler(t, cfg, localState)

				ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/imports/import-1", headers, nil, nil)
				require.NoError(t, err)
				ctx.SetParam("id", "import-1")

				require.NoError(t, localHandler.HandleCancelImportLift(ctx))
				require.Equal(t, tc.wantStatus, ctx.Response.StatusCode)
			})
		}

		updateState := &round10QueryState{
			importsByID: map[string]storagemodels.Import{
				"import-1": {ID: "import-1", Username: "alice", Status: "pending", CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
			updateErrorOnce: errors.New("boom"),
		}
		updateHandler, _, _ := round11NewHandler(t, cfg, updateState)

		ctxUpdate, err := round10NewLiftContext(http.MethodDelete, "/api/v1/imports/import-1", headers, nil, nil)
		require.NoError(t, err)
		ctxUpdate.SetParam("id", "import-1")

		require.NoError(t, updateHandler.HandleCancelImportLift(ctxUpdate))
		require.Equal(t, http.StatusInternalServerError, ctxUpdate.Response.StatusCode)
	})

	t.Run("HandleCancelImportLift_success_forbidden_not_found", func(t *testing.T) {
		successState := &round10QueryState{
			importsByID: map[string]storagemodels.Import{
				"import-1": {ID: "import-1", Username: "alice", Status: "pending", Total: 5, CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
		}
		successHandler, _, _ := round11NewHandler(t, cfg, successState)

		ctxSuccess, err := round10NewLiftContext(http.MethodDelete, "/api/v1/imports/import-1", headers, nil, nil)
		require.NoError(t, err)
		ctxSuccess.SetParam("id", "import-1")
		require.NoError(t, successHandler.HandleCancelImportLift(ctxSuccess))
		require.Equal(t, http.StatusOK, ctxSuccess.Response.StatusCode)
		job := ctxSuccess.Response.Body.(apimodels.ImportJob)
		require.Equal(t, "cancelled", job.Status)
		require.NotNil(t, job.Total)

		forbiddenState := &round10QueryState{
			importsByID: map[string]storagemodels.Import{
				"import-1": {ID: "import-1", Username: "bob", Status: "pending", CreatedAt: time.Now().Add(-1 * time.Hour)},
			},
		}
		forbiddenHandler, _, _ := round11NewHandler(t, cfg, forbiddenState)
		ctxForbidden, err := round10NewLiftContext(http.MethodDelete, "/api/v1/imports/import-1", headers, nil, nil)
		require.NoError(t, err)
		ctxForbidden.SetParam("id", "import-1")
		require.NoError(t, forbiddenHandler.HandleCancelImportLift(ctxForbidden))
		require.Equal(t, http.StatusForbidden, ctxForbidden.Response.StatusCode)

		notFoundState := &round10QueryState{
			notFoundPKSK: map[string]bool{
				"IMPORT#missing#IMPORT#missing": true,
			},
		}
		notFoundHandler, _, _ := round11NewHandler(t, cfg, notFoundState)
		ctxNotFound, err := round10NewLiftContext(http.MethodDelete, "/api/v1/imports/missing", headers, nil, nil)
		require.NoError(t, err)
		ctxNotFound.SetParam("id", "missing")
		require.NoError(t, notFoundHandler.HandleCancelImportLift(ctxNotFound))
		require.Equal(t, http.StatusNotFound, ctxNotFound.Response.StatusCode)
	})
}

func TestImportsRound12_validateImportFile(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")

	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("invalid_file_returns_400", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, h.validateImportFile(ctx, []byte{}, "followers"))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("warnings_do_not_fail", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		data := []byte(`{"script":"<script>alert(1)</script>"}`)
		require.NoError(t, h.validateImportFile(ctx, data, "followers"))
	})

	t.Run("file_validator_creation_failure_falls_back_to_basic", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config")
		require.NoError(t, os.WriteFile(configPath, []byte("[default]\nregion=us-east-1\n"), 0o600))

		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		t.Setenv("AWS_PROFILE", "missing-profile")
		t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
		t.Setenv("AWS_CONFIG_FILE", configPath)
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		require.Error(t, h.validateImportFile(ctx, []byte("no-commas-and-not-json"), "followers"))
	})
}

func TestImportsRound12_authenticateImportStatusRequest(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("missing_token_returns_401", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", nil, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateImportStatusRequest(ctx)
		require.NoError(t, err)
		require.Empty(t, username)
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("invalid_token_returns_401", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", map[string]string{"Authorization": "Bearer bad"}, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateImportStatusRequest(ctx)
		require.NoError(t, err)
		require.Empty(t, username)
		require.Equal(t, http.StatusUnauthorized, ctx.Response.StatusCode)
	})

	t.Run("success_returns_username", func(t *testing.T) {
		token := round11SignToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead}, "sess-1")
		headers := map[string]string{"Authorization": "Bearer " + token}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/imports", headers, nil, nil)
		require.NoError(t, err)

		username, err := h.authenticateImportStatusRequest(ctx)
		require.NoError(t, err)
		require.Equal(t, "alice", username)
	})
}
