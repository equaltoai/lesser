package streaming

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cloudwatchTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMetricsTracker_SessionLifecycle(t *testing.T) {
	tracker := NewMetricsTracker(nil, nil) // covers nil-logger handling

	tracker.StartSession("s1", "u1", "m1")

	metricsAny, ok := tracker.sessionMetrics.Load("s1")
	require.True(t, ok)
	metrics := metricsAny.(*SessionMetrics)

	// Make QoE calculations meaningful.
	metrics.StartTime = time.Now().Add(-2 * time.Minute)
	metrics.CurrentQuality = Quality480p
	metrics.LastQualityChange = time.Now().Add(-10 * time.Second)

	tracker.TrackQualitySwitch("s1", Quality480p, Quality720p)
	tracker.TrackQualitySwitch("s1", Quality720p, Quality1080p)

	tracker.TrackRebufferEvent("s1", 500*time.Millisecond)
	tracker.TrackSegmentLoad("s1", 10, 200*time.Millisecond, 1234)
	tracker.TrackSegmentFailure("s1", 11, "timeout")

	for i := 0; i < 101; i++ {
		tracker.TrackBufferHealth("s1", time.Duration(i)*time.Second, 1000+i)
	}

	copy1 := tracker.GetSessionMetrics("s1")
	require.NotNil(t, copy1)
	assert.Equal(t, metrics.SessionID, copy1.SessionID)
	require.Len(t, copy1.BufferHealthHistory, 100) // oldest evicted

	// Ensure GetSessionMetrics returns a safe copy of the map.
	copy1.TimeInEachQuality[Quality4K] = time.Hour
	after := tracker.GetSessionMetrics("s1")
	require.NotNil(t, after)
	_, ok = after.TimeInEachQuality[Quality4K]
	assert.False(t, ok)

	final := tracker.EndSession("s1")
	require.NotNil(t, final)
	assert.GreaterOrEqual(t, final.SegmentSuccessRate, 0.0)
	assert.LessOrEqual(t, final.QoEScore, 1.0)

	assert.Nil(t, tracker.GetSessionMetrics("s1"))
}

func TestMetricsTracker_Cleanup(t *testing.T) {
	tracker := NewMetricsTracker(nil, zap.NewNop())

	tracker.StartSession("s1", "u1", "m1")
	tracker.StartSession("s2", "u2", "m2")

	metricsAny, _ := tracker.sessionMetrics.Load("s1")
	metricsAny.(*SessionMetrics).LastUpdate = time.Now().Add(-2 * time.Hour)

	tracker.Cleanup(time.Hour)

	assert.Nil(t, tracker.GetSessionMetrics("s1"))
	assert.NotNil(t, tracker.GetSessionMetrics("s2"))
}

func TestMetricsTracker_publishBatch_SuccessAndError(t *testing.T) {
	rec := &cloudWatchRecorder{}
	srv := newTestCloudWatchServer(t, rec)
	t.Cleanup(srv.Close)

	tracker := &MetricsTracker{
		cloudWatch:    newTestCloudWatchClient(srv.URL),
		logger:        zap.NewNop(),
		namespace:     "Lesser/Streaming",
		batchSize:     20,
		batchInterval: time.Hour,
		lastPublish:   time.Now().Add(-time.Hour),
	}

	// Success
	tracker.addToBatch(cloudwatchTypes.MetricDatum{
		MetricName: aws.String("Event"),
		Timestamp:  aws.Time(time.Now()),
		Value:      aws.Float64(1),
		Unit:       cloudwatchTypes.StandardUnitCount,
	})
	tracker.publishBatch()

	rec.mu.Lock()
	putCalls := rec.putCalls
	rec.mu.Unlock()
	assert.Equal(t, 1, putCalls)

	// Error with retry (re-add every other metric)
	rec.mu.Lock()
	rec.putShouldError = true
	rec.mu.Unlock()

	for i := 0; i < 4; i++ {
		tracker.addToBatch(cloudwatchTypes.MetricDatum{
			MetricName: aws.String("Event"),
			Timestamp:  aws.Time(time.Now()),
			Value:      aws.Float64(float64(i + 1)),
			Unit:       cloudwatchTypes.StandardUnitCount,
		})
	}
	tracker.publishBatch()

	tracker.batchMutex.RLock()
	batchLen := len(tracker.metricsBatch)
	tracker.batchMutex.RUnlock()
	assert.Equal(t, 2, batchLen)
}

func TestMetricsTracker_addToBatch_NoCloudWatch(t *testing.T) {
	tracker := &MetricsTracker{cloudWatch: nil}
	tracker.addToBatch(cloudwatchTypes.MetricDatum{MetricName: aws.String("Event")})

	tracker.batchMutex.RLock()
	defer tracker.batchMutex.RUnlock()
	assert.Empty(t, tracker.metricsBatch)
}

