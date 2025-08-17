// Package quotes provides quote functionality and relationship management for status quotes.
package quotes

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// QuoteService provides quote posts functionality
type QuoteService struct {
	repos  interfaces.RepositoryRegistry
	logger *zap.Logger
}

// NewQuoteService creates a new quote service
func NewQuoteService(repos interfaces.RepositoryRegistry, logger *zap.Logger) *QuoteService {
	return &QuoteService{
		repos:  repos,
		logger: logger,
	}
}

// CreateQuoteRequest represents a request to create a quote post
type CreateQuoteRequest struct {
	QuoterUsername string
	TargetStatusID string
	Content        string
	Visibility     string
	SpoilerText    string
	Sensitive      bool
	Language       string
}

// QuotePostResult represents the result of creating a quote post
type QuotePostResult struct {
	QuoteStatus       *models.Status
	QuoteRelationship *models.QuoteRelationship
	TargetStatus      *models.Status
}

// CreateQuotePost creates a new quote post
func (qs *QuoteService) CreateQuotePost(ctx context.Context, req *CreateQuoteRequest) (*QuotePostResult, error) {
	// Validate input
	if err := qs.validateCreateQuoteRequest(req); err != nil {
		return nil, fmt.Errorf("invalid quote request: %w", err)
	}

	// Get the target status
	targetStatus, err := qs.repos.Status().GetStatus(ctx, req.TargetStatusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target status: %w", err)
	}
	if targetStatus == nil {
		return nil, fmt.Errorf("target status not found")
	}

	// Check if target is quotable
	if !qs.isStatusQuotable(targetStatus) {
		return nil, fmt.Errorf("target status is not quotable")
	}

	// Check quote permissions
	canQuote, err := qs.checkQuotePermissions(ctx, req.QuoterUsername, targetStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to check quote permissions: %w", err)
	}
	if !canQuote {
		return nil, fmt.Errorf("not authorized to quote this status")
	}

	// Create the quote status
	quoteStatus, err := qs.createQuoteStatus(ctx, req, targetStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to create quote status: %w", err)
	}

	// Create the quote relationship
	quoteRel, err := qs.createQuoteRelationship(ctx, quoteStatus, targetStatus)
	if err != nil {
		// If relationship creation fails, we should clean up the status
		qs.logger.Error("failed to create quote relationship, status may be orphaned",
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create quote relationship: %w", err)
	}

	// Update quote counts
	if err := qs.updateQuoteCounts(ctx, targetStatus.StatusID, 1); err != nil {
		qs.logger.Warn("failed to update quote count", zap.Error(err))
		// Don't fail the operation for this
	}

	// Create notification for original author
	if err := qs.createQuoteNotification(ctx, quoteStatus, targetStatus); err != nil {
		qs.logger.Warn("failed to create quote notification", zap.Error(err))
		// Don't fail the operation for this
	}

	return &QuotePostResult{
		QuoteStatus:       quoteStatus,
		QuoteRelationship: quoteRel,
		TargetStatus:      targetStatus,
	}, nil
}

// GetQuotesForStatus retrieves quote posts for a given status
func (qs *QuoteService) GetQuotesForStatus(ctx context.Context, statusID string, limit, offset int) ([]*models.Status, error) {
	// Get quote relationships for the status
	relationships, err := qs.getQuoteRelationships(ctx, statusID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get quote relationships: %w", err)
	}

	// Get the quote statuses
	var quoteStatuses []*models.Status
	for _, rel := range relationships {
		if !rel.IsActive() {
			continue
		}

		status, err := qs.repos.Status().GetStatus(ctx, rel.QuoterNoteID)
		if err != nil {
			qs.logger.Warn("failed to get quote status",
				zap.String("status_id", rel.QuoterNoteID),
				zap.Error(err))
			continue
		}

		if status != nil {
			quoteStatuses = append(quoteStatuses, status)
		}
	}

	return quoteStatuses, nil
}

// DeleteQuotePost removes a quote post and its relationship
func (qs *QuoteService) DeleteQuotePost(ctx context.Context, quoteStatusID, targetStatusID, username string) error {
	// Get the quote relationship
	rel, err := qs.getQuoteRelationshipByIDs(ctx, quoteStatusID, targetStatusID)
	if err != nil {
		return fmt.Errorf("failed to get quote relationship: %w", err)
	}
	if rel == nil {
		return fmt.Errorf("quote relationship not found")
	}

	// Verify ownership
	if rel.QuoterID != username {
		return fmt.Errorf("not authorized to delete this quote")
	}

	// Mark relationship as withdrawn
	rel.Withdraw()
	if err := qs.saveQuoteRelationship(ctx, rel); err != nil {
		return fmt.Errorf("failed to withdraw quote relationship: %w", err)
	}

	// Update quote counts
	if err := qs.updateQuoteCounts(ctx, targetStatusID, -1); err != nil {
		qs.logger.Warn("failed to update quote count", zap.Error(err))
	}

	return nil
}

// GetQuotePermissions retrieves quote permissions for a user
func (qs *QuoteService) GetQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error) {
	permissions, err := qs.getQuotePermissions(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get quote permissions: %w", err)
	}

	// If no permissions exist, return defaults
	if permissions == nil {
		permissions = &models.QuotePermissions{
			Username: username,
		}
		permissions.SetDefaults()
	}

	return permissions, nil
}

// UpdateQuotePermissions updates quote permissions for a user
func (qs *QuoteService) UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	permissions.UpdateKeys()
	
	// Try to get existing permissions first
	existing, err := qs.repos.Quote().GetQuotePermissions(ctx, permissions.Username)
	if err != nil {
		return fmt.Errorf("failed to check existing permissions: %w", err)
	}
	
	if existing == nil {
		// Create new permissions
		err = qs.repos.Quote().CreateQuotePermissions(ctx, permissions)
	} else {
		// Update existing permissions
		err = qs.repos.Quote().UpdateQuotePermissions(ctx, permissions)
	}
	
	if err != nil {
		return fmt.Errorf("failed to save quote permissions: %w", err)
	}
	
	qs.logger.Info("quote permissions updated",
		zap.String("username", permissions.Username),
		zap.Bool("allow_public", permissions.AllowPublic))
	
	return nil
}

// Helper methods

func (qs *QuoteService) validateCreateQuoteRequest(req *CreateQuoteRequest) error {
	if err := common.ValidateRequiredParam("req.QuoterUsername", req.QuoterUsername); err != nil {
		return common.ValidateRequiredParam("quoter_username", req.QuoterUsername)
	}
	if err := common.ValidateRequiredParam("req.TargetStatusID", req.TargetStatusID); err != nil {
		return common.ValidateRequiredParam("target_status_id", req.TargetStatusID)
	}
	if len(req.Content) > 500 {
		return fmt.Errorf("quote content too long")
	}
	return nil
}

func (qs *QuoteService) isStatusQuotable(status *models.Status) bool {
	// Check if status allows quotes
	// This would depend on the status visibility and user preferences
	return status.Visibility == "public" || status.Visibility == "unlisted"
}

func (qs *QuoteService) checkQuotePermissions(ctx context.Context, quoterUsername string, targetStatus *models.Status) (bool, error) {
	// Get quote permissions for the target status author
	permissions, err := qs.GetQuotePermissions(ctx, targetStatus.AuthorUsername)
	if err != nil {
		return false, err
	}

	// Check if quoter is in block list
	for _, blocked := range permissions.BlockList {
		if blocked == quoterUsername {
			return false, nil
		}
	}

	// Check permission levels
	if permissions.AllowPublic {
		return true, nil
	}

	// Check if quoter follows the target author
	if permissions.AllowFollowers {
		isFollowing, err := qs.checkFollowRelationship(ctx, quoterUsername, targetStatus.AuthorUsername)
		if err != nil {
			return false, err
		}
		if isFollowing {
			return true, nil
		}
	}

	// Check if quoter is mentioned in the original status
	if permissions.AllowMentioned {
		isMentioned := qs.checkMentioned(targetStatus, quoterUsername)
		if isMentioned {
			return true, nil
		}
	}

	return false, nil
}

func (qs *QuoteService) createQuoteStatus(ctx context.Context, req *CreateQuoteRequest, _ *models.Status) (*models.Status, error) {
	now := time.Now()
	
	// Create new status for the quote
	quoteStatus := &models.Status{
		StatusID:       generateStatusID(),
		AuthorUsername: req.QuoterUsername,
		Content:        req.Content,
		Visibility:     req.Visibility,
		Sensitive:      req.Sensitive,
		Language:       req.Language,
		CreatedAt:      now,
		ModifiedAt:     now,
		PublishedAt:    now,
		
		// Quote-specific fields would be stored in the Note field
		// For now, we'll use a placeholder approach
	}

	// Save the status
	err := qs.repos.Status().CreateStatus(ctx, quoteStatus)
	if err != nil {
		return nil, err
	}

	return quoteStatus, nil
}

func (qs *QuoteService) createQuoteRelationship(ctx context.Context, quoteStatus, targetStatus *models.Status) (*models.QuoteRelationship, error) {
	rel := &models.QuoteRelationship{
		QuoterNoteID:   quoteStatus.StatusID,
		TargetNoteID:   targetStatus.StatusID,
		QuoterID:       quoteStatus.AuthorUsername,
		TargetAuthorID: targetStatus.AuthorUsername,
		Timestamp:      time.Now(),
		Withdrawn:      false,
	}

	rel.GenerateID()
	rel.UpdateKeys()

	// Save the relationship
	err := qs.repos.Quote().CreateQuoteRelationship(ctx, rel)
	if err != nil {
		return nil, err
	}

	return rel, nil
}

func (qs *QuoteService) getQuoteRelationships(ctx context.Context, statusID string, limit, _ int) ([]*models.QuoteRelationship, error) {
	opts := interfaces.PaginationOptions{
		Limit: limit,
	}
	
	result, err := qs.repos.Quote().GetQuotesForStatus(ctx, statusID, opts)
	if err != nil {
		return nil, err
	}
	
	return result.Items, nil
}

func (qs *QuoteService) getQuoteRelationshipByIDs(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	return qs.repos.Quote().GetQuoteRelationship(ctx, quoteStatusID, targetStatusID)
}

func (qs *QuoteService) saveQuoteRelationship(ctx context.Context, rel *models.QuoteRelationship) error {
	return qs.repos.Quote().UpdateQuoteRelationship(ctx, rel)
}

func (qs *QuoteService) getQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error) {
	return qs.repos.Quote().GetQuotePermissions(ctx, username)
}

func (qs *QuoteService) updateQuoteCounts(_ context.Context, statusID string, delta int) error {
	// Placeholder implementation
	// In reality, this would update the quote count on the target status
	qs.logger.Info("updating quote count",
		zap.String("status_id", statusID),
		zap.Int("delta", delta))
	return nil
}

func (qs *QuoteService) createQuoteNotification(_ context.Context, quoteStatus, targetStatus *models.Status) error {
	// Placeholder implementation
	// In reality, this would create a notification for the original author
	qs.logger.Info("creating quote notification",
		zap.String("quoter", quoteStatus.AuthorUsername),
		zap.String("target_author", targetStatus.AuthorUsername))
	return nil
}

func (qs *QuoteService) checkFollowRelationship(_ context.Context, _ string, _ string) (bool, error) {
	// Placeholder implementation
	// In reality, this would check if follower follows followee
	return false, nil
}

func (qs *QuoteService) checkMentioned(_ *models.Status, _ string) bool {
	// Simple check if username is mentioned in the status content
	return false // Placeholder
}

func generateStatusID() string {
	// Generate a unique status ID
	return fmt.Sprintf("quote_%d", time.Now().UnixNano())
}