package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestAuthService_WebAuthnWrappers_CallUnderlyingService(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hashed"}
	existingCred := &storage.WebAuthnCredential{ID: base64.StdEncoding.EncodeToString([]byte{1}), UserID: "alice", PublicKey: []byte("pk")}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{existingCred}
	repo.credentialsByID[existingCred.ID] = existingCred

	engine := &fakeWebAuthnEngine{
		beginRegChallenge:   "chal-reg",
		beginLoginChallenge: "chal-login",
		createCredential: &webauthn.Credential{
			ID:        []byte{9, 9},
			PublicKey: []byte("pub"),
			Authenticator: webauthn.Authenticator{
				AAGUID:    []byte{0, 1, 2},
				SignCount: 7,
			},
		},
	}

	webAuthnSvc := &WebAuthnService{
		webAuthn: engine,
		repo:     repo,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	as := &AuthService{webAuthnService: webAuthnSvc}

	creation, regChallenge, err := as.BeginWebAuthnRegistration(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, creation)
	require.Equal(t, "chal-reg", regChallenge)

	require.NoError(t, as.FinishWebAuthnRegistration(context.Background(), "alice", regChallenge, []byte("ignored"), ""))

	assertion, loginChallenge, err := as.BeginWebAuthnLogin(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, assertion)
	require.Equal(t, "chal-login", loginChallenge)

	credentials, err := as.GetWebAuthnCredentials(context.Background(), "alice")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(credentials), 1)

	credID := credentials[0].ID
	require.NoError(t, as.UpdateWebAuthnCredentialName(context.Background(), "alice", credID, "new-name"))
	require.NoError(t, as.DeleteWebAuthnCredential(context.Background(), "alice", credID))
}

func TestAuthService_TrustDevice_Success(t *testing.T) {
	t.Parallel()

	accountRepo := newInMemoryAuthAccountRepo()
	accountRepo.devices["dev-1"] = &storage.Device{DeviceID: "dev-1", Username: "alice"}

	sessionRepo := newInMemorySessionRepo()
	sessionRepo.devices["dev-1"] = &Device{DeviceID: "dev-1", Username: "alice", TrustLevel: TrustLevelUntrusted}

	as := &AuthService{
		accountRepo:    accountRepo,
		sessionManager: newSessionManager(sessionRepo),
	}

	require.NoError(t, as.TrustDevice(context.Background(), "alice", "dev-1"))
	require.Equal(t, TrustLevelTrusted, sessionRepo.devices["dev-1"].TrustLevel)
}

func TestAuthService_RefreshAccessToken_FallsBackWhenRotationAndActivityUpdatesFail(t *testing.T) {
	t.Parallel()

	repo := newInMemorySessionRepo()
	repo.sessions["sid-1"] = &Session{
		SessionID:    "sid-1",
		Username:     "alice",
		RefreshToken: "rt-1",
		DeviceID:     "dev-1",
		IPAddress:    "192.0.2.10",
		LastActivity: time.Now(),
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	sessionManager := newSessionManager(&sessionRepoUpdateErr{inMemorySessionRepo: repo, err: errors.New("update failed")})

	as := &AuthService{
		sessionManager: sessionManager,
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{},
	}

	resp, err := as.RefreshAccessToken(context.Background(), "rt-1", "203.0.113.5")
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.Equal(t, "rt-1", resp.RefreshToken)
}
