package repositories

import (
	"context"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

const (
	oauthDeviceSessionSK = "SESSION"
)

func (r *AccountRepository) CreateOAuthDeviceSession(ctx context.Context, session *storage.OAuthDeviceSession) error {
	if r == nil || r.db == nil {
		return ErrorHandler.HandleCreateError(storage.ErrDatabaseConnectionFailed, "oauth device session", "init")
	}
	if session == nil {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, "oauth device session", "nil")
	}

	session.DeviceCodeHash = strings.TrimSpace(session.DeviceCodeHash)
	session.UserCode = strings.TrimSpace(session.UserCode)
	session.ClientID = strings.TrimSpace(session.ClientID)
	session.Status = strings.TrimSpace(session.Status)

	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"device_code_hash": session.DeviceCodeHash,
		"user_code":        session.UserCode,
		"client_id":        session.ClientID,
		"status":           session.Status,
	}); err != nil {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, "oauth device session", "validation")
	}
	if session.ExpiresAt.IsZero() {
		return ErrorHandler.HandleCreateError(storage.ErrInvalidInput, "oauth device session", "expires_at")
	}

	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now

	model := oauthDeviceSessionModelFromStorage(session)
	_ = model.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailed") || strings.Contains(err.Error(), "already exists") {
			return ErrorHandler.HandleCreateError(storage.ErrAlreadyExists, "oauth device session", session.DeviceCodeHash)
		}
		r.logger.Error("failed to create oauth device session",
			zap.Error(err),
			zap.String("client_id", session.ClientID),
		)
		return ErrorHandler.HandleCreateError(err, "oauth device session", session.DeviceCodeHash)
	}

	return nil
}

func (r *AccountRepository) GetOAuthDeviceSession(ctx context.Context, deviceCodeHash string) (*storage.OAuthDeviceSession, error) {
	if r == nil || r.db == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrDatabaseConnectionFailed, "oauth device session", "init")
	}

	deviceCodeHash = strings.TrimSpace(deviceCodeHash)
	if deviceCodeHash == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "oauth device session", "device_code_hash")
	}

	pk := "OAUTH_DEVICE#" + deviceCodeHash
	sk := oauthDeviceSessionSK

	var model models.OAuthDeviceSession
	err := r.db.WithContext(ctx).Model(&models.OAuthDeviceSession{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "oauth device session", deviceCodeHash)
		}
		r.logger.Error("failed to get oauth device session",
			zap.Error(err),
			zap.String("device_code_hash", deviceCodeHash),
		)
		return nil, ErrorHandler.HandleGetError(err, "oauth device session", deviceCodeHash)
	}

	out := oauthDeviceSessionStorageFromModel(&model)
	if out == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "oauth device session", deviceCodeHash)
	}

	return out, nil
}

func (r *AccountRepository) GetOAuthDeviceSessionByUserCode(ctx context.Context, userCode string) (*storage.OAuthDeviceSession, error) {
	if r == nil || r.db == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrDatabaseConnectionFailed, "oauth device session", "init")
	}

	userCode = strings.TrimSpace(userCode)
	if userCode == "" {
		return nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "oauth device session", "user_code")
	}

	gsi1PK := "OAUTH_DEVICE_USER_CODE#" + userCode

	var modelsOut []models.OAuthDeviceSession
	err := r.db.WithContext(ctx).Model(&models.OAuthDeviceSession{}).
		Index(models.IndexGSI1).
		Where("gsi1PK", "=", gsi1PK).
		Limit(2).
		All(&modelsOut)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "oauth device session", userCode)
		}
		r.logger.Error("failed to get oauth device session by user code", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "oauth device session", userCode)
	}

	if len(modelsOut) == 0 {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "oauth device session", userCode)
	}

	// user_code is expected to be unique; if duplicates exist, pick the first item.
	out := oauthDeviceSessionStorageFromModel(&modelsOut[0])
	if out == nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, "oauth device session", userCode)
	}

	return out, nil
}

func (r *AccountRepository) UpdateOAuthDeviceSession(ctx context.Context, session *storage.OAuthDeviceSession) error {
	if r == nil || r.db == nil {
		return ErrorHandler.HandleUpdateError(storage.ErrDatabaseConnectionFailed, "oauth device session", "init")
	}
	if session == nil {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, "oauth device session", "nil")
	}

	session.DeviceCodeHash = strings.TrimSpace(session.DeviceCodeHash)
	if session.DeviceCodeHash == "" {
		return ErrorHandler.HandleUpdateError(storage.ErrInvalidInput, "oauth device session", "device_code_hash")
	}

	session.UpdatedAt = time.Now().UTC()
	model := oauthDeviceSessionModelFromStorage(session)
	_ = model.UpdateKeys()

	if err := r.db.WithContext(ctx).Model(model).Update(); err != nil {
		r.logger.Error("failed to update oauth device session",
			zap.Error(err),
			zap.String("device_code_hash", session.DeviceCodeHash),
		)
		return ErrorHandler.HandleUpdateError(err, "oauth device session", session.DeviceCodeHash)
	}

	return nil
}

func oauthDeviceSessionModelFromStorage(session *storage.OAuthDeviceSession) *models.OAuthDeviceSession {
	if session == nil {
		return nil
	}

	var lastPolledAt *time.Time
	if !session.LastPolledAt.IsZero() {
		t := session.LastPolledAt
		lastPolledAt = &t
	}
	var approvedAt *time.Time
	if !session.ApprovedAt.IsZero() {
		t := session.ApprovedAt
		approvedAt = &t
	}
	var deniedAt *time.Time
	if !session.DeniedAt.IsZero() {
		t := session.DeniedAt
		deniedAt = &t
	}
	var consumedAt *time.Time
	if !session.ConsumedAt.IsZero() {
		t := session.ConsumedAt
		consumedAt = &t
	}

	return &models.OAuthDeviceSession{
		DeviceCodeHash:   session.DeviceCodeHash,
		UserCode:         session.UserCode,
		ClientID:         session.ClientID,
		Scopes:           session.Scopes,
		Status:           session.Status,
		IntervalSeconds:  session.IntervalSeconds,
		PollCount:        session.PollCount,
		LastPolledAt:     lastPolledAt,
		ApprovedUsername: session.ApprovedUsername,
		ApprovedAt:       approvedAt,
		DeniedAt:         deniedAt,
		ConsumedAt:       consumedAt,
		CreatedAt:        session.CreatedAt,
		UpdatedAt:        session.UpdatedAt,
		ExpiresAt:        session.ExpiresAt,
	}
}

func oauthDeviceSessionStorageFromModel(model *models.OAuthDeviceSession) *storage.OAuthDeviceSession {
	if model == nil {
		return nil
	}

	out := &storage.OAuthDeviceSession{
		DeviceCodeHash:   model.DeviceCodeHash,
		UserCode:         model.UserCode,
		ClientID:         model.ClientID,
		Scopes:           model.Scopes,
		Status:           model.Status,
		IntervalSeconds:  model.IntervalSeconds,
		PollCount:        model.PollCount,
		ApprovedUsername: model.ApprovedUsername,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
		ExpiresAt:        model.ExpiresAt,
	}
	if model.LastPolledAt != nil {
		out.LastPolledAt = *model.LastPolledAt
	}
	if model.ApprovedAt != nil {
		out.ApprovedAt = *model.ApprovedAt
	}
	if model.DeniedAt != nil {
		out.DeniedAt = *model.DeniedAt
	}
	if model.ConsumedAt != nil {
		out.ConsumedAt = *model.ConsumedAt
	}
	return out
}
