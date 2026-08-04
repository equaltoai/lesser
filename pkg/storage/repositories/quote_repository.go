package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// QuoteRepository implements quote operations using enhanced DynamORM patterns
type QuoteRepository struct {
	relationshipRepo *EnhancedBaseRepository[*models.QuoteRelationship]
	permissionsRepo  *EnhancedBaseRepository[*models.QuotePermissions]
	logger           *zap.Logger
}

// NewQuoteRepository creates a new quote repository with enhanced functionality
func NewQuoteRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *QuoteRepository {
	// Create enhanced repositories for quote operations
	relationshipRepo := NewEnhancedBaseRepository[*models.QuoteRelationship](db, tableName, logger, costService, "QuoteRepository.Relationship", "quote_relationship")
	relationshipRepo.SetValidationService(NewDefaultValidationService())
	relationshipRepo.SetPermissionService(NewDefaultPermissionService()) // Quote permissions
	relationshipRepo.SetCachingService(NewInMemoryCachingService())      // Cache quote relationships
	relationshipRepo.SetEventService(NewDefaultEventService())           // Quote events

	permissionsRepo := NewEnhancedBaseRepository[*models.QuotePermissions](db, tableName, logger, costService, "QuoteRepository.Permissions", "quote_permissions")
	permissionsRepo.SetValidationService(NewDefaultValidationService())
	permissionsRepo.SetPermissionService(NewDefaultPermissionService()) // Permission validation
	permissionsRepo.SetCachingService(NewInMemoryCachingService())      // Cache permissions
	permissionsRepo.SetEventService(NewDefaultEventService())

	return &QuoteRepository{
		relationshipRepo: relationshipRepo,
		permissionsRepo:  permissionsRepo,
		logger:           logger,
	}
}

// CreateQuoteRelationship creates a new quote relationship
func (r *QuoteRepository) CreateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error {
	err := r.relationshipRepo.ValidateAndCreate(ctx, relationship)
	if err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Debug("quote relationship already exists",
				zap.String("quoter_note_id", relationship.QuoterNoteID),
				zap.String("target_note_id", relationship.TargetNoteID))
			return nil
		}
		r.logger.Error("failed to create quote relationship",
			zap.String("quoter_note_id", relationship.QuoterNoteID),
			zap.String("target_note_id", relationship.TargetNoteID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQuoteRelationshipCreateFailed, err)
	}

	r.logger.Info("created quote relationship",
		zap.String("quoter_note_id", relationship.QuoterNoteID),
		zap.String("target_note_id", relationship.TargetNoteID))

	return nil
}

// GetQuoteRelationship retrieves a quote relationship by quoter and target note IDs
func (r *QuoteRepository) GetQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	relationship := &models.QuoteRelationship{}
	pk := fmt.Sprintf("QUOTE#%s", quoteStatusID)
	sk := fmt.Sprintf("QUOTED#%s", targetStatusID)

	err := r.relationshipRepo.Get(ctx, pk, sk, relationship)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityQuoteRelationship, fmt.Sprintf("%s->%s", quoteStatusID, targetStatusID))
		}
		r.logger.Error("failed to get quote relationship",
			zap.String("quote_status_id", quoteStatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityQuoteRelationship, fmt.Sprintf("%s->%s", quoteStatusID, targetStatusID))
	}

	return relationship, nil
}

// UpdateQuoteRelationship updates an existing quote relationship
func (r *QuoteRepository) UpdateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error {
	err := r.relationshipRepo.Update(ctx, relationship)
	if err != nil {
		r.logger.Error("failed to update quote relationship",
			zap.String("quoter_note_id", relationship.QuoterNoteID),
			zap.String("target_note_id", relationship.TargetNoteID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQuoteRelationshipUpdateFailed, err)
	}

	r.logger.Info("updated quote relationship",
		zap.String("quoter_note_id", relationship.QuoterNoteID),
		zap.String("target_note_id", relationship.TargetNoteID))

	return nil
}

// DeleteQuoteRelationship deletes a quote relationship
func (r *QuoteRepository) DeleteQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) error {
	pk := fmt.Sprintf("QUOTE#%s", quoteStatusID)
	sk := fmt.Sprintf("QUOTED#%s", targetStatusID)

	err := r.relationshipRepo.Delete(ctx, pk, sk)
	if err != nil {
		r.logger.Error("failed to delete quote relationship",
			zap.String("quote_status_id", quoteStatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQuoteRelationshipDeleteFailed, err)
	}

	r.logger.Info("deleted quote relationship",
		zap.String("quote_status_id", quoteStatusID),
		zap.String("target_status_id", targetStatusID))

	return nil
}

// getQuotesByGSI is a generic helper for querying quotes by GSI
func (r *QuoteRepository) getQuotesByGSI(ctx context.Context, gsiKey, gsiValue string, opts interfaces.PaginationOptions, _ string) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	db := r.relationshipRepo.GetDB()
	query := db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where(gsiKey, "=", gsiValue).
		Limit(opts.Limit)

	if opts.Cursor != "" {
		query = query.Cursor(opts.Cursor)
	}

	var quotesData []models.QuoteRelationship
	err := query.All(&quotesData)
	if err != nil {
		r.logger.Error("failed to query quote relationships",
			zap.String("gsi_value", gsiValue),
			zap.Error(err))
		return nil, fmt.Errorf("%w: %w", ErrQuoteRelationshipQueryFailed, err)
	}

	// Convert to pointer slice and filter out withdrawn quotes
	activeQuotes := make([]*models.QuoteRelationship, 0, len(quotesData))
	for i := range quotesData {
		if quotesData[i].IsActive() {
			activeQuotes = append(activeQuotes, &quotesData[i])
		}
	}

	result := &interfaces.PaginatedResult[*models.QuoteRelationship]{
		Items:   activeQuotes,
		HasMore: len(quotesData) == opts.Limit,
		Total:   -1, // Total count not calculated for performance
	}

	if len(quotesData) > 0 {
		lastQuote := &quotesData[len(quotesData)-1]
		result.NextCursor = fmt.Sprintf("%s#%s", lastQuote.PK, lastQuote.SK)
	}

	return result, nil
}

// GetQuotesForStatus retrieves quotes for a given status using GSI1
func (r *QuoteRepository) GetQuotesForStatus(ctx context.Context, statusID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	return r.getQuotesByGSI(ctx, "gsi1PK", fmt.Sprintf("QUOTED#%s", statusID), opts, "get quotes for status")
}

// GetQuotesByUser retrieves quotes created by a specific user
func (r *QuoteRepository) GetQuotesByUser(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	return r.getQuotesByGSI(ctx, "gsi2PK", fmt.Sprintf("QUOTER#%s", userID), opts, "get quotes by user")
}

// CreateQuotePermissions creates new quote permissions for a user
func (r *QuoteRepository) CreateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	err := r.permissionsRepo.ValidateAndCreate(ctx, permissions)
	if err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Debug("quote permissions already exist",
				zap.String("username", permissions.Username))
			return nil
		}
		r.logger.Error("failed to create quote permissions",
			zap.String("username", permissions.Username),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQuotePermissionsCreateFailed, err)
	}

	r.logger.Info("created quote permissions",
		zap.String("username", permissions.Username))

	return nil
}

// GetQuotePermissions retrieves quote permissions for a user
func (r *QuoteRepository) GetQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error) {
	permissions := &models.QuotePermissions{}
	pk := fmt.Sprintf("USER#%s", username)
	sk := "QUOTE_PERMISSIONS"

	err := r.permissionsRepo.Get(ctx, pk, sk, permissions)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityQuotePermissions, username)
		}
		r.logger.Error("failed to get quote permissions",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityQuotePermissions, username)
	}

	return permissions, nil
}

// GetQuotePermissionsBatch returns account quote defaults with one batch read.
// Missing rows use the same public defaults as QuoteService.GetQuotePermissions.
func (r *QuoteRepository) GetQuotePermissionsBatch(ctx context.Context, usernames []string) (map[string]*models.QuotePermissions, error) {
	permissionsByUser := make(map[string]*models.QuotePermissions, len(usernames))
	keys := make([]struct{ PK, SK string }, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, rawUsername := range usernames {
		username := strings.TrimSpace(rawUsername)
		if username == "" {
			continue
		}
		defaults := &models.QuotePermissions{Username: username}
		defaults.SetDefaults()
		permissionsByUser[username] = defaults
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		keys = append(keys, struct{ PK, SK string }{PK: "USER#" + username, SK: "QUOTE_PERMISSIONS"})
	}
	if len(keys) == 0 {
		return permissionsByUser, nil
	}

	permissions, err := r.permissionsRepo.BatchGet(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrQuotePermissionsGetFailed, err)
	}
	for _, permission := range permissions {
		if permission == nil || strings.TrimSpace(permission.Username) == "" {
			continue
		}
		permissionsByUser[permission.Username] = permission
	}
	return permissionsByUser, nil
}

// UpdateQuotePermissions updates existing quote permissions
func (r *QuoteRepository) UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	err := r.permissionsRepo.Update(ctx, permissions)
	if err != nil {
		r.logger.Error("failed to update quote permissions",
			zap.String("username", permissions.Username),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQuotePermissionsUpdateFailed, err)
	}

	r.logger.Info("updated quote permissions",
		zap.String("username", permissions.Username))

	return nil
}

// DeleteQuotePermissions deletes quote permissions for a user
func (r *QuoteRepository) DeleteQuotePermissions(ctx context.Context, username string) error {
	pk := fmt.Sprintf("USER#%s", username)
	sk := "QUOTE_PERMISSIONS"

	err := r.permissionsRepo.Delete(ctx, pk, sk)
	if err != nil {
		r.logger.Error("failed to delete quote permissions",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("%w: %w", ErrQuotePermissionsDeleteFailed, err)
	}

	r.logger.Info("deleted quote permissions",
		zap.String("username", username))

	return nil
}

// GetQuoteCount gets the total number of quotes for a status
func (r *QuoteRepository) GetQuoteCount(ctx context.Context, statusID string) (int64, error) {
	db := r.relationshipRepo.GetDB()
	count, err := db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where("gsi1PK", "=", fmt.Sprintf("QUOTED#%s", statusID)).
		Where("Withdrawn", "=", false).
		Count()

	if err != nil {
		r.logger.Error("failed to get quote count",
			zap.String("status_id", statusID),
			zap.Error(err))
		return 0, fmt.Errorf("%w: %w", ErrQuoteCountQueryFailed, err)
	}

	return count, nil
}

// IncrementQuoteCount increments the quote count for a status
// Note: This is typically handled by updating the Status model directly
func (r *QuoteRepository) IncrementQuoteCount(_ context.Context, statusID string) error {
	// This would typically update a counter in the Status model
	// For now, we'll log the operation
	r.logger.Info("incrementing quote count",
		zap.String("status_id", statusID))
	return nil
}

// WithdrawQuotes withdraws all quotes of a note created by a specific user
func (r *QuoteRepository) WithdrawQuotes(ctx context.Context, noteID, userID string) (int, error) {
	db := r.relationshipRepo.GetDB()

	// Query all quotes by this user on this note
	var quotes []models.QuoteRelationship
	err := db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where("gsi2PK", "=", fmt.Sprintf("QUOTER#%s", userID)).
		Filter("TargetNoteID", "=", noteID).
		Filter("Withdrawn", "=", false).
		All(&quotes)

	if err != nil {
		r.logger.Error("failed to query quotes for withdrawal",
			zap.String("note_id", noteID),
			zap.String("user_id", userID),
			zap.Error(err))
		return 0, fmt.Errorf("%w: %w", ErrQuoteRelationshipQueryFailed, err)
	}

	// Withdraw each quote
	count := 0
	now := time.Now()
	for _, quote := range quotes {
		quote.Withdrawn = true
		quote.WithdrawnAt = &now

		// Update keys to clear GSI entries
		if err := quote.UpdateKeys(); err != nil {
			r.logger.Error("failed to update quote keys",
				zap.String("quote_id", quote.ID),
				zap.Error(err))
			continue
		}

		// Update the quote in the database
		err = r.relationshipRepo.Update(ctx, &quote)
		if err != nil {
			r.logger.Error("failed to withdraw quote",
				zap.String("quote_id", quote.ID),
				zap.Error(err))
			continue
		}

		count++
	}

	r.logger.Info("withdrew quotes",
		zap.String("note_id", noteID),
		zap.String("user_id", userID),
		zap.Int("count", count))

	return count, nil
}

// DecrementQuoteCount decrements the quote count for a status
func (r *QuoteRepository) DecrementQuoteCount(_ context.Context, statusID string) error {
	// This would typically update a counter in the Status model
	// For now, we'll log the operation
	r.logger.Info("decrementing quote count",
		zap.String("status_id", statusID))
	return nil
}
