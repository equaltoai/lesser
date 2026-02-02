package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

// WalletRepository implements wallet authentication storage operations using enhanced patterns
type WalletRepository struct {
	*EnhancedBaseRepository[*models.WalletChallenge] // Primary model for EnhancedBaseRepository operations
	// For wallet credentials, we'll use the DB directly since we need multiple model types
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewWalletRepository creates a new wallet repository with enhanced functionality
func NewWalletRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *WalletRepository {
	// Create enhanced repository optimized for wallet operations
	enhancedRepo := NewEnhancedBaseRepository[*models.WalletChallenge](db, tableName, logger, costService, "WalletRepository", "wallet")

	// Set up enhanced services for wallet operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Wallet challenges cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for security monitoring

	return &WalletRepository{
		EnhancedBaseRepository: enhancedRepo,
		db:                     db,
		tableName:              tableName,
		logger:                 logger,
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
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the challenge
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet challenge",
			zap.String("challengeID", challenge.ID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "wallet challenge", challenge.ID)
	}

	return nil
}

// GetWalletChallenge retrieves a wallet challenge by ID using BaseRepository
func (r *WalletRepository) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	model := &models.WalletChallenge{}

	// Use BaseRepository Get method with proper key pattern
	pk := fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)
	sk := "CHALLENGE"

	err := r.Get(ctx, pk, sk, model)
	if err != nil {
		// Legacy implementation returns specific error for not found
		return nil, ErrorHandler.HandleGetError(err, "wallet challenge", challengeID)
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

// StoreWalletCredential stores a wallet credential linked to a user with cost tracking
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
	_ = model.UpdateKeys() // Ignore error as this is internal model operation

	// Create the wallet credential
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet credential",
			zap.String("username", credential.Username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "wallet credential", credential.Username)
	}

	// Track cost for wallet credential creation
	if r.GetCostService() != nil {
		if trackErr := r.TrackWrite(ctx, "PutItem", 1); trackErr != nil {
			r.logger.Warn("failed to track write cost for wallet credential", zap.Error(trackErr))
		}
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
		} else {
			// Track cleanup cost
			if r.GetCostService() != nil {
				if trackErr := r.TrackWrite(ctx, "DeleteItem", 1); trackErr != nil {
					r.logger.Warn("failed to track cleanup cost", zap.Error(trackErr))
				}
			}
		}

		r.logger.Error("failed to store wallet index",
			zap.String("username", credential.Username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "wallet index", credential.Username)
	}

	// Track cost for index creation
	if r.GetCostService() != nil {
		if trackErr := r.TrackWrite(ctx, "PutItem", 1); trackErr != nil {
			r.logger.Warn("failed to track write cost for wallet index", zap.Error(trackErr))
		}
	}

	return nil
}

// GetWalletCredential retrieves a wallet credential by wallet type and address with cost tracking
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
		return nil, ErrorHandler.HandleQueryError(err, "wallet index", "address lookup")
	}

	// Track cost for index query
	if r.GetCostService() != nil {
		readUnits := int64(len(indexes))
		if readUnits == 0 {
			readUnits = 1 // Minimum for the query operation itself
		}
		if trackErr := r.TrackRead(ctx, "Query", readUnits); trackErr != nil {
			r.logger.Warn("failed to track read cost for wallet index query", zap.Error(trackErr))
		}
	}

	if err := common.ValidateSliceNotEmpty("indexes", indexes); err != nil {
		return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityWalletCredential, address)
	}

	// Extract username from the index
	username := indexes[0].Username
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil, ErrorHandler.HandleGetError(err, "wallet index", "wallet")
	}

	// Now get the actual wallet credential
	var model models.WalletCredential
	err = r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityWalletCredential, address)
		}
		r.logger.Error("failed to get wallet credential",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityWalletCredential, username)
	}

	// Track cost for wallet credential get
	if r.GetCostService() != nil {
		if trackErr := r.TrackRead(ctx, "GetItem", 1); trackErr != nil {
			r.logger.Warn("failed to track read cost for wallet credential", zap.Error(trackErr))
		}
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

// DeleteWalletChallenge deletes a wallet challenge using BaseRepository
func (r *WalletRepository) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	// Use BaseRepository Delete method with proper key pattern
	pk := fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)
	sk := "CHALLENGE"

	return r.Delete(ctx, pk, sk)
}

// GetUserWalletCredentials retrieves all wallet credentials for a user
func (r *WalletRepository) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	var walletModels []models.WalletCredential

	// Query all wallets for a user using the database directly since we need WalletCredential model
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "begins_with", "WALLET#").
		All(&walletModels)

	if err != nil {
		r.logger.Error("failed to query user wallet credentials",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "wallet credential", "user credentials")
	}

	// Track cost using BaseRepository cost tracking
	if r.GetCostService() != nil {
		itemCount := int64(len(walletModels))
		if err := r.TrackRead(ctx, "Query", itemCount); err != nil {
			r.logger.Warn("failed to track cost for GetUserWalletCredentials", zap.Error(err))
		}
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

// DeleteWalletCredential deletes a wallet credential with cost tracking
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
		// Track read cost
		if r.GetCostService() != nil {
			if trackErr := r.TrackRead(ctx, "GetItem", 1); trackErr != nil {
				r.logger.Warn("failed to track read cost for DeleteWalletCredential", zap.Error(trackErr))
			}
		}
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
		return ErrorHandler.HandleDeleteError(err, "wallet credential", username)
	}

	// Track delete cost for wallet credential
	if r.GetCostService() != nil {
		if trackErr := r.TrackWrite(ctx, "DeleteItem", 1); trackErr != nil {
			r.logger.Warn("failed to track delete cost for wallet credential", zap.Error(trackErr))
		}
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
	} else {
		// Track delete cost for index
		if r.GetCostService() != nil {
			if trackErr := r.TrackWrite(ctx, "DeleteItem", 1); trackErr != nil {
				r.logger.Warn("failed to track delete cost for wallet index", zap.Error(trackErr))
			}
		}
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
			return ErrorHandler.HandleGetError(err, "wallet credential", username)
		}
		r.logger.Error("failed to get wallet credential for update",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleGetError(err, "wallet credential", username)
	}

	// Track read cost
	if r.GetCostService() != nil {
		if err := r.TrackRead(ctx, "GetItem", 1); err != nil {
			r.logger.Warn("failed to track read cost for UpdateWalletLastUsed", zap.Error(err))
		}
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
		return ErrorHandler.HandleUpdateError(err, "wallet credential", username)
	}

	// Track write cost
	if r.GetCostService() != nil {
		if err := r.TrackWrite(ctx, "UpdateItem", 1); err != nil {
			r.logger.Warn("failed to track write cost for UpdateWalletLastUsed", zap.Error(err))
		}
	}

	return nil
}
