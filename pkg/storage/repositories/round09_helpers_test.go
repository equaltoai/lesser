package repositories

import (
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func newRound09CostService() *cost.TrackingService {
	// Construct a cost tracking service that won't flush during unit tests.
	// CloudWatch client is nil; as long as we never flush, this stays local-only.
	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsBatchSize = 1_000_000
	cfg.MetricsFlushInterval = 24 * time.Hour
	cfg.EnableDetailedMetrics = false
	cfg.CloudWatchNamespace = "Lesser/Test"

	return cost.NewTrackingService(nil, zap.NewNop(), cfg)
}

func mockMatchedByType[T any]() interface{} {
	return mock.MatchedBy(func(v interface{}) bool {
		_, ok := v.(T)
		return ok
	})
}
