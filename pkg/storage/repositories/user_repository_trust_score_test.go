package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestUserRepository_GetTrustScore_CalculatesWithPropagationAndCaches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	actorA := "actorA"
	actorB := "actorB"
	actorC := "actorC"
	actorD := "actorD"

	// Cache miss for actorA.
	mockQuery.On("First", mock.AnythingOfType("*models.TrustScore")).Return(dynamormerrors.ErrItemNotFound).Once()
	// Cache hits for propagation nodes (deterministic order).
	mockQuery.On("First", mock.AnythingOfType("*models.TrustScore")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.TrustScore)
		out.ActorID = actorB
		out.Category = models.TrustCategoryGeneral
		out.Score = 0.9
		out.CacheTTL = time.Now().Add(time.Hour)
	}).Return(nil).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.TrustScore")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.TrustScore)
		out.ActorID = actorC
		out.Category = models.TrustCategoryGeneral
		out.Score = 0.05
		out.CacheTTL = time.Now().Add(time.Hour)
	}).Return(nil).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.TrustScore")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.TrustScore)
		out.ActorID = actorD
		out.Category = models.TrustCategoryGeneral
		out.Score = 0.8
		out.CacheTTL = time.Now().Add(time.Hour)
	}).Return(nil).Once()

	// Trust relationships for actorA (direct) and actorB (propagation expansion).
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.TrustRelationship)
		*dest = []*models.TrustRelationship{
			// Direct: actorB trusts actorA.
			{
				GSI1PK:     "TRUSTED#" + actorA + "#general",
				GSI1SK:     "TRUSTER#" + actorB,
				TrusterID:  actorB,
				TrusteeID:  actorA,
				Category:   models.TrustCategoryGeneral,
				Score:      0.9,
				Confidence: 1.0,
				Type:       "RELATIONSHIP",
			},
			// Expansion: actorC and actorD trust actorB.
			{
				GSI1PK:     "TRUSTED#" + actorB + "#general",
				GSI1SK:     "TRUSTER#" + actorC,
				TrusterID:  actorC,
				TrusteeID:  actorB,
				Category:   models.TrustCategoryGeneral,
				Score:      0.5,
				Confidence: 1.0,
				Type:       "RELATIONSHIP",
			},
			{
				GSI1PK:     "TRUSTED#" + actorB + "#general",
				GSI1SK:     "TRUSTER#" + actorD,
				TrusterID:  actorD,
				TrusteeID:  actorB,
				Category:   models.TrustCategoryGeneral,
				Score:      0.7,
				Confidence: 1.0,
				Type:       "RELATIONSHIP",
			},
		}
	}).Return(nil)

	score, err := repo.GetTrustScore(context.Background(), actorA, "general")
	assert.NoError(t, err)
	assert.NotNil(t, score)
	assert.Equal(t, actorA, score.ActorID)
	assert.Greater(t, score.DirectScore, 0.0)
	assert.Greater(t, score.PropagatedScore, 0.0)
	assert.Greater(t, score.Score, 0.0)
	assert.Equal(t, 1, score.TrusterCount)
	assert.False(t, score.LastCalculated.IsZero())
	assert.True(t, score.CacheTTL.After(time.Now()))
}

func TestUserRepository_GetTrustScore_CacheWriteFailureReturnsCalculatedScore(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// Cache miss.
	mockQuery.On("First", mock.AnythingOfType("*models.TrustScore")).Return(dynamormerrors.ErrItemNotFound).Once()
	// No trust relationships.
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.TrustRelationship)
		*dest = nil
	}).Return(nil)
	// Cache write fails.
	mockQuery.On("Create").Return(ErrTestMockError)

	score, err := repo.GetTrustScore(context.Background(), "actorA", "general")
	assert.NoError(t, err)
	assert.NotNil(t, score)
}
