// Package main implements the trend-aggregator Lambda function for aggregating trending content and hashtags.
package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
)

type trendingRepository interface {
	GetRecentHashtags(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingHashtag, error)
	GetRecentStatusesWithEngagement(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingStatus, error)
	GetRecentLinks(ctx context.Context, since time.Time, limit int) ([]*storage.TrendingLink, error)
	StoreHashtagTrend(ctx context.Context, trend any) error
	StoreStatusTrend(ctx context.Context, trend any) error
	StoreLinkTrend(ctx context.Context, trend any) error
	DeleteOldHashtagTrends(ctx context.Context, before time.Time) error
	DeleteOldStatusTrends(ctx context.Context, before time.Time) error
	DeleteOldLinkTrends(ctx context.Context, before time.Time) error
}

// TrendAggregatorHandler runs scheduled trend aggregation.
type TrendAggregatorHandler struct {
	db           core.DB
	trendingRepo trendingRepository
	logger       *zap.Logger
}

// NewTrendAggregatorHandler creates a new trend aggregator handler
func NewTrendAggregatorHandler(db core.DB, logger *zap.Logger) *TrendAggregatorHandler {
	return &TrendAggregatorHandler{
		db:           db,
		trendingRepo: repositories.NewTrendingRepository(db, logger, nil),
		logger:       logger,
	}
}

// HandleScheduledEvent runs the daily aggregation task (invoked by EventBridge schedules).
func (h *TrendAggregatorHandler) HandleScheduledEvent(ctx *apptheory.EventContext, _ events.EventBridgeEvent) (any, error) {
	start := time.Now()
	requestID := ""
	if ctx != nil {
		requestID = ctx.RequestID
	}
	runCtx := context.Background()
	if ctx != nil {
		runCtx = ctx.Context()
	}

	h.logger.Info("starting trend aggregation",
		zap.String("request_id", requestID),
	)

	// Default to daily aggregation for scheduled events
	// The time range is configured in the EventBridge rule
	since := time.Now().Add(-24 * time.Hour)
	timeRange := "day"

	// Process different types of trends
	processedCount := 0

	// 1. Aggregate hashtag trends
	hashtagCount, err := h.aggregateHashtagTrends(runCtx, since)
	if err != nil {
		h.logger.Error("error aggregating hashtag trends",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
	} else {
		processedCount += hashtagCount
		h.logger.Info("aggregated hashtag trends",
			zap.String("request_id", requestID),
			zap.Int("count", hashtagCount),
		)
	}

	// 2. Aggregate status trends
	statusCount, err := h.aggregateStatusTrends(runCtx, since)
	if err != nil {
		h.logger.Error("error aggregating status trends",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
	} else {
		processedCount += statusCount
		h.logger.Info("aggregated status trends",
			zap.String("request_id", requestID),
			zap.Int("count", statusCount),
		)
	}

	// 3. Aggregate link trends
	linkCount, err := h.aggregateLinkTrends(runCtx, since)
	if err != nil {
		h.logger.Error("error aggregating link trends",
			zap.String("request_id", requestID),
			zap.Error(err),
		)
	} else {
		processedCount += linkCount
		h.logger.Info("aggregated link trends",
			zap.String("request_id", requestID),
			zap.Int("count", linkCount),
		)
	}

	// Clean up old trend data
	h.cleanupOldTrends(runCtx)

	duration := time.Since(start)
	h.logger.Info("completed trend aggregation",
		zap.String("request_id", requestID),
		zap.Int("processed_items", processedCount),
		zap.String("time_range", timeRange),
		zap.Duration("duration", duration),
	)

	return nil, nil
}

// aggregateHashtagTrends processes hashtag usage and updates trending scores
func (h *TrendAggregatorHandler) aggregateHashtagTrends(ctx context.Context, since time.Time) (int, error) {
	// 1. Get recent hashtag usage from repository
	hashtags, err := h.trendingRepo.GetRecentHashtags(ctx, since, 1000)
	if err != nil {
		return 0, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get recent hashtags")
	}

	h.logger.Debug("retrieved recent hashtags for aggregation",
		zap.Int("hashtag_count", len(hashtags)),
		zap.Time("since", since),
	)

	// 2. Count unique users and total usage
	hashtagStats := make(map[string]*HashtagTrendData)
	for _, hashtag := range hashtags {
		// Extract user ID from hashtag data (assuming it's in the struct)
		userID := hashtag.UserID // This field might need adjustment based on actual struct
		if err := common.ValidateRequiredParam("userID", userID); err != nil {
			continue // Skip if no user ID
		}

		if stats, exists := hashtagStats[hashtag.Name]; exists {
			stats.TotalUses++
			stats.UniqueUsers[userID] = true
		} else {
			hashtagStats[hashtag.Name] = &HashtagTrendData{
				Name:        hashtag.Name,
				TotalUses:   1,
				UniqueUsers: map[string]bool{userID: true},
				FirstSeen:   hashtag.FirstSeen,
				LastSeen:    hashtag.LastUsed,
			}
		}
		if hashtag.LastUsed.After(hashtagStats[hashtag.Name].LastSeen) {
			hashtagStats[hashtag.Name].LastSeen = hashtag.LastUsed
		}
	}

	// 3. Calculate trend scores and store trending hashtags
	trendingCount := 0
	for name, stats := range hashtagStats {
		// Calculate trend score based on velocity and engagement
		uniqueUserCount := len(stats.UniqueUsers)
		timeSpan := stats.LastSeen.Sub(stats.FirstSeen).Hours()
		if timeSpan == 0 {
			timeSpan = 1 // Prevent division by zero
		}
		velocity := float64(stats.TotalUses) / timeSpan
		trendScore := velocity * float64(uniqueUserCount) * 100

		// Only store if meets minimum threshold
		if trendScore > 10.0 && stats.TotalUses > 3 {
			// Create trend record using storage interface format for compatibility
			trend := &HashtagTrendStorage{
				Name:          name,
				UsageCount:    int64(stats.TotalUses),
				UniqueUsers:   int64(uniqueUserCount),
				FirstSeen:     stats.FirstSeen,
				LastUsed:      stats.LastSeen,
				TrendingScore: trendScore,
				Velocity:      velocity,
			}

			if err := h.trendingRepo.StoreHashtagTrend(ctx, trend); err != nil {
				h.logger.Error("failed to store hashtag trend",
					zap.String("hashtag", name),
					zap.Error(err),
				)
			} else {
				trendingCount++
			}
		}
	}

	h.logger.Debug("processed hashtag trends",
		zap.Int("total_hashtags", len(hashtagStats)),
		zap.Int("trending_count", trendingCount),
	)

	return trendingCount, nil
}

// aggregateStatusTrends processes status engagement and updates trending scores
func (h *TrendAggregatorHandler) aggregateStatusTrends(ctx context.Context, since time.Time) (int, error) {
	// 1. Get recent statuses with engagement from repository
	statuses, err := h.trendingRepo.GetRecentStatusesWithEngagement(ctx, since, 1000)
	if err != nil {
		return 0, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get recent statuses")
	}

	h.logger.Debug("retrieved recent statuses for aggregation",
		zap.Int("status_count", len(statuses)),
		zap.Time("since", since),
	)

	trendingCount := 0
	for _, status := range statuses {
		// 2. Use a default trust score for simplicity
		// In a full implementation, you'd get this from a user repository
		authorTrust := 1.0 // Default trust score

		// 3. Calculate trend scores based on engagement velocity
		age := time.Since(status.CreatedAt).Hours()
		if age <= 0 {
			age = 0.1 // Prevent division by zero
		}

		// Weighted engagement score
		engagementScore := (status.Likes * 1) + (status.Boosts * 2) + (status.Replies * 3)
		velocity := float64(engagementScore) / age
		trendScore := velocity * authorTrust

		// 4. Store top trending statuses (minimum threshold)
		if trendScore > 5.0 && engagementScore > 5 {
			trend := &StatusTrendStorage{
				ID:            status.ID,
				AuthorID:      status.AuthorID,
				Content:       status.Content,
				Engagements:   int64(engagementScore),
				PublishedAt:   status.CreatedAt,
				Likes:         status.Likes,
				Boosts:        status.Boosts,
				Replies:       status.Replies,
				TrendingScore: trendScore,
				Velocity:      velocity,
			}

			if err := h.trendingRepo.StoreStatusTrend(ctx, trend); err != nil {
				h.logger.Error("failed to store status trend",
					zap.String("status_id", status.ID),
					zap.Error(err),
				)
			} else {
				trendingCount++
			}
		}
	}

	h.logger.Debug("processed status trends",
		zap.Int("total_statuses", len(statuses)),
		zap.Int("trending_count", trendingCount),
	)

	return trendingCount, nil
}

// aggregateLinkTrends processes link shares and updates trending scores
func (h *TrendAggregatorHandler) aggregateLinkTrends(ctx context.Context, since time.Time) (int, error) {
	// 1. Get recent links from repository
	links, err := h.trendingRepo.GetRecentLinks(ctx, since, 1000)
	if err != nil {
		return 0, pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to get recent links")
	}

	h.logger.Debug("retrieved recent links for aggregation",
		zap.Int("link_count", len(links)),
		zap.Time("since", since),
	)

	// 2. Count shares and unique sharers
	linkStats := make(map[string]*LinkTrendData)
	for _, link := range links {
		userID := link.UserID
		if err := common.ValidateRequiredParam("userID", userID); err != nil {
			continue // Skip if no user ID
		}

		if stats, exists := linkStats[link.URL]; exists {
			stats.ShareCount++
			stats.UniqueSharers[userID] = true
		} else {
			linkStats[link.URL] = &LinkTrendData{
				URL:           link.URL,
				Title:         link.Title,
				Description:   link.Description,
				ShareCount:    1,
				UniqueSharers: map[string]bool{userID: true},
				FirstShared:   link.CreatedAt,
				LastShared:    link.CreatedAt,
			}
		}
		if link.CreatedAt.After(linkStats[link.URL].LastShared) {
			linkStats[link.URL].LastShared = link.CreatedAt
		}
	}

	trendingCount := 0
	for url, stats := range linkStats {
		// 3. Calculate trend scores
		uniqueSharerCount := len(stats.UniqueSharers)
		timeSpan := stats.LastShared.Sub(stats.FirstShared).Hours()
		if timeSpan == 0 {
			timeSpan = 1
		}
		velocity := float64(stats.ShareCount) / timeSpan
		trendScore := velocity * float64(uniqueSharerCount) * 50

		// 4. Store top trending links
		if trendScore > 5.0 && stats.ShareCount > 2 {
			trend := &LinkTrendStorage{
				URL:           url,
				Title:         stats.Title,
				Description:   stats.Description,
				ShareCount:    int64(stats.ShareCount),
				CreatedAt:     stats.FirstShared,
				TrendingScore: trendScore,
				Velocity:      velocity,
			}

			if err := h.trendingRepo.StoreLinkTrend(ctx, trend); err != nil {
				h.logger.Error("failed to store link trend",
					zap.String("url", url),
					zap.Error(err),
				)
			} else {
				trendingCount++
			}
		}
	}

	h.logger.Debug("processed link trends",
		zap.Int("total_links", len(linkStats)),
		zap.Int("trending_count", trendingCount),
	)

	return trendingCount, nil
}

// cleanupOldTrends removes outdated trend data
func (h *TrendAggregatorHandler) cleanupOldTrends(ctx context.Context) {
	// Remove trend data older than 7 days
	cutoff := time.Now().AddDate(0, 0, -7)

	h.logger.Info("starting cleanup of old trend data",
		zap.Time("cutoff", cutoff),
	)

	// IMPORTANT:
	// Manual cleanup previously issued DynamoDB Scans and caused catastrophic data loss by deleting
	// non-trend items. Trend models write TTLs (`ttl = updatedAt + 7d`) so expiration is handled by
	// DynamoDB TTL. We keep this scheduled step for observability, but do not delete anything here.
	h.logger.Info("skipping manual trend cleanup (ttl handles expiration)",
		zap.Int("retention_days", 7),
	)
}

// HashtagTrendData holds hashtag trending information
type HashtagTrendData struct {
	Name        string
	TotalUses   int
	UniqueUsers map[string]bool
	FirstSeen   time.Time
	LastSeen    time.Time
}

// LinkTrendData holds link trending information
type LinkTrendData struct {
	URL           string
	Title         string
	Description   string
	ShareCount    int
	UniqueSharers map[string]bool
	FirstShared   time.Time
	LastShared    time.Time
}

// HashtagTrendStorage represents trending hashtag data for storage
type HashtagTrendStorage struct {
	Name          string
	UsageCount    int64
	UniqueUsers   int64
	FirstSeen     time.Time
	LastUsed      time.Time
	TrendingScore float64
	Velocity      float64
}

// StatusTrendStorage represents trending status data for storage
type StatusTrendStorage struct {
	ID            string
	AuthorID      string
	Content       string
	Engagements   int64
	PublishedAt   time.Time
	Likes         int
	Boosts        int
	Replies       int
	TrendingScore float64
	Velocity      float64
}

// LinkTrendStorage represents trending link data for storage
type LinkTrendStorage struct {
	URL           string
	Title         string
	Description   string
	ShareCount    int64
	CreatedAt     time.Time
	TrendingScore float64
	Velocity      float64
}

var (
	lambdaCtx *common.LambdaContext
	handler   *TrendAggregatorHandler
	db        core.DB
)

var (
	mustInitializeLambdaFn     = common.MustInitializeLambda
	newLambdaOptimizedClientFn = theorydb.NewLambdaOptimizedClient
	lambdaStartFn              = lambda.Start
)

func init() {
	if common.RunningUnitTests() {
		return
	}
	initializeTrendAggregator()
}

func initializeTrendAggregator() {
	// Initialize Lambda with basic configuration for trend aggregation
	lambdaCtx = mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName:        "trend-aggregator",
		LambdaType:         common.LambdaTypeBasic,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableTracing:      true,
		EnableHealthCheck:  false,
		EnableCostTracking: true,
		RequestTimeout:     30 * time.Second,
		RetryMaxAttempts:   3,
	})

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = newLambdaOptimizedClientFn(context.Background(), lambdaCtx.Config.Region)
	if err != nil {
		lambdaCtx.Logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize handler
	handler = NewTrendAggregatorHandler(db, lambdaCtx.Logger)
}

func runTrendAggregator() {
	app := apptheory.New()

	appName := strings.TrimSpace(os.Getenv("APP_NAME"))
	stage := strings.TrimSpace(os.Getenv("STAGE"))
	ruleName := naming.ResourceNameWithApp(appName, "trend-aggregator-schedule-0", stage)

	app.EventBridge(apptheory.EventBridgeRule(ruleName), handler.HandleScheduledEvent)

	lambdaStartFn(func(ctx context.Context, event json.RawMessage) (any, error) {
		return app.HandleLambda(ctx, event)
	})
}

func main() {
	runTrendAggregator()
}
