package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recoveryCodesRepoDeleteErr struct {
	*inMemoryRecoveryCodesRepo
	err error
}

func (r *recoveryCodesRepoDeleteErr) DeleteAllRecoveryCodes(_ context.Context, _ string) error {
	return r.err
}

type recoveryCodesRepoStoreErr struct {
	*inMemoryRecoveryCodesRepo
	err error
}

func (r *recoveryCodesRepoStoreErr) StoreRecoveryCode(_ context.Context, _ string, _ *storage.RecoveryCodeItem) error {
	return r.err
}

type recoveryCodesRepoGetErr struct {
	*inMemoryRecoveryCodesRepo
	err error
}

func (r *recoveryCodesRepoGetErr) GetRecoveryCodes(_ context.Context, _ string) ([]*storage.RecoveryCodeItem, error) {
	return nil, r.err
}

type recoveryCodesRepoMarkErr struct {
	*inMemoryRecoveryCodesRepo
	err error
}

func (r *recoveryCodesRepoMarkErr) MarkRecoveryCodeUsed(_ context.Context, _ string, _ string) error {
	return r.err
}

func TestRecoveryCodeService_ErrorBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// clearExistingCodes error.
	svc := &RecoveryCodeService{
		repo:   &recoveryCodesRepoDeleteErr{inMemoryRecoveryCodesRepo: &inMemoryRecoveryCodesRepo{}, err: errors.New("delete failed")},
		logger: zap.NewNop(),
	}
	_, err := svc.GenerateRecoveryCodes(ctx, "alice", 1)
	require.ErrorIs(t, err, ErrRecoveryCodeClear)

	// StoreRecoveryCode error.
	baseRepo := &inMemoryRecoveryCodesRepo{}
	svc.repo = &recoveryCodesRepoStoreErr{inMemoryRecoveryCodesRepo: baseRepo, err: errors.New("store failed")}
	_, err = svc.GenerateRecoveryCodes(ctx, "alice", 1)
	require.ErrorIs(t, err, ErrRecoveryCodeStorage)

	// GetRecoveryCodes error.
	svc.repo = &recoveryCodesRepoGetErr{inMemoryRecoveryCodesRepo: baseRepo, err: errors.New("db down")}
	_, err = svc.ValidateRecoveryCode(ctx, "alice", "code")
	require.ErrorIs(t, err, ErrRecoveryCodeRetrieval)

	// MarkRecoveryCodeUsed error.
	svc.repo = baseRepo
	codes, err := svc.GenerateRecoveryCodes(ctx, "alice", 1)
	require.NoError(t, err)
	svc.repo = &recoveryCodesRepoMarkErr{inMemoryRecoveryCodesRepo: baseRepo, err: errors.New("mark failed")}
	ok, err := svc.ValidateRecoveryCode(ctx, "alice", codes[0])
	require.False(t, ok)
	require.ErrorIs(t, err, ErrRecoveryCodeMarkUsed)
}
