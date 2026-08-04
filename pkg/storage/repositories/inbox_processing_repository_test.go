package repositories

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestInboxProcessingRepository_TryRecordTargetAndForget(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewInboxProcessingRepository(mockDB, "tbl", zap.NewNop(), nil)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()
	created, err := repo.TryRecordTarget(ctx, "https://remote.example/activities/create-1", "https://example.com/users/alice", "Create")
	require.NoError(t, err)
	require.True(t, created)

	mockQuery.On("IfNotExists").Return(mockQuery).Once()
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed).Once()
	created, err = repo.TryRecordTarget(ctx, "https://remote.example/activities/create-1", "https://example.com/users/alice", "Create")
	require.NoError(t, err)
	require.False(t, created)

	mockQuery.On("Where", "PK", "=", mock.MatchedBy(func(value string) bool {
		return strings.HasPrefix(value, "INBOX_ACTIVITY#")
	})).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", mock.MatchedBy(func(value string) bool {
		return strings.HasPrefix(value, "TARGET#")
	})).Return(mockQuery).Once()
	mockQuery.On("Delete").Return(nil).Once()
	require.NoError(t, repo.ForgetTarget(ctx, "https://remote.example/activities/create-1", "https://example.com/users/alice"))
}

func TestInboxProcessingRepository_TryRecordTargetValidationError(t *testing.T) {
	repo := NewInboxProcessingRepository(new(mocks.MockDB), "tbl", zap.NewNop(), nil)
	created, err := repo.TryRecordTarget(context.Background(), "", "https://example.com/users/alice", "Create")
	require.Error(t, err)
	require.False(t, created)
}
