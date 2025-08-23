package auth

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	"go.uber.org/zap"
)

// SecretsManager interface for AWS Secrets Manager operations
type SecretsManager interface {
	StorePrivateKey(ctx context.Context, keyID, privateKeyPEM string) error
	RetrievePrivateKey(ctx context.Context, keyID string) (string, error)
	DeletePrivateKey(ctx context.Context, keyID string) error
	GenerateAndStoreKeyPair(ctx context.Context, keyID string) (publicKeyPEM, privateKeyPEM string, err error)
	RotateKey(ctx context.Context, keyID string) (publicKeyPEM, privateKeyPEM string, err error)
}

// AWSSecretsManager implements SecretsManager using AWS Secrets Manager
type AWSSecretsManager struct {
	client      *secretsmanager.Client
	logger      *zap.Logger
	keyPrefix   string
	region      string
	cache       *secretCache
	cacheTTL    time.Duration
	description string
}

// SecretValue represents the structure stored in AWS Secrets Manager
type SecretValue struct {
	PrivateKeyPEM string    `json:"private_key_pem"`
	CreatedAt     time.Time `json:"created_at"`
	KeyType       string    `json:"key_type"`
	Version       string    `json:"version"`
}

// secretCache provides in-memory caching for secrets
type secretCache struct {
	mutex   sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// SecretsManagerConfig holds configuration for the secrets manager
type SecretsManagerConfig struct {
	Region      string
	KeyPrefix   string
	CacheTTL    time.Duration
	Description string
}

// NewAWSSecretsManager creates a new AWS Secrets Manager client
func NewAWSSecretsManager(cfg SecretsManagerConfig, logger *zap.Logger) (*AWSSecretsManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Set defaults
	if err := common.ValidateRequiredParam("cfg.Region", cfg.Region); err != nil {
		cfg.Region = "us-east-1" // Default region
	}
	if err := common.ValidateRequiredParam("cfg.KeyPrefix", cfg.KeyPrefix); err != nil {
		cfg.KeyPrefix = "lesser/actor-keys"
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute // Default cache TTL
	}
	if err := common.ValidateRequiredParam("cfg.Description", cfg.Description); err != nil {
		cfg.Description = "Lesser ActivityPub actor private keys"
	}

	// Load AWS config with region
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, errors.Join(ErrAWSConfigLoad, err)
	}

	// Create Secrets Manager client
	client := secretsmanager.NewFromConfig(awsCfg)

	// Test connectivity by listing a single secret (don't fail if none exist)
	_, err = client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{
		MaxResults: aws.Int32(1),
	})
	if err != nil {
		logger.Error("failed to connect to AWS Secrets Manager",
			zap.String("region", cfg.Region),
			zap.Error(err))
		return nil, errors.Join(ErrSecretsManagerConnection, err)
	}

	logger.Info("AWS Secrets Manager client initialized",
		zap.String("region", cfg.Region),
		zap.String("key_prefix", cfg.KeyPrefix),
		zap.Duration("cache_ttl", cfg.CacheTTL))

	return &AWSSecretsManager{
		client:      client,
		logger:      logger,
		keyPrefix:   cfg.KeyPrefix,
		region:      cfg.Region,
		cacheTTL:    cfg.CacheTTL,
		description: cfg.Description,
		cache: &secretCache{
			entries: make(map[string]*cacheEntry),
		},
	}, nil
}

// StorePrivateKey stores a private key in AWS Secrets Manager
func (sm *AWSSecretsManager) StorePrivateKey(ctx context.Context, keyID, privateKeyPEM string) error {
	secretName := sm.getSecretName(keyID)

	// Validate the private key format
	if err := sm.validatePrivateKey(privateKeyPEM); err != nil {
		return errors.Join(ErrInvalidPrivateKeyFormat, err)
	}

	secretValue := SecretValue{
		PrivateKeyPEM: privateKeyPEM,
		CreatedAt:     time.Now(),
		KeyType:       "RSA",
		Version:       "1.0",
	}

	secretJSON, err := json.Marshal(secretValue)
	if err != nil {
		return errors.Join(ErrSecretValueMarshal, err)
	}

	// Try to create the secret first
	err = sm.createSecret(ctx, secretName, string(secretJSON))
	if err != nil {
		// If secret already exists, try to update it
		if sm.isSecretAlreadyExistsError(err) {
			return sm.updateSecret(ctx, secretName, string(secretJSON))
		}
		sm.logger.Error("failed to create secret",
			zap.String("secret_name", secretName),
			zap.Error(err))
		return errors.Join(ErrSecretCreation, err)
	}

	// Invalidate cache
	sm.invalidateCache(keyID)

	sm.logger.Info("private key stored in Secrets Manager",
		zap.String("key_id", keyID),
		zap.String("secret_name", secretName))

	return nil
}

// RetrievePrivateKey retrieves a private key from AWS Secrets Manager
func (sm *AWSSecretsManager) RetrievePrivateKey(ctx context.Context, keyID string) (string, error) {
	// Check cache first
	if cachedValue := sm.getFromCache(keyID); cachedValue != "" {
		return cachedValue, nil
	}

	secretName := sm.getSecretName(keyID)

	// Retrieve secret with retry
	secretValue, err := sm.getSecretWithRetry(ctx, secretName, 3)
	if err != nil {
		sm.logger.Error("failed to retrieve private key",
			zap.String("key_id", keyID),
			zap.Error(err))
		return "", errors.Join(ErrPrivateKeyRetrieval, err)
	}

	var secret SecretValue
	if err := json.Unmarshal([]byte(secretValue), &secret); err != nil {
		return "", errors.Join(ErrSecretValueUnmarshal, err)
	}

	// Validate the retrieved key
	if err := sm.validatePrivateKey(secret.PrivateKeyPEM); err != nil {
		return "", errors.Join(ErrRetrievedPrivateKeyInvalid, err)
	}

	// Cache the result
	sm.putInCache(keyID, secret.PrivateKeyPEM)

	sm.logger.Debug("private key retrieved from Secrets Manager",
		zap.String("key_id", keyID),
		zap.String("secret_name", secretName),
		zap.Time("created_at", secret.CreatedAt))

	return secret.PrivateKeyPEM, nil
}

// DeletePrivateKey deletes a private key from AWS Secrets Manager
func (sm *AWSSecretsManager) DeletePrivateKey(ctx context.Context, keyID string) error {
	secretName := sm.getSecretName(keyID)

	// Delete the secret with immediate deletion (not recoverable)
	_, err := sm.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	if err != nil {
		// Check if secret doesn't exist (not an error for our purposes)
		if sm.isSecretNotFoundError(err) {
			sm.logger.Debug("secret already deleted or doesn't exist",
				zap.String("key_id", keyID),
				zap.String("secret_name", secretName))
			return nil
		}
		sm.logger.Error("failed to delete secret",
			zap.String("secret_name", secretName),
			zap.Error(err))
		return errors.Join(ErrSecretDeletion, err)
	}

	// Invalidate cache
	sm.invalidateCache(keyID)

	sm.logger.Info("private key deleted from Secrets Manager",
		zap.String("key_id", keyID),
		zap.String("secret_name", secretName))

	return nil
}

// GenerateAndStoreKeyPair generates a new RSA key pair and stores it
func (sm *AWSSecretsManager) GenerateAndStoreKeyPair(ctx context.Context, keyID string) (publicKeyPEM, privateKeyPEM string, err error) {
	// Generate RSA key pair
	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		return "", "", errors.Join(ErrRSAKeyPairGeneration, err)
	}

	// Encode private key to PEM
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", errors.Join(ErrPrivateKeyMarshal, err)
	}

	privateKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}))

	// Encode public key to PEM
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", errors.Join(ErrPublicKeyMarshal, err)
	}

	publicKeyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}))

	// Store the private key
	if err := sm.StorePrivateKey(ctx, keyID, privateKeyPEM); err != nil {
		return "", "", errors.Join(ErrGeneratedPrivateKeyStorage, err)
	}

	sm.logger.Info("generated and stored new key pair",
		zap.String("key_id", keyID))

	return publicKeyPEM, privateKeyPEM, nil
}

// RotateKey generates a new key pair and replaces the existing one
func (sm *AWSSecretsManager) RotateKey(ctx context.Context, keyID string) (publicKeyPEM, privateKeyPEM string, err error) {
	sm.logger.Info("rotating key",
		zap.String("key_id", keyID))

	// Generate new key pair
	publicKeyPEM, privateKeyPEM, err = sm.GenerateAndStoreKeyPair(ctx, keyID)
	if err != nil {
		return "", "", errors.Join(ErrKeyPairGenerationRotation, err)
	}

	sm.logger.Info("key rotation completed",
		zap.String("key_id", keyID))

	return publicKeyPEM, privateKeyPEM, nil
}

// Helper methods

// getSecretName constructs the full secret name
func (sm *AWSSecretsManager) getSecretName(keyID string) string {
	return fmt.Sprintf("%s/%s", sm.keyPrefix, keyID)
}

// validatePrivateKey validates that the PEM string contains a valid private key
func (sm *AWSSecretsManager) validatePrivateKey(privateKeyPEM string) error {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return ErrPEMBlockDecode
	}

	// Try to parse as PKCS8 private key
	_, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try to parse as PKCS1 RSA private key
		_, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return errors.Join(ErrPrivateKeyParse, err)
		}
	}

	return nil
}

// createSecret creates a new secret
func (sm *AWSSecretsManager) createSecret(ctx context.Context, secretName, secretValue string) error {
	_, err := sm.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretString: aws.String(secretValue),
		Description:  aws.String(sm.description),
		Tags: []types.Tag{
			{
				Key:   aws.String("Service"),
				Value: aws.String("lesser"),
			},
			{
				Key:   aws.String("Component"),
				Value: aws.String("actor-keys"),
			},
			{
				Key:   aws.String("CreatedBy"),
				Value: aws.String("lesser-secrets-manager"),
			},
		},
	})
	return err
}

// updateSecret updates an existing secret
func (sm *AWSSecretsManager) updateSecret(ctx context.Context, secretName, secretValue string) error {
	_, err := sm.client.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(secretValue),
	})
	return err
}

// getSecretWithRetry retrieves a secret with retry logic
func (sm *AWSSecretsManager) getSecretWithRetry(ctx context.Context, secretName string, maxRetries int) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := sm.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
			SecretId: aws.String(secretName),
		})

		if err != nil {
			lastErr = err
			sm.logger.Warn("failed to get secret",
				zap.String("secret_name", secretName),
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			continue
		}

		if resp.SecretString == nil {
			return "", ErrSecretValueNil
		}

		return *resp.SecretString, nil
	}

	sm.logger.Error("failed to get secret after retries",
		zap.Int("max_attempts", maxRetries+1),
		zap.Error(lastErr))
	return "", errors.Join(ErrSecretRetrievalRetries, lastErr)
}

// Cache methods

// getFromCache retrieves a value from cache
func (sm *AWSSecretsManager) getFromCache(keyID string) string {
	sm.cache.mutex.RLock()
	defer sm.cache.mutex.RUnlock()

	entry, exists := sm.cache.entries[keyID]
	if !exists || time.Now().After(entry.expiresAt) {
		return ""
	}

	return entry.value
}

// putInCache stores a value in cache
func (sm *AWSSecretsManager) putInCache(keyID, value string) {
	sm.cache.mutex.Lock()
	defer sm.cache.mutex.Unlock()

	sm.cache.entries[keyID] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(sm.cacheTTL),
	}
}

// invalidateCache removes a key from cache
func (sm *AWSSecretsManager) invalidateCache(keyID string) {
	sm.cache.mutex.Lock()
	defer sm.cache.mutex.Unlock()

	delete(sm.cache.entries, keyID)
}

// CleanupCache removes expired entries from cache
func (sm *AWSSecretsManager) CleanupCache() {
	sm.cache.mutex.Lock()
	defer sm.cache.mutex.Unlock()

	now := time.Now()
	for keyID, entry := range sm.cache.entries {
		if now.After(entry.expiresAt) {
			delete(sm.cache.entries, keyID)
		}
	}
}

// Error checking methods

// isSecretAlreadyExistsError checks if the error indicates a secret already exists
func (sm *AWSSecretsManager) isSecretAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return containsError(err, "ResourceExistsException")
}

// isSecretNotFoundError checks if the error indicates a secret was not found
func (sm *AWSSecretsManager) isSecretNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return containsError(err, "ResourceNotFoundException")
}

// containsError checks if an error contains a specific AWS error code
func containsError(err error, errorCode string) bool {
	if err == nil {
		return false
	}

	// Check for AWS API error
	if awsErr, ok := err.(interface{ ErrorCode() string }); ok {
		return awsErr.ErrorCode() == errorCode
	}

	// Fallback to string matching
	return fmt.Sprintf("%v", err) == errorCode
}

// GetCacheStats returns cache statistics for monitoring
func (sm *AWSSecretsManager) GetCacheStats() map[string]interface{} {
	sm.cache.mutex.RLock()
	defer sm.cache.mutex.RUnlock()

	expired := 0
	now := time.Now()
	for _, entry := range sm.cache.entries {
		if now.After(entry.expiresAt) {
			expired++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(sm.cache.entries),
		"expired_entries": expired,
		"cache_ttl":       sm.cacheTTL.String(),
	}
}
