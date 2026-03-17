package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type inMemorySessionRepo struct {
	nextSessionID int
	sessions      map[string]*Session
	devices       map[string]*Device
	deleted       []string
	updated       []string
}

func newInMemorySessionRepo() *inMemorySessionRepo {
	return &inMemorySessionRepo{
		nextSessionID: 1,
		sessions:      make(map[string]*Session),
		devices:       make(map[string]*Device),
	}
}

func (r *inMemorySessionRepo) CreateSession(_ context.Context, username, ipAddress, userAgent string) (*Session, error) {
	sessionID := "sid-" + time.Now().Format("150405") + "-" + string(rune('a'+r.nextSessionID))
	r.nextSessionID++

	session := &Session{
		SessionID:    sessionID,
		Username:     username,
		RefreshToken: "rt-" + sessionID,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(SessionDuration),
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
	}

	r.sessions[sessionID] = session
	return session, nil
}

func (r *inMemorySessionRepo) GetSessionByRefreshToken(_ context.Context, refreshToken string) (*Session, error) {
	for _, session := range r.sessions {
		if session.RefreshToken == refreshToken || session.PreviousRefreshToken == refreshToken {
			return session, nil
		}
	}
	return nil, errors.New("not found")
}

func (r *inMemorySessionRepo) GetSession(_ context.Context, sessionID string) (*Session, error) {
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, errors.New("not found")
	}
	return session, nil
}

func (r *inMemorySessionRepo) UpdateSession(_ context.Context, sessionID, refreshToken, ipAddress string, lastActivity, expiresAt time.Time) error {
	session, ok := r.sessions[sessionID]
	if !ok {
		return errors.New("not found")
	}

	session.RefreshToken = refreshToken
	session.IPAddress = ipAddress
	session.LastActivity = lastActivity
	session.ExpiresAt = expiresAt

	r.updated = append(r.updated, sessionID)
	return nil
}

func (r *inMemorySessionRepo) DeleteSession(_ context.Context, sessionID string) error {
	delete(r.sessions, sessionID)
	r.deleted = append(r.deleted, sessionID)
	return nil
}

func (r *inMemorySessionRepo) GetUserSessions(_ context.Context, username string) ([]*Session, error) {
	var result []*Session
	for _, session := range r.sessions {
		if session.Username == username {
			result = append(result, session)
		}
	}
	return result, nil
}

func (r *inMemorySessionRepo) GetUserDevices(_ context.Context, username string) ([]*Device, error) {
	var result []*Device
	for _, device := range r.devices {
		if device.Username == username {
			result = append(result, device)
		}
	}
	return result, nil
}

func (r *inMemorySessionRepo) GetDevice(_ context.Context, deviceID string) (*Device, error) {
	device, ok := r.devices[deviceID]
	if !ok {
		return nil, errors.New("not found")
	}
	return device, nil
}

func (r *inMemorySessionRepo) CreateDevice(_ context.Context, device *Device) error {
	r.devices[device.DeviceID] = device
	return nil
}

func (r *inMemorySessionRepo) UpdateDevice(_ context.Context, device *Device) error {
	if _, ok := r.devices[device.DeviceID]; !ok {
		return errors.New("not found")
	}
	r.devices[device.DeviceID] = device
	return nil
}

func TestSessionManager_CreateSession_EnforcesLimitsAndCreatesDevice(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)

	// Seed sessions at limit; one oldest should be removed.
	oldestID := "sid-oldest"
	repo.sessions[oldestID] = &Session{
		SessionID:    oldestID,
		Username:     "alice",
		RefreshToken: "rt-oldest",
		CreatedAt:    time.Now().Add(-10 * time.Hour),
		LastActivity: time.Now(),
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}
	for i := 0; i < MaxSessionsPerUser-1; i++ {
		id := "sid-" + string(rune('b'+i))
		repo.sessions[id] = &Session{
			SessionID:    id,
			Username:     "alice",
			RefreshToken: "rt-" + id,
			CreatedAt:    time.Now().Add(-1 * time.Hour),
			LastActivity: time.Now(),
			ExpiresAt:    time.Now().Add(1 * time.Hour),
		}
	}

	session, err := sm.CreateSession(context.Background(), "alice", "iPhone", "Mozilla/5.0 (iPhone)", "192.0.2.10", "password")
	require.NoError(t, err)
	require.NotEmpty(t, session.SessionID)
	require.NotEmpty(t, session.DeviceID)
	require.Equal(t, "iPhone", session.DeviceName)
	require.Equal(t, "password", session.AuthMethod)

	require.Contains(t, repo.deleted, oldestID)
	require.NotEmpty(t, repo.devices)

	// Device type detection should have marked this as mobile.
	createdDevice, err := repo.GetDevice(context.Background(), session.DeviceID)
	require.NoError(t, err)
	require.Equal(t, "mobile", createdDevice.DeviceType)
	require.Equal(t, TrustLevelUntrusted, createdDevice.TrustLevel)
}

func TestSessionManager_ValidateRefreshToken_RotationWindow(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)

	current := &Session{
		SessionID:            "sid-1",
		Username:             "alice",
		RefreshToken:         "rt-new",
		PreviousRefreshToken: "rt-old",
		TokenRotatedAt:       time.Now(),
		ExpiresAt:            time.Now().Add(1 * time.Hour),
		IPAddress:            "192.0.2.1",
		LastActivity:         time.Now(),
		CreatedAt:            time.Now(),
	}
	repo.sessions[current.SessionID] = current

	// Previous token should be accepted within grace period.
	got, err := sm.ValidateRefreshToken(context.Background(), "rt-old")
	require.NoError(t, err)
	require.Equal(t, current.SessionID, got.SessionID)

	// But not after the grace window.
	current.TokenRotatedAt = time.Now().Add(-RefreshTokenRotationWindow - time.Second)
	_, err = sm.ValidateRefreshToken(context.Background(), "rt-old")
	require.ErrorIs(t, err, ErrInvalidRefreshToken)
}

func TestSessionManager_ValidateRefreshToken_ExpiredSession(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)

	repo.sessions["sid-expired"] = &Session{
		SessionID:    "sid-expired",
		Username:     "alice",
		RefreshToken: "rt-expired",
		ExpiresAt:    time.Now().Add(-time.Minute),
		LastActivity: time.Now(),
		CreatedAt:    time.Now(),
	}

	_, err := sm.ValidateRefreshToken(context.Background(), "rt-expired")
	require.ErrorIs(t, err, ErrSessionExpired)
}

func TestSessionManager_RotateRefreshToken_ExtendsExpiryAndUpdatesRepo(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)

	session := &Session{
		SessionID:    "sid-rotate",
		Username:     "alice",
		RefreshToken: "rt-old",
		IPAddress:    "192.0.2.1",
		LastActivity: time.Now().Add(-time.Hour),
		CreatedAt:    time.Now().Add(-time.Hour),
		ExpiresAt:    time.Now().Add(10 * time.Minute), // triggers sliding extension
	}
	repo.sessions[session.SessionID] = session

	newToken, err := sm.RotateRefreshToken(context.Background(), session)
	require.NoError(t, err)
	require.NotEmpty(t, newToken)
	require.NotEqual(t, "rt-old", newToken)
	require.Equal(t, newToken, session.RefreshToken)
	require.Equal(t, "rt-old", session.PreviousRefreshToken)
	require.Contains(t, repo.updated, session.SessionID)
	require.True(t, time.Until(session.ExpiresAt) > SessionDuration/2)
}

func TestSessionManager_UpdateSessionActivity_SlidingExpiry(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	sm := newSessionManager(repo)

	session := &Session{
		SessionID:    "sid-activity",
		Username:     "alice",
		RefreshToken: "rt",
		IPAddress:    "192.0.2.1",
		LastActivity: time.Now().Add(-time.Hour),
		CreatedAt:    time.Now().Add(-time.Hour),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}
	repo.sessions[session.SessionID] = session

	require.NoError(t, sm.UpdateSessionActivity(context.Background(), session.SessionID, "192.0.2.99"))
	require.Equal(t, "192.0.2.99", session.IPAddress)
	require.Contains(t, repo.updated, session.SessionID)
	require.True(t, time.Until(session.ExpiresAt) > SessionDuration/2)
}

func TestSessionManager_DetectAnomalousSession(t *testing.T) {
	t.Parallel()

	sm := newSessionManager(nil)

	session := &Session{
		IPAddress:    "192.0.2.1",
		LastActivity: time.Now(),
	}

	isAnomalous, reason := sm.DetectAnomalousSession(context.Background(), session, "203.0.113.5")
	require.True(t, isAnomalous)
	require.NotEmpty(t, reason)

	session.LastActivity = time.Now().Add(-SessionInactivityTimeout - time.Second)
	isAnomalous, reason = sm.DetectAnomalousSession(context.Background(), session, "192.168.1.2")
	require.True(t, isAnomalous)
	require.NotEmpty(t, reason)

	session.LastActivity = time.Now()
	isAnomalous, reason = sm.DetectAnomalousSession(context.Background(), session, "192.0.2.1")
	require.False(t, isAnomalous)
	require.Empty(t, reason)
}

func TestSessionManager_CleanupInactiveSessions(t *testing.T) {
	t.Parallel()

	sm := newSessionManager(nil)
	require.NoError(t, sm.CleanupInactiveSessions(context.Background()))
}

func TestSessionManager_TokenVersioningHelpers(t *testing.T) {
	t.Parallel()

	sm := newSessionManager(nil)

	require.Equal(t, 0, sm.GetTokenVersion("alice"))
	sm.InvalidateAllUserTokens("alice")
	require.GreaterOrEqual(t, sm.GetTokenVersion("alice"), 100)
}
