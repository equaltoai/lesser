package repositories

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm/marshalers"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// ActorRepositoryDeps interface for dependencies - implemented by the storage adapter
type ActorRepositoryDeps interface {
	GetFollowing(ctx context.Context, username string, limit int, cursor string) ([]string, string, error)
	GetPreference(ctx context.Context, username, key string) (any, error)
	SetPreference(ctx context.Context, username, key string, value any) error
}

// ActorRepository implements actor operations using DynamORM
type ActorRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
	deps      ActorRepositoryDeps
}

// NewActorRepository creates a new actor repository
func NewActorRepository(db core.DB, tableName string, logger *zap.Logger) *ActorRepository {
	return &ActorRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// SetDependencies sets the dependencies for cross-repository operations
func (r *ActorRepository) SetDependencies(deps ActorRepositoryDeps) {
	r.deps = deps
}

// CreateActor creates a new actor in DynamoDB
func (r *ActorRepository) CreateActor(ctx context.Context, actor *activitypub.Actor, privateKey string) error {
	// Validate actor entity using centralized validation
	if err := common.ValidateActorEntity(actor.ID, actor.PreferredUsername); err != nil {
		return err
	}

	username := actor.PreferredUsername
	numericID := common.GenerateNumericID(username)

	// Encrypt private key if encryption is available
	encryptedKey := privateKey
	if encryptor, err := getEncryptor(); err == nil {
		if encrypted, err := encryptor.Encrypt([]byte(privateKey)); err == nil {
			encryptedKey = base64.StdEncoding.EncodeToString(encrypted)
		} else {
			common.WithContext(ctx).Warn("failed to encrypt private key", zap.Error(err))
		}
	} else {
		common.WithContext(ctx).Warn("encryption not available, storing private key in plaintext", zap.Error(err))
	}

	// Create the DynamORM model
	actorModel := &models.Actor{
		Username:       username,
		Actor:          actor,
		PrivateKey:     encryptedKey,
		NumericID:      numericID,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
	}

	// Set domain for GSI3 if available
	domain := lesserconfig.Get().Domain
	if domain != "" {
		actorModel.GSI3PK = "DOMAIN#" + domain
		actorModel.GSI3SK = username
	}

	// Create the actor using DynamORM
	err := r.db.WithContext(ctx).Model(actorModel).Create()
	if err != nil {
		if errors.IsConditionFailed(err) {
			return common.ConflictError{
				Resource: "actor",
				Message:  fmt.Sprintf("actor %s already exists", username),
			}
		}
		return fmt.Errorf("failed to create actor: %w", err)
	}

	return nil
}

// GetActor retrieves an actor by username
func (r *ActorRepository) GetActor(ctx context.Context, username string) (*activitypub.Actor, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	return actorModel.Actor, nil
}

// GetActorWithMetadata retrieves an actor with metadata
func (r *ActorRepository) GetActorWithMetadata(ctx context.Context, username string) (*activitypub.Actor, *storage.ActorMetadata, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, common.ActorNotFoundError{Username: username}
		}
		return nil, nil, fmt.Errorf("failed to get actor: %w", err)
	}

	metadata := &storage.ActorMetadata{
		CreatedAt:    actorModel.CreatedAt,
		UpdatedAt:    actorModel.UpdatedAt,
		LastStatusAt: actorModel.LastStatusAt,
		Fields:       convertActorFields(actorModel.Fields),
	}

	return actorModel.Actor, metadata, nil
}

// GetActorByNumericID retrieves an actor by numeric ID
func (r *ActorRepository) GetActorByNumericID(ctx context.Context, numericID string) (*activitypub.Actor, error) {
	// First get the numeric ID mapping
	var mapping models.NumericIDMapping
	err := r.db.WithContext(ctx).Model(&models.NumericIDMapping{}).
		Where("PK", "=", "NUMERIC_ID#"+numericID).
		Where("SK", "=", "METADATA").
		First(&mapping)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("actor not found: %s", numericID)
		}
		return nil, fmt.Errorf("failed to get numeric ID mapping: %w", err)
	}

	// Now get the actual actor using the username
	return r.GetActor(ctx, mapping.Username)
}

// GetActorPrivateKey retrieves an actor's private key
func (r *ActorRepository) GetActorPrivateKey(ctx context.Context, username string) (string, error) {
	var actorModel models.Actor

	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("PrivateKey").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", common.ActorNotFoundError{Username: username}
		}
		return "", fmt.Errorf("failed to get actor private key: %w", err)
	}

	// Decrypt private key if it's encrypted
	privateKey := actorModel.PrivateKey
	if encryptor, err := getEncryptor(); err == nil {
		// Try to decode as base64 - if it fails, assume it's plaintext
		if decoded, err := base64.StdEncoding.DecodeString(privateKey); err == nil {
			if decrypted, err := encryptor.Decrypt(decoded); err == nil {
				privateKey = string(decrypted)
			} else {
				common.WithContext(ctx).Warn("failed to decrypt private key", zap.Error(err))
			}
		}
	}
	return privateKey, nil
}

// UpdateActor updates an existing actor
func (r *ActorRepository) UpdateActor(ctx context.Context, actor *activitypub.Actor) error {
	// Validate actor entity using centralized validation
	if err := common.ValidateActorEntity(actor.ID, actor.PreferredUsername); err != nil {
		return err
	}
	
	username := actor.PreferredUsername

	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update the actor data
	actorModel.Actor = actor

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor: %w", err)
	}

	return nil
}

// UpdateActorLastStatusTime updates the last status timestamp
func (r *ActorRepository) UpdateActorLastStatusTime(ctx context.Context, username string) error {
	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update last status time
	now := time.Now()
	actorModel.LastStatusAt = &now

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor last status time: %w", err)
	}

	return nil
}

// SetActorFields updates the profile fields for an actor
func (r *ActorRepository) SetActorFields(ctx context.Context, username string, fields []storage.ActorField) error {
	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Convert and update fields
	actorModel.Fields = convertStorageActorFields(fields)

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		return fmt.Errorf("failed to update actor fields: %w", err)
	}

	return nil
}

// DeleteActor deletes an actor
func (r *ActorRepository) DeleteActor(ctx context.Context, username string) error {
	// Delete the actor using DynamORM
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Delete()
	if err != nil {
		if errors.IsNotFound(err) {
			return common.ActorNotFoundError{Username: username}
		}
		return fmt.Errorf("failed to delete actor: %w", err)
	}

	return nil
}

// SearchAccounts searches for actors by username or display name
func (r *ActorRepository) SearchAccounts(ctx context.Context, query string, limit int, _ bool, _ int) ([]*activitypub.Actor, error) {
	if err := common.ValidateRequiredParam("query", query); err != nil {
		// For empty query, return recent active discoverable actors
		return r.getRecentActiveActors(ctx, limit)
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	var actors []models.Actor

	// Try username search first using GSI1
	if len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("username-search-index").
			Where("GSI1PK", "=", "USERNAME_SEARCH#"+prefix).
			Filter("GSI1SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by username: %w", err)
		}
	}

	// If no results and query could be a display name, try name search
	if len(actors) == 0 && len(normalizedQuery) >= 2 {
		prefix := normalizedQuery[:2]
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("name-search-index").
			Where("GSI2PK", "=", "NAME_SEARCH#"+prefix).
			Filter("GSI2SK", "BEGINS_WITH", normalizedQuery).
			Limit(limit).
			All(&actors)
		if err != nil {
			return nil, fmt.Errorf("failed to search actors by name: %w", err)
		}
	}

	// Convert to activitypub.Actor slice
	result := make([]*activitypub.Actor, 0, len(actors))
	for _, actor := range actors {
		if actor.Actor != nil {
			result = append(result, actor.Actor)
		}
	}

	return result, nil
}

// GetSearchSuggestions returns search suggestions for autocomplete
func (r *ActorRepository) GetSearchSuggestions(ctx context.Context, prefix string) ([]storage.SearchSuggestion, error) {
	if len(prefix) < 2 {
		return []storage.SearchSuggestion{}, nil
	}

	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	prefixKey := normalizedPrefix[:2]

	var actors []models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Index("username-search-index").
		Where("GSI1PK", "=", "USERNAME_SEARCH#"+prefixKey).
		Filter("GSI1SK", "BEGINS_WITH", normalizedPrefix).
		Limit(10).
		All(&actors)
	if err != nil {
		return nil, fmt.Errorf("failed to get search suggestions: %w", err)
	}

	suggestions := make([]storage.SearchSuggestion, 0, len(actors))
	for _, actor := range actors {
		suggestions = append(suggestions, storage.SearchSuggestion{
			Type:  "account",
			Value: actor.Username,
			Score: 100, // Could be based on follower count or activity
		})
	}

	return suggestions, nil
}

// Helper functions

// convertActorFields converts models.ActorField to storage.ActorField
func convertActorFields(fields []models.ActorField) []storage.ActorField {
	result := make([]storage.ActorField, len(fields))
	for i, field := range fields {
		result[i] = storage.ActorField{
			Name:  field.Name,
			Value: field.Value,
			VerifiedAt: func() time.Time {
				if field.VerifiedAt != nil {
					return *field.VerifiedAt
				}
				return time.Time{}
			}(),
		}
	}
	return result
}

// convertStorageActorFields converts storage.ActorField to models.ActorField
func convertStorageActorFields(fields []storage.ActorField) []models.ActorField {
	result := make([]models.ActorField, len(fields))
	for i, field := range fields {
		result[i] = models.ActorField{
			Name:  field.Name,
			Value: field.Value,
			VerifiedAt: func() *time.Time {
				if !field.VerifiedAt.IsZero() {
					return &field.VerifiedAt
				}
				return nil
			}(),
		}
	}
	return result
}

// KMSEncryptor implements envelope encryption using AWS KMS
type KMSEncryptor struct {
	kmsClient    *kms.Client
	keyID        string
	mutex        sync.RWMutex
	dataKeyCache map[string]*dataKeyCacheEntry
	logger       *zap.Logger
}

// dataKeyCacheEntry represents a cached data key with TTL
type dataKeyCacheEntry struct {
	key       []byte
	encrypted []byte
	expiresAt time.Time
}

// NewKMSEncryptor creates a new KMS encryptor with data key caching
func NewKMSEncryptor(keyID string) (*KMSEncryptor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Load AWS config
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create KMS client
	kmsClient := kms.NewFromConfig(cfg)

	// Verify key exists and access
	_, err = kmsClient.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(keyID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to access KMS key %s: %w", keyID, err)
	}

	return &KMSEncryptor{
		kmsClient:    kmsClient,
		keyID:        keyID,
		dataKeyCache: make(map[string]*dataKeyCacheEntry),
		logger:       zap.L().Named("kms-encryptor"),
	}, nil
}

// Encrypt encrypts plaintext using envelope encryption
func (e *KMSEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate or get cached data key
	dataKey, encryptedDataKey, err := e.getDataKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get data key: %w", err)
	}
	defer e.clearKey(dataKey) // Clear from memory

	// Encrypt data locally with AES-GCM
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Format: [encryptedDataKeyLength(4 bytes)][encryptedDataKey][ciphertext]
	result := make([]byte, 4+len(encryptedDataKey)+len(ciphertext))

	// Store encrypted data key length (big endian)
	result[0] = byte(len(encryptedDataKey) >> 24)
	result[1] = byte(len(encryptedDataKey) >> 16)
	result[2] = byte(len(encryptedDataKey) >> 8)
	result[3] = byte(len(encryptedDataKey))

	// Store encrypted data key
	copy(result[4:4+len(encryptedDataKey)], encryptedDataKey)

	// Store ciphertext
	copy(result[4+len(encryptedDataKey):], ciphertext)

	return result, nil
}

// Decrypt decrypts ciphertext using envelope encryption
func (e *KMSEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(ciphertext) < 4 {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract encrypted data key length
	encryptedKeyLen := int(ciphertext[0])<<24 | int(ciphertext[1])<<16 | int(ciphertext[2])<<8 | int(ciphertext[3])
	if len(ciphertext) < 4+encryptedKeyLen {
		return nil, fmt.Errorf("invalid ciphertext format")
	}

	// Extract encrypted data key and encrypted data
	encryptedDataKey := ciphertext[4 : 4+encryptedKeyLen]
	encryptedData := ciphertext[4+encryptedKeyLen:]

	// Decrypt data key with KMS (with retry)
	dataKey, err := e.decryptDataKeyWithRetry(ctx, encryptedDataKey, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data key: %w", err)
	}
	defer e.clearKey(dataKey) // Clear from memory

	// Decrypt data locally with AES-GCM
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(encryptedData) < nonceSize {
		return nil, fmt.Errorf("encrypted data too short")
	}

	nonce, encryptedData := encryptedData[:nonceSize], encryptedData[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// getDataKey gets or generates a data encryption key with caching
func (e *KMSEncryptor) getDataKey(ctx context.Context) ([]byte, []byte, error) {
	cacheKey := "default" // For simplicity, using single cache key

	// Check cache first
	e.mutex.RLock()
	if entry, exists := e.dataKeyCache[cacheKey]; exists && time.Now().Before(entry.expiresAt) {
		// Return cached key copy
		keyCopy := make([]byte, len(entry.key))
		copy(keyCopy, entry.key)
		encryptedCopy := make([]byte, len(entry.encrypted))
		copy(encryptedCopy, entry.encrypted)
		e.mutex.RUnlock()
		return keyCopy, encryptedCopy, nil
	}
	e.mutex.RUnlock()

	// Generate new data key with retry
	key, encrypted, err := e.generateDataKeyWithRetry(ctx, 3)
	if err != nil {
		return nil, nil, err
	}

	// Cache the key (5 minute TTL)
	e.mutex.Lock()
	e.dataKeyCache[cacheKey] = &dataKeyCacheEntry{
		key:       key,
		encrypted: encrypted,
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	e.mutex.Unlock()

	// Return copies
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	encryptedCopy := make([]byte, len(encrypted))
	copy(encryptedCopy, encrypted)

	return keyCopy, encryptedCopy, nil
}

// generateDataKeyWithRetry generates a new data key with retry logic
func (e *KMSEncryptor) generateDataKeyWithRetry(ctx context.Context, maxRetries int) ([]byte, []byte, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := e.kmsClient.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
			KeyId:   aws.String(e.keyID),
			KeySpec: "AES_256",
			EncryptionContext: map[string]string{
				"service":   "lesser",
				"component": "actor-private-key",
				"version":   "1.0",
			},
		})

		if err != nil {
			lastErr = err
			e.logger.Warn("KMS GenerateDataKey failed",
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		return resp.Plaintext, resp.CiphertextBlob, nil
	}

	return nil, nil, fmt.Errorf("failed to generate data key after %d attempts: %w", maxRetries+1, lastErr)
}

// decryptDataKeyWithRetry decrypts a data key with retry logic
func (e *KMSEncryptor) decryptDataKeyWithRetry(ctx context.Context, encryptedKey []byte, maxRetries int) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := e.kmsClient.Decrypt(ctx, &kms.DecryptInput{
			CiphertextBlob: encryptedKey,
			EncryptionContext: map[string]string{
				"service":   "lesser",
				"component": "actor-private-key",
				"version":   "1.0",
			},
		})

		if err != nil {
			lastErr = err
			e.logger.Warn("KMS Decrypt failed",
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		return resp.Plaintext, nil
	}

	return nil, fmt.Errorf("failed to decrypt data key after %d attempts: %w", maxRetries+1, lastErr)
}

// clearKey securely clears encryption key from memory
func (e *KMSEncryptor) clearKey(key []byte) {
	for i := range key {
		key[i] = 0
	}
}

// CleanupCache removes expired entries from the data key cache
func (e *KMSEncryptor) CleanupCache() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	now := time.Now()
	for key, entry := range e.dataKeyCache {
		if now.After(entry.expiresAt) {
			// Clear sensitive data before removal
			e.clearKey(entry.key)
			e.clearKey(entry.encrypted)
			delete(e.dataKeyCache, key)
		}
	}
}

// getEncryptor returns an encryptor for private key encryption
// Prioritizes KMS, falls back to AES encryption gracefully
func getEncryptor() (marshalers.Encryptor, error) {
	// First check for KMS
	if kmsKeyID := lesserconfig.Get().KMSKeyID; kmsKeyID != "" {
		kmsEncryptor, err := NewKMSEncryptor(kmsKeyID)
		if err != nil {
			// Log KMS error but continue to AES fallback
			zap.L().Warn("Failed to initialize KMS encryptor, falling back to AES",
				zap.String("keyID", kmsKeyID),
				zap.Error(err))
		} else {
			zap.L().Info("Using KMS encryption for actor private keys",
				zap.String("keyID", kmsKeyID))
			return kmsEncryptor, nil
		}
	}

	// Fallback to AES encryption
	encryptionKey := os.Getenv("DYNAMODB_ENCRYPTION_KEY")
	if err := common.ValidateRequiredParam("encryption_key", encryptionKey); err != nil {
		// Try alternative env var
		encryptionKey = os.Getenv("ACTOR_PRIVATE_KEY_ENCRYPTION")
	}

	if encryptionKey != "" {
		// Decode base64 key
		key, err := base64.StdEncoding.DecodeString(encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("invalid encryption key format: %w", err)
		}
		zap.L().Info("Using AES encryption for actor private keys")
		return marshalers.NewAESEncryptorWithKey(key)
	}

	return nil, fmt.Errorf("no encryption key available (neither KMS nor AES)")
}

// GetActorByUsername retrieves an actor by username
func (r *ActorRepository) GetActorByUsername(_ context.Context, username string) (*activitypub.Actor, error) {
	// Query for the actor
	var actorModel models.Actor

	query := r.db.Model(&actorModel).
		Where("PK = ? AND SK = ?",
			fmt.Sprintf("ACTOR#%s", username),
			"PROFILE")

	if err := query.First(&actorModel); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("actor not found")
		}
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Convert to ActivityPub actor
	return r.modelToActivityPubActor(&actorModel)
}

// modelToActivityPubActor converts a model to an ActivityPub actor
func (r *ActorRepository) modelToActivityPubActor(model *models.Actor) (*activitypub.Actor, error) {
	// The actor is stored as a JSON field in the model
	if model.Actor == nil {
		return nil, fmt.Errorf("actor data is missing")
	}

	// Return the stored actor directly
	return model.Actor, nil
}

// GetAccountSuggestions gets suggested accounts for a user based on "friends of friends" algorithm
//
//nolint:dupl // Account suggestion algorithms are shared between actor repositories
func (r *ActorRepository) GetAccountSuggestions(ctx context.Context, userID string, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetAccountSuggestions"), zap.String("user_id", userID))

	if r.deps == nil {
		log.Warn("dependencies not set, returning empty suggestions")
		return []*activitypub.Actor{}, nil
	}

	// Get user's following list
	following, err := r.getUserFollowing(ctx, userID, log)
	if err != nil {
		return r.getDiscoverableActors(ctx, limit)
	}

	// Build exclusion set
	userFollows := r.buildExclusionSet(following, userID)

	// Collect suggestion candidates from mutual connections
	candidates := r.collectSuggestionCandidates(ctx, userID, following, userFollows)

	// Score and sort candidates
	scored := r.scoreCandidates(candidates)

	// Get actor details for top suggestions
	suggestions := r.buildSuggestions(ctx, scored, limit)

	// Fill remaining slots if needed
	suggestions = r.fillRemainingSuggestions(ctx, suggestions, userFollows, limit)

	log.Info("generated account suggestions",
		zap.Int("requested_limit", limit),
		zap.Int("returned_count", len(suggestions)))

	return suggestions, nil
}

// getUserFollowing gets the list of users that the current user follows
func (r *ActorRepository) getUserFollowing(ctx context.Context, userID string, log *zap.Logger) ([]string, error) {
	following, _, err := r.deps.GetFollowing(ctx, userID, 100, "")
	if err != nil {
		log.Error("failed to get user following for suggestions", zap.Error(err))
		return nil, err
	}
	return following, nil
}

// buildExclusionSet creates a set of actor IDs to exclude from suggestions
func (r *ActorRepository) buildExclusionSet(following []string, userID string) map[string]bool {
	userFollows := make(map[string]bool)
	for _, followedID := range following {
		userFollows[followedID] = true
	}
	userFollows[userID] = true // Exclude self
	return userFollows
}

// suggestionCandidate holds information about a potential suggestion
type suggestionCandidate struct {
	actorID string
	score   int
}

// collectSuggestionCandidates collects candidates from users that the current user follows
func (r *ActorRepository) collectSuggestionCandidates(ctx context.Context, userID string, following []string, userFollows map[string]bool) map[string]int {
	candidates := make(map[string]int) // actorID -> score
	processedActors := make(map[string]bool)

	for i, followedUserID := range following {
		if i >= 20 { // Limit to prevent excessive API calls
			break
		}

		r.processMutualConnections(ctx, userID, followedUserID, userFollows, processedActors, candidates)
	}

	return candidates
}

// processMutualConnections processes mutual connections for a single followed user
func (r *ActorRepository) processMutualConnections(ctx context.Context, userID, followedUserID string, userFollows, processedActors map[string]bool, candidates map[string]int) {
	followedUsername := r.extractUsernameFromActorID(followedUserID)
	if err := common.ValidateRequiredParam("followed_username", followedUsername); err != nil {
		return
	}

	// Get who this followed user follows
	theirFollowing, _, err := r.deps.GetFollowing(ctx, followedUsername, 50, "")
	if err != nil {
		return // Skip if we can't get their following
	}

	// Score each of their follows
	for _, candidate := range theirFollowing {
		if r.shouldSkipCandidate(ctx, userID, candidate, userFollows, processedActors) {
			continue
		}

		candidates[candidate]++
		processedActors[candidate] = true
	}
}

// shouldSkipCandidate checks if a candidate should be skipped
func (r *ActorRepository) shouldSkipCandidate(ctx context.Context, userID, candidate string, userFollows, processedActors map[string]bool) bool {
	// Skip if user already follows or we've processed
	if userFollows[candidate] || processedActors[candidate] {
		return true
	}

	// Check if user has dismissed this suggestion
	return r.isSuggestionDismissed(ctx, userID, candidate)
}

// isSuggestionDismissed checks if a suggestion has been dismissed by the user
func (r *ActorRepository) isSuggestionDismissed(ctx context.Context, userID, candidate string) bool {
	dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", candidate)
	dismissed, _ := r.deps.GetPreference(ctx, userID, dismissedKey)
	if dismissed != nil {
		if isDismissed, ok := dismissed.(bool); ok && isDismissed {
			return true
		}
	}
	return false
}

// scoreCandidates converts candidates map to sorted slice
func (r *ActorRepository) scoreCandidates(candidates map[string]int) []suggestionCandidate {
	scored := make([]suggestionCandidate, 0, len(candidates))
	for actorID, score := range candidates {
		scored = append(scored, suggestionCandidate{actorID: actorID, score: score})
	}

	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return scored
}

// buildSuggestions builds actor suggestions from scored candidates
func (r *ActorRepository) buildSuggestions(ctx context.Context, scored []suggestionCandidate, limit int) []*activitypub.Actor {
	var suggestions []*activitypub.Actor

	for _, candidate := range scored {
		if len(suggestions) >= limit {
			break
		}

		actor := r.loadActorIfDiscoverable(ctx, candidate.actorID)
		if actor != nil {
			suggestions = append(suggestions, actor)
		}
	}

	return suggestions
}

// loadActorIfDiscoverable loads an actor if it's discoverable
func (r *ActorRepository) loadActorIfDiscoverable(ctx context.Context, actorID string) *activitypub.Actor {
	// Validate actor ID using centralized validation (returns nil for backward compatibility)
	if err := common.ValidateEntityID(actorID, "actor"); err != nil {
		return nil
	}
	
	username := r.extractUsernameFromActorID(actorID)
	if err := common.ValidateRequiredParam("username", username); err != nil {
		return nil
	}

	actor, err := r.GetActor(ctx, username)
	if err != nil {
		return nil
	}

	// Only suggest discoverable accounts
	if !actor.Discoverable {
		return nil
	}

	return actor
}

// fillRemainingSuggestions fills remaining slots with discoverable users
func (r *ActorRepository) fillRemainingSuggestions(ctx context.Context, suggestions []*activitypub.Actor, userFollows map[string]bool, limit int) []*activitypub.Actor {
	if len(suggestions) >= limit {
		return suggestions
	}

	remaining := limit - len(suggestions)
	discoverable, err := r.getDiscoverableActors(ctx, remaining*2) // Get more to filter
	if err != nil {
		return suggestions
	}

	for _, actor := range discoverable {
		if len(suggestions) >= limit {
			break
		}

		if !r.shouldIncludeDiscoverable(actor, suggestions, userFollows) {
			continue
		}

		suggestions = append(suggestions, actor)
	}

	return suggestions
}

// shouldIncludeDiscoverable checks if a discoverable actor should be included
func (r *ActorRepository) shouldIncludeDiscoverable(actor *activitypub.Actor, suggestions []*activitypub.Actor, userFollows map[string]bool) bool {
	// Skip if user already follows
	if userFollows[actor.ID] {
		return false
	}

	// Skip if already in suggestions
	for _, existing := range suggestions {
		if existing.ID == actor.ID {
			return false
		}
	}

	return true
}

// MigrationInfo represents account migration information
type MigrationInfo struct {
	AlsoKnownAs []string `json:"also_known_as"`
	MovedTo     string   `json:"moved_to,omitempty"`
}

// UpdateAlsoKnownAs updates the AlsoKnownAs field for an actor
func (r *ActorRepository) UpdateAlsoKnownAs(ctx context.Context, username string, alsoKnownAs []string) error {
	log := r.logger.With(
		zap.String("method", "UpdateAlsoKnownAs"),
		zap.String("username", username),
		zap.Int("also_known_as_count", len(alsoKnownAs)),
	)

	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error("actor not found for alsoKnownAs update")
			return common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get existing actor", zap.Error(err))
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update the alsoKnownAs field in the embedded actor
	if actorModel.Actor == nil {
		log.Error("actor data is nil")
		return fmt.Errorf("actor data is missing")
	}

	actorModel.Actor.AlsoKnownAs = alsoKnownAs

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		log.Error("failed to update actor alsoKnownAs", zap.Error(err))
		return fmt.Errorf("failed to update actor alsoKnownAs: %w", err)
	}

	log.Info("updated actor alsoKnownAs successfully")
	return nil
}

// UpdateMovedTo updates the MovedTo field for an actor
func (r *ActorRepository) UpdateMovedTo(ctx context.Context, username string, movedTo string) error {
	log := r.logger.With(
		zap.String("method", "UpdateMovedTo"),
		zap.String("username", username),
		zap.String("moved_to", movedTo),
	)

	// Get existing actor first
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error("actor not found for movedTo update")
			return common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get existing actor", zap.Error(err))
		return fmt.Errorf("failed to get existing actor: %w", err)
	}

	// Update the movedTo field in the embedded actor
	if actorModel.Actor == nil {
		log.Error("actor data is nil")
		return fmt.Errorf("actor data is missing")
	}

	actorModel.Actor.MovedTo = movedTo

	// Update using DynamORM
	err = r.db.WithContext(ctx).Model(&actorModel).Update()
	if err != nil {
		log.Error("failed to update actor movedTo", zap.Error(err))
		return fmt.Errorf("failed to update actor movedTo: %w", err)
	}

	log.Info("updated actor movedTo successfully")
	return nil
}

// CheckAlsoKnownAs checks if targetActorID is in the AlsoKnownAs slice for the given username
func (r *ActorRepository) CheckAlsoKnownAs(ctx context.Context, username string, targetActorID string) (bool, error) {
	log := r.logger.With(
		zap.String("method", "CheckAlsoKnownAs"),
		zap.String("username", username),
		zap.String("target_actor_id", targetActorID),
	)

	// Get existing actor
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("Actor"). // Only select the Actor field for efficiency
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Debug("actor not found for alsoKnownAs check")
			return false, common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get actor", zap.Error(err))
		return false, fmt.Errorf("failed to get actor: %w", err)
	}

	// Check if the embedded actor data exists
	if actorModel.Actor == nil {
		log.Debug("actor data is nil")
		return false, nil
	}

	// Check if targetActorID is in the AlsoKnownAs slice
	for _, actorID := range actorModel.Actor.AlsoKnownAs {
		if actorID == targetActorID {
			log.Debug("target actor ID found in alsoKnownAs")
			return true, nil
		}
	}

	log.Debug("target actor ID not found in alsoKnownAs")
	return false, nil
}

// GetActorMigrationInfo returns migration information for an actor
func (r *ActorRepository) GetActorMigrationInfo(ctx context.Context, username string) (*MigrationInfo, error) {
	log := r.logger.With(
		zap.String("method", "GetActorMigrationInfo"),
		zap.String("username", username),
	)

	// Get existing actor
	var actorModel models.Actor
	err := r.db.WithContext(ctx).Model(&models.Actor{}).
		Where("PK", "=", "ACTOR#"+username).
		Where("SK", "=", "PROFILE").
		Select("Actor"). // Only select the Actor field for efficiency
		First(&actorModel)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error("actor not found for migration info")
			return nil, common.ActorNotFoundError{Username: username}
		}
		log.Error("failed to get actor", zap.Error(err))
		return nil, fmt.Errorf("failed to get actor: %w", err)
	}

	// Check if the embedded actor data exists
	if actorModel.Actor == nil {
		log.Error("actor data is nil")
		return nil, fmt.Errorf("actor data is missing")
	}

	// Create and return migration info
	migrationInfo := &MigrationInfo{
		AlsoKnownAs: actorModel.Actor.AlsoKnownAs,
		MovedTo:     actorModel.Actor.MovedTo,
	}

	log.Debug("retrieved actor migration info",
		zap.Int("also_known_as_count", len(migrationInfo.AlsoKnownAs)),
		zap.String("moved_to", migrationInfo.MovedTo),
	)

	return migrationInfo, nil
}

// RemoveAccountSuggestion removes an account from suggestions for a user
func (r *ActorRepository) RemoveAccountSuggestion(ctx context.Context, userID, targetID string) error {
	log := r.logger.With(
		zap.String("method", "RemoveAccountSuggestion"),
		zap.String("user_id", userID),
		zap.String("target_id", targetID),
	)

	if r.deps == nil {
		log.Error("dependencies not set")
		return fmt.Errorf("dependencies not available")
	}

	// Store the dismissed suggestion in user preferences
	// This prevents the account from being suggested again
	dismissedKey := fmt.Sprintf("dismissed_suggestion:%s", targetID)
	err := r.deps.SetPreference(ctx, userID, dismissedKey, true)
	if err != nil {
		log.Error("failed to store dismissed suggestion preference", zap.Error(err))
		return fmt.Errorf("failed to remove account suggestion: %w", err)
	}

	log.Info("account suggestion removed")

	return nil
}

// Helper functions

// getDiscoverableActors returns actors marked as discoverable
func (r *ActorRepository) getDiscoverableActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	return r.getRecentActiveActors(ctx, limit)
}

// getRecentActiveActors returns recently active actors using the activity index
func (r *ActorRepository) getRecentActiveActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "getRecentActiveActors"))

	// Query using the activity index (GSI5) to get recently active actors
	// Try multiple days to get enough results
	var allActors []models.Actor
	now := time.Now()

	for days := 0; days < 7 && len(allActors) < limit*2; days++ {
		searchDate := now.AddDate(0, 0, -days)
		dateKey := "ACTIVE#" + searchDate.Format(common.DateFormat)

		var actors []models.Actor
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("activity-index").
			Where("GSI5PK", "=", dateKey).
			OrderBy("GSI5SK", "DESC"). // Get most recent first
			Limit(limit).
			All(&actors)
		if err != nil {
			log.Warn("failed to query activity index", zap.String("date", dateKey), zap.Error(err))
			continue
		}

		allActors = append(allActors, actors...)
	}

	// If no recent activity found, fall back to popularity index
	if err := common.ValidateSliceNotEmpty("all_actors", allActors); err != nil {
		log.Debug("no recent activity found, falling back to popularity index")
		return r.getPopularActors(ctx, limit)
	}

	// Convert to activitypub.Actor slice and filter for discoverable
	result := make([]*activitypub.Actor, 0, limit)
	seen := make(map[string]bool)

	for _, actor := range allActors {
		if len(result) >= limit {
			break
		}

		// Avoid duplicates
		if seen[actor.Username] {
			continue
		}
		seen[actor.Username] = true

		if actor.Actor != nil && actor.Actor.Discoverable {
			result = append(result, actor.Actor)
		}
	}

	log.Debug("retrieved recent active actors", zap.Int("count", len(result)))
	return result, nil
}

// getPopularActors returns actors sorted by popularity (follower count)
func (r *ActorRepository) getPopularActors(ctx context.Context, limit int) ([]*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "getPopularActors"))

	// Query popularity buckets starting from highest
	buckets := []string{"10K+", "1K+", "100+", "10+", "0-9"}
	var allActors []models.Actor

	for _, bucket := range buckets {
		if len(allActors) >= limit {
			break
		}

		var actors []models.Actor
		err := r.db.WithContext(ctx).Model(&models.Actor{}).
			Index("popularity-index").
			Where("GSI4PK", "=", "ACTOR_RANK#"+bucket).
			OrderBy("GSI4SK", "DESC"). // Highest follower count first
			Limit(limit - len(allActors)).
			All(&actors)
		if err != nil {
			log.Warn("failed to query popularity index", zap.String("bucket", bucket), zap.Error(err))
			continue
		}

		allActors = append(allActors, actors...)
	}

	// Convert to activitypub.Actor slice
	result := make([]*activitypub.Actor, 0, len(allActors))
	for _, actor := range allActors {
		if len(result) >= limit {
			break
		}

		if actor.Actor != nil && actor.Actor.Discoverable {
			result = append(result, actor.Actor)
		}
	}

	log.Debug("retrieved popular actors", zap.Int("count", len(result)))
	return result, nil
}

// extractUsernameFromActorID extracts username from actor ID
func (r *ActorRepository) extractUsernameFromActorID(actorID string) string {
	// Handle local actor IDs like "https://example.com/users/username"
	parts := strings.Split(actorID, "/")
	if err := common.ValidateSliceNotEmpty("parts", parts); err == nil {
		username := parts[len(parts)-1]
		// Remove any @ prefix if present
		username = strings.TrimPrefix(username, "@")
		return username
	}

	// Handle direct username format
	return strings.TrimPrefix(actorID, "@")
}

// GetCachedRemoteActor retrieves a cached remote actor by handle
//
//nolint:dupl // Remote actor caching patterns are shared between actor repositories
func (r *ActorRepository) GetCachedRemoteActor(ctx context.Context, handle string) (*activitypub.Actor, error) {
	log := r.logger.With(zap.String("method", "GetCachedRemoteActor"), zap.String("handle", handle))

	var remoteActor models.RemoteActor

	err := r.db.WithContext(ctx).Model(&models.RemoteActor{}).
		Where("PK", "=", fmt.Sprintf("REMOTE_ACTOR#%s", handle)).
		Where("SK", "=", "PROFILE").
		First(&remoteActor)
	if err != nil {
		if errors.IsNotFound(err) {
			// Extract username from handle for error (consistent with legacy)
			username := strings.Split(handle, "@")[0]
			return nil, common.ActorNotFoundError{Username: username}
		}
		return nil, fmt.Errorf("failed to get cached remote actor: %w", err)
	}

	// Check if the cache has expired (consistent with legacy behavior)
	if time.Now().After(remoteActor.ExpiresAt) {
		log.Debug("cached remote actor expired",
			zap.Time("expired_at", remoteActor.ExpiresAt))
		// Extract username from handle for error (consistent with legacy)
		username := strings.Split(handle, "@")[0]
		return nil, common.ActorNotFoundError{Username: username}
	}

	log.Debug("retrieved cached remote actor",
		zap.String("actor_id", remoteActor.Actor.ID))

	return remoteActor.Actor, nil
}
