package repositories

import (
	"context"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

type webSocketConnectionValidationService struct {
	base *DefaultValidationService
}

func NewWebSocketConnectionValidationService() ValidationService {
	return &webSocketConnectionValidationService{base: NewDefaultValidationService()}
}

func (v *webSocketConnectionValidationService) ValidateModel(ctx context.Context, model BaseModel) error {
	return v.base.ValidateModel(ctx, model)
}

func (v *webSocketConnectionValidationService) ValidateBusinessRules(ctx context.Context, model BaseModel, action string) error {
	return v.base.ValidateBusinessRules(ctx, model, action)
}

func (v *webSocketConnectionValidationService) ValidateRequiredFields(ctx context.Context, model BaseModel) error {
	conn, ok := model.(*models.WebSocketConnection)
	if !ok || conn == nil {
		return v.base.ValidateRequiredFields(ctx, model)
	}

	// For authenticated connections, `Username` must be present.
	if conn.UserID != "" && conn.Username == "" {
		return errors.ValidationFailed("Username", "field 'Username' is required")
	}

	// Anonymous connections are allowed for public streams, so permit empty username.
	if conn.UserID == "" && conn.Username == "" {
		original := conn.Username
		conn.Username = "_"
		defer func() { conn.Username = original }()
	}

	return v.base.ValidateRequiredFields(ctx, model)
}
