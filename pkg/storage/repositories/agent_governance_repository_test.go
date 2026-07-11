package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestAgentGovernanceRepository_GetAgentGovernanceState_Success(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	verifiedAt := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(query).Once()
	query.On("Where", "PK", "=", "USER#agent").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.AgentGovernanceState)
		*record = models.AgentGovernanceState{
			PK:               "USER#agent",
			SK:               models.SKAgentGovernance,
			Username:         "agent",
			DelegatedScopes:  []string{"read", "write"},
			Verified:         true,
			VerifiedAt:       &verifiedAt,
			CreatedAt:        verifiedAt.Add(-time.Hour),
			UpdatedAt:        verifiedAt,
			Version:          3,
			QuarantineStatus: storage.AgentQuarantineStatusApproved,
		}
	}).Return(nil).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	state, err := repo.GetAgentGovernanceState(ctx, "Agent")
	require.NoError(t, err)
	require.Equal(t, "agent", state.Username)
	require.Equal(t, []string{"read", "write"}, state.DelegatedScopes)
	require.True(t, state.Verified)
	require.NotNil(t, state.VerifiedAt)
	require.Equal(t, verifiedAt, *state.VerifiedAt)
	require.Equal(t, 3, state.Version)
}

func TestAgentGovernanceRepository_GetAgentGovernanceState_NotFound(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(query).Once()
	query.On("Where", "PK", "=", "USER#agent").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	state, err := repo.GetAgentGovernanceState(ctx, "agent")
	require.Nil(t, state)
	require.ErrorIs(t, err, storage.ErrNotFound)
}

func TestAgentGovernanceRepository_PutAgentGovernanceState_CreatesNormalizedRow(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(record *models.AgentGovernanceState) bool {
		return record != nil &&
			record.Username == "agent" &&
			record.PK == "USER#agent" &&
			record.SK == models.SKAgentGovernance &&
			record.QuarantineStatus == storage.AgentQuarantineStatusQuarantined &&
			record.SelfSovereign &&
			record.Verified &&
			len(record.DelegatedScopes) == 2 &&
			record.DelegatedScopes[0] == "read" &&
			record.DelegatedScopes[1] == "write" &&
			len(record.SelfScopes) == 2 &&
			record.SelfScopes[0] == "follow" &&
			record.SelfScopes[1] == "write" &&
			record.Version == 0
	})).Return(query).Once()
	query.On("Where", "PK", "=", "USER#agent").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Remove", mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("ConditionNotExists", "Version").Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	state := &storage.AgentGovernanceState{
		Username:         " Agent ",
		QuarantineStatus: storage.AgentQuarantineStatusQuarantined,
		DelegatedScopes:  []string{"write", "read", "read", ""},
		SelfScopes:       []string{"follow", "write", "write"},
		SelfSovereign:    true,
		Verified:         true,
	}
	err := repo.PutAgentGovernanceState(ctx, state)
	require.NoError(t, err)
	require.Equal(t, "agent", state.Username)
	require.Equal(t, 1, state.Version)
}

func TestAgentGovernanceRepository_PutAgentGovernanceState_UsesOptimisticLockingOnUpdate(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)
	existingCreatedAt := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)

	mockDB.On("Model", mock.MatchedBy(func(record *models.AgentGovernanceState) bool {
		return record != nil &&
			record.Username == "agent" &&
			record.CreatedAt.Equal(existingCreatedAt) &&
			record.UpdatedAt.Equal(now) &&
			len(record.DelegatedScopes) == 2 &&
			record.DelegatedScopes[0] == "follow" &&
			record.DelegatedScopes[1] == "read" &&
			record.Version == 4
	})).Return(query).Once()
	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	query.On("Where", "PK", "=", "USER#agent").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Remove", mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("ConditionVersion", int64(4)).Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(nil).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	state := &storage.AgentGovernanceState{
		Username:        "agent",
		DelegatedScopes: []string{"read", "follow"},
		CreatedAt:       existingCreatedAt,
		UpdatedAt:       now,
		Version:         4,
	}
	err := repo.PutAgentGovernanceState(ctx, state)
	require.NoError(t, err)
	require.Equal(t, 5, state.Version)
}

func TestAgentGovernanceRepository_PutAgentGovernanceState_MapsConditionFailuresToVersionConflict(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	updateBuilder := new(mocks.MockUpdateBuilder)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(query).Once()
	query.On("Where", "PK", "=", "USER#agent").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("UpdateBuilder").Return(updateBuilder).Once()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Remove", mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("ConditionVersion", int64(2)).Return(updateBuilder).Once()
	updateBuilder.On("Execute").Return(dynamormerrors.ErrConditionFailed).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	err := repo.PutAgentGovernanceState(ctx, &storage.AgentGovernanceState{
		Username:  "agent",
		CreatedAt: time.Now().Add(-time.Hour).UTC(),
		UpdatedAt: time.Now().UTC(),
		Version:   2,
	})
	require.ErrorIs(t, err, storage.ErrVersionConflict)
}

func TestAgentGovernanceRepository_GetAgentGovernanceStatesByUsernames_Dedupes(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	query := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(query).Twice()
	query.On("Where", "PK", "=", "USER#agent").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.AgentGovernanceState)
		*record = models.AgentGovernanceState{Username: "agent", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	}).Return(nil).Once()
	query.On("Where", "PK", "=", "USER#missing").Return(query).Once()
	query.On("Where", "SK", "=", models.SKAgentGovernance).Return(query).Once()
	query.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	states, err := repo.GetAgentGovernanceStatesByUsernames(ctx, []string{"Agent", "agent", "missing"})
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Contains(t, states, "agent")
}
