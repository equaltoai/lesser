package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type authAccountRepoWalletCredsErr struct {
	*inMemoryAuthAccountRepo
	err error
}

func (r *authAccountRepoWalletCredsErr) GetUserWalletCredentials(_ context.Context, _ string) ([]*storage.WalletCredential, error) {
	return nil, r.err
}

type sessionRepoCreateErr struct {
	*inMemorySessionRepo
	err error
}

func (r *sessionRepoCreateErr) CreateSession(_ context.Context, _, _, _ string) (*Session, error) {
	return nil, r.err
}

func newSignedWalletVerifyRequest(t *testing.T, walletSvc *WalletService, username string) (*WalletVerifyRequest, string) {
	t.Helper()

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	challenge, err := walletSvc.CreateChallenge(context.Background(), address, 1, username)
	require.NoError(t, err)

	msgHash := accounts.TextHash([]byte(challenge.Message))
	signatureBytes, err := crypto.Sign(msgHash, privateKey)
	require.NoError(t, err)
	signatureBytes[64] += 27 // normalize V

	return &WalletVerifyRequest{
		ChallengeID: challenge.ID,
		Address:     address,
		Signature:   hexutil.Encode(signatureBytes),
		Message:     challenge.Message,
	}, address
}

func TestAuthService_VerifyWalletSignature_ReturnsWrappedError(t *testing.T) {
	t.Parallel()

	walletRepo := newInMemoryWalletRepo()
	walletSvc := &WalletService{repo: walletRepo, logger: zap.NewNop()}
	as := &AuthService{walletService: walletSvc}

	err := as.VerifyWalletSignature(context.Background(), &WalletVerifyRequest{ChallengeID: "missing"})
	require.ErrorIs(t, err, ErrSignatureVerificationFailed)
}

func TestAuthService_LoginWithWallet_ErrorBranches(t *testing.T) {
	t.Parallel()

	walletRepo := newInMemoryWalletRepo()
	walletSvc := &WalletService{repo: walletRepo, logger: zap.NewNop()}
	req, address := newSignedWalletVerifyRequest(t, walletSvc, "alice")

	accountRepo := newInMemoryAuthAccountRepo()
	accountRepo.users["alice"] = &storage.User{Username: "alice", Approved: true}

	as := &AuthService{
		accountRepo:    accountRepo,
		walletService:  walletSvc,
		sessionManager: newSessionManager(newInMemorySessionRepo()),
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{},
	}

	// Wallet not linked to this username.
	accountRepo.walletCreds["alice"] = []*storage.WalletCredential{{Username: "alice", Address: "0xdeadbeef"}}
	_, err := as.LoginWithWallet(context.Background(), req, "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrWalletCheck)

	// Suspended user.
	accountRepo.walletCreds["alice"] = []*storage.WalletCredential{{Username: "alice", Address: address}}
	accountRepo.users["alice"].Suspended = true
	_, err = as.LoginWithWallet(context.Background(), req, "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserSuspended)

	// Not approved user.
	accountRepo.users["alice"].Suspended = false
	accountRepo.users["alice"].Approved = false
	_, err = as.LoginWithWallet(context.Background(), req, "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserNotApproved)

	// User retrieval error.
	accountRepo.users["alice"].Approved = true
	accountRepo.errGetUser = errors.New("db down")
	_, err = as.LoginWithWallet(context.Background(), req, "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserRetrievalFailed)
	accountRepo.errGetUser = nil

	// Wallet credential retrieval error.
	as.accountRepo = &authAccountRepoWalletCredsErr{inMemoryAuthAccountRepo: accountRepo, err: errors.New("wallet cred read failed")}
	_, err = as.LoginWithWallet(context.Background(), req, "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrWalletCheck)

	// Signature verification failure.
	as.accountRepo = accountRepo
	_, err = as.LoginWithWallet(context.Background(), &WalletVerifyRequest{ChallengeID: "missing"}, "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrSignatureVerificationFailed)
}

func TestAuthService_LoginWithWalletAfterLinking_ErrorBranches(t *testing.T) {
	t.Parallel()

	accountRepo := newInMemoryAuthAccountRepo()
	accountRepo.users["alice"] = &storage.User{Username: "alice", Approved: true}

	as := &AuthService{
		accountRepo:    accountRepo,
		sessionManager: newSessionManager(newInMemorySessionRepo()),
		auditLogger:    &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: false}},
		jwtSecret:      []byte("a-very-strong-jwt-key-without-weak-patterns-9876543210"),
		config:         &config.Config{},
	}

	// Missing repo.
	as.accountRepo = nil
	_, err := as.LoginWithWalletAfterLinking(context.Background(), "alice", "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserRetrievalFailed)

	as.accountRepo = accountRepo

	// User retrieval error.
	accountRepo.errGetUser = errors.New("db down")
	_, err = as.LoginWithWalletAfterLinking(context.Background(), "alice", "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserRetrievalFailed)
	accountRepo.errGetUser = nil

	// Suspended / not approved.
	accountRepo.users["alice"].Suspended = true
	_, err = as.LoginWithWalletAfterLinking(context.Background(), "alice", "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserSuspended)
	accountRepo.users["alice"].Suspended = false
	accountRepo.users["alice"].Approved = false
	_, err = as.LoginWithWalletAfterLinking(context.Background(), "alice", "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrUserNotApproved)
	accountRepo.users["alice"].Approved = true

	// Session creation failure.
	repo := newInMemorySessionRepo()
	repo.sessions["seed"] = &Session{SessionID: "seed", Username: "alice", ExpiresAt: time.Now().Add(time.Hour)}
	as.sessionManager = newSessionManager(&sessionRepoCreateErr{inMemorySessionRepo: repo, err: errors.New("create failed")})
	_, err = as.LoginWithWalletAfterLinking(context.Background(), "alice", "device", "ua", "203.0.113.5")
	require.ErrorIs(t, err, ErrSessionCreationFailed)
}
