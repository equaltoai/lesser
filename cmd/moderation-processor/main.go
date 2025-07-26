package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamodb"
	"github.com/equaltoai/lesser/pkg/trust"
	"go.uber.org/zap"
)

var (
	store           storage.Storage
	consensusEngine *moderation.ConsensusEngine
	logger          *zap.Logger
)

// storageAdapter adapts storage.Storage to moderation.StorageInterface
type storageAdapter struct {
	storage storage.Storage
}

func (s *storageAdapter) GetModerationEvent(ctx context.Context, eventID string) (*moderation.ModerationEvent, error) {
	return s.storage.GetModerationEvent(ctx, eventID)
}

func (s *storageAdapter) AddModerationReview(ctx context.Context, review *moderation.Review) error {
	return s.storage.AddModerationReview(ctx, review)
}

func (s *storageAdapter) GetModerationReviews(ctx context.Context, eventID string) ([]*moderation.Review, error) {
	return s.storage.GetModerationReviews(ctx, eventID)
}

func (s *storageAdapter) CreateModerationDecision(ctx context.Context, decision *moderation.ModerationDecision) error {
	return s.storage.CreateModerationDecision(ctx, decision)
}

func (s *storageAdapter) GetModerationQueue(ctx context.Context, limit int, cursor string) ([]*moderation.QueueItem, string, error) {
	return s.storage.GetModerationQueuePaginated(ctx, limit, cursor)
}

func (s *storageAdapter) GetTrustScore(ctx context.Context, actorID, category string) (*trust.TrustScore, error) {
	return s.storage.GetTrustScore(ctx, actorID, category)
}

func (s *storageAdapter) RecordTrustUpdate(ctx context.Context, update *trust.TrustUpdate) error {
	return s.storage.RecordTrustUpdate(ctx, update)
}

func init() {
	// Initialize logger
	logger = common.Logger()

	// Initialize storage
	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}

	// Initialize consensus engine with storage adapter
	adapter := &storageAdapter{storage: store}
	consensusEngine = moderation.NewConsensusEngine(adapter, nil)
}

// handler processes DynamoDB stream events for moderation
func handler(ctx context.Context, event events.DynamoDBEvent) error {
	logger.Info("Processing DynamoDB stream event",
		zap.Int("records", len(event.Records)),
	)

	for _, record := range event.Records {
		if err := processRecord(ctx, record); err != nil {
			// Log error but continue processing other records
			logger.Error("Failed to process record",
				zap.String("event_id", record.EventID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// processRecord processes a single DynamoDB stream record
func processRecord(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Only process INSERT and MODIFY events
	if record.EventName != "INSERT" && record.EventName != "MODIFY" {
		return nil
	}

	// Check if this is a moderation-related record
	pk := getStringAttribute(record.Change.Keys["PK"])
	sk := getStringAttribute(record.Change.Keys["SK"])

	if pk == "" || sk == "" {
		return nil
	}

	logger.Debug("Processing record",
		zap.String("pk", pk),
		zap.String("sk", sk),
		zap.String("event_name", record.EventName),
	)

	// Handle different types of moderation records
	switch {
	case strings.HasPrefix(pk, "REVIEW#"):
		// New review added
		return handleNewReview(ctx, record)

	case strings.HasPrefix(pk, "EVENT#") && record.EventName == "INSERT":
		// New moderation event created
		return handleNewEvent(ctx, record)

	case strings.HasPrefix(pk, "DECISION#"):
		// Decision made - trigger actions
		return handleDecision(ctx, record)
	}

	return nil
}

// handleNewReview processes a new review and checks for consensus
func handleNewReview(ctx context.Context, record events.DynamoDBEventRecord) error {
	// Extract event ID from PK (REVIEW#eventID)
	pk := getStringAttribute(record.Change.Keys["PK"])
	parts := strings.Split(pk, "#")
	if len(parts) < 2 {
		return fmt.Errorf("invalid review PK format: %s", pk)
	}
	eventID := parts[1]

	// Extract reviewer ID from SK (REVIEWER#reviewerID)
	sk := getStringAttribute(record.Change.Keys["SK"])
	reviewerParts := strings.Split(sk, "#")
	if len(reviewerParts) < 2 {
		return fmt.Errorf("invalid review SK format: %s", sk)
	}
	reviewerID := reviewerParts[1]

	logger.Info("Processing new review",
		zap.String("event_id", eventID),
		zap.String("reviewer_id", reviewerID),
	)

	// Get the review details
	review, err := getReviewFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract review: %w", err)
	}

	// Process the review and check for consensus
	decision, err := consensusEngine.ProcessReview(ctx, eventID, review)
	if err != nil {
		logger.Warn("Failed to process review",
			zap.String("event_id", eventID),
			zap.Error(err),
		)
		return nil // Don't fail the whole batch
	}

	if decision != nil {
		logger.Info("Consensus reached",
			zap.String("event_id", eventID),
			zap.String("decision_id", decision.ID),
			zap.String("action", string(decision.Action)),
			zap.Float64("consensus_score", decision.ConsensusScore),
		)
	}

	return nil
}

// handleNewEvent processes a new moderation event
func handleNewEvent(ctx context.Context, record events.DynamoDBEventRecord) error {
	event, err := getEventFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract event: %w", err)
	}

	logger.Info("New moderation event created",
		zap.String("event_id", event.ID),
		zap.String("object_id", event.ObjectID),
		zap.String("type", string(event.EventType)),
		zap.String("category", string(event.Category)),
		zap.Int("severity", int(event.Severity)),
	)

	// Send notifications to moderators
	if err := sendModeratorNotification(ctx, event); err != nil {
		logger.Error("failed to send moderator notification", zap.Error(err))
	}

	// Trigger automatic actions based on severity
	if err := triggerAutomaticActions(ctx, event); err != nil {
		logger.Error("failed to trigger automatic actions", zap.Error(err))
	}

	return nil
}

// handleDecision processes a moderation decision
func handleDecision(ctx context.Context, record events.DynamoDBEventRecord) error {
	decision, err := getDecisionFromRecord(record)
	if err != nil {
		return fmt.Errorf("failed to extract decision: %w", err)
	}

	logger.Info("Processing moderation decision",
		zap.String("decision_id", decision.ID),
		zap.String("object_id", decision.ObjectID),
		zap.String("action", string(decision.Action)),
	)

	// Apply the decision based on action type
	switch decision.Action {
	case moderation.ActionTypeSilence:
		// Implement account silencing
		if err := silenceAccount(ctx, decision.ObjectID, decision.Reason); err != nil {
			logger.Error("failed to silence account", zap.Error(err))
			return err
		}
		logger.Info("Account silenced", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeSuspend:
		// Implement account suspension
		if err := suspendAccount(ctx, decision.ObjectID, decision.Reason); err != nil {
			logger.Error("failed to suspend account", zap.Error(err))
			return err
		}
		logger.Info("Account suspended", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeRemove:
		// Implement content removal
		if err := removeContent(ctx, decision.ObjectID, decision.Reason); err != nil {
			logger.Error("failed to remove content", zap.Error(err))
			return err
		}
		logger.Info("Content removed", zap.String("object_id", decision.ObjectID))

	case moderation.ActionTypeWarning:
		// Send warning notification
		if err := sendWarningNotification(ctx, decision.ObjectID, decision.Reason); err != nil {
			logger.Error("failed to send warning notification", zap.Error(err))
			return err
		}
		logger.Info("Warning notification sent", zap.String("object_id", decision.ObjectID))
	}

	return nil
}

// Helper functions to extract data from DynamoDB records

func getStringAttribute(attr events.DynamoDBAttributeValue) string {
	if attr.DataType() == events.DataTypeString {
		return attr.String()
	}
	return ""
}

func getReviewFromRecord(record events.DynamoDBEventRecord) (*moderation.Review, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "REVIEW" {
		return nil, fmt.Errorf("not a review record")
	}

	// Extract event ID from PK
	pk := getStringAttribute(record.Change.Keys["PK"])
	eventID := strings.TrimPrefix(pk, "REVIEW#")

	// Extract reviewer ID from SK
	sk := getStringAttribute(record.Change.Keys["SK"])
	reviewerID := strings.TrimPrefix(sk, "REVIEWER#")

	// Extract review data from NewImage
	review := &moderation.Review{
		EventID:    eventID,
		ReviewerID: reviewerID,
		Action:     moderation.ActionTypeWarning, // Default
		Confidence: 0.8,                          // Default
	}

	// Extract action if present
	if actionAttr, ok := record.Change.NewImage["Action"]; ok {
		if action := getStringAttribute(actionAttr); action != "" {
			review.Action = moderation.ActionType(action)
		}
	}

	// Extract confidence if present
	if confAttr, ok := record.Change.NewImage["Confidence"]; ok {
		if confAttr.DataType() == events.DataTypeNumber {
			if conf, err := confAttr.Float(); err == nil {
				review.Confidence = conf
			}
		}
	}

	return review, nil
}

func getEventFromRecord(record events.DynamoDBEventRecord) (*moderation.ModerationEvent, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "EVENT" {
		return nil, fmt.Errorf("not an event record")
	}

	// Extract event ID from PK
	pk := getStringAttribute(record.Change.Keys["PK"])
	eventID := strings.TrimPrefix(pk, "EVENT#")

	// Create event with extracted data
	event := &moderation.ModerationEvent{
		ID:        eventID,
		EventType: moderation.EventTypeFlagged,   // Default
		Category:  moderation.CategoryHateSpeech, // Default
		Severity:  moderation.SeverityMedium,     // Default
	}

	// Extract object ID if present
	if objAttr, ok := record.Change.NewImage["ObjectID"]; ok {
		event.ObjectID = getStringAttribute(objAttr)
	}

	// Extract actor ID if present
	if actorAttr, ok := record.Change.NewImage["ActorID"]; ok {
		event.ActorID = getStringAttribute(actorAttr)
	}

	// Extract event type if present
	if typeAttr, ok := record.Change.NewImage["EventType"]; ok {
		if eventType := getStringAttribute(typeAttr); eventType != "" {
			event.EventType = moderation.EventType(eventType)
		}
	}

	// Extract category if present
	if catAttr, ok := record.Change.NewImage["Category"]; ok {
		if category := getStringAttribute(catAttr); category != "" {
			event.Category = moderation.Category(category)
		}
	}

	// Extract severity if present
	if sevAttr, ok := record.Change.NewImage["Severity"]; ok {
		if sevAttr.DataType() == events.DataTypeNumber {
			if sev, err := sevAttr.Float(); err == nil {
				event.Severity = moderation.Severity(sev)
			}
		}
	}

	return event, nil
}

func getDecisionFromRecord(record events.DynamoDBEventRecord) (*moderation.ModerationDecision, error) {
	// Extract from NewImage
	typeAttr, ok := record.Change.NewImage["Type"]
	if !ok || getStringAttribute(typeAttr) != "DECISION" {
		return nil, fmt.Errorf("not a decision record")
	}

	// Extract decision ID from PK
	pk := getStringAttribute(record.Change.Keys["PK"])
	decisionID := strings.TrimPrefix(pk, "DECISION#")

	// Create decision with extracted data
	decision := &moderation.ModerationDecision{
		ID:     decisionID,
		Action: moderation.ActionTypeWarning, // Default
	}

	// Extract object ID if present
	if objAttr, ok := record.Change.NewImage["ObjectID"]; ok {
		decision.ObjectID = getStringAttribute(objAttr)
	}

	// Extract event ID if present
	if eventAttr, ok := record.Change.NewImage["EventID"]; ok {
		decision.EventID = getStringAttribute(eventAttr)
	}

	// Extract action if present
	if actionAttr, ok := record.Change.NewImage["Action"]; ok {
		if action := getStringAttribute(actionAttr); action != "" {
			decision.Action = moderation.ActionType(action)
		}
	}

	// Extract reason if present
	if reasonAttr, ok := record.Change.NewImage["Reason"]; ok {
		decision.Reason = getStringAttribute(reasonAttr)
	}

	// Extract consensus score if present
	if scoreAttr, ok := record.Change.NewImage["ConsensusScore"]; ok {
		if scoreAttr.DataType() == events.DataTypeNumber {
			if score, err := scoreAttr.Float(); err == nil {
				decision.ConsensusScore = score
			}
		}
	}

	return decision, nil
}

// sendModeratorNotification sends notifications to moderators about moderation events
func sendModeratorNotification(ctx context.Context, event *moderation.ModerationEvent) error {
	// Get list of moderators - using ListUsers with role filter
	users, _, err := store.ListUsers(ctx, 100, "") // Get first 100 users
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	// Filter for moderators
	var moderators []string
	for _, user := range users {
		if user.Role == "moderator" || user.Role == "admin" {
			moderators = append(moderators, user.Username)
		}
	}

	// Create notification for each moderator
	for _, moderatorID := range moderators {
		notification := &storage.Notification{
			ID:        fmt.Sprintf("mod_%s_%d", event.ID, time.Now().UnixNano()),
			Username:  moderatorID,
			Type:      "moderation",
			AccountID: event.ActorID,
			StatusID:  event.ObjectID,
			Read:      false,
			CreatedAt: time.Now(),
		}

		if err := store.CreateNotification(ctx, notification); err != nil {
			logger.Error("failed to create moderator notification",
				zap.String("moderator_id", moderatorID),
				zap.Error(err))
		}
	}

	return nil
}

// triggerAutomaticActions triggers automatic moderation actions based on severity
func triggerAutomaticActions(ctx context.Context, event *moderation.ModerationEvent) error {
	// Only trigger automatic actions for high severity events
	if event.Severity < moderation.SeverityHigh {
		return nil
	}

	// Create automatic decision for high severity content
	decision := &moderation.ModerationDecision{
		ID:               fmt.Sprintf("auto_%s_%d", event.ID, time.Now().UnixNano()),
		EventID:          event.ID,
		ObjectID:         event.ObjectID,
		Action:           getAutomaticAction(event.Category, event.Severity),
		ConsensusScore:   1.0, // Automatic decision has full consensus
		ReviewerCount:    1,
		TrustWeightTotal: 1.0,
		Reviews:          []*moderation.Review{}, // Empty for automatic decisions
		Decided:          time.Now(),
	}

	// Store the decision (this would trigger another Lambda via DynamoDB streams)
	return store.CreateModerationDecision(ctx, decision)
}

// getAutomaticAction determines the appropriate automatic action
func getAutomaticAction(category moderation.Category, severity moderation.Severity) moderation.ActionType {
	switch {
	case category == moderation.CategoryHateSpeech && severity >= moderation.SeverityHigh:
		return moderation.ActionTypeRemove
	case category == moderation.CategorySpam && severity >= moderation.SeverityHigh:
		return moderation.ActionTypeSilence
	case severity >= moderation.SeverityCritical:
		return moderation.ActionTypeSuspend
	default:
		return moderation.ActionTypeWarning
	}
}

// silenceAccount silences an account
func silenceAccount(ctx context.Context, accountID, reason string) error {
	// Extract username from accountID
	username := extractUsernameFromActorID(accountID)
	if username == "" {
		username = accountID // Fallback to using accountID as username
	}

	// Update user to be silenced
	updates := map[string]any{
		"silenced":          true,
		"moderation_reason": reason,
		"moderated_at":      time.Now(),
	}

	err := store.UpdateUser(ctx, username, updates)
	if err != nil {
		return fmt.Errorf("failed to silence account: %w", err)
	}

	logger.Info("silenced account",
		zap.String("account", accountID),
		zap.String("username", username),
		zap.String("reason", reason))
	return nil
}

// suspendAccount suspends an account
func suspendAccount(ctx context.Context, accountID, reason string) error {
	// Extract username from accountID
	username := extractUsernameFromActorID(accountID)
	if username == "" {
		username = accountID // Fallback to using accountID as username
	}

	// Update user to be suspended
	updates := map[string]any{
		"suspended":         true,
		"moderation_reason": reason,
		"moderated_at":      time.Now(),
	}

	err := store.UpdateUser(ctx, username, updates)
	if err != nil {
		return fmt.Errorf("failed to suspend account: %w", err)
	}

	// Clean up relationships - remove all follows involving this user
	if err := cleanupSuspendedAccountRelationships(ctx, username); err != nil {
		logger.Error("failed to clean up relationships for suspended account",
			zap.String("username", username),
			zap.Error(err))
		// Don't fail suspension due to cleanup errors
	}

	logger.Info("suspended account",
		zap.String("account", accountID),
		zap.String("username", username),
		zap.String("reason", reason))
	return nil
}

// removeContent removes content from the system
func removeContent(ctx context.Context, objectID, reason string) error {
	// First try to tombstone the object (marks as deleted but preserves references)
	err := store.TombstoneObject(ctx, objectID, "moderation-system")
	if err != nil {
		// If tombstoning fails, try direct deletion
		logger.Warn("failed to tombstone object, attempting direct deletion",
			zap.String("object_id", objectID),
			zap.Error(err))

		if deleteErr := store.DeleteObject(ctx, objectID); deleteErr != nil {
			return fmt.Errorf("failed to delete object after tombstone failure: %w", deleteErr)
		}
	}

	// Clean up associated data
	if err := cleanupRemovedContent(ctx, objectID); err != nil {
		logger.Error("failed to clean up removed content",
			zap.String("object_id", objectID),
			zap.Error(err))
		// Don't fail removal due to cleanup errors
	}

	logger.Info("content removed",
		zap.String("object", objectID),
		zap.String("reason", reason))
	return nil
}

// sendWarningNotification sends a warning notification to a user
func sendWarningNotification(ctx context.Context, objectID, reason string) error {
	// Get the object to find the author
	obj, err := store.GetObject(ctx, objectID)
	if err != nil {
		return fmt.Errorf("failed to get object: %w", err)
	}

	// Extract username from object
	username := extractUsernameFromObject(obj)
	if username == "" {
		return fmt.Errorf("unable to extract username from object")
	}

	// Create warning notification
	notification := &storage.Notification{
		ID:        fmt.Sprintf("warn_%s_%d", objectID, time.Now().UnixNano()),
		Username:  username,
		Type:      "moderation_warning",
		AccountID: "system", // System-generated warning
		StatusID:  objectID,
		Read:      false,
		CreatedAt: time.Now(),
	}

	return store.CreateNotification(ctx, notification)
}

// extractUsernameFromObject extracts username from an object
func extractUsernameFromObject(obj any) string {
	// Try to extract username from different object types
	switch v := obj.(type) {
	case map[string]any:
		// Check for actor field
		if actor, ok := v["actor"].(string); ok {
			return extractUsernameFromActorID(actor)
		}
		// Check for attributedTo field
		if attributedTo, ok := v["attributedTo"].(string); ok {
			return extractUsernameFromActorID(attributedTo)
		}
		// Check for username field directly
		if username, ok := v["username"].(string); ok {
			return username
		}
	case *activitypub.Note:
		return extractUsernameFromActorID(v.AttributedTo)
	case *activitypub.Activity:
		return extractUsernameFromActorID(v.Actor)
	}

	return "" // Unable to extract username
}

// extractUsernameFromActorID extracts username from an actor ID
func extractUsernameFromActorID(actorID string) string {
	// For local actors: https://domain.com/users/username -> username
	if strings.Contains(actorID, "/users/") {
		parts := strings.Split(actorID, "/users/")
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// For simple usernames without URL
	if !strings.Contains(actorID, "://") {
		return actorID
	}

	return "" // Unable to extract
}

// cleanupSuspendedAccountRelationships removes follows involving a suspended account
func cleanupSuspendedAccountRelationships(ctx context.Context, username string) error {
	// Get all followers and following for this user
	followers, _, err := store.GetFollowers(ctx, username, 1000, "")
	if err != nil {
		logger.Error("failed to get followers for cleanup", zap.Error(err))
	}

	following, _, err := store.GetFollowing(ctx, username, 1000, "")
	if err != nil {
		logger.Error("failed to get following for cleanup", zap.Error(err))
	}

	// Remove all follower relationships
	for _, followerID := range followers {
		followerUsername := extractUsernameFromActorID(followerID)
		if followerUsername != "" {
			if err := store.RemoveFollow(ctx, followerUsername, username); err != nil {
				logger.Error("failed to remove follower relationship",
					zap.String("follower", followerUsername),
					zap.String("suspended_user", username),
					zap.Error(err))
			}
		}
	}

	// Remove all following relationships
	for _, followedID := range following {
		followedUsername := extractUsernameFromActorID(followedID)
		if followedUsername != "" {
			if err := store.RemoveFollow(ctx, username, followedUsername); err != nil {
				logger.Error("failed to remove following relationship",
					zap.String("suspended_user", username),
					zap.String("followed", followedUsername),
					zap.Error(err))
			}
		}
	}

	return nil
}

// cleanupRemovedContent cleans up likes, announces, and other references to removed content
func cleanupRemovedContent(ctx context.Context, objectID string) error {
	// Remove likes for this content
	if err := store.CascadeDeleteLikes(ctx, objectID); err != nil {
		logger.Error("failed to cascade delete likes",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Remove announces (boosts) for this content
	if err := store.CascadeDeleteAnnounces(ctx, objectID); err != nil {
		logger.Error("failed to cascade delete announces",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	// Remove from timelines
	if err := store.DeleteFromTimeline(ctx, "all", "*", objectID); err != nil {
		logger.Error("failed to remove from timelines",
			zap.String("object_id", objectID),
			zap.Error(err))
	}

	return nil
}

func main() {
	lambda.Start(handler)
}
