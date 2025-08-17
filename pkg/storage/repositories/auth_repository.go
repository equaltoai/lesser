package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/common"
)

// AuthRepository handles authentication-related storage operations
type AuthRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewAuthRepository creates a new auth repository
func NewAuthRepository(db core.DB, tableName string, logger *zap.Logger) *AuthRepository {
	return &AuthRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
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

	// BeforeCreate will set up keys
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare WebAuthn credential: %w", err)
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create WebAuthn credential", zap.Error(err))
		return fmt.Errorf("failed to create WebAuthn credential: %w", err)
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
		return nil, fmt.Errorf("failed to get WebAuthn credential: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("modelList", modelList); err != nil {
		return nil, fmt.Errorf("WebAuthn credential not found")
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

	// Query for all WebAuthn credentials
	var modelList []models.WebAuthnCredential
	err := r.db.WithContext(ctx).Model(&models.WebAuthnCredential{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "WEBAUTHN_CRED#").
		All(&modelList)

	if err != nil {
		r.logger.Error("failed to get user WebAuthn credentials", zap.Error(err))
		return nil, fmt.Errorf("failed to get user WebAuthn credentials: %w", err)
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

	// Delete the item
	err = r.db.WithContext(ctx).Model(&models.WebAuthnCredential{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete WebAuthn credential", zap.Error(err))
		return fmt.Errorf("failed to delete WebAuthn credential: %w", err)
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

	// Construct the key
	pk := "USER#" + credential.UserID
	sk := "WEBAUTHN_CRED#" + credentialID

	// Update using DynamORM
	model := &models.WebAuthnCredential{
		PK:         pk,
		SK:         sk,
		LastUsedAt: time.Now(),
		SignCount:  signCount,
	}

	err = r.db.WithContext(ctx).Model(model).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Update()

	if err != nil {
		r.logger.Error("failed to update WebAuthn last used", zap.Error(err))
		return fmt.Errorf("failed to update WebAuthn last used: %w", err)
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

	// BeforeCreate will set up keys and TTL
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare WebAuthn challenge: %w", err)
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create WebAuthn challenge", zap.Error(err))
		return fmt.Errorf("failed to create WebAuthn challenge: %w", err)
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

	// Query for the item
	var model models.WebAuthnChallenge
	err := r.db.WithContext(ctx).Model(&models.WebAuthnChallenge{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("WebAuthn challenge not found")
		}
		r.logger.Error("failed to get WebAuthn challenge", zap.Error(err))
		return nil, fmt.Errorf("failed to get WebAuthn challenge: %w", err)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired challenge
		_ = r.DeleteWebAuthnChallenge(ctx, challengeID)
		return nil, fmt.Errorf("WebAuthn challenge expired")
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

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.WebAuthnChallenge{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete WebAuthn challenge", zap.Error(err))
		return fmt.Errorf("failed to delete WebAuthn challenge: %w", err)
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

	// BeforeCreate will set up keys
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare wallet credential: %w", err)
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet credential", zap.Error(err))
		return fmt.Errorf("failed to store wallet credential: %w", err)
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
			return nil, fmt.Errorf("wallet credential not found")
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
		return nil, fmt.Errorf("invalid wallet index record")
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
		return nil, fmt.Errorf("wallet credential not found: %w", err)
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

	// Query for all wallet credentials
	var modelList []models.WalletCredential
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", pk).
		Where("SK", "BEGINS_WITH", "WALLET#").
		All(&modelList)

	if err != nil {
		r.logger.Error("failed to get user wallets", zap.Error(err))
		return nil, fmt.Errorf("failed to get user wallets: %w", err)
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
		return fmt.Errorf("failed to delete wallet credential: %w", err)
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

	// BeforeCreate will set up keys and TTL
	if err := model.BeforeCreate(); err != nil {
		return fmt.Errorf("failed to prepare wallet challenge: %w", err)
	}

	// Create the item
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet challenge", zap.Error(err))
		return fmt.Errorf("failed to store wallet challenge: %w", err)
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
	sk := "CHALLENGE"

	// Query for the item
	var model models.WalletChallenge
	err := r.db.WithContext(ctx).Model(&models.WalletChallenge{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("wallet challenge not found")
		}
		r.logger.Error("failed to get wallet challenge", zap.Error(err))
		return nil, fmt.Errorf("failed to get wallet challenge: %w", err)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Clean up expired challenge
		_ = r.DeleteWalletChallenge(ctx, challengeID)
		return nil, fmt.Errorf("wallet challenge expired")
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
	sk := "CHALLENGE"

	// Delete the item
	err := r.db.WithContext(ctx).Model(&models.WalletChallenge{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete wallet challenge", zap.Error(err))
		return fmt.Errorf("failed to delete wallet challenge: %w", err)
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

	return fmt.Errorf("unexpected end of retry loop")
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
