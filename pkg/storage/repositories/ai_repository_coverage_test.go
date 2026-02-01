package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/ai"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestAIRepository_SaveAndGetAndStats_CoverageSweep(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockCreateQuery := new(mocks.MockQuery)
	mockQueryWithPrefix := new(mocks.MockQuery)
	mockGetQuery := new(mocks.MockQuery)
	mockStatsQuery1 := new(mocks.MockQuery)
	mockQueueUpdateQuery := new(mocks.MockQuery)

	repo := NewAIRepository(mockDB, "test-table", logger, nil)

	analysisTime := time.Date(2025, 1, 1, 1, 2, 3, 0, time.UTC)
	analysis := &ai.AIAnalysis{
		ID:         "analysis-1",
		ObjectID:   "object-1",
		ObjectType: "status",
		AnalyzedAt: analysisTime,
		Version:    "1.0",
		TextAnalysis: &ai.TextAnalysis{
			ToxicityScore: 0.9,
			PIIEntities:   []ai.PIIEntity{{Type: ai.PiiEmail, Text: "a@b.com"}},
			Categories:    []ai.ContentCategory{{Name: "news"}, {Name: "sports"}},
		},
		ImageAnalysis:    &ai.ImageAnalysis{IsNSFW: true},
		AIDetection:      &ai.AIDetection{AIGeneratedProbability: 0.9},
		SpamAnalysis:     &ai.SpamAnalysis{SpamScore: 0.9},
		OverallRisk:      0.8,
		ModerationAction: "hide",
		Confidence:       0.99,
	}

	// SaveAnalysis -> ValidateAndCreate -> Create
	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil).Once()

	require.NoError(t, repo.SaveAnalysis(ctx, analysis))

	// GetAnalysis -> QueryWithSKPrefix -> sort desc -> convertFromModel
	mockDB.On("Model", mock.Anything).Return(mockQueryWithPrefix).Once()
	mockQueryWithPrefix.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQueryWithPrefix).Maybe()
	mockQueryWithPrefix.On("OrderBy", mock.Anything, mock.Anything).Return(mockQueryWithPrefix).Maybe()
	mockQueryWithPrefix.On("Limit", mock.Anything).Return(mockQueryWithPrefix).Maybe()
	mockQueryWithPrefix.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.AIAnalysis)
		*dest = append(*dest,
			&models.AIAnalysis{ID: "a", ObjectID: "object-1", SK: "ANALYSIS#a", AnalyzedAt: analysisTime.Add(-10 * time.Minute)},
			&models.AIAnalysis{ID: "b", ObjectID: "object-1", SK: "ANALYSIS#b", AnalyzedAt: analysisTime.Add(-1 * time.Minute)},
		)
	}).Return(nil).Once()

	got, err := repo.GetAnalysis(ctx, "object-1")
	require.NoError(t, err)
	require.Equal(t, "b", got.ID)

	// GetAnalysisByID -> Get -> First
	mockDB.On("Model", mock.Anything).Return(mockGetQuery).Once()
	mockGetQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockGetQuery).Maybe()
	mockGetQuery.On("First", mock.AnythingOfType("*models.AIAnalysis")).Run(func(args mock.Arguments) {
		model := args.Get(0).(*models.AIAnalysis)
		model.ID = "analysis-1"
		model.ObjectID = "object-1"
		model.ObjectType = "status"
		model.AnalyzedAt = analysisTime
	}).Return(nil).Once()

	byID, err := repo.GetAnalysisByID(ctx, "object-1", "analysis-1")
	require.NoError(t, err)
	require.Equal(t, "analysis-1", byID.ID)

	// GetStats -> QueryGSIPaginated single page (NextCursor can't be computed due to field-name mismatch)
	mockDB.On("Model", mock.Anything).Return(mockStatsQuery1).Once()
	mockStatsQuery1.On("Index", mock.Anything).Return(mockStatsQuery1).Once()
	mockStatsQuery1.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockStatsQuery1).Maybe()
	mockStatsQuery1.On("OrderBy", mock.Anything, mock.Anything).Return(mockStatsQuery1).Once()
	mockStatsQuery1.On("Limit", mock.Anything).Return(mockStatsQuery1).Once()
	mockStatsQuery1.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.AIAnalysis)
		now := time.Now().UTC()

		*dest = []*models.AIAnalysis{
			{ID: "old", ObjectID: "object-1", ObjectType: "status", AnalyzedAt: now.AddDate(0, 0, -10)},
			{
				ID:         "new",
				ObjectID:   "object-1",
				ObjectType: "status",
				AnalyzedAt: now.Add(-10 * time.Minute),
				TextAnalysis: &ai.TextAnalysis{
					ToxicityScore: 0.9,
					PIIEntities:   []ai.PIIEntity{{Type: ai.PiiEmail, Text: "a@b.com"}},
				},
				SpamAnalysis:     &ai.SpamAnalysis{SpamScore: 0.9},
				AIDetection:      &ai.AIDetection{AIGeneratedProbability: 0.9},
				ImageAnalysis:    &ai.ImageAnalysis{IsNSFW: true},
				ModerationAction: "review",
			},
		}
	}).Return(nil).Once()

	stats, err := repo.GetStats(ctx, "hour")
	require.NoError(t, err)
	require.Equal(t, "hour", stats.Period)
	require.Greater(t, stats.TotalAnalyses, 0)
	require.GreaterOrEqual(t, stats.ToxicityRate, 0.0)

	// QueueForAnalysis -> direct Update
	mockDB.On("Model", mock.Anything).Return(mockQueueUpdateQuery).Once()
	mockQueueUpdateQuery.On("Update", mock.Anything).Return(nil).Once()
	require.NoError(t, repo.QueueForAnalysis(ctx, "object-1"))

	// AnalyzeContent hits preprocess/performMLAnalysis/SaveAnalysis paths.
	mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(nil).Once()
	analysis2, err := repo.AnalyzeContent(ctx, "content", "status")
	require.NoError(t, err)
	require.NotEmpty(t, analysis2.ID)

	// GetContentClassifications uses GetAnalysis and pulls categories.
	mockDB.On("Model", mock.Anything).Return(mockQueryWithPrefix).Once()
	mockQueryWithPrefix.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.AIAnalysis)
		*dest = []*models.AIAnalysis{
			{
				ID:         "c",
				ObjectID:   "content-1",
				SK:         "ANALYSIS#c",
				AnalyzedAt: analysisTime,
				TextAnalysis: &ai.TextAnalysis{
					Categories: []ai.ContentCategory{{Name: "a"}, {Name: "b"}},
				},
			},
		}
	}).Return(nil).Once()
	classifications, err := repo.GetContentClassifications(ctx, "content-1")
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, classifications)

	sub, err := repo.SubscribeToAnalysisEvents(ctx, "user-1", nil)
	require.NoError(t, err)
	require.NotNil(t, sub.Events())
	require.NoError(t, sub.Close())

	require.NoError(t, repo.UpdateModelPerformance(ctx, "model-1", map[string]float64{"acc": 0.9}))
	require.NoError(t, repo.ProcessMLFeedback(ctx, "analysis-1", map[string]interface{}{"ok": true}))
	require.NoError(t, repo.MonitorAIHealth(ctx))

	mockDB.AssertExpectations(t)
}

func TestAIRepository_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	mockDB := new(mocks.MockDB)
	mockCreateQuery := new(mocks.MockQuery)
	mockQuery := new(mocks.MockQuery)
	mockQueueUpdateQuery := new(mocks.MockQuery)
	mockGetQuery := new(mocks.MockQuery)

	repo := NewAIRepository(mockDB, "test-table", logger, nil)

	analysis := &ai.AIAnalysis{
		ID:         "analysis-err",
		ObjectID:   "object-err",
		ObjectType: "status",
		AnalyzedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	mockDB.On("WithContext", ctx).Return(mockDB).Maybe()

	// SaveAnalysis create error
	mockDB.On("Model", mock.Anything).Return(mockCreateQuery).Once()
	mockCreateQuery.On("Create").Return(errors.New("create failed")).Once()
	require.Error(t, repo.SaveAnalysis(ctx, analysis))

	// GetAnalysis query error
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()

	_, err := repo.GetAnalysis(ctx, "object-err")
	require.Error(t, err)

	// GetAnalysisByID not found
	mockDB.On("Model", mock.Anything).Return(mockGetQuery).Once()
	mockGetQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockGetQuery).Maybe()
	mockGetQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	_, err = repo.GetAnalysisByID(ctx, "object-err", "analysis-err")
	require.Error(t, err)

	// QueueForAnalysis update error
	mockDB.On("Model", mock.Anything).Return(mockQueueUpdateQuery).Once()
	mockQueueUpdateQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
	require.Error(t, repo.QueueForAnalysis(ctx, "object-err"))

	mockDB.AssertExpectations(t)
}
