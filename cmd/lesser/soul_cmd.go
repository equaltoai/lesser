package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/ethereum/go-ethereum/accounts"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
)

var soulENSNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*\.eth$`)

func runSoul(argv []string) error {
	if len(argv) == 0 {
		printUsage()
		return nil
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "ens":
		return runSoulENS(argv[1:])
	default:
		return fmt.Errorf("unknown soul command %q", argv[0])
	}
}

func runSoulENS(argv []string) error {
	if len(argv) == 0 {
		printUsage()
		return nil
	}

	switch argv[0] {
	case helpFlagShort, helpFlagLong, helpCommand:
		printUsage()
		return nil
	case "set":
		return runSoulENSSet(argv[1:])
	case "preview", "get":
		return runSoulENSPreview(argv[1:])
	case "publish":
		return runSoulENSPublish(argv[1:])
	default:
		return fmt.Errorf("unknown soul ens command %q", argv[0])
	}
}

type soulTargetFlags struct {
	App        string
	Stage      string
	AWSProfile string
	BaseDomain string
}

type soulTarget struct {
	App        string
	Stage      naming.Stage
	AWSProfile string
	BaseDomain string
	AWSConfig  aws.Config
	TableName  string
}

type soulENSSetFlags struct {
	Target          soulTargetFlags
	AgentID         string
	Name            string
	ResolverAddress string
	Chain           string
}

type soulENSPublishFlags struct {
	Target          soulTargetFlags
	AgentID         string
	ChangeSummary   string
	BootstrapPath   string
	WalletSecretARN string
	PrivateKeyHex   string
	PrivateKeyFile  string
	SkipVerify      bool
}

type soulSigningMaterial struct {
	Address    string
	PrivateKey *ecdsa.PrivateKey
	Source     string
}

type soulResolveAgentResponse struct {
	Version string `json:"version"`
	Agent   struct {
		AgentID string `json:"agentId"`
		Domain  string `json:"domain"`
		LocalID string `json:"localId"`
		Wallet  string `json:"wallet"`
	} `json:"agent"`
}

type soulLatestVersion struct {
	VersionNumber   int
	RegistrationURI string
}

type soulListVersionsResponse struct {
	Versions []struct {
		VersionNumber   int    `json:"version_number"`
		RegistrationURI string `json:"registration_uri"`
	} `json:"versions"`
}

func registerSoulTargetFlags(fs *flag.FlagSet, target *soulTargetFlags) {
	fs.StringVar(&target.App, "app", envOrDefault("LESSER_APP", ""), "app slug")
	fs.StringVar(&target.Stage, "stage", envOrDefault("LESSER_STAGE", valueDev), "deployment stage (dev|staging|live)")
	fs.StringVar(&target.AWSProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name (optional; sets AWS_PROFILE)")
	fs.StringVar(&target.BaseDomain, "base-domain", envOrDefault("LESSER_BASE_DOMAIN", ""), "base domain (used for default bootstrap path only)")
}

func runSoulENSSet(argv []string) error {
	fs := flag.NewFlagSet("lesser soul ens set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args soulENSSetFlags
	registerSoulTargetFlags(fs, &args.Target)
	fs.StringVar(&args.AgentID, "agent-id", "", "soul agent id (0x...)")
	fs.StringVar(&args.Name, "name", "", "ENS name to publish")
	fs.StringVar(&args.ResolverAddress, "resolver-address", "", "optional ENS resolver address override")
	fs.StringVar(&args.Chain, "chain", "sepolia", "ENS chain (sepolia|mainnet)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	ctx := context.Background()
	target, err := resolveSoulTarget(ctx, args.Target)
	if err != nil {
		return err
	}

	cfg, err := buildSoulENSConfig(args.AgentID, args.Name, args.ResolverAddress, args.Chain)
	if err != nil {
		return err
	}

	return withSoulInstanceRepo(target, func(repo *repositories.InstanceRepository) error {
		if err := repo.SetSoulENSChannel(ctx, cfg); err != nil {
			return err
		}

		fragment, err := json.MarshalIndent(soulENSChannelFragment(cfg), "", "  ")
		if err != nil {
			return err
		}

		fmt.Println("Stored soul ENS config:")
		fmt.Println(string(fragment))
		return nil
	})
}

func runSoulENSPreview(argv []string) error {
	fs := flag.NewFlagSet("lesser soul ens preview", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args soulENSSetFlags
	registerSoulTargetFlags(fs, &args.Target)
	fs.StringVar(&args.AgentID, "agent-id", "", "soul agent id (0x...)")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	ctx := context.Background()
	target, err := resolveSoulTarget(ctx, args.Target)
	if err != nil {
		return err
	}

	return withSoulInstanceRepo(target, func(repo *repositories.InstanceRepository) error {
		cfg, err := repo.GetSoulENSChannel(ctx, normalizeSoulAgentID(args.AgentID))
		if err != nil {
			return err
		}
		if cfg == nil {
			return fmt.Errorf("no soul ENS config stored for agent %s", strings.TrimSpace(args.AgentID))
		}

		fragment, err := json.MarshalIndent(soulENSChannelFragment(cfg), "", "  ")
		if err != nil {
			return err
		}

		fmt.Println("Soul ENS config:")
		fmt.Println(string(fragment))
		return nil
	})
}

func runSoulENSPublish(argv []string) error {
	fs := flag.NewFlagSet("lesser soul ens publish", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args soulENSPublishFlags
	registerSoulTargetFlags(fs, &args.Target)
	fs.StringVar(&args.AgentID, "agent-id", "", "soul agent id (0x...)")
	fs.StringVar(&args.ChangeSummary, "change-summary", "update ENS channel", "registration change summary")
	fs.StringVar(&args.BootstrapPath, "bootstrap", "", "path to local bootstrap.json")
	fs.StringVar(&args.WalletSecretARN, "wallet-secret-arn", "", "Secrets Manager ARN containing a wallet private key")
	fs.StringVar(&args.PrivateKeyHex, "private-key-hex", "", "hex-encoded wallet private key")
	fs.StringVar(&args.PrivateKeyFile, "private-key-file", "", "file containing a wallet private key (hex or JSON)")
	fs.BoolVar(&args.SkipVerify, "skip-verify", false, "skip post-publish ENS resolution verification")

	if err := fs.Parse(argv); err != nil {
		return err
	}

	ctx := context.Background()
	target, err := resolveSoulTarget(ctx, args.Target)
	if err != nil {
		return err
	}

	agentID := normalizeSoulAgentID(args.AgentID)
	if _, err := validateSoulAgentID(agentID); err != nil {
		return err
	}

	return withSoulInstanceRepo(target, func(repo *repositories.InstanceRepository) error {
		return publishSoulENSChannel(ctx, target, repo, agentID, args)
	})
}

func resolveSoulTarget(ctx context.Context, flags soulTargetFlags) (soulTarget, error) {
	appName := strings.TrimSpace(flags.App)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	normalizedApp, err := naming.NormalizeAppName(appName)
	if err != nil {
		return soulTarget{}, err
	}

	stage := naming.StageForEnvironment(flags.Stage)
	switch stage {
	case naming.StageDev, naming.StageStaging, naming.StageLive:
	default:
		return soulTarget{}, fmt.Errorf("invalid stage %q (expected dev|staging|live)", flags.Stage)
	}

	awsCfg, profile, err := loadAWSConfigForCLIFn(ctx, flags.AWSProfile)
	if err != nil {
		return soulTarget{}, err
	}

	baseDomain := strings.TrimSpace(flags.BaseDomain)
	if baseDomain != "" {
		baseDomain, err = normalizeBaseDomain(baseDomain)
		if err != nil {
			return soulTarget{}, err
		}
	}

	return soulTarget{
		App:        normalizedApp,
		Stage:      stage,
		AWSProfile: profile,
		BaseDomain: baseDomain,
		AWSConfig:  awsCfg,
		TableName:  stageMainTableName(normalizedApp, stage),
	}, nil
}

func publishSoulENSChannel(
	ctx context.Context,
	target soulTarget,
	repo *repositories.InstanceRepository,
	agentID string,
	args soulENSPublishFlags,
) error {
	cfg, err := repo.GetSoulENSChannel(ctx, agentID)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("no soul ENS config stored for agent %s", agentID)
	}

	effectiveTrust, err := repo.EffectiveTrustConfig(ctx)
	if err != nil {
		return err
	}
	if effectiveTrust == nil || strings.TrimSpace(effectiveTrust.TrustBaseURL) == "" {
		return errors.New("trust config is not available for soul publish")
	}
	if strings.TrimSpace(effectiveTrust.InstanceKeySecretARN) == "" {
		return errors.New("trust config is missing lesser-host instance key secret ARN")
	}

	instanceKey, err := resolveCLISecretValue(ctx, target.AWSConfig, effectiveTrust.InstanceKeySecretARN)
	if err != nil {
		return fmt.Errorf("resolve lesser-host instance key: %w", err)
	}

	signingMaterial, err := resolveSoulSigningMaterial(ctx, target, args)
	if err != nil {
		return err
	}

	regBytes, err := fetchSoulRegistration(ctx, effectiveTrust.TrustBaseURL, agentID)
	if err != nil {
		return err
	}

	latestVersion, err := fetchLatestSoulVersion(ctx, effectiveTrust.TrustBaseURL, agentID)
	if err != nil {
		return err
	}

	signedRegistration, err := buildSignedSoulENSRegistration(regBytes, latestVersion, cfg, signingMaterial, args.ChangeSummary)
	if err != nil {
		return err
	}

	signedBody, err := buildSoulRegistrationUpdatePayload(signedRegistration, latestVersion.VersionNumber)
	if err != nil {
		return err
	}

	if err := publishSoulRegistration(ctx, effectiveTrust.TrustBaseURL, instanceKey, agentID, signedBody); err != nil {
		return err
	}

	fmt.Printf("Published soul ENS channel for %s using %s\n", cfg.Name, signingMaterial.Source)
	if args.SkipVerify {
		return nil
	}

	resolvedAgentID, err := verifySoulENSResolution(ctx, effectiveTrust.TrustBaseURL, cfg.Name)
	if err != nil {
		return err
	}
	if !strings.EqualFold(resolvedAgentID, agentID) {
		return fmt.Errorf("verification returned unexpected agent_id %s", resolvedAgentID)
	}

	fmt.Printf("Verified %s resolves to %s\n", cfg.Name, resolvedAgentID)
	return nil
}

func withSoulInstanceRepo(target soulTarget, fn func(*repositories.InstanceRepository) error) error {
	db, err := tabletheory.New(session.Config{
		Region:              target.AWSConfig.Region,
		CredentialsProvider: target.AWSConfig.Credentials,
	})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	prevTableName := models.MainTableName
	models.MainTableName = target.TableName
	defer func() {
		models.MainTableName = prevTableName
	}()

	repo := repositories.NewInstanceRepository(db, target.TableName, zap.NewNop())
	return fn(repo)
}

func buildSoulENSConfig(agentID, name, resolverAddress, chain string) (*models.InstanceSoulENSChannel, error) {
	normalizedAgentID, err := validateSoulAgentID(agentID)
	if err != nil {
		return nil, err
	}

	normalizedName, err := normalizeSoulENSName(name)
	if err != nil {
		return nil, err
	}

	normalizedResolver, err := normalizeSoulENSResolverAddress(resolverAddress)
	if err != nil {
		return nil, err
	}

	normalizedChain, err := normalizeSoulENSChain(chain)
	if err != nil {
		return nil, err
	}

	cfg := models.NewInstanceSoulENSChannel(normalizedAgentID)
	cfg.Name = normalizedName
	cfg.ResolverAddress = normalizedResolver
	cfg.Chain = normalizedChain
	return cfg, nil
}

func validateSoulAgentID(agentID string) (string, error) {
	normalized := normalizeSoulAgentID(agentID)
	if normalized == "" {
		return "", errors.New("agent id is required")
	}
	if !strings.HasPrefix(normalized, "0x") || len(normalized) != 66 {
		return "", fmt.Errorf("invalid agent id %q", agentID)
	}
	if _, err := hexutil.Decode(normalized); err != nil {
		return "", fmt.Errorf("invalid agent id %q", agentID)
	}
	return normalized, nil
}

func normalizeSoulAgentID(agentID string) string {
	return strings.ToLower(strings.TrimSpace(agentID))
}

func normalizeSoulENSName(name string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "", errors.New("ENS name is required")
	}
	if !soulENSNameRE.MatchString(normalized) {
		return "", fmt.Errorf("invalid ENS name %q", name)
	}
	return normalized, nil
}

func normalizeSoulENSResolverAddress(addr string) (string, error) {
	normalized := strings.TrimSpace(addr)
	if normalized == "" {
		return "", nil
	}
	if !ethcommon.IsHexAddress(normalized) {
		return "", fmt.Errorf("invalid resolver address %q", addr)
	}
	return ethcommon.HexToAddress(normalized).Hex(), nil
}

func normalizeSoulENSChain(chain string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(chain))
	switch normalized {
	case "sepolia", "mainnet":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid chain %q (expected sepolia or mainnet)", chain)
	}
}

func soulENSChannelFragment(cfg *models.InstanceSoulENSChannel) map[string]any {
	channel := map[string]any{
		"name":  strings.TrimSpace(cfg.Name),
		"chain": strings.TrimSpace(cfg.Chain),
	}
	if strings.TrimSpace(cfg.ResolverAddress) != "" {
		channel["resolverAddress"] = strings.TrimSpace(cfg.ResolverAddress)
	}
	return map[string]any{
		"agentId":  strings.TrimSpace(cfg.AgentID),
		"channels": map[string]any{"ens": channel},
	}
}

func resolveSoulSigningMaterial(ctx context.Context, target soulTarget, args soulENSPublishFlags) (*soulSigningMaterial, error) {
	switch {
	case strings.TrimSpace(args.PrivateKeyHex) != "":
		return parseSoulSigningPrivateKey(strings.TrimSpace(args.PrivateKeyHex), "flag --private-key-hex")
	case strings.TrimSpace(args.PrivateKeyFile) != "":
		raw, err := os.ReadFile(args.PrivateKeyFile) // #nosec G304 -- CLI reads an operator-supplied local path
		if err != nil {
			return nil, fmt.Errorf("read private key file: %w", err)
		}
		return parseSoulSigningPayload(string(raw), "file "+args.PrivateKeyFile)
	case strings.TrimSpace(args.WalletSecretARN) != "":
		raw, err := resolveCLISecretValue(ctx, target.AWSConfig, strings.TrimSpace(args.WalletSecretARN))
		if err != nil {
			return nil, fmt.Errorf("resolve wallet secret: %w", err)
		}
		return parseSoulSigningPayload(raw, "secret "+strings.TrimSpace(args.WalletSecretARN))
	default:
		bootstrapPath := strings.TrimSpace(args.BootstrapPath)
		if bootstrapPath == "" {
			bootstrapPath = defaultSoulBootstrapPath(target.App, target.BaseDomain)
		}
		if bootstrapPath == "" {
			return nil, errors.New("signing material is required (set --bootstrap, --wallet-secret-arn, --private-key-file, or --private-key-hex)")
		}

		wallet, err := readBootstrapKeyMaterialFn(bootstrapPath)
		if err != nil {
			return nil, fmt.Errorf("read bootstrap signing material: %w", err)
		}

		path, err := accounts.ParseDerivationPath(wallet.DerivationPath)
		if err != nil {
			return nil, fmt.Errorf("parse bootstrap derivation path: %w", err)
		}

		priv, err := deriveEthereumPrivateKey(bip39Seed(wallet.Mnemonic), path)
		if err != nil {
			return nil, err
		}

		return &soulSigningMaterial{
			Address:    strings.ToLower(strings.TrimSpace(wallet.Address)),
			PrivateKey: priv,
			Source:     "bootstrap " + bootstrapPath,
		}, nil
	}
}

func defaultSoulBootstrapPath(app, baseDomain string) string {
	if strings.TrimSpace(app) == "" || strings.TrimSpace(baseDomain) == "" {
		return ""
	}
	home, err := userHomeDirFn()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".lesser", app, baseDomain, "bootstrap.json")
}

func bip39Seed(mnemonic string) []byte {
	return bip39.NewSeed(strings.TrimSpace(mnemonic), "")
}

func parseSoulSigningPayload(raw string, source string) (*soulSigningMaterial, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("wallet signing payload is empty")
	}

	var payload struct {
		Address    string `json:"address"`
		PrivateKey string `json:"private_key"`
	}
	if strings.HasPrefix(raw, "{") && json.Unmarshal([]byte(raw), &payload) == nil && strings.TrimSpace(payload.PrivateKey) != "" {
		material, err := parseSoulSigningPrivateKey(payload.PrivateKey, source)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(payload.Address) != "" {
			addr := strings.ToLower(strings.TrimSpace(payload.Address))
			if !strings.EqualFold(addr, material.Address) {
				return nil, fmt.Errorf("wallet address mismatch in %s", source)
			}
		}
		return material, nil
	}

	return parseSoulSigningPrivateKey(raw, source)
}

func parseSoulSigningPrivateKey(raw string, source string) (*soulSigningMaterial, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "0x")
	priv, err := crypto.HexToECDSA(raw)
	if err != nil {
		return nil, fmt.Errorf("parse wallet private key from %s: %w", source, err)
	}
	addr := strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
	return &soulSigningMaterial{
		Address:    addr,
		PrivateKey: priv,
		Source:     source,
	}, nil
}

func resolveCLISecretValue(ctx context.Context, cfg aws.Config, secretARN string) (string, error) {
	client := secretsmanager.NewFromConfig(cfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(strings.TrimSpace(secretARN))})
	if err != nil {
		return "", err
	}
	if out.SecretString == nil {
		return "", errors.New("secret does not contain SecretString")
	}

	value := strings.TrimSpace(*out.SecretString)
	if strings.HasPrefix(value, "{") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(value), &payload); err == nil {
			if secretValue, ok := payload["secret"].(string); ok && strings.TrimSpace(secretValue) != "" {
				value = strings.TrimSpace(secretValue)
			}
		}
	}
	if value == "" {
		return "", errors.New("secret value is empty")
	}
	return value, nil
}

func fetchSoulRegistration(ctx context.Context, baseURL string, agentID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/soul/agents/"+agentID+"/registration", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch current registration failed (%d)", resp.StatusCode)
	}
	return body, nil
}

func fetchLatestSoulVersion(ctx context.Context, baseURL string, agentID string) (soulLatestVersion, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.TrimRight(baseURL, "/")+"/api/v1/soul/agents/"+agentID+"/versions?limit=1",
		nil,
	)
	if err != nil {
		return soulLatestVersion{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return soulLatestVersion{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return soulLatestVersion{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return soulLatestVersion{}, fmt.Errorf("fetch latest soul version failed (%d)", resp.StatusCode)
	}

	var parsed soulListVersionsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return soulLatestVersion{}, fmt.Errorf("parse latest soul version: %w", err)
	}
	if len(parsed.Versions) == 0 {
		return soulLatestVersion{}, errors.New("current soul registration has no published versions")
	}

	latest := parsed.Versions[0]
	if latest.VersionNumber <= 0 {
		return soulLatestVersion{}, errors.New("current soul registration returned an invalid version number")
	}
	if strings.TrimSpace(latest.RegistrationURI) == "" {
		return soulLatestVersion{}, errors.New("current soul registration version is missing registration_uri")
	}
	return soulLatestVersion{
		VersionNumber:   latest.VersionNumber,
		RegistrationURI: strings.TrimSpace(latest.RegistrationURI),
	}, nil
}

func buildSignedSoulENSRegistration(
	currentRegistration []byte,
	latestVersion soulLatestVersion,
	cfg *models.InstanceSoulENSChannel,
	signingMaterial *soulSigningMaterial,
	changeSummary string,
) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("soul ENS config is required")
	}
	if signingMaterial == nil || signingMaterial.PrivateKey == nil {
		return nil, errors.New("wallet signing material is required")
	}
	if latestVersion.VersionNumber <= 0 {
		return nil, errors.New("latest soul version is required")
	}
	if strings.TrimSpace(latestVersion.RegistrationURI) == "" {
		return nil, errors.New("latest soul registration URI is required")
	}

	var reg map[string]any
	if err := json.Unmarshal(currentRegistration, &reg); err != nil {
		return nil, fmt.Errorf("parse current registration: %w", err)
	}
	if _, err := normalizeSoulCurrentRegistrationVersion(reg); err != nil {
		return nil, err
	}

	regAgentID := extractSoulStringField(reg, "agentId")
	if regAgentID == "" {
		regAgentID = extractSoulStringField(reg, "agent_id")
	}
	if !strings.EqualFold(regAgentID, strings.TrimSpace(cfg.AgentID)) {
		return nil, fmt.Errorf(
			"current registration agent id %s does not match configured agent %s",
			regAgentID,
			strings.TrimSpace(cfg.AgentID),
		)
	}

	wallet, _ := reg["wallet"].(string)
	if wallet == "" {
		return nil, errors.New("current registration is missing wallet")
	}
	if !strings.EqualFold(strings.ToLower(strings.TrimSpace(wallet)), signingMaterial.Address) {
		return nil, fmt.Errorf("signing wallet %s does not match registration wallet %s", signingMaterial.Address, wallet)
	}

	reg["version"] = "3"
	reg["previousVersionUri"] = strings.TrimSpace(latestVersion.RegistrationURI)
	reg["updated"] = time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(extractSoulStringField(reg, "created")) == "" {
		reg["created"] = time.Now().UTC().Format(time.RFC3339)
	}
	if summary := strings.TrimSpace(changeSummary); summary != "" {
		reg["changeSummary"] = summary
	}

	channels, err := ensureSoulRegistrationObject(reg, "channels")
	if err != nil {
		return nil, err
	}
	ens, err := ensureSoulRegistrationObject(channels, "ens")
	if err != nil {
		return nil, err
	}
	ens["name"] = strings.TrimSpace(cfg.Name)
	ens["chain"] = strings.TrimSpace(cfg.Chain)
	if strings.TrimSpace(cfg.ResolverAddress) != "" {
		ens["resolverAddress"] = strings.TrimSpace(cfg.ResolverAddress)
	} else {
		delete(ens, "resolverAddress")
	}

	attestations, err := ensureSoulRegistrationObject(reg, "attestations")
	if err != nil {
		return nil, err
	}
	delete(attestations, "selfAttestation")

	unsignedBytes, err := json.Marshal(reg)
	if err != nil {
		return nil, fmt.Errorf("marshal unsigned registration: %w", err)
	}
	jcsBytes, err := jsoncanonicalizer.Transform(unsignedBytes)
	if err != nil {
		return nil, fmt.Errorf("canonicalize unsigned registration: %w", err)
	}
	digest := crypto.Keccak256(jcsBytes)

	sig, err := crypto.Sign(accounts.TextHash(digest), signingMaterial.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("sign registration: %w", err)
	}
	attestations["selfAttestation"] = hexutil.Encode(sig)

	signedBytes, err := json.Marshal(reg)
	if err != nil {
		return nil, fmt.Errorf("marshal signed registration: %w", err)
	}
	return signedBytes, nil
}

func buildSoulRegistrationUpdatePayload(registration []byte, expectedVersion int) ([]byte, error) {
	if expectedVersion < 0 {
		return nil, fmt.Errorf("expected version must be non-negative")
	}
	body, err := json.Marshal(struct {
		Registration    json.RawMessage `json:"registration"`
		ExpectedVersion int             `json:"expected_version"`
	}{
		Registration:    json.RawMessage(registration),
		ExpectedVersion: expectedVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal publish request: %w", err)
	}
	return body, nil
}

func ensureSoulRegistrationObject(root map[string]any, key string) (map[string]any, error) {
	if root == nil {
		return nil, errors.New("registration object is nil")
	}
	if existing, ok := root[key]; ok {
		if existingMap, ok := existing.(map[string]any); ok {
			return existingMap, nil
		}
		return nil, fmt.Errorf("%s must be an object", key)
	}
	out := map[string]any{}
	root[key] = out
	return out, nil
}

func extractSoulStringField(root map[string]any, key string) string {
	if root == nil {
		return ""
	}
	raw, ok := root[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func normalizeSoulCurrentRegistrationVersion(reg map[string]any) (string, error) {
	version := extractSoulStringField(reg, "version")
	switch version {
	case "2", "3":
		return version, nil
	case "", "1":
		return "", errors.New("current soul registration is legacy v1; upgrade it to v2 or v3 before publishing ENS channels")
	default:
		return "", fmt.Errorf("unsupported current soul registration version %q", version)
	}
}

func publishSoulRegistration(ctx context.Context, baseURL string, instanceKey string, agentID string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/soul/agents/"+agentID+"/update-registration", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(instanceKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("publish registration failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func verifySoulENSResolution(ctx context.Context, baseURL string, ensName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/soul/resolve/ens/"+ensName, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verify ENS resolution failed (%d)", resp.StatusCode)
	}

	var parsed soulResolveAgentResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.Agent.AgentID) == "" {
		return "", errors.New("verify ENS resolution returned empty agent id")
	}
	return strings.TrimSpace(parsed.Agent.AgentID), nil
}
