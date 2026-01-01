// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
)

// ModerationRepository is a thread-safe in-memory implementation of interfaces.ModerationRepository.
// It stores data in memory for integration-style testing without requiring DynamoDB.
type ModerationRepository struct {
	mu sync.RWMutex

	// Moderation events
	events map[string]*storage.ModerationEvent // keyed by event ID

	// Event indexes
	eventsByObject map[string][]string // objectID -> []eventID
	eventsByActor  map[string][]string // actorID -> []eventID

	// Moderation reviews
	reviews map[string][]*storage.ModerationReview // eventID -> []reviews

	// Moderation decisions
	decisions map[string]*storage.ModerationDecision // objectID -> decision

	// Moderation patterns
	patterns map[string]*storage.ModerationPattern // patternID -> pattern

	// Pattern match analytics
	patternMatches map[string][]*patternMatchRecord // patternID -> []matches

	// Filters
	filters        map[string]*storage.Filter        // filterID -> filter
	filtersByUser  map[string][]string               // username -> []filterID
	filterKeywords map[string][]*storage.FilterKeyword // filterID -> []keywords
	filterStatuses map[string][]*storage.FilterStatus  // filterID -> []statuses

	// Reports
	reports         map[string]*storage.Report // reportID -> report
	reportsByUser   map[string][]string        // username -> []reportID
	reportsByTarget map[string][]string        // targetAccountID -> []reportID
	reportsByStatus map[string][]string        // status -> []reportID
	reportStats     map[string]*storage.ReportStats // username -> stats

	// Flags
	flags          map[string]*storage.Flag // flagID -> flag
	flagsByObject  map[string][]string      // objectID -> []flagID
	flagsByActor   map[string][]string      // actorID -> []flagID
	pendingFlagIDs []string                 // list of pending flag IDs

	// Audit logs
	auditLogs        []*storage.AuditLog // ordered by timestamp
	auditLogsByAdmin map[string][]int    // adminID -> []index in auditLogs
	auditLogsByTarget map[string][]int   // targetID -> []index in auditLogs

	// Review queue
	reviewQueue []*models.ModerationReviewQueue

	// Decision results
	decisionResults map[string][]*models.ModerationDecisionResult // contentID -> []results

	// Analysis results
	analysisResults map[string]map[string]interface{} // contentID -> analysisData
}

// patternMatchRecord stores pattern match data
type patternMatchRecord struct {
	patternID string
	matched   bool
	timestamp time.Time
}

// NewModerationRepository creates a new in-memory moderation repository
func NewModerationRepository() *ModerationRepository {
	return &ModerationRepository{
		events:           make(map[string]*storage.ModerationEvent),
		eventsByObject:   make(map[string][]string),
		eventsByActor:    make(map[string][]string),
		reviews:          make(map[string][]*storage.ModerationReview),
		decisions:        make(map[string]*storage.ModerationDecision),
		patterns:         make(map[string]*storage.ModerationPattern),
		patternMatches:   make(map[string][]*patternMatchRecord),
		filters:          make(map[string]*storage.Filter),
		filtersByUser:    make(map[string][]string),
		filterKeywords:   make(map[string][]*storage.FilterKeyword),
		filterStatuses:   make(map[string][]*storage.FilterStatus),
		reports:          make(map[string]*storage.Report),
		reportsByUser:    make(map[string][]string),
		reportsByTarget:  make(map[string][]string),
		reportsByStatus:  make(map[string][]string),
		reportStats:      make(map[string]*storage.ReportStats),
		flags:            make(map[string]*storage.Flag),
		flagsByObject:    make(map[string][]string),
		flagsByActor:     make(map[string][]string),
		pendingFlagIDs:   []string{},
		auditLogs:        []*storage.AuditLog{},
		auditLogsByAdmin: make(map[string][]int),
		auditLogsByTarget: make(map[string][]int),
		reviewQueue:      []*models.ModerationReviewQueue{},
		decisionResults:  make(map[string][]*models.ModerationDecisionResult),
		analysisResults:  make(map[string]map[string]interface{}),
	}
}

// ===== Moderation Event Operations =====

// CreateModerationEvent creates a new moderation event
func (r *ModerationRepository) CreateModerationEvent(_ context.Context, event *storage.ModerationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if event == nil {
		return fmt.Errorf("event is required")
	}

	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%s", uuid.New().String()[:12])
	}

	if _, exists := r.events[event.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	event.Created = now
	event.Updated = now

	r.events[event.ID] = event
	r.eventsByObject[event.ObjectID] = append(r.eventsByObject[event.ObjectID], event.ID)
	r.eventsByActor[event.ActorID] = append(r.eventsByActor[event.ActorID], event.ID)

	return nil
}

// GetModerationEvent retrieves a moderation event by ID
func (r *ModerationRepository) GetModerationEvent(_ context.Context, eventID string) (*storage.ModerationEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	event, exists := r.events[eventID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return event, nil
}

// GetModerationEvents retrieves moderation events with optional filters
func (r *ModerationRepository) GetModerationEvents(_ context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.ModerationEvent

	for _, event := range r.events {
		if filter != nil {
			if filter.EventType != "" && event.EventType != filter.EventType {
				continue
			}
			if filter.Category != "" && event.Category != filter.Category {
				continue
			}
			if filter.ObjectID != "" && event.ObjectID != filter.ObjectID {
				continue
			}
			if filter.ActorID != "" && event.ActorID != filter.ActorID {
				continue
			}
		}
		results = append(results, event)
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created.After(results[j].Created)
	})

	return paginateEvents(results, limit, cursor)
}

// GetModerationEventsByObject retrieves all moderation events for an object
func (r *ModerationRepository) GetModerationEventsByObject(_ context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	eventIDs := r.eventsByObject[objectID]
	var results []*storage.ModerationEvent

	for _, id := range eventIDs {
		if event, exists := r.events[id]; exists {
			results = append(results, event)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created.After(results[j].Created)
	})

	return paginateEvents(results, limit, cursor)
}

// GetModerationEventsByActor retrieves all moderation events created by an actor
func (r *ModerationRepository) GetModerationEventsByActor(_ context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	eventIDs := r.eventsByActor[actorID]
	var results []*storage.ModerationEvent

	for _, id := range eventIDs {
		if event, exists := r.events[id]; exists {
			results = append(results, event)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created.After(results[j].Created)
	})

	return paginateEvents(results, limit, cursor)
}

// ===== Moderation Queue Operations =====

// GetModerationQueue retrieves pending moderation events
func (r *ModerationRepository) GetModerationQueue(_ context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []*storage.ModerationQueueItem

	for _, event := range r.events {
		if filter != nil {
			if filter.ContentType != "" && event.ObjectType != filter.ContentType {
				continue
			}
		}

		item := &storage.ModerationQueueItem{
			Event:       event,
			Priority:    int(getSeverityValue(event.Severity) * event.ConfidenceScore),
			ReviewCount: len(r.reviews[event.ID]),
		}
		items = append(items, item)
	}

	// Sort by priority descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].Priority > items[j].Priority
	})

	limit := 50
	if filter != nil && filter.Limit > 0 {
		limit = filter.Limit
	}

	if len(items) > limit {
		items = items[:limit]
	}

	return items, nil
}

// GetModerationQueuePaginated retrieves pending moderation events with pagination
func (r *ModerationRepository) GetModerationQueuePaginated(_ context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []*storage.ModerationQueueItem

	for _, event := range r.events {
		item := &storage.ModerationQueueItem{
			Event:       event,
			Priority:    int(getSeverityValue(event.Severity) * event.ConfidenceScore),
			ReviewCount: len(r.reviews[event.ID]),
		}
		items = append(items, item)
	}

	// Sort by priority descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].Priority > items[j].Priority
	})

	// Apply pagination
	startIdx := 0
	if cursor != "" {
		for i, item := range items {
			if item.Event.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	safeLimit := clampLimit(limit)
	endIdx := startIdx + safeLimit
	if endIdx > len(items) {
		endIdx = len(items)
	}

	result := items[startIdx:endIdx]
	var nextCursor string
	if endIdx < len(items) {
		nextCursor = items[endIdx-1].Event.ID
	}

	return result, nextCursor, nil
}

// GetModerationQueueCount returns the count of items in the moderation queue
func (r *ModerationRepository) GetModerationQueueCount(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.events), nil
}

// ===== Moderation Review Operations =====

// AddModerationReview adds a review to a moderation event
func (r *ModerationRepository) AddModerationReview(_ context.Context, review *storage.ModerationReview) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if review == nil {
		return fmt.Errorf("review is required")
	}

	if review.ID == "" {
		review.ID = fmt.Sprintf("rev_%s", uuid.New().String()[:12])
	}

	review.Created = time.Now()
	r.reviews[review.EventID] = append(r.reviews[review.EventID], review)

	return nil
}

// GetModerationReviews retrieves all reviews for a moderation event
func (r *ModerationRepository) GetModerationReviews(_ context.Context, eventID string) ([]*storage.ModerationReview, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reviews := r.reviews[eventID]
	if reviews == nil {
		return []*storage.ModerationReview{}, nil
	}

	return reviews, nil
}

// CreateAdminReview creates an admin review that overrides consensus
func (r *ModerationRepository) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	review := &storage.ModerationReview{
		ID:          fmt.Sprintf("admin_rev_%s", uuid.New().String()[:12]),
		EventID:     eventID,
		ReviewerID:  adminID,
		Action:      string(action),
		Severity:    "critical",
		Confidence:  1.0,
		Note:        fmt.Sprintf("Admin override: %s", reason),
		ReviewerRep: 1000.0,
	}

	return r.AddModerationReview(ctx, review)
}

// GetReviewerStats retrieves statistics for a reviewer
func (r *ModerationRepository) GetReviewerStats(_ context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &storage.ReviewerStats{
		ReviewerID:        reviewerID,
		TotalReviews:      0,
		AccurateReviews:   0,
		AccuracyRate:      0.0,
		ReviewsByCategory: make(map[string]int),
		JoinedAt:          time.Now(),
	}

	for _, reviews := range r.reviews {
		for _, review := range reviews {
			if review.ReviewerID == reviewerID {
				stats.TotalReviews++
				stats.ReviewsByCategory[review.Severity]++
				if review.ReviewerRep > 0.5 {
					stats.AccurateReviews++
				}
				if review.Created.After(stats.LastReviewAt) {
					stats.LastReviewAt = review.Created
				}
			}
		}
	}

	if stats.TotalReviews > 0 {
		stats.AccuracyRate = float64(stats.AccurateReviews) / float64(stats.TotalReviews)
	}

	return stats, nil
}


// ===== Moderation Decision Operations =====

// CreateModerationDecision creates a consensus decision
func (r *ModerationRepository) CreateModerationDecision(_ context.Context, decision *storage.ModerationDecision) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if decision == nil {
		return fmt.Errorf("decision is required")
	}

	if decision.ID == "" {
		decision.ID = fmt.Sprintf("dec_%s", uuid.New().String()[:12])
	}

	decision.Decided = time.Now()
	r.decisions[decision.ObjectID] = decision

	return nil
}

// GetModerationDecision retrieves the current decision for an object
func (r *ModerationRepository) GetModerationDecision(_ context.Context, objectID string) (*storage.ModerationDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	decision, exists := r.decisions[objectID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return decision, nil
}

// StoreModerationDecision stores a moderation decision (alias for CreateModerationDecision)
func (r *ModerationRepository) StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	return r.CreateModerationDecision(ctx, decision)
}

// UpdateModerationDecision updates a moderation decision based on a review
func (r *ModerationRepository) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	currentDecision, exists := r.decisions[contentID]
	if !exists {
		return storage.ErrNotFound
	}

	newDecision := &storage.ModerationDecision{
		ID:               fmt.Sprintf("dec_%s", uuid.New().String()[:12]),
		EventID:          currentDecision.EventID,
		ObjectID:         contentID,
		Action:           review.Action,
		ConsensusScore:   review.Confidence,
		ReviewerCount:    1,
		TrustWeightTotal: review.Confidence,
		Reviews:          []interface{}{review.ID},
		Decided:          time.Now(),
	}

	r.decisions[contentID] = newDecision
	return nil
}

// ===== Moderation Pattern Operations =====

// CreateModerationPattern creates a new moderation pattern
func (r *ModerationRepository) CreateModerationPattern(_ context.Context, pattern *storage.ModerationPattern) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if pattern == nil {
		return fmt.Errorf("pattern is required")
	}

	if pattern.ID == "" {
		pattern.ID = fmt.Sprintf("pat_%s", uuid.New().String()[:12])
	}

	if _, exists := r.patterns[pattern.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	pattern.CreatedAt = now
	pattern.UpdatedAt = now

	r.patterns[pattern.ID] = pattern
	return nil
}

// GetModerationPattern retrieves a specific moderation pattern
func (r *ModerationRepository) GetModerationPattern(_ context.Context, patternID string) (*storage.ModerationPattern, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pattern, exists := r.patterns[patternID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return pattern, nil
}

// GetModerationPatterns retrieves moderation patterns based on criteria
func (r *ModerationRepository) GetModerationPatterns(_ context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.ModerationPattern

	for _, pattern := range r.patterns {
		if active && !pattern.Active {
			continue
		}
		if severity != "" && pattern.Severity != severity {
			continue
		}
		results = append(results, pattern)
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// UpdateModerationPattern updates an existing moderation pattern
func (r *ModerationRepository) UpdateModerationPattern(_ context.Context, pattern *storage.ModerationPattern) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.patterns[pattern.ID]; !exists {
		return storage.ErrNotFound
	}

	pattern.UpdatedAt = time.Now()
	r.patterns[pattern.ID] = pattern
	return nil
}

// DeleteModerationPattern deletes a moderation pattern
func (r *ModerationRepository) DeleteModerationPattern(_ context.Context, patternID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.patterns[patternID]; !exists {
		return storage.ErrNotFound
	}

	delete(r.patterns, patternID)
	return nil
}

// RecordPatternMatch records a moderation pattern match for analytics
func (r *ModerationRepository) RecordPatternMatch(_ context.Context, patternID string, matched bool, timestamp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record := &patternMatchRecord{
		patternID: patternID,
		matched:   matched,
		timestamp: timestamp,
	}

	r.patternMatches[patternID] = append(r.patternMatches[patternID], record)
	return nil
}

// ===== Moderation History Operations =====

// GetModerationHistory retrieves the complete moderation history for an object
func (r *ModerationRepository) GetModerationHistory(_ context.Context, objectID string) (*storage.ModerationHistory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	history := &storage.ModerationHistory{
		ObjectID:  objectID,
		Events:    []storage.ModerationEvent{},
		Decisions: []storage.ModerationDecision{},
		Timeline:  []storage.ModerationTimelineEntry{},
	}

	// Get events for this object
	eventIDs := r.eventsByObject[objectID]
	for _, id := range eventIDs {
		if event, exists := r.events[id]; exists {
			history.Events = append(history.Events, *event)
			history.Timeline = append(history.Timeline, storage.ModerationTimelineEntry{
				Timestamp:   event.Created,
				Type:        "event",
				ActorID:     event.ActorID,
				Description: fmt.Sprintf("%s event: %s", event.EventType, event.Category),
				Metadata: map[string]any{
					"event_id": event.ID,
					"severity": event.Severity,
				},
			})
		}
	}

	// Get decision for this object
	if decision, exists := r.decisions[objectID]; exists {
		history.Decisions = append(history.Decisions, *decision)
		history.CurrentStatus = decision.Action
		history.Timeline = append(history.Timeline, storage.ModerationTimelineEntry{
			Timestamp:   decision.Decided,
			Type:        "decision",
			ActorID:     "system",
			Description: fmt.Sprintf("Decision: %s (consensus: %.2f)", decision.Action, decision.ConsensusScore),
			Metadata: map[string]any{
				"decision_id": decision.ID,
				"action":      decision.Action,
			},
		})
	} else {
		history.CurrentStatus = "pending"
	}

	return history, nil
}

// ===== Filter Operations =====

// CreateFilter creates a new filter
func (r *ModerationRepository) CreateFilter(_ context.Context, filter *storage.Filter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if filter == nil {
		return fmt.Errorf("filter is required")
	}

	if filter.ID == "" {
		filter.ID = uuid.New().String()
	}

	if _, exists := r.filters[filter.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	filter.CreatedAt = now
	filter.UpdatedAt = now

	r.filters[filter.ID] = filter
	r.filtersByUser[filter.Username] = append(r.filtersByUser[filter.Username], filter.ID)

	return nil
}

// GetFilter retrieves a filter by ID
func (r *ModerationRepository) GetFilter(_ context.Context, filterID string) (*storage.Filter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filter, exists := r.filters[filterID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return filter, nil
}

// GetFiltersForUser retrieves all filters for a user
func (r *ModerationRepository) GetFiltersForUser(_ context.Context, username string) ([]*storage.Filter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	filterIDs := r.filtersByUser[username]
	var results []*storage.Filter

	for _, id := range filterIDs {
		if filter, exists := r.filters[id]; exists {
			results = append(results, filter)
		}
	}

	return results, nil
}

// UpdateFilter updates a filter
func (r *ModerationRepository) UpdateFilter(_ context.Context, filterID string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filter, exists := r.filters[filterID]
	if !exists {
		return storage.ErrNotFound
	}

	if title, ok := updates["title"].(string); ok {
		filter.Title = title
	}
	if context, ok := updates["context"].([]string); ok {
		filter.Context = context
	}
	if filterAction, ok := updates["filter_action"].(string); ok {
		filter.FilterAction = filterAction
	}
	if expiresAt, ok := updates["expires_at"].(*time.Time); ok {
		filter.ExpiresAt = expiresAt
	}

	filter.UpdatedAt = time.Now()
	r.filters[filterID] = filter

	return nil
}

// DeleteFilter deletes a filter and all its associated keywords and statuses
func (r *ModerationRepository) DeleteFilter(_ context.Context, filterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	filter, exists := r.filters[filterID]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from user index
	userFilters := r.filtersByUser[filter.Username]
	for i, id := range userFilters {
		if id == filterID {
			r.filtersByUser[filter.Username] = append(userFilters[:i], userFilters[i+1:]...)
			break
		}
	}

	// Delete keywords and statuses
	delete(r.filterKeywords, filterID)
	delete(r.filterStatuses, filterID)
	delete(r.filters, filterID)

	return nil
}

// AddFilterKeyword adds a new keyword to a filter
func (r *ModerationRepository) AddFilterKeyword(_ context.Context, filterID string, keyword *storage.FilterKeyword) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if keyword == nil {
		return fmt.Errorf("keyword is required")
	}

	if keyword.ID == "" {
		keyword.ID = uuid.New().String()
	}

	keyword.FilterID = filterID
	keyword.CreatedAt = time.Now()

	r.filterKeywords[filterID] = append(r.filterKeywords[filterID], keyword)
	return nil
}

// GetFilterKeywords retrieves all keywords for a filter
func (r *ModerationRepository) GetFilterKeywords(_ context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keywords := r.filterKeywords[filterID]
	if keywords == nil {
		return []*storage.FilterKeyword{}, nil
	}

	return keywords, nil
}

// UpdateFilterKeyword updates a filter keyword
func (r *ModerationRepository) UpdateFilterKeyword(_ context.Context, keywordID string, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for filterID, keywords := range r.filterKeywords {
		for i, keyword := range keywords {
			if keyword.ID == keywordID {
				if kw, ok := updates["keyword"].(string); ok {
					keyword.Keyword = kw
				}
				if wholeWord, ok := updates["whole_word"].(bool); ok {
					keyword.WholeWord = wholeWord
				}
				r.filterKeywords[filterID][i] = keyword
				return nil
			}
		}
	}

	return storage.ErrNotFound
}

// DeleteFilterKeyword deletes a filter keyword
func (r *ModerationRepository) DeleteFilterKeyword(_ context.Context, keywordID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for filterID, keywords := range r.filterKeywords {
		for i, keyword := range keywords {
			if keyword.ID == keywordID {
				r.filterKeywords[filterID] = append(keywords[:i], keywords[i+1:]...)
				return nil
			}
		}
	}

	return storage.ErrNotFound
}

// AddFilterStatus adds a new status to a filter
func (r *ModerationRepository) AddFilterStatus(_ context.Context, filterID string, status *storage.FilterStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if status == nil {
		return fmt.Errorf("status is required")
	}

	if status.ID == "" {
		status.ID = uuid.New().String()
	}

	status.FilterID = filterID
	status.CreatedAt = time.Now()

	r.filterStatuses[filterID] = append(r.filterStatuses[filterID], status)
	return nil
}

// GetFilterStatuses retrieves all statuses for a filter
func (r *ModerationRepository) GetFilterStatuses(_ context.Context, filterID string) ([]*storage.FilterStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := r.filterStatuses[filterID]
	if statuses == nil {
		return []*storage.FilterStatus{}, nil
	}

	return statuses, nil
}

// DeleteFilterStatus deletes a filter status
func (r *ModerationRepository) DeleteFilterStatus(_ context.Context, statusID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for filterID, statuses := range r.filterStatuses {
		for i, status := range statuses {
			if status.StatusID == statusID {
				r.filterStatuses[filterID] = append(statuses[:i], statuses[i+1:]...)
				return nil
			}
		}
	}

	return storage.ErrNotFound
}


// ===== Report Operations =====

// CreateReport creates a new report
func (r *ModerationRepository) CreateReport(_ context.Context, report *storage.Report) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if report == nil {
		return fmt.Errorf("report is required")
	}

	if report.ID == "" {
		report.ID = fmt.Sprintf("report_%s", uuid.New().String()[:12])
	}

	if _, exists := r.reports[report.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	report.CreatedAt = now
	report.UpdatedAt = now

	if report.Status == "" {
		report.Status = "open"
	}

	r.reports[report.ID] = report
	r.reportsByUser[report.ReporterID] = append(r.reportsByUser[report.ReporterID], report.ID)
	r.reportsByTarget[report.TargetAccountID] = append(r.reportsByTarget[report.TargetAccountID], report.ID)
	r.reportsByStatus[report.Status] = append(r.reportsByStatus[report.Status], report.ID)

	return nil
}

// GetReport retrieves a report by ID
func (r *ModerationRepository) GetReport(_ context.Context, id string) (*storage.Report, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	report, exists := r.reports[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return report, nil
}

// GetUserReports retrieves all reports created by a user
func (r *ModerationRepository) GetUserReports(_ context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reportIDs := r.reportsByUser[username]
	var results []*storage.Report

	for _, id := range reportIDs {
		if report, exists := r.reports[id]; exists {
			results = append(results, report)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return paginateReports(results, limit, cursor)
}

// GetReportsByTarget retrieves reports targeting a specific account
func (r *ModerationRepository) GetReportsByTarget(_ context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reportIDs := r.reportsByTarget[targetAccountID]
	var results []*storage.Report

	for _, id := range reportIDs {
		if report, exists := r.reports[id]; exists {
			results = append(results, report)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return paginateReports(results, limit, cursor)
}

// GetReportsByStatus retrieves reports with a specific status
func (r *ModerationRepository) GetReportsByStatus(_ context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reportIDs := r.reportsByStatus[string(status)]
	var results []*storage.Report

	for _, id := range reportIDs {
		if report, exists := r.reports[id]; exists {
			results = append(results, report)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return paginateReports(results, limit, cursor)
}

// UpdateReportStatus updates the status of a report
func (r *ModerationRepository) UpdateReportStatus(_ context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	report, exists := r.reports[id]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from old status index
	oldStatusReports := r.reportsByStatus[report.Status]
	for i, rid := range oldStatusReports {
		if rid == id {
			r.reportsByStatus[report.Status] = append(oldStatusReports[:i], oldStatusReports[i+1:]...)
			break
		}
	}

	// Update report
	now := time.Now()
	report.Status = string(status)
	report.ActionTaken = actionTaken
	report.ModeratorID = moderatorID
	report.UpdatedAt = now
	if actionTaken != "" {
		report.ActionTakenAt = &now
	}

	// Add to new status index
	r.reportsByStatus[string(status)] = append(r.reportsByStatus[string(status)], id)

	return nil
}

// AssignReport assigns a report to a moderator
func (r *ModerationRepository) AssignReport(_ context.Context, reportID string, assignedTo string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	report, exists := r.reports[reportID]
	if !exists {
		return storage.ErrNotFound
	}

	report.AssignedTo = assignedTo
	report.UpdatedAt = time.Now()

	return nil
}

// UnassignReport removes assignment from a report
func (r *ModerationRepository) UnassignReport(_ context.Context, reportID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	report, exists := r.reports[reportID]
	if !exists {
		return storage.ErrNotFound
	}

	report.AssignedTo = ""
	report.UpdatedAt = time.Now()

	return nil
}

// GetOpenReportsCount returns the count of open reports
func (r *ModerationRepository) GetOpenReportsCount(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.reportsByStatus["open"]), nil
}

// GetReportedStatuses retrieves statuses associated with a report
func (r *ModerationRepository) GetReportedStatuses(_ context.Context, reportID string) ([]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	report, exists := r.reports[reportID]
	if !exists {
		return nil, storage.ErrNotFound
	}

	result := make([]any, len(report.StatusIDs))
	for i, statusID := range report.StatusIDs {
		result[i] = statusID
	}

	return result, nil
}

// GetReportStats retrieves reporting statistics for a user
func (r *ModerationRepository) GetReportStats(_ context.Context, username string) (*storage.ReportStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats, exists := r.reportStats[username]
	if !exists {
		return &storage.ReportStats{
			TotalReports:    0,
			ResolvedReports: 0,
			FalseReports:    0,
			LastReportAt:    nil,
		}, nil
	}

	return stats, nil
}

// IncrementFalseReports increments the false report count for a user
func (r *ModerationRepository) IncrementFalseReports(_ context.Context, username string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stats, exists := r.reportStats[username]
	if !exists {
		stats = &storage.ReportStats{}
		r.reportStats[username] = stats
	}

	stats.FalseReports++
	now := time.Now()
	stats.LastReportAt = &now

	return nil
}

// ===== Flag Operations =====

// CreateFlag creates a new flag
func (r *ModerationRepository) CreateFlag(_ context.Context, flag *storage.Flag) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if flag == nil {
		return fmt.Errorf("flag is required")
	}

	if flag.ID == "" {
		flag.ID = fmt.Sprintf("flag_%s", uuid.New().String()[:12])
	}

	if _, exists := r.flags[flag.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	flag.CreatedAt = now
	if flag.Published.IsZero() {
		flag.Published = now
	}
	if flag.Status == "" {
		flag.Status = "pending"
	}

	r.flags[flag.ID] = flag

	// Index by objects
	for _, objectID := range flag.Object {
		r.flagsByObject[objectID] = append(r.flagsByObject[objectID], flag.ID)
	}

	// Index by actor
	r.flagsByActor[flag.Actor] = append(r.flagsByActor[flag.Actor], flag.ID)

	// Add to pending if status is pending
	if flag.Status == "pending" {
		r.pendingFlagIDs = append(r.pendingFlagIDs, flag.ID)
	}

	return nil
}

// GetFlag retrieves a flag by ID
func (r *ModerationRepository) GetFlag(_ context.Context, id string) (*storage.Flag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	flag, exists := r.flags[id]
	if !exists {
		return nil, storage.ErrNotFound
	}

	return flag, nil
}

// GetFlagsByObject retrieves all flags for a specific object
func (r *ModerationRepository) GetFlagsByObject(_ context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	flagIDs := r.flagsByObject[objectID]
	var results []*storage.Flag

	for _, id := range flagIDs {
		if flag, exists := r.flags[id]; exists {
			results = append(results, flag)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return paginateFlags(results, limit, cursor)
}

// GetFlagsByActor retrieves all flags created by a specific actor
func (r *ModerationRepository) GetFlagsByActor(_ context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	flagIDs := r.flagsByActor[actorID]
	var results []*storage.Flag

	for _, id := range flagIDs {
		if flag, exists := r.flags[id]; exists {
			results = append(results, flag)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return paginateFlags(results, limit, cursor)
}

// GetPendingFlags retrieves all pending flags
func (r *ModerationRepository) GetPendingFlags(_ context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*storage.Flag

	for _, id := range r.pendingFlagIDs {
		if flag, exists := r.flags[id]; exists {
			results = append(results, flag)
		}
	}

	// Sort by created time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	return paginateFlags(results, limit, cursor)
}

// UpdateFlagStatus updates the status of a flag
func (r *ModerationRepository) UpdateFlagStatus(_ context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	flag, exists := r.flags[id]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from pending if it was pending
	if flag.Status == "pending" {
		for i, fid := range r.pendingFlagIDs {
			if fid == id {
				r.pendingFlagIDs = append(r.pendingFlagIDs[:i], r.pendingFlagIDs[i+1:]...)
				break
			}
		}
	}

	now := time.Now()
	flag.Status = string(status)
	flag.ReviewedBy = reviewedBy
	flag.ReviewedAt = &now
	flag.ReviewNote = reviewNote

	// Add back to pending if new status is pending
	if status == storage.FlagStatusPending {
		r.pendingFlagIDs = append(r.pendingFlagIDs, id)
	}

	return nil
}

// CountPendingFlags returns the count of pending flags
func (r *ModerationRepository) CountPendingFlags(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.pendingFlagIDs), nil
}

// DeleteFlag removes a flag
func (r *ModerationRepository) DeleteFlag(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	flag, exists := r.flags[id]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from object index
	for _, objectID := range flag.Object {
		objectFlags := r.flagsByObject[objectID]
		for i, fid := range objectFlags {
			if fid == id {
				r.flagsByObject[objectID] = append(objectFlags[:i], objectFlags[i+1:]...)
				break
			}
		}
	}

	// Remove from actor index
	actorFlags := r.flagsByActor[flag.Actor]
	for i, fid := range actorFlags {
		if fid == id {
			r.flagsByActor[flag.Actor] = append(actorFlags[:i], actorFlags[i+1:]...)
			break
		}
	}

	// Remove from pending if applicable
	if flag.Status == "pending" {
		for i, fid := range r.pendingFlagIDs {
			if fid == id {
				r.pendingFlagIDs = append(r.pendingFlagIDs[:i], r.pendingFlagIDs[i+1:]...)
				break
			}
		}
	}

	delete(r.flags, id)
	return nil
}


// ===== Audit Log Operations =====

// CreateAuditLog creates a new audit log entry
func (r *ModerationRepository) CreateAuditLog(_ context.Context, auditLog *storage.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if auditLog == nil {
		return fmt.Errorf("audit log is required")
	}

	if auditLog.ID == "" {
		auditLog.ID = fmt.Sprintf("audit_%s", uuid.New().String()[:12])
	}

	auditLog.Timestamp = time.Now()
	auditLog.CreatedAt = auditLog.Timestamp

	idx := len(r.auditLogs)
	r.auditLogs = append(r.auditLogs, auditLog)

	// Index by admin
	r.auditLogsByAdmin[auditLog.AdminID] = append(r.auditLogsByAdmin[auditLog.AdminID], idx)

	// Index by target
	r.auditLogsByTarget[auditLog.TargetID] = append(r.auditLogsByTarget[auditLog.TargetID], idx)

	return nil
}

// GetAuditLogs retrieves audit log entries with pagination
func (r *ModerationRepository) GetAuditLogs(_ context.Context, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Reverse order (newest first)
	var results []*storage.AuditLog
	for i := len(r.auditLogs) - 1; i >= 0; i-- {
		results = append(results, r.auditLogs[i])
	}

	return paginateAuditLogs(results, limit, cursor)
}

// GetAuditLogsByAdmin retrieves audit log entries for a specific admin
func (r *ModerationRepository) GetAuditLogsByAdmin(_ context.Context, adminID string, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	indices := r.auditLogsByAdmin[adminID]
	var results []*storage.AuditLog

	// Reverse order (newest first)
	for i := len(indices) - 1; i >= 0; i-- {
		results = append(results, r.auditLogs[indices[i]])
	}

	return paginateAuditLogs(results, limit, cursor)
}

// GetAuditLogsByTarget retrieves audit log entries for a specific target
func (r *ModerationRepository) GetAuditLogsByTarget(_ context.Context, targetID string, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	indices := r.auditLogsByTarget[targetID]
	var results []*storage.AuditLog

	// Reverse order (newest first)
	for i := len(indices) - 1; i >= 0; i-- {
		results = append(results, r.auditLogs[indices[i]])
	}

	return paginateAuditLogs(results, limit, cursor)
}

// ===== Pending Moderation Operations =====

// GetPendingModerationCount returns the count of pending moderation tasks for a specific moderator
func (r *ModerationRepository) GetPendingModerationCount(_ context.Context, moderatorID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0

	// Count assigned reports that are open or in progress
	for _, report := range r.reports {
		if report.AssignedTo == moderatorID {
			if report.Status == "open" || report.Status == string(storage.ReportStatusInProgress) {
				count++
			}
		}
	}

	// Count assigned flags that are pending
	for _, flag := range r.flags {
		if flag.Status == "pending" {
			count++
		}
	}

	return count, nil
}

// ===== Analysis and Decision Storage Operations =====

// StoreAnalysisResult stores detailed analysis results for audit/appeals
func (r *ModerationRepository) StoreAnalysisResult(_ context.Context, analysisData map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	contentID, ok := analysisData["content_id"].(string)
	if !ok || contentID == "" {
		return fmt.Errorf("content_id is required")
	}

	r.analysisResults[contentID] = analysisData
	return nil
}

// StoreDecision stores a moderation decision with enforcement tracking
func (r *ModerationRepository) StoreDecision(_ context.Context, decisionData map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	contentID, ok := decisionData["content_id"].(string)
	if !ok || contentID == "" {
		return fmt.Errorf("content_id is required")
	}

	action, ok := decisionData["action"].(string)
	if !ok || action == "" {
		return fmt.Errorf("action is required")
	}

	result := &models.ModerationDecisionResult{
		ID:                fmt.Sprintf("decision_%d", time.Now().UnixNano()),
		ContentID:         contentID,
		Action:            action,
		DecidedAt:         time.Now(),
		EnforcementStatus: "pending",
	}

	if authorID, ok := decisionData["author_id"].(string); ok {
		result.AuthorID = authorID
	}
	if confidence, ok := decisionData["confidence"].(float64); ok {
		result.Confidence = confidence
	}
	if requiresReview, ok := decisionData["requires_review"].(bool); ok {
		result.RequiresReview = requiresReview
	}

	r.decisionResults[contentID] = append(r.decisionResults[contentID], result)

	// Add to review queue if requires review
	if result.RequiresReview {
		queueItem := &models.ModerationReviewQueue{
			ID:        fmt.Sprintf("queue_%d", time.Now().UnixNano()),
			ContentID: contentID,
			AuthorID:  result.AuthorID,
			Status:    "pending",
			Category:  "moderation",
			Severity:  "medium",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		r.reviewQueue = append(r.reviewQueue, queueItem)
	}

	return nil
}

// GetReviewQueue retrieves review queue items with filtering
func (r *ModerationRepository) GetReviewQueue(_ context.Context, filters map[string]interface{}) ([]*models.ModerationReviewQueue, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := "pending"
	if filterStatus, ok := filters["status"].(string); ok && filterStatus != "" {
		status = filterStatus
	}

	limit := 50
	if filterLimit, ok := filters["limit"].(int); ok && filterLimit > 0 {
		limit = filterLimit
	}

	var results []*models.ModerationReviewQueue
	for _, item := range r.reviewQueue {
		if item.Status == status {
			results = append(results, item)
		}
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetDecisionHistory retrieves decision history for a specific content ID
func (r *ModerationRepository) GetDecisionHistory(_ context.Context, contentID string) ([]*models.ModerationDecisionResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := r.decisionResults[contentID]
	if results == nil {
		return []*models.ModerationDecisionResult{}, nil
	}

	return results, nil
}

// UpdateEnforcementStatus updates the enforcement status of a decision
func (r *ModerationRepository) UpdateEnforcementStatus(_ context.Context, contentID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	results := r.decisionResults[contentID]
	if len(results) == 0 {
		return storage.ErrNotFound
	}

	// Update the most recent decision
	latestDecision := results[len(results)-1]
	latestDecision.EnforcementStatus = status
	now := time.Now()
	latestDecision.EnforcedAt = &now

	return nil
}

// GetModerationDecisionsByModerator retrieves moderation decisions made by a specific moderator
func (r *ModerationRepository) GetModerationDecisionsByModerator(_ context.Context, moderatorUsername string, limit int) ([]*models.ModerationReview, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.ModerationReview

	for _, reviews := range r.reviews {
		for _, review := range reviews {
			if review.ReviewerID == moderatorUsername {
				modelReview := &models.ModerationReview{
					ID:          review.ID,
					EventID:     review.EventID,
					ReviewerID:  review.ReviewerID,
					ReviewerRep: review.ReviewerRep,
					Action:      review.Action,
					Severity:    review.Severity,
					Note:        review.Note,
					Confidence:  review.Confidence,
					Created:     review.Created,
				}
				results = append(results, modelReview)
			}
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ===== Helper Functions =====

// getSeverityValue converts severity string to numeric value
func getSeverityValue(severity string) float64 {
	switch severity {
	case "low":
		return 1.0
	case "medium":
		return 2.0
	case "high":
		return 3.0
	case "critical":
		return 4.0
	default:
		return 1.0
	}
}

// paginateEvents applies pagination to event results
func paginateEvents(events []*storage.ModerationEvent, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	if len(events) == 0 {
		return []*storage.ModerationEvent{}, "", nil
	}

	safeLimit := clampLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, event := range events {
			if event.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + safeLimit
	if endIdx > len(events) {
		endIdx = len(events)
	}

	result := events[startIdx:endIdx]
	var nextCursor string
	if endIdx < len(events) {
		nextCursor = events[endIdx-1].ID
	}

	return result, nextCursor, nil
}

// paginateReports applies pagination to report results
func paginateReports(reports []*storage.Report, limit int, cursor string) ([]*storage.Report, string, error) {
	if len(reports) == 0 {
		return []*storage.Report{}, "", nil
	}

	safeLimit := clampLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, report := range reports {
			if report.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + safeLimit
	if endIdx > len(reports) {
		endIdx = len(reports)
	}

	result := reports[startIdx:endIdx]
	var nextCursor string
	if endIdx < len(reports) {
		nextCursor = reports[endIdx-1].ID
	}

	return result, nextCursor, nil
}

// paginateFlags applies pagination to flag results
func paginateFlags(flags []*storage.Flag, limit int, cursor string) ([]*storage.Flag, string, error) {
	if len(flags) == 0 {
		return []*storage.Flag{}, "", nil
	}

	safeLimit := clampLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, flag := range flags {
			if flag.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + safeLimit
	if endIdx > len(flags) {
		endIdx = len(flags)
	}

	result := flags[startIdx:endIdx]
	var nextCursor string
	if endIdx < len(flags) {
		nextCursor = flags[endIdx-1].ID
	}

	return result, nextCursor, nil
}

// paginateAuditLogs applies pagination to audit log results
func paginateAuditLogs(logs []*storage.AuditLog, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	if len(logs) == 0 {
		return []*storage.AuditLog{}, "", nil
	}

	safeLimit := clampLimit(limit)

	startIdx := 0
	if cursor != "" {
		for i, log := range logs {
			if log.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + safeLimit
	if endIdx > len(logs) {
		endIdx = len(logs)
	}

	result := logs[startIdx:endIdx]
	var nextCursor string
	if endIdx < len(logs) {
		nextCursor = logs[endIdx-1].ID
	}

	return result, nextCursor, nil
}

// ===== Test Helper Methods =====

// Clear clears all data (test helper)
func (r *ModerationRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = make(map[string]*storage.ModerationEvent)
	r.eventsByObject = make(map[string][]string)
	r.eventsByActor = make(map[string][]string)
	r.reviews = make(map[string][]*storage.ModerationReview)
	r.decisions = make(map[string]*storage.ModerationDecision)
	r.patterns = make(map[string]*storage.ModerationPattern)
	r.patternMatches = make(map[string][]*patternMatchRecord)
	r.filters = make(map[string]*storage.Filter)
	r.filtersByUser = make(map[string][]string)
	r.filterKeywords = make(map[string][]*storage.FilterKeyword)
	r.filterStatuses = make(map[string][]*storage.FilterStatus)
	r.reports = make(map[string]*storage.Report)
	r.reportsByUser = make(map[string][]string)
	r.reportsByTarget = make(map[string][]string)
	r.reportsByStatus = make(map[string][]string)
	r.reportStats = make(map[string]*storage.ReportStats)
	r.flags = make(map[string]*storage.Flag)
	r.flagsByObject = make(map[string][]string)
	r.flagsByActor = make(map[string][]string)
	r.pendingFlagIDs = []string{}
	r.auditLogs = []*storage.AuditLog{}
	r.auditLogsByAdmin = make(map[string][]int)
	r.auditLogsByTarget = make(map[string][]int)
	r.reviewQueue = []*models.ModerationReviewQueue{}
	r.decisionResults = make(map[string][]*models.ModerationDecisionResult)
	r.analysisResults = make(map[string]map[string]interface{})
}

// Ensure ModerationRepository implements interfaces.ModerationRepository
var _ interfaces.ModerationRepository = (*ModerationRepository)(nil)
