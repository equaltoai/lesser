// Package quotes provides quote functionality and relationship management for status quotes.
package quotes

import (
	"context"
	stdErrors "errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// QuoteService provides quote posts functionality
type QuoteService struct {
	storage quoteStorage
	logger  *zap.Logger
}

type quoteStatusRepository interface {
	CreateStatus(ctx context.Context, status *models.Status) error
	GetStatus(ctx context.Context, statusID string) (*models.Status, error)
	UpdateStatus(ctx context.Context, status *models.Status) error
}

type quoteRepository interface {
	CreateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error
	GetQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error)
	UpdateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error
	GetQuotesForStatus(ctx context.Context, statusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error)
	WithdrawQuotes(ctx context.Context, noteID, userID string) (int, error)

	CreateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error
	GetQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error)
	UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error
}

type quoteRelationshipRepository interface {
	IsFollowing(ctx context.Context, followerID, followingID string) (bool, error)
}

type quoteStorage interface {
	Status() quoteStatusRepository
	Quote() quoteRepository
	Relationship() quoteRelationshipRepository
}

type quoteRepositoryStorageWrapper struct {
	storage core.RepositoryStorage
}

func (w quoteRepositoryStorageWrapper) Status() quoteStatusRepository { return w.storage.Status() }
func (w quoteRepositoryStorageWrapper) Quote() quoteRepository        { return w.storage.Quote() }
func (w quoteRepositoryStorageWrapper) Relationship() quoteRelationshipRepository {
	return w.storage.Relationship()
}

// NewQuoteService creates a new quote service
func NewQuoteService(storage core.RepositoryStorage, logger *zap.Logger) *QuoteService {
	return &QuoteService{
		storage: quoteRepositoryStorageWrapper{storage: storage},
		logger:  logger,
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
		qs.logger.Error("quote request validation failed", zap.Error(err))
		return nil, ErrInvalidQuoteRequest
	}

	// Get the target status
	targetStatus, err := qs.storage.Status().GetStatus(ctx, req.TargetStatusID)
	if err != nil {
		qs.logger.Error("failed to get target status", zap.String("status_id", req.TargetStatusID), zap.Error(err))
		return nil, ErrGetTargetStatus(err)
	}
	if targetStatus == nil {
		return nil, ErrTargetStatusNotFound
	}

	// Check if target is quotable
	if !qs.isStatusQuotable(targetStatus) {
		return nil, ErrTargetStatusNotQuotable
	}

	// Check quote permissions
	canQuote, err := qs.CheckQuotePermissions(ctx, req.QuoterUsername, targetStatus)
	if err != nil {
		qs.logger.Error("failed to check quote permissions",
			zap.String("quoter", req.QuoterUsername),
			zap.String("target_author", targetStatus.AuthorUsername),
			zap.Error(err))
		return nil, ErrCheckQuotePermissions(err)
	}
	if !canQuote {
		return nil, ErrNotAuthorizedToQuote
	}

	// Create the quote status
	quoteStatus, err := qs.createQuoteStatus(ctx, req, targetStatus)
	if err != nil {
		qs.logger.Error("failed to create quote status",
			zap.String("quoter", req.QuoterUsername),
			zap.String("target_status_id", req.TargetStatusID),
			zap.Error(err))
		return nil, ErrCreateQuoteStatus
	}

	// Create the quote relationship
	quoteRel, err := qs.createQuoteRelationship(ctx, quoteStatus, targetStatus)
	if err != nil {
		// If relationship creation fails, we should clean up the status
		qs.logger.Error("failed to create quote relationship, status may be orphaned",
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.Error(err))
		return nil, ErrCreateQuoteRelationship
	}

	qs.setQuoteReference(ctx, quoteStatus, targetStatus)

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

// AttachQuoteToStatus links an existing status to a target status as a quote
func (qs *QuoteService) AttachQuoteToStatus(ctx context.Context, quoteStatus *models.Status, targetStatusID string) (*QuotePostResult, error) {
	if quoteStatus == nil || quoteStatus.StatusID == "" || quoteStatus.AuthorUsername == "" {
		qs.logger.Error("invalid quote status provided for attachment")
		return nil, ErrInvalidQuoteRequest
	}

	if err := common.ValidateRequiredParam("target_status_id", targetStatusID); err != nil {
		return nil, err
	}

	targetStatus, err := qs.storage.Status().GetStatus(ctx, targetStatusID)
	if err != nil {
		qs.logger.Error("failed to get target status for quote attachment",
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return nil, ErrGetTargetStatus(err)
	}
	if targetStatus == nil {
		return nil, ErrTargetStatusNotFound
	}

	if !qs.isStatusQuotable(targetStatus) {
		return nil, ErrTargetStatusNotQuotable
	}

	canQuote, err := qs.CheckQuotePermissions(ctx, quoteStatus.AuthorUsername, targetStatus)
	if err != nil {
		return nil, ErrCheckQuotePermissions(err)
	}
	if !canQuote {
		return nil, ErrNotAuthorizedToQuote
	}

	quoteRel, err := qs.createQuoteRelationship(ctx, quoteStatus, targetStatus)
	if err != nil {
		qs.logger.Error("failed to create quote relationship for existing status",
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return nil, ErrCreateQuoteRelationship
	}

	if err := qs.updateQuoteCounts(ctx, targetStatus.StatusID, 1); err != nil {
		qs.logger.Warn("failed to update quote count for attachment",
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
	}

	if err := qs.createQuoteNotification(ctx, quoteStatus, targetStatus); err != nil {
		qs.logger.Warn("failed to create quote notification for attachment",
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
	}

	qs.setQuoteReference(ctx, quoteStatus, targetStatus)

	return &QuotePostResult{
		QuoteStatus:       quoteStatus,
		QuoteRelationship: quoteRel,
		TargetStatus:      targetStatus,
	}, nil
}

func (qs *QuoteService) setQuoteReference(ctx context.Context, quoteStatus, targetStatus *models.Status) {
	if quoteStatus == nil || targetStatus == nil {
		return
	}

	targetAuthorID := targetStatus.AuthorID
	if targetAuthorID == "" {
		targetAuthorID = targetStatus.AuthorUsername
	}

	if quoteStatus.QuoteTargetStatusID == targetStatus.StatusID &&
		quoteStatus.QuoteTargetAuthorID == targetAuthorID {
		return
	}

	quoteStatus.QuoteTargetStatusID = targetStatus.StatusID
	quoteStatus.QuoteTargetAuthorID = targetAuthorID

	if err := qs.storage.Status().UpdateStatus(ctx, quoteStatus); err != nil {
		qs.logger.Warn("failed to persist quote reference",
			zap.String("quote_status_id", quoteStatus.StatusID),
			zap.String("target_status_id", targetStatus.StatusID),
			zap.Error(err))
	}
}

// GetQuotesForStatus retrieves quote posts for a given status
func (qs *QuoteService) GetQuotesForStatus(ctx context.Context, statusID string, limit, offset int) ([]*models.Status, error) {
	// Get quote relationships for the status
	relationships, err := qs.getQuoteRelationships(ctx, statusID, limit, offset)
	if err != nil {
		qs.logger.Error("failed to get quote relationships", zap.String("status_id", statusID), zap.Error(err))
		return nil, ErrGetQuoteRelationships(err)
	}

	// Get the quote statuses
	var quoteStatuses []*models.Status
	for _, rel := range relationships {
		if !rel.IsActive() {
			continue
		}

		status, err := qs.storage.Status().GetStatus(ctx, rel.QuoterNoteID)
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
		qs.logger.Error("failed to get quote relationship",
			zap.String("quote_status_id", quoteStatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return ErrGetQuoteRelationship(err)
	}
	if rel == nil {
		return ErrQuoteRelationshipNotFound
	}

	// Verify ownership
	if rel.QuoterID != username {
		return ErrNotAuthorizedToDeleteQuote
	}

	// Mark relationship as withdrawn
	rel.Withdraw()
	if err := qs.saveQuoteRelationship(ctx, rel); err != nil {
		qs.logger.Error("failed to withdraw quote relationship",
			zap.String("quote_status_id", quoteStatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return ErrWithdrawQuoteRelationship
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
		qs.logger.Error("failed to get quote permissions", zap.String("username", username), zap.Error(err))
		return nil, ErrGetQuotePermissions(err)
	}

	// If no permissions exist, return defaults
	if permissions == nil {
		qs.logger.Warn("quote permissions missing; applying defaults",
			zap.String("username", username))
		permissions = &models.QuotePermissions{
			Username: username,
		}
		permissions.SetDefaults()
	}

	return permissions, nil
}

// UpdateQuotePermissions updates quote permissions for a user
func (qs *QuoteService) UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	if err := permissions.UpdateKeys(); err != nil {
		qs.logger.Error("failed to update quote permissions keys", zap.Error(err))
		return err
	}

	// Try to get existing permissions first
	existing, err := qs.storage.Quote().GetQuotePermissions(ctx, permissions.Username)
	if err != nil {
		// If error is something other than "not found", return it
		if !stdErrors.Is(err, storage.ErrNotFound) && !apperrors.HasCode(err, apperrors.CodeNotFound) {
			return err
		}
		existing = nil
	}

	if existing == nil {
		// Create new permissions
		err = qs.storage.Quote().CreateQuotePermissions(ctx, permissions)
	} else {
		// Update existing permissions
		err = qs.storage.Quote().UpdateQuotePermissions(ctx, permissions)
	}

	if err != nil {
		qs.logger.Error("failed to save quote permissions", zap.String("username", permissions.Username), zap.Error(err))
		return ErrSaveQuotePermissions
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
	if req.Content != "" {
		if err := common.ValidateStringLength("content", req.Content, 0, 500); err != nil {
			return ErrQuoteContentTooLong
		}
	}
	return nil
}

func (qs *QuoteService) isStatusQuotable(status *models.Status) bool {
	// Check if status allows quotes
	// This would depend on the status visibility and user preferences
	return common.IsPubliclyVisible(status.Visibility)
}

// CheckQuotePermissions applies the account-level quote predicate to relationship-minting paths:
// GraphQL quote creation and REST reblog-with-comment. GraphQL createQuoteNote only embeds a URL
// without minting a relationship and is outside this control by design; a blocked user can always
// paste a URL into ordinary post text, while this predicate governs quote relationships.
func (qs *QuoteService) CheckQuotePermissions(ctx context.Context, quoterUsername string, targetStatus *models.Status) (bool, error) {
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
			qs.logger.Warn("follower quote permission lookup failed closed",
				zap.String("quoter", quoterUsername),
				zap.String("target_author", targetStatus.AuthorUsername),
				zap.Error(err))
			return false, nil
		}
		if isFollowing {
			return true, nil
		}
	}

	// Check if quoter is mentioned in the original status
	if permissions.AllowMentioned {
		isMentioned, err := qs.checkMentioned(ctx, targetStatus, quoterUsername)
		if err != nil {
			qs.logger.Warn("mentioned quote permission lookup failed closed",
				zap.String("quoter", quoterUsername),
				zap.String("target_status_id", targetStatus.StatusID),
				zap.Error(err))
			return false, nil
		}
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
	err := qs.storage.Status().CreateStatus(ctx, quoteStatus)
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
	if err := rel.UpdateKeys(); err != nil {
		qs.logger.Error("failed to update quote relationship keys", zap.Error(err))
		return nil, err
	}

	// Save the relationship
	err := qs.storage.Quote().CreateQuoteRelationship(ctx, rel)
	if err != nil {
		return nil, err
	}

	return rel, nil
}

func (qs *QuoteService) getQuoteRelationships(ctx context.Context, statusID string, limit, _ int) ([]*models.QuoteRelationship, error) {
	opts := interfaces.PaginationOptions{
		Limit: limit,
	}

	result, err := qs.storage.Quote().GetQuotesForStatus(ctx, statusID, opts)
	if err != nil {
		return nil, err
	}

	return result.Items, nil
}

func (qs *QuoteService) getQuoteRelationshipByIDs(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	return qs.storage.Quote().GetQuoteRelationship(ctx, quoteStatusID, targetStatusID)
}

func (qs *QuoteService) saveQuoteRelationship(ctx context.Context, rel *models.QuoteRelationship) error {
	return qs.storage.Quote().UpdateQuoteRelationship(ctx, rel)
}

func (qs *QuoteService) getQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error) {
	permissions, err := qs.storage.Quote().GetQuotePermissions(ctx, username)
	if err != nil {
		if stdErrors.Is(err, storage.ErrNotFound) || apperrors.HasCode(err, apperrors.CodeNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return permissions, nil
}

func (qs *QuoteService) updateQuoteCounts(_ context.Context, statusID string, delta int) error {
	// Placeholder implementation
	// In reality, this would update the quote count on the target status
	qs.logger.Info("updating quote count",
		zap.String("status_id", statusID),
		zap.Int("delta", delta))
	return nil
}

// WithdrawFromQuotes withdraws all quotes of a note by a user
func (qs *QuoteService) WithdrawFromQuotes(ctx context.Context, noteID, userID string) (*models.Status, int, error) {
	// Validate inputs
	if err := common.ValidateRequiredParam("note_id", noteID); err != nil {
		return nil, 0, err
	}
	if err := common.ValidateRequiredParam("user_id", userID); err != nil {
		return nil, 0, err
	}

	// Get the original note to return in the payload
	note, err := qs.storage.Status().GetStatus(ctx, noteID)
	if err != nil {
		qs.logger.Error("failed to get note for withdrawal",
			zap.String("note_id", noteID),
			zap.Error(err))
		return nil, 0, ErrGetTargetStatus(err)
	}
	if note == nil {
		return nil, 0, ErrTargetStatusNotFound
	}

	// Withdraw all quotes
	count, err := qs.storage.Quote().WithdrawQuotes(ctx, noteID, userID)
	if err != nil {
		qs.logger.Error("failed to withdraw quotes",
			zap.String("note_id", noteID),
			zap.String("user_id", userID),
			zap.Error(err))
		return nil, 0, fmt.Errorf("failed to withdraw quotes: %w", err)
	}

	qs.logger.Info("withdrew quotes from note",
		zap.String("note_id", noteID),
		zap.String("user_id", userID),
		zap.Int("withdrawn_count", count))

	return note, count, nil
}

func (qs *QuoteService) createQuoteNotification(_ context.Context, quoteStatus, targetStatus *models.Status) error {
	// Placeholder implementation
	// In reality, this would create a notification for the original author
	qs.logger.Info("creating quote notification",
		zap.String("quoter", quoteStatus.AuthorUsername),
		zap.String("target_author", targetStatus.AuthorUsername))
	return nil
}

func (qs *QuoteService) checkFollowRelationship(ctx context.Context, quoterUsername, targetAuthorUsername string) (bool, error) {
	if qs.storage == nil || qs.storage.Relationship() == nil {
		return false, fmt.Errorf("relationship repository unavailable")
	}
	return qs.storage.Relationship().IsFollowing(ctx, quoterUsername, targetAuthorUsername)
}

func (qs *QuoteService) checkMentioned(ctx context.Context, targetStatus *models.Status, quoterUsername string) (bool, error) {
	if targetStatus == nil || strings.TrimSpace(targetStatus.StatusID) == "" {
		return false, nil
	}

	// Re-read the canonical persisted status by its exact primary key. Quote creation can receive
	// a caller-resolved Status value, but the permission decision must use the stored mention
	// projection rather than request content or an ad-hoc content re-parse.
	persisted, err := qs.storage.Status().GetStatus(ctx, targetStatus.StatusID)
	if err != nil {
		return false, err
	}
	if persisted == nil {
		return false, nil
	}

	quoter := strings.TrimSpace(strings.TrimPrefix(quoterUsername, "@"))
	if quoter == "" {
		return false, nil
	}
	localDomain := common.ExtractDomainFromActorID(persisted.AuthorID)
	for _, rawMention := range persisted.Mentions {
		if storedMentionMatchesLocalUsername(rawMention, quoter, localDomain) {
			return true, nil
		}
	}
	return false, nil
}

func storedMentionMatchesLocalUsername(rawMention, username, localDomain string) bool {
	mention := strings.TrimSpace(rawMention)
	if mention == "" {
		return false
	}
	if !strings.Contains(mention, "://") {
		return strings.EqualFold(strings.TrimPrefix(mention, "@"), username)
	}
	if localDomain == "" || !common.IsLocalActorID(mention, localDomain) {
		return false
	}

	parsed, err := url.Parse(mention)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 2 && strings.EqualFold(parts[0], "users") {
		return strings.EqualFold(strings.TrimPrefix(parts[1], "@"), username)
	}
	if len(parts) == 1 && strings.HasPrefix(parts[0], "@") {
		return strings.EqualFold(strings.TrimPrefix(parts[0], "@"), username)
	}
	return false
}

func generateStatusID() string {
	// Generate a unique status ID
	return fmt.Sprintf("quote_%d", time.Now().UnixNano())
}
