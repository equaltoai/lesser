package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type inMemoryAuthAccountRepo struct {
	users         map[string]*storage.User
	actors        map[string]*activitypub.Actor
	devices       map[string]*storage.Device
	sessions      map[string]*storage.Session
	walletCreds   map[string][]*storage.WalletCredential
	recoveryStore map[string]map[string]any

	errGetUser            error
	errUpdateUser         error
	errStoreRecoveryToken error
}

func newInMemoryAuthAccountRepo() *inMemoryAuthAccountRepo {
	return &inMemoryAuthAccountRepo{
		users:         make(map[string]*storage.User),
		actors:        make(map[string]*activitypub.Actor),
		devices:       make(map[string]*storage.Device),
		sessions:      make(map[string]*storage.Session),
		walletCreds:   make(map[string][]*storage.WalletCredential),
		recoveryStore: make(map[string]map[string]any),
	}
}

func (r *inMemoryAuthAccountRepo) GetUser(_ context.Context, username string) (*storage.User, error) {
	if r.errGetUser != nil {
		return nil, r.errGetUser
	}
	user, ok := r.users[username]
	if !ok {
		return nil, errors.New("not found")
	}
	return user, nil
}

func (r *inMemoryAuthAccountRepo) UpdateUser(_ context.Context, username string, updates map[string]any) error {
	if r.errUpdateUser != nil {
		return r.errUpdateUser
	}
	user, ok := r.users[username]
	if !ok {
		return errors.New("not found")
	}
	if v, ok := updates["password_hash"].(string); ok {
		user.PasswordHash = v
	}
	return nil
}

func (r *inMemoryAuthAccountRepo) GetActor(_ context.Context, username string) (*activitypub.Actor, error) {
	actor, ok := r.actors[username]
	if !ok {
		return nil, errors.New("not found")
	}
	return actor, nil
}

func (r *inMemoryAuthAccountRepo) GetDevice(_ context.Context, deviceID string) (*storage.Device, error) {
	device, ok := r.devices[deviceID]
	if !ok {
		return nil, errors.New("not found")
	}
	return device, nil
}

func (r *inMemoryAuthAccountRepo) GetSession(_ context.Context, sessionID string) (*storage.Session, error) {
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, errors.New("not found")
	}
	return session, nil
}

func (r *inMemoryAuthAccountRepo) GetUserWalletCredentials(_ context.Context, username string) ([]*storage.WalletCredential, error) {
	return r.walletCreds[username], nil
}

func (r *inMemoryAuthAccountRepo) MarkWalletChallengeSpent(_ context.Context, _ string) error {
	return nil
}

func (r *inMemoryAuthAccountRepo) GetWalletChallenge(_ context.Context, _ string) (*storage.WalletChallenge, error) {
	return nil, errors.New("not implemented")
}

func (r *inMemoryAuthAccountRepo) StoreRecoveryToken(_ context.Context, key string, data map[string]any) error {
	if r.errStoreRecoveryToken != nil {
		return r.errStoreRecoveryToken
	}
	r.recoveryStore[key] = data
	return nil
}

type fakeRateLimitRepo struct {
	limited  map[string]bool
	unlockAt map[string]time.Time
	attempts map[string]int
}

func newFakeRateLimitRepo() *fakeRateLimitRepo {
	return &fakeRateLimitRepo{
		limited:  make(map[string]bool),
		unlockAt: make(map[string]time.Time),
		attempts: make(map[string]int),
	}
}

func (r *fakeRateLimitRepo) IsRateLimited(_ context.Context, identifier string) (bool, time.Time, error) {
	return r.limited[identifier], r.unlockAt[identifier], nil
}

func (r *fakeRateLimitRepo) RecordLoginAttempt(_ context.Context, identifier string, success bool) error {
	if !success {
		r.attempts[identifier]++
	}
	return nil
}

func (r *fakeRateLimitRepo) ClearLoginAttempts(_ context.Context, identifier string) error {
	delete(r.attempts, identifier)
	return nil
}

func (r *fakeRateLimitRepo) GetLoginAttemptCount(_ context.Context, identifier string, _ time.Time) (int, error) {
	return r.attempts[identifier], nil
}

type oauthStub struct {
	validateClientErr      error
	validateRedirectURIErr error
	authCode               string
}

func (s oauthStub) ValidateClient(_ context.Context, _, _ string) error { return s.validateClientErr }
func (s oauthStub) ValidateRedirectURI(_ context.Context, _, _ string) error {
	return s.validateRedirectURIErr
}
func (s oauthStub) GenerateAuthorizationCode() (string, error) { return s.authCode, nil }

func TestAuthService_AuthenticateWithPassword_SuccessAndErrorPaths(t *testing.T) {
	t.Parallel()

	as := &AuthService{
		accountRepo:    newInMemoryAuthAccountRepo(),
		sessionManager: newSessionManager(newInMemorySessionRepo()),
		rateLimiter:    &RateLimiter{accountRepo: newFakeRateLimitRepo()},
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{Domain: "example.com"},
		oauthService:   oauthStub{authCode: "code"},
	}

	_, err := as.AuthenticateWithPassword(context.Background(), "alice", "password", "device", "ua", "192.0.2.10")
	require.ErrorIs(t, err, ErrPasswordAuthDisabled)
}

func TestAuthService_RefreshAccessToken_AndLegacyDelegates(t *testing.T) {
	t.Parallel()

	accountRepo := newInMemoryAuthAccountRepo()
	sessionRepo := newInMemorySessionRepo()
	sessionManager := newSessionManager(sessionRepo)
	rateLimiter := &RateLimiter{accountRepo: newFakeRateLimitRepo()}

	as := &AuthService{
		accountRepo:    accountRepo,
		sessionManager: sessionManager,
		rateLimiter:    rateLimiter,
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{},
		oauthService: oauthStub{
			validateClientErr:      errors.New("bad client"),
			validateRedirectURIErr: errors.New("bad redirect"),
			authCode:               "code-1",
		},
	}

	session := &Session{
		SessionID:    "sid",
		Username:     "alice",
		RefreshToken: "rt",
		DeviceID:     "dev",
		IPAddress:    "192.0.2.10",
		LastActivity: time.Now(),
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	sessionRepo.sessions[session.SessionID] = session

	// Successful refresh rotates token by default.
	resp, err := as.RefreshAccessToken(context.Background(), "rt", "192.0.2.11")
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.NotEmpty(t, resp.RefreshToken)

	// Invalid refresh token.
	_, err = as.RefreshAccessToken(context.Background(), "rt-missing", "192.0.2.11")
	require.Error(t, err)

	// Legacy delegates.
	require.Error(t, as.ValidateClient(context.Background(), "", ""))
	require.Error(t, as.ValidateRedirectURI(context.Background(), "id", "uri"))
	code, err := as.GenerateAuthorizationCode()
	require.NoError(t, err)
	require.Equal(t, "code-1", code)
}

func TestAuthService_ChangePassword_TrustDevice_GetConfig_AndRecoveryToken(t *testing.T) {
	t.Parallel()

	accountRepo := newInMemoryAuthAccountRepo()
	accountRepo.users["alice"] = &storage.User{Username: "alice", PasswordHash: "hash", Approved: true}
	accountRepo.devices["dev-1"] = &storage.Device{DeviceID: "dev-1", Username: "bob"}

	sessionManager := newSessionManager(newInMemorySessionRepo())
	as := &AuthService{
		accountRepo:    accountRepo,
		sessionManager: sessionManager,
		rateLimiter:    &RateLimiter{accountRepo: newFakeRateLimitRepo()},
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{Stage: ""},
	}

	require.ErrorIs(t, as.ChangePassword(context.Background(), "alice", "wrong", "new-password-strong-1234567890"), ErrPasswordAuthDisabled)

	require.ErrorIs(t, as.TrustDevice(context.Background(), "alice", "dev-1"), ErrDeviceOwnershipMismatch)

	cfg := as.GetConfig()
	require.Equal(t, "development", cfg.Environment)

	token, err := as.GenerateRecoveryToken(context.Background(), "alice", "webauthn")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	accountRepo.errStoreRecoveryToken = errors.New("store failed")
	_, err = as.GenerateRecoveryToken(context.Background(), "alice", "webauthn")
	require.Error(t, err)

	// WebAuthn nil checks.
	_, _, err = as.BeginWebAuthnRegistration(context.Background(), "alice")
	require.ErrorIs(t, err, ErrWebAuthnNotConfigured)
	require.ErrorIs(t, as.FinishWebAuthnRegistration(context.Background(), "alice", "c", nil, ""), ErrWebAuthnNotConfigured)
	_, _, err = as.BeginWebAuthnLogin(context.Background(), "alice")
	require.ErrorIs(t, err, ErrWebAuthnNotConfigured)
	_, err = as.FinishWebAuthnLogin(context.Background(), "alice", "c", nil, "device", "ua", "ip")
	require.ErrorIs(t, err, ErrWebAuthnNotConfigured)
	_, err = as.GetWebAuthnCredentials(context.Background(), "alice")
	require.ErrorIs(t, err, ErrWebAuthnNotConfigured)
	require.ErrorIs(t, as.DeleteWebAuthnCredential(context.Background(), "alice", "id"), ErrWebAuthnNotConfigured)
	require.ErrorIs(t, as.UpdateWebAuthnCredentialName(context.Background(), "alice", "id", "name"), ErrWebAuthnNotConfigured)
}
