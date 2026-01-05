package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type sessionRepoUpsertUpdate struct {
	*inMemorySessionRepo
}

func (r *sessionRepoUpsertUpdate) UpdateSession(_ context.Context, sessionID, refreshToken, ipAddress string, lastActivity, expiresAt time.Time) error {
	session, ok := r.sessions[sessionID]
	if !ok {
		session = &Session{SessionID: sessionID, Username: "alice"}
		r.sessions[sessionID] = session
	}

	session.RefreshToken = refreshToken
	session.IPAddress = ipAddress
	session.LastActivity = lastActivity
	session.ExpiresAt = expiresAt

	r.updated = append(r.updated, sessionID)
	return nil
}

func TestSessionLifecycleManager_CreateSessionWithLifecycle_ErrorBranches(t *testing.T) {
	t.Parallel()

	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())
	cfg := DefaultSessionLifecycleConfig()

	// enforceConcurrentSessionLimits error.
	repoErr := &sessionRepoGetUserSessionsErr{inMemorySessionRepo: newInMemorySessionRepo(), err: errors.New("db down")}
	slmErr := NewSessionLifecycleManager(newSessionManager(repoErr), securityManager, nil, zap.NewNop(), cfg)
	_, err := slmErr.CreateSessionWithLifecycle(context.Background(), "alice", "device", "curl/8.0", "192.0.2.10", "password")
	require.ErrorIs(t, err, ErrConcurrentSessionLimitExceeded)

	// sessionManager.CreateSession error.
	repoCreateErr := &sessionRepoCreateErr{inMemorySessionRepo: newInMemorySessionRepo(), err: errors.New("create failed")}
	slmCreateErr := NewSessionLifecycleManager(newSessionManager(repoCreateErr), securityManager, nil, zap.NewNop(), cfg)
	_, err = slmCreateErr.CreateSessionWithLifecycle(context.Background(), "alice", "device", "curl/8.0", "192.0.2.10", "password")
	require.ErrorIs(t, err, ErrSessionCreationFailed)
}

func TestSessionLifecycleManager_CreateSessionWithLifecycle_SessionFixationUpdatePath(t *testing.T) {
	t.Parallel()

	repo := &sessionRepoUpsertUpdate{inMemorySessionRepo: newInMemorySessionRepo()}
	sessionManager := newSessionManager(repo)

	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())
	cfg := DefaultSessionLifecycleConfig()
	cfg.SessionFixationPrevention = true

	slm := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), cfg)
	session, err := slm.CreateSessionWithLifecycle(context.Background(), "alice", "device", "curl/8.0", "192.0.2.10", "password")
	require.NoError(t, err)
	require.NotEmpty(t, session.SessionID)
	require.Contains(t, repo.updated, session.SessionID)
}
