package repositories

import (
	"context"

	"github.com/equaltoai/lesser/pkg/errors"
)

func validateRequiredFieldsAllowAnonymousUsername(ctx context.Context, base *DefaultValidationService, model BaseModel, userID string, username *string) error {
	if username == nil {
		return base.ValidateRequiredFields(ctx, model)
	}

	// For authenticated connections, `Username` must be present.
	if userID != "" && *username == "" {
		return errors.ValidationFailed("Username", "field 'Username' is required")
	}

	// Anonymous connections are allowed for public streams, so permit empty username.
	if userID == "" && *username == "" {
		original := *username
		*username = "_"
		defer func() { *username = original }()
	}

	return base.ValidateRequiredFields(ctx, model)
}
