package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/theory-cloud/tabletheory"
	theorydb "github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type upArgs struct {
	App                    string
	BaseDomain             string
	AWSProfile             string
	Stage                  string
	BootstrapWalletAddress string
	WithStaging            bool
	OutPath                string
	RebuildLambdas         bool
}

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

func prepareUpEnv(ctx context.Context, args upArgs) (*upEnv, error) {
	repoRoot, err := findRepoRootFn()
	if err != nil {
		return nil, err
	}

	app, err := naming.NormalizeAppName(args.App)
	if err != nil {
		return nil, err
	}

	baseDomain, err := normalizeBaseDomain(args.BaseDomain)
	if err != nil {
		return nil, err
	}

	awsProfile := strings.TrimSpace(args.AWSProfile)
	if awsProfile == "" {
		return nil, errors.New("aws profile is required")
	}

	awsCfg, err := loadAWSConfigFromProfileFn(ctx, awsProfile)
	if err != nil {
		return nil, err
	}

	accountID, err := resolveAWSAccountIDFn(ctx, awsCfg)
	if err != nil {
		return nil, err
	}

	hostedZone, err := resolveHostedZoneFn(ctx, awsCfg, baseDomain)
	if err != nil {
		return nil, err
	}

	stages, err := selectUpStages(args.WithStaging, args.Stage)
	if err != nil {
		return nil, err
	}
	newDB := bootstrapDBFactory(func() (theorydb.DB, error) {
		db, dbErr := tabletheory.New(session.Config{
			Region:              awsCfg.Region,
			CredentialsProvider: awsCfg.Credentials,
		})
		if dbErr != nil {
			return nil, dbErr
		}
		return db, nil
	})

	existingBootstrapAddr, bootstrapRequired, err := inspectBootstrapRequirementsFn(ctx, newDB, app, stages)
	if err != nil {
		return nil, err
	}
	desiredBootstrapAddr := strings.ToLower(strings.TrimSpace(args.BootstrapWalletAddress))
	if desiredBootstrapAddr != "" && existingBootstrapAddr != "" && !strings.EqualFold(existingBootstrapAddr, desiredBootstrapAddr) {
		return nil, fmt.Errorf("--bootstrap-wallet-address %s does not match deployed bootstrap address %s", desiredBootstrapAddr, existingBootstrapAddr)
	}

	stateDir, err := ensureLocalStateDir(app, baseDomain)
	if err != nil {
		return nil, err
	}

	bootstrap := bootstrapWallet{
		Address:        strings.ToLower(strings.TrimSpace(existingBootstrapAddr)),
		DerivationPath: defaultBootstrapDerivationPath,
		ChainID:        1,
	}
	if desiredBootstrapAddr != "" {
		bootstrap.Address = desiredBootstrapAddr
	}
	if bootstrapRequired && desiredBootstrapAddr == "" {
		bootstrap, err = determineBootstrapWalletFn(existingBootstrapAddr)
		if err != nil {
			return nil, err
		}
	}

	if bootstrap.Mnemonic != "" && strings.TrimSpace(args.OutPath) == "" {
		defaultPath := filepath.Join(stateDir, "bootstrap.json")
		return nil, fmt.Errorf("bootstrap wallet generated; --out is required to persist the mnemonic (recommended: %s)", defaultPath)
	}

	if args.OutPath != "" && bootstrap.Mnemonic == "" {
		defaultPath := filepath.Join(stateDir, "bootstrap.json")
		if !fileExists(defaultPath) {
			return nil, errors.New("--out requires local bootstrap key material; no mnemonic found in ~/.lesser (cannot recover from AWS)")
		}
		loaded, err := readBootstrapKeyMaterialFn(defaultPath)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(loaded.Address, bootstrap.Address) {
			return nil, fmt.Errorf("local bootstrap key material address %s does not match deployed bootstrap address %s", loaded.Address, bootstrap.Address)
		}
		bootstrap = loaded
	}

	return &upEnv{
		args:              args,
		repoRoot:          repoRoot,
		app:               app,
		baseDomain:        baseDomain,
		awsProfile:        awsProfile,
		awsCfg:            awsCfg,
		accountID:         accountID,
		hostedZone:        hostedZone,
		stages:            stages,
		newDB:             newDB,
		bootstrap:         bootstrap,
		bootstrapRequired: bootstrapRequired,
		stateDir:          stateDir,
	}, nil
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
	if err := ensureToolsAvailableFn(); err != nil {
		return err
	}
	if err := buildLambdaZipsFn(e.repoRoot, e.args.RebuildLambdas); err != nil {
		return err
	}
	if err := e.handleBootstrapOutput(); err != nil {
		return err
	}

	fmt.Println("\nAWS environment:")
	fmt.Println("  profile:", e.awsProfile)
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

	statePath := filepath.Join(e.stateDir, "state.json")
	if err := writeReceiptFn(statePath, receipt); err != nil {
		return err
	}

	e.printSummary(statePath)
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
	receipt := newUpReceipt(e.app, e.baseDomain, e.awsProfile, e.accountID, e.awsCfg.Region, e.stages, e.hostedZone)

	if err := cdkBootstrapFn(ctx, e.repoRoot, e.awsProfile, e.accountID, e.awsCfg.Region); err != nil {
		return nil, err
	}

	fmt.Println("\nEnsuring API Gateway account logging role...")
	if err := ensureAPIGatewayCloudWatchLogsRoleFn(ctx, e.awsCfg); err != nil {
		return nil, err
	}

	sharedStack := naming.SharedStackName(e.app)
	fmt.Println("\nDeploying shared stack:", sharedStack)
	sharedResult, err := cdkDeployWithOutputsFn(ctx, e.repoRoot, e.awsProfile, cdkDeployRequest{
		StackName:    sharedStack,
		App:          e.app,
		BaseDomain:   e.baseDomain,
		HostedZoneID: e.hostedZone.ID,
		Region:       e.awsCfg.Region,
		StageFilter:  string(naming.StageShared),
		WithStaging:  e.args.WithStaging,
	})
	if err != nil {
		return nil, err
	}
	receipt.SharedOutputs = sharedResult.Outputs

	for _, stage := range e.stages {
		stack := naming.StageStackName(e.app, stage)
		fmt.Println("\nDeploying stage stack:", stack)
		stageResult, err := cdkDeployWithOutputsFn(ctx, e.repoRoot, e.awsProfile, cdkDeployRequest{
			StackName:    stack,
			App:          e.app,
			BaseDomain:   e.baseDomain,
			HostedZoneID: e.hostedZone.ID,
			Region:       e.awsCfg.Region,
			StageFilter:  string(stage),
			WithStaging:  e.args.WithStaging,
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
	printStageURLs(e.stages, e.baseDomain)
	fmt.Println("\nNext steps:")
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
	fs.StringVar(&args.AWSProfile, "aws-profile", "", "AWS profile name to use (sets AWS_PROFILE)")
	fs.StringVar(&args.Stage, "stage", "", "deploy a single stage (dev|staging|live); default deploys dev+live")
	fs.StringVar(&args.BootstrapWalletAddress, "bootstrap-wallet-address", "", "use this bootstrap wallet address instead of generating a mnemonic (env: LESSER_BOOTSTRAP_WALLET_ADDRESS)")
	fs.BoolVar(&args.WithStaging, "with-staging", false, "also deploy staging")
	fs.StringVar(&args.OutPath, "out", "", "write bootstrap key material to this path (0600). Required on first deploy.")
	fs.BoolVar(&args.RebuildLambdas, "rebuild-lambdas", false, "force rebuild Lambda zip artifacts")

	if err := fs.Parse(argv); err != nil {
		return upArgs{}, err
	}

	if strings.TrimSpace(args.App) == "" || strings.TrimSpace(args.BaseDomain) == "" || strings.TrimSpace(args.AWSProfile) == "" {
		return upArgs{}, errors.New("required flags: --app, --base-domain, --aws-profile")
	}
	if strings.TrimSpace(args.BootstrapWalletAddress) == "" {
		args.BootstrapWalletAddress = strings.TrimSpace(os.Getenv("LESSER_BOOTSTRAP_WALLET_ADDRESS"))
	}
	if strings.TrimSpace(args.BootstrapWalletAddress) != "" {
		normalized, err := normalizeBootstrapWalletAddress(args.BootstrapWalletAddress)
		if err != nil {
			return upArgs{}, err
		}
		args.BootstrapWalletAddress = normalized
	}

	return args, nil
}

func normalizeBootstrapWalletAddress(input string) (string, error) {
	addr := strings.TrimSpace(input)
	if !ethcommon.IsHexAddress(addr) {
		return "", fmt.Errorf("invalid bootstrap wallet address %q", input)
	}
	return strings.ToLower(ethcommon.HexToAddress(addr).Hex()), nil
}
