package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type csrfRepoStub struct {
	storeCalled bool
	storeToken  string
	storeUserID string

	getToken     string
	getUserID    string
	getExpiresAt time.Time
	getValid     bool
	getErr       error

	validateErr error

	count int
}

func (s *csrfRepoStub) Store(_ context.Context, token string, userID string, _ time.Time) error {
	s.storeCalled = true
	s.storeToken = token
	s.storeUserID = userID
	return nil
}

func (s *csrfRepoStub) Get(_ context.Context, _ string) (string, string, time.Time, bool, error) {
	return s.getToken, s.getUserID, s.getExpiresAt, s.getValid, s.getErr
}

func (s *csrfRepoStub) Delete(_ context.Context, _ string) error { return nil }

func (s *csrfRepoStub) CleanExpired(_ context.Context) error { return nil }

func (s *csrfRepoStub) ValidateAndConsume(_ context.Context, _ string, _ string) error {
	return s.validateErr
}

func (s *csrfRepoStub) GetUserActiveTokenCount(_ context.Context, _ string) (int, error) {
	return s.count, nil
}

func (s *csrfRepoStub) CleanupUserTokens(_ context.Context, _ string) error { return nil }

func TestDynamORMCSRFStore_WrapperBehavior(t *testing.T) {
	stub := &csrfRepoStub{
		getToken:     "t",
		getUserID:    "u",
		getExpiresAt: time.Now().Add(time.Hour),
		getValid:     true,
	}
	store := &DynamORMCSRFStore{repo: stub}

	require.NoError(t, store.Store("t", CSRFToken{UserID: "u", ExpiresAt: time.Now().Add(time.Hour)}))
	assert.True(t, stub.storeCalled)
	assert.Equal(t, "t", stub.storeToken)
	assert.Equal(t, "u", stub.storeUserID)

	out, err := store.Get("t")
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "t", out.Token)
	assert.Equal(t, "u", out.UserID)

	stub.getValid = false
	_, err = store.Get("t")
	require.ErrorIs(t, err, ErrInvalidCSRF)

	boom := errors.New("boom")
	stub.getErr = boom
	_, err = store.Get("t")
	require.ErrorIs(t, err, boom)

	stub.validateErr = errors.New("invalid CSRF token")
	require.ErrorIs(t, store.ValidateAndConsume("t", "u"), ErrInvalidCSRF)

	stub.validateErr = errors.New("expired CSRF token")
	require.ErrorIs(t, store.ValidateAndConsume("t", "u"), ErrExpiredCSRF)

	stub.validateErr = errors.New("other")
	require.Error(t, store.ValidateAndConsume("t", "u"))

	stub.count = 3
	count, err := store.GetUserActiveTokenCount("u")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestDynamORMCSRFStore_AdditionalWrapperMethods(t *testing.T) {
	stub := &csrfRepoStub{}
	store := &DynamORMCSRFStore{repo: stub}

	require.NoError(t, store.Delete("t"))
	require.NoError(t, store.CleanExpired())
	require.NoError(t, store.CleanupUserTokens("u"))

	stub.validateErr = nil
	require.NoError(t, store.ValidateAndConsume("t", "u"))
}
