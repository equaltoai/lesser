package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	tablemocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func newRateLimitRepoWithFirstHook(t *testing.T, firstErr error, firstHook func(any), createCalls *int) *storageRepos.RateLimitRepository {
	t.Helper()

	db := new(tablemocks.MockDB)
	q := new(tablemocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()

	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(firstErr).Run(func(args mock.Arguments) {
		if firstErr != nil || firstHook == nil {
			return
		}
		firstHook(args.Get(0))
	}).Maybe()

	q.On("Create").Return(nil).Run(func(mock.Arguments) {
		if createCalls == nil {
			return
		}
		*createCalls = *createCalls + 1
	}).Maybe()

	q.On("Delete").Return(nil).Maybe()

	return storageRepos.NewRateLimitRepository(db, "test-table", zap.NewNop(), nil)
}

func TestCLIAutomationSessionID_Round21(t *testing.T) {
	require.Equal(t, "", cliAutomationSessionID(nil))
	require.Equal(t, "sid", cliAutomationSessionID(&auth.Claims{SessionID: " sid "}))
	require.Equal(t, "client", cliAutomationSessionID(&auth.Claims{ClientID: "client"}))
	require.Equal(t, "sub", cliAutomationSessionID(&auth.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "sub"}}))
	require.Equal(t, "user", cliAutomationSessionID(&auth.Claims{Username: "user"}))
}

func TestResolveCLIAutomationLimits_Round21(t *testing.T) {
	defaults := resolveCLIAutomationLimits(nil)
	require.Equal(t, cliAutomationDefaultSustainedLimit, defaults.sustainedLimit)
	require.Equal(t, cliAutomationDefaultSustainedWindow, defaults.sustainedWindow)
	require.Equal(t, cliAutomationDefaultBurstLimit, defaults.burstLimit)
	require.Equal(t, cliAutomationDefaultBurstWindow, defaults.burstWindow)
	require.Equal(t, cliAutomationDefaultConcurrencyLimit, defaults.concurrencyLimit)
	require.Equal(t, cliAutomationDefaultErrorRateThresh, defaults.errorRateThreshold)
	require.Equal(t, cliAutomationDefaultErrorRateMin, defaults.errorRateMin)
	require.Equal(t, cliAutomationDefaultErrorRateWindow, defaults.errorRateWindow)
	require.Equal(t, cliAutomationDefaultLockoutDuration, defaults.lockoutDuration)

	cfg := &config.Config{
		CLIAutomationSustainedLimit:     120,
		CLIAutomationSustainedWindow:    2 * time.Minute,
		CLIAutomationBurstLimit:         10,
		CLIAutomationBurstWindow:        5 * time.Second,
		CLIAutomationConcurrencyLimit:   7,
		CLIAutomationErrorRateThreshold: 0.25,
		CLIAutomationErrorRateMin:       50,
		CLIAutomationErrorRateWindow:    4 * time.Minute,
		CLIAutomationLockoutDuration:    10 * time.Minute,
	}
	limits := resolveCLIAutomationLimits(cfg)
	require.Equal(t, cfg.CLIAutomationSustainedLimit, limits.sustainedLimit)
	require.Equal(t, cfg.CLIAutomationSustainedWindow, limits.sustainedWindow)
	require.Equal(t, cfg.CLIAutomationBurstLimit, limits.burstLimit)
	require.Equal(t, cfg.CLIAutomationBurstWindow, limits.burstWindow)
	require.Equal(t, cfg.CLIAutomationConcurrencyLimit, limits.concurrencyLimit)
	require.Equal(t, cfg.CLIAutomationErrorRateThreshold, limits.errorRateThreshold)
	require.Equal(t, cfg.CLIAutomationErrorRateMin, limits.errorRateMin)
	require.Equal(t, cfg.CLIAutomationErrorRateWindow, limits.errorRateWindow)
	require.Equal(t, cfg.CLIAutomationLockoutDuration, limits.lockoutDuration)
}

func TestMaybeApplyCLIAutomationErrorRateLockout_Round21(t *testing.T) {
	t.Run("returns early when inputs missing", func(t *testing.T) {
		maybeApplyCLIAutomationErrorRateLockout(nil, nil, "", nil, nil, 0, 0, 0, 0)
	})

	t.Run("no error only increments total", func(t *testing.T) {
		createCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if limit, ok := dest.(*storageModels.APIRateLimit); ok {
				limit.Count = 1
				limit.Window = time.Now().UTC()
			}
		}, &createCalls)

		maybeApplyCLIAutomationErrorRateLockout(&apptheory.Context{}, rateRepo, "sid", apptheory.Text(http.StatusOK, "ok"), nil, 0, 0, 0, 0)
		require.Equal(t, 1, createCalls)
	})

	t.Run("error below minRequests returns without lockout", func(t *testing.T) {
		createCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if limit, ok := dest.(*storageModels.APIRateLimit); ok {
				limit.Count = 1
				limit.Window = time.Now().UTC()
			}
		}, &createCalls)

		maybeApplyCLIAutomationErrorRateLockout(&apptheory.Context{}, rateRepo, "sid", apptheory.Text(http.StatusInternalServerError, "boom"), nil, 0, 0, 0, 0)
		require.Equal(t, 2, createCalls)
	})

	t.Run("error rate equal threshold does not lock", func(t *testing.T) {
		createCalls := 0
		firstCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			limit, ok := dest.(*storageModels.APIRateLimit)
			if !ok {
				return
			}
			firstCalls++
			switch firstCalls {
			case 3:
				limit.Count = 10
			case 4:
				limit.Count = 1
			default:
				limit.Count = 0
			}
			limit.Window = time.Now().UTC()
		}, &createCalls)

		maybeApplyCLIAutomationErrorRateLockout(&apptheory.Context{}, rateRepo, "sid", apptheory.Text(http.StatusInternalServerError, "boom"), nil, 0, 0, 0, 0)
		require.Equal(t, 2, createCalls)
	})

	t.Run("error rate above threshold imposes lockout", func(t *testing.T) {
		createCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if limit, ok := dest.(*storageModels.APIRateLimit); ok {
				limit.Count = 10
				limit.Window = time.Now().UTC()
			}
		}, &createCalls)

		maybeApplyCLIAutomationErrorRateLockout(&apptheory.Context{}, rateRepo, "sid", apptheory.Text(http.StatusInternalServerError, "boom"), nil, 0, 0, 0, 0)
		require.Equal(t, 3, createCalls)
	})
}

func TestMaybeApplyAgentErrorRateLockout_Round21(t *testing.T) {
	t.Run("returns early when inputs missing", func(t *testing.T) {
		maybeApplyAgentErrorRateLockout(nil, nil, nil, nil, nil)
	})

	t.Run("imposes lockout when error rate high", func(t *testing.T) {
		createCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if limit, ok := dest.(*storageModels.APIRateLimit); ok {
				limit.Count = agentErrorRateMinRequests
				limit.Window = time.Now().UTC()
			}
		}, &createCalls)

		maybeApplyAgentErrorRateLockout(&apptheory.Context{}, rateRepo, &auth.Claims{Username: "agent"}, apptheory.Text(http.StatusInternalServerError, "boom"), nil)
		require.Equal(t, 3, createCalls)
	})

	t.Run("read only failures do not count toward lockout", func(t *testing.T) {
		createCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if limit, ok := dest.(*storageModels.APIRateLimit); ok {
				limit.Count = agentErrorRateMinRequests
				limit.Window = time.Now().UTC()
			}
		}, &createCalls)

		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/api/v1/timelines/public"}}
		maybeApplyAgentErrorRateLockout(ctx, rateRepo, &auth.Claims{Username: "agent"}, apptheory.Text(http.StatusInternalServerError, "boom"), nil)
		require.Equal(t, 1, createCalls)
	})

	t.Run("client errors do not count toward lockout", func(t *testing.T) {
		createCalls := 0
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if limit, ok := dest.(*storageModels.APIRateLimit); ok {
				limit.Count = agentErrorRateMinRequests
				limit.Window = time.Now().UTC()
			}
		}, &createCalls)

		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/api/v1/accounts/bob/follow"}}
		maybeApplyAgentErrorRateLockout(ctx, rateRepo, &auth.Claims{Username: "agent"}, apptheory.Text(http.StatusBadRequest, "bad"), nil)
		require.Equal(t, 1, createCalls)
	})
}

func TestRejectLockedCLIAutomation_Round21(t *testing.T) {
	t.Run("unlocked returns nil response", func(t *testing.T) {
		rateRepo := newRateLimitRepoWithFirstHook(t, dynamormErrors.ErrItemNotFound, nil, nil)
		resp, err := rejectLockedCLIAutomation(&apptheory.Context{}, rateRepo, "sid")
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("locked returns 429 with retry-after headers", func(t *testing.T) {
		unlockAt := time.Now().UTC().Add(90 * time.Second)
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if lockout, ok := dest.(*storageModels.RateLimitLockout); ok {
				lockout.UnlockTime = unlockAt
			}
		}, nil)

		resp, err := rejectLockedCLIAutomation(&apptheory.Context{}, rateRepo, "sid")
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusTooManyRequests, resp.Status)
		require.Equal(t, []string{"0"}, resp.Headers["x-ratelimit-remaining"])
		require.NotEmpty(t, resp.Headers["retry-after"])
	})
}

func TestHandleCLIAutomationSafetyRails_Round21(t *testing.T) {
	t.Run("rate limit storage errors are fail-open", func(t *testing.T) {
		rateRepo, update := newRateLimitRepoForCLISafety(t)
		update.On("ExecuteWithResult", mock.Anything).Return(errors.New("rate limit storage down")).Twice()

		concurrencyDB, q, _ := newConcurrencyDB(t)
		q.On("Create").Return(nil).Once()

		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("GetDB").Return(concurrencyDB).Maybe()

		called := false
		resp, err := handleCLIAutomationSafetyRails(&apptheory.Context{}, nil, repos, rateRepo, zap.NewNop(), func(*apptheory.Context) (*apptheory.Response, error) {
			called = true
			return apptheory.Text(http.StatusOK, "ok"), nil
		}, &auth.Claims{ClientClass: auth.ClientClassCLI, SessionID: "sid"})
		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("missing session identifiers falls back to unknown", func(t *testing.T) {
		rateRepo, update := newRateLimitRepoForCLISafety(t)
		update.On("ExecuteWithResult", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			limit := args.Get(0).(*storageModels.APIRateLimit)
			limit.Count = 1
		}).Twice()

		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("GetDB").Return(nil).Maybe()

		called := false
		resp, err := handleCLIAutomationSafetyRails(&apptheory.Context{}, nil, repos, rateRepo, zap.NewNop(), func(*apptheory.Context) (*apptheory.Response, error) {
			called = true
			return apptheory.Text(http.StatusOK, "ok"), nil
		}, &auth.Claims{ClientClass: auth.ClientClassCLI})
		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("locked sessions are rejected before throttle checks", func(t *testing.T) {
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if lockout, ok := dest.(*storageModels.RateLimitLockout); ok {
				lockout.UnlockTime = time.Now().UTC().Add(time.Hour)
			}
		}, nil)

		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("GetDB").Return(nil).Maybe()

		called := false
		resp, err := handleCLIAutomationSafetyRails(&apptheory.Context{}, nil, repos, rateRepo, zap.NewNop(), func(*apptheory.Context) (*apptheory.Response, error) {
			called = true
			return apptheory.Text(http.StatusOK, "ok"), nil
		}, &auth.Claims{ClientClass: auth.ClientClassCLI, SessionID: "sid"})
		require.NoError(t, err)
		require.False(t, called)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusTooManyRequests, resp.Status)
	})
}

func TestAgentSafetyHelpers_Round21(t *testing.T) {
	t.Run("agentClaimsFromContext", func(t *testing.T) {
		require.Nil(t, agentClaimsFromContext(nil))

		ctx := &apptheory.Context{}
		require.Nil(t, agentClaimsFromContext(ctx))

		ctx.Set("claims", "not-claims")
		require.Nil(t, agentClaimsFromContext(ctx))

		want := &auth.Claims{Username: "alice"}
		ctx.Set("claims", want)
		require.Equal(t, want, agentClaimsFromContext(ctx))
	})

	t.Run("isCLIAutomationClaims", func(t *testing.T) {
		require.False(t, isCLIAutomationClaims(nil))
		require.True(t, isCLIAutomationClaims(&auth.Claims{ClientClass: auth.ClientClassCLI}))
		require.True(t, isCLIAutomationClaims(&auth.Claims{ClientClass: "CLI"}))
		require.False(t, isCLIAutomationClaims(&auth.Claims{ClientClass: "web"}))
	})

	t.Run("identifier helpers", func(t *testing.T) {
		require.Equal(t, "agent:unknown", agentLockoutIdentifier(""))
		require.Equal(t, "agent:alice", agentLockoutIdentifier("Alice"))
		require.Equal(t, "agent:alice", agentRateLimitUserID("Alice"))
		require.Equal(t, "cli:unknown", cliAutomationLockoutIdentifier(""))
		require.Equal(t, "cli:sid", cliAutomationLockoutIdentifier("SID"))
	})

	t.Run("ceilSeconds and maxInt", func(t *testing.T) {
		require.Equal(t, 1, ceilSeconds(0))
		require.Equal(t, 1, ceilSeconds(500*time.Millisecond))
		require.Equal(t, 1, ceilSeconds(time.Second))
		require.Equal(t, 2, ceilSeconds(time.Second+time.Nanosecond))

		require.Equal(t, 2, maxInt(1, 2))
		require.Equal(t, 3, maxInt(3, 2))
	})

	t.Run("attachRateLimitHeaders fills expected headers", func(t *testing.T) {
		attachRateLimitHeaders(nil, rateLimitHeaders{})

		resp := &apptheory.Response{Status: http.StatusOK}
		attachRateLimitHeaders(resp, rateLimitHeaders{
			limit:             10,
			remaining:         -1,
			resetTimeUnix:     123,
			retryAfterSeconds: 5,
		})
		require.Equal(t, []string{"10"}, resp.Headers["x-ratelimit-limit"])
		require.Equal(t, []string{"0"}, resp.Headers["x-ratelimit-remaining"])
		require.Equal(t, []string{"123"}, resp.Headers["x-ratelimit-reset"])
		require.Equal(t, []string{"5"}, resp.Headers["retry-after"])
	})

	t.Run("cliTooManyRequestsResponse normalizes retry-after", func(t *testing.T) {
		resp, err := cliTooManyRequestsResponse("busy", 0)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusTooManyRequests, resp.Status)
		require.Equal(t, []string{"1"}, resp.Headers["retry-after"])
	})
}

func TestAgentLockoutPaths_Round21(t *testing.T) {
	t.Run("rejectLockedAgent returns nil when unlocked", func(t *testing.T) {
		rateRepo := newRateLimitRepoWithFirstHook(t, dynamormErrors.ErrItemNotFound, nil, nil)
		resp, err := rejectLockedAgent(&apptheory.Context{}, rateRepo, &auth.Claims{Username: "agent"})
		require.NoError(t, err)
		require.Nil(t, resp)
	})

	t.Run("rejectLockedAgent returns 429 when locked", func(t *testing.T) {
		rateRepo := newRateLimitRepoWithFirstHook(t, nil, func(dest any) {
			if lockout, ok := dest.(*storageModels.RateLimitLockout); ok {
				lockout.UnlockTime = time.Now().UTC().Add(time.Hour)
			}
		}, nil)

		resp, err := rejectLockedAgent(&apptheory.Context{}, rateRepo, &auth.Claims{Username: "agent"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusTooManyRequests, resp.Status)
		require.Contains(t, strings.ToLower(string(resp.Body)), "agent_locked")
	})

	t.Run("enforceAgentConcurrency handles session id sources", func(t *testing.T) {
		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("GetDB").Return(nil).Maybe()

		lease, err := enforceAgentConcurrency(&apptheory.Context{}, repos, &auth.Claims{})
		require.NoError(t, err)
		require.Nil(t, lease)

		lease, err = enforceAgentConcurrency(&apptheory.Context{}, repos, &auth.Claims{SessionID: "sess"})
		require.NoError(t, err)
		require.Nil(t, lease)

		lease, err = enforceAgentConcurrency(&apptheory.Context{}, repos, &auth.Claims{AgentSessionID: "agent-sess"})
		require.NoError(t, err)
		require.Nil(t, lease)
	})
}

func TestSafeRateLimitRepo_Round21(t *testing.T) {
	repos := &apiHandlers.MockRepositoryStorage{}
	repos.On("RateLimit").Return((*storageRepos.RateLimitRepository)(nil)).Run(func(mock.Arguments) {
		panic("boom")
	}).Maybe()

	require.NotPanics(t, func() {
		require.Nil(t, safeRateLimitRepo(repos))
	})
}
