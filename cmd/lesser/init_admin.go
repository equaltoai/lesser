package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/ethereum/go-ethereum/accounts"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/theory-cloud/tabletheory"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type initAdminArgs struct {
	App                   string
	BaseDomain            string
	AWSProfile            string
	Stage                 string
	ProvisioningInputPath string
	Username              string
	WalletAddr            string
	ChainID               int
	Signature             string
	Message               string
	MessageFile           string
	KMSKeyID              string
	ReservedList          string
}

var reservedAdminWallets = map[string]string{
	"0x80189edb676d51b2fb2257b2ad38e018b20ca46e": "lesser.host admin wallet",
	"0x1e14865a53a994b01b9ccfef42669dc0bfe98805": "Safe + 1% recipient (TipSplitter.lesserWallet)",
}

var (
	tabletheoryNewFn                 = func(cfg session.Config) (theorydb.DB, error) { return tabletheory.New(cfg) }
	ensureWalletNotLinkedElsewhereFn = ensureWalletNotLinkedElsewhere
	ensureAdminUserFn                = ensureAdminUser
	ensureActorFn                    = ensureActor
	ensureWalletCredentialFn         = ensureWalletCredential
	ensureWalletIndexFn              = ensureWalletIndex
	ensureInstanceActivatedFn        = ensureInstanceActivated
)

func runInitAdmin(argv []string) error {
	args, err := parseInitAdminArgs(argv)
	if err != nil {
		return err
	}

	app, err := naming.NormalizeAppName(args.App)
	if err != nil {
		return err
	}

	baseDomain, err := normalizeBaseDomain(args.BaseDomain)
	if err != nil {
		return err
	}

	stageValue := strings.TrimSpace(args.Stage)
	if stageValue == "" {
		return errors.New("stage is required")
	}
	stage := naming.StageForEnvironment(stageValue)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return fmt.Errorf("invalid --stage %q (expected dev|staging|live)", args.Stage)
	}

	stageDomain := naming.StageDomain(stage, baseDomain)
	if strings.TrimSpace(stageDomain) == "" {
		return fmt.Errorf("unable to derive stage domain from --stage %s and --base-domain %s", stage, baseDomain)
	}

	username := strings.TrimSpace(args.Username)
	if username == "" {
		username = app
	}
	if strings.EqualFold(username, storagemodels.DefaultBootstrapUsername) {
		return fmt.Errorf("username %q is reserved", username)
	}
	if err := common.ValidateUsername(username); err != nil {
		return err
	}

	walletAddr, err := normalizeEthAddress(args.WalletAddr)
	if err != nil {
		return err
	}
	if err := rejectReservedWallet(walletAddr, args.ReservedList); err != nil {
		return err
	}

	message := args.Message
	if message == "" && strings.TrimSpace(args.MessageFile) != "" {
		data, readErr := os.ReadFile(strings.TrimSpace(args.MessageFile)) // #nosec G304 -- CLI reads operator-provided local path
		if readErr != nil {
			return fmt.Errorf("read --message-file: %w", readErr)
		}
		message = string(data)
	}
	if message == "" {
		return errors.New("message is required (provide --message or --message-file)")
	}

	signature := strings.TrimSpace(args.Signature)
	if signature == "" {
		return errors.New("signature is required")
	}

	// Verify before touching AWS or state.
	if err := verifyEthereumPersonalSign(walletAddr, message, signature); err != nil {
		return err
	}

	ctx := context.Background()

	awsCfg, _, err := loadAWSConfigForCLIFn(ctx, args.AWSProfile)
	if err != nil {
		return err
	}

	db, err := tabletheoryNewFn(session.Config{
		Region:              awsCfg.Region,
		CredentialsProvider: awsCfg.Credentials,
	})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	tableName := stageMainTableName(app, stage)
	storagemodels.MainTableName = tableName

	kmsKeyID := strings.TrimSpace(args.KMSKeyID)
	if kmsKeyID == "" {
		kmsKeyID = fmt.Sprintf("alias/%s", naming.SharedResourceName(app, "encryption"))
	}
	kmsClient := kms.NewFromConfig(awsCfg)

	if err := ensureWalletNotLinkedElsewhereFn(ctx, db, walletAddr, username); err != nil {
		return err
	}

	now := time.Now().UTC()

	if err := ensureAdminUserFn(ctx, db, username, now); err != nil {
		return err
	}

	if err := ensureActorFn(ctx, db, kmsClient, kmsKeyID, username, stageDomain, now); err != nil {
		return err
	}

	if err := ensureWalletCredentialFn(ctx, db, username, walletAddr, args.ChainID, now); err != nil {
		return err
	}
	if err := ensureWalletIndexFn(ctx, db, username, walletAddr); err != nil {
		return err
	}

	if err := ensureInstanceActivatedFn(ctx, db, tableName, username, now); err != nil {
		return err
	}

	fmt.Println("\nInitialized instance admin:")
	fmt.Println("  stage:", stage)
	fmt.Println("  domain:", stageDomain)
	fmt.Println("  username:", username)
	fmt.Println("  wallet:", walletAddr)
	return nil
}

func parseInitAdminArgs(argv []string) (initAdminArgs, error) {
	fs := flag.NewFlagSet("lesser init-admin", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args initAdminArgs
	fs.StringVar(&args.App, "app", "", "app name slug (e.g. my-lesser)")
	fs.StringVar(&args.BaseDomain, "base-domain", "", "base domain with an existing public hosted zone (e.g. example.com)")
	fs.StringVar(&args.AWSProfile, "aws-profile", "", "AWS profile name to use (sets AWS_PROFILE)")
	fs.StringVar(&args.Stage, "stage", "", "target stage (dev|staging|live)")
	fs.StringVar(&args.ProvisioningInputPath, "provisioning-input", "", "managed provisioning input JSON (schema=1|2)")
	fs.StringVar(&args.Username, "username", "", "admin username (default: exactly to --app)")
	fs.StringVar(&args.WalletAddr, "wallet-address", "", "admin wallet address (0x...)")
	fs.IntVar(&args.ChainID, "chain-id", 1, "wallet chain id (default: 1)")
	fs.StringVar(&args.Signature, "signature", "", "EIP-191 personal_sign signature in hex (0x...)")
	fs.StringVar(&args.Message, "message", "", "exact message bytes that were signed (recommended: use --message-file)")
	fs.StringVar(&args.MessageFile, "message-file", "", "path to file containing the exact signed message")
	fs.StringVar(&args.KMSKeyID, "kms-key-id", "", "KMS key ID/ARN/alias for actor private key encryption (default: alias/<app>-shared-encryption)")
	fs.StringVar(&args.ReservedList, "reserved-wallets", "", "comma-separated wallet addresses to reject in addition to built-ins")

	if err := fs.Parse(argv); err != nil {
		return initAdminArgs{}, err
	}

	chainIDSet := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "chain-id" {
			chainIDSet = true
		}
	})

	if strings.TrimSpace(args.ProvisioningInputPath) != "" {
		in, err := readManagedProvisioningInput(args.ProvisioningInputPath)
		if err != nil {
			return initAdminArgs{}, err
		}
		if strings.TrimSpace(args.App) == "" {
			args.App = in.Slug
		}
		if strings.TrimSpace(args.Stage) == "" {
			args.Stage = in.Stage
		}
		if strings.TrimSpace(args.Username) == "" {
			args.Username = in.AdminUsername
		}
		if strings.TrimSpace(args.WalletAddr) == "" {
			args.WalletAddr = in.AdminWalletAddress
		}
		if !chainIDSet && in.AdminWalletChainID > 0 {
			args.ChainID = in.AdminWalletChainID
		}
		if strings.TrimSpace(args.Message) == "" && strings.TrimSpace(args.MessageFile) == "" {
			args.Message = in.ConsentMessage
		}
		if strings.TrimSpace(args.Signature) == "" {
			args.Signature = in.ConsentSignature
		}
	}

	if strings.TrimSpace(args.App) == "" ||
		strings.TrimSpace(args.BaseDomain) == "" ||
		strings.TrimSpace(args.Stage) == "" ||
		strings.TrimSpace(args.WalletAddr) == "" ||
		strings.TrimSpace(args.Signature) == "" {
		return initAdminArgs{}, errors.New("required flags: --base-domain, --signature, --app/--stage/--wallet-address (or --provisioning-input), and (--message or --message-file)")
	}
	if strings.TrimSpace(args.Message) == "" && strings.TrimSpace(args.MessageFile) == "" {
		return initAdminArgs{}, errors.New("required flags: --message or --message-file")
	}
	if args.ChainID <= 0 {
		return initAdminArgs{}, errors.New("--chain-id must be positive")
	}

	return args, nil
}

func normalizeEthAddress(input string) (string, error) {
	addr := strings.TrimSpace(input)
	if !ethcommon.IsHexAddress(addr) {
		return "", fmt.Errorf("invalid wallet address %q", input)
	}
	return strings.ToLower(ethcommon.HexToAddress(addr).Hex()), nil
}

func rejectReservedWallet(walletAddr string, extraReservedCSV string) error {
	addr := strings.ToLower(strings.TrimSpace(walletAddr))
	if reason, ok := reservedAdminWallets[addr]; ok {
		return fmt.Errorf("wallet address %s is reserved and cannot be used as instance admin wallet (%s)", addr, reason)
	}

	extraReservedCSV = strings.TrimSpace(extraReservedCSV)
	if extraReservedCSV == "" {
		return nil
	}

	for _, raw := range strings.Split(extraReservedCSV, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		normalized, err := normalizeEthAddress(raw)
		if err != nil {
			return fmt.Errorf("invalid --reserved-wallets entry %q: %w", raw, err)
		}
		if strings.EqualFold(normalized, addr) {
			return fmt.Errorf("wallet address %s is reserved and cannot be used as instance admin wallet", addr)
		}
	}
	return nil
}

func verifyEthereumPersonalSign(address, message, signature string) error {
	sig, err := hexutil.Decode(strings.TrimSpace(signature))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != 65 {
		return fmt.Errorf("invalid signature length %d (expected 65)", len(sig))
	}

	// Transform V from Ethereum-specific to standard.
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}

	msgHash := accounts.TextHash([]byte(message))
	pubKey, err := crypto.SigToPub(msgHash, sig)
	if err != nil {
		return fmt.Errorf("recover public key: %w", err)
	}

	recovered := crypto.PubkeyToAddress(*pubKey).Hex()
	if !strings.EqualFold(strings.TrimPrefix(recovered, "0x"), strings.TrimPrefix(address, "0x")) {
		return fmt.Errorf("signature address mismatch (expected %s, got %s)", address, recovered)
	}
	return nil
}

func ensureWalletNotLinkedElsewhere(ctx context.Context, db theorydb.DB, walletAddr string, username string) error {
	if db == nil {
		return errors.New("database is nil")
	}

	pk := fmt.Sprintf("WALLET#%s#%s", "ethereum", strings.ToLower(strings.TrimSpace(walletAddr)))

	var indexes []storagemodels.WalletIndex
	err := db.WithContext(ctx).Model(&storagemodels.WalletIndex{}).
		Where("PK", "=", pk).
		All(&indexes)
	if err != nil && !theorydbErrors.IsNotFound(err) {
		return fmt.Errorf("query wallet index: %w", err)
	}

	for _, idx := range indexes {
		if strings.TrimSpace(idx.Username) == "" {
			continue
		}
		if !strings.EqualFold(idx.Username, username) {
			return fmt.Errorf("wallet %s is already linked to username %q", walletAddr, idx.Username)
		}
	}
	return nil
}

func ensureAdminUser(ctx context.Context, db theorydb.DB, username string, now time.Time) error {
	if db == nil {
		return errors.New("database is nil")
	}

	pk := fmt.Sprintf(storagemodels.KeyPatternUser, username)

	var existing storagemodels.User
	err := db.WithContext(ctx).Model(&storagemodels.User{}).
		Where("PK", "=", pk).
		Where("SK", "=", storagemodels.SKMetadata).
		ConsistentRead().
		First(&existing)
	if err != nil && !theorydbErrors.IsNotFound(err) {
		return fmt.Errorf("get user %q: %w", username, err)
	}

	if theorydbErrors.IsNotFound(err) {
		user := &storagemodels.User{
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
		if err := user.UpdateKeys(); err != nil {
			return err
		}

		if err := db.WithContext(ctx).Model(user).Create(); err != nil && !theorydbErrors.IsConditionFailed(err) {
			return fmt.Errorf("create user %q: %w", username, err)
		}
		return nil
	}

	// Ensure user has admin role and is unlocked/approved.
	if existing.Role == "admin" && existing.Approved && !existing.Locked {
		return nil
	}

	builder := db.WithContext(ctx).Model(&storagemodels.User{}).
		Where("PK", "=", pk).
		Where("SK", "=", storagemodels.SKMetadata).
		UpdateBuilder()
	builder.Set("Role", "admin").
		Set("Approved", true).
		Set("Locked", false).
		Set("UpdatedAt", now)
	if err := builder.Execute(); err != nil {
		return fmt.Errorf("update user %q admin fields: %w", username, err)
	}
	return nil
}

func ensureActor(ctx context.Context, db theorydb.DB, kmsClient *kms.Client, kmsKeyID string, username string, domain string, now time.Time) error {
	if db == nil {
		return errors.New("database is nil")
	}

	pk := fmt.Sprintf(storagemodels.KeyPatternActor, username)

	var existing storagemodels.Actor
	err := db.WithContext(ctx).Model(&storagemodels.Actor{}).
		Where("PK", "=", pk).
		Where("SK", "=", storagemodels.SKProfile).
		ConsistentRead().
		First(&existing)
	if err != nil && !theorydbErrors.IsNotFound(err) {
		return fmt.Errorf("get actor %q: %w", username, err)
	}
	if err == nil {
		return nil
	}

	privateKeyPEM, publicKeyPEM, err := generateRSAKeyPairPEM(4096)
	if err != nil {
		return err
	}

	encryptedActorPrivateKey, err := encryptWithKMS(ctx, kmsClient, kmsKeyID, []byte(privateKeyPEM))
	if err != nil {
		return fmt.Errorf("encrypt actor private key: %w", err)
	}
	encryptedActorPrivateKeyB64 := base64.StdEncoding.EncodeToString(encryptedActorPrivateKey)

	actor, err := buildActorModel(username, domain, publicKeyPEM, encryptedActorPrivateKeyB64, now)
	if err != nil {
		return err
	}

	if err := db.WithContext(ctx).Model(actor).Create(); err != nil && !theorydbErrors.IsConditionFailed(err) {
		return fmt.Errorf("create actor %q: %w", username, err)
	}
	return nil
}

func ensureWalletCredential(ctx context.Context, db theorydb.DB, username string, walletAddr string, chainID int, now time.Time) error {
	if db == nil {
		return errors.New("database is nil")
	}

	walletAddr = strings.ToLower(strings.TrimSpace(walletAddr))
	pk := fmt.Sprintf(storagemodels.KeyPatternUser, username)
	sk := fmt.Sprintf("WALLET#%s", walletAddr)

	var existing storagemodels.WalletCredential
	err := db.WithContext(ctx).Model(&storagemodels.WalletCredential{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		ConsistentRead().
		First(&existing)
	if err != nil && !theorydbErrors.IsNotFound(err) {
		return fmt.Errorf("get wallet credential: %w", err)
	}
	if err == nil {
		return nil
	}

	cred := &storagemodels.WalletCredential{
		Username: username,
		Address:  walletAddr,
		ChainID:  chainID,
		Type:     "ethereum",
		LinkedAt: now,
		LastUsed: now,
	}
	if err := cred.UpdateKeys(); err != nil {
		return err
	}

	if err := db.WithContext(ctx).Model(cred).Create(); err != nil && !theorydbErrors.IsConditionFailed(err) {
		return fmt.Errorf("create wallet credential: %w", err)
	}
	return nil
}

func ensureWalletIndex(ctx context.Context, db theorydb.DB, username string, walletAddr string) error {
	if db == nil {
		return errors.New("database is nil")
	}

	walletAddr = strings.ToLower(strings.TrimSpace(walletAddr))
	pk := fmt.Sprintf("WALLET#%s#%s", "ethereum", walletAddr)
	sk := fmt.Sprintf(storagemodels.KeyPatternUser, username)

	var existing storagemodels.WalletIndex
	err := db.WithContext(ctx).Model(&storagemodels.WalletIndex{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		ConsistentRead().
		First(&existing)
	if err != nil && !theorydbErrors.IsNotFound(err) {
		return fmt.Errorf("get wallet index: %w", err)
	}
	if err == nil {
		return nil
	}

	idx := &storagemodels.WalletIndex{}
	idx.UpdateKeys("ethereum", walletAddr, username)
	if err := db.WithContext(ctx).Model(idx).Create(); err != nil && !theorydbErrors.IsConditionFailed(err) {
		return fmt.Errorf("create wallet index: %w", err)
	}
	return nil
}

func ensureInstanceActivated(ctx context.Context, db theorydb.DB, tableName string, username string, now time.Time) error {
	if db == nil {
		return errors.New("database is nil")
	}

	bootstrapTableName = tableName

	var current bootstrapInstanceStateRecord
	err := db.WithContext(ctx).Model(&bootstrapInstanceStateRecord{}).
		Where("PK", "=", instanceConfigKeyPK).
		Where("SK", "=", "STATE").
		ConsistentRead().
		First(&current)
	if err != nil && !theorydbErrors.IsNotFound(err) {
		return fmt.Errorf("get instance state: %w", err)
	}
	if err == nil && strings.TrimSpace(current.PrimaryAdminUsername) != "" && !strings.EqualFold(current.PrimaryAdminUsername, username) {
		return fmt.Errorf("instance already has primary admin %q; refusing to overwrite with %q", current.PrimaryAdminUsername, username)
	}

	builder := db.WithContext(ctx).Model(&bootstrapInstanceStateRecord{}).
		Where("PK", "=", instanceConfigKeyPK).
		Where("SK", "=", "STATE").
		UpdateBuilder()

	builder.Set("Locked", false).
		Set("PrimaryAdminUsername", username).
		Set("BootstrapWalletAddress", "").
		Set("UpdatedAt", now).
		SetIfNotExists("BootstrapUsername", storagemodels.DefaultBootstrapUsername, storagemodels.DefaultBootstrapUsername).
		SetIfNotExists("CreatedAt", now, now).
		SetIfNotExists("ActivatedAt", now, now)

	if err := builder.Execute(); err != nil {
		return fmt.Errorf("update instance state: %w", err)
	}
	return nil
}

func encryptWithKMS(ctx context.Context, client *kms.Client, keyID string, plaintext []byte) ([]byte, error) {
	if client == nil {
		return nil, errors.New("kms client is nil")
	}
	if strings.TrimSpace(keyID) == "" {
		return nil, errors.New("kms key id is empty")
	}

	out, err := client.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(keyID),
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}
	return out.CiphertextBlob, nil
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
