package auth

import (
	"context"
	"testing"
	"time"

	lessererrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

type inMemoryWalletRepo struct {
	challenges        map[string]*storage.WalletChallenge
	walletsByUserAddr map[string]*storage.WalletCredential
	walletsByAddr     map[string]*storage.WalletCredential
	passkeysByUser    map[string][]*storage.WebAuthnCredential
	deleteConditioned func(context.Context, string, string, string, string, string) error

	errGetUserWallets error
	errStoreWallet    error
	errDeleteWallet   error
	errMarkUsed       error
	errUpdateLastUsed error
}

func newInMemoryWalletRepo() *inMemoryWalletRepo {
	return &inMemoryWalletRepo{
		challenges:        make(map[string]*storage.WalletChallenge),
		walletsByUserAddr: make(map[string]*storage.WalletCredential),
		walletsByAddr:     make(map[string]*storage.WalletCredential),
		passkeysByUser:    make(map[string][]*storage.WebAuthnCredential),
	}
}

func walletKey(username, address string) string { return username + "|" + address }

func (r *inMemoryWalletRepo) StoreWalletChallenge(_ context.Context, challenge *storage.WalletChallenge) error {
	r.challenges[challenge.ID] = challenge
	return nil
}

func (r *inMemoryWalletRepo) GetWalletChallenge(_ context.Context, challengeID string) (*storage.WalletChallenge, error) {
	challenge, ok := r.challenges[challengeID]
	if !ok {
		return nil, lessererrors.ItemNotFoundWithID("wallet_challenge", challengeID)
	}
	return challenge, nil
}

func (r *inMemoryWalletRepo) GetUserWebAuthnCredentials(_ context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	return append([]*storage.WebAuthnCredential(nil), r.passkeysByUser[username]...), nil
}

func (r *inMemoryWalletRepo) DeleteWalletChallenge(_ context.Context, challengeID string) error {
	delete(r.challenges, challengeID)
	return nil
}

func (r *inMemoryWalletRepo) MarkWalletChallengeUsed(_ context.Context, challengeID string) error {
	if r.errMarkUsed != nil {
		return r.errMarkUsed
	}
	challenge, ok := r.challenges[challengeID]
	if !ok {
		return lessererrors.ItemNotFoundWithID("wallet_challenge", challengeID)
	}
	challenge.Used = true
	return nil
}

func (r *inMemoryWalletRepo) GetWalletCredential(_ context.Context, address string) (*storage.WalletCredential, error) {
	cred, ok := r.walletsByAddr[address]
	if !ok {
		return nil, lessererrors.ItemNotFoundWithID("wallet_credential", address)
	}
	return cred, nil
}

func (r *inMemoryWalletRepo) UpdateWalletLastUsed(_ context.Context, username, address string) error {
	if r.errUpdateLastUsed != nil {
		return r.errUpdateLastUsed
	}
	cred, ok := r.walletsByUserAddr[walletKey(username, address)]
	if !ok {
		return lessererrors.ItemNotFoundWithID("wallet_credential", address)
	}
	cred.LastUsed = time.Now()
	return nil
}

func (r *inMemoryWalletRepo) GetUserWalletCredentials(_ context.Context, username string) ([]*storage.WalletCredential, error) {
	if r.errGetUserWallets != nil {
		return nil, r.errGetUserWallets
	}
	var result []*storage.WalletCredential
	for _, cred := range r.walletsByUserAddr {
		if cred.Username == username {
			result = append(result, cred)
		}
	}
	if len(result) == 0 {
		return nil, lessererrors.ItemNotFound("wallet_credential")
	}
	return result, nil
}

func (r *inMemoryWalletRepo) StoreWalletCredential(_ context.Context, credential *storage.WalletCredential) error {
	if r.errStoreWallet != nil {
		return r.errStoreWallet
	}
	r.walletsByUserAddr[walletKey(credential.Username, credential.Address)] = credential
	r.walletsByAddr[credential.Address] = credential
	return nil
}

func (r *inMemoryWalletRepo) DeleteWalletCredential(_ context.Context, username, address string) error {
	if r.errDeleteWallet != nil {
		return r.errDeleteWallet
	}
	delete(r.walletsByUserAddr, walletKey(username, address))
	delete(r.walletsByAddr, address)
	return nil
}

func (r *inMemoryWalletRepo) DeleteWalletCredentialConditionedOnSurvivor(
	_ context.Context,
	username, address, _ string,
	survivingPasskeyID string,
	survivingWalletAddress string,
) error {
	if r.deleteConditioned != nil {
		return r.deleteConditioned(context.Background(), username, address, "", survivingPasskeyID, survivingWalletAddress)
	}
	if survivingPasskeyID != "" {
		found := false
		for _, passkey := range r.passkeysByUser[username] {
			if passkey != nil && passkey.ID == survivingPasskeyID {
				found = true
				break
			}
		}
		if !found {
			return dynamormerrors.ErrConditionFailed
		}
	} else if survivingWalletAddress != "" {
		if _, ok := r.walletsByUserAddr[walletKey(username, survivingWalletAddress)]; !ok {
			return dynamormerrors.ErrConditionFailed
		}
	}
	return r.DeleteWalletCredential(context.Background(), username, address)
}

func TestWalletService_CreateChallenge_VerifySignatureAndLinkFlow(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	_, err := svc.CreateChallenge(context.Background(), "0xabc", 1, "")
	require.Error(t, err)

	// Create a real wallet signature for the challenge message.
	privateKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	challenge, err := svc.CreateChallenge(context.Background(), address, 1, "alice")
	require.NoError(t, err)
	require.NotEmpty(t, challenge.ID)
	require.Contains(t, challenge.Message, "authenticate with Lesser as 'alice'")

	msgHash := accounts.TextHash([]byte(challenge.Message))
	signatureBytes, err := crypto.Sign(msgHash, privateKey)
	require.NoError(t, err)
	signatureBytes[64] += 27 // exercise Ethereum v normalization branch
	signature := hexutil.Encode(signatureBytes)

	repo.errMarkUsed = lessererrors.NewStorageError(lessererrors.CodeConflict, "failed to mark used")
	repo.errUpdateLastUsed = lessererrors.NewStorageError(lessererrors.CodeInternal, "failed to update")

	username, err := svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: challenge.ID,
		Address:     address,
		Signature:   signature,
		Message:     challenge.Message,
	})
	require.NoError(t, err)
	require.Equal(t, "alice", username)

	// LinkWallet handles "not found" by creating the wallet credential.
	created, err := svc.LinkWallet(context.Background(), "alice", address, 1, "ethereum")
	require.NoError(t, err)
	require.True(t, created)

	// Idempotent re-link returns nil.
	created, err = svc.LinkWallet(context.Background(), "alice", address, 1, "ethereum")
	require.NoError(t, err)
	require.False(t, created)

	wallets, err := svc.GetUserWallets(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, wallets, 1)

	repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
		{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
	}
	require.NoError(t, svc.UnlinkWallet(context.Background(), "alice", address))
}

func TestWalletService_VerifySignature_ErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	svc := &WalletService{repo: repo, logger: zap.NewNop()}

	// Expired challenge deletes the challenge and returns ErrChallengeExpired.
	repo.challenges["expired"] = &storage.WalletChallenge{
		ID:        "expired",
		Username:  "alice",
		Address:   "0xabc",
		ChainID:   1,
		Message:   "msg",
		IssuedAt:  time.Now().Add(-10 * time.Minute),
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	_, err := svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: "expired",
		Address:     "0xabc",
		Signature:   "0x00",
		Message:     "msg",
	})
	require.ErrorIs(t, err, ErrChallengeExpired)
	require.Empty(t, repo.challenges)

	// Spent challenge also returns ErrChallengeExpired.
	repo.challenges["spent"] = &storage.WalletChallenge{
		ID:        "spent",
		Username:  "alice",
		Address:   "0xabc",
		ChainID:   1,
		Message:   "msg",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
		Spent:     true,
	}
	_, err = svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: "spent",
		Address:     "0xabc",
		Signature:   "0x00",
		Message:     "msg",
	})
	require.ErrorIs(t, err, ErrChallengeExpired)

	// Message mismatch.
	repo.challenges["mismatch"] = &storage.WalletChallenge{
		ID:        "mismatch",
		Username:  "alice",
		Address:   "0xabc",
		ChainID:   1,
		Message:   "expected",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	}
	_, err = svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: "mismatch",
		Address:     "0xabc",
		Signature:   "0x00",
		Message:     "actual",
	})
	require.ErrorIs(t, err, ErrMessageMismatch)

	// Address mismatch.
	_, err = svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: "mismatch",
		Address:     "0xdef",
		Signature:   "0x00",
		Message:     "expected",
	})
	require.ErrorIs(t, err, ErrAddressMismatch)

	// Signature verification fails (bad signature length).
	repo.challenges["sigfail"] = &storage.WalletChallenge{
		ID:        "sigfail",
		Username:  "alice",
		Address:   common.HexToAddress("0x1").Hex(),
		ChainID:   1,
		Message:   "Sign this message to authenticate with Lesser",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Minute),
	}
	_, err = svc.VerifySignature(context.Background(), &WalletVerifyRequest{
		ChallengeID: "sigfail",
		Address:     common.HexToAddress("0x1").Hex(),
		Signature:   hexutil.Encode([]byte{1, 2, 3}),
		Message:     "Sign this message to authenticate with Lesser",
	})
	require.ErrorIs(t, err, ErrSignatureVerification)
}

func TestWalletHelpers(t *testing.T) {
	t.Parallel()

	nonce, err := generateNonce()
	require.NoError(t, err)
	require.Len(t, nonce, 32)

	msg := buildAuthMessage("example.com", "0xAbC", 1, "nonce", "alice", "2024-01-01T00:00:00Z", "2024-01-01T00:05:00Z")
	require.Contains(t, msg, "example.com wants you to sign in with your Ethereum account")
	require.Contains(t, msg, "authenticate with Lesser as 'alice'")
}
