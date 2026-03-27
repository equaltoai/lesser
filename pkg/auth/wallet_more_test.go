package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type walletRepoNilWallets struct {
	*inMemoryWalletRepo
}

func (r *walletRepoNilWallets) GetUserWalletCredentials(_ context.Context, _ string) ([]*storage.WalletCredential, error) {
	return nil, nil
}

type walletRepoStoreChallengeErr struct {
	*inMemoryWalletRepo
	err error
}

func (r *walletRepoStoreChallengeErr) StoreWalletChallenge(_ context.Context, _ *storage.WalletChallenge) error {
	return r.err
}

func TestNewWalletService_WithNilRepos_DoesNotPanic(t *testing.T) {
	t.Parallel()

	svc := NewWalletService(nil)
	require.NotNil(t, svc)
	require.Nil(t, svc.repo)
}

func TestWalletService_VerifySignature_UsesExistingWalletLinkWhenChallengeUsernameMissing(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	now := time.Now()
	message := buildAuthMessage("example.com", address, 1, "nonce", "", now.Format(time.RFC3339), now.Add(5*time.Minute).Format(time.RFC3339))
	msgHash := accounts.TextHash([]byte(message))
	signatureBytes, err := crypto.Sign(msgHash, privateKey)
	require.NoError(t, err)
	signatureBytes[64] += 27

	challenge := &storage.WalletChallenge{
		ID:        "cid-1",
		Username:  "",
		Address:   address,
		ChainID:   1,
		Nonce:     "nonce",
		Message:   message,
		IssuedAt:  now,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	require.NoError(t, repo.StoreWalletChallenge(context.Background(), challenge))

	// Existing wallet link is used when the challenge doesn't bind a username.
	addrLower := strings.ToLower(address)
	repo.walletsByAddr[addrLower] = &storage.WalletCredential{Username: "alice", Address: addrLower, ChainID: 1}
	repo.walletsByUserAddr[walletKey("alice", addrLower)] = repo.walletsByAddr[addrLower]

	username, err := svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: challenge.ID,
		Address:     address,
		Signature:   hexutil.Encode(signatureBytes),
		Message:     message,
	})
	require.NoError(t, err)
	require.Equal(t, "alice", username)
}

func TestWalletService_GetUserWallets_NilWalletSlice_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	repo := &walletRepoNilWallets{inMemoryWalletRepo: newInMemoryWalletRepo()}
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	wallets, err := svc.GetUserWallets(context.Background(), "alice")
	require.NoError(t, err)
	require.Empty(t, wallets)
}

func TestWalletService_LinkAndUnlink_ErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	repo.errGetUserWallets = errors.New("db down")
	_, err := svc.LinkWallet(context.Background(), "alice", "0xabc", 1, "ethereum")
	require.ErrorIs(t, err, ErrWalletCheck)

	repo.errGetUserWallets = nil
	repo.errDeleteWallet = errors.New("delete failed")
	require.ErrorIs(t, svc.UnlinkWallet(context.Background(), "alice", "0xabc"), ErrWalletDeletion)
}

func TestWalletService_CreateChallenge_AndGetUserWallets_ErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	repo.errGetUserWallets = errors.New("db down")
	_, err := svc.GetUserWallets(context.Background(), "alice")
	require.ErrorIs(t, err, ErrWalletRetrieval)

	// Store challenge failure.
	svc.repo = &walletRepoStoreChallengeErr{inMemoryWalletRepo: repo, err: errors.New("store failed")}
	_, err = svc.CreateChallenge(context.Background(), "0xabc", 1, "alice")
	require.ErrorIs(t, err, ErrChallengeStorage)
}

func TestWalletService_LinkWallet_StoreError_AndVerifySignatureHelpers(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	// Store wallet credential error.
	repo.errStoreWallet = errors.New("write failed")
	_, err := svc.LinkWallet(context.Background(), "alice", "0xabc", 1, "ethereum")
	require.ErrorIs(t, err, ErrWalletStorage)

	// verifyEthereumSignature invalid signature format.
	require.ErrorIs(t, svc.verifyEthereumSignature("0xabc", "msg", "bad"), ErrInvalidSignatureFormat)

	// Public key recovery error (65 bytes, but invalid signature).
	require.ErrorIs(t, svc.verifyEthereumSignature("0xabc", "msg", hexutil.Encode(make([]byte, 65))), ErrPublicKeyRecovery)

	// SIWE parse warning branch (valid signature + contains SIWE marker string).
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()
	message := "Sign this message to authenticate with Lesser\nnot-siwe"
	msgHash := accounts.TextHash([]byte(message))
	signatureBytes, err := crypto.Sign(msgHash, privateKey)
	require.NoError(t, err)
	signatureBytes[64] += 27
	signature := hexutil.Encode(signatureBytes)

	require.NoError(t, svc.verifyEthereumSignature(address, message, signature))
	require.ErrorIs(t, svc.verifyEthereumSignature("0x0000000000000000000000000000000000000000", message, signature), ErrSignatureAddressMismatch)
}
