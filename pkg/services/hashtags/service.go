// Package hashtags implements the hashtag follow/mute business logic used by GraphQL resolvers.
package hashtags

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"go.uber.org/zap"
)

const (
	defaultRelatedHashtagSampleSize = 50
	defaultFollowedHashtagLimit     = 20
	maxFollowedHashtagLimit         = 100
)

// HashtagRepository defines the storage interface needed by the hashtag service.
type HashtagRepository interface {
	FollowHashtag(ctx context.Context, userID, hashtag string) error
	UnfollowHashtag(ctx context.Context, userID, hashtag string) error
	IsFollowingHashtag(ctx context.Context, userID, hashtag string) (bool, error)
	GetFollowedHashtags(ctx context.Context, userID string, limit int, cursor string) ([]*storage.HashtagFollow, string, error)
	GetHashtagInfo(ctx context.Context, hashtag string) (*storage.Hashtag, error)
	GetHashtagStats(ctx context.Context, hashtag string) (any, error)
	GetHashtagTimelineAdvanced(ctx context.Context, hashtag string, maxID *string, limit int, visibility string) ([]*storage.StatusSearchResult, error)
	UpdateHashtagNotificationSettings(ctx context.Context, userID, hashtag string, settings *storage.HashtagNotificationSettings) error
	MuteHashtag(ctx context.Context, userID, hashtag string, until *time.Time) error
	IsHashtagMuted(ctx context.Context, userID, hashtag string) (bool, error)
	GetHashtagNotificationSettings(ctx context.Context, userID, hashtag string) (*storage.HashtagNotificationSettings, error)
}

// Service coordinates hashtag follow/mute state with storage repositories and the streaming layer.
type Service struct {
	hashtagRepo HashtagRepository
	accountRepo interfaces.AccountRepository
	objectRepo  *repositories.ObjectRepository
	publisher   streaming.Publisher
	logger      *zap.Logger
}

// Hashtag captures the service-level representation of a hashtag enriched with viewer state.
type Hashtag struct {
	Name                 string
	URL                  string
	PostCount            int
	FollowerCount        int
	TrendingScore        float64
	Related              []string
	IsFollowing          bool
	IsMuted              bool
	FollowedAt           *time.Time
	NotificationSettings *storage.HashtagNotificationSettings
	Stats                *storage.HashtagStats
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ActivityEvent represents a hashtag-related streaming event forwarded from the global bus.
type ActivityEvent struct {
	Hashtag   string
	StatusID  string
	ActorID   string
	Timestamp time.Time
	Event     *streaming.InternalEvent
}

// NewService wires repositories and infrastructure needed for the hashtag service.
func NewService(
	hashtagRepo HashtagRepository,
	accountRepo interfaces.AccountRepository,
	objectRepo *repositories.ObjectRepository,
	publisher streaming.Publisher,
	logger *zap.Logger,
) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{
		hashtagRepo: hashtagRepo,
		accountRepo: accountRepo,
		objectRepo:  objectRepo,
		publisher:   publisher,
		logger:      logger,
	}
}

// GetHashtag loads the latest hashtag metadata plus viewer specific state.
func (s *Service) GetHashtag(ctx context.Context, name, viewerID string) (*Hashtag, error) {
	tag := normalizeHashtagName(name)
	if tag == "" {
		return nil, ErrHashtagNameRequired
	}

	info, err := s.hashtagRepo.GetHashtagInfo(ctx, tag)
	if err != nil {
		s.logger.Error("failed to fetch hashtag info",
			zap.String("hashtag", tag),
			zap.Error(err))
		return nil, ErrGetHashtag
	}

	stats := s.loadHashtagStats(ctx, tag)

	isFollowing := false
	var settings *storage.HashtagNotificationSettings
	isMuted := false

	if viewerID != "" {
		isFollowing, err = s.hashtagRepo.IsFollowingHashtag(ctx, viewerID, tag)
		if err != nil {
			s.logger.Error("failed to check follow relationship",
				zap.String("viewer", viewerID),
				zap.String("hashtag", tag),
				zap.Error(err))
			return nil, ErrGetHashtag
		}

		settings, err = s.hashtagRepo.GetHashtagNotificationSettings(ctx, viewerID, tag)
		if err != nil {
			s.logger.Error("failed to load hashtag notification settings",
				zap.String("viewer", viewerID),
				zap.String("hashtag", tag),
				zap.Error(err))
			return nil, ErrGetHashtag
		}

		isMuted, err = s.hashtagRepo.IsHashtagMuted(ctx, viewerID, tag)
		if err != nil {
			s.logger.Error("failed to check hashtag mute status",
				zap.String("viewer", viewerID),
				zap.String("hashtag", tag),
				zap.Error(err))
			return nil, ErrGetHashtag
		}
	}

	result := &Hashtag{
		Name:                 tag,
		IsFollowing:          isFollowing,
		IsMuted:              isMuted,
		NotificationSettings: settings,
		Stats:                stats,
	}

	if info != nil {
		result.URL = info.URL
		result.PostCount = info.UsageCount
		result.FollowerCount = info.Accounts
		result.CreatedAt = info.CreatedAt
		result.UpdatedAt = info.UpdatedAt
	}

	if stats != nil {
		if stats.UsageCount > result.PostCount {
			result.PostCount = stats.UsageCount
		}
		if int(stats.TotalAccounts) > result.FollowerCount {
			result.FollowerCount = int(stats.TotalAccounts)
		}
		result.TrendingScore = stats.TrendingScore
	}

	result.Related = s.relatedHashtags(ctx, tag, 5)

	return result, nil
}

// FollowHashtag creates a follow record, optionally updates notification settings, and emits streaming events.
func (s *Service) FollowHashtag(ctx context.Context, userID, hashtag string, settings *storage.HashtagNotificationSettings) (*Hashtag, error) {
	tag := normalizeHashtagName(hashtag)
	if tag == "" {
		return nil, ErrHashtagNameRequired
	}

	if err := s.hashtagRepo.FollowHashtag(ctx, userID, tag); err != nil {
		s.logger.Error("failed to follow hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tag),
			zap.Error(err))
		return nil, ErrFollowHashtag
	}

	if settings != nil {
		clone := cloneNotificationSettings(settings, userID, tag)
		if err := s.hashtagRepo.UpdateHashtagNotificationSettings(ctx, userID, tag, clone); err != nil {
			s.logger.Error("failed to apply hashtag notification settings",
				zap.String("user_id", userID),
				zap.String("hashtag", tag),
				zap.Error(err))
			return nil, ErrFollowHashtag
		}
	}

	s.publishUserEvent(ctx, streaming.HashtagFollowed, userID, tag)
	s.publishInternalHashtagEvent(streaming.ActionFollow, tag)

	return s.GetHashtag(ctx, tag, userID)
}

// UnfollowHashtag deletes a follow record and notifies streaming consumers.
func (s *Service) UnfollowHashtag(ctx context.Context, userID, hashtag string) (*Hashtag, error) {
	tag := normalizeHashtagName(hashtag)
	if tag == "" {
		return nil, ErrHashtagNameRequired
	}

	if err := s.hashtagRepo.UnfollowHashtag(ctx, userID, tag); err != nil {
		s.logger.Error("failed to unfollow hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tag),
			zap.Error(err))
		return nil, ErrUnfollowHashtag
	}

	s.publishUserEvent(ctx, "hashtag.unfollowed", userID, tag)
	s.publishInternalHashtagEvent(streaming.ActionUnfollow, tag)

	return s.GetHashtag(ctx, tag, userID)
}

// GetFollowedHashtags returns the viewer's followed hashtags enriched with current metadata.
func (s *Service) GetFollowedHashtags(
	ctx context.Context,
	userID string,
	pagination *interfaces.PaginationOptions,
) ([]*Hashtag, string, error) {
	if userID == "" {
		return nil, "", ErrHashtagNameRequired
	}

	limit := defaultFollowedHashtagLimit
	cursor := ""
	if pagination != nil {
		if pagination.Limit > 0 {
			limit = pagination.Limit
		}
		cursor = pagination.Cursor
	}

	if limit <= 0 {
		limit = defaultFollowedHashtagLimit
	}
	if limit > maxFollowedHashtagLimit {
		limit = maxFollowedHashtagLimit
	}

	follows, nextCursor, err := s.hashtagRepo.GetFollowedHashtags(ctx, userID, limit, cursor)
	if err != nil {
		s.logger.Error("failed to load followed hashtags",
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, "", ErrGetFollowedHashtags
	}

	results := make([]*Hashtag, 0, len(follows))
	for _, follow := range follows {
		if follow == nil {
			continue
		}
		tag, err := s.GetHashtag(ctx, follow.Hashtag, userID)
		if err != nil {
			s.logger.Warn("failed to enrich followed hashtag",
				zap.String("user_id", userID),
				zap.String("hashtag", follow.Hashtag),
				zap.Error(err))
			continue
		}
		followedAt := follow.CreatedAt
		tag.FollowedAt = &followedAt
		results = append(results, tag)
	}

	return results, nextCursor, nil
}

// MuteHashtag persists a mute entry for the user and broadcasts the change.
func (s *Service) MuteHashtag(ctx context.Context, userID, hashtag string, until *time.Time) (*Hashtag, error) {
	tag := normalizeHashtagName(hashtag)
	if tag == "" {
		return nil, ErrHashtagNameRequired
	}

	if err := s.hashtagRepo.MuteHashtag(ctx, userID, tag, until); err != nil {
		s.logger.Error("failed to mute hashtag",
			zap.String("user_id", userID),
			zap.String("hashtag", tag),
			zap.Error(err))
		return nil, ErrMuteHashtag
	}

	s.publishUserEvent(ctx, "hashtag.muted", userID, tag)
	s.publishInternalHashtagEvent(streaming.ActionUpdate, tag)

	return s.GetHashtag(ctx, tag, userID)
}

// GetHashtagActivity subscribes to hashtag-related events via DynamoDB-backed streaming.
// DEPRECATED: This method uses in-memory EventBus pattern that doesn't work on Lambda.
// Clients should use GraphQL subscriptions (SubscribeToHashtagActivity) instead,
// which properly persists subscriptions in DynamoDB and delivers via stream-router.
func (s *Service) GetHashtagActivity(ctx context.Context, hashtags []string) (<-chan *ActivityEvent, error) {
	cleaned := uniqueNormalizedHashtags(hashtags)
	if len(cleaned) == 0 {
		return nil, ErrHashtagNameRequired
	}

	// Return empty channel with deprecation warning
	// This functionality is replaced by GraphQL subscriptions in the graph layer
	s.logger.Warn("GetHashtagActivity called - this method is deprecated on Lambda, use GraphQL subscriptions instead",
		zap.Strings("hashtags", cleaned))

	out := make(chan *ActivityEvent, 100)
	close(out) // Close immediately to prevent blocking

	return out, nil
}

// loadHashtagStats attempts to load hashtag stats and logs conversion failures gracefully.
func (s *Service) loadHashtagStats(ctx context.Context, hashtag string) *storage.HashtagStats {
	statsAny, err := s.hashtagRepo.GetHashtagStats(ctx, hashtag)
	if err != nil {
		s.logger.Warn("failed to load hashtag stats",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return nil
	}
	if statsAny == nil {
		return nil
	}

	if stats, ok := statsAny.(*storage.HashtagStats); ok {
		return stats
	}

	s.logger.Warn("unexpected hashtag stats type",
		zap.String("hashtag", hashtag),
		zap.String("type", fmt.Sprintf("%T", statsAny)))
	return nil
}

// relatedHashtags derives co-occurring hashtags from recent timeline content.
func (s *Service) relatedHashtags(ctx context.Context, hashtag string, limit int) []string {
	if limit <= 0 {
		return []string{}
	}

	posts, err := s.hashtagRepo.GetHashtagTimelineAdvanced(ctx, hashtag, nil, defaultRelatedHashtagSampleSize, "")
	if err != nil {
		s.logger.Debug("failed to load related hashtag timeline sample",
			zap.String("hashtag", hashtag),
			zap.Error(err))
		return []string{}
	}

	frequency := make(map[string]int)
	current := strings.ToLower(hashtag)

	for _, post := range posts {
		if post == nil {
			continue
		}
		for _, tag := range extractHashtagsFromContent(post.Content) {
			tagLower := strings.ToLower(tag)
			if tagLower == "" || tagLower == current {
				continue
			}
			frequency[tagLower]++
		}
	}

	if len(frequency) == 0 {
		return []string{}
	}

	type pair struct {
		name  string
		count int
	}
	sorted := make([]pair, 0, len(frequency))
	for name, count := range frequency {
		sorted = append(sorted, pair{name: name, count: count})
	}

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	result := make([]string, 0, limit)
	for i := 0; i < len(sorted) && i < limit; i++ {
		result = append(result, sorted[i].name)
	}
	return result
}

// publishUserEvent notifies websocket clients about hashtag follow-related operations.
func (s *Service) publishUserEvent(ctx context.Context, eventType, userID, hashtag string) {
	if s.publisher == nil {
		return
	}

	event := streaming.NewEvent(eventType).
		ForStream(fmt.Sprintf("%s:%s", streaming.UserStream, userID)).
		WithData("user_id", userID).
		WithData("hashtag", hashtag).
		Build()

	if err := s.publisher.PublishToUser(ctx, userID, event); err != nil {
		s.logger.Warn("failed to publish user hashtag event",
			zap.String("type", eventType),
			zap.String("user_id", userID),
			zap.String("hashtag", hashtag),
			zap.Error(err))
	}

	// Broadcast to hashtag stream as well so hashtag timelines receive updates.
	if err := s.publisher.PublishToStream(ctx, streaming.HashtagStreamName(hashtag), event); err != nil {
		s.logger.Debug("failed to publish hashtag stream event",
			zap.String("type", eventType),
			zap.String("hashtag", hashtag),
			zap.Error(err))
	}
}

// publishInternalHashtagEvent sends a hashtag update event via the queue publisher to DynamoDB.
// The event will be picked up by stream-router and delivered to WebSocket subscribers.
func (s *Service) publishInternalHashtagEvent(action streaming.EventAction, hashtag string) {
	if s.publisher == nil {
		s.logger.Debug("publisher not available, skipping hashtag event",
			zap.String("hashtag", hashtag))
		return
	}

	// Build event payload for queue-based delivery
	event := &streaming.Event{
		Type:   string(streaming.EventTypeHashtagUpdate),
		Stream: streaming.HashtagStreamName(hashtag),
		Payload: map[string]interface{}{
			"hashtag":    hashtag,
			"action":     string(action),
			"count":      0,
			"period":     "realtime",
			"updated_at": time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now(),
	}

	// Publish to the hashtag stream - stream-router will fan out to subscribers
	if err := s.publisher.PublishToStream(context.Background(), event.Stream, event); err != nil {
		s.logger.Debug("failed to publish hashtag event",
			zap.String("hashtag", hashtag),
			zap.String("action", string(action)),
			zap.Error(err))
	}
}

// wrapActivityEvent converts a raw internal event to an ActivityEvent understood by service consumers.
func wrapActivityEvent(event *streaming.InternalEvent) *ActivityEvent {
	if event == nil {
		return nil
	}

	result := &ActivityEvent{
		Event:     event,
		Timestamp: event.Timestamp,
	}

	switch payload := event.Data.(type) {
	case *streaming.HashtagEventPayload:
		result.Hashtag = payload.Hashtag
		result.Timestamp = payload.UpdatedAt
	case *streaming.StatusEventPayload:
		if len(payload.Hashtags) > 0 {
			result.Hashtag = payload.Hashtags[0]
		}
		result.StatusID = payload.StatusID
		result.ActorID = payload.AuthorID
		result.Timestamp = payload.CreatedAt
	}

	return result
}

// cloneNotificationSettings creates a safe copy with enforced user/hashtag identity.
func cloneNotificationSettings(settings *storage.HashtagNotificationSettings, userID, hashtag string) *storage.HashtagNotificationSettings {
	if settings == nil {
		return nil
	}

	clone := *settings
	if len(settings.Filters) > 0 {
		clone.Filters = make([]*storage.NotificationFilter, len(settings.Filters))
		for i, filter := range settings.Filters {
			if filter == nil {
				continue
			}
			copied := *filter
			clone.Filters[i] = &copied
		}
	}
	if len(settings.Metadata) > 0 {
		copied := make(map[string]interface{}, len(settings.Metadata))
		for k, v := range settings.Metadata {
			copied[k] = v
		}
		clone.Metadata = copied
	}
	clone.UserID = userID
	clone.Hashtag = hashtag
	return &clone
}

// extractHashtagsFromContent performs basic hashtag parsing from the raw content.
func extractHashtagsFromContent(content string) []string {
	words := strings.Fields(content)
	results := make([]string, 0, len(words))

	for _, word := range words {
		if !strings.HasPrefix(word, "#") || len(word) == 1 {
			continue
		}
		tag := strings.Trim(word, "#.,!?;:()[]{}\"'")
		tag = normalizeHashtagName(tag)
		if tag != "" {
			results = append(results, tag)
		}
	}

	return results
}

// normalizeHashtagName lowercases and trims a hashtag identifier.
func normalizeHashtagName(name string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.ToLower(name), "#"))
}

// uniqueNormalizedHashtags removes duplicates while normalizing each hashtag.
func uniqueNormalizedHashtags(inputs []string) []string {
	dedupe := make(map[string]struct{})
	order := make([]string, 0, len(inputs))

	for _, raw := range inputs {
		tag := normalizeHashtagName(raw)
		if tag == "" {
			continue
		}
		if _, exists := dedupe[tag]; exists {
			continue
		}
		dedupe[tag] = struct{}{}
		order = append(order, tag)
	}

	return order
}

// buildHashtagStreams creates stream names for the given hashtags plus the global hashtag stream.
func buildHashtagStreams(hashtags []string) []string {
	streams := make([]string, 0, len(hashtags)+1)
	streams = append(streams, "hashtags:global")
	for _, tag := range hashtags {
		streams = append(streams, streaming.HashtagStreamName(tag))
	}
	return streams
}
