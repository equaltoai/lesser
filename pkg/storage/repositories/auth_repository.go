package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// AuthRepository handles authentication-related storage operations using enhanced patterns
type AuthRepository struct {
	*EnhancedBaseRepository[*models.WebAuthnCredential]
	// Auth-specific dependencies
	costService *cost.TrackingService
}

// NewAuthRepository creates a new auth repository with enhanced functionality
func NewAuthRepository(db core.DB, tableName string, logger *zap.Logger) *AuthRepository {
	// Create enhanced repository optimized for auth operations
	enhancedRepo := NewEnhancedBaseRepository[*models.WebAuthnCredential](db, tableName, logger, nil, "AuthRepository", "webauthn_credential")

	// Set up enhanced services for auth operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Critical for security
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Auth credentials cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for security events

	return &AuthRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// NewAuthRepositoryWithCostTracking creates a new auth repository with cost tracking
func NewAuthRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *AuthRepository {
	// Create enhanced repository with cost tracking
	enhancedRepo := NewEnhancedBaseRepository[*models.WebAuthnCredential](db, tableName, logger, costService, "AuthRepository", "webauthn_credential")

	// Set up enhanced services for auth operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService()) // Critical for security
	enhancedRepo.SetCachingService(NewInMemoryCachingService())      // Auth credentials cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())           // Important for security events

	return &AuthRepository{
		EnhancedBaseRepository: enhancedRepo,
		costService:            costService,
	}
}

// WebAuthn Credential Operations

// CreateWebAuthnCredential creates a new WebAuthn credential
func (r *AuthRepository) CreateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	// Create DynamORM model
	model := &models.WebAuthnCredential{
		ID:              credential.ID,
		UserID:          credential.UserID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.AAGUID,
		SignCount:       credential.SignCount,
		CloneWarning:    credential.CloneWarning,
		BackupEligible:  credential.BackupEligible,
		BackupState:     credential.BackupState,
		CreatedAt:       credential.CreatedAt,
		LastUsedAt:      credential.LastUsedAt,
		Name:            credential.Name,
	}

	// Set default timestamps if not provided
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now()
	}
	if model.LastUsedAt.IsZero() {
		model.LastUsedAt = model.CreatedAt
	}

	// Use BaseRepository Create method
	err := r.Create(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnCredential, credential.CredentialID)
	}

	r.logger.Debug("created WebAuthn credential",
		zap.String("credential_id", credential.ID),
		zap.String("user_id", credential.UserID))

	return nil
}

// GetWebAuthnCredential retrieves a WebAuthn credential by ID
func (r *AuthRepository) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	// Need to scan for the credential since we don't know the user
	var modelList []models.WebAuthnCredential
	err := r.db.WithContext(ctx).Model(&models.WebAuthnCredential{}).
		Where("id", "=", credentialID).
		Where("Type", "=", "WebAuthnCredential").
		Scan(&modelList)

	if err != nil {
		r.logger.Error("failed to get WebAuthn credential", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityWebAuthnCredential, credentialID)
	}

	if err := common.ValidateSliceNotEmpty("modelList", modelList); err != nil {
		return nil, ErrorHandler.HandleGetError(nil, EntityWebAuthnCredential, credentialID)
	}

	// Convert to storage model
	model := modelList[0]
	result := &storage.WebAuthnCredential{
		ID:              model.ID,
		UserID:          model.UserID,
		PublicKey:       model.PublicKey,
		AttestationType: model.AttestationType,
		AAGUID:          model.AAGUID,
		SignCount:       model.SignCount,
		CloneWarning:    model.CloneWarning,
		BackupEligible:  model.BackupEligible,
		BackupState:     model.BackupState,
		CreatedAt:       model.CreatedAt,
		LastUsedAt:      model.LastUsedAt,
		Name:            model.Name,
	}

	r.logger.Debug("retrieved WebAuthn credential",
		zap.String("credential_id", credentialID),
		zap.String("user_id", result.UserID))

	return result, nil
}

// GetUserWebAuthnCredentials retrieves all WebAuthn credentials for a user
func (r *AuthRepository) GetUserWebAuthnCredentials(ctx context.Context, userID string) ([]*storage.WebAuthnCredential, error) {
	// Construct the key prefix
	pk := "USER#" + userID

	// Use BaseRepository QueryWithSKPrefix method
	modelList, err := r.QueryWithSKPrefix(ctx, pk, "WEBAUTHN_CRED#", 0)
	if err != nil {
		r.logger.Error("failed to get user WebAuthn credentials", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityWebAuthnCredential, fmt.Sprintf("user %s", userID))
	}

	// Convert to storage models
	results := make([]*storage.WebAuthnCredential, len(modelList))
	for i, model := range modelList {
		results[i] = &storage.WebAuthnCredential{
			ID:              model.ID,
			UserID:          model.UserID,
			PublicKey:       model.PublicKey,
			AttestationType: model.AttestationType,
			AAGUID:          model.AAGUID,
			SignCount:       model.SignCount,
			CloneWarning:    model.CloneWarning,
			BackupEligible:  model.BackupEligible,
			BackupState:     model.BackupState,
			CreatedAt:       model.CreatedAt,
			LastUsedAt:      model.LastUsedAt,
			Name:            model.Name,
		}
	}

	r.logger.Debug("retrieved user WebAuthn credentials",
		zap.String("user_id", userID),
		zap.Int("count", len(results)))

	return results, nil
}

// DeleteWebAuthnCredential deletes a WebAuthn credential
func (r *AuthRepository) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	// First get the credential to find the user
	credential, err := r.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	// Construct the key
	pk := "USER#" + credential.UserID
	sk := "WEBAUTHN_CRED#" + credentialID

	// Use BaseRepository Delete method
	err = r.Delete(ctx, pk, sk)
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityWebAuthnCredential, credentialID)
	}

	r.logger.Debug("deleted WebAuthn credential",
		zap.String("credential_id", credentialID),
		zap.String("user_id", credential.UserID))

	return nil
}

// UpdateWebAuthnLastUsed updates the last used timestamp and sign count
func (r *AuthRepository) UpdateWebAuthnLastUsed(ctx context.Context, credentialID string, signCount uint32) error {
	// First get the credential to find the user
	credential, err := r.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return err
	}

	// Create model with updated fields
	model := &models.WebAuthnCredential{
		ID:         credentialID,
		UserID:     credential.UserID,
		LastUsedAt: time.Now(),
		SignCount:  signCount,
		// Preserve other fields from existing credential
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.AAGUID,
		CloneWarning:    credential.CloneWarning,
		BackupEligible:  credential.BackupEligible,
		BackupState:     credential.BackupState,
		CreatedAt:       credential.CreatedAt,
		Name:            credential.Name,
	}

	// Use BaseRepository Update method
	err = r.Update(ctx, model)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityWebAuthnCredential, credentialID)
	}

	r.logger.Debug("updated WebAuthn last used",
		zap.String("credential_id", credentialID),
		zap.Uint32("sign_count", signCount))

	return nil
}

// WebAuthn Challenge Operations

// CreateWebAuthnChallenge stores a temporary WebAuthn challenge
func (r *AuthRepository) CreateWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
	// Create DynamORM model
	model := &models.WebAuthnChallenge{
		Challenge: challenge.Challenge,
		UserID:    challenge.UserID,
		SessionData: func() []byte {
			data, ok := challenge.SessionData.([]byte)
			if ok {
				return data
			}
			return nil
		}(),
		ExpiresAt: challenge.ExpiresAt,
		Type:      challenge.Type,
	}

	// Use helper method for WebAuthnChallenge creation
	err := r.createWebAuthnChallenge(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnChallenge, challenge.Challenge)
	}

	r.logger.Debug("created WebAuthn challenge",
		zap.String("challenge", challenge.Challenge),
		zap.String("user_id", challenge.UserID),
		zap.String("type", challenge.Type))

	return nil
}

// GetWebAuthnChallenge retrieves a WebAuthn challenge
func (r *AuthRepository) GetWebAuthnChallenge(ctx context.Context, challengeID string) (*storage.WebAuthnChallenge, error) {
	// Construct the key
	pk := "CHALLENGE#" + challengeID
	sk := "WEBAUTHN"

	// Use helper method for WebAuthnChallenge retrieval
	model, err := r.getWebAuthnChallenge(ctx, pk, sk)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(nil, EntityWebAuthnChallenge, challengeID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityWebAuthnChallenge, challengeID)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired challenge
		_ = r.DeleteWebAuthnChallenge(ctx, challengeID)
		return nil, ErrorHandler.HandleGetError(nil, EntityWebAuthnChallenge, challengeID)
	}

	// Convert to storage model
	result := &storage.WebAuthnChallenge{
		Challenge:   model.Challenge,
		UserID:      model.UserID,
		SessionData: model.SessionData,
		ExpiresAt:   model.ExpiresAt,
		Type:        model.Type,
	}

	r.logger.Debug("retrieved WebAuthn challenge",
		zap.String("challenge", challengeID),
		zap.String("user_id", result.UserID))

	return result, nil
}

// DeleteWebAuthnChallenge deletes a WebAuthn challenge
func (r *AuthRepository) DeleteWebAuthnChallenge(ctx context.Context, challengeID string) error {
	// Construct the key
	pk := "CHALLENGE#" + challengeID
	sk := "WEBAUTHN"

	// Use helper method for WebAuthnChallenge deletion
	err := r.deleteWebAuthnChallenge(ctx, pk, sk)
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityWebAuthnChallenge, challengeID)
	}

	r.logger.Debug("deleted WebAuthn challenge", zap.String("challenge", challengeID))
	return nil
}

// Wallet Credential Operations

// StoreWalletCredential stores a wallet credential linked to a user
func (r *AuthRepository) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	// Create DynamORM model
	model := &models.WalletCredential{
		Username: credential.Username,
		Address:  credential.Address,
		ChainID:  credential.ChainID,
		Type:     credential.Type,
		ENS:      credential.ENS,
		LinkedAt: credential.LinkedAt,
		LastUsed: credential.LastUsed,
	}

	// Set default timestamps
	if model.LinkedAt.IsZero() {
		model.LinkedAt = time.Now()
	}
	if model.LastUsed.IsZero() {
		model.LastUsed = model.LinkedAt
	}

	// Use helper method for WalletCredential creation
	err := r.createWalletCredential(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityWalletCredential, credential.Username)
	}

	// Also create a reverse index for wallet->user lookup
	// Normalize address
	normalizedAddress := strings.ToLower(credential.Address)
	reverseIndexPK := fmt.Sprintf("WALLET#%s#%s", credential.Type, normalizedAddress)
	reverseIndexSK := "USER#" + credential.Username

	// Create reverse index with retry logic for production reliability
	// Convert storage.WalletCredential to models.WalletCredential for the index
	walletModel := &models.WalletCredential{
		Username: credential.Username,
		Address:  normalizedAddress,
		ChainID:  credential.ChainID,
		Type:     credential.Type,
		ENS:      credential.ENS,
		LinkedAt: credential.LinkedAt,
		LastUsed: credential.LastUsed,
	}
	walletModel.PK = reverseIndexPK
	walletModel.SK = reverseIndexSK

	err = r.createReverseIndexWithRetry(ctx, walletModel, credential.Username, normalizedAddress)
	if err != nil {
		r.logger.Error("failed to create wallet reverse index after retries",
			zap.String("username", credential.Username),
			zap.String("address", normalizedAddress),
			zap.Error(err))

		// In production, this should trigger an alert/metric
		// The main credential is still saved, but reverse lookups may fail
		// Consider implementing a background job to retry failed reverse indexes
	}

	r.logger.Debug("stored wallet credential",
		zap.String("username", credential.Username),
		zap.String("address", credential.Address),
		zap.String("type", credential.Type))

	return nil
}

// GetWalletByAddress retrieves wallet credentials by address
func (r *AuthRepository) GetWalletByAddress(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	// Normalize address
	normalizedAddress := strings.ToLower(address)

	// First try to find via reverse index
	reverseIndexPK := fmt.Sprintf("WALLET#%s#%s", walletType, normalizedAddress)
	reverseIndexSK := "USER#"

	// Query using the index model
	type IndexRecord struct {
		PK       string `dynamorm:"pk"`
		SK       string `dynamorm:"sk"`
		Type     string `json:"Type"`
		Username string `json:"Username"`
	}

	var indexRecords []IndexRecord
	err := r.db.WithContext(ctx).Model(&IndexRecord{}).
		Where("PK", "=", reverseIndexPK).
		Where("SK", "BEGINS_WITH", reverseIndexSK).
		All(&indexRecords)

	if err != nil || len(indexRecords) == 0 {
		// Fallback to scanning (less efficient)
		var modelList []models.WalletCredential
		err = r.db.WithContext(ctx).Model(&models.WalletCredential{}).
			Where("address", "=", address).
			Where("type", "=", walletType).
			Scan(&modelList)

		if err != nil || len(modelList) == 0 {
			return nil, ErrorHandler.HandleGetError(nil, EntityWalletCredential, address)
		}

		model := modelList[0]
		return &storage.WalletCredential{
			Username: model.Username,
			Address:  model.Address,
			ChainID:  model.ChainID,
			Type:     model.Type,
			ENS:      model.ENS,
			LinkedAt: model.LinkedAt,
			LastUsed: model.LastUsed,
		}, nil
	}

	// Get username from index
	username := indexRecords[0].Username

	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrorHandler.HandleGetError(nil, EntityWalletCredential, address)
	}

	// Now get the actual wallet credential
	pk := "USER#" + username
	sk := "WALLET#" + normalizedAddress

	var model models.WalletCredential
	err = r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		return nil, ErrorHandler.HandleGetError(err, EntityWalletCredential, username)
	}

	return &storage.WalletCredential{
		Username: model.Username,
		Address:  model.Address,
		ChainID:  model.ChainID,
		Type:     model.Type,
		ENS:      model.ENS,
		LinkedAt: model.LinkedAt,
		LastUsed: model.LastUsed,
	}, nil
}

// GetUserWallets retrieves all wallet credentials for a user
func (r *AuthRepository) GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	// Construct the key prefix
	pk := "USER#" + username

	// Use queryWalletCredentials helper method
	modelList, _, err := r.queryWalletCredentials(ctx, pk, "WALLET#", 0, "")
	if err != nil {
		r.logger.Error("failed to get user wallets", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityWalletCredential, username)
	}

	// Convert to storage models
	results := make([]*storage.WalletCredential, len(modelList))
	for i, model := range modelList {
		results[i] = &storage.WalletCredential{
			Username: model.Username,
			Address:  model.Address,
			ChainID:  model.ChainID,
			Type:     model.Type,
			ENS:      model.ENS,
			LinkedAt: model.LinkedAt,
			LastUsed: model.LastUsed,
		}
	}

	r.logger.Debug("retrieved user wallets",
		zap.String("username", username),
		zap.Int("count", len(results)))

	return results, nil
}

// DeleteWalletCredential deletes a wallet credential
func (r *AuthRepository) DeleteWalletCredential(ctx context.Context, username, address string) error {
	// Normalize address
	normalizedAddress := strings.ToLower(address)

	// Construct the key
	pk := "USER#" + username
	sk := "WALLET#" + normalizedAddress

	// Delete the main item
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete wallet credential", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityWalletCredential, username)
	}

	// Also try to delete the reverse index (best effort)
	// We need to get the wallet type first
	var model models.WalletCredential
	_ = r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if model.Type != "" {
		reverseIndexPK := fmt.Sprintf("WALLET#%s#%s", model.Type, normalizedAddress)
		reverseIndexSK := "USER#" + username

		// Delete using the index model
		type IndexRecord struct {
			PK string `dynamorm:"pk"`
			SK string `dynamorm:"sk"`
		}
		_ = r.db.WithContext(ctx).Model(&IndexRecord{}).
			Where("PK", "=", reverseIndexPK).
			Where("SK", "=", reverseIndexSK).
			Delete()
	}

	r.logger.Debug("deleted wallet credential",
		zap.String("username", username),
		zap.String("address", address))

	return nil
}

// Wallet Challenge Operations

// StoreWalletChallenge stores a temporary wallet authentication challenge
func (r *AuthRepository) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	// Create DynamORM model
	model := &models.WalletChallenge{
		ID:        challenge.ID,
		Username:  challenge.Username,
		Address:   challenge.Address,
		ChainID:   challenge.ChainID,
		Nonce:     challenge.Nonce,
		Message:   challenge.Message,
		IssuedAt:  challenge.IssuedAt,
		ExpiresAt: challenge.ExpiresAt,
	}

	// Set default timestamps
	if model.IssuedAt.IsZero() {
		model.IssuedAt = time.Now()
	}

	// Use helper method for WalletChallenge creation
	err := r.createWalletChallenge(ctx, model)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, EntityWalletChallenge, challenge.ID)
	}

	r.logger.Debug("stored wallet challenge",
		zap.String("challenge_id", challenge.ID),
		zap.String("address", challenge.Address))

	return nil
}

// GetWalletChallenge retrieves a wallet challenge by ID
func (r *AuthRepository) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	// Construct the key
	pk := "WALLET_CHALLENGE#" + challengeID
	sk := SKChallenge

	// Use helper method for WalletChallenge retrieval
	model, err := r.getWalletChallenge(ctx, pk, sk)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(nil, EntityWalletChallenge, challengeID)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityWalletChallenge, challengeID)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired challenge
		_ = r.DeleteWalletChallenge(ctx, challengeID)
		return nil, ErrorHandler.HandleGetError(nil, EntityWalletChallenge, challengeID)
	}

	// Convert to storage model
	result := &storage.WalletChallenge{
		ID:        model.ID,
		Username:  model.Username,
		Address:   model.Address,
		ChainID:   model.ChainID,
		Nonce:     model.Nonce,
		Message:   model.Message,
		IssuedAt:  model.IssuedAt,
		ExpiresAt: model.ExpiresAt,
	}

	r.logger.Debug("retrieved wallet challenge",
		zap.String("challenge_id", challengeID),
		zap.String("address", result.Address))

	return result, nil
}

// DeleteWalletChallenge deletes a wallet challenge
func (r *AuthRepository) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	// Construct the key
	pk := "WALLET_CHALLENGE#" + challengeID
	sk := SKChallenge

	// Use helper method for WalletChallenge deletion
	err := r.deleteWalletChallenge(ctx, pk, sk)
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, EntityWalletChallenge, challengeID)
	}

	r.logger.Debug("deleted wallet challenge", zap.String("challenge_id", challengeID))
	return nil
}

// createReverseIndexWithRetry attempts to create a reverse index with exponential backoff
func (r *AuthRepository) createReverseIndexWithRetry(ctx context.Context, indexModel *models.WalletCredential, username, address string) error {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := r.db.WithContext(ctx).Model(indexModel).Create()
		if err == nil {
			// Success
			if attempt > 0 {
				r.logger.Info("reverse index created successfully after retry",
					zap.String("username", username),
					zap.String("address", address),
					zap.Int("attempt", attempt+1))
			}
			return nil
		}

		// Check if it's a recoverable error
		if !r.isRecoverableIndexError(err) {
			// Non-recoverable error (e.g., validation error)
			r.logger.Warn("non-recoverable reverse index error",
				zap.String("username", username),
				zap.String("address", address),
				zap.Error(err))
			return err
		}

		// If this is the last attempt, don't wait
		if attempt == maxRetries-1 {
			r.logger.Error("max retries exceeded for reverse index",
				zap.String("username", username),
				zap.String("address", address),
				zap.Int("attempts", maxRetries),
				zap.Error(err))
			return err
		}

		// Wait with exponential backoff
		delay := baseDelay * (1 << attempt) // 100ms, 200ms, 400ms
		r.logger.Debug("retrying reverse index creation",
			zap.String("username", username),
			zap.String("address", address),
			zap.Int("attempt", attempt+1),
			zap.Duration("delay", delay),
			zap.Error(err))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next retry
		}
	}

	return ErrorHandler.HandleCreateError(nil, EntityWalletCredential, "retry")
}

// isRecoverableIndexError determines if an index creation error is worth retrying
func (r *AuthRepository) isRecoverableIndexError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Recoverable errors (temporary issues)
	recoverableErrors := []string{
		"throttling",
		"timeout",
		"service unavailable",
		"internal server error",
		"provisioned throughput exceeded",
	}

	for _, recoverable := range recoverableErrors {
		if strings.Contains(strings.ToLower(errStr), recoverable) {
			return true
		}
	}

	// Non-recoverable errors (data/validation issues)
	nonRecoverableErrors := []string{
		"conditional check failed", // Item already exists
		"validation exception",
		"resource not found",
		"access denied",
	}

	for _, nonRecoverable := range nonRecoverableErrors {
		if strings.Contains(strings.ToLower(errStr), nonRecoverable) {
			return false
		}
	}

	// Default to recoverable for unknown errors (be optimistic)
	return true
}

// === TYPED HELPER METHODS FOR NON-PRIMARY MODELS ===
// These methods provide type-safe operations for models other than WebAuthnCredential

// ChallengeModel interface for any model that has UpdateKeys and can be used in challenge operations
type ChallengeModel interface {
	UpdateKeys() error
}

// createChallenge creates any challenge model using proper typing and cost tracking
func (r *AuthRepository) createChallenge(ctx context.Context, model ChallengeModel, operationSuffix, logName, identifier string) error {
	// Update keys before saving
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "Challenge", "keys")
	}

	// Track cost if available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "PutItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("AuthRepository_%s_%d", operationSuffix, time.Now().UnixNano()),
		}
		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn(fmt.Sprintf("failed to track DynamoDB %s operation cost", logName),
					zap.String("identifier", identifier),
					zap.Error(trackErr))
			}
		}()
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to create %s", logName),
			zap.Error(err),
			zap.String("identifier", identifier))
		return err
	}

	return nil
}

// createWebAuthnChallenge creates a WebAuthnChallenge using the generic helper
func (r *AuthRepository) createWebAuthnChallenge(ctx context.Context, model *models.WebAuthnChallenge) error {
	return r.createChallenge(ctx, model, "createChallenge", "WebAuthn challenge", model.Challenge)
}

// getChallengeModel retrieves any challenge model by keys with cost tracking
func (r *AuthRepository) getChallengeModel(ctx context.Context, pk, sk, operationSuffix, logName string, modelPtr interface{}) error {
	// Track cost if available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "GetItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  1,
			ConsumedWriteUnits: 0,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("AuthRepository_%s_%d", operationSuffix, time.Now().UnixNano()),
		}
		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn(fmt.Sprintf("failed to track DynamoDB %s operation cost", logName),
					zap.String("pk", pk),
					zap.Error(trackErr))
			}
		}()
	}

	err := r.db.WithContext(ctx).Model(modelPtr).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(modelPtr)

	if err != nil {
		return err
	}

	return nil
}

// getWebAuthnChallenge retrieves a WebAuthnChallenge using the generic helper
func (r *AuthRepository) getWebAuthnChallenge(ctx context.Context, pk, sk string) (*models.WebAuthnChallenge, error) {
	var model models.WebAuthnChallenge
	err := r.getChallengeModel(ctx, pk, sk, "getChallenge", "get challenge", &model)
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// deleteChallengeModel deletes any challenge model by keys with cost tracking
func (r *AuthRepository) deleteChallengeModel(ctx context.Context, pk, sk, operationSuffix, logName string, modelPtr interface{}) error {
	// Track cost if available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "DeleteItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("AuthRepository_%s_%d", operationSuffix, time.Now().UnixNano()),
		}
		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn(fmt.Sprintf("failed to track DynamoDB %s operation cost", logName),
					zap.String("pk", pk),
					zap.Error(trackErr))
			}
		}()
	}

	err := r.db.WithContext(ctx).Model(modelPtr).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to delete %s", logName),
			zap.Error(err),
			zap.String("pk", pk),
			zap.String("sk", sk))
		return err
	}

	return nil
}

// deleteWebAuthnChallenge deletes a WebAuthnChallenge using the generic helper
func (r *AuthRepository) deleteWebAuthnChallenge(ctx context.Context, pk, sk string) error {
	return r.deleteChallengeModel(ctx, pk, sk, "deleteChallenge", "delete challenge", &models.WebAuthnChallenge{})
}

// createWalletCredential creates a WalletCredential using proper typing
func (r *AuthRepository) createWalletCredential(ctx context.Context, model *models.WalletCredential) error {
	// Update keys before saving
	if err := model.UpdateKeys(); err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityWalletCredential, "keys")
	}

	// Track cost if available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "PutItem",
			TableName:          r.tableName,
			ConsumedReadUnits:  0,
			ConsumedWriteUnits: 1,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("AuthRepository_createWallet_%d", time.Now().UnixNano()),
		}
		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB create wallet operation cost",
					zap.String("username", model.Username),
					zap.Error(trackErr))
			}
		}()
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create wallet credential",
			zap.Error(err),
			zap.String("username", model.Username),
			zap.String("address", model.Address))
		return err
	}

	return nil
}

// createWalletChallenge creates a WalletChallenge using the generic helper
func (r *AuthRepository) createWalletChallenge(ctx context.Context, model *models.WalletChallenge) error {
	return r.createChallenge(ctx, model, "createWalletChallenge", "wallet challenge", model.ID)
}

const (
	walletCredentialDefaultLimit = 25
	walletCredentialMaxLimit     = 100
)

func clampWalletCredentialLimit(limit int) int {
	if limit <= 0 {
		return walletCredentialDefaultLimit
	}
	if limit > walletCredentialMaxLimit {
		return walletCredentialMaxLimit
	}
	return limit
}

// queryWalletCredentials queries wallet credentials with SK prefix
func (r *AuthRepository) queryWalletCredentials(ctx context.Context, pk, skPrefix string, limit int, cursor string) ([]models.WalletCredential, string, error) {
	// Track cost if available
	if r.costService != nil {
		operation := cost.DynamoOperation{
			Type:               "Query",
			TableName:          r.tableName,
			ConsumedReadUnits:  1, // Will be updated based on actual results
			ConsumedWriteUnits: 0,
			ItemCount:          1,
			Timestamp:          time.Now(),
			OperationID:        fmt.Sprintf("AuthRepository_queryWallets_%d", time.Now().UnixNano()),
		}
		defer func() {
			if trackErr := r.costService.TrackDynamoOperation(ctx, operation); trackErr != nil {
				r.logger.Warn("failed to track DynamoDB wallet query operation cost",
					zap.String("pk", pk),
					zap.Error(trackErr))
			}
		}()
	}

	var modelList []models.WalletCredential
	safeLimit := clampWalletCredentialLimit(limit)

	query := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", skPrefix).
		OrderBy("SK", "ASC").
		Limit(safeLimit + 1)

	if cursor != "" {
		query = query.Where("SK", ">", cursor)
	}

	err := query.All(&modelList)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(modelList) > safeLimit {
		nextCursor = modelList[safeLimit-1].SK
		modelList = modelList[:safeLimit]
	}

	return modelList, nextCursor, nil
}

// getWalletChallenge retrieves a WalletChallenge using the generic helper
func (r *AuthRepository) getWalletChallenge(ctx context.Context, pk, sk string) (*models.WalletChallenge, error) {
	var model models.WalletChallenge
	err := r.getChallengeModel(ctx, pk, sk, "getWalletChallenge", "get wallet challenge", &model)
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// deleteWalletChallenge deletes a WalletChallenge using the generic helper
func (r *AuthRepository) deleteWalletChallenge(ctx context.Context, pk, sk string) error {
	return r.deleteChallengeModel(ctx, pk, sk, "deleteWalletChallenge", "delete wallet challenge", &models.WalletChallenge{})
}
