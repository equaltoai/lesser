// Package hashtags provides hashtag management and discovery services for the Lesser ActivityPub server.
//
// This service handles all operations related to hashtags including:
// - Following and unfollowing hashtags
// - Retrieving hashtag timelines (single and multi-hashtag)
// - Getting hashtag statistics and trends
// - Discovering suggested hashtags
// - Managing hashtag notifications
// - Muting hashtags
package hashtags

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

// Service provides business logic for hashtag operations
type Service struct {
	hashtagRepo      *repositories.HashtagRepository
	statusRepo       *repositories.StatusRepository
	relationshipRepo *repositories.RelationshipRepository
	publisher        streaming.Publisher
	logger           *zap.Logger
	domain           string
}

// NewService creates a new hashtag service
func NewService(
	hashtagRepo *repositories.HashtagRepository,
	statusRepo *repositories.StatusRepository,
	relationshipRepo *repositories.RelationshipRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
	domain string,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		hashtagRepo:      hashtagRepo,
		statusRepo:       statusRepo,
		relationshipRepo: relationshipRepo,
		publisher:        publisher,
		logger:           logger,
		domain:           domain,
	}
}

// Query and Command types for CQRS pattern

// GetHashtagQuery contains parameters for retrieving hashtag information
type GetHashtagQuery struct {
	Name     string `json:"name" validate:"required"`
	ViewerID string `json:"viewer_id"` // For checking following status
}

// FollowHashtagCommand contains data needed to follow a hashtag
type FollowHashtagCommand struct {
	UserID               string `json:"user_id" validate:"required"`
	Hashtag              string `json:"hashtag" validate:"required"`
	NotificationsEnabled bool   `json:"notifications_enabled"`
}

// UnfollowHashtagCommand contains data needed to unfollow a hashtag
type UnfollowHashtagCommand struct {
	UserID  string `json:"user_id" validate:"required"`
	Hashtag string `json:"hashtag" validate:"required"`
}

// GetFollowedHashtagsQuery contains parameters for retrieving followed hashtags
type GetFollowedHashtagsQuery struct {
	UserID  string `json:"user_id" validate:"required"`
	First   int    `json:"first"`
	AfterSK string `json:"after_sk"` // Cursor for pagination
}

// GetHashtagTimelineQuery contains parameters for retrieving hashtag timeline
type GetHashtagTimelineQuery struct {
	Hashtag    string  `json:"hashtag" validate:"required"`
	First      int     `json:"first"`
	After      *string `json:"after"` // Status ID cursor
	ViewerID   string  `json:"viewer_id"`
	Visibility string  `json:"visibility"` // Optional visibility filter
}

// GetMultiHashtagTimelineQuery contains parameters for multi-hashtag timeline
type GetMultiHashtagTimelineQuery struct {
	Hashtags []string `json:"hashtags" validate:"required,min=1"`
	Mode     string   `json:"mode" validate:"required,oneof=ANY ALL"`
	First    int      `json:"first"`
	After    *string  `json:"after"`
	ViewerID string   `json:"viewer_id"`
}

// GetSuggestedHashtagsQuery contains parameters for hashtag suggestions
type GetSuggestedHashtagsQuery struct {
	UserID string `json:"user_id"`
	Limit  int    `json:"limit"`
}

// UpdateHashtagNotificationsCommand contains data for updating notification settings
type UpdateHashtagNotificationsCommand struct {
	UserID  string `json:"user_id" validate:"required"`
	Hashtag string `json:"hashtag" validate:"required"`
	Notify  bool   `json:"notify"`
}

// MuteHashtagCommand contains data for muting a hashtag
type MuteHashtagCommand struct {
	UserID  string     `json:"user_id" validate:"required"`
	Hashtag string     `json:"hashtag" validate:"required"`
	Until   *time.Time `json:"until"` // Optional expiration
}

// Result types

// HashtagResult contains hashtag information
type HashtagResult struct {
	Name        string                `json:"name"`
	URL         string                `json:"url"`
	Following   bool                  `json:"following"`
	Stats       *storage.HashtagStats `json:"stats"`
	RelatedTags []string              `json:"related_tags"`
	Events      []*streaming.Event    `json:"events"`
}

// FollowHashtagResult contains result of follow operation
type FollowHashtagResult struct {
	Hashtag string             `json:"hashtag"`
	Events  []*streaming.Event `json:"events"`
}

// HashtagConnection represents a paginated list of hashtags
type HashtagConnection struct {
	Hashtags   []*HashtagInfo `json:"hashtags"`
	NextCursor string         `json:"next_cursor"`
	HasMore    bool           `json:"has_more"`
}

// HashtagInfo contains basic hashtag information
type HashtagInfo struct {
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	UsageCount int       `json:"usage_count"`
	LastUsed   time.Time `json:"last_used"`
	Following  bool      `json:"following"`
	Muted      bool      `json:"muted"`
}

// PostConnection represents a paginated list of posts
type PostConnection struct {
	Posts      []*storage.StatusSearchResult `json:"posts"`
	NextCursor string                        `json:"next_cursor"`
	HasMore    bool                          `json:"has_more"`
}

// HashtagSuggestion represents a suggested hashtag
type HashtagSuggestion struct {
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	Reason     string  `json:"reason"` // Why it's suggested
	Score      float64 `json:"score"`
	UsageCount int     `json:"usage_count"`
}

// Service methods

// GetHashtag retrieves hashtag information with optional viewer context
func (s *Service) GetHashtag(ctx context.Context, query *GetHashtagQuery) (*HashtagResult, error) {
	s.logger.Debug("getting hashtag",
		zap.String("name", query.Name),
		zap.String("viewer_id", query.ViewerID))

	// Normalize hashtag name
	tagName := strings.ToLower(strings.TrimPrefix(query.Name, "#"))

	// Get hashtag info
	hashtagInfo, err := s.hashtagRepo.GetHashtagInfo(ctx, tagName)
	if err != nil {
		return nil, ErrGetHashtag
	}

	if hashtagInfo == nil {
		// Hashtag doesn't exist yet
		return &HashtagResult{
			Name:        tagName,
			URL:         fmt.Sprintf("https://%s/tags/%s", s.domain, tagName),
			Following:   false,
			Stats:       nil,
			RelatedTags: []string{},
		}, nil
	}

	// Check if viewer is following
	following := false
	if query.ViewerID != "" {
		following, _ = s.hashtagRepo.IsFollowingHashtag(ctx, query.ViewerID, tagName)
	}

	// Get hashtag stats
	statsAny, err := s.hashtagRepo.GetHashtagStats(ctx, tagName)
	if err != nil {
		s.logger.Warn("failed to get hashtag stats",
			zap.String("hashtag", tagName),
			zap.Error(err))
	}

	var stats *storage.HashtagStats
	if statsAny != nil {
		if s, ok := statsAny.(*storage.HashtagStats); ok {
			stats = s
		}
	}

	// Get related hashtags (simplified - would be more sophisticated in production)
	relatedTags := s.getRelatedHashtags(ctx, tagName, 5)

	return &HashtagResult{
		Name:        tagName,
		URL:         hashtagInfo.URL,
		Following:   following,
		Stats:       stats,
		RelatedTags: relatedTags,
	}, nil
}

// FollowHashtag creates a follow relationship for a hashtag
func (s *Service) FollowHashtag(ctx context.Context, cmd *FollowHashtagCommand) (*FollowHashtagResult, error) {
	s.logger.Info("following hashtag",
		zap.String("user_id", cmd.UserID),
		zap.String("hashtag", cmd.Hashtag))

	// Validate hashtag name
	if err := s.validateHashtagName(cmd.Hashtag); err != nil {
		return nil, err
	}

	// Create follow relationship
	err := s.hashtagRepo.FollowHashtag(ctx, cmd.UserID, cmd.Hashtag)
	if err != nil {
		return nil, ErrFollowHashtag
	}

	// Update notification settings if needed
	if !cmd.NotificationsEnabled {
		_ = s.hashtagRepo.UpdateHashtagNotificationSettings(ctx, cmd.UserID, cmd.Hashtag, false)
	}

	// Emit events
	events := s.emitHashtagFollowedEvents(ctx, cmd.UserID, cmd.Hashtag)

	return &FollowHashtagResult{
		Hashtag: cmd.Hashtag,
		Events:  events,
	}, nil
}

// UnfollowHashtag removes a follow relationship for a hashtag
func (s *Service) UnfollowHashtag(ctx context.Context, cmd *UnfollowHashtagCommand) (*FollowHashtagResult, error) {
	s.logger.Info("unfollowing hashtag",
		zap.String("user_id", cmd.UserID),
		zap.String("hashtag", cmd.Hashtag))

	// Remove follow relationship
	err := s.hashtagRepo.UnfollowHashtag(ctx, cmd.UserID, cmd.Hashtag)
	if err != nil {
		return nil, ErrUnfollowHashtag
	}

	// Emit events
	events := s.emitHashtagUnfollowedEvents(ctx, cmd.UserID, cmd.Hashtag)

	return &FollowHashtagResult{
		Hashtag: cmd.Hashtag,
		Events:  events,
	}, nil
}

// GetFollowedHashtags retrieves hashtags followed by a user with pagination
func (s *Service) GetFollowedHashtags(ctx context.Context, query *GetFollowedHashtagsQuery) (*HashtagConnection, error) {
	s.logger.Debug("getting followed hashtags",
		zap.String("user_id", query.UserID),
		zap.Int("first", query.First))

	// Set default limit
	limit := query.First
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Get followed hashtags from repository
	hashtags, nextCursor, err := s.hashtagRepo.GetFollowedHashtags(ctx, query.UserID, limit, query.AfterSK)
	if err != nil {
		return nil, ErrGetFollowedHashtags
	}

	// Get detailed info for each hashtag
	hashtagInfos := make([]*HashtagInfo, 0, len(hashtags))
	for _, tag := range hashtags {
		info, err := s.hashtagRepo.GetHashtagInfo(ctx, tag)
		if err != nil {
			s.logger.Warn("failed to get hashtag info",
				zap.String("hashtag", tag),
				zap.Error(err))
			continue
		}

		if info != nil {
			hashtagInfos = append(hashtagInfos, &HashtagInfo{
				Name:       info.Name,
				URL:        info.URL,
				UsageCount: info.UsageCount,
				LastUsed:   info.LastUsed,
				Following:  true,
				Muted:      false, // Would check mute status in production
			})
		}
	}

	return &HashtagConnection{
		Hashtags:   hashtagInfos,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	}, nil
}

// GetHashtagTimeline retrieves posts for a single hashtag
func (s *Service) GetHashtagTimeline(ctx context.Context, query *GetHashtagTimelineQuery) (*PostConnection, error) {
	s.logger.Debug("getting hashtag timeline",
		zap.String("hashtag", query.Hashtag),
		zap.Int("first", query.First))

	// Set default limit
	limit := query.First
	if limit <= 0 || limit > 40 {
		limit = 20
	}

	// Get timeline from repository
	results, err := s.hashtagRepo.GetHashtagTimelineAdvanced(ctx, query.Hashtag, query.After, limit, query.Visibility)
	if err != nil {
		return nil, ErrGetHashtagTimeline
	}

	// Determine next cursor
	nextCursor := ""
	if len(results) >= limit {
		nextCursor = results[len(results)-1].StatusID
	}

	return &PostConnection{
		Posts:      results,
		NextCursor: nextCursor,
		HasMore:    len(results) >= limit,
	}, nil
}

// GetMultiHashtagTimeline retrieves posts for multiple hashtags
func (s *Service) GetMultiHashtagTimeline(ctx context.Context, query *GetMultiHashtagTimelineQuery) (*PostConnection, error) {
	s.logger.Debug("getting multi-hashtag timeline",
		zap.Strings("hashtags", query.Hashtags),
		zap.String("mode", query.Mode),
		zap.Int("first", query.First))

	// Set default limit
	limit := query.First
	if limit <= 0 || limit > 40 {
		limit = 20
	}

	// Handle different modes
	var results []*storage.StatusSearchResult
	var err error

	switch query.Mode {
	case "ANY":
		// Union mode: get posts from any of the hashtags
		results, err = s.hashtagRepo.GetMultiHashtagTimeline(ctx, query.Hashtags, query.After, limit, query.ViewerID)
	case "ALL":
		// Intersection mode: get posts that have all hashtags
		results, err = s.getHashtagIntersection(ctx, query.Hashtags, query.After, limit)
	default:
		return nil, ErrInvalidMode
	}

	if err != nil {
		return nil, ErrGetMultiHashtagTimeline
	}

	// Determine next cursor
	nextCursor := ""
	if len(results) >= limit {
		nextCursor = results[len(results)-1].StatusID
	}

	return &PostConnection{
		Posts:      results,
		NextCursor: nextCursor,
		HasMore:    len(results) >= limit,
	}, nil
}

// GetSuggestedHashtags returns hashtag suggestions for a user
func (s *Service) GetSuggestedHashtags(ctx context.Context, query *GetSuggestedHashtagsQuery) ([]*HashtagSuggestion, error) {
	s.logger.Debug("getting suggested hashtags",
		zap.String("user_id", query.UserID),
		zap.Int("limit", query.Limit))

	// Set default limit
	limit := query.Limit
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	// Get trending hashtags (primary source of suggestions)
	since := time.Now().AddDate(0, 0, -7) // Last 7 days
	trending, err := s.hashtagRepo.GetTrendingHashtags(ctx, since, limit)
	if err != nil {
		s.logger.Warn("failed to get trending hashtags for suggestions",
			zap.Error(err))
		// Fall back to recent hashtags
		trending, _ = s.hashtagRepo.GetRecentHashtags(ctx, since, limit)
	}

	// Convert to suggestions
	suggestions := make([]*HashtagSuggestion, 0, len(trending))
	for _, tag := range trending {
		suggestions = append(suggestions, &HashtagSuggestion{
			Name:       tag.Name,
			URL:        tag.URL,
			Reason:     "trending",
			Score:      float64(tag.UsageCount),
			UsageCount: int(tag.UsageCount),
		})
	}

	// If we have a user, personalize suggestions
	if query.UserID != "" {
		suggestions = s.personalizeHashtagSuggestions(ctx, query.UserID, suggestions, limit)
	}

	// Limit results
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	return suggestions, nil
}

// UpdateHashtagNotifications updates notification settings for a followed hashtag
func (s *Service) UpdateHashtagNotifications(ctx context.Context, cmd *UpdateHashtagNotificationsCommand) error {
	s.logger.Info("updating hashtag notification settings",
		zap.String("user_id", cmd.UserID),
		zap.String("hashtag", cmd.Hashtag),
		zap.Bool("notify", cmd.Notify))

	err := s.hashtagRepo.UpdateHashtagNotificationSettings(ctx, cmd.UserID, cmd.Hashtag, cmd.Notify)
	if err != nil {
		return ErrUpdateHashtagNotifications
	}

	return nil
}

// MuteHashtag mutes a hashtag for a user
func (s *Service) MuteHashtag(ctx context.Context, cmd *MuteHashtagCommand) error {
	s.logger.Info("muting hashtag",
		zap.String("user_id", cmd.UserID),
		zap.String("hashtag", cmd.Hashtag))

	err := s.hashtagRepo.MuteHashtag(ctx, cmd.UserID, cmd.Hashtag)
	if err != nil {
		return ErrMuteHashtag
	}

	// Emit events
	_ = s.emitHashtagMutedEvents(ctx, cmd.UserID, cmd.Hashtag)

	return nil
}

// UnmuteHashtag unmutes a hashtag for a user
func (s *Service) UnmuteHashtag(ctx context.Context, userID, hashtag string) error {
	s.logger.Info("unmuting hashtag",
		zap.String("user_id", userID),
		zap.String("hashtag", hashtag))

	err := s.hashtagRepo.UnmuteHashtag(ctx, userID, hashtag)
	if err != nil {
		return ErrUnmuteHashtag
	}

	// Emit events
	_ = s.emitHashtagUnmutedEvents(ctx, userID, hashtag)

	return nil
}

// IsFollowingHashtag checks if a user is following a hashtag
func (s *Service) IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error) {
	following, err := s.hashtagRepo.IsFollowingHashtag(ctx, userID, hashtag)
	if err != nil {
		return false, ErrCheckFollowingHashtag
	}
	return following, nil
}

// Private helper methods

// validateHashtagName validates a hashtag name
func (s *Service) validateHashtagName(hashtag string) error {
	tagName := strings.TrimPrefix(hashtag, "#")
	if err := common.ValidateRequiredParam("hashtag", tagName); err != nil {
		return ErrHashtagNameRequired
	}
	if err := common.ValidateStringLength("hashtag", tagName, 1, 100); err != nil {
		return ErrHashtagNameTooLong
	}
	return nil
}

// getRelatedHashtags retrieves hashtags related to the given hashtag
func (s *Service) getRelatedHashtags(ctx context.Context, hashtag string, limit int) []string {
	// Query for recent posts with this hashtag
	posts, err := s.hashtagRepo.GetHashtagTimelineAdvanced(ctx, hashtag, nil, 50, "") // Get up to 50 recent posts
	if err != nil || len(posts) == 0 {
		return []string{}
	}

	// Count co-occurring hashtags
	hashtagCount := make(map[string]int)
	for _, post := range posts {
		// Extract hashtags from content
		tags := extractHashtagsFromContent(post.Content)
		for _, tag := range tags {
			// Skip the current hashtag and count others
			tagLower := strings.ToLower(tag)
			currentTagLower := strings.ToLower(hashtag)
			if tagLower != currentTagLower {
				hashtagCount[tagLower]++
			}
		}
	}

	// Sort by frequency
	type hashtagFreq struct {
		name  string
		count int
	}
	sorted := make([]hashtagFreq, 0, len(hashtagCount))
	for name, count := range hashtagCount {
		sorted = append(sorted, hashtagFreq{name: name, count: count})
	}

	// Sort descending by count
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Return top N
	result := make([]string, 0, limit)
	for i := 0; i < len(sorted) && i < limit; i++ {
		result = append(result, sorted[i].name)
	}

	return result
}

// extractHashtagsFromContent extracts hashtags from post content
func extractHashtagsFromContent(content string) []string {
	// Simple hashtag extraction - matches #word patterns
	hashtags := []string{}
	words := strings.Fields(content)
	for _, word := range words {
		if strings.HasPrefix(word, "#") {
			// Remove # and any trailing punctuation
			tag := strings.TrimPrefix(word, "#")
			tag = strings.TrimRight(tag, ".,!?;:")
			if len(tag) > 0 {
				hashtags = append(hashtags, tag)
			}
		}
	}
	return hashtags
}

// getHashtagIntersection gets posts that contain all specified hashtags
func (s *Service) getHashtagIntersection(ctx context.Context, hashtags []string, after *string, limit int) ([]*storage.StatusSearchResult, error) {
	if len(hashtags) == 0 {
		return []*storage.StatusSearchResult{}, nil
	}

	// Get timeline for first hashtag
	results, err := s.hashtagRepo.GetHashtagTimelineAdvanced(ctx, hashtags[0], after, limit*2, "")
	if err != nil {
		return nil, err
	}

	// For each result, check if it contains all other hashtags
	filtered := make([]*storage.StatusSearchResult, 0)
	for _, result := range results {
		hasAll := true
		for _, tag := range hashtags[1:] {
			tagLower := strings.ToLower(strings.TrimPrefix(tag, "#"))
			contentLower := strings.ToLower(result.Content)
			if !strings.Contains(contentLower, "#"+tagLower) {
				hasAll = false
				break
			}
		}
		if hasAll {
			filtered = append(filtered, result)
			if len(filtered) >= limit {
				break
			}
		}
	}

	return filtered, nil
}

// personalizeHashtagSuggestions adds personalized suggestions based on user activity
func (s *Service) personalizeHashtagSuggestions(ctx context.Context, userID string, suggestions []*HashtagSuggestion, limit int) []*HashtagSuggestion {
	// Get user's followed accounts to find related hashtags
	// This is a simplified version - production would be more sophisticated

	// Get suggested hashtags from repository (these are already trending/popular)
	searchResults, err := s.hashtagRepo.GetSuggestedHashtags(ctx, userID, limit)
	if err != nil {
		s.logger.Warn("failed to get personalized suggestions",
			zap.String("user_id", userID),
			zap.Error(err))
		return suggestions
	}

	// Merge with existing suggestions, avoiding duplicates
	existingNames := make(map[string]bool)
	for _, sugg := range suggestions {
		existingNames[sugg.Name] = true
	}

	for _, result := range searchResults {
		if !existingNames[result.Name] && len(suggestions) < limit {
			suggestions = append(suggestions, &HashtagSuggestion{
				Name:       result.Name,
				URL:        result.URL,
				Reason:     "popular",
				Score:      1.0,
				UsageCount: 0, // Would get from result if available
			})
			existingNames[result.Name] = true
		}
	}

	return suggestions
}

// Event emission helpers

// emitHashtagFollowedEvents emits events when a hashtag is followed
func (s *Service) emitHashtagFollowedEvents(ctx context.Context, userID, hashtag string) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "hashtag.followed",
		Stream:    fmt.Sprintf("user:%s", userID),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id": userID,
			"hashtag": hashtag,
		},
	}

	if err := s.publisher.PublishToUser(ctx, userID, event); err != nil {
		s.logger.Error("failed to publish hashtag followed event",
			zap.Error(err))
		return nil
	}

	return []*streaming.Event{event}
}

// emitHashtagUnfollowedEvents emits events when a hashtag is unfollowed
func (s *Service) emitHashtagUnfollowedEvents(ctx context.Context, userID, hashtag string) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "hashtag.unfollowed",
		Stream:    fmt.Sprintf("user:%s", userID),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id": userID,
			"hashtag": hashtag,
		},
	}

	if err := s.publisher.PublishToUser(ctx, userID, event); err != nil {
		s.logger.Error("failed to publish hashtag unfollowed event",
			zap.Error(err))
		return nil
	}

	return []*streaming.Event{event}
}

// emitHashtagMutedEvents emits events when a hashtag is muted
func (s *Service) emitHashtagMutedEvents(ctx context.Context, userID, hashtag string) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "hashtag.muted",
		Stream:    fmt.Sprintf("user:%s", userID),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id": userID,
			"hashtag": hashtag,
		},
	}

	if err := s.publisher.PublishToUser(ctx, userID, event); err != nil {
		s.logger.Error("failed to publish hashtag muted event",
			zap.Error(err))
		return nil
	}

	return []*streaming.Event{event}
}

// emitHashtagUnmutedEvents emits events when a hashtag is unmuted
func (s *Service) emitHashtagUnmutedEvents(ctx context.Context, userID, hashtag string) []*streaming.Event {
	if s.publisher == nil {
		return nil
	}

	event := &streaming.Event{
		Type:      "hashtag.unmuted",
		Stream:    fmt.Sprintf("user:%s", userID),
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"user_id": userID,
			"hashtag": hashtag,
		},
	}

	if err := s.publisher.PublishToUser(ctx, userID, event); err != nil {
		s.logger.Error("failed to publish hashtag unmuted event",
			zap.Error(err))
		return nil
	}

	return []*streaming.Event{event}
}

// GetHashtagActivity retrieves activity for a hashtag stream
//
//nolint:gocognit // Complex event handling and conversion logic
func (s *Service) GetHashtagActivity(ctx context.Context, hashtags []string) (<-chan *streaming.Event, error) {
	if s.publisher == nil {
		return nil, ErrPublisherNotAvailable
	}

	activityChan := make(chan *streaming.Event, 100)

	// Start background goroutine that listens to actual events
	go func() {
		defer close(activityChan)

		// Get the global event bus
		eventBus := streaming.GetGlobalEventBus(s.logger)
		if eventBus == nil || !eventBus.IsRunning() {
			s.logger.Error("event bus not available for hashtag activity")
			return
		}

		// Normalize hashtags
		normalizedTags := make([]string, len(hashtags))
		for i, tag := range hashtags {
			normalizedTags[i] = strings.ToLower(strings.TrimPrefix(tag, "#"))
		}

		// Build event filter with hashtag streams
		streams := make([]string, len(normalizedTags))
		for i, tag := range normalizedTags {
			streams[i] = fmt.Sprintf("hashtag:%s", tag)
		}

		eventFilter := &streaming.EventFilter{
			Types: []streaming.EventType{
				streaming.EventTypeStatus,        // Status posts
				streaming.EventTypeStatusUpdate,  // Updated posts
				streaming.EventTypeHashtagTrend,  // Hashtag trends
				streaming.EventTypeHashtagUpdate, // Hashtag updates
			},
			Streams: streams,
		}

		// Subscribe to event bus
		subscriber, err := eventBus.Subscribe(
			fmt.Sprintf("hashtag_activity_%d", time.Now().UnixNano()),
			eventFilter,
			100,
		)
		if err != nil {
			s.logger.Error("failed to subscribe to hashtag activity", zap.Error(err))
			return
		}
		defer subscriber.Close()

		s.logger.Info("hashtag activity subscription started", zap.Strings("hashtags", normalizedTags))

		// Forward events from subscriber to channel
		// Note: subscriber.Channel returns InternalEvent, we need to convert to Event
		for {
			select {
			case internalEvent := <-subscriber.Channel:
				if internalEvent == nil {
					return
				}

				// Convert InternalEvent to Event with populated payload
				payload := make(map[string]interface{})

				// Include the actual data from the internal event
				if internalEvent.Data != nil {
					payload["data"] = internalEvent.Data
				}

				// Include additional context
				if internalEvent.ActorID != "" {
					payload["actor_id"] = internalEvent.ActorID
				}
				if internalEvent.TargetID != "" {
					payload["target_id"] = internalEvent.TargetID
				}
				if internalEvent.UserID != "" {
					payload["user_id"] = internalEvent.UserID
				}

				// Include metadata
				if len(internalEvent.Metadata) > 0 {
					payload["metadata"] = internalEvent.Metadata
				}

				// Determine stream name from normalized tags
				streamName := ""
				if len(normalizedTags) > 0 {
					streamName = fmt.Sprintf("hashtag:%s", normalizedTags[0])
				}

				event := &streaming.Event{
					Type:      string(internalEvent.Type),
					Stream:    streamName,
					Payload:   payload, // Now includes actual data
					Timestamp: internalEvent.Timestamp,
				}

				select {
				case activityChan <- event:
				case <-ctx.Done():
					return
				}
			case <-subscriber.Quit:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return activityChan, nil
}
