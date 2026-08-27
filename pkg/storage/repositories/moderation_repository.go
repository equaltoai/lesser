package repositories

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// ModerationRepository implements moderation operations using enhanced DynamORM patterns
type ModerationRepository struct {
	*EnhancedBaseRepository[*models.ModerationEvent]
}

// NewModerationRepository creates a new moderation repository with enhanced functionality
func NewModerationRepository(db core.DB, tableName string, logger *zap.Logger) *ModerationRepository {
	// Create enhanced repository optimized for moderation operations
	enhancedRepo := NewEnhancedBaseRepository[*models.ModerationEvent](db, tableName, logger, nil, "ModerationRepository", "moderation_event")

	// Set up enhanced services for moderation operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Critical for moderation permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Moderation decisions cached
	enhancedRepo.SetEventService(NewDefaultEventService())           // Critical for moderation audit events

	return &ModerationRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// generateRandomString generates a cryptographically secure random string of 12 characters
func generateRandomString() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 12
	result := make([]byte, length)
	for i := range result {
		// Use crypto/rand for secure random generation
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fall back to timestamp-based generation if crypto/rand fails
			result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		} else {
			result[i] = charset[n.Int64()]
		}
	}
	return string(result)
}

// CreateModerationEvent creates a new moderation event using BaseRepository
func (r *ModerationRepository) CreateModerationEvent(ctx context.Context, event *storage.ModerationEvent) error {
	if common.ValidateRequiredParam("event.ID", event.ID) != nil {
		event.ID = fmt.Sprintf("evt_%s", generateRandomString())
	}
	event.Created = time.Now()
	event.Updated = event.Created

	// Set TTL if not specified (30 days default)
	if event.TTL == 0 {
		event.TTL = time.Now().Add(30 * 24 * time.Hour).Unix()
	}

	// Create model
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
		Type:            models.ModerationTypeEvent,
		TTL:             event.TTL,
		CreatedAt:       event.Created,
	}

	// Use BaseRepository Create method (automatically calls UpdateKeys and tracks cost)
	if err := r.Create(ctx, model); err != nil {
		r.logger.Error("Failed to create moderation event",
			zap.Error(err),
			zap.String("event_id", event.ID),
			zap.String("object_id", event.ObjectID))
		return ErrorHandler.HandleCreateError(err, EntityModerationEvent, event.ID)
	}

	r.logger.Debug("Created moderation event",
		zap.String("event_id", event.ID),
		zap.String("object_id", event.ObjectID),
		zap.String("type", event.EventType))

	return nil
}

// GetModerationEvent retrieves a moderation event by ID
func (r *ModerationRepository) GetModerationEvent(ctx context.Context, eventID string) (*storage.ModerationEvent, error) {
	var model models.ModerationEvent

	// Query using GSI3 for event ID lookup
	err := r.db.WithContext(ctx).Model(&model).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("EVENTID#%s", eventID)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityModerationEvent, eventID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityModerationEvent, eventID)
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
		Where("gsi2PK", "=", fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)).
		Limit(limit)

	if err := query.All(&models); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityModerationEvent, "queue")
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
			if filter.Action != "" && event.EventType != filter.Action {
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
	var eventModels []models.ModerationEvent

	query := r.db.WithContext(ctx).Model(&models.ModerationEvent{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&eventModels); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityModerationEvent, "queue paginated")
	}

	items := make([]*storage.ModerationQueueItem, 0, len(eventModels))
	for _, model := range eventModels {
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

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("eventModels", eventModels, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = eventModels[limit-1].GSI2SK
		eventModels = eventModels[:limit] // Trim to requested limit

		// Re-process the trimmed models to create items
		items = make([]*storage.ModerationQueueItem, 0, len(eventModels))
		for _, model := range eventModels {
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
	}

	return items, nextCursor, nil
}

// GetModerationEventsByObject retrieves all moderation events for an object
func (r *ModerationRepository) GetModerationEventsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	var models []models.ModerationEvent

	query := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("EVENT#%s", objectID)).
		OrderBy("SK", "DESC")

	if cursor != "" {
		query = query.Where("SK", "<", cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityModerationEvent, "by object")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("moderation models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].SK
		models = models[:limit] // Trim to requested limit
	}

	events := make([]*storage.ModerationEvent, 0, len(models))
	for _, model := range models {
		if model.Type == ModerationTypeEvent {
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

	return events, nextCursor, nil
}

// GetModerationEventsByActor retrieves all moderation events created by an actor
func (r *ModerationRepository) GetModerationEventsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	var models []models.ModerationEvent

	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("ACTOR#%s", actorID)).
		OrderBy("gsi1SK", "DESC")

	if cursor != "" {
		query = query.Where("gsi1SK", "<", cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityModerationEvent, "by actor")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].GSI1SK
		models = models[:limit] // Trim to requested limit
	}

	events := make([]*storage.ModerationEvent, 0, len(models))
	for _, model := range models {
		if model.Type == ModerationTypeEvent {
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

	return events, nextCursor, nil
}

// AddModerationReview adds a review to a moderation event
func (r *ModerationRepository) AddModerationReview(ctx context.Context, review *storage.ModerationReview) error {
	if common.ValidateRequiredParam("review.ID", review.ID) != nil {
		review.ID = fmt.Sprintf("rev_%s", generateRandomString())
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
	model.UpdateKeys() // Internal model operation

	// Create the review
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to add moderation review",
			zap.Error(err),
			zap.String("review_id", review.ID),
			zap.String("event_id", review.EventID))
		return ErrorHandler.HandleCreateError(err, "moderation review", review.ID)
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
		return nil, ErrorHandler.HandleQueryError(err, "moderation review", "query")
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
	if common.ValidateRequiredParam("decision.ID", decision.ID) != nil {
		decision.ID = fmt.Sprintf("dec_%s", generateRandomString())
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
		Reviews:          append([]interface{}(nil), decision.Reviews...),
		Metadata:         decision.Metadata,
		Decided:          decision.Decided,
		Expires:          decision.Expires,
		Type:             "DECISION",
		CreatedAt:        time.Now(),
		TTL:              time.Now().Add(90 * 24 * time.Hour).Unix(), // 90 days retention
	}
	model.UpdateKeys() // Internal model operation

	// Create the decision
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create moderation decision",
			zap.Error(err),
			zap.String("decision_id", decision.ID),
			zap.String("object_id", decision.ObjectID))
		return ErrorHandler.HandleCreateError(err, EntityModerationDecision, decision.ID)
	}

	r.logger.Info("Created moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", decision.Action),
		zap.Float64("consensus", decision.ConsensusScore))

	return nil
}

// GetModerationDecision retrieves the current decision for an object
func (r *ModerationRepository) GetModerationDecision(ctx context.Context, objectID string) (*storage.ModerationDecision, error) {
	var model models.ModerationDecision

	// Query using GSI1 for active decisions
	err := r.db.WithContext(ctx).Model(&model).
		Index("gsi1").
		Where("gsi1PK", "=", "ACTIVE_DECISIONS").
		Where(gsi1SKField, "=", fmt.Sprintf("OBJECT#%s", objectID)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityModerationDecision, objectID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityModerationDecision, objectID)
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
		return ErrorHandler.HandleGetError(err, EntityModerationDecision, "current")
	}

	// If no decision exists, we cannot update it
	if currentDecision == nil {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityModerationDecision, contentID)
	}

	// Create a new moderation decision based on the review
	newDecision := &storage.ModerationDecision{
		ID:               fmt.Sprintf("dec_%s", generateRandomString()),
		EventID:          currentDecision.EventID,
		ObjectID:         contentID,
		Action:           review.Action,
		ConsensusScore:   review.Confidence,
		ReviewerCount:    1,
		TrustWeightTotal: review.Confidence, // Using Confidence as Weight substitute
		Reviews: []interface{}{
			fmt.Sprintf("rev_%s", generateRandomString()),
		},
		Decided: time.Now(),
	}

	// Create the updated decision
	if err := r.CreateModerationDecision(ctx, newDecision); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityModerationDecision, contentID)
	}

	r.logger.Info("Updated moderation decision",
		zap.String("content_id", contentID),
		zap.String("reviewer", review.ReviewerID),
		zap.String("action", review.Action),
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
			Where("gsi2PK", "=", fmt.Sprintf("MODERATION_PATTERNS#%s", severity)).
			Limit(limit).
			All(&patternModels)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityModerationPattern, "query")
		}
	} else if active {
		// Query by active status only using GSI1
		err := r.db.WithContext(ctx).Model(&patternModels).
			Index("gsi1").
			Where("gsi1PK", "=", "MODERATION_PATTERNS#ACTIVE").
			Limit(limit).
			All(&patternModels)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityModerationPattern, "query")
		}
	} else {
		// All patterns (regardless of active status or severity) through GSI3.
		// The previous implementation scanned for SK = "PATTERN" and returned
		// nothing (the model writes SK = METADATA); the keyed query makes this
		// branch functional.
		err := r.db.WithContext(ctx).Model(&models.ModerationPattern{}).
			Index("gsi3").
			Where("gsi3PK", "=", "MODERATION_PATTERNS#ALL").
			OrderBy("gsi3SK", "ASC").
			Limit(limit).
			All(&patternModels)
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, EntityModerationPattern, "query")
		}
	}

	// Convert to storage patterns
	result := make([]*storage.ModerationPattern, len(patternModels))
	for i, model := range patternModels {
		result[i] = &storage.ModerationPattern{
			ID:          model.PatternID,
			Name:        model.Name,
			Description: model.Description,
			Type:        model.Type,
			Content:     model.Pattern,
			Severity:    fmt.Sprintf("%.2f", model.Severity),
			Active:      model.Active,
			CreatedAt:   model.CreatedAt,
			UpdatedAt:   model.UpdatedAt,
		}
	}

	return result, nil
}

// UpdateModerationPattern updates an existing moderation pattern
func (r *ModerationRepository) UpdateModerationPattern(ctx context.Context, pattern *storage.ModerationPattern) error {
	pattern.UpdatedAt = time.Now()

	// Parse severity from string to float64
	severity, _ := strconv.ParseFloat(pattern.Severity, 64)

	// Create model
	model := &models.ModerationPattern{
		PK:          fmt.Sprintf("PATTERN#%s", pattern.ID),
		SK:          "METADATA",
		PatternID:   pattern.ID,
		Name:        pattern.Name,
		Description: pattern.Description,
		Type:        pattern.Type,
		Pattern:     pattern.Content,
		Severity:    severity,
		Active:      pattern.Active,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
		TTL:         time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
	if err := model.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Update the pattern
	if err := r.db.WithContext(ctx).Model(model).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityModerationPattern, pattern.ID)
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
		return ErrorHandler.HandleDeleteError(err, EntityModerationPattern, patternID)
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
	if common.ValidateRequiredParam("pattern.ID", pattern.ID) != nil {
		pattern.ID = fmt.Sprintf("pat_%s", generateRandomString())
	}

	// Set timestamps
	now := time.Now()
	pattern.CreatedAt = now
	pattern.UpdatedAt = now

	// Parse severity from string to float64
	severity, _ := strconv.ParseFloat(pattern.Severity, 64)

	// Create model
	model := &models.ModerationPattern{
		PK:          fmt.Sprintf("PATTERN#%s", pattern.ID),
		SK:          "METADATA",
		PatternID:   pattern.ID,
		Name:        pattern.Name,
		Description: pattern.Description,
		Type:        pattern.Type,
		Pattern:     pattern.Content,
		Severity:    severity,
		Active:      pattern.Active,
		CreatedAt:   pattern.CreatedAt,
		UpdatedAt:   pattern.UpdatedAt,
		TTL:         time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
	if err := model.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Create the pattern
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityModerationPattern, pattern.ID)
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
		return nil, ErrorHandler.HandleGetError(err, EntityModerationPattern, patternID)
	}

	// Convert to storage pattern
	pattern := &storage.ModerationPattern{
		ID:          model.PatternID,
		Name:        model.Name,
		Description: model.Description,
		Type:        model.Type,
		Content:     model.Pattern,
		Severity:    fmt.Sprintf("%.2f", model.Severity),
		Active:      model.Active,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
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
	if common.ValidateSliceNotEmpty("history.Decisions", history.Decisions) == nil {
		lastDecision := history.Decisions[len(history.Decisions)-1]
		history.CurrentStatus = lastDecision.Action
	} else {
		history.CurrentStatus = StatusPending
	}

	return history, nil
}

// GetModerationEvents retrieves all moderation events with optional filters
func (r *ModerationRepository) GetModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	// Check if we should scan all events
	if r.shouldScanAllEvents(filter) {
		return r.scanAllModerationEvents(ctx, filter, limit, cursor)
	}

	// Route to specific query based on filter
	if filter.ObjectID != "" {
		return r.GetModerationEventsByObject(ctx, filter.ObjectID, limit, cursor)
	}

	if filter.ActorID != "" {
		return r.GetModerationEventsByActor(ctx, filter.ActorID, limit, cursor)
	}

	// Query by event type and category
	if filter.EventType != "" || filter.Category != "" {
		return r.queryByTypeAndCategory(ctx, filter, limit, cursor)
	}

	// Fallback to scan
	return r.scanAllModerationEvents(ctx, filter, limit, cursor)
}

// shouldScanAllEvents checks if we should scan all events instead of using an index
func (r *ModerationRepository) shouldScanAllEvents(filter *storage.ModerationEventFilter) bool {
	return filter == nil || (common.ValidateRequiredParam("eventType", filter.EventType) != nil &&
		common.ValidateRequiredParam("category", filter.Category) != nil &&
		common.ValidateRequiredParam("actorID", filter.ActorID) != nil &&
		common.ValidateRequiredParam("objectID", filter.ObjectID) != nil)
}

// queryByTypeAndCategory queries events by type and category using GSI2
func (r *ModerationRepository) queryByTypeAndCategory(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	// Build GSI2 key
	gsi2pk := r.buildGSI2Key(filter)

	// Execute query
	models, err := r.executeGSI2Query(ctx, gsi2pk, limit, cursor)
	if err != nil {
		return nil, "", err
	}

	// Process results
	events := r.processModelsToEvents(models, filter, limit)
	nextCursor := r.determineNextCursor(models, limit)

	// If we have a next cursor, reprocess with exact limit
	if nextCursor != "" {
		models = models[:limit]
		events = r.processModelsToEvents(models, filter, limit)
	}

	return events, nextCursor, nil
}

// buildGSI2Key builds the GSI2 partition key based on filter
func (r *ModerationRepository) buildGSI2Key(filter *storage.ModerationEventFilter) string {
	eventType := r.getEventType(filter)
	category := r.getCategory(filter)

	gsi2pk := fmt.Sprintf("TYPE#%s", eventType)
	if category != "" {
		gsi2pk = fmt.Sprintf("TYPE#%s#%s", eventType, category)
	}
	return gsi2pk
}

// getEventType extracts event type from filter or returns default
func (r *ModerationRepository) getEventType(filter *storage.ModerationEventFilter) storage.EventType {
	if filter.EventType != "" {
		return storage.EventType(filter.EventType)
	}
	return storage.EventTypeFlagged
}

// getCategory extracts category from filter
func (r *ModerationRepository) getCategory(filter *storage.ModerationEventFilter) string {
	if filter.Category != "" {
		return filter.Category
	}
	return ""
}

// executeGSI2Query executes the GSI2 query
func (r *ModerationRepository) executeGSI2Query(ctx context.Context, gsi2pk string, limit int, cursor string) ([]models.ModerationEvent, error) {
	var models []models.ModerationEvent
	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi2").
		Where("gsi2PK", "=", gsi2pk).
		OrderBy("gsi2SK", "DESC")

	if cursor != "" {
		query = query.Where("gsi2SK", "<", cursor)
	}

	query = query.Limit(limit + 1) // Get one more to check for pagination

	if err := query.All(&models); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityModerationEvent, "escalation query")
	}

	return models, nil
}

// processModelsToEvents converts models to events with filtering
func (r *ModerationRepository) processModelsToEvents(models []models.ModerationEvent, filter *storage.ModerationEventFilter, limit int) []*storage.ModerationEvent {
	events := make([]*storage.ModerationEvent, 0, len(models))

	for _, model := range models {
		if model.Type != ModerationTypeEvent {
			continue
		}

		event := r.modelToEvent(&model)
		if r.matchesEventFilter(event, filter) {
			events = append(events, event)
		}

		if len(events) >= limit {
			break
		}
	}

	return events
}

// modelToEvent converts a model to an event
func (r *ModerationRepository) modelToEvent(model *models.ModerationEvent) *storage.ModerationEvent {
	return &storage.ModerationEvent{
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
}

// determineNextCursor determines if there are more pages
func (r *ModerationRepository) determineNextCursor(models []models.ModerationEvent, limit int) string {
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		return models[limit-1].GSI2SK
	}
	return ""
}

// scanAllModerationEvents performs a scan operation to get all events
func (r *ModerationRepository) scanAllModerationEvents(ctx context.Context, filter *storage.ModerationEventFilter, limit int, cursor string) ([]*storage.ModerationEvent, string, error) {
	sanitizedLimit := limit
	switch {
	case sanitizedLimit <= 0:
		sanitizedLimit = 50
	case sanitizedLimit > 200:
		sanitizedLimit = 200
	}

	pageSize := sanitizedLimit * 3
	if pageSize < 50 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	events := make([]*storage.ModerationEvent, 0, sanitizedLimit)
	nextCursor := ""
	scanCursor := cursor

	for tries := 0; tries < 25 && len(events) < sanitizedLimit; tries++ {
		var pageModels []models.ModerationEvent

		query := r.db.WithContext(ctx).Model(&pageModels).
			Index("gsi4").
			Where("gsi4PK", "=", "MODERATION_EVENTS").
			OrderBy("gsi4SK", "DESC")

		if scanCursor != "" {
			query = query.Where("gsi4SK", "<", scanCursor)
		}

		if err := query.Limit(pageSize + 1).All(&pageModels); err != nil {
			return nil, "", ErrorHandler.HandleQueryError(err, EntityModerationEvent, "escalation query")
		}

		hasMore := len(pageModels) > pageSize
		if hasMore {
			pageModels = pageModels[:pageSize]
		}

		for _, model := range pageModels {
			if model.Type != ModerationTypeEvent {
				continue
			}

			event := r.modelToEvent(&model)
			if !r.matchesEventFilter(event, filter) {
				continue
			}

			events = append(events, event)
			if len(events) == sanitizedLimit {
				nextCursor = model.GSI4SK
				break
			}
		}

		if len(events) == sanitizedLimit {
			break
		}

		if !hasMore || len(pageModels) == 0 {
			break
		}
		scanCursor = pageModels[len(pageModels)-1].GSI4SK
	}

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
		ID:          fmt.Sprintf("admin_rev_%s", generateRandomString()),
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
		return ErrorHandler.HandleCreateError(err, "admin review", review.ID)
	}

	// Get the event to get the object ID
	event, err := r.GetModerationEvent(ctx, eventID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityModerationEvent, eventID)
	}

	// Immediately create a decision based on the admin action
	decision := &storage.ModerationDecision{
		ID:               fmt.Sprintf("admin_dec_%s", generateRandomString()),
		EventID:          eventID,
		ObjectID:         event.ObjectID,
		Action:           string(action), // Convert ActionType to string
		ConsensusScore:   1.0,            // Admin override has full consensus
		ReviewerCount:    1,
		TrustWeightTotal: 1000.0,
		Reviews: []interface{}{
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
		Decided: time.Now(),
	}

	// Create the decision
	if err := r.CreateModerationDecision(ctx, decision); err != nil {
		return ErrorHandler.HandleCreateError(err, "admin decision", decision.ID)
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

	// Count all reviews by this reviewer through GSI1 instead of scanning the
	// whole table and filtering in Go.
	var reviews []models.ModerationReview
	err = r.db.WithContext(ctx).Model(&models.ModerationReview{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("REVIEWER#%s", reviewerID)).
		All(&reviews)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "admin review", "scan")
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
		Where("gsi2PK", "=", fmt.Sprintf("TYPE#%s#pending", moderation.EventTypeFlagged)).
		Count()

	if err != nil {
		// If error, return 0 instead of failing (matches legacy behavior)
		return 0, nil
	}

	return int(count), nil
}

// RecordPatternMatch records a moderation pattern match for analytics
func (r *ModerationRepository) RecordPatternMatch(_ context.Context, patternID string, matched bool, timestamp time.Time) error {
	// Create analytics record
	analytics := &models.ModerationAnalytics{
		PatternID: patternID,
		Matched:   matched,
		Timestamp: timestamp,
		CreatedAt: time.Now(),
	}
	_ = analytics.UpdateKeys() // Ignore error as this is internal model operation

	err := r.db.Model(analytics).Create()
	if err != nil {
		r.logger.Error("failed to record pattern match",
			zap.String("pattern_id", patternID),
			zap.Bool("matched", matched),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "pattern match", "pattern match")
	}

	return nil
}

// FILTER METHODS

// CreateFilter creates a new filter
func (r *ModerationRepository) CreateFilter(ctx context.Context, filter *storage.Filter) error {
	// Generate ID if not provided
	if common.ValidateRequiredParam("filter.ID", filter.ID) != nil {
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
	if err := model.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Create the filter
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create filter",
			zap.Error(err),
			zap.String("filter_id", filter.ID),
			zap.String("username", filter.Username))
		return ErrorHandler.HandleCreateError(err, EntityFilter, filter.ID)
	}

	r.logger.Debug("Created filter",
		zap.String("filter_id", filter.ID),
		zap.String("username", filter.Username),
		zap.String("title", filter.Title))

	return nil
}

// GetFilter retrieves a filter by ID
func (r *ModerationRepository) GetFilter(ctx context.Context, filterID string) (*storage.Filter, error) {
	var models []models.Filter

	err := r.db.WithContext(ctx).Model(&models).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Limit(1).
		All(&models)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilter, "by name")
	}

	if err := common.ValidateSliceNotEmpty("filters", models); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityFilter, "not found")
	}

	model := models[0]
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

// GetFiltersForUser retrieves all filters for a user
func (r *ModerationRepository) GetFiltersForUser(ctx context.Context, username string) ([]*storage.Filter, error) {
	var models []models.Filter

	err := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", ">=", "FILTER#").
		Where("SK", "<", "FILTER~"). // Use ~ as upper bound since it's after # in ASCII
		All(&models)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFilter, "query")
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
		return ErrorHandler.HandleGetError(err, EntityFilter, filterID)
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
	if err := model.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	if err := r.db.WithContext(ctx).Model(model).Update(); err != nil {
		r.logger.Error("Failed to update filter",
			zap.Error(err),
			zap.String("filter_id", filterID))
		return ErrorHandler.HandleUpdateError(err, EntityFilter, filterID)
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
		return ErrorHandler.HandleGetError(err, EntityFilter, filterID)
	}

	// Delete all keywords first
	keywords, err := r.GetFilterKeywords(ctx, filterID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityFilterKeyword, "deletion")
	}
	for _, keyword := range keywords {
		if err := r.DeleteFilterKeyword(ctx, filterID, keyword.ID); err != nil {
			r.logger.Error("Failed to delete filter keyword during filter deletion",
				zap.Error(err),
				zap.String("keyword_id", keyword.ID))
			// Continue with other deletions
		}
	}

	// Delete all statuses
	statuses, err := r.GetFilterStatuses(ctx, filterID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityFilterStatus, "deletion")
	}
	for _, status := range statuses {
		if err := r.DeleteFilterStatus(ctx, filterID, status.StatusID); err != nil {
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
		return ErrorHandler.HandleDeleteError(err, EntityFilter, filterID)
	}

	r.logger.Debug("Deleted filter",
		zap.String("filter_id", filterID),
		zap.String("username", filter.Username))

	return nil
}

// AddFilterKeyword adds a new keyword to a filter
func (r *ModerationRepository) AddFilterKeyword(ctx context.Context, filterID string, keyword *storage.FilterKeyword) error {
	// Generate UUID if not provided
	if common.ValidateRequiredParam("keyword.ID", keyword.ID) != nil {
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
	if err := model.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Create the keyword
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to add filter keyword",
			zap.Error(err),
			zap.String("filter_id", filterID),
			zap.String("keyword_id", keyword.ID),
			zap.String("keyword", keyword.Keyword))
		return ErrorHandler.HandleCreateError(err, EntityFilterKeyword, keyword.ID)
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
		return nil, ErrorHandler.HandleQueryError(err, EntityFilterKeyword, "query")
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
func (r *ModerationRepository) UpdateFilterKeyword(ctx context.Context, filterID string, keywordID string, updates map[string]any) error {
	// Point read the keyword within its filter partition (no scan needed)
	var existing models.FilterKeyword
	err := r.db.WithContext(ctx).Model(&existing).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", "=", fmt.Sprintf("KEYWORD#%s", keywordID)).
		First(&existing)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityFilterKeyword, keywordID)
		}
		return ErrorHandler.HandleGetError(err, EntityFilterKeyword, keywordID)
	}

	// Apply updates
	if keyword, ok := updates["keyword"].(string); ok {
		existing.Keyword = keyword
	}
	if wholeWord, ok := updates["whole_word"].(bool); ok {
		existing.WholeWord = wholeWord
	}

	// Update the keyword
	if err := r.db.WithContext(ctx).Model(&existing).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityFilterKeyword, keywordID)
	}

	return nil
}

// DeleteFilterKeyword deletes a filter keyword
func (r *ModerationRepository) DeleteFilterKeyword(ctx context.Context, filterID string, keywordID string) error {
	err := r.db.WithContext(ctx).Model(&models.FilterKeyword{}).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", "=", fmt.Sprintf("KEYWORD#%s", keywordID)).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "filter entity", keywordID)
	}

	return nil
}

// AddFilterStatus adds a new status to a filter
func (r *ModerationRepository) AddFilterStatus(ctx context.Context, filterID string, status *storage.FilterStatus) error {
	// Generate UUID if not provided
	if common.ValidateRequiredParam("status.ID", status.ID) != nil {
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
	if err := model.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}

	// Create the status
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to add filter status",
			zap.Error(err),
			zap.String("filter_id", filterID),
			zap.String("status_filter_id", status.ID),
			zap.String("status_id", status.StatusID))
		return ErrorHandler.HandleCreateError(err, EntityFilterStatus, status.ID)
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
		return nil, ErrorHandler.HandleQueryError(err, EntityFilterStatus, "query")
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
func (r *ModerationRepository) DeleteFilterStatus(ctx context.Context, filterID string, statusID string) error {
	err := r.db.WithContext(ctx).Model(&models.FilterStatus{}).
		Where("PK", "=", fmt.Sprintf("FILTER#%s", filterID)).
		Where("SK", "=", fmt.Sprintf("STATUS#%s", statusID)).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "filter entity", statusID)
	}

	return nil
}

// convertReportModelToStorage converts a Report model to storage.Report
// This helper eliminates duplication in report conversion across different query methods
func (r *ModerationRepository) convertReportModelToStorage(model models.Report) *storage.Report {
	return &storage.Report{
		ID:              model.ID,
		ReporterID:      model.ReporterID,
		TargetAccountID: model.TargetAccountID,
		StatusIDs:       model.StatusIDs,
		Comment:         model.Comment,
		Category:        model.Category,
		RuleIDs: func() []string {
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

// convertAuditLogModelToStorage converts an AuditLog model to storage.AuditLog
// This helper eliminates duplication in audit log conversion across different query methods
func (r *ModerationRepository) convertAuditLogModelToStorage(model models.AuditLog) *storage.AuditLog {
	return &storage.AuditLog{
		ID:         model.ID,
		AdminID:    model.AdminID,
		AdminRole:  model.AdminRole,
		Action:     model.Action,
		TargetType: model.TargetType,
		TargetID:   model.TargetID,
		Reason:     model.Reason,
		Details:    model.Details,
		IPAddress:  model.IPAddress,
		UserAgent:  model.UserAgent,
		RequestID:  model.RequestID,
		Timestamp:  model.Timestamp,
		CreatedAt:  model.CreatedAt,
	}
}

// REPORT METHODS

// AssignReport assigns a report to a moderator
func (r *ModerationRepository) AssignReport(ctx context.Context, reportID string, assignedTo string) error {
	// First get the existing report
	report, err := r.GetReport(ctx, reportID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityReport, reportID)
	}

	// Update the fields
	now := time.Now()
	report.AssignedTo = assignedTo
	report.UpdatedAt = now

	// Convert to model and update
	model := &models.Report{
		ID:              report.ID,
		ReporterID:      report.ReporterID,
		TargetAccountID: report.TargetAccountID,
		StatusIDs:       report.StatusIDs,
		Comment:         report.Comment,
		Category:        report.Category,
		RuleIDs: func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            report.Status,
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys() // Internal model operation

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to assign report",
			zap.Error(err),
			zap.String("report_id", reportID),
			zap.String("assigned_to", assignedTo))
		return ErrorHandler.HandleUpdateError(err, EntityReport, reportID)
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
		return ErrorHandler.HandleGetError(err, EntityReport, reportID)
	}

	// Update the fields
	now := time.Now()
	report.AssignedTo = ""
	report.UpdatedAt = now

	// Convert to model and update
	model := &models.Report{
		ID:              report.ID,
		ReporterID:      report.ReporterID,
		TargetAccountID: report.TargetAccountID,
		StatusIDs:       report.StatusIDs,
		Comment:         report.Comment,
		Category:        report.Category,
		RuleIDs: func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            report.Status,
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys() // Internal model operation

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to unassign report",
			zap.Error(err),
			zap.String("report_id", reportID))
		return ErrorHandler.HandleUpdateError(err, EntityReport, reportID)
	}

	r.logger.Debug("Unassigned report",
		zap.String("report_id", reportID))

	return nil
}

// GetOpenReportsCount returns the count of open reports
func (r *ModerationRepository) GetOpenReportsCount(ctx context.Context) (int, error) {
	// Count reports with status "open" using GSI3
	count, err := r.db.WithContext(ctx).Model(&models.Report{}).
		Index("gsi3").
		Where("gsi3PK", "=", "STATUS#open").
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
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityReport, reportID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityReport, reportID)
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
	if common.ValidateRequiredParam("flag.ID", flag.ID) != nil {
		flag.ID = fmt.Sprintf("flag_%s", generateRandomString())
	}

	// Set timestamps and defaults
	now := time.Now()
	flag.CreatedAt = now
	if flag.Published.IsZero() {
		flag.Published = now
	}
	if common.ValidateRequiredParam("flag.Status", flag.Status) != nil {
		flag.Status = StatusPending
	}

	// Create model and update keys
	model := &models.Flag{
		ID:         flag.ID,
		Actor:      flag.Actor,
		Object:     flag.Object,
		Content:    flag.Content,
		Published:  flag.Published,
		Status:     flag.Status,
		ReviewedBy: flag.ReviewedBy,
		ReviewedAt: flag.ReviewedAt,
		ReviewNote: flag.ReviewNote,
		CreatedAt:  flag.CreatedAt,
	}
	model.UpdateKeys() // Internal model operation

	// Create the flag
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create flag",
			zap.Error(err),
			zap.String("flag_id", flag.ID),
			zap.String("actor", flag.Actor))
		return ErrorHandler.HandleCreateError(err, EntityFlag, flag.ID)
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
		Limit(10).                                     // Reasonable limit
		All(&models)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, EntityFlag, "query")
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

	return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityFlag, id)
}

// GetFlagsByObject retrieves all flags for a specific object
func (r *ModerationRepository) GetFlagsByObject(ctx context.Context, objectID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	var models []models.Flag

	query := r.db.WithContext(ctx).Model(&models).
		Where("PK", "=", fmt.Sprintf("FLAG#%s", objectID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityFlag, "by object")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].SK
		models = models[:limit] // Trim to requested limit
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

	return flags, nextCursor, nil
}

// GetFlagsByActor retrieves all flags created by a specific actor
func (r *ModerationRepository) GetFlagsByActor(ctx context.Context, actorID string, limit int, cursor string) ([]*storage.Flag, string, error) {
	var models []models.Flag

	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("ACTOR#%s", actorID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityFlag, "by actor")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].GSI1SK
		models = models[:limit] // Trim to requested limit
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

	return flags, nextCursor, nil
}

// GetPendingFlags retrieves all pending flags
func (r *ModerationRepository) GetPendingFlags(ctx context.Context, limit int, cursor string) ([]*storage.Flag, string, error) {
	var models []models.Flag

	query := r.db.WithContext(ctx).Model(&models).
		Index("gsi2").
		Where("gsi2PK", "=", "FLAG_STATUS#pending").
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityFlag, "pending")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].GSI2SK
		models = models[:limit] // Trim to requested limit
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

	return flags, nextCursor, nil
}

// UpdateFlagStatus updates the status of a flag
func (r *ModerationRepository) UpdateFlagStatus(ctx context.Context, id string, status storage.FlagStatus, reviewedBy string, reviewNote string) error {
	// First find the flag to get its primary key
	flag, err := r.GetFlag(ctx, id)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityFlag, id)
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
		Status:     flag.Status,
		ReviewedBy: flag.ReviewedBy,
		ReviewedAt: flag.ReviewedAt,
		ReviewNote: flag.ReviewNote,
		CreatedAt:  flag.CreatedAt,
	}
	model.UpdateKeys() // Internal model operation

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to update flag status",
			zap.Error(err),
			zap.String("flag_id", id),
			zap.String("status", string(status)))
		return ErrorHandler.HandleUpdateError(err, EntityFlag, id)
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
		Index("gsi2").
		Where("gsi2PK", "=", "FLAG_STATUS#pending").
		Count()

	if err != nil {
		r.logger.Error("Failed to count pending flags", zap.Error(err))
		// Return 0 instead of error to match legacy behavior
		return 0, nil
	}

	return int(count), nil
}

// DeleteFlag removes a flag
func (r *ModerationRepository) DeleteFlag(ctx context.Context, id string) error {
	// First find the flag to get its primary key information
	flag, err := r.GetFlag(ctx, id)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityFlag, id)
	}

	// Convert to model to get correct keys
	model := &models.Flag{
		ID:         flag.ID,
		Actor:      flag.Actor,
		Object:     flag.Object,
		Content:    flag.Content,
		Published:  flag.Published,
		Status:     flag.Status,
		ReviewedBy: flag.ReviewedBy,
		ReviewedAt: flag.ReviewedAt,
		ReviewNote: flag.ReviewNote,
		CreatedAt:  flag.CreatedAt,
	}
	model.UpdateKeys() // Internal model operation

	// Delete the flag
	if err := r.db.WithContext(ctx).Model(model).Delete(); err != nil {
		r.logger.Error("Failed to delete flag",
			zap.Error(err),
			zap.String("flag_id", id),
			zap.String("actor", flag.Actor))
		return ErrorHandler.HandleDeleteError(err, EntityFlag, id)
	}

	r.logger.Debug("Deleted flag",
		zap.String("flag_id", id),
		zap.String("actor", flag.Actor),
		zap.Strings("objects", flag.Object))

	return nil
}

// ADDITIONAL REPORT METHODS

// CreateReport creates a new report
func (r *ModerationRepository) CreateReport(ctx context.Context, report *storage.Report) error {
	// Generate ID if not provided
	if common.ValidateRequiredParam("report.ID", report.ID) != nil {
		report.ID = fmt.Sprintf("report_%s", generateRandomString())
	}

	// Set timestamps
	now := time.Now()
	report.CreatedAt = now
	report.UpdatedAt = now

	// Set default status if not provided
	if common.ValidateRequiredParam("report.Status", report.Status) != nil {
		report.Status = "open"
	}

	// Create model and update keys
	model := &models.Report{
		ID:              report.ID,
		ReporterID:      report.ReporterID,
		TargetAccountID: report.TargetAccountID,
		StatusIDs:       report.StatusIDs,
		Comment:         report.Comment,
		Category:        report.Category,
		RuleIDs: func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            report.Status,
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys() // Internal model operation

	// Create the report
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create report",
			zap.Error(err),
			zap.String("report_id", report.ID),
			zap.String("reporter_id", report.ReporterID))
		return ErrorHandler.HandleCreateError(err, EntityReport, report.ID)
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
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityReport, id)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityReport, id)
	}

	return &storage.Report{
		ID:              model.ID,
		ReporterID:      model.ReporterID,
		TargetAccountID: model.TargetAccountID,
		StatusIDs:       model.StatusIDs,
		Comment:         model.Comment,
		Category:        model.Category,
		RuleIDs: func() []string {
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
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("USER#%s", username)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityReport, "user reports")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].GSI1SK
		models = models[:limit] // Trim to requested limit
	}

	reports := make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = &storage.Report{
			ID:              model.ID,
			ReporterID:      model.ReporterID,
			TargetAccountID: model.TargetAccountID,
			StatusIDs:       model.StatusIDs,
			Comment:         model.Comment,
			Category:        model.Category,
			RuleIDs: func() []string {
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

	return reports, nextCursor, nil
}

// UpdateReportStatus updates the status of a report
func (r *ModerationRepository) UpdateReportStatus(ctx context.Context, id string, status storage.ReportStatus, actionTaken string, moderatorID string) error {
	// First get the existing report
	report, err := r.GetReport(ctx, id)
	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityReport, id)
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
		ID:              report.ID,
		ReporterID:      report.ReporterID,
		TargetAccountID: report.TargetAccountID,
		StatusIDs:       report.StatusIDs,
		Comment:         report.Comment,
		Category:        report.Category,
		RuleIDs: func() []int {
			var result []int
			for _, ruleID := range report.RuleIDs {
				if id, err := strconv.Atoi(ruleID); err == nil {
					result = append(result, id)
				}
			}
			return result
		}(),
		Forwarded:         report.Forwarded,
		Status:            report.Status,
		ActionTaken:       report.ActionTaken,
		ActionTakenAt:     report.ActionTakenAt,
		ModeratorID:       report.ModeratorID,
		ModerationEventID: report.ModerationEventID,
		CreatedAt:         report.CreatedAt,
		UpdatedAt:         report.UpdatedAt,
		AssignedTo:        report.AssignedTo,
	}
	model.UpdateKeys() // Internal model operation

	err = r.db.WithContext(ctx).Model(model).Update()

	if err != nil {
		r.logger.Error("Failed to update report status",
			zap.Error(err),
			zap.String("report_id", id),
			zap.String("status", string(status)))
		return ErrorHandler.HandleUpdateError(err, EntityReport, id)
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
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("REPORTED#%s", targetAccountID)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&models); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityReport, "by target")
	}

	reports := make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = &storage.Report{
			ID:              model.ID,
			ReporterID:      model.ReporterID,
			TargetAccountID: model.TargetAccountID,
			StatusIDs:       model.StatusIDs,
			Comment:         model.Comment,
			Category:        model.Category,
			RuleIDs: func() []string {
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

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("models", models, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = models[limit-1].GSI2SK
		models = models[:limit] // Trim to requested limit
	}

	// Convert to storage types using our helper method
	reports = make([]*storage.Report, len(models))
	for i, model := range models {
		reports[i] = r.convertReportModelToStorage(model)
	}

	return reports, nextCursor, nil
}

// GetReportsByStatus retrieves reports with a specific status
func (r *ModerationRepository) GetReportsByStatus(ctx context.Context, status storage.ReportStatus, limit int, cursor string) ([]*storage.Report, string, error) {
	var reportModels []models.Report

	query := r.db.WithContext(ctx).Model(&models.Report{}).
		Index("gsi3").
		Where("gsi3PK", "=", fmt.Sprintf("STATUS#%s", string(status))).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to check if more pages exist
	query = query.Limit(limit + 1)

	if err := query.All(&reportModels); err != nil {
		if errors.IsNotFound(err) {
			return []*storage.Report{}, "", nil
		}
		return nil, "", ErrorHandler.HandleQueryError(err, EntityReport, "by status")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("reportModels", reportModels, limit); err != nil {
		// We got more results than requested, so there are more pages
		nextCursor = reportModels[limit-1].GSI3SK
		reportModels = reportModels[:limit] // Trim to requested limit
	}

	// Convert to storage types using our helper method
	reports := make([]*storage.Report, len(reportModels))
	for i, model := range reportModels {
		reports[i] = r.convertReportModelToStorage(model)
	}

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
		return nil, ErrorHandler.HandleGetError(err, "report stats", username)
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
		return ErrorHandler.HandleGetError(err, "report stats", username)
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
			return ErrorHandler.HandleUpdateError(err, "report stats", username)
		}
	}

	r.logger.Debug("Incremented false reports",
		zap.String("username", username),
		zap.Int("new_count", model.FalseReports))

	return nil
}

// CreateAuditLog creates a new audit log entry
func (r *ModerationRepository) CreateAuditLog(ctx context.Context, auditLog *storage.AuditLog) error {
	if common.ValidateRequiredParam("auditLog.ID", auditLog.ID) != nil {
		auditLog.ID = fmt.Sprintf("audit_%s", generateRandomString())
	}
	auditLog.Timestamp = time.Now()
	auditLog.CreatedAt = auditLog.Timestamp

	// Create model and update keys
	model := &models.AuditLog{
		ID:         auditLog.ID,
		AdminID:    auditLog.AdminID,
		AdminRole:  auditLog.AdminRole,
		Action:     auditLog.Action,
		TargetType: auditLog.TargetType,
		TargetID:   auditLog.TargetID,
		Reason:     auditLog.Reason,
		Details:    auditLog.Details,
		IPAddress:  auditLog.IPAddress,
		UserAgent:  auditLog.UserAgent,
		RequestID:  auditLog.RequestID,
		Timestamp:  auditLog.Timestamp,
		CreatedAt:  auditLog.CreatedAt,
	}
	model.UpdateKeys() // Internal model operation

	// Create the audit log entry
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to create audit log entry",
			zap.Error(err),
			zap.String("audit_id", auditLog.ID),
			zap.String("admin_id", auditLog.AdminID),
			zap.String("action", auditLog.Action))
		return ErrorHandler.HandleCreateError(err, "audit log", auditLog.ID)
	}

	r.logger.Debug("Created audit log entry",
		zap.String("audit_id", auditLog.ID),
		zap.String("admin_id", auditLog.AdminID),
		zap.String("action", auditLog.Action),
		zap.String("target_type", auditLog.TargetType),
		zap.String("target_id", auditLog.TargetID))

	return nil
}

// GetAuditLogs retrieves audit log entries with pagination
func (r *ModerationRepository) GetAuditLogs(ctx context.Context, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	query := r.db.WithContext(ctx).Model(&models.AuditLog{}).
		Where("PK", "=", "AUDIT_LOG").
		Limit(limit)

	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	var models []*models.AuditLog
	if err := query.Scan(&models); err != nil {
		r.logger.Error("Failed to get audit logs",
			zap.Error(err),
			zap.Int("limit", limit))
		return nil, "", ErrorHandler.HandleQueryError(err, "audit log", "query")
	}

	// Convert models to storage types
	logs := make([]*storage.AuditLog, 0, len(models))
	for _, model := range models {
		log := &storage.AuditLog{
			ID:         model.ID,
			AdminID:    model.AdminID,
			AdminRole:  model.AdminRole,
			Action:     model.Action,
			TargetType: model.TargetType,
			TargetID:   model.TargetID,
			Reason:     model.Reason,
			Details:    model.Details,
			IPAddress:  model.IPAddress,
			UserAgent:  model.UserAgent,
			RequestID:  model.RequestID,
			Timestamp:  model.Timestamp,
			CreatedAt:  model.CreatedAt,
		}
		logs = append(logs, log)
	}

	// Get next cursor - use the last item's SK if we got results
	nextCursor := ""
	if common.ValidateSliceNotEmpty("models", models) == nil {
		nextCursor = models[len(models)-1].SK
	}

	r.logger.Debug("Retrieved audit logs",
		zap.Int("count", len(logs)),
		zap.String("next_cursor", nextCursor))

	return logs, nextCursor, nil
}

// getAuditLogsByGSI is a helper function to retrieve audit logs using a specific GSI
func (r *ModerationRepository) getAuditLogsByGSI(ctx context.Context, gsiIndex, pkField, skField, idPrefix, id string, limit int, cursor string, logContext string) ([]*storage.AuditLog, string, error) {
	query := r.db.WithContext(ctx).Model(&models.AuditLog{}).
		Where(pkField, "=", fmt.Sprintf("%s#%s", idPrefix, id)).
		Index(gsiIndex).
		Limit(limit)

	if cursor != "" {
		query = query.Where(skField, ">", cursor)
	}

	var models []*models.AuditLog
	if err := query.Scan(&models); err != nil {
		r.logger.Error(fmt.Sprintf("Failed to get audit logs by %s", logContext),
			zap.Error(err),
			zap.String(fmt.Sprintf("%s_id", logContext), id),
			zap.Int("limit", limit))
		return nil, "", ErrorHandler.HandleQueryError(err, "audit log", logContext)
	}

	// Convert models to storage types using our helper method
	logs := make([]*storage.AuditLog, 0, len(models))
	for _, model := range models {
		logs = append(logs, r.convertAuditLogModelToStorage(*model))
	}

	// Get next cursor - use the last item's SK field if we got results
	nextCursor := ""
	if common.ValidateSliceNotEmpty("models", models) == nil {
		if skField == gsi1SKField {
			nextCursor = models[len(models)-1].GSI1SK
		} else {
			nextCursor = models[len(models)-1].GSI2SK
		}
	}

	r.logger.Debug(fmt.Sprintf("Retrieved audit logs by %s", logContext),
		zap.String(fmt.Sprintf("%s_id", logContext), id),
		zap.Int("count", len(logs)),
		zap.String("next_cursor", nextCursor))

	return logs, nextCursor, nil
}

// GetAuditLogsByAdmin retrieves audit log entries for a specific admin
func (r *ModerationRepository) GetAuditLogsByAdmin(ctx context.Context, adminID string, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	return r.getAuditLogsByGSI(ctx, "gsi1", "gsi1PK", gsi1SKField, "ADMIN", adminID, limit, cursor, "admin")
}

// GetAuditLogsByTarget retrieves audit log entries for a specific target
func (r *ModerationRepository) GetAuditLogsByTarget(ctx context.Context, targetID string, limit int, cursor string) ([]*storage.AuditLog, string, error) {
	return r.getAuditLogsByGSI(ctx, "gsi2", "gsi2PK", "gsi2SK", "TARGET", targetID, limit, cursor, "target")
}

// GetPendingModerationCount returns the count of pending moderation tasks for a specific moderator
func (r *ModerationRepository) GetPendingModerationCount(ctx context.Context, moderatorID string) (int, error) {
	r.logger.Debug("Getting pending moderation count for moderator",
		zap.String("moderator_id", moderatorID))

	// Count assigned reports that are still pending (open or in progress)
	var reportCount int

	// Get open reports
	openReportModels := []models.Report{}
	err := r.db.WithContext(ctx).Model(&models.Report{}).
		Where("AssignedTo", "=", moderatorID).
		Where("Status", "=", string(storage.ReportStatusOpen)).
		All(&openReportModels)
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to query open assigned reports",
			zap.String("moderator_id", moderatorID),
			zap.Error(err))
	}

	// Get in-progress reports
	inProgressReportModels := []models.Report{}
	err = r.db.WithContext(ctx).Model(&models.Report{}).
		Where("AssignedTo", "=", moderatorID).
		Where("Status", "=", string(storage.ReportStatusInProgress)).
		All(&inProgressReportModels)
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to query in-progress assigned reports",
			zap.String("moderator_id", moderatorID),
			zap.Error(err))
	}

	reportCount = len(openReportModels) + len(inProgressReportModels)

	// Count assigned flags that are still pending
	var flagCount int
	flagModels := []models.Flag{}
	err = r.db.WithContext(ctx).Model(&models.Flag{}).
		Where("AssignedTo", "=", moderatorID).
		Where("Status", "=", string(storage.FlagStatusPending)).
		All(&flagModels)
	if err != nil && !errors.IsNotFound(err) {
		r.logger.Warn("failed to query assigned flags",
			zap.String("moderator_id", moderatorID),
			zap.Error(err))
	} else {
		flagCount = len(flagModels)
	}

	// Total pending tasks = pending reports + pending flags
	totalPending := reportCount + flagCount

	r.logger.Debug("Retrieved pending moderation count",
		zap.String("moderator_id", moderatorID),
		zap.Int("pending_reports", reportCount),
		zap.Int("pending_flags", flagCount),
		zap.Int("total_pending", totalPending))

	return totalPending, nil
}

// StoreAnalysisResult stores detailed analysis results for audit/appeals
func (r *ModerationRepository) StoreAnalysisResult(ctx context.Context, analysisData map[string]interface{}) error {
	r.logger.Debug("Storing analysis result",
		zap.String("content_id", fmt.Sprintf("%v", analysisData["content_id"])),
		zap.String("analysis_type", fmt.Sprintf("%v", analysisData["analysis_type"])))

	// Extract required fields
	contentID, ok := analysisData["content_id"].(string)
	if !ok {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, "analysis result", "validation")
	}
	if err := common.ValidateRequiredParam("content_id", contentID); err != nil {
		return err
	}

	authorID, ok := analysisData["author_id"].(string)
	if !ok {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, "analysis result", "validation")
	}
	if err := common.ValidateRequiredParam("author_id", authorID); err != nil {
		return err
	}

	analysisType, ok := analysisData["analysis_type"].(string)
	if !ok || common.ValidateRequiredParam("analysisType", analysisType) != nil {
		analysisType = "combined"
	}

	// Create analysis result model
	model := &models.ModerationAnalysisResult{
		ID:           fmt.Sprintf("analysis_%d", time.Now().UnixNano()),
		ContentID:    contentID,
		AuthorID:     authorID,
		AnalysisType: analysisType,
		AnalyzedAt:   time.Now(),
	}

	// Set optional fields
	if contentType, ok := analysisData["content_type"].(string); ok {
		model.ContentType = contentType
	}

	if confidence, ok := analysisData["confidence"].(float64); ok {
		model.Confidence = confidence
	}

	if results, ok := analysisData["results"].(map[string]interface{}); ok {
		model.Results = results
	}

	if patternMatches, ok := analysisData["pattern_matches"].([]interface{}); ok {
		model.PatternMatches = patternMatches
	}

	if threatMatches, ok := analysisData["threat_matches"].([]interface{}); ok {
		model.ThreatMatches = threatMatches
	}

	if reputationScore, ok := analysisData["reputation_score"]; ok {
		model.ReputationScore = reputationScore
	}

	if processingTime, ok := analysisData["processing_time"].(int64); ok {
		model.ProcessingTime = processingTime
	}

	// Update keys and create
	model.UpdateKeys() // Internal model operation

	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to store analysis result",
			zap.Error(err),
			zap.String("content_id", contentID))
		return ErrorHandler.HandleCreateError(err, "analysis result", model.ID)
	}

	r.logger.Debug("Successfully stored analysis result",
		zap.String("content_id", contentID),
		zap.String("analysis_id", model.ID))

	return nil
}

// StoreDecision stores a moderation decision with enforcement tracking
func (r *ModerationRepository) StoreDecision(ctx context.Context, decisionData map[string]interface{}) error {
	r.logger.Debug("Storing moderation decision",
		zap.String("content_id", fmt.Sprintf("%v", decisionData["content_id"])),
		zap.String("action", fmt.Sprintf("%v", decisionData["action"])))

	// Extract required fields
	contentID, ok := decisionData["content_id"].(string)
	if !ok {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityModerationDecision, "validation")
	}
	if err := common.ValidateRequiredParam("content_id", contentID); err != nil {
		return err
	}

	action, ok := decisionData["action"].(string)
	if !ok {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, EntityModerationDecision, "validation")
	}
	if err := common.ValidateRequiredParam("action", action); err != nil {
		return err
	}

	// Create decision result model
	model := &models.ModerationDecisionResult{
		ID:                fmt.Sprintf("decision_%d", time.Now().UnixNano()),
		ContentID:         contentID,
		Action:            action,
		DecidedAt:         time.Now(),
		EnforcementStatus: "pending",
	}

	// Set optional fields
	if authorID, ok := decisionData["author_id"].(string); ok {
		model.AuthorID = authorID
	}

	if confidence, ok := decisionData["confidence"].(float64); ok {
		model.Confidence = confidence
	}

	if reasons, ok := decisionData["reasons"].([]interface{}); ok {
		model.Reasons = reasons
	}

	if requiresReview, ok := decisionData["requires_review"].(bool); ok {
		model.RequiresReview = requiresReview
	}

	if reviewPriority, ok := decisionData["review_priority"].(int); ok {
		model.ReviewPriority = reviewPriority
	}

	if recommendations, ok := decisionData["recommendations"].([]string); ok {
		model.Recommendations = recommendations
	}

	if expiresAt, ok := decisionData["expires_at"].(time.Time); ok {
		model.ExpiresAt = &expiresAt
	}

	if metadata, ok := decisionData["metadata"].(map[string]interface{}); ok {
		model.Metadata = metadata
	}

	// Update keys and create
	model.UpdateKeys() // Internal model operation

	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		r.logger.Error("Failed to store decision",
			zap.Error(err),
			zap.String("content_id", contentID))
		return ErrorHandler.HandleCreateError(err, EntityModerationDecision, model.ID)
	}

	// Add to review queue if requires review
	if model.RequiresReview {
		if err := r.addToReviewQueue(ctx, model); err != nil {
			r.logger.Warn("Failed to add decision to review queue",
				zap.Error(err),
				zap.String("decision_id", model.ID))
		}
	}

	r.logger.Debug("Successfully stored decision",
		zap.String("content_id", contentID),
		zap.String("decision_id", model.ID),
		zap.String("action", action))

	return nil
}

// GetReviewQueue retrieves review queue items with filtering
func (r *ModerationRepository) GetReviewQueue(ctx context.Context, filters map[string]interface{}) ([]*models.ModerationReviewQueue, error) {
	// Validate filters using centralized validation
	if err := common.ValidateQueryFilters(filters); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "review queue", "validation")
	}

	status := StatusPending
	if filterStatus, ok := filters["status"].(string); ok && filterStatus != "" {
		status = filterStatus
	}

	limit := 50
	if filterLimit, ok := filters["limit"].(int); ok && filterLimit > 0 {
		limit = filterLimit
	}

	r.logger.Debug("Getting review queue",
		zap.String("status", status),
		zap.Int("limit", limit))

	var queueItems []*models.ModerationReviewQueue

	err := r.db.WithContext(ctx).Model(&models.ModerationReviewQueue{}).
		Where("PK", "=", fmt.Sprintf("REVIEW_QUEUE#%s", status)).
		Where("SK", "prefix", "PRIORITY#").
		Limit(limit).
		All(&queueItems)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*models.ModerationReviewQueue{}, nil
		}
		r.logger.Error("Failed to get review queue",
			zap.Error(err),
			zap.String("status", status))
		return nil, ErrorHandler.HandleQueryError(err, "review queue", "query")
	}

	r.logger.Debug("Retrieved review queue items",
		zap.String("status", status),
		zap.Int("count", len(queueItems)))

	return queueItems, nil
}

// GetDecisionHistory retrieves decision history for a specific content ID
func (r *ModerationRepository) GetDecisionHistory(ctx context.Context, contentID string) ([]*models.ModerationDecisionResult, error) {
	r.logger.Debug("Getting decision history",
		zap.String("content_id", contentID))

	var decisions []*models.ModerationDecisionResult

	err := r.db.WithContext(ctx).Model(&models.ModerationDecisionResult{}).
		Where("PK", "=", fmt.Sprintf("DECISION_RESULT#%s", contentID)).
		Where("SK", "prefix", "TIME#").
		All(&decisions)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*models.ModerationDecisionResult{}, nil
		}
		r.logger.Error("Failed to get decision history",
			zap.Error(err),
			zap.String("content_id", contentID))
		return nil, ErrorHandler.HandleQueryError(err, EntityModerationDecision, "history")
	}

	r.logger.Debug("Retrieved decision history",
		zap.String("content_id", contentID),
		zap.Int("count", len(decisions)))

	return decisions, nil
}

// UpdateEnforcementStatus updates the enforcement status of a decision
func (r *ModerationRepository) UpdateEnforcementStatus(ctx context.Context, contentID, status string) error {
	r.logger.Debug("Updating enforcement status",
		zap.String("content_id", contentID),
		zap.String("status", status))

	// Get the most recent decision for this content
	var decisions []*models.ModerationDecisionResult
	err := r.db.WithContext(ctx).Model(&models.ModerationDecisionResult{}).
		Where("PK", "=", fmt.Sprintf("DECISION_RESULT#%s", contentID)).
		Where("SK", "prefix", "TIME#").
		Limit(1).
		All(&decisions)

	if err != nil || len(decisions) == 0 {
		r.logger.Error("No decision found to update enforcement status",
			zap.Error(err),
			zap.String("content_id", contentID))
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityModerationDecision, contentID)
	}

	decision := decisions[0]
	decision.EnforcementStatus = status
	now := time.Now()

	switch status {
	case "applied":
		decision.EnforcedAt = &now
		decision.EnforcementError = ""
	case "failed":
		decision.EnforcedAt = &now
		// Keep existing error message if any
	case "expired":
		decision.EnforcedAt = &now
	}

	// Update keys (in case GSI keys need updating)
	decision.UpdateKeys() // Internal model operation

	if err := r.db.WithContext(ctx).Model(decision).Update(); err != nil {
		r.logger.Error("Failed to update enforcement status",
			zap.Error(err),
			zap.String("content_id", contentID))
		return ErrorHandler.HandleUpdateError(err, EntityModerationDecision, contentID)
	}

	r.logger.Debug("Successfully updated enforcement status",
		zap.String("content_id", contentID),
		zap.String("status", status))

	return nil
}

// addToReviewQueue adds a decision to the review queue
func (r *ModerationRepository) addToReviewQueue(ctx context.Context, decision *models.ModerationDecisionResult) error {
	queueItem := &models.ModerationReviewQueue{
		ID:        fmt.Sprintf("queue_%d", time.Now().UnixNano()),
		ContentID: decision.ContentID,
		AuthorID:  decision.AuthorID,
		Status:    "pending",
		Priority:  decision.ReviewPriority,
		Category:  "moderation",
		Severity:  "medium", // Default
		Reason:    fmt.Sprintf("Action: %s (requires review)", decision.Action),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set evidence from decision
	if common.ValidateSliceNotEmpty("decision.Reasons", decision.Reasons) == nil || common.ValidateSliceNotEmpty("decision.Recommendations", decision.Recommendations) == nil {
		queueItem.Evidence = map[string]interface{}{
			"reasons":         decision.Reasons,
			"recommendations": decision.Recommendations,
			"confidence":      decision.Confidence,
		}
	}

	// Set deadline based on priority
	switch {
	case queueItem.Priority >= 8:
		deadline := time.Now().Add(1 * time.Hour)
		queueItem.Deadline = &deadline
	case queueItem.Priority >= 5:
		deadline := time.Now().Add(24 * time.Hour)
		queueItem.Deadline = &deadline
	default:
		deadline := time.Now().Add(7 * 24 * time.Hour)
		queueItem.Deadline = &deadline
	}

	// Update keys and create
	queueItem.UpdateKeys() // Internal model operation

	if err := r.db.WithContext(ctx).Model(queueItem).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, "review queue", queueItem.ID)
	}

	return nil
}

// GetModerationDecisionsByModerator retrieves moderation decisions made by a specific moderator
func (r *ModerationRepository) GetModerationDecisionsByModerator(ctx context.Context, moderatorUsername string, limit int) ([]*models.ModerationReview, error) {
	// Query moderation reviews by reviewer ID using the existing key structure
	// ModerationReview has SK=REVIEWER#{reviewer_id}, so we can scan for reviews by this moderator

	var reviews []*models.ModerationReview

	// Query reviews where SK starts with "REVIEWER#{moderatorUsername}"
	query := r.db.WithContext(ctx).Model(&models.ModerationReview{}).
		Where("SK", "=", fmt.Sprintf("REVIEWER#%s", moderatorUsername))

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.All(&reviews); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "moderation review", moderatorUsername)
	}

	return reviews, nil
}
