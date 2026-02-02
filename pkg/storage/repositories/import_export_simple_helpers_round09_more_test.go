package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestRound09_ImportExportSimpleHelpers_CostQueries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	call := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]round09CostRecord)
		if !ok {
			return
		}
		call++
		switch call {
		case 1:
			*ptr = append(*ptr, round09CostRecord{ts: start.Add(1 * time.Hour), cost: 10})
		default:
			*ptr = append(*ptr, round09CostRecord{ts: end.Add(1 * time.Hour), cost: 20})
		}
	}).Return(nil).Maybe()

	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now().UTC())

	costs, err := getCostsByDateRange[round09CostRecord](ctx, mockDB, logger, start, end, 10, "export", round09CostRecord{})
	require.NoError(t, err)
	require.Len(t, costs, 2)
	require.True(t, costs[0].ts.After(costs[1].ts))

	mockDBErr := new(mocks.MockDB)
	mockQueryErr := new(mocks.MockQuery)
	mockQueryErr.On("All", mock.Anything).Return(errors.New("boom")).Once()
	mockQueryErr.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]round09CostRecord)
		if !ok {
			return
		}
		*ptr = append(*ptr, round09CostRecord{ts: end.Add(1 * time.Hour), cost: 20})
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, time.Now().UTC())

	costs, err = getCostsByDateRange[round09CostRecord](ctx, mockDBErr, logger, start, end, 1, "import", round09CostRecord{})
	require.NoError(t, err)
	require.Len(t, costs, 1)

	mockDBUserErrExport := new(mocks.MockDB)
	mockQueryUserErrExport := new(mocks.MockQuery)
	mockQueryUserErrExport.On("All", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBUserErrExport, mockQueryUserErrExport, nil, time.Now().UTC())

	_, err = getUserCosts[round09CostRecord](ctx, mockDBUserErrExport, logger, "user-1", start, end, 10, "export", round09CostRecord{})
	require.Error(t, err)

	mockDBUserErrImport := new(mocks.MockDB)
	mockQueryUserErrImport := new(mocks.MockQuery)
	mockQueryUserErrImport.On("All", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBUserErrImport, mockQueryUserErrImport, nil, time.Now().UTC())

	_, err = getUserCosts[round09CostRecord](ctx, mockDBUserErrImport, logger, "user-1", start, end, 10, "import", round09CostRecord{})
	require.Error(t, err)
}

func TestRound09_ImportExportSimpleHelpers_HighCostOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		ptr, ok := args.Get(0).(*[]round09CostRecord)
		if !ok {
			return
		}
		*ptr = append(*ptr,
			round09CostRecord{ts: start.Add(2 * time.Hour), cost: 500},
			round09CostRecord{ts: start.Add(1 * time.Hour), cost: 10},
		)
	}).Return(nil).Once()
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, time.Now().UTC())

	ops, err := getHighCostOperations[round09CostRecord](ctx, mockDB, logger, 100, start, end, 1, "export", round09CostRecord{})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	require.GreaterOrEqual(t, ops[0].cost, int64(100))
}
