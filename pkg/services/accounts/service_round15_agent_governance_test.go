package accounts

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type agentGovernanceServiceStorage struct {
	*MockRepositoryStorage
	accountRepo *repositories.AccountRepository
}

func (s *agentGovernanceServiceStorage) Account() *repositories.AccountRepository {
	return s.accountRepo
}

func TestServiceAgentGovernanceHelpersRequireStorage(t *testing.T) {
	svc := NewService(nil, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	state, err := svc.GetAgentGovernanceState(context.Background(), "agent")
	require.Nil(t, state)
	require.ErrorIs(t, err, ErrStorageNotAvailable)

	states, err := svc.GetAgentGovernanceStatesByUsernames(context.Background(), []string{"agent"})
	require.Nil(t, states)
	require.ErrorIs(t, err, ErrStorageNotAvailable)

	err = svc.PutAgentGovernanceState(context.Background(), &storage.AgentGovernanceState{Username: "agent"})
	require.ErrorIs(t, err, ErrStorageNotAvailable)
}

func TestServiceAgentGovernanceHelpersRequireAccountRepository(t *testing.T) {
	svc := NewService(NewMockRepositoryStorage(), streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	state, err := svc.GetAgentGovernanceState(context.Background(), "agent")
	require.Nil(t, state)
	require.ErrorIs(t, err, ErrAccountRepositoryNotAvailable)

	states, err := svc.GetAgentGovernanceStatesByUsernames(context.Background(), []string{"agent"})
	require.Nil(t, states)
	require.ErrorIs(t, err, ErrAccountRepositoryNotAvailable)

	err = svc.PutAgentGovernanceState(context.Background(), &storage.AgentGovernanceState{Username: "agent"})
	require.ErrorIs(t, err, ErrAccountRepositoryNotAvailable)
}

func TestServiceAgentGovernanceHelpersDelegateToRepository(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Times(3)
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(loadQuery).Twice()
	loadQuery.On("Where", "PK", "=", "USER#agent").Return(loadQuery).Twice()
	loadQuery.On("Where", "SK", "=", models.SKAgentGovernance).Return(loadQuery).Twice()
	loadQuery.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.AgentGovernanceState)
		*record = models.AgentGovernanceState{
			Username:        "agent",
			PK:              "USER#agent",
			SK:              models.SKAgentGovernance,
			DelegatedScopes: []string{"read"},
			CreatedAt:       now.Add(-time.Hour),
			UpdatedAt:       now,
		}
	}).Return(nil).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Return(dynamormerrors.ErrItemNotFound).Once()

	loadQuery.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Remove", mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("ConditionNotExists", "Version").Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	accountRepo := repositories.NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	storageImpl := &agentGovernanceServiceStorage{
		MockRepositoryStorage: NewMockRepositoryStorage(),
		accountRepo:           accountRepo,
	}
	svc := NewService(storageImpl, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	state, err := svc.GetAgentGovernanceState(ctx, "agent")
	require.NoError(t, err)
	require.Equal(t, []string{"read"}, state.DelegatedScopes)

	err = svc.PutAgentGovernanceState(ctx, &storage.AgentGovernanceState{
		Username:        "agent",
		DelegatedScopes: []string{"write", "read"},
	})
	require.NoError(t, err)
}
