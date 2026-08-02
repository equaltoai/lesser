package graph

import (
	"context"
	stdErrors "errors"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// UpdateQuotePermissions is the resolver for the updateQuotePermissions field.
func (r *mutationResolver) UpdateQuotePermissions(ctx context.Context, noteID string, quoteable bool, permission model.QuotePermission) (*model.UpdateQuotePermissionsPayload, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	// Verify user owns the note
	statusRepo := r.Registry.GetStorage().Status()
	if statusRepo == nil {
		r.Logger.Error("status repository not available")
		return nil, errors.ServiceUnavailable("status repository")
	}

	status, err := statusRepo.GetStatus(ctx, noteID)
	if err != nil {
		r.Logger.Error("failed to get status",
			zap.String("note_id", noteID),
			zap.Error(err))
		return nil, stdErrors.Join(stdErrors.New("processing failed"), err)
	}

	// Verify ownership
	if status.AuthorUsername != username {
		r.Logger.Warn("user cannot update quote permissions for note owned by another user",
			zap.String("user", username),
			zap.String("author", status.AuthorUsername))
		return nil, errors.Forbidden("cannot update quote permissions for note owned by another user")
	}

	// Build quote permissions for the note
	storagePermissions := &storage.QuotePermissions{
		AllowPublic:    false,
		AllowFollowers: false,
		AllowMentioned: false,
	}

	// Map GraphQL permission enum to storage model
	if quoteable {
		switch permission {
		case model.QuotePermissionEveryone:
			storagePermissions.AllowPublic = true
			storagePermissions.AllowFollowers = true
			storagePermissions.AllowMentioned = true
		case model.QuotePermissionFollowers:
			storagePermissions.AllowPublic = false
			storagePermissions.AllowFollowers = true
			storagePermissions.AllowMentioned = true
		case model.QuotePermissionNone:
			storagePermissions.AllowPublic = false
			storagePermissions.AllowFollowers = false
			storagePermissions.AllowMentioned = false
		}
	}

	// Update permissions using object repository
	objectRepo := r.Registry.GetStorage().Object()
	if objectRepo == nil {
		r.Logger.Error("object repository not available")
		return nil, errors.ServiceUnavailable("object repository")
	}

	err = objectRepo.UpdateQuotePermissions(ctx, noteID, storagePermissions)
	if err != nil {
		r.Logger.Error("failed to update quote permissions",
			zap.String("user", username),
			zap.String("note_id", noteID),
			zap.Error(err))
		return nil, stdErrors.Join(stdErrors.New("processing failed"), err)
	}

	// Track cost using centralized tracker
	r.trackDynamoOperation(ctx, "write", 1)

	// Read the stored control back rather than echoing the request. Status metadata is the
	// authoritative per-note record, and the mutation payload must describe that record.
	storedQuoteType, err := objectRepo.GetQuoteType(ctx, noteID)
	if err != nil {
		r.Logger.Error("failed to read updated quote permissions",
			zap.String("user", username),
			zap.String("note_id", noteID),
			zap.Error(err))
		return nil, stdErrors.Join(stdErrors.New("processing failed"), err)
	}
	storedQuoteable, storedPermission := graphQLQuoteControl(storedQuoteType)

	// Get the note to return
	note := r.convertStatusToObject(ctx, status)
	note.Quoteable = storedQuoteable
	note.QuotePermissions = storedPermission

	r.Logger.Info("quote permissions updated",
		zap.String("user", username),
		zap.String("note_id", noteID),
		zap.Bool("quoteable", quoteable),
		zap.String("permission", string(permission)))

	return &model.UpdateQuotePermissionsPayload{
		Success:        true,
		Note:           note,
		AffectedQuotes: 0, // Would require complex query to calculate
	}, nil
}

func graphQLQuoteControl(storedQuoteType string) (bool, model.QuotePermission) {
	switch storedQuoteType {
	case "public":
		return true, model.QuotePermissionEveryone
	case EventTypeFollowers:
		return true, model.QuotePermissionFollowers
	case "mentioned":
		return true, model.QuotePermissionFollowers
	default:
		return false, model.QuotePermissionNone
	}
}
