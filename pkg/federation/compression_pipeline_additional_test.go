package federation

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type fakeCompressionFederationRepo struct {
	metrics []*models.FederationAnalyticsTimeSeries
	getErr  error

	storeErrOnce bool
	storeCalls   int
}

func (f *fakeCompressionFederationRepo) GetDetailedMetricsByPeriod(_ context.Context, period string, startTime, endTime time.Time, limit int) ([]*models.FederationAnalyticsTimeSeries, error) {
	_ = period
	_ = startTime
	_ = endTime
	_ = limit
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.metrics, nil
}

func (f *fakeCompressionFederationRepo) StoreDetailedFederationMetrics(_ context.Context, _ *models.FederationAnalyticsTimeSeries) error {
	f.storeCalls++
	if f.storeErrOnce {
		f.storeErrOnce = false
		return errors.New("store failed")
	}
	return nil
}

func TestCompressionPipeline_CompressOldData_And_CompressTimeSeriesData(t *testing.T) {
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	t.Run("NewCompressionPipeline_smoke", func(t *testing.T) {
		p := NewCompressionPipeline(nil, logger)
		require.NotNil(t, p)
	})

	t.Run("compressTimeSeriesData_no_metrics_is_noop", func(t *testing.T) {
		repo := &fakeCompressionFederationRepo{metrics: []*models.FederationAnalyticsTimeSeries{}}
		p := &CompressionPipeline{federationRepo: repo, logger: logger}

		err := p.compressTimeSeriesData(ctx, "5min", time.Now().Add(-24*time.Hour), "GZIP_JSON")
		require.NoError(t, err)
		assert.Equal(t, 0, repo.storeCalls)
	})

	t.Run("compressTimeSeriesData_compresses_and_continues_on_store_errors", func(t *testing.T) {
		metric := &models.FederationAnalyticsTimeSeries{
			Domain:      "example.com",
			Period:      "5min",
			Timestamp:   time.Now().Add(-25 * time.Hour),
			HealthScore: 0.9,
		}

		repo := &fakeCompressionFederationRepo{
			metrics:      []*models.FederationAnalyticsTimeSeries{metric},
			storeErrOnce: true,
			storeCalls:   0,
		}
		p := &CompressionPipeline{federationRepo: repo, logger: logger}

		err := p.compressTimeSeriesData(ctx, "5min", time.Now().Add(-24*time.Hour), "GZIP_JSON")
		require.NoError(t, err)
		assert.NotEmpty(t, metric.CompressedData)
		assert.Equal(t, 1, repo.storeCalls)
	})

	t.Run("CompressOldData_wraps_errors", func(t *testing.T) {
		repo := &fakeCompressionFederationRepo{getErr: errors.New("query failed")}
		p := &CompressionPipeline{federationRepo: repo, logger: logger}

		err := p.CompressOldData(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCompressionFailed)
	})

	t.Run("CompressOldData_success_path", func(t *testing.T) {
		repo := &fakeCompressionFederationRepo{metrics: []*models.FederationAnalyticsTimeSeries{}}
		p := &CompressionPipeline{federationRepo: repo, logger: logger}

		require.NoError(t, p.CompressOldData(ctx))
	})

	t.Run("archiveToS3_smoke", func(t *testing.T) {
		p := &CompressionPipeline{logger: logger}
		require.NoError(t, p.archiveToS3(ctx, time.Now().Add(-7*24*time.Hour)))
	})

	t.Run("compressTimeSeriesData_continues_on_compress_errors", func(t *testing.T) {
		metric := &models.FederationAnalyticsTimeSeries{
			Domain:      "example.com",
			Period:      "5min",
			Timestamp:   time.Now().Add(-25 * time.Hour),
			HealthScore: math.NaN(), // triggers json.Marshal error
		}

		repo := &fakeCompressionFederationRepo{
			metrics:    []*models.FederationAnalyticsTimeSeries{metric},
			storeCalls: 0,
		}
		p := &CompressionPipeline{federationRepo: repo, logger: logger}

		require.NoError(t, p.compressTimeSeriesData(ctx, "5min", time.Now().Add(-24*time.Hour), "GZIP_JSON"))
		assert.Equal(t, 0, repo.storeCalls)
	})
}
