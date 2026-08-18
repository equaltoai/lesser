package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storageinterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type storageProviderStub struct {
	activityRepo storageinterfaces.ActivityRepository
}

func (s *storageProviderStub) Account() *repositories.AccountRepository { return nil }
func (s *storageProviderStub) Actor() storageinterfaces.ActorRepository { return nil }
func (s *storageProviderStub) Activity() storageinterfaces.ActivityRepository {
	return s.activityRepo
}
func (s *storageProviderStub) Notification() storageinterfaces.NotificationRepository { return nil }
func (s *storageProviderStub) Recovery() *repositories.RecoveryRepository             { return nil }
func (s *storageProviderStub) Audit() *repositories.AuditRepository                   { return nil }

func TestNewAuthService_ValidatesJWTSecretAndInitializes(t *testing.T) {
	t.Parallel()

	_, err := NewAuthService(&config.Config{JWTSecret: ""}, nil)
	require.Error(t, err)

	_, err = NewAuthService(&config.Config{JWTSecret: "too-short"}, nil)
	require.Error(t, err)

	repos := testmocks.NewMockRepositoryStorage()
	svc, err := NewAuthService(&config.Config{
		JWTSecret: "a-very-strong-jwt-key-without-weak-patterns-9876543210",
		Domain:    "example.com",
	}, repos)
	require.NoError(t, err)
	require.NotNil(t, svc.sessionManager)
	require.NotNil(t, svc.rateLimiter)
	require.NotNil(t, svc.walletService)
	require.NotNil(t, svc.auditLogger)
	require.NotNil(t, svc.oauthService)
}

func TestAuthService_SessionAndRateLimitDelegates_AndGetStore(t *testing.T) {
	t.Parallel()

	accountRepo := newInMemoryAuthAccountRepo()
	sessionRepo := newInMemorySessionRepo()
	sessionRepo.sessions["s1"] = &Session{SessionID: "s1", Username: "alice"}
	sessionRepo.sessions["s2"] = &Session{SessionID: "s2", Username: "alice"}
	sessionRepo.devices["d1"] = &Device{DeviceID: "d1", Username: "alice"}

	sm := newSessionManager(sessionRepo)

	rlRepo := newFakeRateLimitRepo()
	rlRepo.attempts["account:alice"] = 3
	rateLimiter := &RateLimiter{accountRepo: rlRepo}

	as := &AuthService{
		repos:          testmocks.NewMockRepositoryStorage(),
		accountRepo:    accountRepo,
		sessionManager: sm,
		rateLimiter:    rateLimiter,
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{},
	}

	require.NoError(t, as.Logout(context.Background(), "s1"))
	require.Contains(t, sessionRepo.deleted, "s1")

	require.NoError(t, as.LogoutAllDevices(context.Background(), "alice"))
	require.Contains(t, sessionRepo.deleted, "s2")

	devices, err := as.GetUserDevices(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, devices, 1)

	status, err := as.GetAccountStatus(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, 2, status.RemainingAttempts)

	require.NoError(t, as.ClearAccountLockout(context.Background(), "alice"))
	require.Empty(t, rlRepo.attempts["account:alice"])

	require.NotNil(t, as.GetStore())
}

func TestAuthService_ActivityFallbackBranches(t *testing.T) {
	t.Parallel()

	mockActivity := testmocks.NewMockActivityRepository()
	as := &AuthService{activityRepo: mockActivity}
	require.Same(t, mockActivity, as.activityRepo)

	repos := &storageProviderStub{activityRepo: testmocks.NewMockActivityRepository()}
	as = &AuthService{repos: repos}
	require.Nil(t, as.activityRepo)
	require.NotNil(t, as.repos.Activity())

	as = &AuthService{}
	require.Nil(t, as.activityRepo)
	require.Nil(t, as.repos)
}

func TestAuthService_WalletFlows(t *testing.T) {
	t.Parallel()

	walletRepo := newInMemoryWalletRepo()
	walletSvc := &WalletService{repo: walletRepo, logger: zap.NewNop()}

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	challenge, err := walletSvc.CreateChallenge(context.Background(), address, 1, "alice")
	require.NoError(t, err)

	msgHash := accounts.TextHash([]byte(challenge.Message))
	signatureBytes, err := crypto.Sign(msgHash, privateKey)
	require.NoError(t, err)
	signatureBytes[64] += 27
	signature := hexutil.Encode(signatureBytes)

	accountRepo := newInMemoryAuthAccountRepo()
	accountRepo.users["alice"] = &storage.User{Username: "alice", Approved: true}
	accountRepo.walletCreds["alice"] = []*storage.WalletCredential{{Username: "alice", Address: address}}

	sessionRepo := newInMemorySessionRepo()
	sessionManager := newSessionManager(sessionRepo)

	as := &AuthService{
		accountRepo:    accountRepo,
		sessionManager: sessionManager,
		walletService:  walletSvc,
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{},
	}

	// CreateWalletChallenge delegates.
	_, err = as.CreateWalletChallenge(context.Background(), address, 1, "alice")
	require.NoError(t, err)

	// VerifyWalletSignature success.
	require.NoError(t, as.VerifyWalletSignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: challenge.ID,
		Address:     address,
		Signature:   signature,
		Message:     challenge.Message,
	}))

	resp, err := as.LoginWithWallet(context.Background(), &WalletVerifyRequest{
		ChallengeID: challenge.ID,
		Address:     address,
		Signature:   signature,
		Message:     challenge.Message,
	}, "device", "ua", "192.0.2.10")
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.Equal(t, "alice", resp.Me)

	resp, err = as.LoginWithWalletAfterLinking(context.Background(), "alice", "device", "ua", "192.0.2.10")
	require.NoError(t, err)
	require.Equal(t, "alice", resp.Me)

	created, err := as.LinkWallet(context.Background(), "alice", address, 1, "ethereum")
	require.NoError(t, err)
	require.True(t, created)
	wallets, err := as.GetUserWallets(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, wallets, 1)
	walletRepo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
		{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
	}
	require.NoError(t, as.UnlinkWallet(context.Background(), "alice", address))

	// Challenge helpers.
	require.NoError(t, as.MarkWalletChallengeSpent(context.Background(), "cid"))
	_, err = as.GetWalletChallenge(context.Background(), "cid")
	require.Error(t, err)

	// Coverage for error branches.
	as.accountRepo = nil
	_, err = as.LoginWithWallet(context.Background(), &WalletVerifyRequest{}, "d", "ua", "ip")
	require.Error(t, err)
	require.ErrorIs(t, as.MarkWalletChallengeSpent(context.Background(), "x"), ErrWalletCheck)
	require.ErrorIs(t, as.ResetWalletChallengeSpent(context.Background(), "x"), ErrWalletCheck)
	_, err = as.GetWalletChallenge(context.Background(), "x")
	require.ErrorIs(t, err, ErrWalletCheck)
}

func TestAuthService_FinishWebAuthnLogin_Success(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice"}

	credID := "AA=="
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{
		{ID: credID, UserID: "alice", PublicKey: []byte("pub"), SignCount: 1},
	}
	repo.credentialsByID[credID] = repo.credentialsByUsername["alice"][0]

	engine := &fakeWebAuthnEngine{
		beginLoginChallenge: "chal-login",
		loginCredential: &webauthn.Credential{
			ID: []byte{0},
			Authenticator: webauthn.Authenticator{
				SignCount: 2,
			},
		},
	}

	sessionData := webauthn.SessionData{Challenge: "chal-login", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)}
	sessionBytes, err := json.Marshal(sessionData)
	require.NoError(t, err)
	repo.challengesByChallenge["chal-login"] = &storage.WebAuthnChallenge{
		Challenge:   "chal-login",
		UserID:      "alice",
		SessionData: sessionBytes,
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
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

	sessionRepo := newInMemorySessionRepo()
	sessionManager := newSessionManager(sessionRepo)

	as := &AuthService{
		webAuthnService: webAuthnSvc,
		sessionManager:  sessionManager,
		auditLogger:     &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:       []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:          &config.Config{},
	}

	resp, err := as.FinishWebAuthnLogin(context.Background(), "alice", "chal-login", []byte("ignored"), "device", "ua", "192.0.2.10")
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
	require.Equal(t, credID, resp.CredentialID)

	// Error path when FinishLogin fails.
	_, err = as.FinishWebAuthnLogin(context.Background(), "alice", "missing", []byte("ignored"), "device", "ua", "ip")
	require.Error(t, err)
}
