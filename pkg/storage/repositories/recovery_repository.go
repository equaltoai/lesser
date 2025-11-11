package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RecoveryRepository implements recovery operations using enhanced repository patterns
type RecoveryRepository struct {
	*EnhancedBaseRepository[*models.RecoveryRequest]
}

// NewRecoveryRepository creates a new recovery repository with enhanced functionality
func NewRecoveryRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RecoveryRepository {
	// Create enhanced repository optimized for recovery operations
	enhancedRepo := NewEnhancedBaseRepository[*models.RecoveryRequest](db, tableName, logger, costService, "RecoveryRepository", "recovery")

	// Set up enhanced services for recovery operations - SECURITY CRITICAL
	enhancedRepo.SetValidationService(NewDefaultValidationService()) // Critical recovery validation
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Standard permissions
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Recovery requests cached
	enhancedRepo.SetEventService(NewDefaultEventService())           // Security event tracking

	return &RecoveryRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// Trustee operations

// StoreTrustee stores a trustee configuration for social recovery
func (r *RecoveryRepository) StoreTrustee(ctx context.Context, username string, trustee *storage.TrusteeConfig) error {
	// Convert storage.TrusteeConfig to models.Trustee
	model := &models.Trustee{
		Username:  username,
		ActorID:   trustee.ActorID,
		AddedAt:   trustee.AddedAt,
		Confirmed: trustee.Confirmed,
	}

	// Update keys
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityTrustee, "trustee keys")
	}

	// Create the trustee using DynamORM
	if err := r.BaseRepository.db.WithContext(ctx).Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityTrustee, trustee.ActorID)
	}

	return nil
}

// GetTrustees retrieves all trustees for a user
func (r *RecoveryRepository) GetTrustees(ctx context.Context, username string) ([]*storage.TrusteeConfig, error) {
	var trustees []models.Trustee

	pk := fmt.Sprintf("USER#%s", username)
	err := r.BaseRepository.db.WithContext(ctx).Model(&models.Trustee{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "TRUSTEE#").
		All(&trustees)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.TrusteeConfig{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityTrustee, "trustees")
	}

	// Convert models to storage types
	result := make([]*storage.TrusteeConfig, len(trustees))
	for i, trustee := range trustees {
		result[i] = &storage.TrusteeConfig{
			Username:  trustee.Username,
			ActorID:   trustee.ActorID,
			AddedAt:   trustee.AddedAt,
			Confirmed: trustee.Confirmed,
		}
	}

	return result, nil
}

// DeleteTrustee removes a trustee
func (r *RecoveryRepository) DeleteTrustee(ctx context.Context, username, trusteeActorID string) error {
	pk := fmt.Sprintf("USER#%s", username)
	sk := fmt.Sprintf("TRUSTEE#%s", trusteeActorID)

	if err := r.BaseRepository.db.WithContext(ctx).Model(&models.Trustee{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete(); err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityTrustee, trusteeActorID)
	}

	return nil
}

// UpdateTrusteeConfirmed updates the confirmed status of a trustee
func (r *RecoveryRepository) UpdateTrusteeConfirmed(ctx context.Context, username, trusteeActorID string, confirmed bool) error {
	pk := fmt.Sprintf("USER#%s", username)
	sk := fmt.Sprintf("TRUSTEE#%s", trusteeActorID)

	// Get the existing trustee
	var model models.Trustee
	err := r.BaseRepository.db.WithContext(ctx).Model(&models.Trustee{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityTrustee, trusteeActorID)
	}

	// Update the confirmed status
	model.Confirmed = confirmed
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityTrustee, "trustee keys")
	}

	// Save the changes
	if err := r.BaseRepository.db.WithContext(ctx).Model(&model).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityTrustee, trusteeActorID)
	}

	return nil
}

// Recovery request operations

// StoreRecoveryRequest stores a social recovery request
func (r *RecoveryRepository) StoreRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	// Convert storage.SocialRecoveryRequest to models.RecoveryRequest
	// Initialize ReceivedVotes map based on TrusteeVotes
	receivedVotes := make(map[string]bool)
	for _, trusteeID := range request.TrusteeVotes {
		receivedVotes[trusteeID] = true
	}

	model := &models.RecoveryRequest{
		ID:            request.ID,
		Username:      request.Username,
		InitiatedAt:   request.InitiatedAt,
		ExpiresAt:     request.ExpiresAt,
		RequiredVotes: request.RequiredVotes,
		ReceivedVotes: receivedVotes,
		RecoveryToken: request.RecoveryToken,
		Status:        request.Status,
	}

	// Update keys (this will set PK, SK, GSI keys, and TTL)
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the recovery request
	if err := r.db.WithContext(ctx).Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityRecoveryRequest, request.ID)
	}

	return nil
}

// GetRecoveryRequest retrieves a recovery request by ID
func (r *RecoveryRepository) GetRecoveryRequest(ctx context.Context, requestID string) (*storage.SocialRecoveryRequest, error) {
	var model models.RecoveryRequest

	pk := fmt.Sprintf("RECOVERY#%s", requestID)
	err := r.Get(ctx, pk, "REQUEST", &model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityRecoveryRequest, requestID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityRecoveryRequest, requestID)
	}

	// Convert model to storage type
	// Convert ReceivedVotes map to TrusteeVotes slice and count
	trusteeVotes := make([]string, 0, len(model.ReceivedVotes))
	for trusteeID, voted := range model.ReceivedVotes {
		if voted {
			trusteeVotes = append(trusteeVotes, trusteeID)
		}
	}

	result := &storage.SocialRecoveryRequest{
		ID:            model.ID,
		Username:      model.Username,
		RequestorID:   model.Username, // Assuming requestor is the username
		InitiatedAt:   model.InitiatedAt,
		ExpiresAt:     model.ExpiresAt,
		RequiredVotes: model.RequiredVotes,
		ReceivedVotes: len(trusteeVotes),
		TrusteeVotes:  trusteeVotes,
		RecoveryToken: model.RecoveryToken,
		Status:        model.Status,
		CreatedAt:     model.InitiatedAt, // Use InitiatedAt as CreatedAt
	}

	return result, nil
}

// UpdateRecoveryRequest updates a recovery request
func (r *RecoveryRepository) UpdateRecoveryRequest(ctx context.Context, request *storage.SocialRecoveryRequest) error {
	// For update, we'll just call StoreRecoveryRequest since it's a full replacement
	return r.StoreRecoveryRequest(ctx, request)
}

// DeleteRecoveryRequest deletes a recovery request
func (r *RecoveryRepository) DeleteRecoveryRequest(ctx context.Context, requestID string) error {
	pk := fmt.Sprintf("RECOVERY#%s", requestID)
	return r.Delete(ctx, pk, "REQUEST")
}

// GetActiveRecoveryRequests gets all active recovery requests for a user
func (r *RecoveryRepository) GetActiveRecoveryRequests(ctx context.Context, username string) ([]*storage.SocialRecoveryRequest, error) {
	var requests []models.RecoveryRequest

	gsi1pk := fmt.Sprintf("USER#%s", username)
	now := time.Now()

	// Query using GSI1
	err := r.BaseRepository.db.WithContext(ctx).Model(&models.RecoveryRequest{}).
		Index("GSI1").
		Where("gsi1PK", "=", gsi1pk).
		Where("gsi1SK", "begins_with", "RECOVERY#").
		All(&requests)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.SocialRecoveryRequest{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRecoveryRequest, "active requests")
	}

	// Filter for active (pending and not expired) requests
	result := make([]*storage.SocialRecoveryRequest, 0)
	for _, req := range requests {
		if req.Status == models.StatusPending && now.Before(req.ExpiresAt) {
			// Convert ReceivedVotes map to TrusteeVotes slice and count
			trusteeVotes := make([]string, 0, len(req.ReceivedVotes))
			for trusteeID, voted := range req.ReceivedVotes {
				if voted {
					trusteeVotes = append(trusteeVotes, trusteeID)
				}
			}

			result = append(result, &storage.SocialRecoveryRequest{
				ID:            req.ID,
				Username:      req.Username,
				RequestorID:   req.Username,
				InitiatedAt:   req.InitiatedAt,
				ExpiresAt:     req.ExpiresAt,
				RequiredVotes: req.RequiredVotes,
				ReceivedVotes: len(trusteeVotes),
				TrusteeVotes:  trusteeVotes,
				RecoveryToken: req.RecoveryToken,
				Status:        req.Status,
				CreatedAt:     req.InitiatedAt,
			})
		}
	}

	return result, nil
}

// Recovery code operations

// StoreRecoveryCode stores a recovery code
func (r *RecoveryRepository) StoreRecoveryCode(ctx context.Context, username string, code *storage.RecoveryCodeItem) error {
	// Convert storage.RecoveryCodeItem to models.RecoveryCode
	model := &models.RecoveryCode{
		Username:  username,
		CodeHash:  code.CodeHash,
		CreatedAt: code.CreatedAt,
		UsedAt:    code.UsedAt,
		Position:  code.Position,
	}

	// Update keys
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRecoveryCode, "keys")
	}

	// Create the recovery code
	if err := r.BaseRepository.db.WithContext(ctx).Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityRecoveryCode, username)
	}

	return nil
}

// GetRecoveryCodes retrieves all recovery codes for a user
func (r *RecoveryRepository) GetRecoveryCodes(ctx context.Context, username string) ([]*storage.RecoveryCodeItem, error) {
	var codes []models.RecoveryCode

	pk := fmt.Sprintf("USER#%s", username)
	err := r.BaseRepository.db.WithContext(ctx).Model(&models.RecoveryCode{}).
		Where("PK", "=", pk).
		Where("SK", "begins_with", "RECOVERY_CODE#").
		All(&codes)

	if err != nil {
		if errors.IsNotFound(err) {
			return []*storage.RecoveryCodeItem{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, EntityRecoveryCode, "codes")
	}

	// Convert models to storage types
	result := make([]*storage.RecoveryCodeItem, len(codes))
	for i, code := range codes {
		result[i] = &storage.RecoveryCodeItem{
			Username:  code.Username,
			CodeHash:  code.CodeHash,
			CreatedAt: code.CreatedAt,
			UsedAt:    code.UsedAt,
			Position:  code.Position,
		}
	}

	return result, nil
}

// MarkRecoveryCodeUsed marks a recovery code as used
func (r *RecoveryRepository) MarkRecoveryCodeUsed(ctx context.Context, username, codeHash string) error {
	// First, find the code by hash
	codes, err := r.GetRecoveryCodes(ctx, username)
	if err != nil {
		return err
	}

	var targetCode *storage.RecoveryCodeItem
	for _, code := range codes {
		if code.CodeHash == codeHash {
			targetCode = code
			break
		}
	}

	if targetCode == nil {
		return ErrorHandler.HandleGetError(storage.ErrNotFound, EntityRecoveryCode, codeHash)
	}

	// Update the code with used timestamp
	pk := fmt.Sprintf("USER#%s", username)
	sk := fmt.Sprintf("RECOVERY_CODE#%d", targetCode.Position)
	now := time.Now()

	// Get the existing recovery code
	var codeModel models.RecoveryCode
	err = r.BaseRepository.db.WithContext(ctx).Model(&models.RecoveryCode{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&codeModel)

	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityRecoveryCode, username)
	}

	// Update the used timestamp
	codeModel.UsedAt = &now
	if err := codeModel.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRecoveryCode, "keys")
	}

	// Save the changes
	if err := r.BaseRepository.db.WithContext(ctx).Model(&codeModel).Update(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRecoveryCode, username)
	}

	return nil
}

// DeleteAllRecoveryCodes deletes all recovery codes for a user
func (r *RecoveryRepository) DeleteAllRecoveryCodes(ctx context.Context, username string) error {
	// First, get all codes
	codes, err := r.GetRecoveryCodes(ctx, username)
	if err != nil {
		return err
	}

	// Delete each code
	pk := fmt.Sprintf("USER#%s", username)
	for _, code := range codes {
		sk := fmt.Sprintf("RECOVERY_CODE#%d", code.Position)
		if err := r.BaseRepository.db.WithContext(ctx).Model(&models.RecoveryCode{}).
			Where("PK", "=", pk).
			Where("SK", "=", sk).
			Delete(); err != nil {
			return ErrorHandler.HandleDeleteError(err, EntityRecoveryCode, username)
		}
	}

	return nil
}

// CountUnusedRecoveryCodes counts how many unused recovery codes the user has
func (r *RecoveryRepository) CountUnusedRecoveryCodes(ctx context.Context, username string) (int, error) {
	codes, err := r.GetRecoveryCodes(ctx, username)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, code := range codes {
		if code.UsedAt == nil {
			count++
		}
	}

	return count, nil
}

// Recovery token operations

// StoreRecoveryToken stores a generic recovery token with data
func (r *RecoveryRepository) StoreRecoveryToken(ctx context.Context, key string, data map[string]any) error {
	// Create the recovery token model
	model := &models.RecoveryToken{
		PK:        key,
		Data:      data,
		CreatedAt: time.Now(),
	}

	// Update keys (this will set SK and TTL)
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityRecoveryToken, "keys")
	}

	// Create the recovery token
	if err := r.BaseRepository.db.WithContext(ctx).Model(model).Create(); err != nil {
		return ErrorHandler.HandleCreateError(err, EntityRecoveryToken, key)
	}

	return nil
}

// GetRecoveryToken retrieves a recovery token by key
func (r *RecoveryRepository) GetRecoveryToken(ctx context.Context, key string) (map[string]any, error) {
	var model models.RecoveryToken

	err := r.BaseRepository.db.WithContext(ctx).Model(&models.RecoveryToken{}).
		Where("PK", "=", key).
		Where("SK", "=", "TOKEN").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityRecoveryToken, key)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityRecoveryToken, key)
	}

	return model.Data, nil
}

// DeleteRecoveryToken deletes a recovery token
func (r *RecoveryRepository) DeleteRecoveryToken(ctx context.Context, key string) error {
	if err := r.BaseRepository.db.WithContext(ctx).Model(&models.RecoveryToken{}).
		Where("PK", "=", key).
		Where("SK", "=", "TOKEN").
		Delete(); err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityRecoveryToken, key)
	}

	return nil
}
