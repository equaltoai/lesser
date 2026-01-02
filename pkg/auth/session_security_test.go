package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestSessionSecurityManager_BasicsAndTokenHelpers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ssm := NewSessionSecurityManager(logger, nil)
	require.NotNil(t, ssm)
	require.NotNil(t, ssm.config)

	token, err := ssm.GenerateCSRFToken()
	require.NoError(t, err)
	require.NotEmpty(t, token)

	assert.False(t, ssm.ValidateCSRFToken("", token))
	assert.False(t, ssm.ValidateCSRFToken(token, ""))
	assert.True(t, ssm.ValidateCSRFToken(token, token))

	fp := ssm.GenerateDeviceFingerprint("ua", "192.0.2.1", "en")
	require.NotNil(t, fp)
	assert.NotEmpty(t, fp.Fingerprint)
}

func TestSessionSecurityManager_ValidateSessionSecurity_MainBranches(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ssm := NewSessionSecurityManager(logger, &AdvancedSessionSecurityConfig{
		EnableIPBinding:            true,
		EnableDeviceFingerprinting: true,
		SessionTimeout:             24 * time.Hour,
		SessionFixationPrevention:  true,
		GracePeriod:                time.Hour,
	})

	now := time.Now()

	expired := &Session{
		SessionID:      "s1",
		ExpiresAt:      now.Add(-time.Minute),
		LastActivity:   now,
		IPAddress:      "192.0.2.1",
		UserAgent:      "ua",
		CreatedAt:      now.Add(-time.Hour),
		RefreshToken:   "rt",
		DeviceID:       "dev",
		DeviceName:     "d",
		AuthMethod:     "password",
		TokenRotatedAt: now,
	}

	res, err := ssm.ValidateSessionSecurity(context.Background(), expired, ssm.GenerateDeviceFingerprint("ua", "192.0.2.1", ""))
	require.NoError(t, err)
	assert.False(t, res.Valid)
	assert.Contains(t, res.RiskFactors, "session_expired")

	inactive := *expired
	inactive.ExpiresAt = now.Add(time.Hour)
	inactive.LastActivity = now.Add(-48 * time.Hour)
	res, err = ssm.ValidateSessionSecurity(context.Background(), &inactive, ssm.GenerateDeviceFingerprint("ua", "192.0.2.1", ""))
	require.NoError(t, err)
	assert.False(t, res.Valid)
	assert.Contains(t, res.RiskFactors, "session_inactive")

	// IP binding mismatch outside subnet triggers challenge.
	okSession := *expired
	okSession.ExpiresAt = now.Add(time.Hour)
	okSession.LastActivity = now.Add(-time.Minute)
	okSession.IPAddress = "192.0.2.1"
	okSession.UserAgent = "ua"
	res, err = ssm.ValidateSessionSecurity(context.Background(), &okSession, ssm.GenerateDeviceFingerprint("ua", "198.51.100.2", ""))
	require.NoError(t, err)
	assert.True(t, res.Valid)
	assert.True(t, res.RequiresChallenge)
	assert.Contains(t, res.RiskFactors, "ip_address_changed")

	// Device user-agent mismatch reduces trust.
	res, err = ssm.ValidateSessionSecurity(context.Background(), &okSession, ssm.GenerateDeviceFingerprint("other", "192.0.2.1", ""))
	require.NoError(t, err)
	assert.Contains(t, res.RiskFactors, "device_fingerprint_changed")
}

func TestSessionSecurityManager_AnomaliesAndRiskScoring(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ssm := NewSessionSecurityManager(logger, nil)

	now := time.Now()
	session := &Session{
		SessionID:    "s",
		IPAddress:    "192.0.2.1",
		UserAgent:    "ua",
		CreatedAt:    now.Add(-10 * 24 * time.Hour),
		LastActivity: now,
		ExpiresAt:    now.Add(time.Hour),
	}

	fp := ssm.GenerateDeviceFingerprint("other", "198.51.100.2", "")
	flags := ssm.DetectSessionAnomalies(session, fp)
	assert.True(t, flags.IPChanged)
	assert.True(t, flags.DeviceChanged)

	validation := &SecurityValidationResult{TrustScore: 0.4}
	risk := ssm.CalculateSessionRisk(session, flags, validation)
	assert.Greater(t, risk, 0.0)
	assert.LessOrEqual(t, risk, 1.0)

	assert.True(t, ssm.ShouldRequire2FA(0.7, session))
	assert.True(t, ssm.ShouldRequire2FA(0.4, session))
	assert.False(t, ssm.ShouldRequire2FA(0.1, session))
}

func TestSessionSecurityManager_MiscHelpers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ssm := NewSessionSecurityManager(logger, &AdvancedSessionSecurityConfig{SessionFixationPrevention: false})

	assert.False(t, ssm.isIPInSameSubnet("not-an-ip", "192.0.2.1"))
	assert.True(t, ssm.isIPInSameSubnet("192.0.2.1", "192.0.2.99"))
	assert.False(t, ssm.isIPInSameSubnet("192.0.2.1", "198.51.100.2"))

	old := "sid"
	sid, err := ssm.PreventSessionFixation(old)
	require.NoError(t, err)
	assert.Equal(t, old, sid)

	ssm.config.SessionFixationPrevention = true
	newID, err := ssm.PreventSessionFixation(old)
	require.NoError(t, err)
	assert.NotEmpty(t, newID)
	assert.NotEqual(t, old, newID)

	cookie, err := ssm.GenerateSecureSessionCookie("sid", "alice")
	require.NoError(t, err)
	assert.Len(t, cookie, 64)

	missing := ssm.ValidateSecurityHeaders(map[string]string{"X-Frame-Options": "SAMEORIGIN"})
	assert.NotEmpty(t, missing)

	assert.True(t, ssm.IsHighRiskUserAgent("curl/8.0"))
	assert.False(t, ssm.IsHighRiskUserAgent("Mozilla/5.0"))

	require.NoError(t, ssm.RotateSessionSecrets(&Session{SessionID: "sid"}))
}
