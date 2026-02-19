package dataloader

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestDataLoader_NewDataLoader_DefaultsZeroConfigAndNilLogger(t *testing.T) {
	loader := NewDataLoader(func(_ context.Context, _ []string) ([]string, []error) {
		return []string{}, []error{}
	}, Config{}, nil)

	require.Equal(t, 1*time.Millisecond, loader.wait)
	require.Equal(t, 100, loader.maxBatch)
	require.Equal(t, 5*time.Minute, loader.cacheExpiry)
	require.NotNil(t, loader.logger)
}

func TestDataLoader_Load_ExpiresCacheEntries(t *testing.T) {
	calls := 0
	batchFn := func(_ context.Context, keys []string) ([]string, []error) {
		calls++
		return []string{"v:" + keys[0]}, []error{nil}
	}

	cfg := DefaultConfig()
	cfg.CacheExpiry = -1

	loader := NewDataLoader(batchFn, cfg, zap.NewNop())
	ctx := context.Background()

	_, err := loader.Load(ctx, "a")
	require.NoError(t, err)
	_, err = loader.Load(ctx, "a")
	require.NoError(t, err)

	require.Equal(t, 2, calls)

	stats := loader.GetStats()
	require.Equal(t, int64(0), stats.Hits)
	require.Equal(t, int64(2), stats.Misses)
	require.Equal(t, int64(2), stats.Batches)
}

func TestDataLoader_Load_ReturnsBatchErrors(t *testing.T) {
	loader := NewDataLoader(func(_ context.Context, _ []string) ([]string, []error) {
		return []string{""}, []error{errors.New("boom")}
	}, DefaultConfig(), zap.NewNop())

	_, err := loader.Load(context.Background(), "a")
	require.Error(t, err)
}

func TestDataLoader_LoadMany_UsesPerKeyLoadForSmallRequests(t *testing.T) {
	calls := 0
	batchFn := func(_ context.Context, keys []string) ([]string, []error) {
		calls++
		return []string{"v:" + keys[0]}, []error{nil}
	}

	loader := NewDataLoader(batchFn, DefaultConfig(), zap.NewNop())

	results, errs := loader.LoadMany(context.Background(), []string{"a", "b"})
	require.Len(t, results, 2)
	require.Len(t, errs, 2)
	require.Equal(t, 2, calls)
	require.Equal(t, "v:a", results[0])
	require.Equal(t, "v:b", results[1])
}

func TestDataLoader_ClearAll_NoopsWhenCacheDisabled(t *testing.T) {
	batchFn := func(_ context.Context, keys []string) ([]string, []error) {
		return []string{"v:" + keys[0]}, []error{nil}
	}

	cfg := DefaultConfig()
	cfg.Cache = false

	loader := NewDataLoader(batchFn, cfg, zap.NewNop())
	loader.ClearAll()
	loader.Clear("a")
	loader.Prime("a", "primed", nil)
}

func TestDataLoader_ClearAll_ClearsCacheWhenEnabled(t *testing.T) {
	loader := NewDataLoader(func(_ context.Context, keys []string) ([]string, []error) {
		return []string{"v:" + keys[0]}, []error{nil}
	}, DefaultConfig(), zap.NewNop())

	loader.Prime("a", "primed:a", nil)
	loader.Prime("b", "primed:b", nil)
	require.Equal(t, 2, loader.GetStats().CacheSize)

	loader.ClearAll()
	require.Equal(t, 0, loader.GetStats().CacheSize)
}

func TestDataLoader_LoadMany_BatchesWithoutCache(t *testing.T) {
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

	cfg := DefaultConfig()
	cfg.Cache = false

	loader := NewDataLoader(batchFn, cfg, zap.NewNop())
	results, errs := loader.LoadMany(context.Background(), []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"})
	require.Len(t, results, 11)
	require.Len(t, errs, 11)
	require.Equal(t, 1, calls)
}

func TestDataLoader_LoadMany_UsesCachedEntriesWhenEnabled(t *testing.T) {
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
	loader.Prime("a", "cached:a", nil)

	keys := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}
	results, errs := loader.LoadMany(context.Background(), keys)
	require.Len(t, results, len(keys))
	require.Len(t, errs, len(keys))
	require.Equal(t, 1, calls)
	require.Equal(t, "cached:a", results[0])
}
