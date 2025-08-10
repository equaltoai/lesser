// Package services provides a unified service layer for business logic operations
package services

import (
	"context"
	"time"

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