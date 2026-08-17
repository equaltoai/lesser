package repositories

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// ===== WebAuthn Methods =====
// This file contains WebAuthn-related methods for the AccountRepository

func webAuthnCredentialModelFromStorage(credential *storage.WebAuthnCredential) *models.WebAuthnCredential {
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

	return model
}

func webAuthnCredentialFromModel(model *models.WebAuthnCredential) *storage.WebAuthnCredential {
	if model == nil {
		return nil
	}

	return &storage.WebAuthnCredential{
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

func ensureWebAuthnCredentialCanonicalKeys(model *models.WebAuthnCredential) {
	if model == nil {
		return
	}

	model.PK = fmt.Sprintf("USER#%s", model.UserID)
	model.SK = fmt.Sprintf("WEBAUTHN_CRED#%s", model.ID)
	model.GSI1PK = fmt.Sprintf("WEBAUTHN_CREDENTIAL#%s", model.ID)
	model.GSI1SK = fmt.Sprintf("USER#%s", model.UserID)
}

func createWebAuthnCredentialRecord(ctx context.Context, db core.DB, credential *storage.WebAuthnCredential) error {
	model := webAuthnCredentialModelFromStorage(credential)
	if err := model.BeforeCreate(); err != nil {
		return err
	}

	return db.WithContext(ctx).Model(model).Create()
}

func lookupWebAuthnCredentialModel(ctx context.Context, db core.DB, credentialID string) (*models.WebAuthnCredential, error) {
	var model models.WebAuthnCredential

	err := db.WithContext(ctx).Model(&model).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("WEBAUTHN_CREDENTIAL#%s", credentialID)).
		Limit(1).
		First(&model)

	if err != nil {
		return nil, err
	}

	ensureWebAuthnCredentialCanonicalKeys(&model)
	return &model, nil
}

func listWebAuthnCredentialModels(ctx context.Context, db core.DB, userID string) ([]models.WebAuthnCredential, error) {
	var credentials []models.WebAuthnCredential

	err := db.WithContext(ctx).Model(&models.WebAuthnCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", userID)).
		Where("SK", "BEGINS_WITH", "WEBAUTHN_CRED#").
		All(&credentials)

	if err != nil {
		return nil, err
	}

	for i := range credentials {
		ensureWebAuthnCredentialCanonicalKeys(&credentials[i])
	}

	return credentials, nil
}

func deleteWebAuthnCredentialRecord(ctx context.Context, db core.DB, credentialID string) error {
	credential, err := lookupWebAuthnCredentialModel(ctx, db, credentialID)
	if err != nil {
		return err
	}

	return db.WithContext(ctx).Model(&models.WebAuthnCredential{}).
		Where("PK", "=", credential.PK).
		Where("SK", "=", credential.SK).
		Delete()
}

func newWebAuthnCredentialKeyModel(username, credentialID string) *models.WebAuthnCredential {
	model := &models.WebAuthnCredential{
		ID:     credentialID,
		UserID: username,
	}
	ensureWebAuthnCredentialCanonicalKeys(model)
	return model
}

func newWalletCredentialKeyModel(username, address string) *models.WalletCredential {
	model := &models.WalletCredential{
		Username: username,
		Address:  strings.ToLower(address),
	}
	_ = model.UpdateKeys()
	return model
}

func newWalletIndexKeyModel(username, address, walletType string) *models.WalletIndex {
	index := &models.WalletIndex{}
	index.UpdateKeys(walletType, address, username)
	return index
}

func updateWebAuthnCredentialLastUsedRecord(ctx context.Context, db core.DB, credentialID string, signCount uint32) error {
	credential, err := lookupWebAuthnCredentialModel(ctx, db, credentialID)
	if err != nil {
		return err
	}

	return persistWebAuthnCredentialAuthenticationState(
		ctx,
		db,
		credential,
		signCount,
		credential.CloneWarning,
		credential.BackupState,
		time.Now().UTC(),
	)
}

func updateWebAuthnCredentialNameRecord(ctx context.Context, db core.DB, credentialID string, name string) error {
	credential, err := lookupWebAuthnCredentialModel(ctx, db, credentialID)
	if err != nil {
		return err
	}

	update, err := keyedUpdateBuilder(ctx, db, credential)
	if err != nil {
		return err
	}

	return update.
		Set("Name", name).
		Execute()
}

func persistWebAuthnCredentialAuthenticationState(
	ctx context.Context,
	db core.DB,
	credential *models.WebAuthnCredential,
	signCount uint32,
	cloneWarning bool,
	backupState bool,
	lastUsedAt time.Time,
) error {
	if credential == nil {
		return storage.ErrInvalidInput
	}

	ensureWebAuthnCredentialCanonicalKeys(credential)
	if lastUsedAt.IsZero() {
		lastUsedAt = time.Now().UTC()
	} else {
		lastUsedAt = lastUsedAt.UTC()
	}

	update, err := keyedUpdateBuilder(ctx, db, credential)
	if err != nil {
		return err
	}

	return update.
		Set("SignCount", signCount).
		Set("CloneWarning", cloneWarning).
		Set("BackupState", backupState).
		Set("LastUsedAt", lastUsedAt).
		Execute()
}

func updateWebAuthnCredentialAuthenticationStateRecord(
	ctx context.Context,
	db core.DB,
	credentialID string,
	signCount uint32,
	cloneWarning bool,
	backupState bool,
	lastUsedAt time.Time,
) error {
	credential, err := lookupWebAuthnCredentialModel(ctx, db, credentialID)
	if err != nil {
		return err
	}

	return persistWebAuthnCredentialAuthenticationState(
		ctx,
		db,
		credential,
		signCount,
		cloneWarning,
		backupState,
		lastUsedAt,
	)
}

// CreateWebAuthnCredential creates a new WebAuthn credential for a user
func (r *AccountRepository) CreateWebAuthnCredential(ctx context.Context, credential *storage.WebAuthnCredential) error {
	err := createWebAuthnCredentialRecord(ctx, r.db, credential)
	if err != nil {
		r.logger.Error("failed to create WebAuthn credential",
			zap.String("id", credential.ID),
			zap.String("userID", credential.UserID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnCredential, credential.ID)
	}

	r.logger.Info("created WebAuthn credential",
		zap.String("id", credential.ID),
		zap.String("userID", credential.UserID))

	return nil
}

// GetWebAuthnCredential retrieves a WebAuthn credential by ID
func (r *AccountRepository) GetWebAuthnCredential(ctx context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	model, err := lookupWebAuthnCredentialModel(ctx, r.db, credentialID)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleNotFound(err, EntityWebAuthnCredential, credentialID)
		}
		r.logger.Error("failed to get WebAuthn credential",
			zap.String("id", credentialID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityWebAuthnCredential, credentialID)
	}

	return webAuthnCredentialFromModel(model), nil
}

// GetUserWebAuthnCredentials retrieves all WebAuthn credentials for a user
func (r *AccountRepository) GetUserWebAuthnCredentials(ctx context.Context, userID string) ([]*storage.WebAuthnCredential, error) {
	credentials, err := listWebAuthnCredentialModels(ctx, r.db, userID)
	if err != nil {
		r.logger.Error("failed to get user WebAuthn credentials",
			zap.String("userID", userID),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "webauthn credentials")
	}

	result := make([]*storage.WebAuthnCredential, len(credentials))
	for i := range credentials {
		result[i] = webAuthnCredentialFromModel(&credentials[i])
	}

	return result, nil
}

// DeleteWebAuthnCredential removes a WebAuthn credential
func (r *AccountRepository) DeleteWebAuthnCredential(ctx context.Context, credentialID string) error {
	err := deleteWebAuthnCredentialRecord(ctx, r.db, credentialID)

	if err != nil && !dynamormerrors.IsNotFound(err) {
		r.logger.Error("failed to delete WebAuthn credential",
			zap.String("id", credentialID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityWebAuthnCredential, credentialID)
	}

	r.logger.Info("deleted WebAuthn credential",
		zap.String("id", credentialID))

	return nil
}

// DeleteWebAuthnCredentialConditionedOnSurvivor removes a passkey only if the
// caller-provided surviving authenticator still exists at commit time.
func (r *AccountRepository) DeleteWebAuthnCredentialConditionedOnSurvivor(
	ctx context.Context,
	username string,
	credentialID string,
	survivingPasskeyID string,
	survivingWalletAddress string,
) error {
	target := newWebAuthnCredentialKeyModel(username, credentialID)
	survivor, err := guardedAuthenticatorModel(username, survivingPasskeyID, survivingWalletAddress)
	if err != nil {
		return err
	}

	err = r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(target, tabletheory.IfExists())
		tx.ConditionCheck(survivor, tabletheory.IfExists())
		return nil
	})
	if err != nil {
		r.logAuthenticatorTransactionError("guarded WebAuthn delete failed", err,
			zap.String("username", username),
			zap.String("credential_id", credentialID),
			zap.String("surviving_passkey_id", survivingPasskeyID),
			zap.String("surviving_wallet_address", survivingWalletAddress),
		)
		if dynamormerrors.IsConditionFailed(err) {
			return err
		}
		return ErrorHandler.HandleDeleteError(err, EntityWebAuthnCredential, credentialID)
	}

	r.logger.Info("deleted WebAuthn credential with survivor guard",
		zap.String("username", username),
		zap.String("credential_id", credentialID))
	return nil
}

// UpdateWebAuthnLastUsed updates the last used timestamp and sign count for a credential
func (r *AccountRepository) UpdateWebAuthnLastUsed(ctx context.Context, credentialID string, signCount uint32) error {
	err := updateWebAuthnCredentialLastUsedRecord(ctx, r.db, credentialID, signCount)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return ErrorHandler.HandleNotFound(err, EntityWebAuthnCredential, credentialID)
		}
		return ErrorHandler.HandleGetError(err, EntityWebAuthnCredential, credentialID)
	}

	return nil
}

// UpdateWebAuthnCredentialName persists a renamed WebAuthn credential without mutating its last-used timestamp.
func (r *AccountRepository) UpdateWebAuthnCredentialName(ctx context.Context, credentialID string, name string) error {
	err := updateWebAuthnCredentialNameRecord(ctx, r.db, credentialID, name)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return ErrorHandler.HandleNotFound(err, EntityWebAuthnCredential, credentialID)
		}
		return ErrorHandler.HandleUpdateError(err, EntityWebAuthnCredential, credentialID)
	}

	return nil
}

// UpdateWebAuthnAuthenticationState persists the authentication-derived WebAuthn credential state.
func (r *AccountRepository) UpdateWebAuthnAuthenticationState(
	ctx context.Context,
	credentialID string,
	signCount uint32,
	cloneWarning bool,
	backupState bool,
	lastUsedAt time.Time,
) error {
	err := updateWebAuthnCredentialAuthenticationStateRecord(
		ctx,
		r.db,
		credentialID,
		signCount,
		cloneWarning,
		backupState,
		lastUsedAt,
	)
	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return ErrorHandler.HandleNotFound(err, EntityWebAuthnCredential, credentialID)
		}
		return ErrorHandler.HandleUpdateError(err, EntityWebAuthnCredential, credentialID)
	}

	return nil
}

// CreateWebAuthnChallenge creates a WebAuthn challenge for authentication
func (r *AccountRepository) CreateWebAuthnChallenge(ctx context.Context, challenge *storage.WebAuthnChallenge) error {
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
		Type:      challenge.Type,
		ExpiresAt: challenge.ExpiresAt,
	}

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to create WebAuthn challenge",
			zap.String("challenge", challenge.Challenge),
			zap.String("userID", challenge.UserID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWebAuthnChallenge, challenge.Challenge)
	}

	return nil
}

// GetWebAuthnChallenge retrieves a WebAuthn challenge by challenge value
func (r *AccountRepository) GetWebAuthnChallenge(ctx context.Context, challenge string) (*storage.WebAuthnChallenge, error) {
	var model models.WebAuthnChallenge

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("CHALLENGE#%s", challenge)).
		Where("SK", "=", "WEBAUTHN").
		First(&model)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleNotFound(err, EntityWebAuthnChallenge, challenge)
		}
		r.logger.Error("failed to get WebAuthn challenge",
			zap.String("challenge", challenge),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityWebAuthnChallenge, challenge)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Remove expired challenge
		if err := r.DeleteWebAuthnChallenge(ctx, challenge); err != nil {
			r.logger.Warn("failed to delete expired WebAuthn challenge",
				zap.String("challenge", challenge),
				zap.Error(err))
		}
		return nil, ErrorHandler.HandleNotFound(stdErrors.New("challenge expired"), EntityWebAuthnChallenge, challenge)
	}

	return &storage.WebAuthnChallenge{
		Challenge:   model.Challenge,
		UserID:      model.UserID,
		SessionData: model.SessionData,
		Type:        model.Type,
		ExpiresAt:   model.ExpiresAt,
	}, nil
}

// DeleteWebAuthnChallenge removes a WebAuthn challenge
func (r *AccountRepository) DeleteWebAuthnChallenge(ctx context.Context, challenge string) error {
	err := r.db.WithContext(ctx).Model(&models.WebAuthnChallenge{}).
		Where("PK", "=", fmt.Sprintf("CHALLENGE#%s", challenge)).
		Where("SK", "=", "WEBAUTHN").
		Delete()

	if err != nil && !dynamormerrors.IsNotFound(err) {
		r.logger.Error("failed to delete WebAuthn challenge",
			zap.String("challenge", challenge),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityWebAuthnChallenge, challenge)
	}

	return nil
}

// StoreWalletCredential stores a wallet-based credential
func (r *AccountRepository) StoreWalletCredential(ctx context.Context, credential *storage.WalletCredential) error {
	// Validate required fields
	if credential.Username == "" {
		r.logger.Error("username is required for wallet credential",
			zap.String("address", credential.Address))
		return ErrorHandler.HandleCreateError(stdErrors.New("username is required"), EntityWalletCredential, credential.Address)
	}
	if credential.Address == "" {
		r.logger.Error("address is required for wallet credential",
			zap.String("username", credential.Username))
		return ErrorHandler.HandleCreateError(stdErrors.New("address is required"), EntityWalletCredential, credential.Username)
	}

	model := &models.WalletCredential{
		Username: credential.Username,
		Address:  credential.Address,
		ChainID:  credential.ChainID,
		Type:     credential.Type,
		ENS:      credential.ENS,
		LinkedAt: credential.LinkedAt,
		LastUsed: credential.LastUsed,
	}

	// BeforeCreate will set keys and timestamps
	if err := model.BeforeCreate(); err != nil {
		r.logger.Error("failed to prepare wallet credential for creation",
			zap.String("username", credential.Username),
			zap.String("address", credential.Address),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWalletCredential, credential.Address)
	}

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		if dynamormerrors.IsConditionFailed(err) {
			return ErrorHandler.HandleCreateError(stdErrors.New("already exists"), EntityWalletCredential, credential.Address)
		}
		r.logger.Error("failed to store wallet credential",
			zap.String("address", credential.Address),
			zap.String("username", credential.Username),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWalletCredential, credential.Address)
	}

	// Create the reverse index entry for wallet->user lookup
	// This allows querying all users for a given wallet
	walletType := credential.Type
	if walletType == "" {
		walletType = "ethereum" // Default to ethereum if not specified
	}
	index := &models.WalletIndex{
		Username:   credential.Username,
		WalletType: walletType,
		Address:    credential.Address,
	}

	// BeforeCreate will set keys
	if err := index.BeforeCreate(); err != nil {
		r.logger.Warn("failed to prepare wallet index for creation",
			zap.String("address", credential.Address),
			zap.String("username", credential.Username),
			zap.Error(err))
		// Non-fatal - continue
	} else {
		err = r.db.WithContext(ctx).Model(index).Create()
		if err != nil {
			// Log error but don't fail the operation - index is for optimization
			// The credential is already stored, we can rebuild indexes if needed
			r.logger.Warn("failed to create wallet index entry",
				zap.String("address", credential.Address),
				zap.String("username", credential.Username),
				zap.String("walletType", walletType),
				zap.Error(err))
			// Continue - the main credential is stored successfully
		}
	}

	return nil
}

// GetWalletCredential retrieves a wallet credential by address
func (r *AccountRepository) GetWalletCredential(ctx context.Context, address string) (*storage.WalletCredential, error) {
	// Normalize address
	address = strings.ToLower(address)

	// Query the index to find ANY user with this wallet
	var indexes []models.WalletIndex

	// Try common wallet types
	walletTypes := []string{"ethereum", "solana", "bitcoin"}
	for _, wType := range walletTypes {
		err := r.db.WithContext(ctx).Model(&models.WalletIndex{}).
			Where("PK", "=", fmt.Sprintf("WALLET#%s#%s", wType, address)).
			Limit(1).
			All(&indexes)

		if err == nil && len(indexes) > 0 {
			break
		}
	}

	if len(indexes) == 0 {
		return nil, ErrorHandler.HandleNotFound(storage.ErrNotFound, EntityWalletCredential, address)
	}

	// Get the first user's wallet credential
	username := indexes[0].Username
	var model models.WalletCredential
	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		First(&model)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleNotFound(err, EntityWalletCredential, address)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityWalletCredential, address)
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

// GetUserWalletCredentials retrieves all wallet credentials for a user
func (r *AccountRepository) GetUserWalletCredentials(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	var wallets []models.WalletCredential

	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "BEGINS_WITH", "WALLET#").
		All(&wallets)

	if err != nil {
		r.logger.Error("failed to get user wallet credentials",
			zap.String("username", username),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityUser, "wallet credentials")
	}

	result := make([]*storage.WalletCredential, len(wallets))
	for i, wallet := range wallets {
		result[i] = &storage.WalletCredential{
			Username: wallet.Username,
			Address:  wallet.Address,
			ChainID:  wallet.ChainID,
			Type:     wallet.Type,
			ENS:      wallet.ENS,
			LinkedAt: wallet.LinkedAt,
			LastUsed: wallet.LastUsed,
		}
	}

	return result, nil
}

// GetAllUsersForWallet retrieves all users linked to a specific wallet
func (r *AccountRepository) GetAllUsersForWallet(ctx context.Context, walletType, address string) ([]string, error) {
	address = strings.ToLower(address)

	var indexes []models.WalletIndex
	err := r.db.WithContext(ctx).Model(&models.WalletIndex{}).
		Where("PK", "=", fmt.Sprintf("WALLET#%s#%s", walletType, address)).
		All(&indexes)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return []string{}, nil // Return empty slice, not an error
		}
		r.logger.Error("failed to query wallet index",
			zap.String("walletType", walletType),
			zap.String("address", address),
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, EntityWalletCredential, "wallet index lookup")
	}

	usernames := make([]string, len(indexes))
	for i, idx := range indexes {
		usernames[i] = idx.Username
	}

	return usernames, nil
}

// DeleteWalletCredentialByAddress removes a wallet credential by username and address
func (r *AccountRepository) DeleteWalletCredentialByAddress(ctx context.Context, username, address string) error {
	address = strings.ToLower(address)

	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		Delete()

	if err != nil && !dynamormerrors.IsNotFound(err) {
		r.logger.Error("failed to delete wallet credential",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityWalletCredential, address)
	}

	return nil
}

// ===== Wallet Challenge Operations =====

// StoreWalletChallenge stores a wallet-based authentication challenge
func (r *AccountRepository) StoreWalletChallenge(ctx context.Context, challenge *storage.WalletChallenge) error {
	model := &models.WalletChallenge{
		ID:                    challenge.ID,
		Address:               challenge.Address,
		ChainID:               challenge.ChainID,
		Nonce:                 challenge.Nonce,
		Message:               challenge.Message,
		IssuedAt:              challenge.IssuedAt,
		ExpiresAt:             challenge.ExpiresAt,
		Username:              challenge.Username,
		RegistrationCompleted: challenge.RegistrationCompleted,
	}

	// Update keys before creating (required for DynamoDB)
	if err := model.UpdateKeys(); err != nil {
		r.logger.Error("failed to update wallet challenge keys",
			zap.String("id", challenge.ID),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWalletChallenge, challenge.ID)
	}

	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		r.logger.Error("failed to store wallet challenge",
			zap.String("id", challenge.ID),
			zap.String("address", challenge.Address),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, EntityWalletChallenge, challenge.ID)
	}

	return nil
}

// GetWalletChallenge retrieves a wallet challenge by challenge ID
func (r *AccountRepository) GetWalletChallenge(ctx context.Context, challengeID string) (*storage.WalletChallenge, error) {
	var model models.WalletChallenge

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(&model)

	if err != nil {
		if dynamormerrors.IsNotFound(err) {
			return nil, ErrorHandler.HandleNotFound(err, EntityWalletChallenge, challengeID)
		}
		r.logger.Error("failed to get wallet challenge",
			zap.String("challengeID", challengeID),
			zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, EntityWalletChallenge, challengeID)
	}

	// Check if expired
	if time.Now().After(model.ExpiresAt) {
		// Remove expired challenge
		if err := r.DeleteWalletChallenge(ctx, challengeID); err != nil {
			r.logger.Warn("failed to delete expired wallet challenge",
				zap.String("challengeID", challengeID),
				zap.Error(err))
		}
		return nil, ErrorHandler.HandleNotFound(stdErrors.New("challenge expired"), EntityWalletChallenge, challengeID)
	}

	return &storage.WalletChallenge{
		ID:                    model.ID,
		Address:               model.Address,
		ChainID:               model.ChainID,
		Nonce:                 model.Nonce,
		Message:               model.Message,
		IssuedAt:              model.IssuedAt,
		ExpiresAt:             model.ExpiresAt,
		Username:              model.Username,
		Used:                  model.Used,
		Spent:                 model.Spent,
		RegistrationCompleted: model.RegistrationCompleted,
	}, nil
}

// DeleteWalletChallenge removes a wallet challenge
func (r *AccountRepository) DeleteWalletChallenge(ctx context.Context, challengeID string) error {
	err := r.db.WithContext(ctx).Model(&models.WalletChallenge{}).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		Delete()

	if err != nil && !dynamormerrors.IsNotFound(err) {
		r.logger.Error("failed to delete wallet challenge",
			zap.String("challengeID", challengeID),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityWalletChallenge, challengeID)
	}

	return nil
}

// MarkWalletChallengeUsed marks a challenge as used (first verification)
func (r *AccountRepository) MarkWalletChallengeUsed(ctx context.Context, challengeID string) error {
	var model models.WalletChallenge

	// Get the challenge
	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(&model)

	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityWalletChallenge, challengeID)
	}

	// Mark as used
	model.Used = true

	// Update
	err = r.db.WithContext(ctx).Model(&model).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityWalletChallenge, challengeID)
	}

	r.logger.Debug("marked wallet challenge as used",
		zap.String("challengeID", challengeID))

	return nil
}

// MarkWalletChallengeRegistrationCompleted marks a verified challenge as bound to a completed account registration.
func (r *AccountRepository) MarkWalletChallengeRegistrationCompleted(ctx context.Context, challengeID string) error {
	var model models.WalletChallenge

	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(&model)

	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityWalletChallenge, challengeID)
	}

	model.RegistrationCompleted = true

	err = r.db.WithContext(ctx).Model(&model).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityWalletChallenge, challengeID)
	}

	r.logger.Debug("marked wallet challenge registration completed",
		zap.String("challengeID", challengeID))

	return nil
}

func (r *AccountRepository) setWalletChallengeSpent(ctx context.Context, challengeID string, spent bool) error {
	var model models.WalletChallenge

	// Get the challenge
	err := r.db.WithContext(ctx).Model(&model).
		Where("PK", "=", fmt.Sprintf("WALLET_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(&model)

	if err != nil {
		return ErrorHandler.HandleGetError(err, EntityWalletChallenge, challengeID)
	}

	model.Spent = spent

	err = r.db.WithContext(ctx).Model(&model).Update()
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, EntityWalletChallenge, challengeID)
	}

	return nil
}

// MarkWalletChallengeSpent marks a challenge as spent (second verification)
func (r *AccountRepository) MarkWalletChallengeSpent(ctx context.Context, challengeID string) error {
	if err := r.setWalletChallengeSpent(ctx, challengeID, true); err != nil {
		return err
	}

	r.logger.Debug("marked wallet challenge as spent",
		zap.String("challengeID", challengeID))

	return nil
}

// ResetWalletChallengeSpent clears the spent flag so a completed registration
// proof can be retried after downstream work is rolled back.
func (r *AccountRepository) ResetWalletChallengeSpent(ctx context.Context, challengeID string) error {
	if err := r.setWalletChallengeSpent(ctx, challengeID, false); err != nil {
		return err
	}

	r.logger.Debug("reset wallet challenge spent flag",
		zap.String("challengeID", challengeID))

	return nil
}

// GetWalletByAddress retrieves a wallet credential by address and type
// This uses the index to find any user with this wallet, then filters by type if specified
func (r *AccountRepository) GetWalletByAddress(ctx context.Context, walletType, address string) (*storage.WalletCredential, error) {
	// Use GetWalletCredential which properly queries via the index
	wallet, err := r.GetWalletCredential(ctx, address)
	if err != nil {
		return nil, err
	}

	// Filter by wallet type if specified
	if walletType != "" && wallet.Type != walletType {
		return nil, ErrorHandler.HandleNotFound(stdErrors.New("type mismatch"), EntityWalletCredential, address)
	}

	return wallet, nil
}

// GetUserWallets retrieves all wallet credentials for a user
func (r *AccountRepository) GetUserWallets(ctx context.Context, username string) ([]*storage.WalletCredential, error) {
	return r.GetUserWalletCredentials(ctx, username)
}

// DeleteWalletCredential removes a wallet credential by username and address
func (r *AccountRepository) DeleteWalletCredential(ctx context.Context, username, address string) error {
	// Normalize address
	address = strings.ToLower(address)

	// Delete the main credential
	err := r.db.WithContext(ctx).Model(&models.WalletCredential{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Where("SK", "=", fmt.Sprintf("WALLET#%s", address)).
		Delete()

	if err != nil && !dynamormerrors.IsNotFound(err) {
		r.logger.Error("failed to delete wallet credential",
			zap.String("username", username),
			zap.String("address", address),
			zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, EntityWalletCredential, address)
	}

	// Also delete the index entries for all wallet types
	walletTypes := []string{"ethereum", "solana", "bitcoin"}
	for _, wType := range walletTypes {
		indexErr := r.db.WithContext(ctx).Model(&models.WalletIndex{}).
			Where("PK", "=", fmt.Sprintf("WALLET#%s#%s", wType, address)).
			Where("SK", "=", fmt.Sprintf("USER#%s", username)).
			Delete()

		if indexErr != nil && !dynamormerrors.IsNotFound(indexErr) {
			r.logger.Warn("failed to delete wallet index entry",
				zap.String("username", username),
				zap.String("address", address),
				zap.String("walletType", wType),
				zap.Error(indexErr))
			// Non-fatal - continue
		}
	}

	return nil
}

// DeleteWalletCredentialConditionedOnSurvivor removes a wallet only if the
// caller-provided surviving authenticator still exists at commit time.
func (r *AccountRepository) DeleteWalletCredentialConditionedOnSurvivor(
	ctx context.Context,
	username, address, walletType string,
	survivingPasskeyID string,
	survivingWalletAddress string,
) error {
	address = strings.ToLower(address)
	target := newWalletCredentialKeyModel(username, address)
	survivor, err := guardedAuthenticatorModel(username, survivingPasskeyID, survivingWalletAddress)
	if err != nil {
		return err
	}

	err = r.transactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(target, tabletheory.IfExists())
		tx.ConditionCheck(survivor, tabletheory.IfExists())
		return nil
	})
	if err != nil {
		r.logAuthenticatorTransactionError("guarded wallet delete failed", err,
			zap.String("username", username),
			zap.String("address", address),
			zap.String("surviving_passkey_id", survivingPasskeyID),
			zap.String("surviving_wallet_address", survivingWalletAddress),
		)
		if dynamormerrors.IsConditionFailed(err) {
			return err
		}
		return ErrorHandler.HandleDeleteError(err, EntityWalletCredential, address)
	}

	r.deleteWalletIndexEntriesBestEffort(ctx, username, address, walletType)

	r.logger.Info("deleted wallet credential with survivor guard",
		zap.String("username", username),
		zap.String("address", address),
		zap.String("wallet_type", walletType))
	return nil
}

func guardedAuthenticatorModel(username, survivingPasskeyID, survivingWalletAddress string) (any, error) {
	if strings.TrimSpace(survivingPasskeyID) != "" {
		return newWebAuthnCredentialKeyModel(username, survivingPasskeyID), nil
	}
	if strings.TrimSpace(survivingWalletAddress) != "" {
		return newWalletCredentialKeyModel(username, survivingWalletAddress), nil
	}
	return nil, storage.ErrInvalidInput
}

func (r *AccountRepository) deleteWalletIndexEntriesBestEffort(ctx context.Context, username, address, walletType string) {
	walletTypes := []string{"ethereum", "solana", "bitcoin"}
	if strings.TrimSpace(walletType) != "" {
		walletTypes = []string{walletType}
	}

	for _, candidateType := range walletTypes {
		indexErr := r.db.WithContext(ctx).Model(newWalletIndexKeyModel(username, address, candidateType)).
			Where("PK", "=", fmt.Sprintf("WALLET#%s#%s", candidateType, strings.ToLower(address))).
			Where("SK", "=", fmt.Sprintf("USER#%s", username)).
			Delete()

		if indexErr != nil && !dynamormerrors.IsNotFound(indexErr) {
			r.logger.Warn("failed to delete wallet index entry after guarded wallet delete",
				zap.String("username", username),
				zap.String("address", address),
				zap.String("walletType", candidateType),
				zap.Error(indexErr))
		}
	}
}

func (r *AccountRepository) logAuthenticatorTransactionError(message string, err error, fields ...zap.Field) {
	var txErr *dynamormerrors.TransactionError
	if stdErrors.As(err, &txErr) {
		fields = append(fields,
			zap.Int("transaction_op_index", txErr.OperationIndex),
			zap.String("transaction_operation", txErr.Operation),
			zap.String("transaction_reason", txErr.Reason),
		)
	}
	r.logger.Warn(message, append(fields, zap.Error(err))...)
}
