package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type sessionRepoUpdateErr struct {
	*inMemorySessionRepo
	err error
}

func (r *sessionRepoUpdateErr) UpdateSession(_ context.Context, _, _, _ string, _ time.Time, _ time.Time) error {
	return r.err
}

type sessionRepoGetSessionErr struct {
	*inMemorySessionRepo
	err error
}

func (r *sessionRepoGetSessionErr) GetSession(_ context.Context, _ string) (*Session, error) { return nil, r.err }

func TestSessionLifecycleManager_CreateSessionWithLifecycle_ConcurrencyAndFixationBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	repo.sessions["old"] = &Session{
		SessionID:    "old",
		Username:     "alice",
		RefreshToken: "rt-old",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	repo.sessions["newer"] = &Session{
		SessionID:    "newer",
		Username:     "alice",
		RefreshToken: "rt-new",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	sessionManager := newSessionManager(repo)

	securityCfg := DefaultAdvancedSessionSecurityConfig()
	securityCfg.SessionFixationPrevention = false // exercise "no ID change" branch
	securityManager := NewSessionSecurityManager(zap.NewNop(), securityCfg)

	cfg := DefaultSessionLifecycleConfig()
	cfg.ConcurrentSessionLimit = 1
	cfg.SessionFixationPrevention = true

	slm := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), cfg)
	_, err := slm.CreateSessionWithLifecycle(context.Background(), "alice", "device", "curl/8.0", "192.0.2.10", "password")
	require.NoError(t, err)
	require.Contains(t, repo.deleted, "old")
}

func TestSessionLifecycleManager_RefreshSessionWithRotation_Branches(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sessionManager := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())
	cfg := DefaultSessionLifecycleConfig()
	cfg.RequireRefreshRotation = true

	slm := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), cfg)

	// Missing token -> ErrInvalidRefreshTokenProvided.
	_, _, err := slm.RefreshSessionWithRotation(context.Background(), "missing", "192.0.2.10", "ua")
	require.ErrorIs(t, err, ErrInvalidRefreshTokenProvided)

	// Security invalid due to inactivity -> ErrSessionSecurityValidationFailed.
	repo.sessions["sid-inactive"] = &Session{
		SessionID:    "sid-inactive",
		Username:     "alice",
		RefreshToken: "rt-inactive",
		IPAddress:    "192.0.2.10",
		UserAgent:    "ua",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now().Add(-48 * time.Hour),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	_, _, err = slm.RefreshSessionWithRotation(context.Background(), "rt-inactive", "192.0.2.10", "ua")
	require.ErrorIs(t, err, ErrSessionSecurityValidationFailed)

	// Max session lifetime reached -> ErrSessionMaxLifetimeReached.
	repo.sessions["sid-old"] = &Session{
		SessionID:    "sid-old",
		Username:     "alice",
		RefreshToken: "rt-old",
		IPAddress:    "192.0.2.10",
		UserAgent:    "ua",
		CreatedAt:    time.Now().Add(-31 * 24 * time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	_, _, err = slm.RefreshSessionWithRotation(context.Background(), "rt-old", "192.0.2.10", "ua")
	require.ErrorIs(t, err, ErrSessionMaxLifetimeReached)

	// Rotation failure when UpdateSession fails.
	repo2 := newInMemorySessionRepo()
	repo2.sessions["sid-rot"] = &Session{
		SessionID:    "sid-rot",
		Username:     "alice",
		RefreshToken: "rt-rot",
		IPAddress:    "192.0.2.10",
		UserAgent:    "ua",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	sessionManager2 := newSessionManager(&sessionRepoUpdateErr{inMemorySessionRepo: repo2, err: errors.New("update failed")})
	slm2 := NewSessionLifecycleManager(sessionManager2, securityManager, nil, zap.NewNop(), cfg)
	_, _, err = slm2.RefreshSessionWithRotation(context.Background(), "rt-rot", "192.0.2.10", "ua")
	require.ErrorIs(t, err, ErrRefreshTokenRotationFailed)

	// No rotation path returns the same refresh token.
	cfgNoRotate := DefaultSessionLifecycleConfig()
	cfgNoRotate.RequireRefreshRotation = false
	sessionManager3 := newSessionManager(repo)
	slm3 := NewSessionLifecycleManager(sessionManager3, securityManager, nil, zap.NewNop(), cfgNoRotate)
	repo.sessions["sid-norotate"] = &Session{
		SessionID:    "sid-norotate",
		Username:     "alice",
		RefreshToken: "rt-norotate",
		IPAddress:    "192.0.2.10",
		UserAgent:    "ua",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}
	_, token, err := slm3.RefreshSessionWithRotation(context.Background(), "rt-norotate", "192.0.2.10", "ua")
	require.NoError(t, err)
	require.Equal(t, "rt-norotate", token)

	// GetSessionHealth error path.
	slm4 := NewSessionLifecycleManager(newSessionManager(&sessionRepoGetSessionErr{inMemorySessionRepo: repo, err: errors.New("missing")}), securityManager, nil, zap.NewNop(), DefaultSessionLifecycleConfig())
	_, err = slm4.GetSessionHealth(context.Background(), "missing")
	require.Error(t, err)
}

