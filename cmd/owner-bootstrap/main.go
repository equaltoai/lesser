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
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/mastodon"
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

func main() {
	log.SetFlags(0)

	ctx := context.Background()
	args := parseOwnerBootstrapArgs()
	args.applyDefaults()
	if err := args.validate(); err != nil {
		ownerBootstrapFatal("invalid_args", err.Error(), nil)
	}

	runOwnerBootstrap(ctx, args)
}

func parseOwnerBootstrapArgs() ownerBootstrapArgs {
	var args ownerBootstrapArgs

	flag.StringVar(&args.environment, "environment", "", "Environment name (development|staging|production)")
	flag.StringVar(&args.environment, "env", "", "Alias for -environment")
	flag.StringVar(&args.domain, "domain", "", "Instance domain (e.g., dev.lesser.host)")
	flag.StringVar(&args.tableName, "table", "", "DynamoDB table name (default: lesser-<environment>)")
	flag.StringVar(&args.kmsKeyID, "kms-key-id", "alias/lesser-encryption", "KMS key ID/ARN/alias for actor private key encryption")
	flag.StringVar(&args.walletSecretName, "wallet-secret", "", "Secrets Manager name for admin wallet (default: lesser/<environment>/admin-wallet)")
	flag.StringVar(&args.oauthSecretName, "oauth-secret", "", "Secrets Manager name for admin OAuth client (default: lesser/<environment>/admin-oauth)")
	flag.StringVar(&args.username, "username", "admin", "Username to bootstrap (default: admin)")
	flag.IntVar(&args.chainID, "chain-id", 1, "Wallet chain ID to store (default: 1)")
	flag.BoolVar(&args.force, "force", false, "Force bootstrap even if admin exists (may fail if artifacts already exist)")
	flag.Parse()

	return args
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
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		ownerBootstrapFatal("aws_config_load_failed", "load AWS config", map[string]any{"error": err.Error()})
	}

	ddb := dynamodb.NewFromConfig(awsCfg)
	sm := secretsmanager.NewFromConfig(awsCfg)
	kmsClient := kms.NewFromConfig(awsCfg)

	userPK := fmt.Sprintf("USER#%s", args.username)

	state, err := checkBootstrapState(ctx, ddb, sm, args, userPK)
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
	persistResult := persistBootstrapArtifacts(ctx, ddb, sm, args, artifacts)

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

func checkBootstrapState(ctx context.Context, ddb *dynamodb.Client, sm *secretsmanager.Client, args ownerBootstrapArgs, userPK string) (bootstrapState, error) {
	userExists, err := dynamoItemExists(ctx, ddb, args.tableName, userPK, "METADATA")
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
	items             []transactPut
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

func generateBootstrapArtifacts(ctx context.Context, kmsClient *kms.Client, args ownerBootstrapArgs, now time.Time) bootstrapArtifacts {
	createdAtISO := now.Format(time.RFC3339)

	walletPrivKey, walletAddress, err := generateEthereumWallet()
	if err != nil {
		ownerBootstrapFatal("wallet_generation_failed", "generate ethereum wallet", map[string]any{"error": err.Error()})
	}
	walletAddressLower := strings.ToLower(walletAddress)

	actorPrivateKeyPEM, actorPublicKeyPEM, err := generateRSAKeyPairPEM(4096)
	if err != nil {
		ownerBootstrapFatal("actor_key_generation_failed", "generate RSA keypair", map[string]any{"error": err.Error()})
	}

	encryptedActorPrivateKey, err := encryptWithKMS(ctx, kmsClient, args.kmsKeyID, []byte(actorPrivateKeyPEM))
	if err != nil {
		ownerBootstrapFatal("kms_encrypt_failed", "encrypt actor private key", map[string]any{"error": err.Error(), "kms_key_id": args.kmsKeyID})
	}
	encryptedActorPrivateKeyB64 := base64.StdEncoding.EncodeToString(encryptedActorPrivateKey)

	clientID, err := generateOAuthClientID()
	if err != nil {
		ownerBootstrapFatal("oauth_client_id_generation_failed", "generate oauth client id", map[string]any{"error": err.Error()})
	}
	clientSecret, err := generateOAuthClientSecret()
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

	actorItem, err := buildActorItem(args.username, args.domain, actorPublicKeyPEM, encryptedActorPrivateKeyB64, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build actor item", map[string]any{"error": err.Error()})
	}
	userItem, err := buildUserItem(args.username, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build user item", map[string]any{"error": err.Error()})
	}

	walletCredentialItem, err := buildWalletCredentialItem(args.username, walletAddressLower, args.chainID, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build wallet credential item", map[string]any{"error": err.Error()})
	}
	walletIndexItem, err := buildWalletIndexItem(args.username, walletAddressLower)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build wallet index item", map[string]any{"error": err.Error()})
	}

	oauthClientItem, err := buildOAuthClientItem(args.username, clientID, clientSecret, redirectURIs, now)
	if err != nil {
		ownerBootstrapFatal("item_build_failed", "build oauth client item", map[string]any{"error": err.Error()})
	}

	userPK := fmt.Sprintf("USER#%s", args.username)
	items := []transactPut{
		{item: userItem, pk: userPK, sk: "METADATA"},
		{item: actorItem, pk: fmt.Sprintf("ACTOR#%s", args.username), sk: "PROFILE"},
		{item: walletCredentialItem, pk: fmt.Sprintf("USER#%s", args.username), sk: fmt.Sprintf("WALLET#%s", walletAddressLower)},
		{item: walletIndexItem, pk: fmt.Sprintf("WALLET#ethereum#%s", walletAddressLower), sk: fmt.Sprintf("USER#%s", args.username)},
		{item: oauthClientItem, pk: fmt.Sprintf("OAUTH_CLIENT#%s", clientID), sk: "CLIENT"},
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

func persistBootstrapArtifacts(ctx context.Context, ddb *dynamodb.Client, sm *secretsmanager.Client, args ownerBootstrapArgs, artifacts bootstrapArtifacts) bootstrapPersistResult {
	if err := transactWriteAll(ctx, ddb, args.tableName, artifacts.items); err != nil {
		ownerBootstrapFatal("dynamodb_transact_failed", "write admin artifacts to DynamoDB", map[string]any{"error": err.Error(), "table": args.tableName})
	}

	var result bootstrapPersistResult

	if err := createSecret(ctx, sm, args.walletSecretName, string(artifacts.walletSecretJSON), artifacts.walletDescription); err != nil {
		rollbackSecretsAndDynamo(ctx, ddb, sm, args.tableName, nil, artifacts.items)
		ownerBootstrapFatal("secret_create_failed", "create wallet secret", map[string]any{"error": err.Error(), "secret": args.walletSecretName})
	}
	result.walletCreated = true

	if err := createSecret(ctx, sm, args.oauthSecretName, string(artifacts.oauthSecretJSON), artifacts.oauthDescription); err != nil {
		rollbackSecretsAndDynamo(ctx, ddb, sm, args.tableName, []string{args.walletSecretName}, artifacts.items)
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
	os.Exit(1)
}

func dynamoItemExists(ctx context.Context, ddb *dynamodb.Client, table, pk, sk string) (bool, error) {
	resp, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]dynamotypes.AttributeValue{
			"PK": &dynamotypes.AttributeValueMemberS{Value: pk},
			"SK": &dynamotypes.AttributeValueMemberS{Value: sk},
		},
		ProjectionExpression: aws.String("PK"),
	})
	if err != nil {
		return false, err
	}
	return len(resp.Item) > 0, nil
}

func secretExists(ctx context.Context, sm *secretsmanager.Client, secretName string) (bool, error) {
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

func createSecret(ctx context.Context, sm *secretsmanager.Client, secretName, secretValue, description string) error {
	_, err := sm.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(secretName),
		SecretString: aws.String(secretValue),
		Description:  aws.String(description),
	})
	return err
}

func deleteSecretImmediate(ctx context.Context, sm *secretsmanager.Client, secretName string) error {
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

func encryptWithKMS(ctx context.Context, kmsClient *kms.Client, keyID string, plaintext []byte) ([]byte, error) {
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

func buildActorItem(username, domain, publicKeyPEM, encryptedPrivateKeyB64 string, now time.Time) (map[string]dynamotypes.AttributeValue, error) {
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
	actor.Endpoints = &activitypub.Endpoints{
		SharedInbox: fmt.Sprintf("https://%s/inbox", domain),
	}
	actor.Inbox = fmt.Sprintf("%s/inbox", actorID)
	actor.Outbox = fmt.Sprintf("%s/outbox", actorID)
	actor.Followers = fmt.Sprintf("%s/followers", actorID)
	actor.Following = fmt.Sprintf("%s/following", actorID)

	actorBytes, err := json.Marshal(actor)
	if err != nil {
		return nil, err
	}
	var actorMap map[string]any
	if err := json.Unmarshal(actorBytes, &actorMap); err != nil {
		return nil, err
	}

	usernameLower := strings.ToLower(username)
	prefix := usernameLower
	if len(prefix) >= 2 {
		prefix = prefix[:2]
	}

	displayLower := strings.ToLower(strings.TrimSpace(username))
	displayPrefix := displayLower
	if len(displayPrefix) >= 2 {
		displayPrefix = displayPrefix[:2]
	}

	dateKey := now.Format("2006-01-02")

	item := map[string]any{
		"PK": "ACTOR#" + username,
		"SK": "PROFILE",

		"gsi1PK": "USERNAME_SEARCH#" + prefix,
		"gsi1SK": usernameLower,

		"gsi2PK": "NAME_SEARCH#" + displayPrefix,
		"gsi2SK": displayLower + "#" + username,

		"gsi3PK": "DOMAIN#" + domain,
		"gsi3SK": username,

		"gsi4PK": "ACTOR_RANK#0-9",
		"gsi4SK": fmt.Sprintf("%010d#%s", 0, username),

		"gsi5PK": "ACTIVE#" + dateKey,
		"gsi5SK": fmt.Sprintf("%d#%s", now.Unix(), username),

		"actor":      actorMap,
		"username":   username,
		"privateKey": encryptedPrivateKeyB64,
		"keyType":    "RSA",
		"numericID":  mastodon.GenerateNumericID(username),

		"createdAt": now,
		"updatedAt": now,

		"followerCount":  0,
		"followingCount": 0,
		"statusCount":    0,

		"version": 1,
	}

	return attributevalue.MarshalMap(item)
}

func buildUserItem(username string, now time.Time) (map[string]dynamotypes.AttributeValue, error) {
	createdAtKey := now.Format(time.RFC3339)

	usernameLower := strings.ToLower(username)
	prefix := usernameLower
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}

	item := map[string]any{
		"PK": "USER#" + username,
		"SK": "METADATA",

		"gsi1PK": "USERS",
		"gsi1SK": fmt.Sprintf("%s#%s", createdAtKey, username),

		"gsi3PK": "ROLE#admin",
		"gsi3SK": username,

		"gsi4PK": "STATUS#active",
		"gsi4SK": username,

		"gsi5PK": fmt.Sprintf("USER_HANDLE_PREFIX#%s", prefix),
		"gsi5SK": usernameLower,

		"username": username,
		"approved": true,
		"role":     "admin",

		"locked":       false,
		"discoverable": false,
		"suspended":    false,
		"silenced":     false,

		"recoveryMethods": []string{"wallet"},

		"allowNSFW":          false,
		"requireNSFWWarning": true,

		"createdAt": now,
		"updatedAt": now,

		"version": 1,
	}

	return attributevalue.MarshalMap(item)
}

func buildWalletCredentialItem(username, addressLower string, chainID int, now time.Time) (map[string]dynamotypes.AttributeValue, error) {
	item := map[string]any{
		"PK": fmt.Sprintf("USER#%s", username),
		"SK": fmt.Sprintf("WALLET#%s", addressLower),

		"username": username,
		"address":  addressLower,
		"chainID":  chainID,
		"type":     "ethereum",
		"linkedAt": now,
		"lastUsed": now,
	}
	return attributevalue.MarshalMap(item)
}

func buildWalletIndexItem(username, addressLower string) (map[string]dynamotypes.AttributeValue, error) {
	item := map[string]any{
		"PK": fmt.Sprintf("WALLET#ethereum#%s", addressLower),
		"SK": fmt.Sprintf("USER#%s", username),

		"username":   username,
		"walletType": "ethereum",
		"address":    addressLower,
	}
	return attributevalue.MarshalMap(item)
}

func buildOAuthClientItem(ownerID, clientID, clientSecret string, redirectURIs []string, now time.Time) (map[string]dynamotypes.AttributeValue, error) {
	descTs := encodeDescendingTimestamp(now)
	item := map[string]any{
		"PK": "OAUTH_CLIENT#" + clientID,
		"SK": "CLIENT",

		"gsi1PK": "OWNER#" + ownerID,
		"gsi1SK": "CLIENT#" + clientID,

		"oauthClientsPK": "OAUTH_CLIENTS",
		"oauthClientsSK": fmt.Sprintf("CREATED_AT#%019d#CLIENT#%s", descTs, clientID),

		"clientID":     clientID,
		"clientSecret": clientSecret,
		"name":         "Owner Console",
		"redirectURIs": redirectURIs,
		"grantTypes":   []string{"authorization_code", "refresh_token"},
		"scopes":       []string{"read", "write", "admin"},
		"ownerID":      ownerID,
		"confidential": true,
		"createdAt":    now,
		"updatedAt":    now,
	}
	return attributevalue.MarshalMap(item)
}

func encodeDescendingTimestamp(timestamp time.Time) int64 {
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return math.MaxInt64 - timestamp.UTC().UnixNano()
}

type transactPut struct {
	item map[string]dynamotypes.AttributeValue
	pk   string
	sk   string
}

func transactWriteAll(ctx context.Context, ddb *dynamodb.Client, table string, puts []transactPut) error {
	tx := make([]dynamotypes.TransactWriteItem, 0, len(puts))
	for _, p := range puts {
		tx = append(tx, dynamotypes.TransactWriteItem{
			Put: &dynamotypes.Put{
				TableName:           aws.String(table),
				Item:                p.item,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			},
		})
	}
	_, err := ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: tx})
	return err
}

func rollbackSecretsAndDynamo(ctx context.Context, ddb *dynamodb.Client, sm *secretsmanager.Client, table string, secrets []string, puts []transactPut) {
	if len(secrets) > 0 {
		for _, s := range secrets {
			if err := deleteSecretImmediate(ctx, sm, s); err != nil {
				ownerBootstrapInfo("rollback_secret_failed", "failed to delete secret during rollback", map[string]any{"secret": s, "error": err.Error()})
			} else {
				ownerBootstrapInfo("rollback_secret_deleted", "deleted secret during rollback", map[string]any{"secret": s})
			}
		}
	}

	tx := make([]dynamotypes.TransactWriteItem, 0, len(puts))
	for _, p := range puts {
		tx = append(tx, dynamotypes.TransactWriteItem{
			Delete: &dynamotypes.Delete{
				TableName: aws.String(table),
				Key: map[string]dynamotypes.AttributeValue{
					"PK": &dynamotypes.AttributeValueMemberS{Value: p.pk},
					"SK": &dynamotypes.AttributeValueMemberS{Value: p.sk},
				},
			},
		})
	}
	if len(tx) == 0 {
		return
	}
	if _, err := ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: tx}); err != nil {
		ownerBootstrapInfo("rollback_dynamodb_failed", "failed to rollback DynamoDB items", map[string]any{"error": err.Error(), "table": table})
	} else {
		ownerBootstrapInfo("rollback_dynamodb_complete", "rolled back DynamoDB items", map[string]any{"table": table, "items": len(puts)})
	}
}
