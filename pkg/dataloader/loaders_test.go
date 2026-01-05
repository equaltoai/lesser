package dataloader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDataLoader_Load_CachesByDefault(t *testing.T) {
	calls := 0
	batchFn := func(_ context.Context, keys []string) ([]string, []error) {
		calls++
		require.Len(t, keys, 1)
		return []string{"v:" + keys[0]}, []error{nil}
	}

	loader := NewDataLoader(batchFn, DefaultConfig(), zap.NewNop())
	ctx := context.Background()

	v1, err := loader.Load(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "v:a", v1)

	v2, err := loader.Load(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "v:a", v2)
	require.Equal(t, 1, calls)

	stats := loader.GetStats()
	require.Equal(t, int64(1), stats.Hits)
	require.Equal(t, int64(1), stats.Misses)
	require.Equal(t, int64(1), stats.Batches)
	require.Equal(t, 1, stats.CacheSize)
}

func TestDataLoader_LoadMany_BatchesWhenLarge(t *testing.T) {
	calls := 0
	batchFn := func(_ context.Context, keys []string) ([]string, []error) {
		calls++
		results := make([]string, len(keys))
		errs := make([]error, len(keys))
		for i, k := range keys {
			results[i] = "v:" + k
			errs[i] = nil
		}
		return results, errs
	}

	loader := NewDataLoader(batchFn, DefaultConfig(), zap.NewNop())
	ctx := context.Background()

	keys := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	results, errs := loader.LoadMany(ctx, keys)
	require.Len(t, results, len(keys))
	require.Len(t, errs, len(keys))
	require.Equal(t, 1, calls)

	for i := range keys {
		require.NoError(t, errs[i])
		require.Equal(t, "v:"+keys[i], results[i])
	}
}

func TestDataLoader_PrimeAndClear(t *testing.T) {
	calls := 0
	batchFn := func(_ context.Context, keys []string) ([]string, []error) {
		calls++
		return []string{"loaded:" + keys[0]}, []error{nil}
	}

	loader := NewDataLoader(batchFn, DefaultConfig(), zap.NewNop())
	loader.Prime("a", "primed:a", nil)

	v, err := loader.Load(context.Background(), "a")
	require.NoError(t, err)
	require.Equal(t, "primed:a", v)
	require.Equal(t, 0, calls)

	loader.Clear("a")
	v, err = loader.Load(context.Background(), "a")
	require.NoError(t, err)
	require.Equal(t, "loaded:a", v)
	require.Equal(t, 1, calls)
}
