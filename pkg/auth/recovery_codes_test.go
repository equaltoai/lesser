package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type inMemoryRecoveryCodesRepo struct {
	items           []*storage.RecoveryCodeItem
	deleteCallCount int
}

func (r *inMemoryRecoveryCodesRepo) StoreRecoveryCode(_ context.Context, username string, code *storage.RecoveryCodeItem) error {
	code.Username = username
	r.items = append(r.items, code)
	return nil
}

func (r *inMemoryRecoveryCodesRepo) GetRecoveryCodes(_ context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	var result []*storage.RecoveryCodeItem
	for _, item := range r.items {
		if item.Username == username {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *inMemoryRecoveryCodesRepo) MarkRecoveryCodeUsed(_ context.Context, username, codeHash string) error {
	now := time.Now()
	for _, item := range r.items {
		if item.Username == username && item.CodeHash == codeHash {
			item.UsedAt = &now
			return nil
		}
	}
	return errors.New("recovery code not found")
}

func (r *inMemoryRecoveryCodesRepo) CountUnusedRecoveryCodes(_ context.Context, username string) (int, error) {
	count := 0
	for _, item := range r.items {
		if item.Username != username {
			continue
		}
		if item.UsedAt == nil {
			count++
		}
	}
	return count, nil
}

func (r *inMemoryRecoveryCodesRepo) DeleteAllRecoveryCodes(_ context.Context, username string) error {
	r.deleteCallCount++
	var remaining []*storage.RecoveryCodeItem
	for _, item := range r.items {
		if item.Username != username {
			remaining = append(remaining, item)
		}
	}
	r.items = remaining
	return nil
}

func TestRecoveryCodeService_GenerateValidateAndClear(t *testing.T) {
	t.Parallel()

	repo := &inMemoryRecoveryCodesRepo{}
	svc := &RecoveryCodeService{
		repo:   repo,
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	codes, err := svc.GenerateRecoveryCodes(ctx, "alice", 0)
	require.NoError(t, err)
	require.Len(t, codes, 8)
	require.Equal(t, 1, repo.deleteCallCount)

	unusedCount, err := svc.GetRecoveryCodeCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, 8, unusedCount)

	ok, err := svc.ValidateRecoveryCode(ctx, "alice", codes[0])
	require.NoError(t, err)
	require.True(t, ok)

	unusedCount, err = svc.GetRecoveryCodeCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, 7, unusedCount)

	ok, err = svc.ValidateRecoveryCode(ctx, "alice", codes[0])
	require.NoError(t, err)
	require.False(t, ok)

	ok, err = svc.ValidateRecoveryCode(ctx, "alice", "bad-code")
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, svc.ClearRecoveryCodes(ctx, "alice"))
	require.Equal(t, 2, repo.deleteCallCount)

	unusedCount, err = svc.GetRecoveryCodeCount(ctx, "alice")
	require.NoError(t, err)
	require.Equal(t, 0, unusedCount)
}
