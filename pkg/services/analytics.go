// Package services provides a unified service layer for business logic operations
package services

import (
	"context"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"go.uber.org/zap"
)

// analyticsService implements AnalyticsService
type analyticsService struct {
	deps    *ServiceDependencies
	storage StorageAdapter
	logger  *zap.Logger
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(deps *ServiceDependencies) AnalyticsService {
	return &analyticsService{
		deps:    deps,
		storage: CreateStorageAdapter(deps.Repos),
		logger:  deps.Logger.(*zap.Logger),
	}
}

// RecordStatusCreation records when a user creates a status
func (a *analyticsService) RecordStatusCreation(ctx context.Context, actorID string, timestamp time.Time) error {
	return a.storage.RecordActivity(ctx, "status", actorID, timestamp)
}

// RecordHashtagUsage records hashtag usage for trending analysis
func (a *analyticsService) RecordHashtagUsage(ctx context.Context, hashtags []string, objectID, actorID string) error {
	for _, hashtag := range hashtags {
		if err := a.storage.RecordHashtagUsage(ctx, hashtag, objectID, actorID); err != nil {
			a.logger.Warn("failed to record hashtag usage",
				zap.String("hashtag", hashtag),
				zap.String("object_id", objectID),
				zap.Error(err))
		}
	}
	return nil
}

// RecordLinkShare records when links are shared in posts
func (a *analyticsService) RecordLinkShare(ctx context.Context, links []string, objectID, actorID string) error {
	for _, link := range links {
		if err := a.storage.RecordLinkShare(ctx, link, objectID, actorID); err != nil {
			a.logger.Warn("failed to record link share",
				zap.String("link", link),
				zap.String("object_id", objectID),
				zap.Error(err))
		}
	}
	return nil
}

// RecordEngagement records user engagement with content
func (a *analyticsService) RecordEngagement(ctx context.Context, objectID, engagementType, actorID string) error {
	return a.storage.RecordStatusEngagement(ctx, objectID, engagementType, actorID)
}

// GetInfrastructureHealth returns the current infrastructure health status
func (a *analyticsService) GetInfrastructureHealth(ctx context.Context) (*model.InfrastructureStatus, error) {
	// Query storage for real health metrics
	health, err := a.storage.GetInfrastructureHealth(ctx)
	if err != nil {
		a.logger.Error("failed to get infrastructure health", zap.Error(err))
		return nil, err
	}
	return health, nil
}

// GetInstanceBudgets returns budget information for instances
func (a *analyticsService) GetInstanceBudgets(ctx context.Context, exceeded *bool) ([]*model.InstanceBudget, error) {
	// Query storage for budget data
	budgets, err := a.storage.GetInstanceBudgets(ctx, exceeded)
	if err != nil {
		a.logger.Error("failed to get instance budgets",
			zap.Bool("exceeded_only", exceeded != nil && *exceeded),
			zap.Error(err))
		return nil, err
	}
	return budgets, nil
}

// GetInstanceHealthReport returns comprehensive health report for a domain
func (a *analyticsService) GetInstanceHealthReport(ctx context.Context, domain string) (*model.InstanceHealthReport, error) {
	// Query storage for health report data
	report, err := a.storage.GetInstanceHealthReport(ctx, domain)
	if err != nil {
		a.logger.Error("failed to get instance health report",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, err
	}
	return report, nil
}
