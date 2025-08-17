package repositories

import (
	"context"
	"fmt"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// QuoteRepository implements quote operations using DynamORM
type QuoteRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewQuoteRepository creates a new quote repository
func NewQuoteRepository(db core.DB, tableName string, logger *zap.Logger) *QuoteRepository {
	return &QuoteRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateQuoteRelationship creates a new quote relationship
func (r *QuoteRepository) CreateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error {
	relationship.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(relationship).Create(); err != nil {
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
		return fmt.Errorf("failed to create quote relationship: %w", err)
	}

	r.logger.Info("created quote relationship",
		zap.String("quoter_note_id", relationship.QuoterNoteID),
		zap.String("target_note_id", relationship.TargetNoteID))

	return nil
}

// GetQuoteRelationship retrieves a quote relationship by quoter and target note IDs
func (r *QuoteRepository) GetQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) (*models.QuoteRelationship, error) {
	var relationship models.QuoteRelationship

	err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where("PK", "=", fmt.Sprintf("QUOTE#%s", quoteStatusID)).
		Where("SK", "=", fmt.Sprintf("QUOTED#%s", targetStatusID)).
		First(&relationship)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("failed to get quote relationship",
			zap.String("quote_status_id", quoteStatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get quote relationship: %w", err)
	}

	return &relationship, nil
}

// UpdateQuoteRelationship updates an existing quote relationship
func (r *QuoteRepository) UpdateQuoteRelationship(ctx context.Context, relationship *models.QuoteRelationship) error {
	relationship.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(relationship).Update(); err != nil {
		r.logger.Error("failed to update quote relationship",
			zap.String("quoter_note_id", relationship.QuoterNoteID),
			zap.String("target_note_id", relationship.TargetNoteID),
			zap.Error(err))
		return fmt.Errorf("failed to update quote relationship: %w", err)
	}

	r.logger.Info("updated quote relationship",
		zap.String("quoter_note_id", relationship.QuoterNoteID),
		zap.String("target_note_id", relationship.TargetNoteID))

	return nil
}

// DeleteQuoteRelationship deletes a quote relationship
func (r *QuoteRepository) DeleteQuoteRelationship(ctx context.Context, quoteStatusID, targetStatusID string) error {
	err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where("PK", "=", fmt.Sprintf("QUOTE#%s", quoteStatusID)).
		Where("SK", "=", fmt.Sprintf("QUOTED#%s", targetStatusID)).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete quote relationship",
			zap.String("quote_status_id", quoteStatusID),
			zap.String("target_status_id", targetStatusID),
			zap.Error(err))
		return fmt.Errorf("failed to delete quote relationship: %w", err)
	}

	r.logger.Info("deleted quote relationship",
		zap.String("quote_status_id", quoteStatusID),
		zap.String("target_status_id", targetStatusID))

	return nil
}

// getQuotesByGSI is a generic helper for querying quotes by GSI
func (r *QuoteRepository) getQuotesByGSI(ctx context.Context, gsiKey, gsiValue string, opts interfaces.PaginationOptions, errorContext string) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	query := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where(gsiKey, "=", gsiValue).
		Limit(opts.Limit)

	if opts.Cursor != "" {
		query = query.Cursor(opts.Cursor)
	}

	var quotesData []models.QuoteRelationship
	err := query.All(&quotesData)
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to %s", errorContext),
			zap.String("gsi_value", gsiValue),
			zap.Error(err))
		return nil, fmt.Errorf("failed to %s: %w", errorContext, err)
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
	return r.getQuotesByGSI(ctx, "GSI1PK", fmt.Sprintf("QUOTED#%s", statusID), opts, "get quotes for status")
}

// GetQuotesByUser retrieves quotes created by a specific user
func (r *QuoteRepository) GetQuotesByUser(ctx context.Context, userID string, opts interfaces.PaginationOptions) (*interfaces.PaginatedResult[*models.QuoteRelationship], error) {
	return r.getQuotesByGSI(ctx, "GSI2PK", fmt.Sprintf("QUOTER#%s", userID), opts, "get quotes by user")
}

// CreateQuotePermissions creates new quote permissions for a user
func (r *QuoteRepository) CreateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	permissions.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(permissions).Create(); err != nil {
		if errors.IsConditionFailed(err) {
			r.logger.Debug("quote permissions already exist",
				zap.String("username", permissions.Username))
			return nil
		}
		r.logger.Error("failed to create quote permissions",
			zap.String("username", permissions.Username),
			zap.Error(err))
		return fmt.Errorf("failed to create quote permissions: %w", err)
	}

	r.logger.Info("created quote permissions",
		zap.String("username", permissions.Username))

	return nil
}

// GetQuotePermissions retrieves quote permissions for a user
func (r *QuoteRepository) GetQuotePermissions(ctx context.Context, username string) (*models.QuotePermissions, error) {
	var permissions models.QuotePermissions

	err := r.db.WithContext(ctx).Model(&models.QuotePermissions{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", "QUOTE_PERMISSIONS").
		First(&permissions)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		r.logger.Error("failed to get quote permissions",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get quote permissions: %w", err)
	}

	return &permissions, nil
}

// UpdateQuotePermissions updates existing quote permissions
func (r *QuoteRepository) UpdateQuotePermissions(ctx context.Context, permissions *models.QuotePermissions) error {
	permissions.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(permissions).Update(); err != nil {
		r.logger.Error("failed to update quote permissions",
			zap.String("username", permissions.Username),
			zap.Error(err))
		return fmt.Errorf("failed to update quote permissions: %w", err)
	}

	r.logger.Info("updated quote permissions",
		zap.String("username", permissions.Username))

	return nil
}

// DeleteQuotePermissions deletes quote permissions for a user
func (r *QuoteRepository) DeleteQuotePermissions(ctx context.Context, username string) error {
	err := r.db.WithContext(ctx).Model(&models.QuotePermissions{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", "QUOTE_PERMISSIONS").
		Delete()

	if err != nil {
		r.logger.Error("failed to delete quote permissions",
			zap.String("username", username),
			zap.Error(err))
		return fmt.Errorf("failed to delete quote permissions: %w", err)
	}

	r.logger.Info("deleted quote permissions",
		zap.String("username", username))

	return nil
}

// GetQuoteCount gets the total number of quotes for a status
func (r *QuoteRepository) GetQuoteCount(ctx context.Context, statusID string) (int64, error) {
	count, err := r.db.WithContext(ctx).Model(&models.QuoteRelationship{}).
		Where("GSI1PK", "=", fmt.Sprintf("QUOTED#%s", statusID)).
		Where("Withdrawn", "=", false).
		Count()

	if err != nil {
		r.logger.Error("failed to get quote count",
			zap.String("status_id", statusID),
			zap.Error(err))
		return 0, fmt.Errorf("failed to get quote count: %w", err)
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

// DecrementQuoteCount decrements the quote count for a status
func (r *QuoteRepository) DecrementQuoteCount(_ context.Context, statusID string) error {
	// This would typically update a counter in the Status model
	// For now, we'll log the operation
	r.logger.Info("decrementing quote count",
		zap.String("status_id", statusID))
	return nil
}