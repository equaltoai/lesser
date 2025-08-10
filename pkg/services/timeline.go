package services

import (
	"context"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// timelineService implements TimelineService
type timelineService struct {
	deps    *ServiceDependencies
	storage StorageAdapter
	logger  *zap.Logger
}

// NewTimelineService creates a new timeline service
func NewTimelineService(deps *ServiceDependencies) TimelineService {
	return &timelineService{
		deps:    deps,
		storage: CreateStorageAdapter(deps.Repos),
		logger:  deps.Logger.(*zap.Logger),
	}
}

// FanOutToFollowers fans out an activity to all followers' timelines
func (t *timelineService) FanOutToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	// For Create activities, fan out the post to followers' home timelines
	if activity.Type != activitypub.CreateType {
		return nil // Only fan out Create activities
	}

	// Extract username from actor if not provided
	var username string
	if actor != nil {
		username = actor.PreferredUsername
	} else {
		// Extract from activity actor ID
		// Format: https://domain/users/username or https://domain/@username
		actorID := activity.Actor
		if strings.Contains(actorID, "/users/") {
			parts := strings.Split(actorID, "/users/")
			if len(parts) == 2 {
				username = parts[1]
			}
		} else if strings.Contains(actorID, "/@") {
			parts := strings.Split(actorID, "/@")
			if len(parts) == 2 {
				username = parts[1]
			}
		}
	}

	if username == "" {
		t.logger.Warn("cannot fan out post: no username available")
		return nil
	}

	// Use storage's fan out method
	return t.storage.FanOutPost(ctx, activity)
}

// UpdateTimelines updates timelines when content changes
func (t *timelineService) UpdateTimelines(ctx context.Context, activity *activitypub.Activity) error {
	switch activity.Type {
	case activitypub.CreateType:
		return t.FanOutToFollowers(ctx, activity, nil)
	case activitypub.UpdateType:
		// Update existing timeline entries
		return t.updateTimelineEntries(ctx, activity)
	case activitypub.DeleteType:
		// Remove from timelines
		if objectID, ok := activity.Object.(string); ok {
			return t.RemoveFromTimelines(ctx, objectID)
		}
	case activitypub.LikeType, activitypub.AnnounceType:
		// These might update engagement counters in timelines
		return t.updateEngagementCounters(ctx, activity)
	}
	
	return nil
}

// RemoveFromTimelines removes content from all timelines
func (t *timelineService) RemoveFromTimelines(ctx context.Context, objectID string) error {
	// Get all timeline entries for this object
	timelines := []string{"home", "public", "local"} // Standard timeline types
	
	for _, timelineType := range timelines {
		if err := t.removeFromTimeline(ctx, timelineType, objectID); err != nil {
			t.logger.Warn("failed to remove from timeline",
				zap.String("timeline", timelineType),
				zap.String("object_id", objectID),
				zap.Error(err))
		}
	}
	
	return nil
}

// Helper methods

func (t *timelineService) updateTimelineEntries(_ context.Context, activity *activitypub.Activity) error {
	// Update timeline entries when content is modified
	objectID := ""
	if note, ok := activity.Object.(*activitypub.Note); ok {
		objectID = note.ID
	}
	
	if objectID == "" {
		return nil
	}
	
	// Update timeline entries with new content
	// This would involve updating cached timeline entries
	t.logger.Debug("updating timeline entries for updated content",
		zap.String("object_id", objectID))
	
	return nil
}

func (t *timelineService) updateEngagementCounters(_ context.Context, activity *activitypub.Activity) error {
	// Update like/boost counters in timeline entries
	objectID := ""
	if objID, ok := activity.Object.(string); ok {
		objectID = objID
	}
	
	if objectID == "" {
		return nil
	}
	
	t.logger.Debug("updating engagement counters in timelines",
		zap.String("object_id", objectID),
		zap.String("activity_type", activity.Type))
	
	return nil
}

func (t *timelineService) removeFromTimeline(_ context.Context, timelineType, objectID string) error {
	// Remove specific object from specific timeline type
	// This would involve querying and deleting timeline entries
	
	t.logger.Debug("removing from timeline",
		zap.String("timeline_type", timelineType),
		zap.String("object_id", objectID))
		
	return nil
}