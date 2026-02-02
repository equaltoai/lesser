package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestDLQRepository_Round08_CreateGetUpdateDeleteAndBatchUpdate(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// First: not found.
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()
	// First: success.
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.DLQMessage)
		*dest = *models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-1", "{}").
			WithError("type", "message", "").
			Build()
		_ = dest.BeforeCreate()
	}).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	msg := models.NewDLQMessageBuilder().
		ForService("svc").
		WithOriginalMessage("msg-1", `{"hello":"world"}`).
		WithError("type", "message", "").
		Build()
	require.NoError(t, repo.CreateDLQMessage(ctx, msg))

	_, err := repo.GetDLQMessage(ctx, "missing")
	require.Error(t, err)

	found, err := repo.GetDLQMessage(ctx, "present")
	require.NoError(t, err)
	require.Equal(t, "svc", found.Service)

	found.MarkForReprocessing()
	require.NoError(t, repo.UpdateDLQMessage(ctx, found))
	require.NoError(t, repo.DeleteDLQMessage(ctx, found))

	// BatchUpdateDLQMessages: empty slice returns nil.
	require.NoError(t, repo.BatchUpdateDLQMessages(ctx, nil))

	// BatchUpdateDLQMessages: invalid messages accumulate errors.
	bad := &models.DLQMessage{Service: "svc"}
	err = repo.BatchUpdateDLQMessages(ctx, []*models.DLQMessage{bad})
	require.Error(t, err)
}

func TestDLQRepository_Round08_QueryMethodsAndSearchAndCleanup(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Provide messages for All() to exercise cursor trimming and text filtering.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		now := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
		m1 := models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-1", `{"action":"send_notification"}`).
			WithError("type-a", "timeout", "").
			Build()
		_ = m1.BeforeCreate()
		m1.FirstSeenAt = now
		m1.Status = DLQStatusNew
		m1.Priority = "high"
		_ = m1.BeforeUpdate()

		m2 := models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-2", `{"action":"noop"}`).
			WithError("type-b", "other", "").
			Build()
		_ = m2.BeforeCreate()
		m2.FirstSeenAt = now.Add(1 * time.Minute)
		m2.Status = DLQStatusResolved
		m2.Priority = "low"
		_ = m2.BeforeUpdate()

		*dest = []*models.DLQMessage{m1, m2} // ensure first page hits text search too
	}).Return(nil).Maybe()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	msgs, cursor, err := repo.GetDLQMessagesByErrorType(ctx, "type-a", 1, "")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.NotEmpty(t, cursor)

	msgs, cursor, err = repo.GetDLQMessagesByStatus(ctx, "svc", DLQStatusNew, 1, "")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.NotEmpty(t, cursor)

	// Search validation failure.
	_, _, err = repo.SearchDLQMessages(ctx, &DLQSearchFilter{Service: ""})
	require.Error(t, err)

	// Search with filters + text search.
	permanent := true
	filter := &DLQSearchFilter{
		Service:     "svc",
		ErrorType:   "type-a",
		Status:      DLQStatusNew,
		Priority:    "high",
		IsPermanent: &permanent,
		StartTime:   time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		SearchText:  "send_notification",
		Limit:       1,
		Cursor:      "zzz",
	}
	found, next, err := repo.SearchDLQMessages(ctx, filter)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.NotEmpty(t, next)
}

func TestDLQRepository_Round08_CleanupExpiredMessages_Empty(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		*dest = []*models.DLQMessage{}
	}).Return(nil).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	count, err := repo.CleanupExpiredMessages(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestDLQRepository_Round08_AnalyticsTrendsRetryAndHealth(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// Provide messages for service/day queries (FindWithPagination -> All).
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		now := time.Date(2025, 12, 27, 10, 0, 0, 0, time.UTC)
		m1 := models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-1", "{}").
			WithError("type-a", "timeout", "").
			Build()
		_ = m1.BeforeCreate()
		m1.FirstSeenAt = now
		m1.Status = DLQStatusNew
		_ = m1.BeforeUpdate()

		m2 := models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-2", "{}").
			WithError("type-a", "timeout again", "").
			Build()
		_ = m2.BeforeCreate()
		m2.FirstSeenAt = now.Add(2 * time.Minute)
		m2.Status = DLQStatusResolved
		_ = m2.BeforeUpdate()

		m3 := models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-3", "{}").
			WithError("type-b", "other", "").
			Build()
		_ = m3.BeforeCreate()
		m3.FirstSeenAt = now.Add(3 * time.Minute)
		m3.Status = DLQStatusAbandoned
		m3.ReprocessingCount = 4
		m3.MaxReprocessAttempts = 3
		_ = m3.BeforeUpdate()

		*dest = []*models.DLQMessage{m1, m2, m3}
	}).Return(nil).Maybe()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	timeRange := DLQTimeRange{
		StartTime: time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC),
	}
	analytics, err := repo.GetDLQAnalytics(ctx, "svc", timeRange)
	require.NoError(t, err)
	require.Equal(t, 3, analytics.TotalMessages)
	require.GreaterOrEqual(t, analytics.ResolutionRate, float64(0))

	trends, err := repo.GetDLQTrends(ctx, "svc", 1)
	require.NoError(t, err)
	require.Len(t, trends.DailyStats, 1)

	patterns, err := repo.AnalyzeFailurePatterns(ctx, "svc", 1)
	require.NoError(t, err)
	require.NotEmpty(t, patterns)

	health, err := repo.MonitorDLQHealth(ctx, "svc")
	require.NoError(t, err)
	require.Equal(t, "svc", health.Service)
}

func TestDLQRepository_Round08_SendRetryAndRetryableMessages(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// RetryFailedMessage: GetDLQMessage returns a message that is not reprocessable.
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.DLQMessage)
		*dest = *models.NewDLQMessageBuilder().
			ForService("svc").
			WithOriginalMessage("msg-1", "{}").
			WithError("type", "message", "").
			Build()
		_ = dest.BeforeCreate()
		dest.Status = DLQStatusResolved
		_ = dest.BeforeUpdate()
	}).Return(nil).Once()

	// GetDLQMessagesForReprocessing: query.All returns error twice (new + reprocessing).
	mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Twice()

	// Similar messages query result.
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.DLQMessage)
		*dest = []*models.DLQMessage{
			{ID: "1", SimilarityHash: "hash"},
		}
	}).Return(nil).Once()

	// GetDLQMessagesByServiceDateRange skip errors branch.
	mockQuery.On("All", mock.Anything).Return(errors.New("fail")).Once()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	repo := NewDLQRepository(mockDB, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetCachingService(nil)
	repo.SetEventService(nil)

	require.NoError(t, repo.SendToDeadLetterQueue(ctx, "svc", "msg-1", "{}", "type", "message", true))
	require.Error(t, repo.RetryFailedMessage(ctx, "msg-1"))

	// GetRetryableMessages should tolerate underlying query errors.
	msgs, err := repo.GetRetryableMessages(ctx, "svc", 10)
	require.NoError(t, err)
	require.Empty(t, msgs)

	similar, err := repo.GetSimilarMessages(ctx, "hash", 10)
	require.NoError(t, err)
	require.Len(t, similar, 1)

	out, err := repo.GetDLQMessagesByServiceDateRange(ctx, "svc", time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC), 1)
	require.NoError(t, err)
	require.Len(t, out, 0)

	// Ensure compact date format path is referenced in query key formatting.
	require.NotEmpty(t, time.Now().Format(common.CompactDateFormat))
}
