package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/theory-cloud/tabletheory/v2"
	theorydb "github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
)

type upArgs struct {
	App                    string
	BaseDomain             string
	AWSProfile             string
	Stage                  string
	ProvisioningInputPath  string
	ReleaseDir             string
	BootstrapWalletAddress string
	WithStaging            bool
	OutPath                string
	RebuildLambdas         bool

	LesserHostURL             string
	LesserHostAttestationsURL string
	LesserHostInstanceKeyARN  string
	APICORSAllowedOrigins     string
	InstancePlaneEnabled      *bool
	TranslationEnabled        *bool
	AllowAgents               *bool
	AllowAgentRegistration    *bool

	TipEnabled         *bool
	TipChainID         *int
	TipContractAddress string

	AIEnabled                 *bool
	AIModerationEnabled       *bool
	AINsfwDetectionEnabled    *bool
	AISpamDetectionEnabled    *bool
	AIPiiDetectionEnabled     *bool
	AIContentDetectionEnabled *bool
}

const legacyReleaseAssemblyEnv = "LESSER_USE_LEGACY_RELEASE_ASSEMBLY"

func runUp(argv []string) error {
	args, err := parseUpArgs(argv)
	if err != nil {
		return err
	}

	ctx := context.Background()
	env, err := prepareUpEnv(ctx, args)
	if err != nil {
		return err
	}
	return env.run(ctx)
}

type upEnv struct {
	args              upArgs
	repoRoot          string
	lambdaAssetRoot   string
	releaseVersion    string
	releaseGitSHA     string
	releaseAuthUIDir  string
	releaseAssembly   *releaseDeployAssemblyInstallResult
	app               string
	baseDomain        string
	awsProfile        string
	awsCfg            aws.Config
	accountID         string
	hostedZone        hostedZone
	stages            []naming.Stage
	newDB             bootstrapDBFactory
	bootstrap         bootstrapWallet
	bootstrapRequired bool
	stateDir          string
}

type upAWSContext struct {
	awsCfg     aws.Config
	awsProfile string
	accountID  string
	hostedZone hostedZone
	stages     []naming.Stage
	newDB      bootstrapDBFactory
}

type upBootstrapContext struct {
	bootstrap         bootstrapWallet
	bootstrapRequired bool
	stateDir          string
}

func prepareUpEnv(ctx context.Context, args upArgs) (*upEnv, error) {
	app, baseDomain, args, repoRoot, err := resolveUpExecutionInputs(args)
	if err != nil {
		return nil, err
	}
	awsContext, err := resolveUpAWSContext(ctx, args, baseDomain)
	if err != nil {
		return nil, err
	}
	bootstrapContext, err := resolveUpBootstrapContext(ctx, args, app, baseDomain, awsContext.stages, awsContext.newDB)
	if err != nil {
		return nil, err
	}

	return &upEnv{
		args:              args,
		repoRoot:          repoRoot,
		app:               app,
		baseDomain:        baseDomain,
		awsProfile:        awsContext.awsProfile,
		awsCfg:            awsContext.awsCfg,
		accountID:         awsContext.accountID,
		hostedZone:        awsContext.hostedZone,
		stages:            awsContext.stages,
		newDB:             awsContext.newDB,
		bootstrap:         bootstrapContext.bootstrap,
		bootstrapRequired: bootstrapContext.bootstrapRequired,
		stateDir:          bootstrapContext.stateDir,
	}, nil
}

func resolveUpExecutionInputs(args upArgs) (string, string, upArgs, string, error) {
	app, err := naming.NormalizeAppName(args.App)
	if err != nil {
		return "", "", upArgs{}, "", err
	}

	baseDomain, err := normalizeBaseDomain(args.BaseDomain)
	if err != nil {
		return "", "", upArgs{}, "", err
	}

	args.ReleaseDir, err = resolveUpReleaseDir(args.ReleaseDir)
	if err != nil {
		return "", "", upArgs{}, "", err
	}

	repoRoot := ""
	if requiresDeploySourceCheckout(args) {
		repoRoot, err = findRepoRootFn()
		if err != nil {
			return "", "", upArgs{}, "", err
		}
		if err := validateUpDeployInputs(args, repoRoot); err != nil {
			return "", "", upArgs{}, "", err
		}
	}

	return app, baseDomain, args, repoRoot, nil
}

func validateUpDeployInputs(args upArgs, repoRoot string) error {
	if usesReleaseArtifactsArgs(args) {
		return validateReleaseDeployInputsFn(repoRoot)
	}
	return validateDeploySourceInputsFn(repoRoot)
}

func resolveUpAWSContext(ctx context.Context, args upArgs, baseDomain string) (upAWSContext, error) {
	awsCfg, awsProfile, err := loadAWSConfigForCLIFn(ctx, args.AWSProfile)
	if err != nil {
		return upAWSContext{}, err
	}

	accountID, err := resolveAWSAccountIDFn(ctx, awsCfg)
	if err != nil {
		return upAWSContext{}, err
	}

	hostedZone, err := resolveHostedZoneFn(ctx, awsCfg, baseDomain)
	if err != nil {
		return upAWSContext{}, err
	}

	stages, err := selectUpStages(args.WithStaging, args.Stage)
	if err != nil {
		return upAWSContext{}, err
	}

	return upAWSContext{
		awsCfg:     awsCfg,
		awsProfile: awsProfile,
		accountID:  accountID,
		hostedZone: hostedZone,
		stages:     stages,
		newDB:      newBootstrapDBFactory(awsCfg),
	}, nil
}

func newBootstrapDBFactory(awsCfg aws.Config) bootstrapDBFactory {
	return bootstrapDBFactory(func() (theorydb.DB, error) {
		db, err := tabletheory.New(session.Config{
			Region:              awsCfg.Region,
			CredentialsProvider: awsCfg.Credentials,
		})
		if err != nil {
			return nil, err
		}
		return db, nil
	})
}

func resolveUpBootstrapContext(
	ctx context.Context,
	args upArgs,
	app string,
	baseDomain string,
	stages []naming.Stage,
	newDB bootstrapDBFactory,
) (upBootstrapContext, error) {
	existingBootstrapAddr, bootstrapRequired, err := inspectBootstrapRequirementsFn(ctx, newDB, app, stages)
	if err != nil {
		return upBootstrapContext{}, err
	}

	desiredBootstrapAddr := strings.ToLower(strings.TrimSpace(args.BootstrapWalletAddress))
	if err := validateRequestedBootstrapAddress(existingBootstrapAddr, desiredBootstrapAddr); err != nil {
		return upBootstrapContext{}, err
	}

	stateDir, err := ensureLocalStateDir(app, baseDomain)
	if err != nil {
		return upBootstrapContext{}, err
	}

	bootstrap, err := resolveBootstrapWalletForUp(existingBootstrapAddr, desiredBootstrapAddr, bootstrapRequired)
	if err != nil {
		return upBootstrapContext{}, err
	}
	bootstrap, err = hydrateBootstrapWalletForOutPath(args.OutPath, stateDir, bootstrap)
	if err != nil {
		return upBootstrapContext{}, err
	}
	if err := validateBootstrapMnemonicOutput(args.OutPath, stateDir, bootstrap); err != nil {
		return upBootstrapContext{}, err
	}

	return upBootstrapContext{
		bootstrap:         bootstrap,
		bootstrapRequired: bootstrapRequired,
		stateDir:          stateDir,
	}, nil
}

func validateRequestedBootstrapAddress(existingBootstrapAddr string, desiredBootstrapAddr string) error {
	if desiredBootstrapAddr != "" && existingBootstrapAddr != "" && !strings.EqualFold(existingBootstrapAddr, desiredBootstrapAddr) {
		return fmt.Errorf(
			"--bootstrap-wallet-address %s does not match deployed bootstrap address %s",
			desiredBootstrapAddr,
			existingBootstrapAddr,
		)
	}
	return nil
}

func resolveBootstrapWalletForUp(
	existingBootstrapAddr string,
	desiredBootstrapAddr string,
	bootstrapRequired bool,
) (bootstrapWallet, error) {
	bootstrap := bootstrapWallet{
		Address:        strings.ToLower(strings.TrimSpace(existingBootstrapAddr)),
		DerivationPath: defaultBootstrapDerivationPath,
		ChainID:        1,
	}
	if desiredBootstrapAddr != "" {
		bootstrap.Address = desiredBootstrapAddr
	}
	if !bootstrapRequired || desiredBootstrapAddr != "" {
		return bootstrap, nil
	}
	return determineBootstrapWalletFn(existingBootstrapAddr)
}

func hydrateBootstrapWalletForOutPath(outPath string, stateDir string, bootstrap bootstrapWallet) (bootstrapWallet, error) {
	if outPath == "" || bootstrap.Mnemonic != "" {
		return bootstrap, nil
	}

	defaultPath := filepath.Join(stateDir, "bootstrap.json")
	if !fileExists(defaultPath) {
		return bootstrapWallet{}, errors.New("--out requires local bootstrap key material; no mnemonic found in ~/.lesser (cannot recover from AWS)")
	}

	loaded, err := readBootstrapKeyMaterialFn(defaultPath)
	if err != nil {
		return bootstrapWallet{}, err
	}
	if !strings.EqualFold(loaded.Address, bootstrap.Address) {
		return bootstrapWallet{}, fmt.Errorf(
			"local bootstrap key material address %s does not match deployed bootstrap address %s",
			loaded.Address,
			bootstrap.Address,
		)
	}
	return loaded, nil
}

func validateBootstrapMnemonicOutput(outPath string, stateDir string, bootstrap bootstrapWallet) error {
	if bootstrap.Mnemonic == "" || strings.TrimSpace(outPath) != "" {
		return nil
	}
	defaultPath := filepath.Join(stateDir, "bootstrap.json")
	return fmt.Errorf("bootstrap wallet generated; --out is required to persist the mnemonic (recommended: %s)", defaultPath)
}

func upStages(withStaging bool) []naming.Stage {
	if withStaging {
		return []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive}
	}
	return []naming.Stage{naming.StageDev, naming.StageLive}
}

func selectUpStages(withStaging bool, stage string) ([]naming.Stage, error) {
	value := strings.TrimSpace(strings.ToLower(stage))
	if value == "" {
		return upStages(withStaging), nil
	}
	if withStaging {
		return nil, errors.New("--stage cannot be combined with --with-staging")
	}

	switch value {
	case string(naming.StageDev):
		return []naming.Stage{naming.StageDev}, nil
	case string(naming.StageStaging):
		return []naming.Stage{naming.StageStaging}, nil
	case string(naming.StageLive):
		return []naming.Stage{naming.StageLive}, nil
	default:
		return nil, fmt.Errorf("invalid --stage %q (expected dev|staging|live)", stage)
	}
}

func (e *upEnv) run(ctx context.Context) error {
	if err := e.ensureToolsAvailable(); err != nil {
		return err
	}
	if err := e.prepareLambdaArtifacts(); err != nil {
		return err
	}
	if err := e.handleBootstrapOutput(); err != nil {
		return err
	}

	fmt.Println("\nAWS environment:")
	profileLabel := e.awsProfile
	if strings.TrimSpace(profileLabel) == "" {
		profileLabel = "(ambient)"
	}
	fmt.Println("  profile:", profileLabel)
	fmt.Println("  account:", e.accountID)
	fmt.Println("  region:", e.awsCfg.Region)
	fmt.Println("  hosted_zone:", e.hostedZone.Name, e.hostedZone.ID)

	receipt, err := e.deploy(ctx)
	if err != nil {
		return err
	}

	if err := e.deployUIAssets(ctx, receipt); err != nil {
		return err
	}

	if err := e.bootstrapStages(ctx, receipt); err != nil {
		return err
	}

	if err := e.syncInstanceFeatureConfig(ctx, receipt); err != nil {
		return err
	}

	statePath := filepath.Join(e.stateDir, "state.json")
	if err := writeReceiptFn(statePath, receipt); err != nil {
		return err
	}

	e.printSummary(statePath)
	return nil
}

func (e *upEnv) prepareLambdaArtifacts() error {
	if e.usesReleaseArtifacts() {
		result, err := installReleaseDeployAssetsFn(e.args.ReleaseDir, deployWorkspaceRoot(e.stateDir))
		if err != nil {
			return err
		}
		e.lambdaAssetRoot = result.LambdaAssetRoot
		e.releaseVersion = result.Version
		e.releaseGitSHA = result.GitSHA
		e.releaseAuthUIDir = result.AuthUIDir
		e.releaseAssembly = &result.Assembly

		files, err := relativeLambdaAssetFiles(e.lambdaAssetRoot, result.LambdaFiles)
		if err != nil {
			return err
		}
		if err := writeLambdaAssetMetadata(e.lambdaAssetRoot, lambdaAssetMetadata{
			Schema:         1,
			Mode:           "release",
			Files:          files,
			ReleaseVersion: result.Version,
			ReleaseGitSHA:  result.GitSHA,
			PreparedAt:     time.Now().UTC(),
		}); err != nil {
			return err
		}

		fmt.Printf(
			"✓ Installed %d Lambda artifact(s), auth UI bundle, and deploy assembly from release %s into %s\n",
			len(result.LambdaFiles),
			result.Version,
			deployWorkspaceRoot(e.stateDir),
		)
		return nil
	}

	assetRoot, err := prepareLambdaAssetRootFn(e.stateDir)
	if err != nil {
		return err
	}
	e.lambdaAssetRoot = assetRoot

	if e.args.RebuildLambdas {
		if strings.TrimSpace(e.args.ReleaseDir) != "" {
			fmt.Println("Rebuilding Lambda artifacts from source because --rebuild-lambdas overrides --release-dir.")
		}
		if err := buildLambdaZipsFn(e.repoRoot, true); err != nil {
			return err
		}
		files, err := stageLocalLambdaAssetsFn(e.repoRoot, e.lambdaAssetRoot)
		if err != nil {
			return err
		}
		return e.recordLocalLambdaAssets(files)
	}

	if err := buildLambdaZipsFn(e.repoRoot, false); err != nil {
		return err
	}
	files, err := stageLocalLambdaAssetsFn(e.repoRoot, e.lambdaAssetRoot)
	if err != nil {
		return err
	}
	return e.recordLocalLambdaAssets(files)
}

func (e *upEnv) recordLocalLambdaAssets(files []string) error {
	relFiles, err := relativeLambdaAssetFiles(e.lambdaAssetRoot, files)
	if err != nil {
		return err
	}
	if err := writeLambdaAssetMetadata(e.lambdaAssetRoot, lambdaAssetMetadata{
		Schema:     1,
		Mode:       "source",
		Files:      relFiles,
		PreparedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	fmt.Printf("✓ Staged %d Lambda artifact(s) from local build into %s\n", len(files), e.lambdaAssetRoot)
	return nil
}

func (e *upEnv) handleBootstrapOutput() error {
	if e.bootstrap.Mnemonic == "" {
		return nil
	}

	fmt.Println("\nBootstrap admin wallet:")
	fmt.Println("  address:", e.bootstrap.Address)
	fmt.Println("  derivation_path:", e.bootstrap.DerivationPath)

	if e.args.OutPath == "" {
		return nil
	}

	if err := writeBootstrapKeyMaterialFn(e.args.OutPath, e.bootstrap); err != nil {
		return err
	}
	fmt.Println("Wrote bootstrap key material to:", e.args.OutPath)
	fmt.Println("WARNING: secure this mnemonic. Losing it before activation requires teardown + redeploy.")
	return nil
}

func (e *upEnv) deploy(ctx context.Context) (*upReceipt, error) {
	if e.usesReleaseArtifacts() && releaseAssemblyLegacyModeEnabled() {
		fmt.Printf("\nUsing legacy release assembly deploy path because %s is enabled.\n", legacyReleaseAssemblyEnv)
		return e.deployFromReleaseAssembly(ctx)
	}
	return e.deployFromSource(ctx)
}

func (e *upEnv) deployFromSource(ctx context.Context) (*upReceipt, error) {
	receipt := newUpReceipt(e.app, e.baseDomain, e.awsProfile, e.accountID, e.awsCfg.Region, e.stages, e.hostedZone)
	receipt.Integration = resolveIntegrationReceipt(e.args)

	if err := cdkBootstrapFn(ctx, e.repoRoot, e.awsProfile, e.accountID, e.awsCfg.Region); err != nil {
		return nil, err
	}

	contexts := map[string]string{}
	if v := strings.TrimSpace(e.releaseVersion); v != "" {
		contexts["lesserVersion"] = v
	}
	if receipt.Integration != nil && receipt.Integration.BodyEnabled != nil {
		contexts["bodyEnabled"] = strconv.FormatBool(*receipt.Integration.BodyEnabled)
	}
	if receipt.Integration != nil && receipt.Integration.InstancePlaneEnabled != nil {
		contexts["instancePlaneEnabled"] = strconv.FormatBool(*receipt.Integration.InstancePlaneEnabled)
	}
	if v := strings.TrimSpace(e.args.LesserHostURL); v != "" {
		contexts["lesserHostUrl"] = v
	}
	if v := strings.TrimSpace(e.args.LesserHostAttestationsURL); v != "" {
		contexts["lesserHostAttestationsUrl"] = v
	}
	if v := strings.TrimSpace(e.args.LesserHostInstanceKeyARN); v != "" {
		contexts["lesserHostInstanceKeyArn"] = v
	}
	if e.args.TranslationEnabled != nil {
		contexts["translationEnabled"] = strconv.FormatBool(*e.args.TranslationEnabled)
	}
	if e.args.AllowAgents != nil {
		contexts["allowAgents"] = strconv.FormatBool(*e.args.AllowAgents)
	}
	if e.args.AllowAgentRegistration != nil {
		contexts["allowAgentRegistration"] = strconv.FormatBool(*e.args.AllowAgentRegistration)
	}
	if e.args.TipEnabled != nil {
		contexts["tipEnabled"] = strconv.FormatBool(*e.args.TipEnabled)
	}
	if e.args.TipChainID != nil {
		contexts["tipChainId"] = strconv.Itoa(*e.args.TipChainID)
	}
	if v := strings.TrimSpace(e.args.TipContractAddress); v != "" {
		contexts["tipContractAddress"] = v
	}
	if v := strings.TrimSpace(e.args.APICORSAllowedOrigins); v != "" {
		contexts["apiCorsAllowedOrigins"] = v
	}

	fmt.Println("\nEnsuring API Gateway account logging role...")
	if err := ensureAPIGatewayCloudWatchLogsRoleFn(ctx, e.awsCfg); err != nil {
		return nil, err
	}

	sharedStack := naming.SharedStackName(e.app)
	fmt.Println("\nDeploying shared stack:", sharedStack)
	sharedResult, err := cdkDeployWithOutputsFn(ctx, e.repoRoot, e.awsProfile, cdkDeployRequest{
		StackName:       sharedStack,
		App:             e.app,
		BaseDomain:      e.baseDomain,
		HostedZoneID:    e.hostedZone.ID,
		Region:          e.awsCfg.Region,
		LambdaAssetRoot: e.lambdaAssetRoot,
		OutputsPath:     deployCdkOutputsPath(e.stateDir, sharedStack),
		StageFilter:     string(naming.StageShared),
		WithStaging:     e.args.WithStaging,
		Contexts:        contexts,
	})
	if err != nil {
		return nil, err
	}
	receipt.SharedOutputs = sharedResult.Outputs

	for _, stage := range e.stages {
		stack := naming.StageStackName(e.app, stage)
		fmt.Println("\nDeploying stage stack:", stack)
		stageResult, err := cdkDeployWithOutputsFn(ctx, e.repoRoot, e.awsProfile, cdkDeployRequest{
			StackName:       stack,
			App:             e.app,
			BaseDomain:      e.baseDomain,
			HostedZoneID:    e.hostedZone.ID,
			Region:          e.awsCfg.Region,
			LambdaAssetRoot: e.lambdaAssetRoot,
			OutputsPath:     deployCdkOutputsPath(e.stateDir, stack),
			StageFilter:     string(stage),
			WithStaging:     e.args.WithStaging,
			Contexts:        contexts,
		})
		if err != nil {
			return nil, err
		}

		receipt.Stages[string(stage)].StackName = stack
		receipt.Stages[string(stage)].Domain = naming.StageDomain(stage, e.baseDomain)
		receipt.Stages[string(stage)].StackOutputs = stageResult.Outputs
	}

	return receipt, nil
}

func (e *upEnv) bootstrapStages(ctx context.Context, receipt *upReceipt) error {
	for _, stage := range e.stages {
		state, err := ensureStageBootstrapStateFn(ctx, e.newDB, e.app, stage, e.bootstrap.Address)
		if err != nil {
			return err
		}

		receipt.Stages[string(stage)].BootstrapAddress = state.Address
		receipt.Stages[string(stage)].Locked = state.Locked
		receipt.Stages[string(stage)].TableName = stageMainTableName(e.app, stage)
		receipt.Stages[string(stage)].URLs = stageURLs(stage, e.baseDomain)
		if state.Updated {
			receipt.Stages[string(stage)].BootstrappedAt = time.Now().UTC()
		}
	}
	return nil
}

func (e *upEnv) printSummary(statePath string) {
	fmt.Println("\nDeployment receipt:", statePath)
	if strings.TrimSpace(e.lambdaAssetRoot) != "" {
		fmt.Println("Lambda asset root:", e.lambdaAssetRoot)
	}
	printStageURLs(e.stages, e.baseDomain)
	fmt.Println("\nNext steps:")
	if strings.TrimSpace(e.args.ProvisioningInputPath) != "" {
		fmt.Println("  1) Seed the initial admin wallet with `lesser init-admin` using the provisioning input.")
		fmt.Println("  2) Log in with the admin wallet and optionally enroll a passkey.")
		return
	}
	if e.bootstrap.Mnemonic != "" {
		fmt.Println("  1) Import the bootstrap mnemonic into a wallet (e.g. MetaMask).")
	} else {
		fmt.Println("  1) Use the existing bootstrap wallet configured for this deployment.")
	}
	fmt.Println("  2) Confirm each stage is locked via the setup status endpoint (GET /setup/status).")
	fmt.Println("  3) Open the setup wizard UI (/auth/setup) to create a real admin and finalize activation (deletes bootstrap).")
}

func parseUpArgs(argv []string) (upArgs, error) {
	fs := flag.NewFlagSet("lesser up", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args upArgs
	fs.StringVar(&args.App, "app", "", "app name slug (e.g. my-lesser)")
	fs.StringVar(&args.BaseDomain, "base-domain", "", "base domain with an existing public hosted zone (e.g. example.com)")
	fs.StringVar(&args.AWSProfile, "aws-profile", os.Getenv("AWS_PROFILE"), "AWS profile name to use (sets AWS_PROFILE)")
	fs.StringVar(&args.Stage, "stage", "", "deploy a single stage (dev|staging|live); default deploys dev+live")
	fs.StringVar(&args.ProvisioningInputPath, "provisioning-input", "", "managed provisioning input JSON (schema=1|2)")
	fs.StringVar(&args.ReleaseDir, "release-dir", "", "directory containing release deploy assets (checksums.txt, lesser-release.json, lesser-lambda-bundle.tar.gz, lesser-lambda-bundle.json, lesser-auth-ui.tar.gz, lesser-deploy-assembly.tar.gz, lesser-deploy-assembly.json)")
	fs.StringVar(&args.BootstrapWalletAddress, "bootstrap-wallet-address", "", "use this bootstrap wallet address instead of generating a mnemonic (env: LESSER_BOOTSTRAP_WALLET_ADDRESS)")
	fs.StringVar(&args.APICORSAllowedOrigins, "api-cors-allowed-origins", "", "comma-separated browser API CORS origins (env: API_CORS_ALLOWED_ORIGINS)")
	fs.BoolVar(&args.WithStaging, "with-staging", false, "also deploy staging")
	fs.StringVar(&args.OutPath, "out", "", "write bootstrap key material to this path (0600). Required on first deploy.")
	fs.BoolVar(&args.RebuildLambdas, "rebuild-lambdas", false, "force rebuild Lambda zip artifacts")

	if err := fs.Parse(argv); err != nil {
		return upArgs{}, err
	}

	if strings.TrimSpace(args.ProvisioningInputPath) != "" {
		in, err := readManagedProvisioningInput(args.ProvisioningInputPath)
		if err != nil {
			return upArgs{}, err
		}
		applyManagedProvisioningDefaults(&args, in)
	}

	if strings.TrimSpace(args.App) == "" || strings.TrimSpace(args.BaseDomain) == "" {
		return upArgs{}, errors.New("required flags: --base-domain and --app (or --provisioning-input)")
	}
	if strings.TrimSpace(args.BootstrapWalletAddress) == "" {
		args.BootstrapWalletAddress = strings.TrimSpace(os.Getenv("LESSER_BOOTSTRAP_WALLET_ADDRESS"))
	}
	if strings.TrimSpace(args.APICORSAllowedOrigins) == "" {
		args.APICORSAllowedOrigins = firstNonEmpty(os.Getenv("API_CORS_ALLOWED_ORIGINS"), os.Getenv("CORS_ALLOWED_ORIGINS"))
	}
	if strings.TrimSpace(args.BootstrapWalletAddress) != "" {
		normalized, err := normalizeBootstrapWalletAddress(args.BootstrapWalletAddress)
		if err != nil {
			return upArgs{}, err
		}
		args.BootstrapWalletAddress = normalized
		if err := rejectReservedWallet(args.BootstrapWalletAddress, ""); err != nil {
			return upArgs{}, err
		}
	}

	return args, nil
}

func usesReleaseArtifactsArgs(args upArgs) bool {
	return strings.TrimSpace(args.ReleaseDir) != "" && !args.RebuildLambdas
}

func (e *upEnv) usesReleaseArtifacts() bool {
	return usesReleaseArtifactsArgs(e.args)
}

func (e *upEnv) ensureToolsAvailable() error {
	if e.usesReleaseArtifacts() {
		return ensureReleaseDeployToolsAvailableFn()
	}
	return ensureToolsAvailableFn()
}

func requiresDeploySourceCheckout(_ upArgs) bool {
	return true
}

func releaseAssemblyLegacyModeEnabled() bool {
	value := strings.TrimSpace(os.Getenv(legacyReleaseAssemblyEnv))
	return value == "1" || strings.EqualFold(value, "true")
}

func normalizeReleaseDir(input string) (string, error) {
	releaseDir := strings.TrimSpace(input)
	if releaseDir == "" {
		return "", nil
	}

	absReleaseDir, err := filepath.Abs(releaseDir)
	if err != nil {
		return "", fmt.Errorf("resolve --release-dir %q: %w", input, err)
	}

	info, err := os.Stat(absReleaseDir)
	if err != nil {
		return "", fmt.Errorf("stat --release-dir %s: %w", absReleaseDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("--release-dir must be a directory: %s", absReleaseDir)
	}

	return absReleaseDir, nil
}

func resolveUpReleaseDir(input string) (string, error) {
	return normalizeReleaseDir(input)
}

func applyManagedProvisioningDefaults(args *upArgs, in managedProvisioningInput) {
	if args == nil {
		return
	}

	if strings.TrimSpace(args.App) == "" {
		args.App = in.Slug
	}
	if strings.TrimSpace(args.Stage) == "" {
		args.Stage = in.Stage
	}
	if strings.TrimSpace(args.BootstrapWalletAddress) == "" {
		args.BootstrapWalletAddress = in.AdminWalletAddress
	}
	if strings.TrimSpace(args.LesserHostURL) == "" {
		args.LesserHostURL = in.LesserHostURL
	}
	if strings.TrimSpace(args.LesserHostAttestationsURL) == "" {
		args.LesserHostAttestationsURL = in.LesserHostAttestationsURL
	}
	if strings.TrimSpace(args.LesserHostInstanceKeyARN) == "" {
		args.LesserHostInstanceKeyARN = in.LesserHostInstanceKeyARN
	}
	if args.InstancePlaneEnabled == nil {
		args.InstancePlaneEnabled = in.InstancePlaneEnabled
	}
	if strings.TrimSpace(args.APICORSAllowedOrigins) == "" {
		args.APICORSAllowedOrigins = in.APICORSAllowedOrigins
	}
	if args.TranslationEnabled == nil {
		args.TranslationEnabled = in.TranslationEnabled
	}
	if args.AllowAgents == nil {
		args.AllowAgents = in.AllowAgents
	}
	if args.AllowAgentRegistration == nil {
		args.AllowAgentRegistration = in.AllowAgentRegistration
	}
	if args.TipEnabled == nil {
		args.TipEnabled = in.TipEnabled
	}
	if args.TipChainID == nil {
		args.TipChainID = in.TipChainID
	}
	if strings.TrimSpace(args.TipContractAddress) == "" {
		args.TipContractAddress = in.TipContractAddress
	}
	if args.AIEnabled == nil {
		args.AIEnabled = in.AIEnabled
	}
	if args.AIModerationEnabled == nil {
		args.AIModerationEnabled = in.AIModerationEnabled
	}
	if args.AINsfwDetectionEnabled == nil {
		args.AINsfwDetectionEnabled = in.AINsfwDetectionEnabled
	}
	if args.AISpamDetectionEnabled == nil {
		args.AISpamDetectionEnabled = in.AISpamDetectionEnabled
	}
	if args.AIPiiDetectionEnabled == nil {
		args.AIPiiDetectionEnabled = in.AIPiiDetectionEnabled
	}
	if args.AIContentDetectionEnabled == nil {
		args.AIContentDetectionEnabled = in.AIContentDetectionEnabled
	}
}

func normalizeBootstrapWalletAddress(input string) (string, error) {
	addr := strings.TrimSpace(input)
	if !ethcommon.IsHexAddress(addr) {
		return "", fmt.Errorf("invalid bootstrap wallet address %q", input)
	}
	return strings.ToLower(ethcommon.HexToAddress(addr).Hex()), nil
}
