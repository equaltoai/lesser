package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSessionLifecycleManager_CreateSessionWithLifecycle_EnforcesConcurrentLimit(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), &AdvancedSessionSecurityConfig{
		EnableIPBinding:            true,
		EnableDeviceFingerprinting: true,
		SessionFixationPrevention:  false,
		SessionTimeout:             24 * time.Hour,
	})

	oldestID := "sid-existing"
	repo.sessions[oldestID] = &Session{
		SessionID:    oldestID,
		Username:     "alice",
		RefreshToken: "rt-existing",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}

	slm := NewSessionLifecycleManager(sm, securityManager, nil, zap.NewNop(), &SessionLifecycleConfig{
		SessionDuration:           7 * 24 * time.Hour,
		MaxSessionDuration:        30 * 24 * time.Hour,
		InactivityTimeout:         24 * time.Hour,
		RefreshTokenDuration:      30 * 24 * time.Hour,
		CleanupInterval:           10 * time.Millisecond,
		ExpiredSessionGracePeriod: 15 * time.Minute,
		MaxInactiveSessions:       5,
		RequireRefreshRotation:    true,
		SessionFixationPrevention: true, // calls into securityManager, but returns the same session ID
		ConcurrentSessionLimit:    1,
		AllowSessionExtension:     true,
		ExtensionThreshold:        6 * time.Hour,
		MaxSessionExtensions:      3,
	})

	session, err := slm.CreateSessionWithLifecycle(context.Background(), "alice", "device", "curl/8.0", "192.0.2.10", "password")
	require.NoError(t, err)
	require.NotEmpty(t, session.SessionID)
	require.Contains(t, repo.deleted, oldestID)

	// Exercise ScheduleCleanup quickly (should exit on context cancellation).
	ctx, cancel := context.WithCancel(context.Background())
	slm.ScheduleCleanup(ctx)
	cancel()
}

func TestSessionLifecycleManager_RefreshSessionWithRotation_SecurityAndExtensionPaths(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())

	slm := NewSessionLifecycleManager(sm, securityManager, nil, zap.NewNop(), &SessionLifecycleConfig{
		SessionDuration:        7 * 24 * time.Hour,
		MaxSessionDuration:     30 * 24 * time.Hour,
		InactivityTimeout:      24 * time.Hour,
		RequireRefreshRotation: true,
		AllowSessionExtension:  true,
		ExtensionThreshold:     6 * time.Hour,
		MaxSessionExtensions:   3,
	})

	active := &Session{
		SessionID:    "sid-active",
		Username:     "alice",
		RefreshToken: "rt-active",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(1 * time.Hour), // should trigger extension
	}
	repo.sessions[active.SessionID] = active

	updatedSession, newToken, err := slm.RefreshSessionWithRotation(context.Background(), "rt-active", "192.0.2.10", "Mozilla/5.0")
	require.NoError(t, err)
	require.Equal(t, active.SessionID, updatedSession.SessionID)
	require.NotEmpty(t, newToken)
	require.NotEqual(t, "rt-active", newToken)
	require.Equal(t, newToken, updatedSession.RefreshToken)

	// Security validation failure (inactive session).
	inactive := &Session{
		SessionID:    "sid-inactive",
		Username:     "alice",
		RefreshToken: "rt-inactive",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now(),
		LastActivity: time.Now().Add(-48 * time.Hour),
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	repo.sessions[inactive.SessionID] = inactive
	_, _, err = slm.RefreshSessionWithRotation(context.Background(), "rt-inactive", "192.0.2.10", "Mozilla/5.0")
	require.ErrorIs(t, err, ErrSessionSecurityValidationFailed)

	// Max lifetime reached.
	old := &Session{
		SessionID:    "sid-old",
		Username:     "alice",
		RefreshToken: "rt-old",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now().Add(-31 * 24 * time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	repo.sessions[old.SessionID] = old
	_, _, err = slm.RefreshSessionWithRotation(context.Background(), "rt-old", "192.0.2.10", "Mozilla/5.0")
	require.ErrorIs(t, err, ErrSessionMaxLifetimeReached)
}

func TestSessionLifecycleManager_GetSessionHealth(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())

	slm := NewSessionLifecycleManager(sm, securityManager, nil, zap.NewNop(), DefaultSessionLifecycleConfig())

	session := &Session{
		SessionID:    "sid-health",
		Username:     "alice",
		RefreshToken: "rt",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now().Add(-time.Minute),
		ExpiresAt:    time.Now().Add(2 * time.Hour),
	}
	repo.sessions[session.SessionID] = session

	health, err := slm.GetSessionHealth(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Equal(t, session.SessionID, health.SessionID)
	require.True(t, health.IsActive)
	require.NotZero(t, health.TimeUntilExpiry)
}
