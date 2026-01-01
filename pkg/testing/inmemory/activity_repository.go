// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
)

// ActivityRepository is a thread-safe in-memory implementation of interfaces.ActivityRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type ActivityRepository struct {
	mu sync.RWMutex

	// Core activity storage
	activities map[string]*activityEntry // keyed by activity ID

	// Inbox index: username -> []activityID (sorted by timestamp desc)
	inboxByUser map[string][]string

	// Outbox index: username -> []activityID (sorted by timestamp desc)
	outboxByUser map[string][]string

	// Activity metrics
	activityMetrics map[string][]*activityMetric // actorID -> []metrics

	// Federation activity records
	federationActivities map[string][]*storage.FederationActivity // domain -> []activities

	// Weekly activity stats
	weeklyStats map[int64]*storage.WeeklyActivity // weekTimestamp -> stats
}

// activityEntry stores an activity with its metadata
type activityEntry struct {
	activity  *activitypub.Activity
	username  string // extracted from actor ID
	isInbox   bool   // true if this is an inbox activity
	createdAt time.Time
}

// activityMetric stores activity metric data
type activityMetric struct {
	activityType string
	actorID      string
	timestamp    time.Time
}

// NewActivityRepository creates a new in-memory activity repository
func NewActivityRepository() *ActivityRepository {
	return &ActivityRepository{
		activities:           make(map[string]*activityEntry),
		inboxByUser:          make(map[string][]string),
		outboxByUser:         make(map[string][]string),
		activityMetrics:      make(map[string][]*activityMetric),
		federationActivities: make(map[string][]*storage.FederationActivity),
		weeklyStats:          make(map[int64]*storage.WeeklyActivity),
	}
}

// ===== Core Activity Operations =====

// CreateActivity stores an activity in the database
func (r *ActivityRepository) CreateActivity(_ context.Context, activity *activitypub.Activity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if activity == nil || activity.ID == "" {
		return fmt.Errorf("activity ID is required")
	}

	// Extract username from actor ID
	username := extractUsernameFromActorID(activity.Actor)
	if username == "" {
		return fmt.Errorf("could not extract username from actor ID: %s", activity.Actor)
	}

	// Check if activity already exists
	if _, exists := r.activities[activity.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	isInbox := isInboxActivityCheck(activity, username)

	entry := &activityEntry{
		activity:  activity,
		username:  username,
		isInbox:   isInbox,
		createdAt: now,
	}

	r.activities[activity.ID] = entry

	// Index by inbox or outbox
	if isInbox {
		r.inboxByUser[username] = append([]string{activity.ID}, r.inboxByUser[username]...)
	} else {
		r.outboxByUser[username] = append([]string{activity.ID}, r.outboxByUser[username]...)
	}

	return nil
}

// GetActivity retrieves an activity by ID
func (r *ActivityRepository) GetActivity(_ context.Context, id string) (*activitypub.Activity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.activities[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return entry.activity, nil
}

// ===== Inbox/Outbox Operations =====

// GetInboxActivities retrieves inbox activities for a user with pagination
func (r *ActivityRepository) GetInboxActivities(_ context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	activityIDs := r.inboxByUser[username]
	if len(activityIDs) == 0 {
		return []*activitypub.Activity{}, "", nil
	}

	// Apply safe limit
	safeLimit := clampLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, id := range activityIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*activitypub.Activity
	var nextCursor string

	for i := startIdx; i < len(activityIDs) && len(results) < safeLimit; i++ {
		if entry, exists := r.activities[activityIDs[i]]; exists {
			results = append(results, entry.activity)
		}
	}

	// Set next cursor if there are more results
	if startIdx+safeLimit < len(activityIDs) {
		nextCursor = activityIDs[startIdx+safeLimit-1]
	}

	return results, nextCursor, nil
}

// GetOutboxActivities retrieves activities created by a user with pagination
func (r *ActivityRepository) GetOutboxActivities(_ context.Context, username string, limit int, cursor string) ([]*activitypub.Activity, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	activityIDs := r.outboxByUser[username]
	if len(activityIDs) == 0 {
		return []*activitypub.Activity{}, "", nil
	}

	// Apply safe limit
	safeLimit := clampLimit(limit)

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, id := range activityIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	var results []*activitypub.Activity
	var nextCursor string

	for i := startIdx; i < len(activityIDs) && len(results) < safeLimit; i++ {
		if entry, exists := r.activities[activityIDs[i]]; exists {
			results = append(results, entry.activity)
		}
	}

	// Set next cursor if there are more results
	if startIdx+safeLimit < len(activityIDs) {
		nextCursor = activityIDs[startIdx+safeLimit-1]
	}

	return results, nextCursor, nil
}

// ===== Collection Operations =====

// GetCollection retrieves a collection for an actor
func (r *ActivityRepository) GetCollection(ctx context.Context, username, collectionType string, limit int, cursor string) (*activitypub.OrderedCollectionPage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	baseURL := "https://example.com" // Default for testing
	actorID := fmt.Sprintf("%s/users/%s", baseURL, username)
	collectionID := fmt.Sprintf("%s/%s", actorID, collectionType)

	var activities []*activitypub.Activity
	var nextCursor string
	var err error

	switch collectionType {
	case activitypub.InboxCollection:
		// Need to release lock before calling GetInboxActivities
		r.mu.RUnlock()
		activities, nextCursor, err = r.GetInboxActivities(ctx, username, limit, cursor)
		r.mu.RLock()
		if err != nil {
			return nil, err
		}

	case activitypub.OutboxCollection:
		// Need to release lock before calling GetOutboxActivities
		r.mu.RUnlock()
		activities, nextCursor, err = r.GetOutboxActivities(ctx, username, limit, cursor)
		r.mu.RLock()
		if err != nil {
			return nil, err
		}

	case activitypub.FollowersCollection, activitypub.FollowingCollection, activitypub.LikedCollection:
		// These collections are handled by other repositories
		return nil, fmt.Errorf("collection type %s is handled by other repositories", collectionType)

	default:
		// Return empty collection for unknown types
		activities = []*activitypub.Activity{}
	}

	// Build the collection page
	page := &activitypub.OrderedCollectionPage{
		CollectionPage: activitypub.CollectionPage{
			Collection: activitypub.Collection{
				BaseObject: activitypub.BaseObject{
					Context: activitypub.Context,
					ID:      collectionID,
					Type:    activitypub.OrderedCollectionType,
				},
			},
			PartOf: collectionID,
		},
	}

	// Convert activities to interfaces
	items := make([]any, len(activities))
	for i, activity := range activities {
		items[i] = activity
	}
	page.OrderedItems = items
	page.TotalItems = len(items)

	// Set pagination info
	if nextCursor != "" {
		page.Next = fmt.Sprintf("%s?cursor=%s&limit=%d", collectionID, nextCursor, limit)
	}

	return page, nil
}

// ===== Analytics and Metrics Operations =====

// GetWeeklyActivity retrieves weekly activity statistics
func (r *ActivityRepository) GetWeeklyActivity(_ context.Context, weekTimestamp int64) (*storage.WeeklyActivity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if we have cached stats
	if stats, exists := r.weeklyStats[weekTimestamp]; exists {
		return stats, nil
	}

	// Calculate stats from activities
	weekStart := time.Unix(weekTimestamp, 0)
	weekEnd := weekStart.Add(7 * 24 * time.Hour)

	statuses := 0
	logins := 0
	registrations := 0

	for _, entry := range r.activities {
		if entry.createdAt.After(weekStart) && entry.createdAt.Before(weekEnd) {
			if entry.activity != nil && entry.activity.Type == "Create" {
				statuses++
			}
		}
	}

	return &storage.WeeklyActivity{
		Week:          fmt.Sprintf("%d", weekTimestamp),
		Statuses:      statuses,
		Logins:        logins,
		Registrations: registrations,
	}, nil
}

// RecordActivity records general activity metrics
func (r *ActivityRepository) RecordActivity(_ context.Context, activityType string, actorID string, timestamp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	metric := &activityMetric{
		activityType: activityType,
		actorID:      actorID,
		timestamp:    timestamp,
	}

	r.activityMetrics[actorID] = append(r.activityMetrics[actorID], metric)
	return nil
}

// GetHashtagActivity retrieves activities related to a hashtag since a given time
func (r *ActivityRepository) GetHashtagActivity(_ context.Context, hashtag string, since time.Time) ([]*storage.Activity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.Activity
	hashtagLower := strings.ToLower("#" + hashtag)

	for _, entry := range r.activities {
		if entry.createdAt.Before(since) {
			continue
		}

		if entry.activity == nil {
			continue
		}

		// Check if activity contains the hashtag
		content := extractActivityContent(entry.activity)
		if strings.Contains(strings.ToLower(content), hashtagLower) {
			results = append(results, &storage.Activity{
				ID:        entry.activity.ID,
				Type:      entry.activity.Type,
				Actor:     entry.activity.Actor,
				Object:    fmt.Sprintf("%v", entry.activity.Object),
				Published: entry.createdAt,
				Content:   content,
			})
		}
	}

	// Sort by timestamp descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Published.After(results[j].Published)
	})

	return results, nil
}

// ===== Federation Operations =====

// RecordFederationActivity records federation activity metrics
func (r *ActivityRepository) RecordFederationActivity(_ context.Context, activity *storage.FederationActivity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if activity == nil || activity.Domain == "" {
		return fmt.Errorf("federation activity domain is required")
	}

	r.federationActivities[activity.Domain] = append(r.federationActivities[activity.Domain], activity)
	return nil
}

// ===== Helper Functions =====

// extractUsernameFromActorID extracts username from an actor ID
// e.g., "https://example.com/users/alice" -> "alice"
func extractUsernameFromActorID(actorID string) string {
	parts := strings.Split(actorID, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

// isInboxActivityCheck determines if an activity should be stored in the inbox
func isInboxActivityCheck(activity *activitypub.Activity, localUsername string) bool {
	actorUsername := extractUsernameFromActorID(activity.Actor)
	return actorUsername != localUsername
}

// extractActivityContent extracts content from an ActivityPub activity
func extractActivityContent(activity *activitypub.Activity) string {
	if activity.Object == nil {
		return ""
	}

	// Try to extract content from object
	if objMap, ok := activity.Object.(map[string]interface{}); ok {
		if content, exists := objMap["content"]; exists {
			if contentStr, ok := content.(string); ok {
				return contentStr
			}
		}
	}
	return ""
}

// clampLimit ensures limit is within valid bounds
func clampLimit(limit int) int {
	const defaultLimit = 20
	const maxLimit = 100

	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// ===== Test Helper Methods =====

// SetWeeklyStats sets weekly stats for testing
func (r *ActivityRepository) SetWeeklyStats(weekTimestamp int64, stats *storage.WeeklyActivity) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.weeklyStats[weekTimestamp] = stats
}

// GetFederationActivities returns federation activities for a domain (test helper)
func (r *ActivityRepository) GetFederationActivities(domain string) []*storage.FederationActivity {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.federationActivities[domain]
}

// GetActivityMetrics returns activity metrics for an actor (test helper)
func (r *ActivityRepository) GetActivityMetrics(actorID string) []*activityMetric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.activityMetrics[actorID]
}

// Clear clears all data (test helper)
func (r *ActivityRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activities = make(map[string]*activityEntry)
	r.inboxByUser = make(map[string][]string)
	r.outboxByUser = make(map[string][]string)
	r.activityMetrics = make(map[string][]*activityMetric)
	r.federationActivities = make(map[string][]*storage.FederationActivity)
	r.weeklyStats = make(map[int64]*storage.WeeklyActivity)
}

// Ensure ActivityRepository implements interfaces.ActivityRepository
var _ interfaces.ActivityRepository = (*ActivityRepository)(nil)
