package advanced

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestNewPatternRepositoryAdapter_NilRepository(t *testing.T) {
	assert.Nil(t, NewPatternRepositoryAdapter(nil))
}

func TestPatternRepositoryAdapter_ModelConversions(t *testing.T) {
	assert.Nil(t, toModelPattern(nil))
	assert.Nil(t, fromModelPattern(nil))

	now := time.Now().UTC()
	pattern := &ModerationPattern{
		ID:          "p1",
		Pattern:     "spam",
		Type:        "keyword",
		Category:    "spam",
		Name:        "Spam",
		Severity:    0.9,
		Description: "desc",
		Active:      true,
		Flags:       []string{"a", "b"},
		CreatedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		HitCount:    42,
		LastHit:     now.Add(2 * time.Second),
	}

	modelPattern := toModelPattern(pattern)
	require.NotNil(t, modelPattern)
	assert.Equal(t, pattern.ID, modelPattern.PatternID)
	assert.Equal(t, pattern.Pattern, modelPattern.Pattern)
	assert.Equal(t, pattern.Type, modelPattern.Type)
	assert.Equal(t, pattern.Category, modelPattern.Category)
	assert.Equal(t, pattern.Name, modelPattern.Name)
	assert.Equal(t, pattern.Severity, modelPattern.Severity)
	assert.Equal(t, pattern.Description, modelPattern.Description)
	assert.Equal(t, pattern.Active, modelPattern.Active)
	assert.Equal(t, pattern.Flags, modelPattern.Flags)
	assert.Equal(t, pattern.CreatedAt, modelPattern.CreatedAt)
	assert.Equal(t, pattern.UpdatedAt, modelPattern.UpdatedAt)
	assert.Equal(t, pattern.HitCount, modelPattern.HitCount)
	assert.Equal(t, pattern.LastHit, modelPattern.LastHit)

	roundTrip := fromModelPattern(modelPattern)
	require.NotNil(t, roundTrip)
	assert.Equal(t, pattern, roundTrip)
}

func TestPatternRepositoryAdapter_GetPatterns_AppliesFiltersAndLimit(t *testing.T) {
	ctx := context.Background()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", models.SKMetadata).Return(mockQuery).Once()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.ModerationPattern)
		*dest = []*models.ModerationPattern{
			{PatternID: "p1", Type: "keyword", Category: "spam", Severity: 0.1, Active: true},
			{PatternID: "p2", Type: "regex", Category: "spam", Severity: 0.9, Active: true},
			{PatternID: "p3", Type: "regex", Category: "spam", Severity: 0.95, Active: true},
		}
	}).Once()

	repo := repositories.NewPatternRepository(mockDB, "table", zap.NewNop(), nil)
	adapter := NewPatternRepositoryAdapter(repo)
	require.NotNil(t, adapter)

	active := true
	patterns, err := adapter.GetPatterns(ctx, PatternFilter{
		Category:    "spam",
		Type:        "regex",
		Active:      &active,
		MinSeverity: 0.9,
		Limit:       1,
	})
	require.NoError(t, err)
	require.Len(t, patterns, 1)
	assert.Equal(t, "p2", patterns[0].ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestModerationError_Error(t *testing.T) {
	err := &ModerationError{Message: "boom"}
	assert.Equal(t, "boom", err.Error())
}
