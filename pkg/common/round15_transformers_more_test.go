package common

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTransformers_MoreCoverage(t *testing.T) {
	t.Run("base transformer with nil function returns TransformationError with unwrap", func(t *testing.T) {
		tr := NewBaseTransformer[SourceData, TargetData]("nil_transformer", nil, zap.NewNop())
		_, err := tr.Transform(context.Background(), SourceData{ID: "1"})
		require.Error(t, err)

		var te TransformationError
		require.ErrorAs(t, err, &te)
		assert.Equal(t, "nil_transformer", te.Step)
		assert.NotNil(t, te.Cause)

		assert.NotEmpty(t, err.Error())
		assert.NotNil(t, stdErrors.Unwrap(err))
	})

	t.Run("batch transformer concurrent path", func(t *testing.T) {
		bt := NewBatchTransformer(
			"concurrent_batch",
			func(_ context.Context, source SourceData) (TargetData, error) {
				return TargetData{Identifier: source.ID, Processed: true}, nil
			},
			nil,
			zap.NewNop(),
		)
		bt.concurrent = true

		sources := []SourceData{
			{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"}, {ID: "6"},
		}
		out, err := bt.TransformBatch(context.Background(), sources)
		require.NoError(t, err)
		require.Len(t, out, len(sources))
		for i := range sources {
			assert.Equal(t, sources[i].ID, out[i].Identifier)
		}
	})

	t.Run("batch transformer concurrent path stops on first error", func(t *testing.T) {
		bt := NewBatchTransformer(
			"concurrent_batch_err",
			func(_ context.Context, source SourceData) (TargetData, error) {
				if source.ID == "bad" {
					return TargetData{}, stdErrors.New("boom")
				}
				return TargetData{Identifier: source.ID}, nil
			},
			nil,
			zap.NewNop(),
		)
		bt.concurrent = true

		_, err := bt.TransformBatch(context.Background(), []SourceData{
			{ID: "1"}, {ID: "2"}, {ID: "bad"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
		})
		require.Error(t, err)
	})

	t.Run("cache stats and clear", func(t *testing.T) {
		cache := NewTransformationCache[string, int](time.Hour, 10, func(s string) string { return s }, zap.NewNop())

		cache.Set("a", 1)
		_, ok := cache.Get("a")
		require.True(t, ok) // hit

		_, ok = cache.Get("missing")
		require.False(t, ok) // miss

		hits, misses, hitRate := cache.Stats()
		assert.Equal(t, int64(1), hits)
		assert.Equal(t, int64(1), misses)
		assert.Equal(t, 0.5, hitRate)

		cache.Clear()
		_, ok = cache.Get("a")
		assert.False(t, ok)
	})

	t.Run("cache ttl expiry and nil keyFunc", func(t *testing.T) {
		cache := NewTransformationCache[string, int](time.Millisecond, 10, func(s string) string { return s }, zap.NewNop())
		cache.cache.Store("a", CacheEntry[int]{Value: 1, CreatedAt: time.Now().Add(-time.Hour)})
		_, ok := cache.Get("a")
		assert.False(t, ok)

		nilCache := NewTransformationCache[string, int](time.Hour, 10, nil, zap.NewNop())
		_, ok = nilCache.Get("a")
		assert.False(t, ok)
		nilCache.Set("a", 1)
	})

	t.Run("conditional transformer can transform when condition is nil", func(t *testing.T) {
		ct := NewConditionalTransformer(
			"nil_condition",
			func(_ context.Context, source string) (string, error) { return source + "_ok", nil },
			nil,
			zap.NewNop(),
		)

		assert.True(t, ct.CanTransform(context.Background(), "x"))
		out, err := ct.Transform(context.Background(), "x")
		require.NoError(t, err)
		assert.Equal(t, "x_ok", out)
	})

	t.Run("metrics reset", func(t *testing.T) {
		metrics := NewTransformationMetrics()
		metrics.RecordTransformation(time.Millisecond, true)
		metrics.RecordTransformation(time.Millisecond, false)

		metrics.Reset()
		count, errs, avg, rate := metrics.GetStats()
		assert.Equal(t, int64(0), count)
		assert.Equal(t, int64(0), errs)
		assert.Equal(t, time.Duration(0), avg)
		assert.Equal(t, 0.0, rate)
	})

	t.Run("registry uses a nop logger when nil", func(t *testing.T) {
		reg := NewTransformationRegistry(nil)
		require.NoError(t, reg.Register("x", struct{}{}))
	})
}
