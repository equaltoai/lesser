package auth

import (
	"context"
	"errors"
	"testing"

	lessererrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

func TestWebAuthnService_DeleteCredential_ClassifiesGuardedRemovalFailuresFromPostState(t *testing.T) {
	t.Parallel()

	for _, txFailure := range []struct {
		name string
		err  error
	}{
		{
			name: "condition_failed",
			err:  guardedRemovalTransactionError(dynamormerrors.ErrConditionFailed, "delete", 0, "ConditionalCheckFailed"),
		},
		{
			name: "transaction_conflict",
			err:  guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
		},
	} {
		txFailure := txFailure
		t.Run(txFailure.name, func(t *testing.T) {
			t.Parallel()

			t.Run("target already removed returns credential not found", func(t *testing.T) {
				t.Parallel()

				repo := newInMemoryWebAuthnRepo()
				target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk1")}
				survivor := &storage.WebAuthnCredential{ID: "cred-2", UserID: "alice", PublicKey: []byte("pk2")}
				repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{target, survivor}
				repo.credentialsByID[target.ID] = target
				repo.credentialsByID[survivor.ID] = survivor
				repo.walletsByUsername["alice"] = []*storage.WalletCredential{
					{Username: "alice", Address: "0xabc", Type: "ethereum"},
				}
				repo.deleteConditionedFunc = func(context.Context, string, string, string, string) error {
					require.NoError(t, repo.DeleteWebAuthnCredential(context.Background(), target.ID))
					return txFailure.err
				}

				svc := &WebAuthnService{repo: repo}
				err := svc.DeleteCredential(context.Background(), "alice", target.ID)
				require.ErrorIs(t, err, ErrCredentialNotFound)
				require.NotErrorIs(t, err, ErrLastAuthMethodDelete)
			})

			t.Run("target still present and no survivor returns invariant rejection", func(t *testing.T) {
				t.Parallel()

				repo := newInMemoryWebAuthnRepo()
				target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
				survivorWallet := &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum"}
				repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{target}
				repo.credentialsByID[target.ID] = target
				repo.walletsByUsername["alice"] = []*storage.WalletCredential{survivorWallet}
				repo.deleteConditionedFunc = func(context.Context, string, string, string, string) error {
					repo.walletsByUsername["alice"] = []*storage.WalletCredential{}
					return txFailure.err
				}

				svc := &WebAuthnService{repo: repo}
				err := svc.DeleteCredential(context.Background(), "alice", target.ID)
				require.ErrorIs(t, err, ErrLastAuthMethodDelete)
				require.NotErrorIs(t, err, ErrCredentialNotFound)
			})

			t.Run("target still present and survivor remains returns original failure", func(t *testing.T) {
				t.Parallel()

				repo := newInMemoryWebAuthnRepo()
				target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk1")}
				survivor := &storage.WebAuthnCredential{ID: "cred-2", UserID: "alice", PublicKey: []byte("pk2")}
				repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{target, survivor}
				repo.credentialsByID[target.ID] = target
				repo.credentialsByID[survivor.ID] = survivor
				repo.deleteConditionedFunc = func(context.Context, string, string, string, string) error {
					return txFailure.err
				}

				svc := &WebAuthnService{repo: repo}
				err := svc.DeleteCredential(context.Background(), "alice", target.ID)
				require.ErrorIs(t, err, txFailure.err)
				require.NotErrorIs(t, err, ErrCredentialNotFound)
				require.NotErrorIs(t, err, ErrLastAuthMethodDelete)
			})
		})
	}
}

func TestWalletService_UnlinkWallet_ClassifiesGuardedRemovalFailuresFromPostState(t *testing.T) {
	t.Parallel()

	for _, txFailure := range []struct {
		name string
		err  error
	}{
		{
			name: "condition_failed",
			err:  guardedRemovalTransactionError(dynamormerrors.ErrConditionFailed, "delete", 0, "ConditionalCheckFailed"),
		},
		{
			name: "transaction_conflict",
			err:  guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
		},
	} {
		txFailure := txFailure
		t.Run(txFailure.name, func(t *testing.T) {
			t.Parallel()

			t.Run("target already removed returns not found", func(t *testing.T) {
				t.Parallel()

				repo := newInMemoryWalletRepo()
				repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
					{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
				}
				repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
				repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]
				repo.deleteConditioned = func(context.Context, string, string, string, string, string) error {
					require.NoError(t, repo.DeleteWalletCredential(context.Background(), "alice", "0xabc"))
					return txFailure.err
				}

				svc := &WalletService{repo: repo, logger: zap.NewNop()}
				err := svc.UnlinkWallet(context.Background(), "alice", "0xabc")
				require.True(t, isAuthenticatorRemovalTargetNotFound(err))
				require.NotErrorIs(t, err, ErrLastAuthMethodDelete)
			})

			t.Run("target still present and no survivor returns invariant rejection", func(t *testing.T) {
				t.Parallel()

				repo := newInMemoryWalletRepo()
				repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
					{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
				}
				repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
				repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]
				repo.deleteConditioned = func(context.Context, string, string, string, string, string) error {
					repo.passkeysByUser["alice"] = nil
					return txFailure.err
				}

				svc := &WalletService{repo: repo, logger: zap.NewNop()}
				err := svc.UnlinkWallet(context.Background(), "alice", "0xabc")
				require.ErrorIs(t, err, ErrLastAuthMethodDelete)
			})

			t.Run("target still present and survivor remains returns delete failure", func(t *testing.T) {
				t.Parallel()

				repo := newInMemoryWalletRepo()
				repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
					{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
				}
				repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
				repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]
				repo.deleteConditioned = func(context.Context, string, string, string, string, string) error {
					return txFailure.err
				}

				svc := &WalletService{repo: repo, logger: zap.NewNop()}
				err := svc.UnlinkWallet(context.Background(), "alice", "0xabc")
				require.ErrorIs(t, err, ErrWalletDeletion)
				require.ErrorIs(t, err, txFailure.err)
				require.NotErrorIs(t, err, ErrLastAuthMethodDelete)
				require.False(t, isAuthenticatorRemovalTargetNotFound(err))
			})
		})
	}
}

func TestWebAuthnService_DeleteCredential_RereadFailureReturns500(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{target}
	repo.credentialsByID[target.ID] = target
	repo.deleteConditionedFunc = func(context.Context, string, string, string, string) error {
		return guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict")
	}

	lookupErr := errors.New("lookup failed")
	svc := &WebAuthnService{
		repo: &webAuthnRepoGetCredentialErr{
			inMemoryWebAuthnRepo: repo,
			err:                  lookupErr,
		},
	}
	err := svc.DeleteCredential(context.Background(), "alice", target.ID)
	require.ErrorIs(t, err, lookupErr)
}

func TestClassifyGuardedWebAuthnRemovalFailure_DirectBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	require.NoError(t, classifyGuardedWebAuthnRemovalFailure(context.Background(), repo, "alice", "cred-1", nil))

	target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "bob", PublicKey: []byte("pk")}
	repo.credentialsByID[target.ID] = target
	require.ErrorIs(t, classifyGuardedWebAuthnRemovalFailure(
		context.Background(),
		repo,
		"alice",
		target.ID,
		guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
	), ErrCredentialNotFound)

	repo.credentialsByID[target.ID] = &storage.WebAuthnCredential{ID: target.ID, UserID: "alice", PublicKey: []byte("pk")}
	inventoryErr := errors.New("wallet inventory failed")
	require.ErrorIs(t, classifyGuardedWebAuthnRemovalFailure(
		context.Background(),
		&webAuthnRepoGetWalletErr{inMemoryWebAuthnRepo: repo, err: inventoryErr},
		"alice",
		target.ID,
		guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
	), inventoryErr)
}

type walletRepoGetCredentialErr struct {
	*inMemoryWalletRepo
	err error
}

func (r *walletRepoGetCredentialErr) GetWalletCredential(_ context.Context, _ string) (*storage.WalletCredential, error) {
	return nil, r.err
}

type walletRepoGetPasskeysErr struct {
	*inMemoryWalletRepo
	err error
}

func (r *walletRepoGetPasskeysErr) GetUserWebAuthnCredentials(_ context.Context, _ string) ([]*storage.WebAuthnCredential, error) {
	return nil, r.err
}

func TestClassifyGuardedWalletRemovalFailure_DirectBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	require.NoError(t, classifyGuardedWalletRemovalFailure(context.Background(), repo, "alice", "0xabc", nil))

	lookupErr := errors.New("wallet lookup failed")
	require.ErrorIs(t, classifyGuardedWalletRemovalFailure(
		context.Background(),
		&walletRepoGetCredentialErr{inMemoryWalletRepo: repo, err: lookupErr},
		"alice",
		"0xabc",
		guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
	), lookupErr)

	repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "bob", Address: "0xabc", Type: "ethereum", ChainID: 1}
	repo.walletsByUserAddr[walletKey("bob", "0xabc")] = repo.walletsByAddr["0xabc"]
	require.True(t, isAuthenticatorRemovalTargetNotFound(classifyGuardedWalletRemovalFailure(
		context.Background(),
		repo,
		"alice",
		"0xabc",
		guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
	)))

	repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
	repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]
	passkeyErr := errors.New("passkey inventory failed")
	require.ErrorIs(t, classifyGuardedWalletRemovalFailure(
		context.Background(),
		&walletRepoGetPasskeysErr{inMemoryWalletRepo: repo, err: passkeyErr},
		"alice",
		"0xabc",
		guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict"),
	), passkeyErr)
}

func TestIsAuthenticatorRemovalTargetNotFound(t *testing.T) {
	t.Parallel()

	require.True(t, isAuthenticatorRemovalTargetNotFound(lessererrors.ItemNotFoundWithID("credential", "cred-1")))
	require.True(t, isAuthenticatorRemovalTargetNotFound(lessererrors.CredentialNotFound()))
	require.True(t, isAuthenticatorRemovalTargetNotFound(dynamormerrors.ErrItemNotFound))
	require.False(t, isAuthenticatorRemovalTargetNotFound(errors.New("boom")))
}

func guardedRemovalTransactionError(err error, operation string, operationIndex int, reason string) error {
	return &dynamormerrors.TransactionError{
		Err:            err,
		Operation:      operation,
		OperationIndex: operationIndex,
		Reason:         reason,
	}
}
