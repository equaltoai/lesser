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

func TestWebAuthnService_DeleteCredential_ClassifiesTransactionConflictAsInvariantRejection(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{target}
	repo.credentialsByID[target.ID] = target
	repo.walletsByUsername["alice"] = []*storage.WalletCredential{
		{Username: "alice", Address: "0xabc", Type: "ethereum"},
	}
	repo.deleteConditionedFunc = func(context.Context, string, string, string, string) error {
		return guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict")
	}

	svc := &WebAuthnService{repo: repo}
	err := svc.DeleteCredential(context.Background(), "alice", target.ID)
	require.ErrorIs(t, err, ErrLastAuthMethodDelete)
}

func TestWalletService_UnlinkWallet_ClassifiesTransactionConflictAsInvariantRejection(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
		{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
	}
	repo.walletsByAddr["0xabc"] = &storage.WalletCredential{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}
	repo.walletsByUserAddr[walletKey("alice", "0xabc")] = repo.walletsByAddr["0xabc"]
	repo.deleteConditioned = func(context.Context, string, string, string, string, string) error {
		return guardedRemovalTransactionError(dynamormerrors.ErrTransactionConflict, "delete", 0, "TransactionConflict")
	}

	svc := &WalletService{repo: repo, logger: zap.NewNop()}
	err := svc.UnlinkWallet(context.Background(), "alice", "0xabc")
	require.ErrorIs(t, err, ErrLastAuthMethodDelete)
}

func TestWebAuthnService_DeleteCredential_ClassifiesConditionFailuresByRereadingTarget(t *testing.T) {
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
			return guardedRemovalTransactionError(dynamormerrors.ErrConditionFailed, "delete", 0, "ConditionalCheckFailed")
		}

		svc := &WebAuthnService{repo: repo}
		err := svc.DeleteCredential(context.Background(), "alice", target.ID)
		require.ErrorIs(t, err, ErrCredentialNotFound)
		require.NotErrorIs(t, err, ErrLastAuthMethodDelete)
	})

	t.Run("target still present keeps invariant rejection", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		target := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
		repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{target}
		repo.credentialsByID[target.ID] = target
		repo.walletsByUsername["alice"] = []*storage.WalletCredential{
			{Username: "alice", Address: "0xabc", Type: "ethereum"},
		}
		repo.deleteConditionedFunc = func(context.Context, string, string, string, string) error {
			return guardedRemovalTransactionError(dynamormerrors.ErrConditionFailed, "condition_check", 1, "ConditionalCheckFailed")
		}

		svc := &WebAuthnService{repo: repo}
		err := svc.DeleteCredential(context.Background(), "alice", target.ID)
		require.ErrorIs(t, err, ErrLastAuthMethodDelete)
		require.NotErrorIs(t, err, ErrCredentialNotFound)
	})
}

func TestClassifyGuardedAuthenticatorRemovalFailure(t *testing.T) {
	t.Parallel()

	require.NoError(t, classifyGuardedAuthenticatorRemovalFailure(nil))

	plainErr := errors.New("boom")
	require.Same(t, plainErr, classifyGuardedAuthenticatorRemovalFailure(plainErr))
}

func TestClassifyGuardedWebAuthnRemovalFailure_Passthroughs(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	require.NoError(t, classifyGuardedWebAuthnRemovalFailure(context.Background(), repo, "alice", "cred-1", nil))

	plainErr := errors.New("boom")
	require.Same(t, plainErr, classifyGuardedWebAuthnRemovalFailure(context.Background(), repo, "alice", "cred-1", plainErr))

	lookupErr := errors.New("lookup failed")
	repo.deleteConditionedFunc = nil
	repo.credentialsByID = map[string]*storage.WebAuthnCredential{}
	require.Same(t, lookupErr, classifyGuardedWebAuthnRemovalFailure(
		context.Background(),
		&webAuthnRepoGetCredentialErr{inMemoryWebAuthnRepo: repo, err: lookupErr},
		"alice",
		"cred-1",
		guardedRemovalTransactionError(dynamormerrors.ErrConditionFailed, "condition_check", 1, "ConditionalCheckFailed"),
	))
}

func TestIsAuthenticatorRemovalTargetNotFound(t *testing.T) {
	t.Parallel()

	require.True(t, isAuthenticatorRemovalTargetNotFound(lessererrors.ItemNotFoundWithID("credential", "cred-1")))
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
