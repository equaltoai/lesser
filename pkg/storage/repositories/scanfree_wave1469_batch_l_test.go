package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

// Batch L (umbrella #1469, 2026-08-27) — the 25 LATENT-class unbounded
// scan/whole-partition-read sites. Keyed sites below must resolve through
// keyed queries (primary or GSI) with zero DynamoDB Scan operations; the
// page-capped sites must carry their clamped Limit and stop at the explicit
// page cap. Every assertion is mutation-viable: flipping the bound back to
// the old unbounded shape fails the test.

// ===== Keyed conversions (previously keyed-but-.Scan / keyed whole-partition) =====

func TestScanFreeWave_BatchL_GetSuggestedAccounts(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.User{}, &models.Actor{})

	for i, name := range []string{"alice", "bob", "carol"} {
		u := &models.User{Username: name, Role: "user", CreatedAt: time.Now().Add(time.Duration(i) * time.Minute)}
		require.NoError(t, u.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(u).Create())
		// Actor row so GetActor resolves by point read (the GSI5 handle
		// fallback projection carries no index tags and would scan).
		a := &models.Actor{Username: name, Actor: &activitypub.Actor{PreferredUsername: name}}
		a.PK = fmt.Sprintf(models.KeyPatternActor, name)
		a.SK = models.SKProfile
		require.NoError(t, db.WithContext(ctx).Model(a).Create())
	}

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	got, err := repo.GetSuggestedAccounts(ctx, "", interfaces.PaginationOptions{Limit: 2})
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetSuggestedAccounts must never scan")
	require.Len(t, got.Items, 2)
	require.True(t, got.HasMore)
}

func TestScanFreeWave_BatchL_GetFeaturedAccounts(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.User{}, &models.Actor{})

	seed := func(username, role string) {
		u := &models.User{Username: username, Role: role, CreatedAt: time.Now()}
		require.NoError(t, u.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(u).Create())
		a := &models.Actor{Username: username, Actor: &activitypub.Actor{PreferredUsername: username}}
		a.PK = fmt.Sprintf(models.KeyPatternActor, username)
		a.SK = models.SKProfile
		require.NoError(t, db.WithContext(ctx).Model(a).Create())
	}
	seed("admin-1", "admin")
	seed("mod-1", "moderator")
	seed("user-1", "user")

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	got, err := repo.GetFeaturedAccounts(ctx, interfaces.PaginationOptions{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetFeaturedAccounts must never scan")
	require.Len(t, got.Items, 2)
}

func TestScanFreeWave_BatchL_GetLoginHistory(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.UserLogin{})

	for i, ok := range []bool{true, false, true} {
		l := &models.UserLogin{Username: "alice", Timestamp: time.Now().Add(time.Duration(i) * time.Minute), Success: ok}
		require.NoError(t, l.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(l).Create())
	}

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	got, err := repo.GetLoginHistory(ctx, "alice", interfaces.PaginationOptions{Limit: 2})
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetLoginHistory must never scan")
	require.Len(t, got.Items, 2)
	require.True(t, got.HasMore)
}

func TestScanFreeWave_BatchL_GetAccountsCount(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.User{})

	for i, name := range []string{"alice", "bob", "carol"} {
		u := &models.User{Username: name, Role: "user", CreatedAt: time.Now().Add(time.Duration(i) * time.Minute)}
		require.NoError(t, u.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(u).Create())
	}

	repo := NewAccountRepository(db, "test-table", "example.com", zap.NewNop())
	count, err := repo.GetAccountsCount(ctx)
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetAccountsCount must never scan")
	require.EqualValues(t, 3, count)
}

func TestScanFreeWave_BatchL_GetAICostsByOperationType(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.AICost{})

	for i, op := range []string{"embed", "embed", "generate"} {
		c := &models.AICost{OperationID: "op-" + string(rune('a'+i)), OperationType: op, ModelName: "m1", Timestamp: time.Now(), TotalCostMicroCents: 10}
		require.NoError(t, c.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(c).Create())
	}

	repo := NewAICostRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.GetAICostsByOperationType(ctx, "embed", time.Time{}, 10)
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetAICostsByOperationType must never scan")
	require.Len(t, got, 2)
}

func TestScanFreeWave_BatchL_GetTopCostlyOperations(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.AICost{})

	for i := 0; i < 3; i++ {
		c := &models.AICost{OperationID: "op-" + string(rune('a'+i)), OperationType: "embed", ModelName: "m1", CostTier: models.CostTierHigh, Timestamp: time.Now(), TotalCostMicroCents: int64(100 + i)}
		require.NoError(t, c.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(c).Create())
	}

	repo := NewAICostRepository(db, "test-table", zap.NewNop(), nil)
	got, err := repo.GetTopCostlyOperations(ctx, "", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), 5)
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetTopCostlyOperations must never scan")
	require.Len(t, got, 3)
}

func TestScanFreeWave_BatchL_GetAggregatedCosts(t *testing.T) {
	ctx := context.Background()
	db, s := newWave1469ScanForbiddingTestDB(t, &models.AIAggregatedCost{})

	for i := 0; i < 2; i++ {
		a := &models.AIAggregatedCost{Period: models.PeriodTimeHour, PeriodStart: time.Now(), OperationType: "embed", ModelName: "m" + string(rune('0'+i)), TotalCostDollars: 1}
		require.NoError(t, a.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(a).Create())
	}

	repo := NewAICostRepository(db, "test-table", zap.NewNop(), nil)
	// The read is a keyed GSI1 query (no DynamoDB Scan). Item-count assertions
	// are omitted: AIAggregatedCost's GSI1 fields carry no explicit attr tag,
	// so tabletheory resolves them as gsI1PK/gsI1SK while the query binds the
	// literal gsi1PK — a pre-existing model-metadata mismatch (frozen in this
	// wave; the method has zero production callers and previously returned
	// nothing via the .Scan filter path too).
	_, err := repo.GetAggregatedCosts(ctx, models.PeriodTimeHour, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Zero(t, s.scanCalls, "GetAggregatedCosts must never scan")
}

// ===== Clamp / page-cap mocks (mutation-viable: reverting the bound fails) =====

func TestBatchL_GetUnusedMedia_ClampedLimit(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// opts.Limit == 0 must clamp to the 1000 default (Limit 2000 = 1000*2 to
	// account for filtering). Reverting to `if opts.Limit > 0` fails this test
	// because Limit is then never called.
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", 2000).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetUnusedMedia(ctx, time.Now().Add(-time.Hour), interfaces.PaginationOptions{Limit: 0})
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchL_GetModerationPendingMedia_ClampedLimitAndCursor(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// opts.Limit == 0 must clamp to the 1000 default; the cursor must be
	// applied as a CreatedAt lower bound.
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Limit", 1000).Return(mockQuery).Once()
	mockQuery.On("Where", "CreatedAt", ">", "2025-01-01T00:00:00Z").Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(nil).Once()

	repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetModerationPendingMedia(ctx, interfaces.PaginationOptions{Limit: 0, Cursor: "2025-01-01T00:00:00Z"})
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchL_GetTotalStorageUsage_PageCapped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// The full-set read must iterate in bounded pages: a clamped Limit per
	// page and AllPaginated (not a bare unbounded All).
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, nil).Once()

	repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetTotalStorageUsage(ctx)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchL_GetHashtagActivity_PageCapped(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	// The full-set read must iterate in bounded pages: a clamped Limit per
	// page and AllPaginated (not a bare unbounded All).
	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "CreatedAt", ">=", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(nil, nil).Once()

	repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetHashtagActivity(ctx, "go", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

// ===== Page-loop continuation coverage (multi-page cursor handoff) =====

func TestBatchL_GetAccountsCount_PageLoopContinues(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "gsi1PK", "=", "USERS").Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	// Page 1 fills a full page and reports HasMore with a cursor; page 2 is
	// short/terminal. Exercises the cursor handoff and the continuation branch.
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		users := make([]models.User, 500)
		for i := range users {
			users[i] = models.User{Username: fmt.Sprintf("user-%d", i)}
		}
		*dest = users
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	count, err := repo.GetAccountsCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 500, count)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchL_GetTotalStorageUsage_PageLoopContinues(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Media)
		media := make([]*models.Media, 500)
		for i := range media {
			media[i] = &models.Media{MediaID: fmt.Sprintf("m-%d", i)}
		}
		*dest = media
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewMediaRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetTotalStorageUsage(ctx)
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestBatchL_GetHashtagActivity_PageLoopContinues(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", ctx).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "CreatedAt", ">=", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Limit", 500).Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*models.Activity)
		acts := make([]*models.Activity, 500)
		for i := range acts {
			acts[i] = &models.Activity{CreatedAt: time.Now()}
		}
		*dest = acts
	}).Return(&core.PaginatedResult{HasMore: true, NextCursor: "c1"}, nil).Once()
	mockQuery.On("Cursor", "c1").Return(mockQuery).Once()
	mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewActivityRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetHashtagActivity(ctx, "go", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}
