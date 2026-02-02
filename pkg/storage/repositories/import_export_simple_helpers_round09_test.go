package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

type round09CostRecord struct {
	ts   time.Time
	cost int64
}

func (r round09CostRecord) GetTimestamp() time.Time { return r.ts }

func (r round09CostRecord) GetTotalCostMicroCents() int64 { return r.cost }

func TestRound09_ImportExportSimpleHelpers_FilteringAndPagination(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	ctx := context.Background()

	items, next, err := getImportExportItemsForUser(ctx, mockDB, logger, "user-1", 2, "", "export", true)
	require.NoError(t, err)
	require.NotNil(t, items)
	require.NotEmpty(t, next)

	items, next, err = getImportExportItemsForUser(ctx, mockDB, logger, "user-1", 2, baseTime.Format(time.RFC3339), "import", false)
	require.NoError(t, err)
	require.NotNil(t, items)
	require.NotEmpty(t, next)

	mockDBErr := new(mocks.MockDB)
	mockQueryErr := new(mocks.MockQuery)
	mockQueryErr.On("Scan", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBErr, mockQueryErr, nil, baseTime)
	_, _, err = getImportExportItemsForUser(ctx, mockDBErr, logger, "user-1", 2, "", "export", true)
	require.Error(t, err)

	all := []string{"a", "b"}
	require.Equal(t, all, filterItemsByStatus(all, nil, func(s string) string { return s }))
	require.Equal(t, []string{"a"}, filterItemsByStatus(all, []string{"a"}, func(s string) string { return s }))
}

func TestRound09_ImportExportSimpleHelpers_ByStatusAndCosts(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	baseTime := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

	ctx := context.Background()

	exports, err := getImportExportItemsByStatus[*models.Export](ctx, mockDB, logger, "user-1", []string{"completed"}, "export", &models.Export{})
	require.NoError(t, err)
	require.NotEmpty(t, exports)

	mockDBAllErr := new(mocks.MockDB)
	mockQueryAllErr := new(mocks.MockQuery)
	mockQueryAllErr.On("All", mock.Anything).Return(errors.New("boom")).Once()
	setupPermissiveRound08Mocks(mockDBAllErr, mockQueryAllErr, nil, baseTime)
	_, err = getImportExportItemsByStatus[*models.Import](ctx, mockDBAllErr, logger, "user-1", []string{"processing"}, "import", &models.Import{})
	require.Error(t, err)

	items := []round09CostRecord{
		{ts: baseTime.Add(3 * time.Hour), cost: 10},
		{ts: baseTime.Add(2 * time.Hour), cost: 200},
		{ts: baseTime.Add(1 * time.Hour), cost: 150},
	}
	filtered := filterHighCostOperations(items, 150, 2, func(i round09CostRecord) int64 { return i.cost })
	require.Len(t, filtered, 2)
	require.GreaterOrEqual(t, filtered[0].cost, int64(150))
}
