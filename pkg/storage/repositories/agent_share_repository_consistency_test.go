package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormMocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// TestIsActiveAgentShareGrantPinsConsistentDirectRead pins the authorization-read
// contract from docs/contracts/agent-share-grants.md: the authz read must be a
// direct base-table GetItem-style First against the exact (PK, SK) with
// ConsistentRead, and must never go through an index (GSI2 is discovery-only).
func TestIsActiveAgentShareGrantPinsConsistentDirectRead(t *testing.T) {
	ctx := context.Background()
	mockDB := new(dynamormMocks.MockDB)
	mockQuery := new(dynamormMocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", models.AgentShareGrantPK("agent-one")).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.AgentShareGrantSK("alice")).Return(mockQuery).Once()
	mockQuery.On("ConsistentRead").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		grant, ok := args.Get(0).(*models.AgentShareGrant)
		if ok {
			*grant = models.AgentShareGrant{
				AgentUsername:   "agent-one",
				GranteeUsername: "alice",
			}
		}
	}).Return(nil).Once()

	repo := NewAgentShareRepository(mockDB, "test-table", zap.NewNop())
	active, err := repo.IsActiveAgentShareGrant(ctx, "agent-one", "alice")
	require.NoError(t, err)
	require.True(t, active)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockQuery.AssertNotCalled(t, "Index", mock.Anything)
	mockQuery.AssertNotCalled(t, "All", mock.Anything)
}
