package auth

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newSessionValidatorFixture(t *testing.T) (*inMemorySessionRepo, *SessionValidator) {
	t.Helper()

	repo := newInMemorySessionRepo()
	sessionManager := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())
	fingerprintManager := NewDeviceFingerprintManager(repo, zap.NewNop(), DefaultDeviceFingerprintConfig())
	lifecycleManager := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), DefaultSessionLifecycleConfig())

	validator := NewSessionValidator(
		sessionManager,
		securityManager,
		lifecycleManager,
		fingerprintManager,
		zap.NewNop(),
		DefaultSessionValidationConfig(),
	)

	return repo, validator
}

func TestSessionValidator_NoSessionIdentifier(t *testing.T) {
	t.Parallel()

	_, validator := newSessionValidatorFixture(t)

	resp, err := validator.ValidateSession(context.Background(), &SessionValidationRequest{
		IPAddress: "192.0.2.10",
		UserAgent: "Mozilla/5.0",
		Timestamp: time.Now(),
	})
	require.NoError(t, err)
	require.False(t, resp.Valid)
	require.Contains(t, resp.FailedChecks, "no_session_identifier")
	require.Equal(t, "deny", resp.SuggestedAction)
}

func TestSessionValidator_ValidateAndExtendSession(t *testing.T) {
	t.Parallel()

	repo, validator := newSessionValidatorFixture(t)

	session := &Session{
		SessionID:    "sid-1",
		Username:     "alice",
		RefreshToken: "rt-1",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0 (iPhone)",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now().Add(-time.Minute),
		ExpiresAt:    time.Now().Add(1 * time.Hour), // triggers extension
	}
	repo.sessions[session.SessionID] = session

	repo.devices["dev-1"] = &storage.Device{
		DeviceID:      "dev-1",
		Username:      "alice",
		DeviceType:    "mobile",
		LastIPAddress: "192.0.2.10",
		LastUserAgent: "Mozilla/5.0 (iPhone)",
		CreatedAt:     time.Now().Add(-2 * time.Hour),
		LastSeenAt:    time.Now().Add(-time.Minute),
		TrustLevel:    TrustLevelTrusted,
	}

	resp, err := validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:   session.SessionID,
		CSRFToken:   "csrf",
		IPAddress:   "192.0.2.10",
		UserAgent:   "Mozilla/5.0 (iPhone)",
		Headers:     map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "X-XSS-Protection": "1; mode=block"},
		Timestamp:   time.Now(),
		RequestPath: "/api/v1/test",
	})
	require.NoError(t, err)
	require.True(t, resp.Valid)
	require.Equal(t, session.SessionID, resp.SessionID)
	require.Equal(t, "alice", resp.Username)
	require.Equal(t, "allow", resp.SuggestedAction)
	require.True(t, resp.ExtendedSession)
	require.True(t, resp.NewExpiresAt.After(time.Now().Add(12*time.Hour)))
	require.Contains(t, resp.ValidatedChecks, "session_extended")

	// Quick path wrappers.
	ok, err := validator.QuickValidateSession(context.Background(), session.SessionID, "192.0.2.10", "Mozilla/5.0 (iPhone)")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestSessionValidator_HighSecurityOverride_RequiresReauth(t *testing.T) {
	t.Parallel()

	repo, validator := newSessionValidatorFixture(t)

	session := &Session{
		SessionID:    "sid-2",
		Username:     "alice",
		RefreshToken: "rt-2",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	repo.sessions[session.SessionID] = session

	resp, err := validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:           session.SessionID,
		IPAddress:           "192.0.2.10",
		UserAgent:           "Mozilla/5.0",
		Timestamp:           time.Now(),
		RequireHighSecurity: true,
		Headers:             map[string]string{},
		DeviceFingerprint:   map[string]string{},
		RequestMethod:       "GET",
	})
	require.NoError(t, err)
	require.True(t, resp.RequiresReauth)
	require.Equal(t, "reauth", resp.SuggestedAction)
	require.Contains(t, resp.RequiredActions, "reauthentication_required")
}

func TestSessionValidator_DeviceMismatch_TriggersChallenge(t *testing.T) {
	t.Parallel()

	repo, validator := newSessionValidatorFixture(t)

	session := &Session{
		SessionID:    "sid-3",
		Username:     "alice",
		RefreshToken: "rt-3",
		IPAddress:    "192.168.1.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	repo.sessions[session.SessionID] = session

	// Similar user agent, same subnet -> low match confidence (< threshold).
	repo.devices["dev-1"] = &storage.Device{
		DeviceID:      "dev-1",
		Username:      "alice",
		LastIPAddress: "192.168.1.11",
		LastUserAgent: "Mozilla/5.0 Chrome/120",
		CreatedAt:     time.Now().Add(-time.Hour),
		LastSeenAt:    time.Now().Add(-time.Minute),
		TrustLevel:    TrustLevelUntrusted,
	}

	resp, err := validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID: session.SessionID,
		IPAddress: "192.168.1.12",
		UserAgent: "Mozilla/5.0 Chrome/121",
		Timestamp: time.Now(),
		CSRFToken: "csrf",
		Headers:   map[string]string{},
	})
	require.NoError(t, err)
	require.True(t, resp.Valid)
	require.True(t, resp.RequiresChallenge)
	require.Contains(t, resp.RequiredActions, "challenge_required")
	require.Equal(t, "challenge", resp.SuggestedAction)
}

func TestSessionValidator_ValidateRefreshTokenRequest_Path(t *testing.T) {
	t.Parallel()

	repo, validator := newSessionValidatorFixture(t)

	session := &Session{
		SessionID:    "sid-rt",
		Username:     "alice",
		RefreshToken: "rt",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	repo.sessions[session.SessionID] = session

	resp, err := validator.ValidateRefreshTokenRequest(context.Background(), "rt", "192.0.2.10", "Mozilla/5.0")
	require.NoError(t, err)
	require.Equal(t, session.SessionID, resp.SessionID)
}
