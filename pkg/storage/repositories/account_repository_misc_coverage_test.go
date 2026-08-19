package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	repoerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_NewAccountRepositoryWithCostTracking(t *testing.T) {
	repo := NewAccountRepositoryWithCostTracking(nil, "test-table", "example.com", zaptest.NewLogger(t), (*cost.TrackingService)(nil))
	require.NotNil(t, repo)
}

func TestAccountRepository_IsAccountNotFound(t *testing.T) {
	require.True(t, isAccountNotFound(storage.ErrNotFound))
	require.True(t, isAccountNotFound(dynamormErrors.ErrItemNotFound))
	require.True(t, isAccountNotFound(repoerrors.ItemNotFound("account")))
	require.True(t, isAccountNotFound(common.ActorNotFoundError{Username: "alice"}))
	require.True(t, isAccountNotFound(errors.Join(errors.New("query failed"), dynamormErrors.ErrItemNotFound)))
	require.False(t, isAccountNotFound(dynamormErrors.ErrTableNotFound))
	require.False(t, isAccountNotFound(dynamormErrors.ErrIndexNotFound))
	require.False(t, isAccountNotFound(errors.New("ResourceNotFoundException: Requested resource not found")))
	require.False(t, isAccountNotFound(errors.New("record not found")))
	require.False(t, isAccountNotFound(errors.New("boom")))
}

func TestAccountRepository_StrictAbsenceHelpersWithoutDatabase(t *testing.T) {
	repo := NewAccountRepository(nil, "test-table", "example.com", zaptest.NewLogger(t))
	ctx := context.Background()

	require.Nil(t, repo.usernameLookupCandidates("  "))

	_, err := repo.lookupUserModelByCanonicalHandle(ctx, "alice")
	require.ErrorIs(t, err, storage.ErrNotFound)

	_, err = repo.loadUserCoreProjectionByUsername(ctx, "alice")
	require.ErrorIs(t, err, storage.ErrNotFound)

	_, err = repo.lookupUserCoreProjectionByCanonicalHandle(ctx, "alice")
	require.ErrorIs(t, err, storage.ErrNotFound)
}
