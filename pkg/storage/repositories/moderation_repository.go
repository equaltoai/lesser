package repositories

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
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
		ID:              event.ID,
		EventType:       event.EventType,
		ObjectID:        event.ObjectID,
		ObjectType:      event.ObjectType,
		ActorID:         event.ActorID,
		Category:        event.Category,
		Severity:        event.Severity,
		ConfidenceScore: event.ConfidenceScore,
		Evidence:        event.Evidence,
		Reason:          event.Reason,
		Created:         event.Created,
		Updated:         event.Updated,
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

	// Convert model to storage.ModerationEvent
	result := &storage.ModerationEvent{
		ID:              model.ID,
		EventType:       model.EventType,
		ObjectID:        model.ObjectID,
		ObjectType:      model.ObjectType,
		ActorID:         model.ActorID,
		Category:        model.Category,
		Severity:        model.Severity,
		ConfidenceScore: model.ConfidenceScore,
		Evidence:        model.Evidence,
		Reason:          model.Reason,
		Created:         model.Created,
		Updated:         model.Updated,
		TTL:             model.TTL,
	}
	return result, nil
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
		// Convert model to storage.ModerationEvent
		event := &storage.ModerationEvent{
			ID:              model.ID,
			EventType:       model.EventType,
			ObjectID:        model.ObjectID,
			ObjectType:      model.ObjectType,
			ActorID:         model.ActorID,
			Category:        model.Category,
			Severity:        model.Severity,
			ConfidenceScore: model.ConfidenceScore,
			Evidence:        model.Evidence,
			Reason:          model.Reason,
			Created:         model.Created,
			Updated:         model.Updated,
			TTL:             model.TTL,
		}

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
			if filter.StartTime != nil && event.Created.Before(*filter.StartTime) {
				continue
			}
			if filter.EndTime != nil && event.Created.After(*filter.EndTime) {
				continue
			}
		}

		// Get review count for this event
		reviewCount, _ := r.countReviews(ctx, event.ID)

		queueItem := &storage.ModerationQueueItem{
			Event:       event,
			Priority:    int(r.getSeverityValue(event.Severity) * event.ConfidenceScore),
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
		// Convert model to storage.ModerationEvent
		event := &storage.ModerationEvent{
			ID:              model.ID,
			EventType:       model.EventType,
			ObjectID:        model.ObjectID,
			ObjectType:      model.ObjectType,
			ActorID:         model.ActorID,
			Category:        model.Category,
			Severity:        model.Severity,
			ConfidenceScore: model.ConfidenceScore,
			Evidence:        model.Evidence,
			Reason:          model.Reason,
			Created:         model.Created,
			Updated:         model.Updated,
			TTL:             model.TTL,
		}

		// Get review count for this event
		reviewCount, _ := r.countReviews(ctx, event.ID)

		queueItem := &storage.ModerationQueueItem{
			Event:       event,
			Priority:    int(r.getSeverityValue(event.Severity) * event.ConfidenceScore),
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

	events := make([]*storage.ModerationEvent, 0, len(models))
	for _, model := range models {
		if model.Type == "EVENT" {
			event := &storage.ModerationEvent{
				ID:              model.ID,
				EventType:       model.EventType,
				ObjectID:        model.ObjectID,
				ObjectType:      model.ObjectType,
				ActorID:         model.ActorID,
				Category:        model.Category,
				Severity:        model.Severity,
				ConfidenceScore: model.ConfidenceScore,
				Evidence:        model.Evidence,
				Reason:          model.Reason,
				Created:         model.Created,
				Updated:         model.Updated,
				TTL:             model.TTL,
			}
			events = append(events, event)
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

	events := make([]*storage.ModerationEvent, 0, len(models))
	for _, model := range models {
		if model.Type == "EVENT" {
			event := &storage.ModerationEvent{
				ID:              model.ID,
				EventType:       model.EventType,
				ObjectID:        model.ObjectID,
				ObjectType:      model.ObjectType,
				ActorID:         model.ActorID,
				Category:        model.Category,
				Severity:        model.Severity,
				ConfidenceScore: model.ConfidenceScore,
				Evidence:        model.Evidence,
				Reason:          model.Reason,
				Created:         model.Created,
				Updated:         model.Updated,
				TTL:             model.TTL,
			}
			events = append(events, event)
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
		ID:          review.ID,
		EventID:     review.EventID,
		ReviewerID:  review.ReviewerID,
		ReviewerRep: review.ReviewerRep,
		Action:      review.Action,
		Severity:    review.Severity,
		Note:        review.Note,
		Tags:        review.Tags,
		Metadata:    review.Metadata,
		Confidence:  review.Confidence,
		Created:     review.Created,
		Type:        "REVIEW",
		CreatedAt:   review.Created,
		TTL:         time.Now().Add(30 * 24 * time.Hour).Unix(),
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
			review := &storage.ModerationReview{
				ID:          model.ID,
				EventID:     model.EventID,
				ReviewerID:  model.ReviewerID,
				ReviewerRep: model.ReviewerRep,
				Action:      model.Action,
				Severity:    model.Severity,
				Note:        model.Note,
				Tags:        model.Tags,
				Metadata:    model.Metadata,
				Confidence:  model.Confidence,
				Created:     model.Created,
			}
			reviews = append(reviews, review)
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
		ID:               decision.ID,
		EventID:          decision.EventID,
		ObjectID:         decision.ObjectID,
		Action:           decision.Action,
		ConsensusScore:   decision.ConsensusScore,
		ReviewerCount:    decision.ReviewerCount,
		TrustWeightTotal: decision.TrustWeightTotal,
		Reviews:          func() []interface{} { 
			var result []interface{}
			for _, r := range decision.Reviews {
				result = append(result, r)
			}
			return result
		}(),
		Metadata:         decision.Metadata,
		Decided:          decision.Decided,
		Expires:          decision.Expires,
		Type:             "DECISION",
		CreatedAt:        time.Now(),
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days retention
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

	return &storage.ModerationDecision{
		ID:               model.ID,
		EventID:          model.EventID,
		ObjectID:         model.ObjectID,
		Action:           model.Action,
		ConsensusScore:   model.ConsensusScore,
		ReviewerCount:    model.ReviewerCount,
		TrustWeightTotal: model.TrustWeightTotal,
		Reviews:          model.Reviews,
		Metadata:         model.Metadata,
		Decided:          model.Decided,
		Expires:          model.Expires,
	}, nil
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
		TrustWeightTotal: review.Confidence, // Using Confidence as Weight substitute
		Reviews: []interface{}{
			fmt.Sprintf("rev_%s", generateRandomString(12)),
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
			result[i].LastMatch = &model.LastMatch
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
	if pattern.LastMatch != nil && !pattern.LastMatch.IsZero() {
		model.LastMatch = *pattern.LastMatch
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

// getSeverityValue converts severity string to numeric value
func (r *ModerationRepository) getSeverityValue(severity string) float64 {
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
		pattern.LastMatch = &model.LastMatch
	}

	return pattern, nil
}

// GetModerationHistory retrieves the complete moderation history for an object
func (r *ModerationRepository) GetModerationHistory(ctx context.Context, objectID string) (*storage.ModerationHistory, error) {
	history := &storage.ModerationHistory{
		ObjectID:  objectID,
		Events:    []storage.ModerationEvent{},
		Decisions: []storage.ModerationDecision{},
		Timeline:  []storage.ModerationTimelineEntry{},
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
				decision := storage.ModerationDecision{
					ID:               model.ID,
					EventID:          model.EventID,
					ObjectID:         model.ObjectID,
					Action:           model.Action,
					ConsensusScore:   model.ConsensusScore,
					ReviewerCount:    model.ReviewerCount,
					TrustWeightTotal: model.TrustWeightTotal,
					Reviews:          model.Reviews,
					Metadata:         model.Metadata,
					Decided:          model.Decided,
					Expires:          model.Expires,
				}
				history.Decisions = append(history.Decisions, decision)
			}
		}
	}

	// Build timeline
	for _, event := range history.Events {
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

	for _, decision := range history.Decisions {
		history.Timeline = append(history.Timeline, storage.ModerationTimelineEntry{
			Timestamp:   time.Now(),
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
	if filter == nil || (filter.EventType == "" && filter.Category == "" && filter.ActorID == "" && filter.ObjectID == "") {
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
	if filter.EventType != "" || filter.Category != "" {
		eventType := storage.EventTypeFlagged
		if filter.EventType != "" {
			eventType = filter.EventType
		}

		category := ""
		if filter.Category != "" {
			category = filter.Category
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
			if model.Type == "EVENT" {
				event := &storage.ModerationEvent{
					ID:              model.ID,
					EventType:       model.EventType,
					ObjectID:        model.ObjectID,
					ObjectType:      model.ObjectType,
					ActorID:         model.ActorID,
					Category:        model.Category,
					Severity:        model.Severity,
					ConfidenceScore: model.ConfidenceScore,
					Evidence:        model.Evidence,
					Reason:          model.Reason,
					Created:         model.Created,
					Updated:         model.Updated,
					TTL:             model.TTL,
				}
				if r.matchesEventFilter(event, filter) {
					events = append(events, event)
				}
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
		if model.Type == "EVENT" {
			event := &storage.ModerationEvent{
				ID:              model.ID,
				EventType:       model.EventType,
				ObjectID:        model.ObjectID,
				ObjectType:      model.ObjectType,
				ActorID:         model.ActorID,
				Category:        model.Category,
				Severity:        model.Severity,
				ConfidenceScore: model.ConfidenceScore,
				Evidence:        model.Evidence,
				Reason:          model.Reason,
				Created:         model.Created,
				Updated:         model.Updated,
				TTL:             model.TTL,
			}
			if r.matchesEventFilter(event, filter) {
				events = append(events, event)
				if len(events) >= limit {
					break
				}
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

	if filter.EventType != "" && event.EventType != filter.EventType {
		return false
	}

	if filter.Category != "" && event.Category != filter.Category {
		return false
	}

	if filter.MinSeverity != nil && r.getSeverityValue(event.Severity) < float64(*filter.MinSeverity) {
		return false
	}

	if filter.StartDate != nil && event.Created.Before(*filter.StartDate) {
		return false
	}

	if filter.EndDate != nil && event.Created.After(*filter.EndDate) {
		return false
	}

	return true
}

// CreateAdminReview creates an admin review that overrides consensus
func (r *ModerationRepository) CreateAdminReview(ctx context.Context, eventID string, adminID string, action storage.ActionType, reason string) error {
	// Create a special review with maximum weight
	review := &storage.ModerationReview{
		ID:          fmt.Sprintf("admin_rev_%s", generateRandomString(12)),
		EventID:     eventID,
		ReviewerID:  adminID,
		Action:      string(action), // Convert ActionType to string
		Severity:    "critical",     // Max severity for admin actions
		Confidence:  1.0,            // Full confidence
		Note:        fmt.Sprintf("Admin override: %s", reason),
		Created:     time.Now(),
		ReviewerRep: 1000.0, // Very high reputation to override consensus
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
		Action:           string(action), // Convert ActionType to string
		ConsensusScore:   1.0, // Admin override has full consensus
		ReviewerCount:    1,
		TrustWeightTotal: 1000.0,
		Reviews:          []interface{}{
			map[string]interface{}{
				"id":          review.ID,
				"event_id":    review.EventID,
				"reviewer_id": review.ReviewerID,
				"action":      review.Action,
				"severity":    review.Severity,
				"confidence":  review.Confidence,
				"notes":       review.Note,
				"weight":      review.ReviewerRep,
				"created":     review.Created,
			},
		},
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
			
			// Track by category - use severity as category since Category field doesn't exist
			category := review.Severity
			stats.ReviewsByCategory[category]++
			
			// Update last review time
			if review.Created.After(lastReviewTime) {
				lastReviewTime = review.Created
			}
			
			// Simplified accuracy check - use ReviewerRep instead of Weight
			if review.ReviewerRep > 0.5 {
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

// FILTER METHODS

// CreateFilter creates a new filter
func (r *ModerationRepository) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	// Generate ID if not provided
	if filter.ID == "" {
		filter.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	filter.CreatedAt = now
	filter.UpdatedAt = now

	// Create model and update keys
	model := &models.Filter{
		ID:           filter.ID,
		Username:     filter.Username,
		Title:        filter.Title,
		Context:      filter.Context,
		FilterAction: filter.FilterAction,
		ExpiresAt:    filter.ExpiresAt,
		CreatedAt:    filter.CreatedAt,
		UpdatedAt:    filter.UpdatedAt,
	}
	model.UpdateKeys()

	// Create the filter
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create filter",
			zap.Error(err),
			zap.String("filter_id", filter.ID),
			zap.String("username", filter.Username))
		return fmt.Errorf("failed to create filter: %w", err)
	}

	r.logger.Debug("Created filter",
		zap.String("filter_id", filter.ID),
		zap.String("username", filter.Username),
		zap.String("title", filter.Title))

	return nil
}

// GetFilter retrieves a filter by ID
func (r *ModerationRepository) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	// We need to scan for the filter since we don't know the username
	var models []models.Filter
	
	err := r.db.WithContext(ctx).Model(&models).
		Where("SK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Limit(10). // Reasonable limit
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query filter: %w", err)
	}

	// Find the matching filter
	for _, model := range models {
		if model.ID == filterID {
			return &storage.Filter{
				ID:           model.ID,
				Username:     model.Username,
				Title:        model.Title,
				Context:      model.Context,
				FilterAction: model.FilterAction,
				ExpiresAt:    model.ExpiresAt,
				CreatedAt:    model.CreatedAt,
				UpdatedAt:    model.UpdatedAt,
			}, nil
		}
	}

	return nil, fmt.Errorf("filter not found")
}

// GetFiltersForUser retrieves all filters for a user
func (r *ModerationRepository) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	var models []models.Filter

	err := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", ">=", "FILTER#").
		Where("SK", "<", "FILTER~"). // Use ~ as upper bound since it's after # in ASCII
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query filters for user: %w", err)
	}

	filters := make([]*storage.Filter, len(models))
	for i, model := range models {
		filters[i] = &storage.Filter{
			ID:           model.ID,
			Username:     model.Username,
			Title:        model.Title,
			Context:      model.Context,
			FilterAction: model.FilterAction,
			ExpiresAt:    model.ExpiresAt,
			CreatedAt:    model.CreatedAt,
			UpdatedAt:    model.UpdatedAt,
		}
	}

	return filters, nil
}

// UpdateFilter updates a filter
func (r *ModerationRepository) UpdateFilter(ctx context.Context, filterID string, updates map[string]any) error {
	// First get the existing filter to find the username
	filter, err := r.GetFilter(ctx, filterID)
	if err != nil {
		return fmt.Errorf("failed to find filter for update: %w", err)
	}

	// Apply updates
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

	// Set updated timestamp
	filter.UpdatedAt = time.Now()

	// Convert to model and update
	model := &models.Filter{
		ID:           filter.ID,
		Username:     filter.Username,
		Title:        filter.Title,
		Context:      filter.Context,
		FilterAction: filter.FilterAction,
		ExpiresAt:    filter.ExpiresAt,
		CreatedAt:    filter.CreatedAt,
		UpdatedAt:    filter.UpdatedAt,
	}
	model.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(model).Update(); err != nil {
		r.logger.Error("Failed to update filter",
			zap.Error(err),
			zap.String("filter_id", filterID))
		return fmt.Errorf("failed to update filter: %w", err)
	}

	r.logger.Debug("Updated filter",
		zap.String("filter_id", filterID),
		zap.String("username", filter.Username))

	return nil
}

// DeleteFilter deletes a filter and all its associated keywords and statuses
func (r *ModerationRepository) DeleteFilter(ctx context.Context, filterID string) error {
	// First get the filter to find the username
	filter, err := r.GetFilter(ctx, filterID)
	if err != nil {
		return fmt.Errorf("failed to find filter for deletion: %w", err)
	}

	// Delete all keywords first
	keywords, err := r.GetFilterKeywords(ctx, filterID)
	if err != nil {
		return fmt.Errorf("failed to get filter keywords for deletion: %w", err)
	}
	for _, keyword := range keywords {
		if err := r.DeleteFilterKeyword(ctx, keyword.ID); err != nil {
			r.logger.Error("Failed to delete filter keyword during filter deletion",
				zap.Error(err),
				zap.String("keyword_id", keyword.ID))
			// Continue with other deletions
		}
	}

	// Delete all statuses
	statuses, err := r.GetFilterStatuses(ctx, filterID)
	if err != nil {
		return fmt.Errorf("failed to get filter statuses for deletion: %w", err)
	}
	for _, status := range statuses {
		if err := r.DeleteFilterStatus(ctx, status.StatusID); err != nil {
			r.logger.Error("Failed to delete filter status during filter deletion",
				zap.Error(err),
				zap.String("status_id", status.StatusID))
			// Continue with other deletions
		}
	}

	// Delete the filter itself
	err = r.db.WithContext(ctx).Model(&models.Filter{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", filter.Username)).
		Where("SK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Delete()

	if err != nil {
		r.logger.Error("Failed to delete filter",
			zap.Error(err),
			zap.String("filter_id", filterID),
			zap.String("username", filter.Username))
		return fmt.Errorf("failed to delete filter: %w", err)
	}

	r.logger.Debug("Deleted filter",
		zap.String("filter_id", filterID),
		zap.String("username", filter.Username))

	return nil
}

// AddFilterKeyword adds a new keyword to a filter
func (r *ModerationRepository) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	// Generate UUID if not provided
	if keyword.ID == "" {
		keyword.ID = uuid.New().String()
	}

	// Set FilterID and CreatedAt
	keyword.FilterID = filterID
	keyword.CreatedAt = time.Now()

	// Create model and update keys
	model := &models.FilterKeyword{
		ID:        keyword.ID,
		FilterID:  keyword.FilterID,
		Keyword:   keyword.Keyword,
		WholeWord: keyword.WholeWord,
		CreatedAt: keyword.CreatedAt,
	}
	model.UpdateKeys()

	// Create the keyword
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to add filter keyword",
			zap.Error(err),
			zap.String("filter_id", filterID),
			zap.String("keyword_id", keyword.ID),
			zap.String("keyword", keyword.Keyword))
		return fmt.Errorf("failed to add filter keyword: %w", err)
	}

	r.logger.Debug("Added filter keyword",
		zap.String("filter_id", filterID),
		zap.String("keyword_id", keyword.ID),
		zap.String("keyword", keyword.Keyword))

	return nil
}

// GetFilterKeywords retrieves all keywords for a filter
func (r *ModerationRepository) GetFilterKeywords(ctx context.Context, filterID string) ([]*storage.FilterKeyword, error) {
	var models []models.FilterKeyword

	// Use range query to get all items with SK starting with "KEYWORD#"
	err := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", ">=", "KEYWORD#").
		Where("SK", "<", "KEYWORD~"). // Use ~ as upper bound since it's after # in ASCII
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query filter keywords: %w", err)
	}

	keywords := make([]*storage.FilterKeyword, len(models))
	for i, model := range models {
		keywords[i] = &storage.FilterKeyword{
			ID:        model.ID,
			FilterID:  model.FilterID,
			Keyword:   model.Keyword,
			WholeWord: model.WholeWord,
			CreatedAt: model.CreatedAt,
		}
	}

	return keywords, nil
}

// UpdateFilterKeyword updates a filter keyword
func (r *ModerationRepository) UpdateFilterKeyword(ctx context.Context, keywordID string, updates map[string]any) error {
	// First get the existing keyword to find its FilterID
	var existingModels []models.FilterKeyword
	
	// We need to scan for the keyword since we don't have the FilterID
	err := r.db.WithContext(ctx).Model(&existingModels).
		Where("SK", "=", fmt.Sprintf("KEYWORD#%s", keywordID)).
		All(&existingModels)

	if err != nil || len(existingModels) == 0 {
		if errors.IsNotFound(err) || len(existingModels) == 0 {
			return fmt.Errorf("filter keyword not found")
		}
		return fmt.Errorf("failed to find filter keyword: %w", err)
	}

	existing := existingModels[0]

	// Apply updates
	if keyword, ok := updates["keyword"].(string); ok {
		existing.Keyword = keyword
	}
	if wholeWord, ok := updates["whole_word"].(bool); ok {
		existing.WholeWord = wholeWord
	}

	// Update the keyword
	if err := r.db.WithContext(ctx).Model(&existing).Update(); err != nil {
		return fmt.Errorf("failed to update filter keyword: %w", err)
	}

	return nil
}

// DeleteFilterKeyword deletes a filter keyword
func (r *ModerationRepository) DeleteFilterKeyword(ctx context.Context, keywordID string) error {
	// First find the keyword to get its FilterID
	var existingModels []models.FilterKeyword
	
	err := r.db.WithContext(ctx).Model(&existingModels).
		Where("SK", "=", fmt.Sprintf("KEYWORD#%s", keywordID)).
		All(&existingModels)

	if err != nil || len(existingModels) == 0 {
		if errors.IsNotFound(err) || len(existingModels) == 0 {
			return fmt.Errorf("filter keyword not found")
		}
		return fmt.Errorf("failed to find filter keyword: %w", err)
	}

	existing := existingModels[0]

	// Delete the keyword
	err = r.db.WithContext(ctx).Model(&models.FilterKeyword{}).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", existing.FilterID)).
		Where("SK", "=", fmt.Sprintf("KEYWORD#%s", keywordID)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete filter keyword: %w", err)
	}

	return nil
}

// AddFilterStatus adds a new status to a filter
func (r *ModerationRepository) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	// Generate UUID if not provided
	if status.ID == "" {
		status.ID = uuid.New().String()
	}

	// Set FilterID and CreatedAt
	status.FilterID = filterID
	status.CreatedAt = time.Now()

	// Create model and update keys
	model := &models.FilterStatus{
		ID:        status.ID,
		FilterID:  status.FilterID,
		StatusID:  status.StatusID,
		CreatedAt: status.CreatedAt,
	}
	model.UpdateKeys()

	// Create the status
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to add filter status",
			zap.Error(err),
			zap.String("filter_id", filterID),
			zap.String("status_filter_id", status.ID),
			zap.String("status_id", status.StatusID))
		return fmt.Errorf("failed to add filter status: %w", err)
	}

	r.logger.Debug("Added filter status",
		zap.String("filter_id", filterID),
		zap.String("status_filter_id", status.ID),
		zap.String("status_id", status.StatusID))

	return nil
}

// GetFilterStatuses retrieves all statuses for a filter
func (r *ModerationRepository) GetFilterStatuses(ctx context.Context, filterID string) ([]*storage.FilterStatus, error) {
	var models []models.FilterStatus

	// Use range query to get all items with SK starting with "STATUS#"
	err := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", ">=", "STATUS#").
		Where("SK", "<", "STATUS~"). // Use ~ as upper bound since it's after # in ASCII
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query filter statuses: %w", err)
	}

	statuses := make([]*storage.FilterStatus, len(models))
	for i, model := range models {
		statuses[i] = &storage.FilterStatus{
			ID:        model.ID,
			FilterID:  model.FilterID,
			StatusID:  model.StatusID,
			CreatedAt: model.CreatedAt,
		}
	}

	return statuses, nil
}

// DeleteFilterStatus deletes a filter status by statusID (the ID being filtered, not the filter entry ID)
func (r *ModerationRepository) DeleteFilterStatus(ctx context.Context, statusID string) error {
	// First find the status to get its FilterID
	var existingModels []models.FilterStatus
	
	// Look for the filter status entry that filters this statusID
	err := r.db.WithContext(ctx).Model(&existingModels).
		Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		All(&existingModels)

	if err != nil || len(existingModels) == 0 {
		if errors.IsNotFound(err) || len(existingModels) == 0 {
			return fmt.Errorf("filter status not found")
		}
		return fmt.Errorf("failed to find filter status: %w", err)
	}

	existing := existingModels[0]

	// Delete the status
	err = r.db.WithContext(ctx).Model(&models.FilterStatus{}).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", existing.FilterID)).
		Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Delete()

	if err != nil {
		return fmt.Errorf("failed to delete filter status: %w", err)
	}

	return nil
}

// REPORT METHODS

// AssignReport assigns a report to a moderator
func (r *ModerationRepository) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	// First get the existing report
	report, err := r.GetReport(ctx, reportID)
	if err != nil {
		return fmt.Errorf("failed to get report for assignment: %w", err)
	}

	// Update the fields
	now := time.Now()
	report.AssignedTo = assignedTo
	report.UpdatedAt = now

	// Convert to model and update
	model := &models.Report{
		ID:                report.ID,
		ReporterID:        report.ReporterID,
		TargetAccountID:   report.TargetAccountID,
		StatusIDs:         report.StatusIDs,
		Comment:           report.Comment,
		Category:          report.Category,
		RuleIDs:           func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            string(report.Status),
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys()

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to assign report",
			zap.Error(err),
			zap.String("report_id", reportID),
			zap.String("assigned_to", assignedTo))
		return fmt.Errorf("failed to assign report: %w", err)
	}

	r.logger.Debug("Assigned report",
		zap.String("report_id", reportID),
		zap.String("assigned_to", assignedTo))

	return nil
}

// UnassignReport removes assignment from a report
func (r *ModerationRepository) UnassignReport(ctx context.Context, reportID string) error {
	// First get the existing report
	report, err := r.GetReport(ctx, reportID)
	if err != nil {
		return fmt.Errorf("failed to get report for unassignment: %w", err)
	}

	// Update the fields
	now := time.Now()
	report.AssignedTo = ""
	report.UpdatedAt = now

	// Convert to model and update
	model := &models.Report{
		ID:                report.ID,
		ReporterID:        report.ReporterID,
		TargetAccountID:   report.TargetAccountID,
		StatusIDs:         report.StatusIDs,
		Comment:           report.Comment,
		Category:          report.Category,
		RuleIDs:           func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            string(report.Status),
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys()

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to unassign report",
			zap.Error(err),
			zap.String("report_id", reportID))
		return fmt.Errorf("failed to unassign report: %w", err)
	}

	r.logger.Debug("Unassigned report",
		zap.String("report_id", reportID))

	return nil
}

// GetOpenReportsCount returns the count of open reports
func (r *ModerationRepository) GetOpenReportsCount(ctx context.Context) (int, error) {
	// Count reports with status "open" using GSI3
	count, err := r.db.WithContext(ctx).Model(&models.Report{}).
		Index("GSI3").
		Where("GSI3PK", "=", "STATUS#open").
		Count()

	if err != nil {
		r.logger.Error("Failed to count open reports", zap.Error(err))
		// Return 0 instead of error to match legacy behavior
		return 0, nil
	}

	return int(count), nil
}

// GetReportedStatuses retrieves statuses associated with a report
func (r *ModerationRepository) GetReportedStatuses(ctx context.Context, reportID string) ([]any, error) {
	// First get the report to access its StatusIDs
	var report models.Report
	err := r.db.WithContext(ctx).Model(&report).
		Where("PK", "=", fmt.Sprintf("REPORT#%s", reportID)).
		Where("SK", "=", "REPORT").
		First(&report)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	// Convert string slice to []any to match interface
	result := make([]any, len(report.StatusIDs))
	for i, statusID := range report.StatusIDs {
		result[i] = statusID
	}

	return result, nil
}

// FLAG METHODS

// CreateFlag creates a new flag
func (r *ModerationRepository) CreateFlag(ctx context.Context, flag *storage.Flag) error {
	// Generate ID if not provided
	if flag.ID == "" {
		flag.ID = fmt.Sprintf("flag_%s", generateRandomString(12))
	}

	// Set timestamps and defaults
	now := time.Now()
	flag.CreatedAt = now
	if flag.Published.IsZero() {
		flag.Published = now
	}
	if flag.Status == "" {
		flag.Status = "pending"
	}

	// Create model and update keys
	model := &models.Flag{
		ID:         flag.ID,
		Actor:      flag.Actor,
		Object:     flag.Object,
		Content:    flag.Content,
		Published:  flag.Published,
		Status:     string(flag.Status),
		ReviewedBy: flag.ReviewedBy,
		ReviewedAt: flag.ReviewedAt,
		ReviewNote: flag.ReviewNote,
		CreatedAt:  flag.CreatedAt,
	}
	model.UpdateKeys()

	// Create the flag
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create flag",
			zap.Error(err),
			zap.String("flag_id", flag.ID),
			zap.String("actor", flag.Actor))
		return fmt.Errorf("failed to create flag: %w", err)
	}

	r.logger.Debug("Created flag",
		zap.String("flag_id", flag.ID),
		zap.String("actor", flag.Actor),
		zap.Strings("objects", flag.Object))

	return nil
}

// GetFlag retrieves a flag by ID
func (r *ModerationRepository) GetFlag(ctx context.Context, id string) (*storage.Flag, error) {
	// We need to scan for the flag since we don't know which object it's under
	var models []models.Flag
	err := r.db.WithContext(ctx).Model(&models).
		Where("SK", "LIKE", fmt.Sprintf("%%#%s", id)). // SK ends with the flag ID
		Limit(10). // Reasonable limit
		All(&models)

	if err != nil {
		return nil, fmt.Errorf("failed to query flag: %w", err)
	}

	// Find the matching flag
	for _, model := range models {
		if model.ID == id {
			return &storage.Flag{
				ID:         model.ID,
				Actor:      model.Actor,
				Object:     model.Object,
				Content:    model.Content,
				Published:  model.Published,
				Status:     model.Status,
				ReviewedBy: model.ReviewedBy,
				ReviewedAt: model.ReviewedAt,
				ReviewNote: model.ReviewNote,
				CreatedAt:  model.CreatedAt,
			}, nil
		}
	}

	return nil, fmt.Errorf("flag not found")
}

// GetFlagsByObject retrieves all flags for a specific object
func (r *ModerationRepository) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	var models []models.Flag

	query := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("FLAG#%s", objectID)).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query flags by object: %w", err)
	}

	flags := make([]*storage.Flag, len(models))
	for i, model := range models {
		flags[i] = &storage.Flag{
			ID:         model.ID,
			Actor:      model.Actor,
			Object:     model.Object,
			Content:    model.Content,
			Published:  model.Published,
			Status:     model.Status,
			ReviewedBy: model.ReviewedBy,
			ReviewedAt: model.ReviewedAt,
			ReviewNote: model.ReviewNote,
			CreatedAt:  model.CreatedAt,
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return flags, nextCursor, nil
}

// GetFlagsByActor retrieves all flags created by a specific actor
func (r *ModerationRepository) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	var models []models.Flag

	query := r.db.WithContext(ctx).Model(&models).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("ACTOR#%s", actorID)).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query flags by actor: %w", err)
	}

	flags := make([]*storage.Flag, len(models))
	for i, model := range models {
		flags[i] = &storage.Flag{
			ID:         model.ID,
			Actor:      model.Actor,
			Object:     model.Object,
			Content:    model.Content,
			Published:  model.Published,
			Status:     model.Status,
			ReviewedBy: model.ReviewedBy,
			ReviewedAt: model.ReviewedAt,
			ReviewNote: model.ReviewNote,
			CreatedAt:  model.CreatedAt,
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return flags, nextCursor, nil
}

// GetPendingFlags retrieves all pending flags
func (r *ModerationRepository) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	var models []models.Flag

	query := r.db.WithContext(ctx).Model(&models).
		Index("GSI2").
		Where("GSI2PK", "=", "FLAG_STATUS#pending").
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query pending flags: %w", err)
	}

	flags := make([]*storage.Flag, len(models))
	for i, model := range models {
		flags[i] = &storage.Flag{
			ID:         model.ID,
			Actor:      model.Actor,
			Object:     model.Object,
			Content:    model.Content,
			Published:  model.Published,
			Status:     model.Status,
			ReviewedBy: model.ReviewedBy,
			ReviewedAt: model.ReviewedAt,
			ReviewNote: model.ReviewNote,
			CreatedAt:  model.CreatedAt,
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return flags, nextCursor, nil
}

// UpdateFlagStatus updates the status of a flag
func (r *ModerationRepository) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	// First find the flag to get its primary key
	flag, err := r.GetFlag(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find flag: %w", err)
	}

	// Update the fields
	now := time.Now()
	flag.Status = string(status)
	flag.ReviewedBy = reviewedBy
	flag.ReviewedAt = &now
	flag.ReviewNote = reviewNote

	// Convert to model and update
	model := &models.Flag{
		ID:         flag.ID,
		Actor:      flag.Actor,
		Object:     flag.Object,
		Content:    flag.Content,
		Published:  flag.Published,
		Status:     string(flag.Status),
		ReviewedBy: flag.ReviewedBy,
		ReviewedAt: flag.ReviewedAt,
		ReviewNote: flag.ReviewNote,
		CreatedAt:  flag.CreatedAt,
	}
	model.UpdateKeys()

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to update flag status",
			zap.Error(err),
			zap.String("flag_id", id),
			zap.String("status", string(status)))
		return fmt.Errorf("failed to update flag status: %w", err)
	}

	r.logger.Debug("Updated flag status",
		zap.String("flag_id", id),
		zap.String("status", string(status)),
		zap.String("reviewed_by", reviewedBy))

	return nil
}

// CountPendingFlags returns the count of pending flags
func (r *ModerationRepository) CountPendingFlags(ctx context.Context) (int, error) {
	// Count flags with status "pending" using GSI2
	count, err := r.db.WithContext(ctx).Model(&models.Flag{}).
		Index("GSI2").
		Where("GSI2PK", "=", "FLAG_STATUS#pending").
		Count()

	if err != nil {
		r.logger.Error("Failed to count pending flags", zap.Error(err))
		// Return 0 instead of error to match legacy behavior
		return 0, nil
	}

	return int(count), nil
}

// ADDITIONAL REPORT METHODS

// CreateReport creates a new report
func (r *ModerationRepository) CreateReport(ctx context.Context, report *storage.Report) error {
	// Generate ID if not provided
	if report.ID == "" {
		report.ID = fmt.Sprintf("report_%s", generateRandomString(12))
	}

	// Set timestamps
	now := time.Now()
	report.CreatedAt = now
	report.UpdatedAt = now

	// Set default status if not provided
	if report.Status == "" {
		report.Status = "open"
	}

	// Create model and update keys
	model := &models.Report{
		ID:                report.ID,
		ReporterID:        report.ReporterID,
		TargetAccountID:   report.TargetAccountID,
		StatusIDs:         report.StatusIDs,
		Comment:           report.Comment,
		Category:          report.Category,
		RuleIDs:           func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            string(report.Status),
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys()

	// Create the report
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create report",
			zap.Error(err),
			zap.String("report_id", report.ID),
			zap.String("reporter_id", report.ReporterID))
		return fmt.Errorf("failed to create report: %w", err)
	}

	r.logger.Debug("Created report",
		zap.String("report_id", report.ID),
		zap.String("reporter_id", report.ReporterID),
		zap.String("target_account_id", report.TargetAccountID))

	return nil
}

// GetReport retrieves a report by ID
func (r *ModerationRepository) GetReport(ctx context.Context, id string) (*storage.Report, error) {
	var model models.Report

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("REPORT#%s", id)).
		Where("SK", "=", "REPORT").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	return &storage.Report{
		ID:                model.ID,
		ReporterID:        model.ReporterID,
		TargetAccountID:   model.TargetAccountID,
		StatusIDs:         model.StatusIDs,
		Comment:           model.Comment,
		Category:          model.Category,
		RuleIDs:           func() []string {
			var result []string
			for _, ruleID := range model.RuleIDs {
				result = append(result, strconv.Itoa(ruleID))
			}
			return result
		}(),
		Forwarded:         model.Forwarded,
		Status:            model.Status,
		ActionTaken:       model.ActionTaken,
		ActionTakenAt:     model.ActionTakenAt,
		ModeratorID:       model.ModeratorID,
		ModerationEventID: model.ModerationEventID,
		CreatedAt:         model.CreatedAt,
		UpdatedAt:         model.UpdatedAt,
		AssignedTo:        model.AssignedTo,
	}, nil
}

// GetUserReports retrieves all reports created by a user
func (r *ModerationRepository) GetUserReports(ctx context.Context, username string, limit int, cursor string) ([]*storage.Report, string, error) {
	var models []models.Report

	query := r.db.WithContext(ctx).Model(&models).
		Index("GSI1").
		Where("GSI1PK", "=", fmt.Sprintf("USER#%s", username)).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query user reports: %w", err)
	}

	reports := make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = &storage.Report{
			ID:                model.ID,
			ReporterID:        model.ReporterID,
			TargetAccountID:   model.TargetAccountID,
			StatusIDs:         model.StatusIDs,
			Comment:           model.Comment,
			Category:          model.Category,
			RuleIDs:           func() []string {
			var result []string
			for _, ruleID := range model.RuleIDs {
				result = append(result, strconv.Itoa(ruleID))
			}
			return result
		}(),
			Forwarded:         model.Forwarded,
			Status:            model.Status,
			ActionTaken:       model.ActionTaken,
			ActionTakenAt:     model.ActionTakenAt,
			ModeratorID:       model.ModeratorID,
			ModerationEventID: model.ModerationEventID,
			CreatedAt:         model.CreatedAt,
			UpdatedAt:         model.UpdatedAt,
			AssignedTo:        model.AssignedTo,
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return reports, nextCursor, nil
}

// UpdateReportStatus updates the status of a report
func (r *ModerationRepository) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	// First get the existing report
	report, err := r.GetReport(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get report for status update: %w", err)
	}

	// Update the fields
	now := time.Now()
	report.Status = string(status)
	report.ActionTaken = actionTaken
	report.ModeratorID = moderatorID
	report.UpdatedAt = now

	// Set ActionTakenAt if action was taken
	if actionTaken != "" {
		report.ActionTakenAt = &now
	}

	// Convert to model and update
	model := &models.Report{
		ID:                report.ID,
		ReporterID:        report.ReporterID,
		TargetAccountID:   report.TargetAccountID,
		StatusIDs:         report.StatusIDs,
		Comment:           report.Comment,
		Category:          report.Category,
		RuleIDs:           func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            string(report.Status),
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys()

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to update report status",
			zap.Error(err),
			zap.String("report_id", id),
			zap.String("status", string(status)))
		return fmt.Errorf("failed to update report status: %w", err)
	}

	r.logger.Debug("Updated report status",
		zap.String("report_id", id),
		zap.String("status", string(status)),
		zap.String("moderator_id", moderatorID))

	return nil
}

// GetReportsByTarget retrieves reports targeting a specific account
func (r *ModerationRepository) GetReportsByTarget(ctx context.Context, targetAccountID string, limit int, cursor string) ([]*storage.Report, string, error) {
	var models []models.Report

	query := r.db.WithContext(ctx).Model(&models).
		Index("GSI2").
		Where("GSI2PK", "=", fmt.Sprintf("REPORTED#%s", targetAccountID)).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query reports by target: %w", err)
	}

	reports := make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = &storage.Report{
			ID:                model.ID,
			ReporterID:        model.ReporterID,
			TargetAccountID:   model.TargetAccountID,
			StatusIDs:         model.StatusIDs,
			Comment:           model.Comment,
			Category:          model.Category,
			RuleIDs:           func() []string {
			var result []string
			for _, ruleID := range model.RuleIDs {
				result = append(result, strconv.Itoa(ruleID))
			}
			return result
		}(),
			Forwarded:         model.Forwarded,
			Status:            model.Status,
			ActionTaken:       model.ActionTaken,
			ActionTakenAt:     model.ActionTakenAt,
			ModeratorID:       model.ModeratorID,
			ModerationEventID: model.ModerationEventID,
			CreatedAt:         model.CreatedAt,
			UpdatedAt:         model.UpdatedAt,
			AssignedTo:        model.AssignedTo,
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return reports, nextCursor, nil
}

// GetReportsByStatus retrieves reports with a specific status
func (r *ModerationRepository) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	var models []models.Report

	query := r.db.WithContext(ctx).Model(&models).
		Index("GSI3").
		Where("GSI3PK", "=", fmt.Sprintf("STATUS#%s", string(status))).
		Limit(limit)

	// TODO: Implement cursor-based pagination
	if err := query.All(&models); err != nil {
		return nil, "", fmt.Errorf("failed to query reports by status: %w", err)
	}

	reports := make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = &storage.Report{
			ID:                model.ID,
			ReporterID:        model.ReporterID,
			TargetAccountID:   model.TargetAccountID,
			StatusIDs:         model.StatusIDs,
			Comment:           model.Comment,
			Category:          model.Category,
			RuleIDs:           func() []string {
			var result []string
			for _, ruleID := range model.RuleIDs {
				result = append(result, strconv.Itoa(ruleID))
			}
			return result
		}(),
			Forwarded:         model.Forwarded,
			Status:            model.Status,
			ActionTaken:       model.ActionTaken,
			ActionTakenAt:     model.ActionTakenAt,
			ModeratorID:       model.ModeratorID,
			ModerationEventID: model.ModerationEventID,
			CreatedAt:         model.CreatedAt,
			UpdatedAt:         model.UpdatedAt,
			AssignedTo:        model.AssignedTo,
		}
	}

	// TODO: Implement proper cursor generation
	nextCursor := ""

	return reports, nextCursor, nil
}

// GetReportStats retrieves reporting statistics for a user
func (r *ModerationRepository) GetReportStats(ctx context.Context, username string) (*storage.ReportStats, error) {
	var model models.ReportStats

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", "REPORT_STATS").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			// Return empty stats if none exist
			return &storage.ReportStats{
				TotalReports:    0,
				ResolvedReports: 0,
				FalseReports:    0,
				LastReportAt:    nil,
			}, nil
		}
		return nil, fmt.Errorf("failed to get report stats: %w", err)
	}

	return &storage.ReportStats{
		TotalReports:    model.TotalReports,
		ResolvedReports: model.ResolvedReports,
		FalseReports:    model.FalseReports,
		LastReportAt:    model.LastReportAt,
	}, nil
}

// IncrementFalseReports increments the false report count for a user
func (r *ModerationRepository) IncrementFalseReports(ctx context.Context, username string) error {
	now := time.Now()

	// Try to get existing stats
	existingStats, err := r.GetReportStats(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get existing report stats: %w", err)
	}

	// Use the existing LastReportAt pointer
	lastReportAt := existingStats.LastReportAt

	// Create/update the stats model
	model := &models.ReportStats{
		TotalReports:      existingStats.TotalReports,
		ResolvedReports:   existingStats.ResolvedReports,
		FalseReports:      existingStats.FalseReports + 1,
		LastReportAt:      lastReportAt,
		LastFalseReportAt: &now,
	}
	model.UpdateKeys(username)

	// Try update first, then create if not found
	err = r.db.WithContext(ctx).Model(model).Update()
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new record if not found
			err = r.db.WithContext(ctx).Model(model).Create()
		}
		if err != nil {
			r.logger.Error("Failed to increment false reports",
				zap.Error(err),
				zap.String("username", username))
			return fmt.Errorf("failed to increment false reports: %w", err)
		}
	}

	r.logger.Debug("Incremented false reports",
		zap.String("username", username),
		zap.Int("new_count", model.FalseReports))

	return nil
}

