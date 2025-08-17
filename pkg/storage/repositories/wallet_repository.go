package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
	"github.com/equaltoai/lesser/pkg/common"
)

// WalletRepository implements the wallet authentication storage operations
type WalletRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewWalletRepository creates a new wallet repository
func NewWalletRepository(db core.DB, tableName string, logger *zap.Logger) *WalletRepository {
	return &WalletRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Wallet challenge operations

// StoreWalletChallenge stores a temporary wallet authentication challenge
func (r *WalletRepository) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	// Convert storage.WalletChallenge to models.WalletChallenge
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

	// Update keys
	model.UpdateKeys()

	// Create the challenge
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet challenge",
			zap.String("challengeID", challenge.ID),
			zap.Error(err))
		return fmt.Errorf("failed to store wallet challenge: %w", err)
	}

	return nil
}

// GetWalletChallenge retrieves a wallet challenge by ID
func (r *WalletRepository) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	var model models.WalletChallenge

	// Query by primary key
	err := r.db.WithContext(ctx).Model(&models.WalletChallenge{}).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			// Legacy implementation returns error for not found
			return nil, fmt.Errorf("wallet challenge not found")
		}
		r.logger.Error("failed to get wallet challenge",
			zap.String("challengeID", challengeID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get wallet challenge: %w", err)
	}

	// Convert back to storage.WalletChallenge
	challenge := &storage.WalletChallenge{
		ID:        model.ID,
		Username:  model.Username,
		Address:   model.Address,
		ChainID:   model.ChainID,
		Nonce:     model.Nonce,
		Message:   model.Message,
		IssuedAt:  model.IssuedAt,
		ExpiresAt: model.ExpiresAt,
	}

	return challenge, nil
}

// Wallet credential operations

// StoreWalletCredential stores a wallet credential linked to a user
func (r *WalletRepository) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	// Normalize address
	address := strings.ToLower(credential.Address)

	// Convert storage.WalletCredential to models.WalletCredential
	model := &models.WalletCredential{
		Username: credential.Username,
		Address:  credential.Address,
		ChainID:  credential.ChainID,
		Type:     credential.Type,
		ENS:      credential.ENS,
		LinkedAt: credential.LinkedAt,
		LastUsed: credential.LastUsed,
	}

	// Update keys
	model.UpdateKeys()

	// Create the wallet credential
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet credential",
			zap.String("username", credential.Username),
			zap.String("address", address),
			zap.Error(err))
		return fmt.Errorf("failed to store wallet credential: %w", err)
	}

	// Also create a reverse index for wallet->user lookup
	index := &models.WalletIndex{}
	index.UpdateKeys(credential.Type, address, credential.Username)

	err = r.db.WithContext(ctx).Model(index).Create()
	if err != nil {
		// Try to clean up the first item
		cleanupErr := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
			Where("PK", "=", fmt.Sprintf("USER#%s", credential.Username)).
			Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
			Delete()

		if cleanupErr != nil {
			r.logger.Warn("failed to cleanup wallet credential after index failure",
				zap.String("username", credential.Username),
				zap.String("address", address),
				zap.Error(cleanupErr))
		}

		r.logger.Error("failed to store wallet index",
			zap.String("username", credential.Username),
			zap.String("address", address),
			zap.Error(err))
		return fmt.Errorf("failed to store wallet index: %w", err)
	}

	return nil
}

// GetWalletCredential retrieves a wallet credential by wallet type and address
func (r *WalletRepository) GetWalletCredential(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	// Normalize address
	address = strings.ToLower(address)

	// First, query the index to find the username
	var indexes []models.WalletIndex
	err := r.db.WithContext(ctx).Model(&models.WalletIndex{}).
		Where("PK", "=", fmt.Sprintf("WALLET#%s#%s", walletType, address)).
		Where("SK", "begins_with", "USER#").
		Limit(1).
		All(&indexes)

	if err != nil {
		r.logger.Error("failed to query wallet index",
			zap.String("walletType", walletType),
			zap.String("address", address),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query wallet index: %w", err)
	}

	if err := common.ValidateSliceNotEmpty("indexes", indexes); err != nil {
		// Legacy implementation returns nil for not found
		return nil, nil
	}

	// Extract username from the index
	username := indexes[0].Username
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, fmt.Errorf("username not found in wallet index")
	}

	// Now get the actual wallet credential
	var model models.WalletCredential
	err = r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			// Legacy implementation returns nil for not found
			return nil, nil
		}
		r.logger.Error("failed to get wallet credential",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get wallet credential: %w", err)
	}

	// Convert back to storage.WalletCredential
	credential := &storage.WalletCredential{
		Username: model.Username,
		Address:  model.Address,
		ChainID:  model.ChainID,
		Type:     model.Type,
		ENS:      model.ENS,
		LinkedAt: model.LinkedAt,
		LastUsed: model.LastUsed,
	}

	return credential, nil
}

// DeleteWalletChallenge deletes a wallet challenge
func (r *WalletRepository) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	// Delete by primary key
	err := r.db.WithContext(ctx).Model(&models.WalletChallenge{}).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		Delete()

	if err != nil {
		r.logger.Error("failed to delete wallet challenge",
			zap.String("challengeID", challengeID),
			zap.Error(err))
		return fmt.Errorf("failed to delete wallet challenge: %w", err)
	}

	return nil
}

// GetUserWalletCredentials retrieves all wallet credentials for a user
func (r *WalletRepository) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	var walletModels []models.WalletCredential

	// Query all wallets for a user
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "begins_with", "WALLET#").
		All(&walletModels)

	if err != nil {
		r.logger.Error("failed to query user wallet credentials",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to query user wallet credentials: %w", err)
	}

	// Convert to storage.WalletCredential slice
	credentials := make([]*storage.WalletCredential, 0, len(walletModels))
	for _, model := range walletModels {
		credential := &storage.WalletCredential{
			Username: model.Username,
			Address:  model.Address,
			ChainID:  model.ChainID,
			Type:     model.Type,
			ENS:      model.ENS,
			LinkedAt: model.LinkedAt,
			LastUsed: model.LastUsed,
		}
		credentials = append(credentials, credential)
	}

	return credentials, nil
}

// DeleteWalletCredential deletes a wallet credential
func (r *WalletRepository) DeleteWalletCredential(ctx context.Context, username, address string) error {
	// Normalize address
	address = strings.ToLower(address)

	// First get the wallet to determine type for index deletion
	var model models.WalletCredential
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		First(&model)

	walletType := "ethereum" // default
	if err == nil {
		walletType = model.Type
	}

	// Delete the wallet credential
	err = r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		Delete()

	if err != nil {
		r.logger.Error("failed to delete wallet credential",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return fmt.Errorf("failed to delete wallet credential: %w", err)
	}

	// Also delete the reverse index
	err = r.db.WithContext(ctx).Model(&models.WalletIndex{}).
		Where("PK", "=", fmt.Sprintf("WALLET#%s#%s", walletType, address)).
		Where("SK", "=", fmt.Sprintf("USER#%s", username)).
		Delete()

	if err != nil {
		// Log but don't fail - index might already be gone
		r.logger.Warn("failed to delete wallet index",
			zap.String("username", username),
			zap.String("address", address),
			zap.String("walletType", walletType),
			zap.Error(err))
	}

	return nil
}

// UpdateWalletLastUsed updates the last used timestamp for a wallet
func (r *WalletRepository) UpdateWalletLastUsed(ctx context.Context, username, address string) error {
	// Normalize address
	address = strings.ToLower(address)

	// First, get the existing wallet credential
	var model models.WalletCredential
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("wallet credential not found")
		}
		r.logger.Error("failed to get wallet credential for update",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return fmt.Errorf("failed to get wallet credential: %w", err)
	}

	// Update the last_used field
	model.LastUsed = time.Now()

	// Save the updated model
	err = r.db.WithContext(ctx).Model(&model).Update()
	if err != nil {
		r.logger.Error("failed to update wallet last used",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return fmt.Errorf("failed to update wallet last used: %w", err)
	}

	return nil
}
