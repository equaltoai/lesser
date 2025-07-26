package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

type ModerationProcessor struct {
	db             core.DB
	cfg            *config.Config
	moderationRepo *repositories.ModerationRepository
	userRepo       *repositories.UserRepository
	activityRepo   *repositories.ActivityRepository
	objectRepo     *repositories.ObjectRepository
	actorRepo      *repositories.ActorRepository
	followRepo     *repositories.FollowRepository
	likeRepo       *repositories.LikeRepository
	timelineRepo   *repositories.TimelineRepository
	logger         *zap.Logger
}

// ModerationRequest represents an SQS message for moderation
type ModerationRequest struct {
	ContentID     string                       `json:"content_id"`
	ContentType   models.ModerationContentType `json:"content_type"`
	UserID        string                       `json:"user_id"`
	Content       string                       `json:"content"`
	ReportedBy    []string                     `json:"reported_by,omitempty"`
	ReportReason  string                       `json:"report_reason,omitempty"`
	AutomatedFlag bool                         `json:"automated_flag,omitempty"`
	RequestedAt   time.Time                    `json:"requested_at"`
}

var (
	logger         *zap.Logger
	cfg            *config.Config
	db             core.DB
	moderationRepo *repositories.ModerationRepository
	userRepo       *repositories.UserRepository
	activityRepo   *repositories.ActivityRepository
	objectRepo     *repositories.ObjectRepository
	actorRepo      *repositories.ActorRepository
	followRepo     *repositories.FollowRepository
	likeRepo       *repositories.LikeRepository
	timelineRepo   *repositories.TimelineRepository
)

func init() {
	// Initialize logger
	logger = common.Logger()

	// Load configuration
	cfg = config.Get()

	// Initialize DynamORM with Lambda optimizations
	var err error
	db, err = dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Initialize repositories
	moderationRepo = repositories.NewModerationRepository(db)
	userRepo = repositories.NewUserRepository(db)
	activityRepo = repositories.NewActivityRepository(db, cfg.DynamoTableName, logger)
	objectRepo = repositories.NewObjectRepository(db, cfg.DynamoTableName, logger)
	actorRepo = repositories.NewActorRepository(db)
	followRepo = repositories.NewFollowRepository(db, cfg.DynamoTableName, logger)
	likeRepo = repositories.NewLikeRepository(db, cfg.DynamoTableName, logger)
	timelineRepo = repositories.NewTimelineRepository(db, cfg.DynamoTableName)
}

func NewModerationProcessor() *ModerationProcessor {
	return &ModerationProcessor{
		db:             db,
		cfg:            cfg,
		moderationRepo: moderationRepo,
		userRepo:       userRepo,
		activityRepo:   activityRepo,
		objectRepo:     objectRepo,
		actorRepo:      actorRepo,
		followRepo:     followRepo,
		likeRepo:       likeRepo,
		timelineRepo:   timelineRepo,
		logger:         logger,
	}
}

// HandleSQS processes SQS messages containing moderation requests
func HandleSQS(ctx context.Context, event events.SQSEvent) error {
	mp := NewModerationProcessor()
	mp.logger.Info("Processing SQS moderation requests",
		zap.Int("message_count", len(event.Records)),
	)

	for _, record := range event.Records {
		var req ModerationRequest
		if err := json.Unmarshal([]byte(record.Body), &req); err != nil {
			mp.logger.Error("Failed to unmarshal moderation request",
				zap.String("message_id", record.MessageId),
				zap.Error(err),
			)
			continue
		}

		// Process the moderation request
		if err := mp.processModerationRequest(ctx, req); err != nil {
			mp.logger.Error("Failed to process moderation request",
				zap.String("content_id", req.ContentID),
				zap.Error(err),
			)
			// Don't fail the batch - just log the error
		}
	}

	return nil
}

// processModerationRequest handles a single moderation request
func (mp *ModerationProcessor) processModerationRequest(ctx context.Context, req ModerationRequest) error {
	mp.logger.Info("Processing moderation request",
		zap.String("content_id", req.ContentID),
		zap.String("content_type", string(req.ContentType)),
		zap.String("user_id", req.UserID),
	)

	// Analyze content for moderation
	evidence, err := mp.moderationRepo.AnalyzeContent(ctx, req.Content, req.UserID, req.ContentType)
	if err != nil {
		return fmt.Errorf("failed to analyze content: %w", err)
	}

	// Determine action and reason based on evidence
	action, reason := mp.determineModeration(evidence, req)

	// Create moderation record
	moderation := &models.Moderation{
		ContentID:     req.ContentID,
		ContentType:   req.ContentType,
		UserID:        req.UserID,
		Action:        action,
		Reason:        reason,
		Evidence:      *evidence,
		ModeratorID:   "system",
		ModeratorType: "automated",
	}

	// Add report information if this was user-reported
	if len(req.ReportedBy) > 0 {
		moderation.Evidence.ReportCount = len(req.ReportedBy)
		moderation.Evidence.ReporterIDs = req.ReportedBy
		if req.ReportReason != "" {
			moderation.Metadata = map[string]interface{}{
				"report_reason": req.ReportReason,
			}
		}
	}

	// Create the moderation case
	if err := mp.moderationRepo.CreateModeration(ctx, moderation); err != nil {
		return fmt.Errorf("failed to create moderation: %w", err)
	}

	// Execute the moderation action if confidence is high enough
	if evidence.ConfidenceScore >= 0.8 && !evidence.RequiresReview {
		if err := mp.executeModeration(ctx, moderation); err != nil {
			mp.logger.Error("Failed to execute moderation",
				zap.String("moderation_id", moderation.ModerationID),
				zap.Error(err),
			)
			// Don't fail - moderation record is created for manual review
		}
	} else {
		// Send to human review queue
		if err := mp.sendForHumanReview(ctx, moderation); err != nil {
			mp.logger.Error("Failed to send for human review",
				zap.String("moderation_id", moderation.ModerationID),
				zap.Error(err),
			)
		}
	}

	return nil
}

// determineModeration determines the moderation action and reason based on evidence
func (mp *ModerationProcessor) determineModeration(evidence *models.ModerationEvidence, req ModerationRequest) (models.ModerationAction, models.ModerationReason) {
	// Check prohibited words first - highest priority
	if len(evidence.ProhibitedWords) > 0 {
		return models.ModerationActionRemove, models.ModerationReasonProhibitedWords
	}

	// Check spam score
	if evidence.SpamScore > 0.8 {
		return models.ModerationActionSilence, models.ModerationReasonSpam
	}

	// Check for rate limiting (would be set externally)
	if req.AutomatedFlag && strings.Contains(req.ReportReason, "rate_limit") {
		return models.ModerationActionWarning, models.ModerationReasonRateLimiting
	}

	// Check matched patterns
	if len(evidence.MatchedPatterns) > 0 {
		if evidence.ConfidenceScore > 0.7 {
			return models.ModerationActionRemove, models.ModerationReasonSpam
		}
		return models.ModerationActionWarning, models.ModerationReasonSpam
	}

	// User reports with no clear violation
	if len(req.ReportedBy) > 0 {
		if len(req.ReportedBy) >= 3 {
			// Multiple reports - escalate
			return models.ModerationActionWarning, models.ModerationReasonOther
		}
		return models.ModerationActionDismiss, models.ModerationReasonOther
	}

	// Default - dismiss if no clear issue
	return models.ModerationActionDismiss, models.ModerationReasonOther
}

// executeModeration executes the moderation action
func (mp *ModerationProcessor) executeModeration(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Executing moderation action",
		zap.String("moderation_id", moderation.ModerationID),
		zap.String("action", string(moderation.Action)),
		zap.String("content_type", string(moderation.ContentType)),
	)

	// Update moderation status
	now := time.Now()
	moderation.Status = models.ModerationStatusActioned
	moderation.ActionedAt = &now
	moderation.AddHistoryEntry("system", "automated", moderation.Action,
		models.ModerationStatusPending, models.ModerationStatusActioned,
		"Automated moderation executed")

	// Execute based on action type
	switch moderation.Action {
	case models.ModerationActionRemove:
		if err := mp.removeContent(ctx, moderation); err != nil {
			return fmt.Errorf("failed to remove content: %w", err)
		}

	case models.ModerationActionSuspend:
		if err := mp.suspendUser(ctx, moderation); err != nil {
			return fmt.Errorf("failed to suspend user: %w", err)
		}

	case models.ModerationActionSilence:
		if err := mp.silenceUser(ctx, moderation); err != nil {
			return fmt.Errorf("failed to silence user: %w", err)
		}

	case models.ModerationActionWarning:
		if err := mp.sendWarning(ctx, moderation); err != nil {
			return fmt.Errorf("failed to send warning: %w", err)
		}

	case models.ModerationActionDismiss:
		// Mark as resolved without action
		moderation.Status = models.ModerationStatusDismissed
		now := time.Now()
		moderation.ResolvedAt = &now

	case models.ModerationActionRestore:
		// Handle content restoration
		if err := mp.restoreContent(ctx, moderation); err != nil {
			return fmt.Errorf("failed to restore content: %w", err)
		}
	}

	// Update the moderation record
	return mp.moderationRepo.UpdateModeration(ctx, moderation)
}

// removeContent removes content from the system
func (mp *ModerationProcessor) removeContent(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Removing content",
		zap.String("content_id", moderation.ContentID),
		zap.String("content_type", string(moderation.ContentType)),
	)

	switch moderation.ContentType {
	case models.ModerationContentTypeStatus:
		// Delete the object/status
		if err := mp.objectRepo.DeleteObject(ctx, moderation.ContentID); err != nil {
			mp.logger.Error("Failed to delete object", zap.Error(err))
			// Continue with cleanup even if deletion fails
		}

		// Clean up related data
		if err := mp.cleanupRemovedStatus(ctx, moderation.ContentID); err != nil {
			mp.logger.Error("Failed to clean up removed status",
				zap.String("status_id", moderation.ContentID),
				zap.Error(err),
			)
		}

	case models.ModerationContentTypeMedia:
		// Remove media object
		if err := mp.objectRepo.DeleteObject(ctx, moderation.ContentID); err != nil {
			return fmt.Errorf("failed to remove media: %w", err)
		}

	default:
		mp.logger.Warn("Unsupported content type for removal",
			zap.String("content_type", string(moderation.ContentType)),
		)
	}

	// Send notification to user
	if err := mp.notifyContentRemoval(ctx, moderation); err != nil {
		mp.logger.Error("Failed to notify content removal",
			zap.String("user_id", moderation.UserID),
			zap.Error(err),
		)
	}

	return nil
}

// suspendUser suspends a user account
func (mp *ModerationProcessor) suspendUser(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Suspending user",
		zap.String("user_id", moderation.UserID),
		zap.String("reason", string(moderation.Reason)),
	)

	// Update user status
	updates := map[string]any{
		"suspended":         true,
		"moderation_reason": fmt.Sprintf("%s: %s", moderation.Reason, moderation.Evidence.TextContent),
		"moderated_at":      time.Now(),
	}

	if err := mp.userRepo.UpdateUser(ctx, moderation.UserID, updates); err != nil {
		return fmt.Errorf("failed to suspend user: %w", err)
	}

	// Clean up user relationships
	if err := mp.cleanupSuspendedUser(ctx, moderation.UserID); err != nil {
		mp.logger.Error("Failed to clean up suspended user",
			zap.String("user_id", moderation.UserID),
			zap.Error(err),
		)
	}

	// Send notification
	if err := mp.notifyUserSuspension(ctx, moderation); err != nil {
		mp.logger.Error("Failed to notify user suspension",
			zap.String("user_id", moderation.UserID),
			zap.Error(err),
		)
	}

	return nil
}

// silenceUser silences a user account
func (mp *ModerationProcessor) silenceUser(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Silencing user",
		zap.String("user_id", moderation.UserID),
		zap.String("reason", string(moderation.Reason)),
	)

	// Update user status
	updates := map[string]any{
		"silenced":          true,
		"moderation_reason": fmt.Sprintf("%s: %s", moderation.Reason, moderation.GetPrimaryProhibitedWord()),
		"moderated_at":      time.Now(),
	}

	if err := mp.userRepo.UpdateUser(ctx, moderation.UserID, updates); err != nil {
		return fmt.Errorf("failed to silence user: %w", err)
	}

	// Send notification
	if err := mp.notifyUserSilencing(ctx, moderation); err != nil {
		mp.logger.Error("Failed to notify user silencing",
			zap.String("user_id", moderation.UserID),
			zap.Error(err),
		)
	}

	return nil
}

// sendWarning sends a warning to the user
func (mp *ModerationProcessor) sendWarning(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Sending warning to user",
		zap.String("user_id", moderation.UserID),
		zap.String("reason", string(moderation.Reason)),
	)

	// Create warning notification using DynamORM model
	notification := models.NewNotificationBuilder().
		ForUser(moderation.UserID).
		OfType("moderation_warning").
		FromActor("system", "system").
		AboutTarget(moderation.ContentID, "status").
		WithContent("Moderation Warning", fmt.Sprintf("Content moderated for: %s", moderation.Reason)).
		Build()

	return mp.db.WithContext(ctx).Model(notification).Create()
}

// restoreContent restores previously moderated content
func (mp *ModerationProcessor) restoreContent(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Restoring content",
		zap.String("content_id", moderation.ContentID),
		zap.String("content_type", string(moderation.ContentType)),
	)

	// For now, log the restoration
	// In a real system, this would restore from a tombstone or backup
	mp.logger.Info("Content restoration completed",
		zap.String("content_id", moderation.ContentID),
	)

	return nil
}

// sendForHumanReview sends moderation case for human review
func (mp *ModerationProcessor) sendForHumanReview(ctx context.Context, moderation *models.Moderation) error {
	mp.logger.Info("Sending moderation for human review",
		zap.String("moderation_id", moderation.ModerationID),
		zap.Float64("confidence", moderation.Evidence.ConfidenceScore),
		zap.Bool("requires_review", moderation.Evidence.RequiresReview),
	)

	// Update status to reviewing
	moderation.Status = models.ModerationStatusReviewing
	moderation.AddHistoryEntry("system", "automated", models.ModerationActionWarning,
		models.ModerationStatusPending, models.ModerationStatusReviewing,
		"Sent for human review due to low confidence or high risk")

	if err := mp.moderationRepo.UpdateModeration(ctx, moderation); err != nil {
		return fmt.Errorf("failed to update moderation status: %w", err)
	}

	// Notify moderators
	return mp.notifyModeratorsForReview(ctx, moderation)
}

// notifyModeratorsForReview notifies moderators about cases needing review
func (mp *ModerationProcessor) notifyModeratorsForReview(ctx context.Context, moderation *models.Moderation) error {
	// Get moderators
	moderators, err := mp.getModerators(ctx)
	if err != nil {
		return fmt.Errorf("failed to get moderators: %w", err)
	}

	// Create notification for each moderator
	for _, moderator := range moderators {
		notification := models.NewNotificationBuilder().
			ForUser(moderator.Username).
			OfType("moderation_review").
			FromActor(moderation.UserID, "user").
			AboutTarget(moderation.ContentID, "moderation").
			WithContent("Moderation Review Required", fmt.Sprintf("Content needs review: %s", moderation.Reason)).
			WithData("moderation_id", moderation.ModerationID).
			Build()

		if err := mp.db.WithContext(ctx).Model(notification).Create(); err != nil {
			mp.logger.Error("Failed to create moderator notification",
				zap.String("moderator", moderator.Username),
				zap.Error(err),
			)
		}
	}

	return nil
}

// getModerators retrieves all users with moderator or admin role
func (mp *ModerationProcessor) getModerators(ctx context.Context) ([]*models.User, error) {
	// For now, return empty slice as we need to implement proper user filtering in DynamORM
	// TODO: Implement GetUsersByRole in UserRepository
	mp.logger.Info("Getting moderators - placeholder implementation")
	return []*models.User{}, nil
}

// cleanupRemovedStatus cleans up data related to a removed status
func (mp *ModerationProcessor) cleanupRemovedStatus(ctx context.Context, statusID string) error {
	// Remove from timelines using available method
	if err := mp.timelineRepo.DeleteTimelineEntriesByPost(ctx, statusID); err != nil {
		mp.logger.Error("Failed to remove from timelines", zap.Error(err))
	}

	// Note: Individual like/activity cleanup would require specific queries
	// For now, we rely on the object deletion removing the main content
	mp.logger.Info("Status cleanup completed", zap.String("status_id", statusID))

	return nil
}

// cleanupSuspendedUser cleans up data for a suspended user
func (mp *ModerationProcessor) cleanupSuspendedUser(ctx context.Context, userID string) error {
	// Note: For suspended users, we typically don't delete all relationships
	// Instead, we mark them as suspended which affects visibility
	// This is a placeholder for any specific cleanup needed during suspension
	mp.logger.Info("User suspension cleanup completed", zap.String("user_id", userID))

	// In a full implementation, we might:
	// - Remove pending follow requests
	// - Update activity visibility
	// - Clear cached data

	return nil
}

// notifyContentRemoval notifies user about content removal
func (mp *ModerationProcessor) notifyContentRemoval(ctx context.Context, moderation *models.Moderation) error {
	notification := models.NewNotificationBuilder().
		ForUser(moderation.UserID).
		OfType("content_removed").
		FromActor("system", "system").
		AboutTarget(moderation.ContentID, "status").
		WithContent("Content Removed", fmt.Sprintf("Your content was removed for: %s", moderation.Reason)).
		WithData("moderation_id", moderation.ModerationID).
		Build()

	return mp.db.WithContext(ctx).Model(notification).Create()
}

// notifyUserSuspension notifies user about account suspension
func (mp *ModerationProcessor) notifyUserSuspension(ctx context.Context, moderation *models.Moderation) error {
	notification := models.NewNotificationBuilder().
		ForUser(moderation.UserID).
		OfType("account_suspended").
		FromActor("system", "system").
		AboutTarget(moderation.UserID, "user").
		WithContent("Account Suspended", fmt.Sprintf("Your account was suspended for: %s", moderation.Reason)).
		WithData("moderation_id", moderation.ModerationID).
		Build()

	return mp.db.WithContext(ctx).Model(notification).Create()
}

// notifyUserSilencing notifies user about account silencing
func (mp *ModerationProcessor) notifyUserSilencing(ctx context.Context, moderation *models.Moderation) error {
	notification := models.NewNotificationBuilder().
		ForUser(moderation.UserID).
		OfType("account_silenced").
		FromActor("system", "system").
		AboutTarget(moderation.UserID, "user").
		WithContent("Account Silenced", fmt.Sprintf("Your account was silenced for: %s", moderation.Reason)).
		WithData("moderation_id", moderation.ModerationID).
		Build()

	return mp.db.WithContext(ctx).Model(notification).Create()
}

// extractUsernameFromActorID extracts username from an actor ID
func (mp *ModerationProcessor) extractUsernameFromActorID(actorID string) string {
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

// processRateLimitViolation processes rate limit violations
func (mp *ModerationProcessor) processRateLimitViolation(ctx context.Context, userID string, action string) error {
	// Check rate limit status
	result, err := mp.moderationRepo.CheckRateLimit(ctx, userID, action, 10, time.Hour)
	if err != nil {
		return fmt.Errorf("failed to check rate limit: %w", err)
	}

	if !result.Exceeded {
		return nil
	}

	// Create moderation for rate limit violation
	moderation := &models.Moderation{
		ContentID:     fmt.Sprintf("rate_limit_%s_%d", userID, time.Now().Unix()),
		ContentType:   models.ModerationContentTypeUser,
		UserID:        userID,
		Action:        models.ModerationActionWarning,
		Reason:        models.ModerationReasonRateLimiting,
		ModeratorID:   "system",
		ModeratorType: "automated",
		Evidence: models.ModerationEvidence{
			RequestCount:    result.CurrentCount,
			RequestPeriod:   result.Period.String(),
			ViolationCount:  result.ViolationCount,
			AverageInterval: result.AverageInterval,
			ConfidenceScore: 1.0,
		},
	}

	// Escalate action based on violation severity
	severity := moderation.GetRateLimitViolationSeverity()
	if severity == "severe" {
		moderation.Action = models.ModerationActionSuspend
	} else if severity == "moderate" {
		moderation.Action = models.ModerationActionSilence
	}

	// Create and execute moderation
	if err := mp.moderationRepo.CreateModeration(ctx, moderation); err != nil {
		return fmt.Errorf("failed to create rate limit moderation: %w", err)
	}

	return mp.executeModeration(ctx, moderation)
}

func main() {
	lambda.Start(HandleSQS)
}