// Package main implements the init-deploy Lambda function for initializing deployment configuration.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"go.uber.org/zap"
)

func main() {
	ctx := context.Background()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := logger.Sync(); err != nil {
			fmt.Printf("Failed to sync logger: %v\n", err)
		}
	}()

	// Get domain from environment or command line
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		if len(os.Args) > 1 {
			domain = os.Args[1]
		} else {
			logger.Fatal("DOMAIN environment variable or command line argument required")
		}
	}

	fmt.Printf("🚀 Initializing deployment for domain: %s\n", domain)

	// Load AWS config
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Fatal("Failed to load AWS config", zap.Error(err))
	}

	// Initialize secrets manager
	secretsClient := secretsmanager.NewFromConfig(cfg)

	// Generate VAPID keys
	fmt.Printf("🔑 Generating VAPID keys...\n")
	publicKey, privateKey, err := generateVAPIDKeys()
	if err != nil {
		logger.Fatal("Failed to generate VAPID keys", zap.Error(err))
	}

	// Store VAPID keys in AWS Secrets Manager
	vapidSecretName := fmt.Sprintf("lesser/%s/vapid-keys", domain)
	vapidSecret := fmt.Sprintf(`{"public_key":"%s","private_key":"%s"}`, publicKey, privateKey)

	if err := storeSecret(ctx, secretsClient, vapidSecretName, vapidSecret); err != nil {
		logger.Fatal("Failed to store VAPID keys", zap.Error(err))
	}

	fmt.Printf("✅ VAPID keys generated and stored in AWS Secrets Manager\n")
	fmt.Printf("   Secret Name: %s\n", vapidSecretName)
	fmt.Printf("   Public Key: %s\n", publicKey)

	// Generate admin password
	adminPassword, err := generateSecurePassword(32)
	if err != nil {
		logger.Fatal("Failed to generate admin password", zap.Error(err))
	}

	// Create admin account
	fmt.Printf("👤 Creating admin account...\n")
	adminUsername := domain

	// Initialize storage
	appConfig := &config.Config{
		Domain:    domain,
		Region:    cfg.Region,
		JWTSecret: os.Getenv("JWT_SECRET"),
	}

	if appConfig.JWTSecret == "" {
		jwtSecret, err := generateSecurePassword(64)
		if err != nil {
			logger.Fatal("Failed to generate JWT secret", zap.Error(err))
		}
		appConfig.JWTSecret = jwtSecret

		// Store JWT secret
		jwtSecretName := fmt.Sprintf("lesser/%s/jwt-secret", domain)
		if err := storeSecret(ctx, secretsClient, jwtSecretName, jwtSecret); err != nil {
			logger.Fatal("Failed to store JWT secret", zap.Error(err))
		}
		fmt.Printf("🔐 JWT secret generated and stored: %s\n", jwtSecretName)
	}

	// Initialize DynamoDB storage
	tableName := os.Getenv("DYNAMO_TABLE_NAME")
	if tableName == "" {
		tableName = fmt.Sprintf("lesser-%s", domain)
	}

	// Initialize DynamORM
	db, err := dynamorm.GetClient(ctx)
	if err != nil {
		logger.Fatal("Failed to initialize DynamORM", zap.Error(err))
	}

	// Create repository factory
	repos, err := factory.NewRepositoryFactory(db, tableName, cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create repository factory", zap.Error(err))
	}

	// Create admin user
	hashedPassword, err := auth.HashPassword(adminPassword)
	if err != nil {
		logger.Fatal("Failed to hash admin password", zap.Error(err))
	}

	adminUser := &storage.User{
		Username:     adminUsername,
		Email:        fmt.Sprintf("admin@%s", domain),
		PasswordHash: hashedPassword,
		Role:         "admin",
		Approved:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := repos.User().CreateUser(ctx, adminUser); err != nil {
		logger.Fatal("Failed to create admin user", zap.Error(err))
	}

	// Store admin credentials
	adminSecretName := fmt.Sprintf("lesser/%s/admin-credentials", domain)
	adminSecret := fmt.Sprintf(`{"username":"%s","password":"%s","email":"%s"}`,
		adminUsername, adminPassword, adminUser.Email)

	if err := storeSecret(ctx, secretsClient, adminSecretName, adminSecret); err != nil {
		logger.Fatal("Failed to store admin credentials", zap.Error(err))
	}

	fmt.Printf("✅ Admin account created successfully\n")
	fmt.Printf("   Username: %s\n", adminUsername)
	fmt.Printf("   Email: %s\n", adminUser.Email)
	fmt.Printf("   Password: %s\n", adminPassword)
	fmt.Printf("   Credentials stored in: %s\n", adminSecretName)

	fmt.Printf("\n🎉 Initial deployment setup complete!\n")
	fmt.Printf("\n📝 Environment Variables for Deployment:\n")
	fmt.Printf("   DOMAIN=%s\n", domain)
	fmt.Printf("   DYNAMO_TABLE_NAME=%s\n", tableName)
	fmt.Printf("   VAPID_SECRET_NAME=%s\n", vapidSecretName)
	fmt.Printf("   JWT_SECRET_NAME=%s\n", fmt.Sprintf("lesser/%s/jwt-secret", domain))
	fmt.Printf("   ADMIN_SECRET_NAME=%s\n", adminSecretName)

	fmt.Printf("\n⚠️  IMPORTANT: Save these credentials securely!\n")
	fmt.Printf("   Admin Username: %s\n", adminUsername)
	fmt.Printf("   Admin Password: %s\n", adminPassword)
}

// generateVAPIDKeys generates ECDSA P-256 keys for VAPID
func generateVAPIDKeys() (string, string, error) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Encode private key to PEM
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Convert ECDSA key to ECDH and get public key bytes
	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return "", "", fmt.Errorf("failed to convert to ECDH key: %w", err)
	}
	publicKeyBytes := ecdhKey.PublicKey().Bytes()

	// Encode public key to base64url (without padding)
	publicKeyB64 := base64.RawURLEncoding.EncodeToString(publicKeyBytes)

	return publicKeyB64, string(privateKeyPEM), nil
}

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[randomIndex.Int64()]
	}
	return string(b), nil
}

// storeSecret stores a secret in AWS Secrets Manager
func storeSecret(ctx context.Context, client *secretsmanager.Client, secretName, secretValue string) error {
	// Try to create the secret first
	_, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretString: aws.String(secretValue),
		Description:  aws.String("Lesser instance secret"),
	})
	if err != nil {
		// If secret already exists, update it
		_, updateErr := client.UpdateSecret(ctx, &secretsmanager.UpdateSecretInput{
			SecretId:     aws.String(secretName),
			SecretString: aws.String(secretValue),
		})
		if updateErr != nil {
			return fmt.Errorf("failed to create or update secret: %w", updateErr)
		}
	}

	return nil
}
