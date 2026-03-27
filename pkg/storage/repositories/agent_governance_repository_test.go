package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
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
	loadQuery := new(mocks.MockQuery)
	createQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER#agent").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", models.SKAgentGovernance).Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Return(dynamormerrors.ErrItemNotFound).Once()

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
			record.SelfScopes[1] == "write"
	})).Return(createQuery).Once()
	createQuery.On("IfNotExists").Return(createQuery).Once()
	createQuery.On("Create").Return(nil).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	err := repo.PutAgentGovernanceState(ctx, &storage.AgentGovernanceState{
		Username:         " Agent ",
		QuarantineStatus: storage.AgentQuarantineStatusQuarantined,
		DelegatedScopes:  []string{"write", "read", "read", ""},
		SelfScopes:       []string{"follow", "write", "write"},
		SelfSovereign:    true,
		Verified:         true,
	})
	require.NoError(t, err)
}

func TestAgentGovernanceRepository_PutAgentGovernanceState_PreservesCreatedAtOnUpdate(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	loadQuery := new(mocks.MockQuery)
	updateQuery := new(mocks.MockQuery)
	existingCreatedAt := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Twice()
	mockDB.On("Model", mock.AnythingOfType("*models.AgentGovernanceState")).Return(loadQuery).Once()
	loadQuery.On("Where", "PK", "=", "USER#agent").Return(loadQuery).Once()
	loadQuery.On("Where", "SK", "=", models.SKAgentGovernance).Return(loadQuery).Once()
	loadQuery.On("First", mock.AnythingOfType("*models.AgentGovernanceState")).Run(func(args mock.Arguments) {
		record := args.Get(0).(*models.AgentGovernanceState)
		*record = models.AgentGovernanceState{
			PK:              "USER#agent",
			SK:              models.SKAgentGovernance,
			Username:        "agent",
			DelegatedScopes: []string{"read"},
			CreatedAt:       existingCreatedAt,
			UpdatedAt:       existingCreatedAt.Add(time.Hour),
		}
	}).Return(nil).Once()

	mockDB.On("Model", mock.MatchedBy(func(record *models.AgentGovernanceState) bool {
		return record != nil &&
			record.Username == "agent" &&
			record.CreatedAt.Equal(existingCreatedAt) &&
			record.UpdatedAt.Equal(now) &&
			len(record.DelegatedScopes) == 2 &&
			record.DelegatedScopes[0] == "follow" &&
			record.DelegatedScopes[1] == "read"
	})).Return(updateQuery).Once()
	updateQuery.On("Update", mock.Anything).Return(nil).Once()

	repo := NewAgentGovernanceRepository(mockDB, "test-table", zap.NewNop())
	err := repo.PutAgentGovernanceState(ctx, &storage.AgentGovernanceState{
		Username:        "agent",
		DelegatedScopes: []string{"read", "follow"},
		CreatedAt:       time.Time{},
		UpdatedAt:       now,
	})
	require.NoError(t, err)
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
