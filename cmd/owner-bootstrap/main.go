// Package main provides a CLI to bootstrap the initial owner/admin artifacts.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/theory-cloud/tabletheory/v3"
	theorydb "github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
)

type walletSecretPayload struct {
	Address    string `json:"address"`
	PrivateKey string `json:"private_key"`
	ChainID    int    `json:"chain_id"`
	WalletType string `json:"wallet_type"`
	Username   string `json:"username"`
	CreatedAt  string `json:"created_at"`
}

type oauthSecretPayload struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	Name         string   `json:"name"`
	Username     string   `json:"username"`
	CreatedAt    string   `json:"created_at"`
}

type jsonLog struct {
	Event   string         `json:"event"`
	Message string         `json:"message,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type ownerBootstrapArgs struct {
	environment      string
	domain           string
	tableName        string
	kmsKeyID         string
	walletSecretName string
	oauthSecretName  string
	username         string
	chainID          int
	force            bool
}

type tableTheoryAPI interface {
	Model(model any) theorydb.Query
	TransactWrite(ctx context.Context, fn func(theorydb.TransactionBuilder) error) error
	Close() error
}

type secretsManagerAPI interface {
	DescribeSecret(ctx context.Context, params *secretsmanager.DescribeSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	CreateSecret(ctx context.Context, params *secretsmanager.CreateSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	DeleteSecret(ctx context.Context, params *secretsmanager.DeleteSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

type kmsAPI interface {
	Encrypt(ctx context.Context, params *kms.EncryptInput, optFns ...func(*kms.Options)) (*kms.EncryptOutput, error)
}

var (
	loadAWSConfigFn    = awsconfig.LoadDefaultConfig
	newTableTheoryDBFn = func(cfg aws.Config) (tableTheoryAPI, error) {
		return tabletheory.New(session.Config{Region: cfg.Region, CredentialsProvider: cfg.Credentials})
	}
	newSecretsManagerClientFn = func(cfg aws.Config) secretsManagerAPI { return secretsmanager.NewFromConfig(cfg) }
	newKMSClientFn            = func(cfg aws.Config) kmsAPI { return kms.NewFromConfig(cfg) }
	exitFn                    = os.Exit

	generateEthereumWalletFn    = generateEthereumWallet
	generateRSAKeyPairPEMFn     = generateRSAKeyPairPEM
	encryptWithKMSFn            = encryptWithKMS
	generateOAuthClientIDFn     = generateOAuthClientID
	generateOAuthClientSecretFn = generateOAuthClientSecret
)

func main() {
	log.SetFlags(0)

	ctx := context.Background()
	args, err := parseOwnerBootstrapArgs(os.Args[1:])
	if err != nil {
		ownerBootstrapFatal("invalid_args", err.Error(), nil)
	}
	args.applyDefaults()
	if err := args.validate(); err != nil {
		ownerBootstrapFatal("invalid_args", err.Error(), nil)
	}

	runOwnerBootstrap(ctx, args)
}

func parseOwnerBootstrapArgs(rawArgs []string) (ownerBootstrapArgs, error) {
	var args ownerBootstrapArgs

	fs := flag.NewFlagSet("owner-bootstrap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&args.environment, "environment", "", "Environment name (development|staging|production)")
	fs.StringVar(&args.environment, "env", "", "Alias for -environment")
	fs.StringVar(&args.domain, "domain", "", "Instance domain (e.g., dev.lesser.host)")
	fs.StringVar(&args.tableName, "table", "", "DynamoDB table name (default: lesser-<environment>)")
	fs.StringVar(&args.kmsKeyID, "kms-key-id", "alias/lesser-encryption", "KMS key ID/ARN/alias for actor private key encryption")
	fs.StringVar(&args.walletSecretName, "wallet-secret", "", "Secrets Manager name for admin wallet (default: lesser/<environment>/admin-wallet)")
	fs.StringVar(&args.oauthSecretName, "oauth-secret", "", "Secrets Manager name for admin OAuth client (default: lesser/<environment>/admin-oauth)")
	fs.StringVar(&args.username, "username", "admin", "Username to bootstrap (default: admin)")
	fs.IntVar(&args.chainID, "chain-id", 1, "Wallet chain ID to store (default: 1)")
	fs.BoolVar(&args.force, "force", false, "Force bootstrap even if admin exists (may fail if artifacts already exist)")

	if err := fs.Parse(rawArgs); err != nil {
		return ownerBootstrapArgs{}, err
	}
	args.username = strings.ToLower(strings.TrimSpace(args.username))

	return args, nil
}

func (a *ownerBootstrapArgs) applyDefaults() {
	if a.tableName == "" {
		a.tableName = fmt.Sprintf("lesser-%s", a.environment)
	}
	if a.walletSecretName == "" {
		a.walletSecretName = fmt.Sprintf("lesser/%s/admin-wallet", a.environment)
	}
	if a.oauthSecretName == "" {
		a.oauthSecretName = fmt.Sprintf("lesser/%s/admin-oauth", a.environment)
	}
}

func (a ownerBootstrapArgs) validate() error {
	if a.environment == "" {
		return fmt.Errorf("environment is required")
	}
	if a.domain == "" {
		return fmt.Errorf("domain is required")
	}
	if a.username == "" {
		return fmt.Errorf("username is required")
	}
	if a.kmsKeyID == "" {
		return fmt.Errorf("kms-key-id is required")
	}
	if a.chainID <= 0 {
		return fmt.Errorf("chain-id must be positive")
	}
	return nil
}

type bootstrapState struct {
	userExists         bool
	walletSecretExists bool
	oauthSecretExists  bool
}

type bootstrapFailure struct {
	event   string
	message string
	fields  map[string]any
}

func runOwnerBootstrap(ctx context.Context, args ownerBootstrapArgs) {
	awsCfg, err := loadAWSConfigFn(ctx)
	if err != nil {
		ownerBootstrapFatal("aws_config_load_failed", "load AWS config", map[string]any{"error": err.Error()})
	}

	storagemodels.MainTableName = args.tableName

	db, err := newTableTheoryDBFn(awsCfg)
	if err != nil {
		ownerBootstrapFatal("tabletheory_init_failed", "init TableTheory", map[string]any{"error": err.Error()})
	}
	defer func() {
		_ = db.Close()
	}()
	sm := newSecretsManagerClientFn(awsCfg)
	kmsClient := newKMSClientFn(awsCfg)

	userPK := fmt.Sprintf("USER#%s", strings.ToLower(strings.TrimSpace(args.username)))

	state, err := checkBootstrapState(ctx, db, sm, args, userPK)
	if err != nil {
		ownerBootstrapFatal("bootstrap_check_failed", "check existing bootstrap state", map[string]any{"error": err.Error()})
	}

	skipFields, failure := validateBootstrapState(state, args, userPK)
	if failure != nil {
		ownerBootstrapFatal(failure.event, failure.message, failure.fields)
	}
	if skipFields != nil {
		ownerBootstrapInfo("already_bootstrapped", "admin user exists; skipping bootstrap", skipFields)
		return
	}

	ownerBootstrapInfo("provisioning_start", "provisioning admin artifacts", map[string]any{
		"environment": args.environment,
		"domain":      args.domain,
		"table":       args.tableName,
		"username":    args.username,
	})

	now := time.Now().UTC()
	artifacts := generateBootstrapArtifacts(ctx, kmsClient, args, now)
	persistResult := persistBootstrapArtifacts(ctx, db, sm, args, artifacts)

	ownerBootstrapInfo("provisioning_complete", "bootstrap complete", map[string]any{
		"environment": args.environment,
		"domain":      args.domain,
		"table":       args.tableName,
		"username":    args.username,
		"wallet": map[string]any{
			"secret":  args.walletSecretName,
			"address": artifacts.walletAddress,
			"created": persistResult.walletCreated,
		},
		"oauth": map[string]any{
			"secret":       args.oauthSecretName,
			"client_id":    artifacts.clientID,
			"created":      persistResult.oauthCreated,
			"redirects":    artifacts.redirectURIs,
			"owner_id":     args.username,
			"confidential": true,
		},
	})
}

func checkBootstrapState(ctx context.Context, db tableTheoryAPI, sm secretsManagerAPI, args ownerBootstrapArgs, userPK string) (bootstrapState, error) {
	userExists, err := userMetadataExists(ctx, db, userPK, storagemodels.SKMetadata)
	if err != nil {
		return bootstrapState{}, fmt.Errorf("check admin user existence: %w", err)
	}

	walletSecretExists, err := secretExists(ctx, sm, args.walletSecretName)
	if err != nil {
		return bootstrapState{}, fmt.Errorf("check wallet secret existence: %w", err)
	}
	oauthSecretExists, err := secretExists(ctx, sm, args.oauthSecretName)
	if err != nil {
		return bootstrapState{}, fmt.Errorf("check oauth secret existence: %w", err)
	}

	return bootstrapState{
		userExists:         userExists,
		walletSecretExists: walletSecretExists,
		oauthSecretExists:  oauthSecretExists,
	}, nil
}

func validateBootstrapState(state bootstrapState, args ownerBootstrapArgs, userPK string) (skipFields map[string]any, failure *bootstrapFailure) {
	if state.userExists && !args.force {
		return map[string]any{
			"table":       args.tableName,
			"username":    args.username,
			"user_pk":     userPK,
			"user_sk":     "METADATA",
			"environment": args.environment,
		}, nil
	}

	if !state.userExists && (state.walletSecretExists || state.oauthSecretExists) {
		return nil, &bootstrapFailure{
			event:   "partial_state",
			message: "admin user missing but secret already exists",
			fields: map[string]any{
				"username":          args.username,
				"user_pk":           userPK,
				"user_sk":           "METADATA",
				"wallet_secret":     args.walletSecretName,
				"wallet_secret_set": state.walletSecretExists,
				"oauth_secret":      args.oauthSecretName,
				"oauth_secret_set":  state.oauthSecretExists,
			},
		}
	}

	if state.userExists && args.force {
		return nil, &bootstrapFailure{
			event:   "force_not_supported",
			message: "force mode is not supported when USER#admin already exists (avoid accidental rotation)",
			fields: map[string]any{
				"username": args.username,
				"user_pk":  userPK,
				"user_sk":  "METADATA",
			},
		}
	}

	return nil, nil
}

type bootstrapArtifacts struct {
	items             []any
	walletSecretJSON  []byte
	oauthSecretJSON   []byte
	walletAddress     string
	walletSecretName  string
	oauthSecretName   string
	clientID          string
	redirectURIs      []string
	createdAtISO      string
	walletDescription string
	oauthDescription  string
}

func generateBootstrapArtifacts(ctx context.Context, kmsClient kmsAPI, args ownerBootstrapArgs, now time.Time) bootstrapArtifacts {
	createdAtISO := now.Format(time.RFC3339)

	walletPrivKey, walletAddress, err := generateEthereumWalletFn()
	if err != nil {
		ownerBootstrapFatal("wallet_generation_failed", "generate ethereum wallet", map[string]any{"error": err.Error()})
	}
	walletAddressLower := strings.ToLower(walletAddress)

	actorPrivateKeyPEM, actorPublicKeyPEM, err := generateRSAKeyPairPEMFn(4096)
	if err != nil {
		ownerBootstrapFatal("actor_key_generation_failed", "generate RSA keypair", map[string]any{"error": err.Error()})
	}

	encryptedActorPrivateKey, err := encryptWithKMSFn(ctx, kmsClient, args.kmsKeyID, []byte(actorPrivateKeyPEM))
	if err != nil {
		ownerBootstrapFatal("kms_encrypt_failed", "encrypt actor private key", map[string]any{"error": err.Error(), "kms_key_id": args.kmsKeyID})
	}
	encryptedActorPrivateKeyB64 := base64.StdEncoding.EncodeToString(encryptedActorPrivateKey)

	clientID, err := generateOAuthClientIDFn()
	if err != nil {
		ownerBootstrapFatal("oauth_client_id_generation_failed", "generate oauth client id", map[string]any{"error": err.Error()})
	}
	clientSecret, err := generateOAuthClientSecretFn()
	if err != nil {
		ownerBootstrapFatal("oauth_client_secret_generation_failed", "generate oauth client secret", map[string]any{"error": err.Error()})
	}

	redirectURIs := []string{
		fmt.Sprintf("https://%s/auth/callback", args.domain),
		"urn:ietf:wg:oauth:2.0:oob",
	}

	walletSecretJSON, err := json.Marshal(walletSecretPayload{
		Address:    walletAddress,
		PrivateKey: walletPrivKey,
		ChainID:    args.chainID,
		WalletType: "ethereum",
		Username:   args.username,
		CreatedAt:  createdAtISO,
	})
	if err != nil {
		ownerBootstrapFatal("json_marshal_failed", "marshal wallet secret", map[string]any{"error": err.Error()})
	}

	oauthSecretJSON, err := json.Marshal(oauthSecretPayload{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURIs: redirectURIs,
		Name:         "Owner Console",
		Username:     args.username,
		CreatedAt:    createdAtISO,
	})
	if err != nil {
		ownerBootstrapFatal("json_marshal_failed", "marshal oauth secret", map[string]any{"error": err.Error()})
	}

	actorModel, err := buildActorModel(args.username, args.domain, actorPublicKeyPEM, encryptedActorPrivateKeyB64, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build actor model", map[string]any{"error": err.Error()})
	}
	userModel, err := buildUserModel(args.username, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build user model", map[string]any{"error": err.Error()})
	}

	walletCredentialModel, err := buildWalletCredentialModel(args.username, walletAddressLower, args.chainID, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build wallet credential model", map[string]any{"error": err.Error()})
	}
	walletIndexModel, err := buildWalletIndexModel(args.username, walletAddressLower)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build wallet index model", map[string]any{"error": err.Error()})
	}

	oauthClientModel, err := buildOAuthClientModel(args.username, clientID, clientSecret, redirectURIs, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build oauth client model", map[string]any{"error": err.Error()})
	}

	items := []any{
		userModel,
		actorModel,
		walletCredentialModel,
		walletIndexModel,
		oauthClientModel,
	}

	return bootstrapArtifacts{
		items:             items,
		walletSecretJSON:  walletSecretJSON,
		oauthSecretJSON:   oauthSecretJSON,
		walletAddress:     walletAddress,
		walletSecretName:  args.walletSecretName,
		oauthSecretName:   args.oauthSecretName,
		clientID:          clientID,
		redirectURIs:      redirectURIs,
		createdAtISO:      createdAtISO,
		walletDescription: fmt.Sprintf("Admin wallet for %s", args.environment),
		oauthDescription:  fmt.Sprintf("Admin OAuth client for %s", args.environment),
	}
}

type bootstrapPersistResult struct {
	walletCreated bool
	oauthCreated  bool
}

func persistBootstrapArtifacts(ctx context.Context, db tableTheoryAPI, sm secretsManagerAPI, args ownerBootstrapArgs, artifacts bootstrapArtifacts) bootstrapPersistResult {
	if err := transactWriteAll(ctx, db, artifacts.items); err != nil {
		ownerBootstrapFatal("tabletheory_transact_failed", "write admin artifacts to DynamoDB", map[string]any{"error": err.Error(), "table": args.tableName})
	}

	var result bootstrapPersistResult

	if err := createSecret(ctx, sm, args.walletSecretName, string(artifacts.walletSecretJSON), artifacts.walletDescription); err != nil {
		rollbackSecretsAndTableTheory(ctx, db, sm, nil, artifacts.items)
		ownerBootstrapFatal("secret_create_failed", "create wallet secret", map[string]any{"error": err.Error(), "secret": args.walletSecretName})
	}
	result.walletCreated = true

	if err := createSecret(ctx, sm, args.oauthSecretName, string(artifacts.oauthSecretJSON), artifacts.oauthDescription); err != nil {
		rollbackSecretsAndTableTheory(ctx, db, sm, []string{args.walletSecretName}, artifacts.items)
		ownerBootstrapFatal("secret_create_failed", "create oauth secret", map[string]any{"error": err.Error(), "secret": args.oauthSecretName})
	}
	result.oauthCreated = true

	return result
}

func ownerBootstrapInfo(event, message string, fields map[string]any) {
	entry := jsonLog{Event: event, Message: message, Fields: fields}
	out, _ := json.Marshal(entry)
	_, _ = fmt.Fprintf(os.Stdout, "[owner-bootstrap] %s\n", string(out))
}

func ownerBootstrapFatal(event, message string, fields map[string]any) {
	entry := jsonLog{Event: event, Message: message, Fields: fields}
	out, _ := json.Marshal(entry)
	_, _ = fmt.Fprintf(os.Stderr, "[owner-bootstrap] %s\n", string(out))
	exitFn(1)
}

func userMetadataExists(ctx context.Context, db tableTheoryAPI, pk, sk string) (bool, error) {
	if db == nil {
		return false, errors.New("database is nil")
	}

	var record storagemodels.User
	err := db.Model(&storagemodels.User{}).
		WithContext(ctx).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		ConsistentRead().
		First(&record)
	if err != nil {
		if errors.Is(err, theorydbErrors.ErrTableNotFound) {
			return false, err
		}
		if theorydbErrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func secretExists(ctx context.Context, sm secretsManagerAPI, secretName string) (bool, error) {
	_, err := sm.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(secretName)})
	if err == nil {
		return true, nil
	}
	var notFound *smstypes.ResourceNotFoundException
	if errors.As(err, &notFound) {
		return false, nil
	}
	return false, err
}

func createSecret(ctx context.Context, sm secretsManagerAPI, secretName, secretValue, description string) error {
	_, err := sm.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretString: aws.String(secretValue),
		Description:  aws.String(description),
	})
	return err
}

func deleteSecretImmediate(ctx context.Context, sm secretsManagerAPI, secretName string) error {
	_, err := sm.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	})
	return err
}

func generateEthereumWallet() (privateKeyHex string, address string, err error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return "", "", err
	}
	privBytes := crypto.FromECDSA(key)
	privateKeyHex = "0x" + hex.EncodeToString(privBytes)
	address = crypto.PubkeyToAddress(key.PublicKey).Hex()
	return privateKeyHex, address, nil
}

func generateRSAKeyPairPEM(bits int) (privateKeyPEM string, publicKeyPEM string, err error) {
	if bits < 2048 {
		return "", "", fmt.Errorf("RSA key size too small: %d", bits)
	}
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", err
	}

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	return string(privPEM), string(pubPEM), nil
}

func encryptWithKMS(ctx context.Context, kmsClient kmsAPI, keyID string, plaintext []byte) ([]byte, error) {
	out, err := kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}
	return out.CiphertextBlob, nil
}

func generateOAuthClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func generateOAuthClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func buildActorModel(username, domain, publicKeyPEM, encryptedPrivateKeyB64 string, now time.Time) (*storagemodels.Actor, error) {
	actorID := fmt.Sprintf("https://%s/users/%s", domain, username)
	actor := activitypub.NewActor(activitypub.PersonType, actorID, username)
	actor.Name = username
	actor.URL = fmt.Sprintf("https://%s/@%s", domain, username)
	actor.CreatedAt = &now
	actor.PublicKey = &activitypub.PublicKey{
		ID:           fmt.Sprintf("%s#main-key", actorID),
		Owner:        actorID,
		PublicKeyPem: publicKeyPEM,
	}
	activitypubutil.ApplyLocalActorIdentifiers(actor, fmt.Sprintf("https://%s", domain), username)

	model := &storagemodels.Actor{
		Actor:          actor,
		Username:       username,
		PrivateKey:     encryptedPrivateKeyB64,
		KeyType:        "RSA",
		NumericID:      common.GenerateNumericID(username),
		CreatedAt:      now,
		UpdatedAt:      now,
		FollowerCount:  0,
		FollowingCount: 0,
		StatusCount:    0,
		Version:        1,
	}

	if domain != "" {
		model.GSI3PK = "DOMAIN#" + domain
		model.GSI3SK = username
	}

	if err := model.UpdateKeys(); err != nil {
		return nil, err
	}

	return model, nil
}

func buildUserModel(username string, now time.Time) (*storagemodels.User, error) {
	model := &storagemodels.User{
		Username:           username,
		Approved:           true,
		Role:               "admin",
		Locked:             false,
		Discoverable:       false,
		Suspended:          false,
		Silenced:           false,
		RecoveryMethods:    []string{"wallet"},
		AllowNSFW:          false,
		RequireNSFWWarning: true,
		CreatedAt:          now,
		UpdatedAt:          now,
		Version:            1,
	}

	if err := model.UpdateKeys(); err != nil {
		return nil, err
	}

	return model, nil
}

func buildWalletCredentialModel(username, addressLower string, chainID int, now time.Time) (*storagemodels.WalletCredential, error) {
	model := &storagemodels.WalletCredential{
		Username: username,
		Address:  addressLower,
		ChainID:  chainID,
		Type:     "ethereum",
		LinkedAt: now,
		LastUsed: now,
	}

	if err := model.UpdateKeys(); err != nil {
		return nil, err
	}

	return model, nil
}

func buildWalletIndexModel(username, addressLower string) (*storagemodels.WalletIndex, error) {
	model := &storagemodels.WalletIndex{}
	model.UpdateKeys("ethereum", addressLower, username)
	return model, nil
}

func buildOAuthClientModel(ownerID, clientID, clientSecret string, redirectURIs []string, now time.Time) (*storagemodels.OAuthClient, error) {
	storedSecret, err := common.HashOAuthClientSecret(clientSecret)
	if err != nil {
		return nil, err
	}

	model := &storagemodels.OAuthClient{
		ClientID:     clientID,
		ClientSecret: storedSecret,
		Name:         "Owner Console",
		RedirectURIs: redirectURIs,
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"read", "write", "admin"},
		ClientClass:  auth.ClientClassOperator,
		OwnerID:      ownerID,
		Confidential: true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := model.UpdateKeys(); err != nil {
		return nil, err
	}

	return model, nil
}

func transactWriteAll(ctx context.Context, db tableTheoryAPI, items []any) error {
	if db == nil {
		return errors.New("database is nil")
	}
	if len(items) == 0 {
		return nil
	}

	return db.TransactWrite(ctx, func(tx theorydb.TransactionBuilder) error {
		for _, item := range items {
			if item == nil {
				return errors.New("transaction item is nil")
			}
			tx.Create(item)
		}
		return nil
	})
}

func rollbackSecretsAndTableTheory(ctx context.Context, db tableTheoryAPI, sm secretsManagerAPI, secrets []string, items []any) {
	if len(secrets) > 0 {
		for _, s := range secrets {
			if err := deleteSecretImmediate(ctx, sm, s); err != nil {
				ownerBootstrapInfo("rollback_secret_failed", "failed to delete secret during rollback", map[string]any{"secret": s, "error": err.Error()})
			} else {
				ownerBootstrapInfo("rollback_secret_deleted", "deleted secret during rollback", map[string]any{"secret": s})
			}
		}
	}

	if len(items) == 0 || db == nil {
		return
	}

	if err := db.TransactWrite(ctx, func(tx theorydb.TransactionBuilder) error {
		for _, item := range items {
			if item == nil {
				continue
			}
			tx.Delete(item)
		}
		return nil
	}); err != nil {
		ownerBootstrapInfo("rollback_dynamodb_failed", "failed to rollback DynamoDB items", map[string]any{"error": err.Error(), "table": storagemodels.MainTableName})
	} else {
		ownerBootstrapInfo("rollback_dynamodb_complete", "rolled back DynamoDB items", map[string]any{"table": storagemodels.MainTableName, "items": len(items)})
	}
}
