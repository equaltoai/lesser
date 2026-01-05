package streaming

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type noopCostTracker struct{}

func (noopCostTracker) TrackDynamoRead(_ int)  {}
func (noopCostTracker) TrackDynamoWrite(_ int) {}

func TestBandwidthTracker_updateCache_KbpsAndMovingAverage(t *testing.T) {
	bt := &BandwidthTracker{
		cacheTTL: 5 * time.Minute,
	}

	now := time.Now()
	bt.updateCache("u1", 0, now)
	bt.updateCache("u1", 1_000_000, now.Add(1*time.Second)) // 8,000 kbps
	bt.updateCache("u1", 500_000, now.Add(2*time.Second))   // 4,000 kbps -> avg = 6,000 kbps

	cached, ok := bt.sessionCache.Load("u1")
	require.True(t, ok)
	stats := cached.(*cachedBandwidthStats)

	assert.Equal(t, int64(1_500_000), stats.TotalBytes)
	assert.Equal(t, int64(1_500_000), stats.SessionBytes)
	assert.Equal(t, 6000, stats.AverageBandwidth)
	assert.Equal(t, 8000, stats.PeakBandwidth)
}

func TestBandwidthTracker_GetBandwidthStats_CacheAndDefault(t *testing.T) {
	bt := &BandwidthTracker{
		cacheTTL: 5 * time.Minute,
	}

	// Default (no cache)
	got, err := bt.GetBandwidthStats(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 5000, got.AverageBandwidth)

	// Cached
	bt.updateCache("u1", 0, time.Now())
	cachedAny, ok := bt.sessionCache.Load("u1")
	require.True(t, ok)
	cached := cachedAny.(*cachedBandwidthStats)
	cached.AverageBandwidth = 9000
	cached.lastUpdate = time.Now()
	bt.sessionCache.Store("u1", cached)

	got, err = bt.GetBandwidthStats(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 9000, got.AverageBandwidth)

	// Expired cache falls back to default
	cached.lastUpdate = time.Now().Add(-10 * time.Minute)
	bt.sessionCache.Store("u1", cached)
	got, err = bt.GetBandwidthStats(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 5000, got.AverageBandwidth)
}

func TestBandwidthTracker_GetOptimalQuality(t *testing.T) {
	bt := &BandwidthTracker{cacheTTL: time.Minute}

	assert.Equal(t, Quality4K, bt.GetOptimalQuality(context.Background(), "u1", 30000))
	assert.Equal(t, Quality1080p, bt.GetOptimalQuality(context.Background(), "u1", 9000))
	assert.Equal(t, Quality720p, bt.GetOptimalQuality(context.Background(), "u1", 5000))
	assert.Equal(t, Quality480p, bt.GetOptimalQuality(context.Background(), "u1", 2000))
	assert.Equal(t, Quality360p, bt.GetOptimalQuality(context.Background(), "u1", 600))
	assert.Equal(t, Quality240p, bt.GetOptimalQuality(context.Background(), "u1", 100))

	bt.sessionCache.Store("u2", &cachedBandwidthStats{BandwidthStats: BandwidthStats{UserID: "u2", AverageBandwidth: 8000}, lastUpdate: time.Now()})
	assert.Equal(t, Quality720p, bt.GetOptimalQuality(context.Background(), "u2", 0))
}

func TestBandwidthTracker_TrackBandwidth_DoesNotFailStreaming(t *testing.T) {
	db := newFakeDynamormDB()
	db.forceCreateErr = errors.New("create boom")

	trendingRepo := repositories.NewTrendingRepository(db, zap.NewNop(), nil)
	storage := &MockAnalytics{analyticsRepo: trendingRepo}

	bt := NewBandwidthTracker(storage, zap.NewNop(), noopCostTracker{}, nil)
	require.NoError(t, bt.TrackBandwidth(context.Background(), "u1", 123))
}

func TestBandwidthTracker_GetBandwidthHistory_SuccessAndNilCloudWatch(t *testing.T) {
	bt := &BandwidthTracker{
		logger:    zap.NewNop(),
		namespace: "Lesser/Streaming/Bandwidth",
	}

	history, err := bt.GetBandwidthHistory(context.Background(), "u1", time.Minute)
	require.NoError(t, err)
	assert.Empty(t, history)

	rec := &cloudWatchRecorder{}
	srv := newTestCloudWatchServer(t, rec)
	t.Cleanup(srv.Close)

	bt.cloudWatch = newTestCloudWatchClient(srv.URL)
	history, err = bt.GetBandwidthHistory(context.Background(), "u1", time.Minute)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "u1", history[0].UserID)
	assert.Equal(t, 8, history[0].Bandwidth)
	assert.False(t, history[0].Timestamp.IsZero())
}

func TestBandwidthTracker_publishBandwidthMetric_Async(t *testing.T) {
	rec := &cloudWatchRecorder{}
	srv := newTestCloudWatchServer(t, rec)
	t.Cleanup(srv.Close)

	bt := &BandwidthTracker{
		logger:     zap.NewNop(),
		cloudWatch: newTestCloudWatchClient(srv.URL),
		namespace:  "Lesser/Streaming/Bandwidth",
		cacheTTL:   time.Minute,
	}

	// Seed cache so we publish BandwidthKbps as well.
	bt.sessionCache.Store("u1", &cachedBandwidthStats{
		BandwidthStats: BandwidthStats{UserID: "u1", AverageBandwidth: 1234},
		lastUpdate:     time.Now(),
	})

	bt.publishBandwidthMetric(context.Background(), "u1", 999, time.Now())

	require.Eventually(t, func() bool {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		return rec.putCalls > 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestBandwidthTracker_RecordBandwidthMeasurement_UpdatesCache(t *testing.T) {
	bt := &BandwidthTracker{cacheTTL: time.Minute}

	now := time.Now()
	require.NoError(t, bt.RecordBandwidthMeasurement(context.Background(), "u1", 4000))

	cachedAny, ok := bt.sessionCache.Load("u1")
	require.True(t, ok)
	cached := cachedAny.(*cachedBandwidthStats)
	assert.Equal(t, "u1", cached.UserID)
	assert.False(t, cached.LastMeasurement.Before(now.Add(-time.Second)))
}

func TestBandwidthTracker_publishBandwidthMetric_NoCloudWatch(t *testing.T) {
	bt := &BandwidthTracker{
		logger: zap.NewNop(),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		bt.publishBandwidthMetric(context.Background(), "u1", 1, time.Now())
	}()
	wg.Wait()
}

