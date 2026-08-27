package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

// TestGetPopularSearchQueries_WindowSemantics pins the caller-visible window
// semantics of GetPopularSearchQueries: scorePopularQueries (the sole caller)
// passes a 7-day window and expects counts aggregated over exactly the raw
// SearchQuery rows inside that window — out-of-window rows must not count, and
// a query with no in-window rows must not be returned.
//
// Wave part 2 batch E rework (#1469): this read deliberately stays on the
// baselined scan over raw SearchQuery rows. The GSI8 PopularQueryCounter
// delegation was reverted because the counter's GSI8 partition key re-points
// on every increment (PopularQueryCounter.UpdateKeys re-keys GSI8PK from the
// Date of the last write), so only today's partition is populated and it
// cannot answer a 7-day window — no per-day partitions exist to aggregate.
func TestGetPopularSearchQueries_WindowSemantics(t *testing.T) {
	ctx := context.Background()
	f := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, f)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.SearchQuery{}))

	now := time.Now().UTC()
	seed := func(userID, query string, searchedAt time.Time, resultCount int) {
		row := &models.SearchQuery{
			Query:       query,
			UserID:      userID,
			ResultCount: resultCount,
			SearchedAt:  searchedAt,
		}
		require.NoError(t, row.UpdateKeys())
		require.NoError(t, db.WithContext(ctx).Model(row).Create())
	}

	// "golang": 3 in-window rows (2 users), 1 out-of-window row.
	seed("user-a", "golang", now.Add(-1*time.Hour), 10)
	seed("user-a", "golang", now.Add(-2*time.Hour), 20)
	seed("user-b", "golang", now.Add(-3*time.Hour), 30)
	seed("user-a", "golang", now.Add(-8*24*time.Hour), 1000) // outside the window
	// "python": 1 in-window row, 2 out-of-window rows.
	seed("user-c", "python", now.Add(-4*time.Hour), 5)
	seed("user-c", "python", now.Add(-8*24*time.Hour), 500)
	seed("user-c", "python", now.Add(-9*24*time.Hour), 600)
	// "rust": only out-of-window rows — must not appear.
	seed("user-d", "rust", now.Add(-10*24*time.Hour), 1)

	repo := NewTrendingRepository(db, zap.NewNop(), nil)
	results, err := repo.GetPopularSearchQueries(ctx, 10, 7*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, results, 2, "rust has no in-window rows and must be absent")

	require.Equal(t, "golang", results[0].Query, "highest in-window count sorts first")
	require.Equal(t, 3, results[0].Count, "only the 3 in-window golang rows count")
	require.Equal(t, 2, results[0].UserCount, "distinct in-window users")
	require.InDelta(t, float64(20), results[0].AvgResults, 0.001, "mean of in-window ResultCount")
	require.WithinDuration(t, now.Add(-1*time.Hour), results[0].LastUsed, time.Minute, "LastUsed is the max in-window SearchedAt")

	require.Equal(t, "python", results[1].Query)
	require.Equal(t, 1, results[1].Count, "out-of-window python rows must not count")
	require.Equal(t, 1, results[1].UserCount)

	// Limit applies after aggregation.
	limited, err := repo.GetPopularSearchQueries(ctx, 1, 7*24*time.Hour)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	require.Equal(t, "golang", limited[0].Query)
}
