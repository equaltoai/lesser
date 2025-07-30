package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ModerationRepository implements moderation operations using DynamORM
type ModerationRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewModerationRepository creates a new moderation repository
func NewModerationRepository(db core.DB, tableName string, logger *zap.Logger) *ModerationRepository {
	return &ModerationRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// generateRandomString generates a random string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}

// CreateModerationEvent creates a new moderation event
func (r *ModerationRepository) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%s", generateRandomString(12))
	}
	event.Created = time.Now()
	event.Updated = event.Created

	// Set TTL if not specified (30 days default)
	if event.TTL == 0 {
		event.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	// Create model and update keys
	model := &models.ModerationEvent{
		ModerationEvent: *event,
		Type:            "EVENT",
		TTL:             event.TTL,
		CreatedAt:       event.Created,
	}
	model.UpdateKeys()

	// Create the event
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create moderation event",
			zap.Error(err),
			zap.String("event_id", event.ID),
			zap.String("object_id", event.ObjectID))
		return fmt.Errorf("failed to create moderation event: %w", err)
	}

	r.logger.Debug("Created moderation event",
		zap.String("event_id", event.ID),
		zap.String("object_id", event.ObjectID),
		zap.String("type", string(event.EventType)))

	return nil
}

// GetModerationEvent retrieves a moderation event by ID
func (r *ModerationRepository) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	var model models.ModerationEvent

	// Query using GSI3 for event ID lookup
	err := r.db.WithContext(ctx).Model(&model).
		Index("gsi3").
		Where("GSI3PK", "=", fmt.Sprintf("EVENTID#%s", eventID)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("moderation event not found")
		}
		return nil, fmt.Errorf("failed to get moderation event: %w", err)
	}

	return &model.ModerationEvent, nil
}

// GetModerationQueue retrieves pending moderation events
func (r *ModerationRepository) GetModerationQueue(ctx context.Context, filter *storage.ModerationFilter) ([]*storage.ModerationQueueItem, error) {
	limit := 50 // Default limit
	if filter != nil && filter.Limit > 0 {
		limit = filter.Limit
	}

	var models []models.ModerationEvent

	// Query using GSI2 for type/category
	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi2").
		Where("GSI2PK", "=", fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)).
		Limit(limit)

	if err := query.All(&models); err != nil {
		return nil, fmt.Errorf("failed to query moderation queue: %w", err)
	}

	items := make([]*storage.ModerationQueueItem, 0, len(models))
	for _, model := range models {
		event := &model.ModerationEvent

		// Apply filters
		if filter != nil {
			// Apply score filters
			if filter.MinScore > 0 && event.ConfidenceScore < filter.MinScore {
				continue
			}
			if filter.MaxScore > 0 && event.ConfidenceScore > filter.MaxScore {
				continue
			}

			// Apply content type filter
			if filter.ContentType != "" && event.ObjectType != filter.ContentType {
				continue
			}

			// Apply action filter
			if filter.Action != "" && string(event.EventType) != filter.Action {
				continue
			}

			// Apply time filters
			if !filter.StartTime.IsZero() && event.Created.Before(filter.StartTime) {
				continue
			}
			if !filter.EndTime.IsZero() && event.Created.After(filter.EndTime) {
				continue
			}
		}

		// Get review count for this event
		reviewCount, _ := r.countReviews(ctx, event.ID)

		queueItem := &storage.ModerationQueueItem{
			Event:       event,
			Priority:    float64(event.Severity) * event.ConfidenceScore,
			ReviewCount: reviewCount,
		}
		items = append(items, queueItem)
	}

	return items, nil
}

// GetModerationQueuePaginated retrieves pending moderation events with pagination
func (r *ModerationRepository) GetModerationQueuePaginated(ctx context.Context, limit int, cursor string) ([]*storage.ModerationQueueItem, string, error) {
	var models []models.ModerationEvent

	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi2").
		Where("GSI2PK", "=", fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)).
		Limit(limit)

	// TODO: Implement cursor-based pagination with DynamORM
	// For now, just return all results without cursor support

	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query moderation queue: %w", err)
	}

	items := make([]*storage.ModerationQueueItem, 0, len(models))
	for _, model := range models {
		event := &model.ModerationEvent

		// Get review count for this event
		reviewCount, _ := r.countReviews(ctx, event.ID)

		queueItem := &storage.ModerationQueueItem{
			Event:       event,
			Priority:    float64(event.Severity) * event.ConfidenceScore,
			ReviewCount: reviewCount,
		}
		items = append(items, queueItem)
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return items, nextCursor, nil
}

// GetModerationEventsByObject retrieves all moderation events for an object
func (r *ModerationRepository) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	var models []models.ModerationEvent

	query := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("EVENT#%s", objectID)).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query moderation events: %w", err)
	}

	events := make([]*storage.ModerationEvent, len(models))
	for i, model := range models {
		if model.Type == "EVENT" {
			events[i] = &model.ModerationEvent
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return events, nextCursor, nil
}

// GetModerationEventsByActor retrieves all moderation events created by an actor
func (r *ModerationRepository) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	var models []models.ModerationEvent

	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi1").
		Where("GSI1PK", "=", fmt.Sprintf("ACTOR#%s", actorID)).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query moderation events by actor: %w", err)
	}

	events := make([]*storage.ModerationEvent, len(models))
	for i, model := range models {
		if model.Type == "EVENT" {
			events[i] = &model.ModerationEvent
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return events, nextCursor, nil
}

// AddModerationReview adds a review to a moderation event
func (r *ModerationRepository) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	if review.ID == "" {
		review.ID = fmt.Sprintf("rev_%s", generateRandomString(12))
	}
	review.Created = time.Now()

	// Create model
	model := &models.ModerationReview{
		Review:    *review,
		Type:      "REVIEW",
		CreatedAt: review.Created,
		TTL:       time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	model.UpdateKeys()

	// Create the review
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to add moderation review",
			zap.Error(err),
			zap.String("review_id", review.ID),
			zap.String("event_id", review.EventID))
		return fmt.Errorf("failed to add review: %w", err)
	}

	r.logger.Debug("Added moderation review",
		zap.String("review_id", review.ID),
		zap.String("event_id", review.EventID),
		zap.String("reviewer", review.ReviewerID))

	return nil
}

// GetModerationReviews retrieves all reviews for a moderation event
func (r *ModerationRepository) GetModerationReviews(ctx context.Context, eventID string) ([]*storage.ModerationReview, error) {
	var models []models.ModerationReview

	err := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("REVIEW#%s", eventID)).
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query reviews: %w", err)
	}

	reviews := make([]*storage.ModerationReview, 0, len(models))
	for _, model := range models {
		if model.Type == "REVIEW" {
			reviews = append(reviews, &model.Review)
		}
	}

	return reviews, nil
}

// CreateModerationDecision creates a consensus decision
func (r *ModerationRepository) CreateModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	if decision.ID == "" {
		decision.ID = fmt.Sprintf("dec_%s", generateRandomString(12))
	}
	decision.Decided = time.Now()

	// Create model
	model := &models.ModerationDecision{
		ModerationDecision: *decision,
		Type:               "DECISION",
		CreatedAt:          decision.Decided,
		TTL:                time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days retention
	}
	model.UpdateKeys()

	// Create the decision
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create moderation decision",
			zap.Error(err),
			zap.String("decision_id", decision.ID),
			zap.String("object_id", decision.ObjectID))
		return fmt.Errorf("failed to create decision: %w", err)
	}

	r.logger.Info("Created moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", string(decision.Action)),
		zap.Float64("consensus", decision.ConsensusScore))

	return nil
}

// GetModerationDecision retrieves the current decision for an object
func (r *ModerationRepository) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	var model models.ModerationDecision

	// Query using GSI1 for active decisions
	err := r.db.WithContext(ctx).Model(&model).
		Index("gsi1").
		Where("GSI1PK", "=", "ACTIVE_DECISIONS").
		Where("GSI1SK", "=", fmt.Sprintf("OBJECT#%s", objectID)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // No decision yet
		}
		return nil, fmt.Errorf("failed to get moderation decision: %w", err)
	}

	return &model.ModerationDecision, nil
}

// StoreModerationDecision stores a moderation decision (alias for CreateModerationDecision)
func (r *ModerationRepository) StoreModerationDecision(ctx context.Context, decision *storage.ModerationDecision) error {
	return r.CreateModerationDecision(ctx, decision)
}

// UpdateModerationDecision updates a moderation decision based on a review
func (r *ModerationRepository) UpdateModerationDecision(ctx context.Context, contentID string, review *storage.ModerationReview) error {
	// Get the current decision for the content
	currentDecision, err := r.GetModerationDecision(ctx, contentID)
	if err != nil {
		return fmt.Errorf("failed to get current moderation decision: %w", err)
	}

	// If no decision exists, we cannot update it
	if currentDecision == nil {
		return fmt.Errorf("no moderation decision exists for content ID: %s", contentID)
	}

	// Create a new moderation decision based on the review
	newDecision := &storage.ModerationDecision{
		ID:               fmt.Sprintf("dec_%s", generateRandomString(12)),
		EventID:          currentDecision.EventID,
		ObjectID:         contentID,
		Action:           review.Action,
		ConsensusScore:   review.Confidence,
		ReviewerCount:    1,
		TrustWeightTotal: review.Weight,
		Reviews: []*moderation.Review{
			{
				ID:         fmt.Sprintf("rev_%s", generateRandomString(12)),
				EventID:    currentDecision.EventID,
				ReviewerID: review.ReviewerID,
				Action:     review.Action,
				Category:   review.Category,
				Severity:   review.Severity,
				Confidence: review.Confidence,
				Notes:      review.Notes,
				Weight:     review.Weight,
				Created:    time.Now(),
			},
		},
		Decided: time.Now(),
	}

	// Create the updated decision
	if err := r.CreateModerationDecision(ctx, newDecision); err != nil {
		return fmt.Errorf("failed to create updated moderation decision: %w", err)
	}

	r.logger.Info("Updated moderation decision",
		zap.String("content_id", contentID),
		zap.String("reviewer", review.ReviewerID),
		zap.String("action", string(review.Action)),
		zap.Float64("confidence", review.Confidence))

	return nil
}

// GetModerationPatterns retrieves moderation patterns based on criteria
func (r *ModerationRepository) GetModerationPatterns(ctx context.Context, active bool, severity string, limit int) ([]*storage.ModerationPattern, error) {
	var patternModels []models.ModerationPattern

	if active && severity != "" {
		// Query by active status and severity using GSI2
		err := r.db.WithContext(ctx).Model(&patternModels).
			Index("gsi2").
			Where("GSI2PK", "=", fmt.Sprintf("MODERATION_PATTERNS#%s", severity)).
			Limit(limit).
			All(&patternModels)
		if err != nil {
			return nil, fmt.Errorf("failed to query moderation patterns: %w", err)
		}
	} else if active {
		// Query by active status only using GSI1
		err := r.db.WithContext(ctx).Model(&patternModels).
			Index("gsi1").
			Where("GSI1PK", "=", "MODERATION_PATTERNS#ACTIVE").
			Limit(limit).
			All(&patternModels)
		if err != nil {
			return nil, fmt.Errorf("failed to query moderation patterns: %w", err)
		}
	} else {
		// Scan for all patterns (less efficient)
		// DynamORM doesn't support BeginsWith, so we query and filter
		err := r.db.WithContext(ctx).Model(&patternModels).
			Where("SK", "=", "PATTERN").
			Limit(limit * 2). // Get extra to account for filtering
			All(&patternModels)
		if err != nil {
			return nil, fmt.Errorf("failed to scan moderation patterns: %w", err)
		}
		
		// Filter to only moderation patterns
		var filtered []models.ModerationPattern
		for i := range patternModels {
			if strings.HasPrefix(patternModels[i].PK, "MODERATION_PATTERN#") {
				filtered = append(filtered, patternModels[i])
				if len(filtered) >= limit {
					break
				}
			}
		}
		patternModels = filtered

		// Filter by severity if specified
		if severity != "" {
			filtered := patternModels[:0]
			for _, p := range patternModels {
				if p.Severity == severity {
					filtered = append(filtered, p)
				}
			}
			patternModels = filtered
		}
	}

	// Convert to storage patterns
	result := make([]*storage.ModerationPattern, len(patternModels))
	for i, model := range patternModels {
		result[i] = &storage.ModerationPattern{
			ID:          model.ID,
			Name:        model.Name,
			Description: model.Description,
			Type:        model.Type,
			Content:     model.Pattern,
			Severity:    model.Severity,
			Active:      model.Active,
			CreatedAt:   model.CreatedAt,
			UpdatedAt:   model.UpdatedAt,
		}
		if !model.LastMatch.IsZero() {
			result[i].LastMatch = model.LastMatch
		}
	}

	return result, nil
}

// UpdateModerationPattern updates an existing moderation pattern
func (r *ModerationRepository) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	pattern.UpdatedAt = time.Now()

	// Create model
	model := &models.ModerationPattern{
		ID:          pattern.ID,
		Name:        pattern.Name,
		Description: pattern.Description,
		Type:        pattern.Type,
		Pattern:     pattern.Content,
		Severity:    pattern.Severity,
		Active:      pattern.Active,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
		TTL:         time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
	if !pattern.LastMatch.IsZero() {
		model.LastMatch = pattern.LastMatch
	}
	model.UpdateKeys()

	// Update the pattern
	if err := r.db.WithContext(ctx).Model(model).Update(); err != nil {
		return fmt.Errorf("failed to update moderation pattern: %w", err)
	}

	return nil
}

// DeleteModerationPattern deletes a moderation pattern
func (r *ModerationRepository) DeleteModerationPattern(ctx context.Context, patternID string) error {
	err := r.db.WithContext(ctx).Model(&models.ModerationPattern{}).
		Where("PK", "=", fmt.Sprintf("MODERATION_PATTERN#%s", patternID)).
		Where("SK", "=", "PATTERN").
		Delete()

	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return fmt.Errorf("failed to delete moderation pattern: %w", err)
	}

	return nil
}

// countReviews is a helper to count reviews for an event
func (r *ModerationRepository) countReviews(ctx context.Context, eventID string) (int, error) {
	count, err := r.db.WithContext(ctx).Model(&models.ModerationReview{}).
		Where("PK", "=", fmt.Sprintf("REVIEW#%s", eventID)).
		Count()

	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// Helper methods from legacy implementation that are still referenced

// CreateModerationPattern creates a new moderation pattern
func (r *ModerationRepository) CreateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	if pattern.ID == "" {
		pattern.ID = fmt.Sprintf("pat_%s", generateRandomString(12))
	}

	// Set timestamps
	now := time.Now()
	pattern.CreatedAt = now
	pattern.UpdatedAt = now

	// Create model
	model := &models.ModerationPattern{
		ID:          pattern.ID,
		Name:        pattern.Name,
		Description: pattern.Description,
		Type:        pattern.Type,
		Pattern:     pattern.Content,
		Severity:    pattern.Severity,
		Active:      pattern.Active,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
		TTL:         time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
	model.UpdateKeys()

	// Create the pattern
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		return fmt.Errorf("failed to create moderation pattern: %w", err)
	}

	return nil
}

// GetModerationPattern retrieves a specific moderation pattern
func (r *ModerationRepository) GetModerationPattern(ctx context.Context, patternID string) (*storage.ModerationPattern, error) {
	var model models.ModerationPattern

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("MODERATION_PATTERN#%s", patternID)).
		Where("SK", "=", "PATTERN").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get moderation pattern: %w", err)
	}

	// Convert to storage pattern
	pattern := &storage.ModerationPattern{
		ID:          model.ID,
		Name:        model.Name,
		Description: model.Description,
		Type:        model.Type,
		Content:     model.Pattern,
		Severity:    model.Severity,
		Active:      model.Active,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
	if !model.LastMatch.IsZero() {
		pattern.LastMatch = model.LastMatch
	}

	return pattern, nil
}

// GetModerationHistory retrieves the complete moderation history for an object
func (r *ModerationRepository) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	history := &storage.ModerationHistory{
		ObjectID:  objectID,
		Events:    []storage.ModerationEvent{},
		Decisions: []storage.ModerationDecision{},
		Timeline:  []moderation.TimelineEntry{},
	}

	// Get all events for the object
	events, _, err := r.GetModerationEventsByObject(ctx, objectID, 100, "")
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e != nil {
			history.Events = append(history.Events, *e)
		}
	}

	// Get all decisions for the object
	var decisionModels []models.ModerationDecision
	err = r.db.WithContext(ctx).Model(&decisionModels).
		Where("PK", "=", fmt.Sprintf("DECISION#%s", objectID)).
		All(&decisionModels)
	
	if err == nil {
		for _, model := range decisionModels {
			if model.Type == "DECISION" {
				history.Decisions = append(history.Decisions, model.ModerationDecision)
			}
		}
	}

	// Build timeline
	for _, event := range history.Events {
		history.Timeline = append(history.Timeline, moderation.TimelineEntry{
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

	for _, decision := range history.Decisions {
		history.Timeline = append(history.Timeline, moderation.TimelineEntry{
			Timestamp:   decision.Decided,
			Type:        "decision",
			ActorID:     "system",
			Description: fmt.Sprintf("Decision: %s (consensus: %.2f)", decision.Action, decision.ConsensusScore),
			Metadata: map[string]any{
				"decision_id": decision.ID,
				"action":      decision.Action,
			},
		})
	}

	// Determine current status
	if len(history.Decisions) > 0 {
		lastDecision := history.Decisions[len(history.Decisions)-1]
		history.CurrentStatus = string(lastDecision.Action)
	} else {
		history.CurrentStatus = "pending"
	}

	return history, nil
}

// GetModerationEvents retrieves all moderation events with optional filters
func (r *ModerationRepository) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	// If no filter or all filter fields are empty, scan all events
	if filter == nil || (filter.EventType == nil && filter.Category == nil && filter.ActorID == "" && filter.ObjectID == "") {
		return r.scanAllModerationEvents(ctx, filter, limit, cursor)
	}

	// Use query based on the most selective filter
	if filter.ObjectID != "" {
		return r.GetModerationEventsByObject(ctx, filter.ObjectID, limit, cursor)
	}

	if filter.ActorID != "" {
		return r.GetModerationEventsByActor(ctx, filter.ActorID, limit, cursor)
	}

	// Query by event type and category using GSI2
	if filter.EventType != nil || filter.Category != nil {
		eventType := moderation.EventTypeFlagged
		if filter.EventType != nil {
			eventType = *filter.EventType
		}

		category := ""
		if filter.Category != nil {
			category = string(*filter.Category)
		}

		gsi2pk := fmt.Sprintf("TYPE#%s", eventType)
		if category != "" {
			gsi2pk = fmt.Sprintf("TYPE#%s#%s", eventType, category)
		}

		var models []models.ModerationEvent
		query := r.db.WithContext(ctx).Model(&models).
			Index("gsi2").
			Where("GSI2PK", "=", gsi2pk).
			Limit(limit)

		// TODO: Implement cursor-based pagination
		if err := query.All(&models); err != nil {
			return nil, "", fmt.Errorf("failed to query moderation events: %w", err)
		}

		events := make([]*storage.ModerationEvent, 0, len(models))
		for _, model := range models {
			if model.Type == "EVENT" && r.matchesEventFilter(&model.ModerationEvent, filter) {
				events = append(events, &model.ModerationEvent)
			}
		}

		// TODO: Implement proper cursor generation
		nextCursor := ""
		
		return events, nextCursor, nil
	}

	// Fallback to scan
	return r.scanAllModerationEvents(ctx, filter, limit, cursor)
}

// scanAllModerationEvents performs a scan operation to get all events
func (r *ModerationRepository) scanAllModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	var models []models.ModerationEvent
	
	// DynamORM doesn't have a direct scan with filter, so we'll query and filter in memory
	// This is less efficient but matches the legacy behavior
	query := r.db.WithContext(ctx).Model(&models).
		Limit(limit * 2) // Get extra to account for filtering
	
	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to scan moderation events: %w", err)
	}

	events := make([]*storage.ModerationEvent, 0, limit)
	for _, model := range models {
		if model.Type == "EVENT" && r.matchesEventFilter(&model.ModerationEvent, filter) {
			events = append(events, &model.ModerationEvent)
			if len(events) >= limit {
				break
			}
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""
	
	return events, nextCursor, nil
}

// matchesEventFilter checks if an event matches the given filter
func (r *ModerationRepository) matchesEventFilter(event *storage.ModerationEvent, filter *storage.ModerationEventFilter) bool {
	if filter == nil {
		return true
	}

	if filter.EventType != nil && event.EventType != *filter.EventType {
		return false
	}

	if filter.Category != nil && event.Category != *filter.Category {
		return false
	}

	if filter.MinSeverity != nil && event.Severity < *filter.MinSeverity {
		return false
	}

	if filter.StartTime != nil && event.Created.Before(*filter.StartTime) {
		return false
	}

	if filter.EndTime != nil && event.Created.After(*filter.EndTime) {
		return false
	}

	return true
}

// CreateAdminReview creates an admin review that overrides consensus
func (r *ModerationRepository) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	// Create a special review with maximum weight
	review := &storage.ModerationReview{
		ID:         fmt.Sprintf("admin_rev_%s", generateRandomString(12)),
		EventID:    eventID,
		ReviewerID: adminID,
		Action:     action,
		Category:   moderation.CategoryOther,    // Admin override doesn't need specific category
		Severity:   moderation.SeverityCritical, // Max severity for admin actions
		Confidence: 1.0,                         // Full confidence
		Notes:      fmt.Sprintf("Admin override: %s", reason),
		Weight:     1000.0, // Very high weight to override consensus
		Created:    time.Now(),
	}

	// Add the review
	if err := r.AddModerationReview(ctx, review); err != nil {
		return fmt.Errorf("failed to add admin review: %w", err)
	}

	// Get the event to get the object ID
	event, err := r.GetModerationEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get moderation event: %w", err)
	}

	// Immediately create a decision based on the admin action
	decision := &storage.ModerationDecision{
		ID:               fmt.Sprintf("admin_dec_%s", generateRandomString(12)),
		EventID:          eventID,
		ObjectID:         event.ObjectID,
		Action:           action,
		ConsensusScore:   1.0, // Admin override has full consensus
		ReviewerCount:    1,
		TrustWeightTotal: 1000.0,
		Reviews:          []*moderation.Review{(*moderation.Review)(review)},
		Decided:          time.Now(),
	}

	// Create the decision
	if err := r.CreateModerationDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to create admin decision: %w", err)
	}

	r.logger.Info("Admin override created",
		zap.String("admin", adminID),
		zap.String("event_id", eventID),
		zap.String("action", string(action)),
		zap.String("reason", reason))

	return nil
}

// GetReviewerStats retrieves statistics for a reviewer
func (r *ModerationRepository) GetReviewerStats(ctx context.Context, reviewerID string) (*storage.ReviewerStats, error) {
	stats := &storage.ReviewerStats{
		ReviewerID:        reviewerID,
		TotalReviews:      0,
		AccurateReviews:   0,
		AccuracyRate:      0.0,
		ReviewsByCategory: make(map[string]int),
		JoinedAt:          time.Now(), // Default, will be updated if we find user
	}

	// Get user to find when they joined
	var user models.User
	err := r.db.WithContext(ctx).Model(&user).
		Where("PK", "=", fmt.Sprintf("USER#%s", reviewerID)).
		Where("SK", "=", "PROFILE").
		First(&user)
	if err == nil {
		stats.JoinedAt = user.CreatedAt
	}

	// Get trust score for moderation category
	var trustScore models.TrustScore
	err = r.db.WithContext(ctx).Model(&trustScore).
		Where("PK", "=", fmt.Sprintf("TRUST_SCORE#%s", reviewerID)).
		Where("SK", "=", "CATEGORY#moderation").
		First(&trustScore)
	if err == nil {
		stats.TrustScore = trustScore.Score
	}

	// Count all reviews by this reviewer
	// Since we don't have a direct index for reviewer lookups, we need to scan
	var reviews []models.ModerationReview
	err = r.db.WithContext(ctx).Model(&reviews).
		Limit(1000). // Reasonable limit
		All(&reviews)
	
	if err != nil {
		return nil, fmt.Errorf("failed to scan reviews: %w", err)
	}

	var lastReviewTime time.Time
	for _, review := range reviews {
		if review.Type == "REVIEW" && review.ReviewerID == reviewerID {
			stats.TotalReviews++
			
			// Track by category
			category := string(review.Category)
			stats.ReviewsByCategory[category]++
			
			// Update last review time
			if review.Created.After(lastReviewTime) {
				lastReviewTime = review.Created
			}
			
			// Simplified accuracy check
			if review.Weight > 0.5 {
				stats.AccurateReviews++
			}
		}
	}

	stats.LastReviewAt = lastReviewTime

	// Calculate accuracy rate
	if stats.TotalReviews > 0 {
		stats.AccuracyRate = float64(stats.AccurateReviews) / float64(stats.TotalReviews)
	}

	return stats, nil
}

// GetModerationQueueCount returns the count of items in the moderation queue
func (r *ModerationRepository) GetModerationQueueCount(ctx context.Context) (int, error) {
	// Count pending moderation events
	count, err := r.db.WithContext(ctx).Model(&models.ModerationEvent{}).
		Index("gsi2").
		Where("GSI2PK", "=", fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)).
		Count()
	
	if err != nil {
		// If error, return 0 instead of failing (matches legacy behavior)
		return 0, nil
	}

	return int(count), nil
}

// RecordPatternMatch records a moderation pattern match for analytics
func (r *ModerationRepository) RecordPatternMatch(ctx context.Context, patternID string, matched bool, timestamp time.Time) error {
	// Create analytics record
	analytics := &models.ModerationAnalytics{
		PatternID: patternID,
		Matched:   matched,
		Timestamp: timestamp,
		CreatedAt: time.Now(),
	}
	analytics.UpdateKeys()
	
	err := r.db.Model(analytics).Create()
	if err != nil {
		r.logger.Error("failed to record pattern match",
			zap.String("pattern_id", patternID),
			zap.Bool("matched", matched),
			zap.Error(err))
		return fmt.Errorf("failed to record pattern match: %w", err)
	}
	
	return nil
}

