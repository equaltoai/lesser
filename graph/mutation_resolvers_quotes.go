package graph

import (
	"context"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// UpdateQuotePermissions is the resolver for the updateQuotePermissions field.
func (r *mutationResolver) UpdateQuotePermissions(ctx context.Context, noteID string, quoteable bool, permission model.QuotePermission) (*model.UpdateQuotePermissionsPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Get the quotes service
	quotesService := r.Registry.Quotes()
	if quotesService == nil {
		r.Logger.Error("quotes service not available")
		return nil, errors.ServiceUnavailable("quotes service")
	}

	// Build quote permissions
	permissions := &models.QuotePermissions{
		Username: username,
	}

	// Map GraphQL permission enum to storage model
	switch permission {
	case model.QuotePermissionEveryone:
		permissions.AllowPublic = true
		permissions.AllowFollowers = true
		permissions.AllowMentioned = true
	case model.QuotePermissionFollowers:
		permissions.AllowPublic = false
		permissions.AllowFollowers = true
		permissions.AllowMentioned = true
	case model.QuotePermissionNone:
		permissions.AllowPublic = false
		permissions.AllowFollowers = false
		permissions.AllowMentioned = false
	}

	// If quoteable is false, override all permissions to deny
	if !quoteable {
		permissions.AllowPublic = false
		permissions.AllowFollowers = false
		permissions.AllowMentioned = false
	}

	// Update permissions
	err = quotesService.UpdateQuotePermissions(ctx, permissions)
	if err != nil {
		r.Logger.Error("failed to update quote permissions",
			zap.String("user", username),
			zap.Error(err))
		return nil, err
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	// Get the note to return
	var note *model.Object
	// Access storage through the unexported field (internal to services package)
	if r.Registry != nil {
		storageRepo := r.Registry.GetStorage()
		if storageRepo != nil {
			statusRepo := storageRepo.Status()
			if statusRepo != nil {
				status, getErr := statusRepo.GetStatus(ctx, noteID)
				if getErr != nil {
					r.Logger.Warn("failed to get note after updating permissions",
						zap.String("note_id", noteID),
						zap.Error(getErr))
				} else if status != nil {
					note = r.convertStatusToObject(ctx, status)
				}
			}
		}
	}

	// Count affected quotes (quotes that may need to be re-checked)
	// For now, we'll return 0 as this would require a complex query
	affectedQuotes := 0

	r.Logger.Info("quote permissions updated",
		zap.String("user", username),
		zap.String("note_id", noteID),
		zap.Bool("quoteable", quoteable),
		zap.String("permission", string(permission)))

	return &model.UpdateQuotePermissionsPayload{
		Success:        true,
		Note:           note,
		AffectedQuotes: affectedQuotes,
	}, nil
}
