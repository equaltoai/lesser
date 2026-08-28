package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch S2 (umbrella #1469, issue #1498) — the moderation/ML unbounded-scan
// baseline: 8 keyed-filter `.Scan` terminals (moderation_repository.go
// GetAuditLogs/getAuditLogsByGSI, moderation_ml_repository.go
// ListSamplesByLabel/ListSamplesByReviewer/ListEffectivenessMetricsByPattern/
// ListEffectivenessMetricsByPeriod, moderation_ml_repository_ext.go
// GetPredictionsByModelVersion/GetPredictionsByReviewStatus) and the key-less
// `goDynamoDBAllNoKey` GetFlag site.
//
// Every `.Scan` site carried key predicates on a chain whose terminal was
// `.Scan` (which tabletheory compiles to a DynamoDB Scan unconditionally), so
// the key predicates were only ever post-read filters. Each conversion
// switches the terminal to a keyed `.All` (identical row set on the same
// index/partition) with a limit clamp/floor so `Limit(n>0)` is always issued
// (a zero/negative limit previously compiled to NO limit — an unbounded read).
// GetFlag moves to an additive GSI3 listing key (FLAGS#ALL / ID#<flag id>,
// pre-existing slot, batch-F precedent) read as an exact keyed point lookup.
//
// Every assertion pins a LITERAL so a mutation dies: the key-condition chain,
// the Limit values (floored/clamped/pass-through), the All/First terminals
// (a mutation restoring `.Scan` dies on the strict mock — no Scan expectation
// is ever registered), and error propagation through the HandleGetError /
// HandleQueryError wraps (require.ErrorIs). Batch S2 contains no whole-partition
// walks, so there are no `walkKeyedPages`/errBoundedPageCapExceeded paths to
// pin; the wrap-path ErrorIs pins below are the batch's analog.

// GetAuditLogs: the AUDIT_LOG partition-key equality + optional SK > cursor
// chain must terminate in a keyed `.All` with a clamped Limit. A zero limit is
// floored to the query-utils default 50 — a mutation dropping the clamp (or
// restoring `.Scan`) dies on the literal/strict expectations.
func TestBatchS2_GetAuditLogs_KeyedAllWithFloor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AuditLog")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AUDIT_LOG").Return(mockQuery).Once()
	mockQuery.On("Limit", 50).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.AuditLog")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.AuditLog)
		a := &models.AuditLog{
			ID: "audit-1", AdminID: "admin-1", Action: "resolve_report",
			TargetType: "report", TargetID: "report-1", Timestamp: time.Now(),
		}
		a.UpdateKeys()
		*dest = []*models.AuditLog{a}
	}).Return(nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	logs, next, err := repo.GetAuditLogs(ctx, 0, "")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotEmpty(t, next)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetAuditLogs with a cursor: the SK > cursor range must be a key condition on
// the same keyed chain, and an oversized limit is clamped to the query-utils
// hard max 100 — a mutation dropping either dies on the literals.
func TestBatchS2_GetAuditLogs_KeyedAllCursorAndMaxClamp(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AuditLog")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AUDIT_LOG").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", ">", "c1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.AuditLog")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.AuditLog)
		a := &models.AuditLog{ID: "audit-2", AdminID: "admin-1", Timestamp: time.Now()}
		a.UpdateKeys()
		*dest = []*models.AuditLog{a}
	}).Return(nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, _, err := repo.GetAuditLogs(ctx, 1000, "c1")
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// getAuditLogsByGSI (via GetAuditLogsByAdmin): the GSI1 ADMIN# partition chain
// must terminate in a keyed `.All` with the sort-key cursor as a key condition
// and a clamped Limit. A mutation restoring `.Scan` dies on the strict mock.
func TestBatchS2_GetAuditLogsByGSI_KeyedAllWithClamp(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AuditLog")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "ADMIN#admin-1").Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Limit", 50).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">", "c1").Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.AuditLog")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.AuditLog)
		a := &models.AuditLog{ID: "audit-3", AdminID: "admin-1", Timestamp: time.Now()}
		a.UpdateKeys()
		*dest = []*models.AuditLog{a}
	}).Return(nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	logs, next, err := repo.GetAuditLogsByAdmin(ctx, "admin-1", 0, "c1")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotEmpty(t, next)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetAuditLogs error: the `.All` error must propagate through the
// HandleQueryError wrap unchanged (require.ErrorIs) — a mutation restoring
// `.Scan` dies on the unexpected-call panic first.
func TestBatchS2_GetAuditLogs_ErrorThroughHandleQueryError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	dbErr := fmt.Errorf("audit boom")

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.AuditLog")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "AUDIT_LOG").Return(mockQuery).Once()
	mockQuery.On("Limit", 1).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.AuditLog")).Return(dbErr).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, _, err := repo.GetAuditLogs(ctx, 1, "")
	require.Error(t, err)
	require.ErrorIs(t, err, dbErr)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ListSamplesByLabel: the gsi2 LABEL# chain must terminate in a keyed `.All`
// with the zero-limit floor (100) applied — a mutation dropping the floor
// issues Limit(0) (unbounded) and dies on the Limit literal.
func TestBatchS2_ListSamplesByLabel_KeyedAllWithFloor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationSample")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "LABEL#spam").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.ModerationSample")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationSample)
		*dest = []models.ModerationSample{{ID: "s1", Label: "spam"}, {ID: "s2", Label: "spam"}}
	}).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	samples, err := repo.ListSamplesByLabel(ctx, "spam", 0)
	require.NoError(t, err)
	require.Len(t, samples, 2)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ListSamplesByReviewer: the gsi1 REVIEWER# chain must terminate in a keyed
// `.All`; a negative limit is floored to 100.
func TestBatchS2_ListSamplesByReviewer_KeyedAllWithFloor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationSample")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "REVIEWER#r1").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.ModerationSample")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationSample)
		*dest = []models.ModerationSample{{ID: "s3", ReviewerID: "r1"}}
	}).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	samples, err := repo.ListSamplesByReviewer(ctx, "r1", -1)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ListEffectivenessMetricsByPattern: the MLMETRICS# partition chain must
// terminate in a keyed `.All` with the zero-limit floor (50) applied.
func TestBatchS2_ListEffectivenessMetricsByPattern_KeyedAllWithFloor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationEffectivenessMetric")).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", "MLMETRICS#p1").Return(mockQuery).Once()
	mockQuery.On("Limit", 50).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.ModerationEffectivenessMetric")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationEffectivenessMetric)
		*dest = []models.ModerationEffectivenessMetric{{PatternID: "p1", Period: "daily"}}
	}).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	metrics, err := repo.ListEffectivenessMetricsByPattern(ctx, "p1", 0)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ListEffectivenessMetricsByPeriod: the gsi1 METRICS# chain must terminate in
// a keyed `.All` with the zero-limit floor (50) applied.
func TestBatchS2_ListEffectivenessMetricsByPeriod_KeyedAllWithFloor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.ModerationEffectivenessMetric")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "METRICS#daily").Return(mockQuery).Once()
	mockQuery.On("Limit", 50).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]models.ModerationEffectivenessMetric")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.ModerationEffectivenessMetric)
		*dest = []models.ModerationEffectivenessMetric{{PatternID: "p1", Period: "daily"}}
	}).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	metrics, err := repo.ListEffectivenessMetricsByPeriod(ctx, "daily", 0)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetPredictionsByModelVersion: the gsi1 MODEL# + gsi1SK range chain must
// terminate in a keyed `.All`. The internal caller passes 1000 (full
// reviewed-prediction set for the period) and that limit MUST pass through
// unclamped — a mutation adding the query-utils 100 max narrows the
// aggregation and dies on the Limit(1000) literal.
func TestBatchS2_GetPredictionsByModelVersion_KeyedAllLimitPassThrough(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "MODEL#v1.0").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", "TIME#2024-01-01T00:00:00Z").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", "TIME#2024-01-31T00:00:00Z").Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Limit", 1000).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.MLPrediction")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MLPrediction)
		*dest = []*models.MLPrediction{{PredictionID: "pred-1", ModelVersion: "v1.0"}}
	}).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	preds, err := repo.GetPredictionsByModelVersion(ctx, "v1.0", start, end, 1000)
	require.NoError(t, err)
	require.Len(t, preds, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetPredictionsByModelVersion zero-limit floor: a limit <= 0 previously
// compiled Limit(0) — no limit — an unbounded read; it must be floored to 100.
func TestBatchS2_GetPredictionsByModelVersion_ZeroLimitFloored(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "MODEL#v1.0").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", ">=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1SK", "<=", mock.AnythingOfType("string")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
	mockQuery.On("Limit", 100).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.MLPrediction")).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	preds, err := repo.GetPredictionsByModelVersion(ctx, "v1.0", start, end, 0)
	require.NoError(t, err)
	require.Empty(t, preds)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetPredictionsByReviewStatus: the gsi2 REVIEW# + gsi2SK range chain must
// terminate in a keyed `.All`; positive limits pass through unchanged.
func TestBatchS2_GetPredictionsByReviewStatus_KeyedAll(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.MLPrediction")).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2PK", "=", "REVIEW#true").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", ">=", "TIME#2024-01-01T00:00:00Z").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi2SK", "<=", "TIME#2024-01-31T00:00:00Z").Return(mockQuery).Once()
	mockQuery.On("Index", "gsi2").Return(mockQuery).Once()
	mockQuery.On("Limit", 5).Return(mockQuery).Once()
	mockQuery.On("All", mock.AnythingOfType("*[]*models.MLPrediction")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.MLPrediction)
		*dest = []*models.MLPrediction{{PredictionID: "pred-2", Reviewed: true}}
	}).Return(nil).Once()

	repo := NewModerationMLRepository(mockDB, "test-table", zap.NewNop())
	preds, err := repo.GetPredictionsByReviewStatus(ctx, true, start, end, 5)
	require.NoError(t, err)
	require.Len(t, preds, 1)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetFlag: the lookup must be an exact keyed point read on the additive gsi3
// listing key (FLAGS#ALL / ID#<flag id>) via `First` — a mutation restoring
// the old key-less `Where("SK","LIKE",...).Limit(10).All(...)` scan dies on
// the unexpected Where/All calls and the missing Index/First expectations.
func TestBatchS2_GetFlag_KeyedGSI3PointRead(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Flag")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "FLAGS#ALL").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3SK", "=", "ID#flag-1").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Flag")).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.Flag)
		*dest = models.Flag{
			ID: "flag-1", Actor: "actor-1", Object: []string{"obj-1"},
			Content: "reason", Status: "pending",
		}
	}).Return(nil).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	got, err := repo.GetFlag(ctx, "flag-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "flag-1", got.ID)
	require.Equal(t, "actor-1", got.Actor)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// GetFlag not-found: the exact-keyed lookup miss must surface as a not-found
// domain error — the sentinel survives the HandleGetError wrap (require
// ErrorIs storage.ErrNotFound). A mutation that drops the IsNotFound branch
// and routes the miss through HandleQueryError (FailedToQuery, which does not
// carry storage.ErrNotFound) dies on the ErrorIs.
func TestBatchS2_GetFlag_NotFoundThroughHandleGetError(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB).Once()
	mockDB.On("Model", mock.AnythingOfType("*models.Flag")).Return(mockQuery).Once()
	mockQuery.On("Index", "gsi3").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3PK", "=", "FLAGS#ALL").Return(mockQuery).Once()
	mockQuery.On("Where", "gsi3SK", "=", "ID#missing").Return(mockQuery).Once()
	mockQuery.On("First", mock.AnythingOfType("*models.Flag")).Return(errors.ErrItemNotFound).Once()

	repo := NewModerationRepository(mockDB, "test-table", zap.NewNop())
	_, err := repo.GetFlag(ctx, "missing")
	require.Error(t, err)
	require.ErrorIs(t, err, storage.ErrNotFound)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// Flag.UpdateKeys must populate the additive gsi3 listing key on every write —
// GetFlag's exact-keyed lookup depends on it. A mutation dropping the GSI3
// population (or the FLAGS#ALL partition / ID#<flag id> shape) dies on the
// literals below.
func TestBatchS2_Flag_UpdateKeys_PopulatesGSI3ListingKey(t *testing.T) {
	f := &models.Flag{
		ID: "flag-1", Actor: "actor-1", Object: []string{"obj-1"},
		Content: "reason", Published: time.Now(), Status: "pending",
	}
	f.UpdateKeys()

	require.Equal(t, "FLAGS#ALL", f.GSI3PK)
	require.Equal(t, "ID#flag-1", f.GSI3SK)
	// The pre-existing GSI1/GSI2 key shapes are unchanged.
	require.Equal(t, "ACTOR#actor-1", f.GSI1PK)
	require.Equal(t, "FLAG_STATUS#pending", f.GSI2PK)
}
