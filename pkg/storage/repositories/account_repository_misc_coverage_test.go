package repositories

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap/zaptest"
)

func TestAccountRepository_NewAccountRepositoryWithCostTracking(t *testing.T) {
	repo := NewAccountRepositoryWithCostTracking(nil, "test-table", "example.com", zaptest.NewLogger(t), (*cost.TrackingService)(nil))
	require.NotNil(t, repo)
}

func TestAccountRepository_IsAccountNotFound(t *testing.T) {
	require.True(t, isAccountNotFound(dynamormErrors.ErrItemNotFound))
	require.True(t, isAccountNotFound(errors.New("record not found")))
	require.False(t, isAccountNotFound(errors.New("boom")))
}
