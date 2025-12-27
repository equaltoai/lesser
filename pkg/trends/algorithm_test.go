package trends

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultAlgorithm_Calculate_SortsByScoreDesc(t *testing.T) {
	algo := NewDefaultAlgorithm()
	anchor := time.Now().Add(-1 * time.Hour)

	items := []TrendItem{
		{ID: "low", UsageCount: 5, LastUsed: anchor},
		{ID: "high", UsageCount: 10, LastUsed: anchor},
	}

	scores := algo.Calculate(items)
	require.Len(t, scores, 2)
	require.Equal(t, "high", scores[0].Item.ID)
	require.Greater(t, scores[0].Score, scores[1].Score)
}
