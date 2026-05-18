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
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	dynamormCore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type secretsClient interface {
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	UpdateSecret(ctx context.Context, params *secretsmanager.UpdateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.UpdateSecretOutput, error)
}

type userCreator interface {
	CreateUser(ctx context.Context, user *storage.User) error
}

type userRepositoryFactory interface {
	User() userCreator
}

type repositoryFactoryAdapter struct {
	userRepo userCreator
}

func (r repositoryFactoryAdapter) User() userCreator {
	return r.userRepo
}

var (
	mustInitializeLambdaFn    = common.MustInitializeLambda
	getAppConfigFn            = config.Get
	loadAWSConfigFn           = awsconfig.LoadDefaultConfig
	newSecretsManagerClientFn = func(cfg aws.Config) secretsClient { return secretsmanager.NewFromConfig(cfg) }
	generateVAPIDKeysFn       = generateVAPIDKeys
	generateSecurePasswordFn  = generateSecurePassword
	getDynamormClientFn       = theorydb.GetClient
	newRepositoryFactoryFn    = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (userRepositoryFactory, error) {
		repos, err := factory.NewRepositoryFactory(db, tableName, logger)
		if err != nil {
			return nil, err
		}
		return repositoryFactoryAdapter{userRepo: repos.User()}, nil
	}
	hashPasswordFn = auth.HashPassword
)

func main() {
	if err := runInitDeploy(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runInitDeploy(ctx context.Context, args []string) error {
	lambdaCtx := mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "init-deploy",
		LambdaType:  common.LambdaTypeBasic,
		Version:     "1.0.0",
	})

	// Get domain from centralized config or command line
	appCfg := getAppConfigFn()
	domain := appCfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		if err := common.ValidateSliceNotEmpty("args", args); err == nil {
			domain = args[0]
		} else {
			return pkgErrors.NewAppError(pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "DOMAIN configuration or command line argument required")
		}
	}

	fmt.Printf("🚀 Initializing deployment for domain: %s\n", domain)

	// Load AWS config
	cfg, err := loadAWSConfigFn(ctx)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to load AWS config")
	}

	// Initialize secrets manager
	secretsClient := newSecretsManagerClientFn(cfg)

	// Generate VAPID keys
	fmt.Printf("🔑 Generating VAPID keys...\n")
	publicKey, privateKey, err := generateVAPIDKeysFn()
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to generate VAPID keys")
	}

	// Store VAPID keys in AWS Secrets Manager
	vapidSecretName := fmt.Sprintf("lesser/%s/vapid-keys", domain)
	vapidSecret := fmt.Sprintf(`{"public_key":"%s","private_key":"%s"}`, publicKey, privateKey)

	if err := storeSecret(ctx, secretsClient, vapidSecretName, vapidSecret); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to store VAPID keys")
	}

	fmt.Printf("✅ VAPID keys generated and stored in AWS Secrets Manager\n")
	fmt.Printf("   Secret Name: %s\n", vapidSecretName)
	fmt.Printf("   Public Key: %s\n", publicKey)

	// Generate admin password
	adminPassword, err := generateSecurePasswordFn(32)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to generate admin password")
	}

	// Create admin account
	fmt.Printf("👤 Creating admin account...\n")
	adminUsername := domain

	// Initialize storage - use the deps Config
	if strings.TrimSpace(lambdaCtx.Config.JWTSecret) == "" && strings.TrimSpace(lambdaCtx.Config.JWTSecretARN) == "" {
		jwtSecret, err := generateSecurePasswordFn(64)
		if err != nil {
			return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to generate JWT secret")
		}

		// Store JWT secret
		jwtSecretName := fmt.Sprintf("lesser/%s/jwt-secret", domain)
		if err := storeSecret(ctx, secretsClient, jwtSecretName, jwtSecret); err != nil {
			return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to store JWT secret")
		}
		fmt.Printf("🔐 JWT secret generated and stored: %s\n", jwtSecretName)
	}

	// Initialize storage independently to avoid import cycles
	db, err := getDynamormClientFn(ctx)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "failed to initialize DynamORM database")
	}

	// Create repository factory
	repos, err := newRepositoryFactoryFn(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to create repository factory")
	}

	// Create admin user
	hashedPassword, err := hashPasswordFn(adminPassword)
	if err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to hash admin password")
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
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to create admin user")
	}

	// Store admin credentials
	adminSecretName := fmt.Sprintf("lesser/%s/admin-credentials", domain)
	adminSecret := fmt.Sprintf(`{"username":"%s","password":"%s","email":"%s"}`,
		adminUsername, adminPassword, adminUser.Email)

	if err := storeSecret(ctx, secretsClient, adminSecretName, adminSecret); err != nil {
		return pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to store admin credentials")
	}

	fmt.Printf("✅ Admin account created successfully\n")
	fmt.Printf("   Username: %s\n", adminUsername)
	fmt.Printf("   Email: %s\n", adminUser.Email)
	fmt.Printf("   Credentials stored in: %s\n", adminSecretName)

	fmt.Printf("\n🎉 Initial deployment setup complete!\n")
	fmt.Printf("\n📝 Environment Variables for Deployment:\n")
	fmt.Printf("   DOMAIN=%s\n", domain)
	fmt.Printf("   DYNAMO_TABLE_NAME=%s\n", lambdaCtx.Config.DynamoTableName)
	fmt.Printf("   VAPID_SECRET_NAME=%s\n", vapidSecretName)
	fmt.Printf("   JWT_SECRET_NAME=%s\n", fmt.Sprintf("lesser/%s/jwt-secret", domain))
	fmt.Printf("   ADMIN_SECRET_NAME=%s\n", adminSecretName)

	fmt.Printf("\n⚠️  IMPORTANT: Save these credentials securely!\n")
	fmt.Printf("   Admin Username: %s\n", adminUsername)
	fmt.Printf("   Admin Password: retrieve from AWS Secrets Manager secret %s\n", adminSecretName)

	return nil
}

// generateVAPIDKeys generates ECDSA P-256 keys for VAPID
func generateVAPIDKeys() (string, string, error) {
	var entropyProbe [1]byte
	if _, err := io.ReadFull(rand.Reader, entropyProbe[:]); err != nil {
		return "", "", pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to read entropy")
	}

	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to generate private key")
	}

	// Encode private key to PEM
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to marshal private key")
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Convert ECDSA key to ECDH and get public key bytes
	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return "", "", pkgErrors.WrapError(err, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to convert to ECDH key")
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
func storeSecret(ctx context.Context, client secretsClient, secretName, secretValue string) error {
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
			return pkgErrors.WrapError(updateErr, pkgErrors.CodeInternal, pkgErrors.CategoryLambda, "Failed to create or update secret")
		}
	}

	return nil
}
