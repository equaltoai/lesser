package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

const (
	clientInstallConfigFileName        = "facetheory.lesser.json"
	clientInstallManifestSchemaVersion = 1
	clientInstallManifestKind          = "facetheory-lesser-install"
	clientInstallBasePath              = "/l"
	clientInstallAssetsRoot            = "l/_assets"
	clientInstallDefaultManifestKey    = "install/current.json"
	clientInstallHistoryRoot           = "installs"
	clientArtifactBucketSuffix         = "client-artifacts"
)

type clientInstallArgs struct {
	App        string
	BaseDomain string
	AWSProfile string
	Stage      string
	StatePath  string
	ConfigPath string
	SkipBuild  bool
}

type faceTheoryLesserConfig struct {
	SchemaVersion int                          `json:"schema_version"`
	AppName       string                       `json:"app_name,omitempty"`
	DisplayName   string                       `json:"display_name,omitempty"`
	Version       string                       `json:"version,omitempty"`
	Build         *faceTheoryLesserBuildConfig `json:"build,omitempty"`
	Server        faceTheoryLesserServerConfig `json:"server"`
	Assets        faceTheoryLesserAssetsConfig `json:"assets"`
}

type faceTheoryLesserBuildConfig struct {
	Command []string `json:"command,omitempty"`
}

type faceTheoryLesserServerConfig struct {
	Dir    string `json:"dir"`
	Entry  string `json:"entry"`
	Export string `json:"export,omitempty"`
}

type faceTheoryLesserAssetsConfig struct {
	Dir string `json:"dir"`
}

type nodePackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

type clientInstallPackage struct {
	AppRoot     string
	AppName     string
	DisplayName string
	Version     string
	InstallID   string
	ServerDir   string
	AssetsDir   string
	Manifest    clientInstallManifest
}

type clientInstallManifest struct {
	SchemaVersion int                         `json:"schema_version"`
	Kind          string                      `json:"kind"`
	AppName       string                      `json:"app_name"`
	DisplayName   string                      `json:"display_name,omitempty"`
	Version       string                      `json:"version"`
	InstallID     string                      `json:"install_id"`
	BasePath      string                      `json:"base_path"`
	Server        clientInstallServerManifest `json:"server"`
	Assets        clientInstallAssetsManifest `json:"assets"`
}

type clientInstallServerManifest struct {
	Root       string   `json:"root,omitempty"`
	Entry      string   `json:"entry"`
	ExportName string   `json:"export_name"`
	Files      []string `json:"files"`
}

type clientInstallAssetsManifest struct {
	Root       string   `json:"root,omitempty"`
	PathPrefix string   `json:"path_prefix"`
	Files      []string `json:"files"`
}

type clientInstallCommand struct {
	appRoot            string
	configPath         string
	statePath          string
	serverRoot         string
	historyManifestKey string
	manifestContent    string
	receipt            *upReceipt
	stages             []naming.Stage
	pkgPlan            *clientInstallPackage
	s3Client           *s3.Client
	cfClient           *cloudfront.Client
}

type clientInstallStageTargets struct {
	stageReceipt   *stageReceipt
	stageKey       string
	assetBucket    string
	artifactBucket string
	manifestKey    string
	distributionID string
}

var installIDSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)

func runClientInstall(argv []string) error {
	args, err := parseClientInstallArgs(argv)
	if err != nil {
		return err
	}

	command, err := newClientInstallCommand(args)
	if err != nil {
		return err
	}

	logClientInstall(command)
	if err := publishClientInstallStages(context.Background(), command); err != nil {
		return err
	}

	return writeReceiptFn(command.statePath, command.receipt)
}

func newClientInstallCommand(args clientInstallArgs) (*clientInstallCommand, error) {
	app, baseDomain, awsProfile, err := normalizeClientInstallInputs(args)
	if err != nil {
		return nil, err
	}

	appRoot, configPath, pkgPlan, err := prepareClientInstallArtifacts(args)
	if err != nil {
		return nil, err
	}

	serverRoot, historyManifestKey, manifestContent, err := buildClientInstallManifestArtifact(pkgPlan)
	if err != nil {
		return nil, err
	}

	statePath, receipt, stages, err := resolveClientInstallReceipt(app, baseDomain, args)
	if err != nil {
		return nil, err
	}

	s3Client, cfClient, err := newClientInstallAWSClients(awsProfile)
	if err != nil {
		return nil, err
	}

	return &clientInstallCommand{
		appRoot:            appRoot,
		configPath:         configPath,
		statePath:          statePath,
		serverRoot:         serverRoot,
		historyManifestKey: historyManifestKey,
		manifestContent:    manifestContent,
		receipt:            receipt,
		stages:             stages,
		pkgPlan:            pkgPlan,
		s3Client:           s3Client,
		cfClient:           cfClient,
	}, nil
}

func normalizeClientInstallInputs(args clientInstallArgs) (string, string, string, error) {
	app, err := naming.NormalizeAppName(args.App)
	if err != nil {
		return "", "", "", err
	}

	baseDomain, err := normalizeBaseDomain(args.BaseDomain)
	if err != nil {
		return "", "", "", err
	}

	awsProfile := strings.TrimSpace(args.AWSProfile)

	return app, baseDomain, awsProfile, nil
}

func prepareClientInstallArtifacts(args clientInstallArgs) (string, string, *clientInstallPackage, error) {
	appRoot, configPath, err := resolveClientInstallRoot(args.ConfigPath)
	if err != nil {
		return "", "", nil, err
	}

	config, err := readFaceTheoryLesserConfig(configPath)
	if err != nil {
		return "", "", nil, err
	}

	pkg, err := readNodePackageJSON(filepath.Join(appRoot, "package.json"))
	if err != nil {
		return "", "", nil, err
	}
	if err := validateFaceTheoryPackage(pkg); err != nil {
		return "", "", nil, err
	}

	if !args.SkipBuild {
		if err := installNodeModulesIfNeeded(appRoot); err != nil {
			return "", "", nil, err
		}
		if err := runFaceTheoryBuild(context.Background(), appRoot, config); err != nil {
			return "", "", nil, err
		}
	}

	pkgPlan, err := prepareClientInstallPackage(appRoot, config, pkg)
	if err != nil {
		return "", "", nil, err
	}

	return appRoot, configPath, pkgPlan, nil
}

func buildClientInstallManifestArtifact(pkgPlan *clientInstallPackage) (string, string, string, error) {
	serverRoot := filepath.ToSlash(filepath.Join(clientInstallHistoryRoot, pkgPlan.InstallID, "server"))
	historyManifestKey := filepath.ToSlash(filepath.Join(clientInstallHistoryRoot, pkgPlan.InstallID, "manifest.json"))
	manifest := pkgPlan.Manifest
	manifest.Server.Root = serverRoot
	manifest.Assets.Root = clientInstallAssetsRoot

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", "", "", fmt.Errorf("marshal install manifest: %w", err)
	}

	return serverRoot, historyManifestKey, string(append(manifestJSON, '\n')), nil
}

func resolveClientInstallReceipt(app, baseDomain string, args clientInstallArgs) (string, *upReceipt, []naming.Stage, error) {
	statePath, err := resolveReceiptPath(app, baseDomain, args.StatePath)
	if err != nil {
		return "", nil, nil, err
	}

	receipt, err := readReceipt(statePath)
	if err != nil {
		return "", nil, nil, err
	}

	stages, err := parseStageSelection(args.Stage)
	if err != nil {
		return "", nil, nil, err
	}

	return statePath, receipt, stages, nil
}

func newClientInstallAWSClients(awsProfile string) (*s3.Client, *cloudfront.Client, error) {
	awsCfg, _, err := loadAWSConfigForCLIFn(context.Background(), awsProfile)
	if err != nil {
		return nil, nil, err
	}

	return s3.NewFromConfig(awsCfg), cloudfront.NewFromConfig(awsCfg), nil
}

func logClientInstall(command *clientInstallCommand) {
	fmt.Println("\nInstalling FaceTheory client:")
	fmt.Println("  app_repo:", command.appRoot)
	fmt.Println("  config:", command.configPath)
	fmt.Println("  receipt:", command.statePath)
	fmt.Println("  install_id:", command.pkgPlan.InstallID)
}

func publishClientInstallStages(ctx context.Context, command *clientInstallCommand) error {
	for _, stage := range command.stages {
		if err := publishClientInstallStage(ctx, command, stage); err != nil {
			return err
		}
	}
	return nil
}

func publishClientInstallStage(ctx context.Context, command *clientInstallCommand, stage naming.Stage) error {
	targets, err := resolveClientInstallStageTargets(command.receipt, stage)
	if err != nil {
		return err
	}

	fmt.Printf("\nPublishing client install (%s):\n", targets.stageKey)
	fmt.Printf("  artifact_bucket: s3://%s/\n", targets.artifactBucket)
	fmt.Printf("  asset_bucket:    s3://%s/\n", targets.assetBucket)

	if err := uploadDirWithPrefixFn(ctx, command.s3Client, targets.artifactBucket, command.serverRoot, command.pkgPlan.ServerDir); err != nil {
		return fmt.Errorf("upload SSR server bundle (%s): %w", targets.stageKey, err)
	}
	if err := uploadDirWithPrefixFn(ctx, command.s3Client, targets.assetBucket, clientInstallAssetsRoot, command.pkgPlan.AssetsDir); err != nil {
		return fmt.Errorf("upload SSR assets (%s): %w", targets.stageKey, err)
	}
	if err := putObjectStringFn(ctx, command.s3Client, targets.artifactBucket, command.historyManifestKey, command.manifestContent, "application/json; charset=utf-8", "no-store"); err != nil {
		return fmt.Errorf("upload install manifest history (%s): %w", targets.stageKey, err)
	}
	if err := putObjectStringFn(ctx, command.s3Client, targets.artifactBucket, targets.manifestKey, command.manifestContent, "application/json; charset=utf-8", "no-store"); err != nil {
		return fmt.Errorf("activate install manifest (%s): %w", targets.stageKey, err)
	}
	if err := invalidateClientPathsFn(ctx, command.cfClient, targets.distributionID); err != nil {
		return fmt.Errorf("cloudfront invalidation (%s): %w", targets.stageKey, err)
	}

	targets.stageReceipt.ClientInstall = &clientInstallReceipt{
		AppName:     command.pkgPlan.AppName,
		DisplayName: command.pkgPlan.DisplayName,
		Version:     command.pkgPlan.Version,
		InstallID:   command.pkgPlan.InstallID,
		ManifestKey: targets.manifestKey,
		ServerRoot:  command.serverRoot,
		AssetsRoot:  clientInstallAssetsRoot,
		InstalledAt: time.Now().UTC(),
	}

	return nil
}

func resolveClientInstallStageTargets(receipt *upReceipt, stage naming.Stage) (*clientInstallStageTargets, error) {
	stageKey := string(stage)
	stageReceipt := receipt.Stages[stageKey]
	if stageReceipt == nil {
		return nil, fmt.Errorf("receipt missing stage %q", stageKey)
	}

	assetBucket := strings.TrimSpace(stageReceipt.StackOutputs["ClientBucketName"])
	if assetBucket == "" {
		assetBucket = naming.S3BucketName(receipt.App, stage, "client", receipt.AccountID, receipt.Region)
	}

	artifactBucket := strings.TrimSpace(stageReceipt.StackOutputs["ClientArtifactBucketName"])
	if artifactBucket == "" {
		artifactBucket = naming.S3BucketName(receipt.App, stage, clientArtifactBucketSuffix, receipt.AccountID, receipt.Region)
	}

	manifestKey := strings.TrimSpace(stageReceipt.StackOutputs["ClientInstallManifestKey"])
	if manifestKey == "" {
		manifestKey = clientInstallDefaultManifestKey
	}

	distributionID := strings.TrimSpace(stageReceipt.StackOutputs["FrontendDistributionId"])
	if distributionID == "" {
		return nil, fmt.Errorf("missing FrontendDistributionId in receipt for stage %q", stageKey)
	}

	return &clientInstallStageTargets{
		stageReceipt:   stageReceipt,
		stageKey:       stageKey,
		assetBucket:    assetBucket,
		artifactBucket: artifactBucket,
		manifestKey:    manifestKey,
		distributionID: distributionID,
	}, nil
}

func parseClientInstallArgs(argv []string) (clientInstallArgs, error) {
	fs := flag.NewFlagSet("lesser client install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var args clientInstallArgs
	fs.StringVar(&args.App, "app", "", "app name slug (e.g. my-lesser)")
	fs.StringVar(&args.BaseDomain, "base-domain", "", "base domain with an existing public hosted zone (e.g. example.com)")
	fs.StringVar(&args.AWSProfile, "aws-profile", "", "AWS profile override (optional; defaults to the ambient AWS credential chain)")
	fs.StringVar(&args.Stage, "stage", "both", "stage to install (dev|live|staging|both|all)")
	fs.StringVar(&args.StatePath, "state", "", "path to deployment receipt (defaults to ~/.lesser/<app>/<base-domain>/state.json)")
	fs.StringVar(&args.ConfigPath, "config", "", "path to facetheory.lesser.json (defaults to searching from cwd upward)")
	fs.BoolVar(&args.SkipBuild, "skip-build", false, "skip running the app build step before packaging install artifacts")

	if err := fs.Parse(argv); err != nil {
		return clientInstallArgs{}, err
	}

	if strings.TrimSpace(args.App) == "" || strings.TrimSpace(args.BaseDomain) == "" {
		return clientInstallArgs{}, errors.New("required flags: --app, --base-domain")
	}

	return args, nil
}

func resolveClientInstallRoot(explicitConfigPath string) (string, string, error) {
	if clean := strings.TrimSpace(explicitConfigPath); clean != "" {
		path, err := filepath.Abs(clean)
		if err != nil {
			return "", "", fmt.Errorf("resolve client install config path: %w", err)
		}
		if !fileExists(path) {
			return "", "", fmt.Errorf("client install config not found at %s", path)
		}
		return filepath.Dir(path), path, nil
	}

	start, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	dir := start
	for {
		candidate := filepath.Join(dir, clientInstallConfigFileName)
		if fileExists(candidate) {
			return dir, candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", fmt.Errorf("unable to locate %s from %s", clientInstallConfigFileName, start)
}

func readFaceTheoryLesserConfig(path string) (*faceTheoryLesserConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-selected config path
	if err != nil {
		return nil, fmt.Errorf("read client install config %s: %w", path, err)
	}

	var cfg faceTheoryLesserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse client install config %s: %w", path, err)
	}

	if cfg.SchemaVersion != clientInstallManifestSchemaVersion {
		return nil, fmt.Errorf("unsupported client install config schema_version %d (expected %d)", cfg.SchemaVersion, clientInstallManifestSchemaVersion)
	}
	if strings.TrimSpace(cfg.Server.Dir) == "" || strings.TrimSpace(cfg.Server.Entry) == "" {
		return nil, errors.New("client install config requires server.dir and server.entry")
	}
	if strings.TrimSpace(cfg.Assets.Dir) == "" {
		return nil, errors.New("client install config requires assets.dir")
	}
	return &cfg, nil
}

func readNodePackageJSON(path string) (*nodePackageJSON, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- derived from located app root
	if err != nil {
		return nil, fmt.Errorf("read package.json %s: %w", path, err)
	}

	var pkg nodePackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse package.json %s: %w", path, err)
	}
	return &pkg, nil
}

func validateFaceTheoryPackage(pkg *nodePackageJSON) error {
	if pkg == nil {
		return errors.New("package.json is required")
	}
	if hasNodeDependency(pkg.Dependencies, "@theory-cloud/facetheory") || hasNodeDependency(pkg.DevDependencies, "@theory-cloud/facetheory") {
		return nil
	}
	return errors.New("package.json must include @theory-cloud/facetheory to use `lesser client install`")
}

func hasNodeDependency(deps map[string]string, name string) bool {
	if len(deps) == 0 {
		return false
	}
	_, ok := deps[name]
	return ok
}

func installNodeModulesIfNeeded(appRoot string) error {
	if info, err := os.Stat(filepath.Join(appRoot, "node_modules")); err == nil && info.IsDir() {
		return nil
	}

	fmt.Println("\nInstalling app dependencies...")
	if err := runCommandFn(context.Background(), "pnpm", []string{"install", "--frozen-lockfile"}, execOptions{Dir: appRoot}); err != nil {
		return fmt.Errorf("pnpm install (client app): %w", err)
	}
	return nil
}

func runFaceTheoryBuild(ctx context.Context, appRoot string, cfg *faceTheoryLesserConfig) error {
	command := []string{"pnpm", "build"}
	if cfg != nil && cfg.Build != nil && len(cfg.Build.Command) > 0 {
		command = append([]string(nil), cfg.Build.Command...)
	}
	if len(command) == 0 {
		return errors.New("client install build command is empty")
	}

	fmt.Println("\nBuilding FaceTheory app...")
	if err := runCommandFn(ctx, command[0], command[1:], execOptions{Dir: appRoot}); err != nil {
		return fmt.Errorf("build FaceTheory app: %w", err)
	}
	return nil
}

func prepareClientInstallPackage(appRoot string, cfg *faceTheoryLesserConfig, pkg *nodePackageJSON) (*clientInstallPackage, error) {
	serverDir := filepath.Clean(filepath.Join(appRoot, cfg.Server.Dir))
	assetsDir := filepath.Clean(filepath.Join(appRoot, cfg.Assets.Dir))
	serverEntry := filepath.ToSlash(strings.Trim(strings.TrimSpace(cfg.Server.Entry), "/"))
	exportName := strings.TrimSpace(cfg.Server.Export)
	if exportName == "" {
		exportName = "handler"
	}

	if _, err := os.Stat(filepath.Join(serverDir, filepath.FromSlash(serverEntry))); err != nil {
		return nil, fmt.Errorf("server entry is missing %s: %w", filepath.Join(serverDir, filepath.FromSlash(serverEntry)), err)
	}
	serverFiles, err := listFiles(serverDir)
	if err != nil {
		return nil, fmt.Errorf("list server files: %w", err)
	}
	assetFiles, err := listFiles(assetsDir)
	if err != nil {
		return nil, fmt.Errorf("list asset files: %w", err)
	}
	if len(assetFiles) == 0 {
		return nil, fmt.Errorf("asset directory %s is empty", assetsDir)
	}

	appName := strings.TrimSpace(cfg.AppName)
	if appName == "" {
		appName = strings.TrimSpace(pkg.Name)
	}
	if appName == "" {
		appName = "facetheory-client"
	}

	displayName := strings.TrimSpace(cfg.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(pkg.Name)
	}
	if displayName == "" {
		displayName = appName
	}

	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = strings.TrimSpace(pkg.Version)
	}
	if version == "" {
		version = "0.0.0"
	}

	installID := buildClientInstallID(appRoot, version)
	manifest := clientInstallManifest{
		SchemaVersion: clientInstallManifestSchemaVersion,
		Kind:          clientInstallManifestKind,
		AppName:       appName,
		DisplayName:   displayName,
		Version:       version,
		InstallID:     installID,
		BasePath:      clientInstallBasePath,
		Server: clientInstallServerManifest{
			Entry:      serverEntry,
			ExportName: exportName,
			Files:      relativePaths(serverFiles),
		},
		Assets: clientInstallAssetsManifest{
			PathPrefix: "/" + clientInstallAssetsRoot,
			Files:      relativePaths(assetFiles),
		},
	}

	return &clientInstallPackage{
		AppRoot:     appRoot,
		AppName:     appName,
		DisplayName: displayName,
		Version:     version,
		InstallID:   installID,
		ServerDir:   serverDir,
		AssetsDir:   assetsDir,
		Manifest:    manifest,
	}, nil
}

func relativePaths(files []localFile) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, filepath.ToSlash(file.RelativePath))
	}
	return out
}

func buildClientInstallID(appRoot string, version string) string {
	parts := []string{
		sanitizeInstallIDPart(version),
		time.Now().UTC().Format("20060102t150405z"),
	}
	if rev := gitShortRevision(appRoot); rev != "" {
		parts = append(parts, sanitizeInstallIDPart(rev))
	}

	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return "install-" + time.Now().UTC().Format("20060102t150405z")
	}
	return strings.Join(clean, "-")
}

func sanitizeInstallIDPart(value string) string {
	clean := strings.ToLower(strings.TrimSpace(value))
	clean = installIDSanitizer.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")
	return clean
}

func gitShortRevision(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--short=12", "HEAD") //nolint:gosec // local git helper for install metadata
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
