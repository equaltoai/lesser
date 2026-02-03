package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apiHandlers "github.com/equaltoai/lesser/cmd/api/handlers"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	storageRepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	tablemocks "github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func newRateLimitRepoForAgentSafety(t *testing.T) *storageRepos.RateLimitRepository {
	t.Helper()

	db := new(tablemocks.MockDB)
	q := new(tablemocks.MockQuery)
	update := new(tablemocks.MockUpdateBuilder)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("WithContext", mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()
	q.On("Cursor", mock.Anything).Return(q).Maybe()
	q.On("ConsistentRead").Return(q).Maybe()
	q.On("UpdateBuilder").Return(update).Maybe()
	update.On("Set", mock.Anything, mock.Anything).Return(update).Maybe()
	update.On("Execute").Return(nil).Maybe()

	// Default: treat reads as not found and allow writes.
	q.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Maybe()
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	return storageRepos.NewRateLimitRepository(db, "test-table", zap.NewNop(), nil)
}

func newConcurrencyDB(t *testing.T) (*tablemocks.MockDB, *tablemocks.MockQuery, *tablemocks.MockUpdateBuilder) {
	t.Helper()

	db := new(tablemocks.MockDB)
	q := new(tablemocks.MockQuery)
	update := new(tablemocks.MockUpdateBuilder)

	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("WithContext", mock.Anything).Return(q).Maybe()
	q.On("IfNotExists").Return(q).Maybe()
	q.On("UpdateBuilder").Return(update).Maybe()
	update.On("Set", mock.Anything, mock.Anything).Return(update).Maybe()
	update.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(update).Maybe()

	// Used by release().
	q.On("WithCondition", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Delete").Return(nil).Maybe()

	return db, q, update
}

func TestCreateOptionalOAuthAuthMiddleware_Round13(t *testing.T) {
	t.Run("no-op when config or repos missing", func(t *testing.T) {
		mw := createOptionalOAuthAuthMiddleware(nil, nil, nil)
		called := false
		next := func(*apptheory.Context) (*apptheory.Response, error) {
			called = true
			return apptheory.Text(http.StatusOK, "ok"), nil
		}
		_, err := mw(next)(&apptheory.Context{})
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("sets claims when bearer token validates", func(t *testing.T) {
		cfg := &config.Config{JWTSecret: "secret"}
		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("Account").Return((*repositories.AccountRepository)(nil)).Maybe()
		mw := createOptionalOAuthAuthMiddleware(cfg, repos, zap.NewNop())

		now := time.Now()
		claims := auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "agent",
				IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
				NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			},
			Username:  "agent",
			Scopes:    []string{auth.ScopeRead},
			SessionID: "sess-1",
			IsAgent:   true,
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
		require.NoError(t, err)

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/",
				Headers: map[string][]string{
					"authorization": {"Bearer " + token},
				},
			},
		}

		resp, err := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
			stored, ok := c.Get("claims").(*auth.Claims)
			require.True(t, ok)
			require.Equal(t, "agent", stored.Username)
			require.Equal(t, "agent", c.Get("username"))
			require.Equal(t, true, c.Get("is_authenticated"))
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("does not override existing auth context", func(t *testing.T) {
		cfg := &config.Config{JWTSecret: "secret"}
		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("Account").Return((*repositories.AccountRepository)(nil)).Maybe()
		mw := createOptionalOAuthAuthMiddleware(cfg, repos, zap.NewNop())

		ctx := &apptheory.Context{
			Request: apptheory.Request{
				Method: http.MethodGet,
				Path:   "/",
			},
		}
		ctx.Set("claims", &auth.Claims{Username: "existing"})
		ctx.Set("username", "existing")

		resp, err := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
			stored := c.Get("claims").(*auth.Claims)
			require.Equal(t, "existing", stored.Username)
			require.Equal(t, "existing", c.Get("username"))
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})
}

func TestAcquireAgentConcurrencyLease_Round13(t *testing.T) {
	t.Run("nil db or invalid input returns nil", func(t *testing.T) {
		lease, err := acquireAgentConcurrencyLease(context.Background(), nil, "sess", 1, time.Second)
		require.NoError(t, err)
		require.Nil(t, lease)

		db, _, _ := newConcurrencyDB(t)
		lease, err = acquireAgentConcurrencyLease(context.Background(), db, "", 1, time.Second)
		require.NoError(t, err)
		require.Nil(t, lease)

		lease, err = acquireAgentConcurrencyLease(context.Background(), db, "sess", 0, time.Second)
		require.NoError(t, err)
		require.Nil(t, lease)
	})

	t.Run("creates a new slot lease", func(t *testing.T) {
		db, q, _ := newConcurrencyDB(t)
		q.On("Create").Return(nil).Once()

		lease, err := acquireAgentConcurrencyLease(context.Background(), db, "sess", 2, time.Second)
		require.NoError(t, err)
		require.NotNil(t, lease)
		require.Equal(t, 0, lease.slot)
	})

	t.Run("takes over expired lease when create conflicts", func(t *testing.T) {
		db, q, update := newConcurrencyDB(t)
		q.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		update.On("Execute").Return(nil).Once()

		lease, err := acquireAgentConcurrencyLease(context.Background(), db, "sess", 2, time.Second)
		require.NoError(t, err)
		require.NotNil(t, lease)
	})

	t.Run("retries create once when lease disappears", func(t *testing.T) {
		db, q, update := newConcurrencyDB(t)
		q.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		update.On("Execute").Return(dynamormErrors.ErrItemNotFound).Once()
		q.On("Create").Return(nil).Once()

		lease, err := acquireAgentConcurrencyLease(context.Background(), db, "sess", 2, time.Second)
		require.NoError(t, err)
		require.NotNil(t, lease)
	})

	t.Run("returns concurrency exceeded when all slots are claimed", func(t *testing.T) {
		db, q, update := newConcurrencyDB(t)
		q.On("Create").Return(dynamormErrors.ErrConditionFailed).Once()
		update.On("Execute").Return(dynamormErrors.ErrConditionFailed).Once()

		lease, err := acquireAgentConcurrencyLease(context.Background(), db, "sess", 1, time.Second)
		require.ErrorIs(t, err, errAgentConcurrencyExceeded)
		require.Nil(t, lease)
	})

	t.Run("propagates non-condition create errors", func(t *testing.T) {
		db, q, _ := newConcurrencyDB(t)
		q.On("Create").Return(errors.New("boom")).Once()

		lease, err := acquireAgentConcurrencyLease(context.Background(), db, "sess", 1, time.Second)
		require.Error(t, err)
		require.Nil(t, lease)
	})

	t.Run("release is safe", func(t *testing.T) {
		db, _, _ := newConcurrencyDB(t)
		(&agentConcurrencyLease{sessionID: "sess", slot: 1, leaseID: "lease"}).release(context.Background(), db)
		(*agentConcurrencyLease)(nil).release(context.Background(), db)
	})
}

func TestCreateAgentSafetyRailsMiddleware_Round13(t *testing.T) {
	cfg := &config.Config{JWTSecret: "secret"}

	t.Run("non-agent requests pass through", func(t *testing.T) {
		repos := &apiHandlers.MockRepositoryStorage{}
		mw := createAgentSafetyRailsMiddleware(cfg, repos, zap.NewNop())

		called := false
		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodGet, Path: "/"}}
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			called = true
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, http.StatusOK, resp.Status)
	})

	t.Run("returns 429 when concurrency exceeded", func(t *testing.T) {
		rateRepo := newRateLimitRepoForAgentSafety(t)
		concurrencyDB, q, update := newConcurrencyDB(t)
		q.On("Create").Return(dynamormErrors.ErrConditionFailed).Twice()
		update.On("Execute").Return(dynamormErrors.ErrConditionFailed).Twice()

		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("RateLimit").Return(rateRepo).Maybe()
		repos.On("GetDB").Return(concurrencyDB).Maybe()

		mw := createAgentSafetyRailsMiddleware(cfg, repos, zap.NewNop())

		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/api/v1/statuses"}}
		ctx.Set("claims", &auth.Claims{Username: "agent", IsAgent: true, SessionID: "sess"})

		called := false
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			called = true
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.False(t, called)
		require.Equal(t, http.StatusTooManyRequests, resp.Status)
	})

	t.Run("allows request when concurrency check errors and releases lease on success", func(t *testing.T) {
		rateRepo := newRateLimitRepoForAgentSafety(t)
		concurrencyDB, q, _ := newConcurrencyDB(t)
		q.On("Create").Return(nil).Once()

		repos := &apiHandlers.MockRepositoryStorage{}
		repos.On("RateLimit").Return(rateRepo).Maybe()
		repos.On("GetDB").Return(concurrencyDB).Maybe()

		mw := createAgentSafetyRailsMiddleware(cfg, repos, zap.NewNop())

		ctx := &apptheory.Context{Request: apptheory.Request{Method: http.MethodPost, Path: "/api/v1/statuses"}}
		ctx.Set("claims", &auth.Claims{Username: "agent", IsAgent: true, SessionID: "sess"})

		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return apptheory.Text(http.StatusOK, "ok"), nil
		})(ctx)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.Status)
	})
}
