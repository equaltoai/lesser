package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSessionValidator_ConfigNilAndBasicFailurePaths(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sessionManager := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())
	fingerprintManager := NewDeviceFingerprintManager(repo, zap.NewNop(), DefaultDeviceFingerprintConfig())
	lifecycleManager := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), DefaultSessionLifecycleConfig())

	validator := NewSessionValidator(sessionManager, securityManager, lifecycleManager, fingerprintManager, zap.NewNop(), nil)
	require.NotNil(t, validator.config)

	// Session not found.
	resp, err := validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:  "missing",
		IPAddress:  "192.0.2.10",
		UserAgent:  "Mozilla/5.0",
		Timestamp:  time.Now(),
		Headers:    map[string]string{},
		RequestMethod: "GET",
	})
	require.NoError(t, err)
	require.False(t, resp.Valid)
	require.Contains(t, resp.FailedChecks, "session_not_found")
	require.Equal(t, "deny", resp.SuggestedAction)

	// Expired session.
	repo.sessions["expired"] = &Session{
		SessionID:    "expired",
		Username:     "alice",
		RefreshToken: "rt",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(-time.Minute),
	}
	resp, err = validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:  "expired",
		IPAddress:  "192.0.2.10",
		UserAgent:  "Mozilla/5.0",
		Timestamp:  time.Now(),
		Headers:    map[string]string{},
		RequestMethod: "GET",
	})
	require.NoError(t, err)
	require.Contains(t, resp.FailedChecks, "session_expired")
	require.Equal(t, "deny", resp.SuggestedAction)

	// Inactive session.
	repo.sessions["inactive"] = &Session{
		SessionID:    "inactive",
		Username:     "alice",
		RefreshToken: "rt",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now().Add(-time.Hour),
		LastActivity: time.Now().Add(-48 * time.Hour),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	resp, err = validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:  "inactive",
		IPAddress:  "192.0.2.10",
		UserAgent:  "Mozilla/5.0",
		Timestamp:  time.Now(),
		Headers:    map[string]string{},
		RequestMethod: "GET",
	})
	require.NoError(t, err)
	require.Contains(t, resp.FailedChecks, "session_inactive")
	require.Equal(t, "deny", resp.SuggestedAction)
}

func TestSessionValidator_SecurityAndScoringBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sessionManager := newSessionManager(repo)
	securityManager := NewSessionSecurityManager(zap.NewNop(), DefaultAdvancedSessionSecurityConfig())
	fingerprintManager := NewDeviceFingerprintManager(repo, zap.NewNop(), DefaultDeviceFingerprintConfig())
	lifecycleManager := NewSessionLifecycleManager(sessionManager, securityManager, nil, zap.NewNop(), DefaultSessionLifecycleConfig())

	cfg := DefaultSessionValidationConfig()
	cfg.RequireSecurityHeaders = true
	cfg.StrictValidation = true
	cfg.LogAllValidations = true
	cfg.MaxSessionAge = 30 * time.Minute

	validator := NewSessionValidator(sessionManager, securityManager, lifecycleManager, fingerprintManager, zap.NewNop(), cfg)

	repo.sessions["sid"] = &Session{
		SessionID:    "sid",
		Username:     "alice",
		RefreshToken: "rt",
		IPAddress:    "192.0.2.10",
		UserAgent:    "Mozilla/5.0",
		CreatedAt:    time.Now().Add(-2 * time.Hour), // triggers "session_too_old"
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	resp, err := validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:  "sid",
		IPAddress:  "203.0.113.5",
		UserAgent:  "curl/8.0",
		CSRFToken:  "csrf",
		Timestamp:  time.Now(),
		RequestPath: "/api/v1/test",
		RequestMethod: "POST",
		Headers:    map[string]string{}, // missing security headers
	})
	require.NoError(t, err)
	require.Contains(t, resp.FailedChecks, "session_too_old")
	require.Contains(t, resp.FailedChecks, "ip_address_changed")
	require.Contains(t, resp.FailedChecks, "high_risk_user_agent")
	require.True(t, resp.RequiresChallenge)
	require.True(t, resp.RequiresReauth)
	require.NotEmpty(t, resp.RequiredActions)

	// With required headers present, "security_headers_present" is validated.
	repo.sessions["sid"].CreatedAt = time.Now() // avoid too old branch to change ratios
	resp, err = validator.ValidateSession(context.Background(), &SessionValidationRequest{
		SessionID:  "sid",
		IPAddress:  "192.0.2.10",
		UserAgent:  "Mozilla/5.0",
		CSRFToken:  "csrf",
		Timestamp:  time.Now(),
		RequestMethod: "POST",
		Headers: map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"X-XSS-Protection":       "1; mode=block",
		},
	})
	require.NoError(t, err)
	require.Contains(t, resp.ValidatedChecks, "security_headers_present")
}

