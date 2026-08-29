package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestUserRepository_GetTrustScore_CalculatesWithPropagationAndCaches(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	repo := NewUserRepository(mockDB, "test-table", zap.NewNop())

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)

	actorA := "actorA"
	actorB := "actorB"
	actorC := "actorC"
	actorD := "actorD"

	// Trust relationships for actorA (direct) and actorB (propagation expansion).
	// GetTrustedByRelationships now queries per category via gsi1PK equality, so
	// the seeded rows are served under the category partition key that holds them.
	// The gsi1PK-scoped Where expectation must be registered before the generic
	// one (testify resolves the first matching expectation).
	lastGSI1PK := ""
	mockQuery.On("Where", "gsi1PK", "=", mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		lastGSI1PK, _ = args.Get(2).(string)
	})
	mockQuery.On("All", mock.AnythingOfType("*[]*models.TrustRelationship")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.TrustRelationship)
		switch lastGSI1PK {
		case "TRUSTED#" + actorA + "#general":
			// Direct: actorB trusts actorA.
			*dest = []*models.TrustRelationship{
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
			}
		case "TRUSTED#" + actorB + "#general":
			// Expansion: actorC and actorD trust actorB.
			*dest = []*models.TrustRelationship{
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
		default:
			*dest = nil
		}
	}).Return(nil)

	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)
	mockQuery.On("CreateOrUpdate").Return(nil)

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

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery)
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", mock.Anything).Return(mockQuery)

	// Cache miss.
	mockQuery.On("First", mock.AnythingOfType("*models.TrustScore")).Return(dynamormerrors.ErrItemNotFound).Once()
	// No trust relationships.
	mockQuery.On("All", mock.AnythingOfType("*[]*models.TrustRelationship")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.TrustRelationship)
		*dest = nil
	}).Return(nil)
	// Cache write fails.
	mockQuery.On("CreateOrUpdate").Return(ErrTestMockError)

	score, err := repo.GetTrustScore(context.Background(), "actorA", "general")
	assert.NoError(t, err)
	assert.NotNil(t, score)
}
