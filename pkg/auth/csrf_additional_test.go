package auth

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory"
	"go.uber.org/zap"
)

type csrfStoreAlwaysErr struct{ err error }

func (s csrfStoreAlwaysErr) Store(string, CSRFToken) error  { return s.err }
func (s csrfStoreAlwaysErr) Get(string) (*CSRFToken, error) { return nil, s.err }
func (s csrfStoreAlwaysErr) Delete(string) error            { return s.err }
func (s csrfStoreAlwaysErr) CleanExpired() error            { return s.err }

func TestGenerateCSRFTokenHandler_UnauthorizedAndSuccessAndError(t *testing.T) {
	t.Parallel()

	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)

	handler := GenerateCSRFTokenHandler(manager)

	// Unauthorized without claims.
	req := httptest.NewRequest(http.MethodGet, "/csrf", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	// Success with claims.
	req = httptest.NewRequest(http.MethodGet, "/csrf", nil).WithContext(WithClaims(context.Background(), &Claims{Username: "alice"}))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "csrf_token")

	// Error when token store fails.
	managerErr := NewCSRFManager(csrfStoreAlwaysErr{err: errors.New("store failed")})
	handlerErr := GenerateCSRFTokenHandler(managerErr)
	req = httptest.NewRequest(http.MethodGet, "/csrf", nil).WithContext(WithClaims(context.Background(), &Claims{Username: "alice"}))
	rec = httptest.NewRecorder()
	handlerErr.ServeHTTP(rec, req)
	require.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
	require.NotEmpty(t, rec.Body.String())
}

func TestCSRFMiddleware_AdditionalBranches(t *testing.T) {
	t.Parallel()

	store := NewMemoryCSRFStore()
	manager := NewCSRFManager(store)
	middleware := CSRFMiddleware(manager)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	// POST with token but no claims -> 401.
	token, err := manager.GenerateToken("alice")
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("X-CSRF-Token", token)
	rec := httptest.NewRecorder()
	middleware(next).ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.False(t, nextCalled)

	// POST with invalid token -> 403.
	req = httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(WithClaims(context.Background(), &Claims{Username: "alice"}))
	req.Header.Set("X-CSRF-Token", "bad")
	rec = httptest.NewRecorder()
	middleware(next).ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)

	// POST with expired token -> 403 and specific message.
	expired, err := manager.GenerateToken("alice")
	require.NoError(t, err)
	csrf, err := store.Get(expired)
	require.NoError(t, err)
	csrf.ExpiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, store.Store(expired, *csrf))

	req = httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(WithClaims(context.Background(), &Claims{Username: "alice"}))
	req.Header.Set("X-CSRF-Token", expired)
	rec = httptest.NewRecorder()
	middleware(next).ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "expired")

	// Token from form value (no header).
	formToken, err := manager.GenerateToken("alice")
	require.NoError(t, err)
	body := bytes.NewBufferString("csrf_token=" + formToken)
	req = httptest.NewRequest(http.MethodPost, "/test", body).WithContext(WithClaims(context.Background(), &Claims{Username: "alice"}))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	nextCalled = false
	middleware(next).ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.True(t, nextCalled)
}

func TestNewCSRFManagerWithDynamORM_ConstructsManager(t *testing.T) {
	t.Parallel()

	manager := NewCSRFManagerWithDynamORM(&tabletheory.DB{}, "test-table", zap.NewNop())
	require.NotNil(t, manager)
	require.NotNil(t, manager.store)
}
