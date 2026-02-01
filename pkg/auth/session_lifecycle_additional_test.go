package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type sessionRepoGetUserSessionsErr struct {
	*inMemorySessionRepo
	err error
}

func (r *sessionRepoGetUserSessionsErr) GetUserSessions(_ context.Context, _ string) ([]*Session, error) {
	return nil, r.err
}

func TestSessionLifecycleManager_CleanupAndRevokeAllUserSessionsWithReason(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	repo.sessions["s1"] = &Session{SessionID: "s1", Username: "alice", CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}
	repo.sessions["s2"] = &Session{SessionID: "s2", Username: "alice", CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}

	sessionManager := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), nil)
	slm := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), nil)

	require.NoError(t, slm.CleanupExpiredSessions(context.Background()))

	require.NoError(t, slm.RevokeAllUserSessionsWithReason(context.Background(), "alice", "logout_all"))
	require.Empty(t, repo.sessions)

	// Error retrieving sessions.
	errorRepo := &sessionRepoGetUserSessionsErr{inMemorySessionRepo: repo, err: errors.New("db down")}
	sessionManagerErr := newSessionManager(errorRepo)
	slmErr := NewSessionLifecycleManager(sessionManagerErr, securityManager, nil, zap.NewNop(), nil)
	require.Error(t, slmErr.RevokeAllUserSessionsWithReason(context.Background(), "alice", "logout_all"))
}
