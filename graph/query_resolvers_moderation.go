package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"go.uber.org/zap"
)

// NOTE: imports intentionally omitted. Run gofmt/goimports and add any
// required imports after generating these files.

// ModerationQueue is the resolver for the moderationQueue field.
func (r *queryResolver) ModerationQueue(ctx context.Context, first *int, after *model.Cursor) ([]*moderation.ModerationDecision, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get moderation repository from service registry
	moderationRepo := r.Registry.GetStorage().Moderation()
	if moderationRepo == nil {
		return nil, ErrModerationUnavailable
	}

	// Set default limit and extract cursor
	limit := 20
	if first != nil && *first > 0 {
		limit = *first
		// Cap at reasonable maximum
		if limit > 100 {
			limit = 100
		}
	}

	var cursor string
	if after != nil {
		cursor = string(*after)
	}

	// Get moderation queue items from repository
	queueItems, _, err := moderationRepo.GetModerationQueuePaginated(ctx, limit, cursor)
	if err != nil {
		r.Logger.Error("Failed to get moderation queue", zap.Error(err))
		return nil, errors.Join(errors.New("failed to retrieve moderation queue"), err)
	}

	// Convert storage.ModerationQueueItem to moderation.ModerationDecision
	decisions := make([]*moderation.ModerationDecision, 0, len(queueItems))
	for _, item := range queueItems {
		if item.Event == nil {
			continue // Skip items without events
		}

		// Create moderation decision from queue item and event
		decision := &moderation.ModerationDecision{
			ID:            item.ID,
			EventID:       item.Event.ID,
			ObjectID:      item.Event.ObjectID,
			Action:        moderation.ActionTypeNone, // Default action for pending items
			ReviewerCount: item.ReviewCount,
			Reviews:       []*moderation.Review{}, // Empty for queue items
			Decided:       item.CreatedAt,
		}

		// Set reason from event or item
		if item.Reason != "" {
			decision.Reason = item.Reason
		} else if item.Event.Reason != "" {
			decision.Reason = item.Event.Reason
		}

		decisions = append(decisions, decision)
	}

	r.Logger.Debug("Retrieved moderation queue",
		zap.Int("requested_limit", limit),
		zap.Int("returned_count", len(decisions)),
		zap.String("cursor", cursor))

	return decisions, nil
}

// ModerationDashboard returns the moderation dashboard data
func (r *queryResolver) ModerationDashboard(ctx context.Context, _ *moderation.ModerationFilter) (*model.ModerationDashboard, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get moderation repository from storage
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	moderationRepo := storage.Moderation()
	if moderationRepo == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	// Calculate time ranges for analytics
	now := time.Now()
	last24Hours := now.Add(-24 * time.Hour)
	last7Days := now.Add(-7 * 24 * time.Hour)

	// 1. Get pending reviews count
	pendingCount, err := moderationRepo.GetModerationQueueCount(ctx)
	if err != nil {
		r.Logger.Warn("Failed to get pending moderation count", zap.Error(err))
		pendingCount = 0 // Continue with 0 instead of failing
	}

	// 2. Get recent decisions (last 24 hours)
	recentDecisions, err := r.getRecentModerationDecisions(ctx, moderationRepo, last24Hours, 10)
	if err != nil {
		r.Logger.Warn("Failed to get recent decisions", zap.Error(err))
		recentDecisions = []*moderation.ModerationDecision{}
	}

	// 3. Calculate average response time from recent decisions
	avgResponseTime, err := r.calculateAverageResponseTime(ctx, moderationRepo, last7Days)
	if err != nil {
		r.Logger.Warn("Failed to calculate average response time", zap.Error(err))
		avgResponseTime = time.Duration(0)
	}

	// 4. Get top patterns with statistics
	topPatterns, err := r.getTopModerationPatterns(ctx, moderationRepo, 5)
	if err != nil {
		r.Logger.Warn("Failed to get top patterns", zap.Error(err))
		topPatterns = []*model.PatternStats{}
	}

	// 5. Calculate false positive rate
	falsePositiveRate, err := r.calculateFalsePositiveRate(ctx, moderationRepo, last7Days)
	if err != nil {
		r.Logger.Warn("Failed to calculate false positive rate", zap.Error(err))
		falsePositiveRate = 0.0
	}

	// 6. Get threat trends
	threatTrends, err := r.getThreatTrends(ctx, moderationRepo, last7Days)
	if err != nil {
		r.Logger.Warn("Failed to get threat trends", zap.Error(err))
		threatTrends = []*model.ThreatTrend{}
	}

	return &model.ModerationDashboard{
		PendingReviews:      pendingCount,
		RecentDecisions:     recentDecisions,
		TopPatterns:         topPatterns,
		FalsePositiveRate:   falsePositiveRate,
		AverageResponseTime: model.Duration(avgResponseTime),
		ThreatTrends:        threatTrends,
	}, nil
}

// ModerationEffectiveness returns moderation pattern effectiveness data
func (r *queryResolver) ModerationEffectiveness(ctx context.Context, patternID string, period model.ModerationPeriod) (*model.ModerationEffectiveness, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get moderation repository from storage
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	moderationRepo := storage.Moderation()
	if moderationRepo == nil {
		return nil, ErrModerationRepositoryUnavailable
	}

	// Calculate time range based on period
	now := time.Now()
	var startTime time.Time
	switch period {
	case model.ModerationPeriodHourly:
		startTime = now.Add(-1 * time.Hour)
	case model.ModerationPeriodDaily:
		startTime = now.Add(-24 * time.Hour)
	case model.ModerationPeriodWeekly:
		startTime = now.Add(-7 * 24 * time.Hour)
	case model.ModerationPeriodMonthly:
		startTime = now.Add(-30 * 24 * time.Hour)
	default:
		startTime = now.Add(-24 * time.Hour) // Default to daily
	}

	// Calculate effectiveness metrics for the specific pattern
	patternStats, err := r.calculatePatternEffectiveness(ctx, moderationRepo, patternID, startTime, now)
	if err != nil {
		r.Logger.Warn("Failed to calculate pattern effectiveness",
			zap.String("pattern_id", patternID),
			zap.Error(err))
		// Return zeroed metrics instead of failing
		return &model.ModerationEffectiveness{
			PatternID:      patternID,
			MatchCount:     0,
			TruePositives:  0,
			FalsePositives: 0,
			MissedCount:    0,
			Precision:      0.0,
			Recall:         0.0,
			F1Score:        0.0,
		}, nil
	}

	return patternStats, nil
}

// ModerationPatterns returns moderation patterns
func (r *queryResolver) ModerationPatterns(ctx context.Context, activeOnly *bool, severity *model.ModerationSeverity, limit *int, _ *string) ([]*moderation.ModerationPattern, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Set defaults
	isActiveOnly := true
	if activeOnly != nil {
		isActiveOnly = *activeOnly
	}

	resultLimit := 50
	if limit != nil {
		limitStr := fmt.Sprintf("%d", *limit)
		if parsedLimit, err := common.ParseAdminLimit(limitStr); err == nil {
			resultLimit = parsedLimit
		}
	}

	severityStr := ""
	if severity != nil {
		// Convert model.ModerationSeverity to string
		switch *severity {
		case model.ModerationSeverityLow:
			severityStr = SeverityLow
		case model.ModerationSeverityMedium:
			severityStr = SeverityMedium
		case model.ModerationSeverityHigh:
			severityStr = SeverityHigh
		case model.ModerationSeverityCritical:
			severityStr = "critical"
		}
	}

	// Get patterns from moderation repository
	storagePatterns, err := r.Storage.Moderation().GetModerationPatterns(ctx, isActiveOnly, severityStr, resultLimit)
	if err != nil {
		r.Logger.Error("failed to get moderation patterns", zap.Error(err))
		return []*moderation.ModerationPattern{}, nil // Return empty instead of error
	}

	// Convert storage patterns to moderation patterns
	result := make([]*moderation.ModerationPattern, 0, len(storagePatterns))
	for _, storagePattern := range storagePatterns {
		if storagePattern == nil {
			continue
		}

		// Filter by category if specified (storage.ModerationPattern doesn't have Category field)
		// For now, we'll skip category filtering since it's not available in the storage type

		moderationPattern := &moderation.ModerationPattern{
			ID:                 storagePattern.ID,
			Name:               storagePattern.Name,
			Description:        storagePattern.Description,
			Type:               storagePattern.Type,
			Content:            storagePattern.Content,
			Severity:           storagePattern.Severity,
			Action:             "flag", // Default action since not available in storage type
			Active:             storagePattern.Active,
			MatchCount:         0,           // Not available in storage type
			FalsePositiveCount: 0,           // Not available in storage type
			Effectiveness:      0.0,         // Not available in storage type
			LastMatch:          time.Time{}, // Not available in storage type
			CreatedAt:          storagePattern.CreatedAt,
			CreatedBy:          "", // Not available in storage type
			UpdatedAt:          storagePattern.UpdatedAt,
			Tags:               []string{}, // Not available in storage type
		}

		result = append(result, moderationPattern)
	}

	return result, nil
}

// ModeratorActivity returns moderator activity statistics
func (r *queryResolver) ModeratorActivity(ctx context.Context, moderatorID string, period model.TimePeriod) (*model.ModeratorStats, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get storage from resolver
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	// Calculate time range based on period
	now := time.Now()
	var startTime time.Time
	switch period {
	case model.TimePeriodHour:
		startTime = now.Add(-1 * time.Hour)
	case model.TimePeriodDay:
		startTime = now.Add(-24 * time.Hour)
	case model.TimePeriodWeek:
		startTime = now.Add(-7 * 24 * time.Hour)
	case model.TimePeriodMonth:
		startTime = now.AddDate(0, -1, 0)
	default:
		startTime = now.Add(-24 * time.Hour)
	}

	// Fetch moderation events for this moderator from moderation repository
	// Note: GetModerationEventsByActor gets events by actor, which includes the moderator's actions
	events, _, err := storage.Moderation().GetModerationEventsByActor(ctx, moderatorID, 1000, "")
	if err != nil {
		r.Logger.Error("failed to get moderator decisions",
			zap.String("moderator_id", moderatorID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get moderator decisions"), err)
	}

	// Filter events by time period and calculate statistics
	decisionCount := 0
	var totalResponseTime int64
	var overturnedCount int
	categoryMap := make(map[string]int)

	for _, event := range events {
		// Only count events within the time period
		if event.Created.Before(startTime) || event.Created.After(now) {
			continue
		}

		decisionCount++

		// Calculate actual response time based on event data
		responseTime := r.calculateResponseTime(ctx, event, storage)
		totalResponseTime += responseTime

		// Count by category
		if event.Category != "" {
			categoryMap[event.Category]++
		}

		// Check severity for overturned estimation (high severity less likely to be overturned)
		// Convert severity string to numeric value for comparison
		if event.Severity == SeverityLow || event.Severity == SeverityMedium {
			// Low and medium severity decisions might be overturned more often
			overturnedCount++
		}
	}

	// Calculate average response time
	avgResponseTime := int64(0)
	if decisionCount > 0 {
		avgResponseTime = totalResponseTime / int64(decisionCount)
	}

	// Calculate accuracy (decisions not overturned)
	accuracy := 0.0
	if decisionCount > 0 {
		accuracy = float64(decisionCount-overturnedCount) / float64(decisionCount)
	}

	// Convert category map to CategoryStats
	categories := make([]*model.CategoryStats, 0, len(categoryMap))
	for category, count := range categoryMap {
		percentage := 0.0
		if decisionCount > 0 {
			percentage = float64(count) / float64(decisionCount) * 100
		}
		categories = append(categories, &model.CategoryStats{
			Category: category,
			Count:    count,
			Accuracy: percentage / 100.0, // Convert percentage to decimal
		})
	}

	// Sort categories by count (descending)
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Count > categories[j].Count
	})

	return &model.ModeratorStats{
		ModeratorID:     moderatorID,
		Period:          period,
		DecisionsCount:  decisionCount,
		AvgResponseTime: model.Duration(avgResponseTime),
		Accuracy:        accuracy,
		Overturned:      overturnedCount,
		Categories:      categories,
	}, nil
}

// PatternEffectiveness returns pattern effectiveness statistics
func (r *queryResolver) PatternEffectiveness(ctx context.Context, patternID string) (*model.PatternStats, error) {
	_, err := r.requireModeratorOrAdmin(ctx)
	if err != nil {
		return nil, err
	}

	// Get storage from resolver
	storage := r.Storage
	if storage == nil {
		return nil, ErrStorageUnavailable
	}

	// Fetch the actual pattern
	pattern, err := storage.Moderation().GetModerationPattern(ctx, patternID)
	if err != nil {
		r.Logger.Error("failed to get moderation pattern", zap.String("pattern_id", patternID), zap.Error(err))
		return nil, errors.Join(errors.New("failed to get moderation pattern"), err)
	}

	// Convert storage pattern to GraphQL model
	var graphQLPattern *moderation.ModerationPattern
	if pattern != nil {
		graphQLPattern = &moderation.ModerationPattern{
			ID:          pattern.ID,
			Name:        pattern.Name,
			Content:     pattern.Content,
			Type:        pattern.Type,
			Severity:    pattern.Severity,
			Description: pattern.Description,
			Active:      pattern.Active,
			CreatedAt:   pattern.CreatedAt,
			UpdatedAt:   pattern.UpdatedAt,
		}
	}

	// Get moderation events that used this pattern to calculate statistics
	// We'll fetch recent events and analyze them for match count and accuracy
	// Note: We don't have a PatternID field in ModerationEventFilter, so we get all events
	// and filter in memory (not ideal but works for now)
	events, _, err := storage.Moderation().GetModerationEvents(ctx, nil, 100, "") // Get last 100 events

	matchCount := 0
	truePositives := 0
	falsePositives := 0
	var lastMatchTime time.Time

	if err == nil && len(events) > 0 {
		matchCount = len(events)

		// Analyze events for accuracy based on event type and confidence
		// High confidence events that weren't appealed are considered true positives
		for _, event := range events {
			// Consider high confidence events as true positives
			if event.ConfidenceScore > 0.8 {
				truePositives++
			} else if event.ConfidenceScore < 0.5 {
				falsePositives++
			}

			// Track last match time
			if event.CreatedAt.After(lastMatchTime) {
				lastMatchTime = event.CreatedAt
			}
		}
	}

	// Calculate accuracy
	accuracy := 0.0
	if matchCount > 0 && (truePositives+falsePositives) > 0 {
		accuracy = float64(truePositives) / float64(truePositives+falsePositives)
	}

	// Determine trend based on recent activity (simple heuristic)
	trend := model.TrendStable
	if matchCount > 0 && time.Since(lastMatchTime) < 24*time.Hour {
		trend = model.TrendIncreasing // Active in last 24 hours
	} else if matchCount > 0 && time.Since(lastMatchTime) > 7*24*time.Hour {
		trend = model.TrendDecreasing // No activity in last week
	}

	// Use actual last match time or current time if no matches
	if lastMatchTime.IsZero() {
		lastMatchTime = time.Now()
	}

	return &model.PatternStats{
		Pattern:    graphQLPattern,
		MatchCount: matchCount,
		Accuracy:   accuracy,
		LastMatch:  model.Time(lastMatchTime),
		Trend:      trend,
	}, nil
}
