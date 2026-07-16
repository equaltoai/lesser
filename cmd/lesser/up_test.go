package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestParseUpArgs(t *testing.T) {
	_, err := parseUpArgs(nil)
	require.Error(t, err)

	args, err := parseUpArgs([]string{"--app", "app", "--base-domain", "example.com", "--aws-profile", "profile"})
	require.NoError(t, err)
	require.Equal(t, "app", args.App)

	t.Run("allows ambient credentials when aws-profile omitted", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "")
		args, err := parseUpArgs([]string{"--app", "app", "--base-domain", "example.com"})
		require.NoError(t, err)
		require.Empty(t, args.AWSProfile)
	})

	t.Run("normalizes bootstrap wallet flag", func(t *testing.T) {
		args, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--bootstrap-wallet-address", "0x1111111111111111111111111111111111111111",
		})
		require.NoError(t, err)
		require.Equal(t, "0x1111111111111111111111111111111111111111", args.BootstrapWalletAddress)
	})

	t.Run("reads bootstrap wallet from env", func(t *testing.T) {
		t.Setenv("LESSER_BOOTSTRAP_WALLET_ADDRESS", "0x2222222222222222222222222222222222222222")
		args, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
		})
		require.NoError(t, err)
		require.Equal(t, "0x2222222222222222222222222222222222222222", args.BootstrapWalletAddress)
	})

	t.Run("accepts release-dir", func(t *testing.T) {
		args, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--release-dir", "./dist/release",
		})
		require.NoError(t, err)
		require.Equal(t, "./dist/release", args.ReleaseDir)
	})

	t.Run("flag overrides env", func(t *testing.T) {
		t.Setenv("LESSER_BOOTSTRAP_WALLET_ADDRESS", "0x2222222222222222222222222222222222222222")
		args, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--bootstrap-wallet-address", "0x1111111111111111111111111111111111111111",
		})
		require.NoError(t, err)
		require.Equal(t, "0x1111111111111111111111111111111111111111", args.BootstrapWalletAddress)
	})

	t.Run("invalid bootstrap wallet errors", func(t *testing.T) {
		_, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--bootstrap-wallet-address", "not-an-address",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid bootstrap wallet address")
	})

	t.Run("rejects reserved bootstrap wallet", func(t *testing.T) {
		_, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--bootstrap-wallet-address", "0x80189edb676d51b2fb2257b2ad38e018b20ca46e",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "reserved")
	})

	t.Run("uses provisioning input when provided", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "provision.json")
		require.NoError(t, os.WriteFile(path, []byte(`{
  "schema": 1,
  "slug": "app",
  "stage": "dev",
  "admin_wallet_address": "0x3333333333333333333333333333333333333333",
  "admin_username": "app",
  "lesser_host_url": "https://lab.lesser.host",
  "lesser_host_instance_key_arn": "arn:aws:secretsmanager:us-east-1:123456789012:secret:instanceKey",
  "instance_plane_enabled": true,
  "api_cors_allowed_origins": "https://app.example.com",
  "translation_enabled": true,
  "tip_enabled": true,
  "tip_chain_id": 10,
  "tip_contract_address": "0xabc",
  "ai_enabled": true
}
`), 0o600))

		args, err := parseUpArgs([]string{
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--provisioning-input", path,
		})
		require.NoError(t, err)
		require.Equal(t, "app", args.App)
		require.Equal(t, "dev", args.Stage)
		require.Equal(t, "0x3333333333333333333333333333333333333333", args.BootstrapWalletAddress)
		require.Equal(t, "https://lab.lesser.host", args.LesserHostURL)
		require.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:instanceKey", args.LesserHostInstanceKeyARN)
		require.NotNil(t, args.InstancePlaneEnabled)
		require.True(t, *args.InstancePlaneEnabled)
		require.Equal(t, "https://app.example.com", args.APICORSAllowedOrigins)
		require.NotNil(t, args.TranslationEnabled)
		require.True(t, *args.TranslationEnabled)
		require.NotNil(t, args.TipEnabled)
		require.True(t, *args.TipEnabled)
		require.NotNil(t, args.TipChainID)
		require.Equal(t, 10, *args.TipChainID)
		require.Equal(t, "0xabc", args.TipContractAddress)
		require.NotNil(t, args.AIEnabled)
		require.True(t, *args.AIEnabled)
	})

	t.Run("api cors flag overrides environment", func(t *testing.T) {
		t.Setenv("API_CORS_ALLOWED_ORIGINS", "https://env.example.com")
		args, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--stage", "dev",
			"--bootstrap-wallet-address", "0x3333333333333333333333333333333333333333",
			"--api-cors-allowed-origins", "https://flag.example.com",
		})
		require.NoError(t, err)
		require.Equal(t, "https://flag.example.com", args.APICORSAllowedOrigins)
	})

	t.Run("api cors falls back to environment", func(t *testing.T) {
		t.Setenv("API_CORS_ALLOWED_ORIGINS", "https://env.example.com")
		args, err := parseUpArgs([]string{
			"--app", "app",
			"--base-domain", "example.com",
			"--stage", "dev",
			"--bootstrap-wallet-address", "0x3333333333333333333333333333333333333333",
		})
		require.NoError(t, err)
		require.Equal(t, "https://env.example.com", args.APICORSAllowedOrigins)
	})

	t.Run("rejects reserved wallet via provisioning input", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "provision.json")
		require.NoError(t, os.WriteFile(path, []byte(`{
  "schema": 1,
  "slug": "app",
  "stage": "dev",
  "admin_wallet_address": "0x1e14865a53a994b01b9ccfef42669dc0bfe98805",
  "admin_username": "app"
}
`), 0o600))

		_, err := parseUpArgs([]string{
			"--base-domain", "example.com",
			"--aws-profile", "profile",
			"--provisioning-input", path,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "reserved")
	})
}

func TestUpStages(t *testing.T) {
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageLive}, upStages(false))
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive}, upStages(true))
}

func TestSelectUpStages(t *testing.T) {
	t.Run("defaults when stage empty", func(t *testing.T) {
		got, err := selectUpStages(false, "")
		require.NoError(t, err)
		require.Equal(t, []naming.Stage{naming.StageDev, naming.StageLive}, got)
	})

	t.Run("stage cannot combine with with-staging", func(t *testing.T) {
		_, err := selectUpStages(true, "dev")
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be combined")
	})

	got, err := selectUpStages(false, "dev")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev}, got)

	got, err = selectUpStages(false, "staging")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageStaging}, got)

	got, err = selectUpStages(false, "live")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageLive}, got)

	_, err = selectUpStages(false, "wat")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid --stage")
}

func TestPrepareUpEnv_RequiresOutWhenBootstrapGenerated(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	previousWallet := determineBootstrapWalletFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
		determineBootstrapWalletFn = previousWallet
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	home := t.TempDir()
	userHomeDirFn = func() (string, error) { return home, nil }

	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "", true, nil
	}
	determineBootstrapWalletFn = func(string) (bootstrapWallet, error) {
		return bootstrapWallet{Address: "0xabc", Mnemonic: "mnemonic", DerivationPath: defaultBootstrapDerivationPath, ChainID: 1}, nil
	}

	_, err := prepareUpEnv(context.Background(), upArgs{App: "app", BaseDomain: "example.com", AWSProfile: "profile"})
	require.Error(t, err)
}

func TestPrepareUpEnv_UsesProvidedBootstrapWalletAddress(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	previousWallet := determineBootstrapWalletFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
		determineBootstrapWalletFn = previousWallet
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "", true, nil
	}
	determineBootstrapWalletFn = func(string) (bootstrapWallet, error) {
		t.Fatal("determineBootstrapWallet should not be called when --bootstrap-wallet-address is set")
		return bootstrapWallet{}, nil
	}

	env, err := prepareUpEnv(context.Background(), upArgs{
		App:                    "app",
		BaseDomain:             "example.com",
		AWSProfile:             "profile",
		BootstrapWalletAddress: "0x1111111111111111111111111111111111111111",
	})
	require.NoError(t, err)
	require.Equal(t, "0x1111111111111111111111111111111111111111", env.bootstrap.Address)
	require.Empty(t, env.bootstrap.Mnemonic)
}

func TestPrepareUpEnv_NormalizesReleaseDir(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0xabc", false, nil
	}

	releaseDir := filepath.Join(t.TempDir(), "release")
	require.NoError(t, os.MkdirAll(releaseDir, 0o755))

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Dir(releaseDir)))
	t.Cleanup(func() { require.NoError(t, os.Chdir(wd)) })

	env, err := prepareUpEnv(context.Background(), upArgs{
		App:        "app",
		BaseDomain: "example.com",
		AWSProfile: "profile",
		ReleaseDir: filepath.Base(releaseDir),
	})
	require.NoError(t, err)
	require.Equal(t, releaseDir, env.args.ReleaseDir)
}

func TestPrepareUpEnv_RejectsInvalidReleaseDir(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0xabc", false, nil
	}

	t.Run("missing directory", func(t *testing.T) {
		_, err := prepareUpEnv(context.Background(), upArgs{
			App:        "app",
			BaseDomain: "example.com",
			AWSProfile: "profile",
			ReleaseDir: filepath.Join(t.TempDir(), "missing"),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "stat --release-dir")
	})

	t.Run("file path", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "release.txt")
		require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))

		_, err := prepareUpEnv(context.Background(), upArgs{
			App:        "app",
			BaseDomain: "example.com",
			AWSProfile: "profile",
			ReleaseDir: filePath,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--release-dir must be a directory")
	})
}

func TestValidateUpDeployInputs(t *testing.T) {
	previousSource := validateDeploySourceInputsFn
	previousRelease := validateReleaseDeployInputsFn
	t.Cleanup(func() {
		validateDeploySourceInputsFn = previousSource
		validateReleaseDeployInputsFn = previousRelease
	})

	t.Run("release artifacts use release deploy requirements", func(t *testing.T) {
		validateDeploySourceInputsFn = func(string) error {
			t.Fatal("source deploy requirements should not run for release artifacts")
			return nil
		}
		validateReleaseDeployInputsFn = func(repoRoot string) error {
			require.Equal(t, "/repo", repoRoot)
			return nil
		}

		require.NoError(t, validateUpDeployInputs(upArgs{ReleaseDir: "/tmp/release"}, "/repo"))
	})

	t.Run("source deploys use source requirements", func(t *testing.T) {
		validateDeploySourceInputsFn = func(repoRoot string) error {
			require.Equal(t, "/repo", repoRoot)
			return nil
		}
		validateReleaseDeployInputsFn = func(string) error {
			t.Fatal("release deploy requirements should not run for source deploys")
			return nil
		}

		require.NoError(t, validateUpDeployInputs(upArgs{}, "/repo"))
	})
}

func TestReleaseAssemblyLegacyModeEnabled(t *testing.T) {
	previous, hadPrevious := os.LookupEnv(legacyReleaseAssemblyEnv)
	t.Cleanup(func() {
		if hadPrevious {
			require.NoError(t, os.Setenv(legacyReleaseAssemblyEnv, previous))
			return
		}
		require.NoError(t, os.Unsetenv(legacyReleaseAssemblyEnv))
	})

	require.NoError(t, os.Unsetenv(legacyReleaseAssemblyEnv))
	require.False(t, releaseAssemblyLegacyModeEnabled())

	require.NoError(t, os.Setenv(legacyReleaseAssemblyEnv, "1"))
	require.True(t, releaseAssemblyLegacyModeEnabled())

	require.NoError(t, os.Setenv(legacyReleaseAssemblyEnv, "true"))
	require.True(t, releaseAssemblyLegacyModeEnabled())
}

func TestUpEnv_Deploy_UsesLegacyReleaseAssemblyWhenEnabled(t *testing.T) {
	previousEnv, hadEnv := os.LookupEnv(legacyReleaseAssemblyEnv)
	previousCdkBootstrap := cdkBootstrapFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousDeploy := deployCloudFormationStackFn
	previousUpload := uploadReleaseAssemblyAssetsFn
	t.Cleanup(func() {
		if hadEnv {
			require.NoError(t, os.Setenv(legacyReleaseAssemblyEnv, previousEnv))
		} else {
			require.NoError(t, os.Unsetenv(legacyReleaseAssemblyEnv))
		}
		cdkBootstrapFn = previousCdkBootstrap
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		deployCloudFormationStackFn = previousDeploy
		uploadReleaseAssemblyAssetsFn = previousUpload
	})

	require.NoError(t, os.Setenv(legacyReleaseAssemblyEnv, "1"))

	assets := stubReleaseDeployAssetsInstallResult(t, t.TempDir())
	cdkBootstrapFn = func(context.Context, string, string, string, string) error {
		t.Fatal("legacy release assembly mode should bypass CDK deploy")
		return nil
	}
	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }
	deployCloudFormationStackFn = func(_ context.Context, _ aws.Config, req cloudFormationDeployRequest) (map[string]string, error) {
		if req.StackName == naming.SharedStackName("app") {
			return map[string]string{"ReleaseAssetBucketName": "release-assets"}, nil
		}
		return map[string]string{}, nil
	}
	uploadReleaseAssemblyAssetsFn = func(context.Context, aws.Config, string, string, string, string, releaseDeployAssemblyInstallResult) (releaseAssemblyUploadResult, error) {
		return releaseAssemblyUploadResult{
			StageTemplateURLs: map[naming.Stage]string{
				naming.StageDev: "https://example.invalid/dev",
			},
		}, nil
	}

	env := &upEnv{
		args:            upArgs{ReleaseDir: "/tmp/release"},
		app:             "app",
		baseDomain:      "example.com",
		awsProfile:      "profile",
		accountID:       "123456789012",
		awsCfg:          aws.Config{Region: "us-east-1"},
		hostedZone:      hostedZone{ID: "Z1", Name: "example.com"},
		stages:          []naming.Stage{naming.StageDev},
		releaseVersion:  assets.Version,
		releaseGitSHA:   assets.GitSHA,
		releaseAssembly: &assets.Assembly,
	}

	receipt, err := env.deploy(context.Background())
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, naming.StageStackName("app", naming.StageDev), receipt.Stages[string(naming.StageDev)].StackName)
}

func TestPrepareUpEnv_ProvidedBootstrapWalletMustMatchDeployed(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }
	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0x2222222222222222222222222222222222222222", false, nil
	}

	_, err := prepareUpEnv(context.Background(), upArgs{
		App:                    "app",
		BaseDomain:             "example.com",
		AWSProfile:             "profile",
		BootstrapWalletAddress: "0x1111111111111111111111111111111111111111",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match deployed bootstrap address")
}

func TestRunUp_HappyPathWithStubs(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	previousTools := ensureToolsAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousPrepareAssetRoot := prepareLambdaAssetRootFn
	previousStageLocalAssets := stageLocalLambdaAssetsFn
	previousBootstrap := ensureStageBootstrapStateFn
	previousCdkBootstrap := cdkBootstrapFn
	previousCdkDeploy := cdkDeployWithOutputsFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousWriteReceipt := writeReceiptFn
	previousBuildAuthUI := buildAuthUIFn
	previousReplaceBucket := replaceBucketWithDirPrefixFn
	previousInvalidate := invalidateFrontendFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
		ensureToolsAvailableFn = previousTools
		buildLambdaZipsFn = previousBuildZips
		prepareLambdaAssetRootFn = previousPrepareAssetRoot
		stageLocalLambdaAssetsFn = previousStageLocalAssets
		ensureStageBootstrapStateFn = previousBootstrap
		cdkBootstrapFn = previousCdkBootstrap
		cdkDeployWithOutputsFn = previousCdkDeploy
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		writeReceiptFn = previousWriteReceipt
		buildAuthUIFn = previousBuildAuthUI
		replaceBucketWithDirPrefixFn = previousReplaceBucket
		invalidateFrontendFn = previousInvalidate
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }

	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0xabc", false, nil
	}

	ensureToolsAvailableFn = func() error { return nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }
	prepareLambdaAssetRootFn = func(string) (string, error) { return filepath.Join(t.TempDir(), "lambda-assets"), nil }
	stageLocalLambdaAssetsFn = func(string, string) ([]string, error) { return nil, nil }
	cdkBootstrapFn = func(context.Context, string, string, string, string) error { return nil }
	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }

	cdkDeployWithOutputsFn = func(_ context.Context, _ string, _ string, req cdkDeployRequest) (cdkDeployResult, error) {
		return cdkDeployResult{StackName: req.StackName, Outputs: map[string]string{"FrontendDistributionId": "DIST"}}, nil
	}

	ensureStageBootstrapStateFn = func(context.Context, bootstrapDBFactory, string, naming.Stage, string) (stageBootstrapState, error) {
		return stageBootstrapState{Locked: true, Address: "0xabc", Updated: true}, nil
	}

	buildAuthUIFn = func(string) (string, error) { return t.TempDir(), nil }
	replaceBucketWithDirPrefixFn = func(context.Context, s3BucketUploaderAPI, string, string, string) error { return nil }
	invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }

	var wrotePath string
	var wroteReceipt *upReceipt
	writeReceiptFn = func(path string, receipt *upReceipt) error {
		wrotePath = path
		wroteReceipt = receipt
		return nil
	}

	require.NoError(t, runUp([]string{"--app", "app", "--base-domain", "example.com", "--aws-profile", "profile"}))
	require.NotEmpty(t, wrotePath)
	require.NotNil(t, wroteReceipt)
	require.Contains(t, wrotePath, filepath.Join(".lesser", "app", "example.com", "state.json"))
	require.Contains(t, wroteReceipt.Stages, "dev")
}

func TestUpEnvDeployFromSource_PassesInstancePlaneContext(t *testing.T) {
	previousCdkBootstrap := cdkBootstrapFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousCdkDeploy := cdkDeployWithOutputsFn
	t.Cleanup(func() {
		cdkBootstrapFn = previousCdkBootstrap
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		cdkDeployWithOutputsFn = previousCdkDeploy
	})

	cdkBootstrapFn = func(context.Context, string, string, string, string) error { return nil }
	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }

	boolValue := func(v bool) *bool { return &v }
	stringValue := func(v string) *string { return &v }

	for _, tc := range []struct {
		name string
		args upArgs
		env  string
		want *string
	}{
		{
			name: "omits when not explicitly set",
			env:  "",
			want: nil,
		},
		{
			name: "passes true from managed args",
			args: upArgs{InstancePlaneEnabled: boolValue(true)},
			env:  "",
			want: stringValue("true"),
		},
		{
			name: "passes false from managed args",
			args: upArgs{InstancePlaneEnabled: boolValue(false)},
			env:  "",
			want: stringValue("false"),
		},
		{
			name: "passes true from environment",
			env:  "yes",
			want: stringValue("true"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("INSTANCE_PLANE_ENABLED", tc.env)

			var requests []cdkDeployRequest
			cdkDeployWithOutputsFn = func(_ context.Context, _ string, _ string, req cdkDeployRequest) (cdkDeployResult, error) {
				requests = append(requests, req)
				return cdkDeployResult{StackName: req.StackName, Outputs: map[string]string{}}, nil
			}

			env := &upEnv{
				args:            tc.args,
				repoRoot:        t.TempDir(),
				lambdaAssetRoot: filepath.Join(t.TempDir(), "lambda-assets"),
				app:             "app",
				baseDomain:      "example.com",
				awsProfile:      "profile",
				accountID:       "123456789012",
				awsCfg:          aws.Config{Region: "us-east-1"},
				hostedZone:      hostedZone{ID: "Z1", Name: "example.com"},
				stages:          []naming.Stage{naming.StageDev},
				stateDir:        t.TempDir(),
			}

			_, err := env.deployFromSource(context.Background())
			require.NoError(t, err)
			require.Len(t, requests, 2)

			for _, req := range requests {
				require.Equal(t, "true", req.Contexts["bodyEnabled"])
				got, ok := req.Contexts["instancePlaneEnabled"]
				if tc.want == nil {
					require.False(t, ok)
					continue
				}
				require.True(t, ok)
				require.Equal(t, *tc.want, got)
			}
		})
	}
}

func TestUpEnv_Run_UsesReleaseDirLambdaArtifacts(t *testing.T) {
	previousTools := ensureToolsAvailableFn
	previousReleaseTools := ensureReleaseDeployToolsAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousInstallRelease := installReleaseDeployAssetsFn
	previousWriteBootstrap := writeBootstrapKeyMaterialFn
	previousBootstrap := ensureStageBootstrapStateFn
	previousCdkBootstrap := cdkBootstrapFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousCdkDeploy := cdkDeployWithOutputsFn
	previousCloudFormationDeploy := deployCloudFormationStackFn
	previousUploadAssembly := uploadReleaseAssemblyAssetsFn
	previousBuildAuthUI := buildAuthUIFn
	previousReplaceBucket := replaceBucketWithDirPrefixFn
	previousInvalidate := invalidateFrontendFn
	previousWriteReceipt := writeReceiptFn
	t.Cleanup(func() {
		ensureToolsAvailableFn = previousTools
		ensureReleaseDeployToolsAvailableFn = previousReleaseTools
		buildLambdaZipsFn = previousBuildZips
		installReleaseDeployAssetsFn = previousInstallRelease
		writeBootstrapKeyMaterialFn = previousWriteBootstrap
		ensureStageBootstrapStateFn = previousBootstrap
		cdkBootstrapFn = previousCdkBootstrap
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		cdkDeployWithOutputsFn = previousCdkDeploy
		deployCloudFormationStackFn = previousCloudFormationDeploy
		uploadReleaseAssemblyAssetsFn = previousUploadAssembly
		buildAuthUIFn = previousBuildAuthUI
		replaceBucketWithDirPrefixFn = previousReplaceBucket
		invalidateFrontendFn = previousInvalidate
		writeReceiptFn = previousWriteReceipt
	})

	env := &upEnv{
		args: upArgs{
			ReleaseDir: "/tmp/release",
		},
		repoRoot:   t.TempDir(),
		app:        "app",
		baseDomain: "example.com",
		awsProfile: "profile",
		awsCfg:     aws.Config{Region: "us-east-1"},
		accountID:  "123456789012",
		hostedZone: hostedZone{ID: "Z1", Name: "example.com"},
		stages:     []naming.Stage{naming.StageDev},
		stateDir:   t.TempDir(),
	}

	ensureToolsAvailableFn = func() error { return nil }
	ensureReleaseDeployToolsAvailableFn = func() error { return nil }
	buildLambdaZipsFn = func(string, bool) error {
		t.Fatal("buildLambdaZips should not run when --release-dir is provided")
		return nil
	}
	installReleaseDeployAssetsFn = func(releaseDir string, workspaceRoot string) (releaseDeployAssetsInstallResult, error) {
		require.Equal(t, "/tmp/release", releaseDir)
		return stubReleaseDeployAssetsInstallResult(t, workspaceRoot), nil
	}
	writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return nil }
	cdkBootstrapFn = func(context.Context, string, string, string, string) error { return nil }
	ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }
	deployCloudFormationStackFn = func(context.Context, aws.Config, cloudFormationDeployRequest) (map[string]string, error) {
		t.Fatal("release-dir deploy should not execute the release assembly path")
		return nil, nil
	}
	uploadReleaseAssemblyAssetsFn = func(context.Context, aws.Config, string, string, string, string, releaseDeployAssemblyInstallResult) (releaseAssemblyUploadResult, error) {
		t.Fatal("release-dir deploy should not upload release assembly templates")
		return releaseAssemblyUploadResult{}, nil
	}
	cdkDeployWithOutputsFn = func(_ context.Context, _ string, _ string, req cdkDeployRequest) (cdkDeployResult, error) {
		return cdkDeployResult{
			StackName: req.StackName,
			Outputs: map[string]string{
				"AuthUIBucketName":       "auth-ui-bucket",
				"FrontendDistributionId": "DIST",
			},
		}, nil
	}
	buildAuthUIFn = func(string) (string, error) { return t.TempDir(), nil }
	replaceBucketWithDirPrefixFn = func(context.Context, s3BucketUploaderAPI, string, string, string) error { return nil }
	invalidateFrontendFn = func(context.Context, *cloudfront.Client, string) error { return nil }
	writeReceiptFn = func(string, *upReceipt) error { return nil }
	ensureStageBootstrapStateFn = func(context.Context, bootstrapDBFactory, string, naming.Stage, string) (stageBootstrapState, error) {
		return stageBootstrapState{Locked: true, Address: "0xabc"}, nil
	}

	require.NoError(t, env.run(context.Background()))

	require.NotEmpty(t, env.lambdaAssetRoot)
	data, err := os.ReadFile(filepath.Join(env.lambdaAssetRoot, lambdaAssetMetadataFileName))
	require.NoError(t, err)
	require.Contains(t, string(data), `"mode": "release"`)
	require.Contains(t, string(data), `"release_version": "v1.2.3"`)
}

func TestUpEnv_PrepareLambdaArtifacts_RebuildOverridesReleaseDir(t *testing.T) {
	previousBuildZips := buildLambdaZipsFn
	previousInstallRelease := installReleaseDeployAssetsFn
	previousPrepareAssetRoot := prepareLambdaAssetRootFn
	previousStageLocalAssets := stageLocalLambdaAssetsFn
	t.Cleanup(func() {
		buildLambdaZipsFn = previousBuildZips
		installReleaseDeployAssetsFn = previousInstallRelease
		prepareLambdaAssetRootFn = previousPrepareAssetRoot
		stageLocalLambdaAssetsFn = previousStageLocalAssets
	})

	env := &upEnv{
		args: upArgs{
			ReleaseDir:     "/tmp/release",
			RebuildLambdas: true,
		},
		repoRoot: t.TempDir(),
	}

	buildCalled := false
	buildLambdaZipsFn = func(repoRoot string, force bool) error {
		buildCalled = true
		require.Equal(t, env.repoRoot, repoRoot)
		require.True(t, force)
		return nil
	}
	prepareLambdaAssetRootFn = func(string) (string, error) { return filepath.Join(t.TempDir(), "lambda-assets"), nil }
	stageLocalLambdaAssetsFn = func(repoRoot string, assetRoot string) ([]string, error) {
		require.Equal(t, env.repoRoot, repoRoot)
		require.NotEmpty(t, assetRoot)
		return []string{filepath.Join(assetRoot, "bin", "api.zip")}, nil
	}
	installReleaseDeployAssetsFn = func(string, string) (releaseDeployAssetsInstallResult, error) {
		t.Fatal("release asset installer should not run when --rebuild-lambdas is set")
		return releaseDeployAssetsInstallResult{}, nil
	}

	require.NoError(t, env.prepareLambdaArtifacts())
	require.True(t, buildCalled)

	data, err := os.ReadFile(filepath.Join(env.lambdaAssetRoot, lambdaAssetMetadataFileName))
	require.NoError(t, err)
	require.Contains(t, string(data), `"mode": "source"`)
}

func TestUpEnv_PrepareLambdaArtifacts_PropagatesAssetRootError(t *testing.T) {
	previousPrepareAssetRoot := prepareLambdaAssetRootFn
	t.Cleanup(func() {
		prepareLambdaAssetRootFn = previousPrepareAssetRoot
	})

	prepareLambdaAssetRootFn = func(string) (string, error) { return "", errSentinel }

	env := &upEnv{
		args:     upArgs{},
		stateDir: t.TempDir(),
	}

	require.ErrorIs(t, env.prepareLambdaArtifacts(), errSentinel)
}

func TestUpEnv_PrepareLambdaArtifacts_PropagatesBuildError(t *testing.T) {
	previousPrepareAssetRoot := prepareLambdaAssetRootFn
	previousBuildZips := buildLambdaZipsFn
	t.Cleanup(func() {
		prepareLambdaAssetRootFn = previousPrepareAssetRoot
		buildLambdaZipsFn = previousBuildZips
	})

	prepareLambdaAssetRootFn = func(string) (string, error) { return filepath.Join(t.TempDir(), "lambda-assets"), nil }
	buildLambdaZipsFn = func(string, bool) error { return errSentinel }

	env := &upEnv{
		args:     upArgs{},
		repoRoot: t.TempDir(),
		stateDir: t.TempDir(),
	}

	require.ErrorIs(t, env.prepareLambdaArtifacts(), errSentinel)
}

func TestUpEnv_PrepareLambdaArtifacts_PropagatesStageLocalError(t *testing.T) {
	previousPrepareAssetRoot := prepareLambdaAssetRootFn
	previousBuildZips := buildLambdaZipsFn
	previousStageLocalAssets := stageLocalLambdaAssetsFn
	t.Cleanup(func() {
		prepareLambdaAssetRootFn = previousPrepareAssetRoot
		buildLambdaZipsFn = previousBuildZips
		stageLocalLambdaAssetsFn = previousStageLocalAssets
	})

	prepareLambdaAssetRootFn = func(string) (string, error) { return filepath.Join(t.TempDir(), "lambda-assets"), nil }
	buildLambdaZipsFn = func(string, bool) error { return nil }
	stageLocalLambdaAssetsFn = func(string, string) ([]string, error) { return nil, errSentinel }

	env := &upEnv{
		args:     upArgs{},
		repoRoot: t.TempDir(),
		stateDir: t.TempDir(),
	}

	require.ErrorIs(t, env.prepareLambdaArtifacts(), errSentinel)
}

func TestUpEnv_PrepareLambdaArtifacts_PropagatesReleaseRelativePathError(t *testing.T) {
	previousInstallRelease := installReleaseDeployAssetsFn
	t.Cleanup(func() {
		installReleaseDeployAssetsFn = previousInstallRelease
	})

	installReleaseDeployAssetsFn = func(_ string, workspaceRoot string) (releaseDeployAssetsInstallResult, error) {
		result := stubReleaseDeployAssetsInstallResult(t, workspaceRoot)
		result.LambdaFiles = []string{filepath.Join(t.TempDir(), "outside.zip")}
		return result, nil
	}

	env := &upEnv{
		args:     upArgs{ReleaseDir: "/tmp/release"},
		repoRoot: t.TempDir(),
		stateDir: t.TempDir(),
	}

	err := env.prepareLambdaArtifacts()
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes asset root")
}

func TestUpEnv_PrepareLambdaArtifacts_PropagatesReleaseMetadataWriteError(t *testing.T) {
	previousInstallRelease := installReleaseDeployAssetsFn
	t.Cleanup(func() {
		installReleaseDeployAssetsFn = previousInstallRelease
	})

	assetRoot := filepath.Join(t.TempDir(), "deploy", "lambda-assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetRoot, lambdaAssetMetadataFileName), 0o755))
	installReleaseDeployAssetsFn = func(_ string, workspaceRoot string) (releaseDeployAssetsInstallResult, error) {
		result := stubReleaseDeployAssetsInstallResult(t, workspaceRoot)
		result.LambdaAssetRoot = assetRoot
		result.LambdaFiles = []string{filepath.Join(assetRoot, "bin", "api.zip")}
		return result, nil
	}

	env := &upEnv{
		args:     upArgs{ReleaseDir: "/tmp/release"},
		repoRoot: t.TempDir(),
		stateDir: t.TempDir(),
	}

	err := env.prepareLambdaArtifacts()
	require.Error(t, err)
	require.Contains(t, err.Error(), "write lambda asset metadata")
}

func stubReleaseDeployAssetsInstallResult(t *testing.T, workspaceRoot string) releaseDeployAssetsInstallResult {
	t.Helper()

	lambdaAssetRoot := filepath.Join(workspaceRoot, "lambda-assets")
	authUIDir := filepath.Join(workspaceRoot, "auth-ui")
	assemblyRoot := filepath.Join(workspaceRoot, "deploy-assembly")
	require.NoError(t, os.MkdirAll(filepath.Join(lambdaAssetRoot, "bin"), 0o755))
	require.NoError(t, os.MkdirAll(authUIDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(assemblyRoot, "templates"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lambdaAssetRoot, "bin", "api.zip"), []byte("api zip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(authUIDir, "index.html"), []byte("<html/>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(assemblyRoot, "templates", "lesser-shared.template.json"), []byte(`{"Parameters":{"AppSlug":{"Type":"String"}}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(assemblyRoot, "templates", "lesser-managed-dev.template.json"), []byte(`{"Parameters":{"AppSlug":{"Type":"String"}}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(assemblyRoot, "templates", "lesser-managed-staging.template.json"), []byte(`{"Parameters":{"AppSlug":{"Type":"String"}}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(assemblyRoot, "templates", "lesser-managed-live.template.json"), []byte(`{"Parameters":{"AppSlug":{"Type":"String"}}}`), 0o644))

	return releaseDeployAssetsInstallResult{
		Version:         "v1.2.3",
		GitSHA:          "0123456789abcdef0123456789abcdef01234567",
		LambdaAssetRoot: lambdaAssetRoot,
		LambdaFiles:     []string{filepath.Join(lambdaAssetRoot, "bin", "api.zip")},
		AuthUIDir:       authUIDir,
		Assembly: releaseDeployAssemblyInstallResult{
			RootDir:        assemblyRoot,
			SharedTemplate: filepath.Join(assemblyRoot, "templates", "lesser-shared.template.json"),
			StageTemplates: map[naming.Stage]string{
				naming.StageDev:     filepath.Join(assemblyRoot, "templates", "lesser-managed-dev.template.json"),
				naming.StageStaging: filepath.Join(assemblyRoot, "templates", "lesser-managed-staging.template.json"),
				naming.StageLive:    filepath.Join(assemblyRoot, "templates", "lesser-managed-live.template.json"),
			},
		},
	}
}

func TestUpEnv_HandleBootstrapOutput_WritesWhenConfigured(t *testing.T) {
	previous := writeBootstrapKeyMaterialFn
	t.Cleanup(func() { writeBootstrapKeyMaterialFn = previous })

	var wrotePath string
	writeBootstrapKeyMaterialFn = func(path string, wallet bootstrapWallet) error {
		wrotePath = path
		require.NotEmpty(t, wallet.Mnemonic)
		return nil
	}

	env := &upEnv{
		args:      upArgs{OutPath: "/tmp/bootstrap.json"},
		stateDir:  t.TempDir(),
		bootstrap: bootstrapWallet{Address: "0xabc", Mnemonic: "mnemonic", DerivationPath: defaultBootstrapDerivationPath, ChainID: 1},
	}

	require.NoError(t, env.handleBootstrapOutput())
	require.Equal(t, "/tmp/bootstrap.json", wrotePath)

	// No out path -> no write.
	wrotePath = ""
	env.args.OutPath = ""
	require.NoError(t, env.handleBootstrapOutput())
	require.Empty(t, wrotePath)

	// No mnemonic -> no write.
	env.bootstrap.Mnemonic = ""
	env.args.OutPath = "/tmp/bootstrap.json"
	require.NoError(t, env.handleBootstrapOutput())
	require.Empty(t, wrotePath)
}

func TestUpEnv_HandleBootstrapOutput_PropagatesWriteError(t *testing.T) {
	previous := writeBootstrapKeyMaterialFn
	t.Cleanup(func() { writeBootstrapKeyMaterialFn = previous })

	writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return errSentinel }

	env := &upEnv{
		args:      upArgs{OutPath: "/tmp/bootstrap.json"},
		bootstrap: bootstrapWallet{Address: "0xabc", Mnemonic: "mnemonic", DerivationPath: defaultBootstrapDerivationPath, ChainID: 1},
	}

	require.ErrorIs(t, env.handleBootstrapOutput(), errSentinel)
}

var errSentinel = errors.New("sentinel")

func TestPrepareUpEnv_OutPathLoadsLocalBootstrapMaterial(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousMkdir := mkdirAllFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	previousRead := readBootstrapKeyMaterialFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		mkdirAllFn = previousMkdir
		loadAWSConfigFromProfileFn = previousLoadAWS
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
		readBootstrapKeyMaterialFn = previousRead
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	home := t.TempDir()
	userHomeDirFn = func() (string, error) { return home, nil }
	mkdirAllFn = os.MkdirAll

	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0xabc", false, nil
	}

	args := upArgs{
		App:        "app",
		BaseDomain: "example.com",
		AWSProfile: "profile",
		OutPath:    "/tmp/bootstrap.json",
	}

	t.Run("missing local material is error", func(t *testing.T) {
		readBootstrapKeyMaterialFn = func(string) (bootstrapWallet, error) {
			t.Fatal("unexpected readBootstrapKeyMaterial call")
			return bootstrapWallet{}, nil
		}

		_, err := prepareUpEnv(context.Background(), args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--out requires local bootstrap key material")
	})

	t.Run("address mismatch is error", func(t *testing.T) {
		stateDir := filepath.Join(home, ".lesser", "app", "example.com")
		require.NoError(t, os.MkdirAll(stateDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "bootstrap.json"), []byte("x"), 0o600))

		readBootstrapKeyMaterialFn = func(string) (bootstrapWallet, error) {
			return bootstrapWallet{Address: "0xdef", Mnemonic: "mnemonic", DerivationPath: defaultBootstrapDerivationPath, ChainID: 1}, nil
		}

		_, err := prepareUpEnv(context.Background(), args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not match deployed bootstrap address")
	})

	t.Run("loads local material on match", func(t *testing.T) {
		stateDir := filepath.Join(home, ".lesser", "app", "example.com")
		require.NoError(t, os.MkdirAll(stateDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, "bootstrap.json"), []byte("x"), 0o600))

		readBootstrapKeyMaterialFn = func(path string) (bootstrapWallet, error) {
			require.Contains(t, path, "bootstrap.json")
			return bootstrapWallet{Address: "0xabc", Mnemonic: "mnemonic", DerivationPath: defaultBootstrapDerivationPath, ChainID: 1}, nil
		}

		env, err := prepareUpEnv(context.Background(), args)
		require.NoError(t, err)
		require.Equal(t, "mnemonic", env.bootstrap.Mnemonic)
	})
}

func TestPrepareUpEnv_PropagatesDependencyErrors(t *testing.T) {
	previousRepoRoot := findRepoRootFn
	previousHome := userHomeDirFn
	previousMkdir := mkdirAllFn
	previousLoadAWS := loadAWSConfigFromProfileFn
	previousLoadAWSCLI := loadAWSConfigForCLIFn
	previousAccount := resolveAWSAccountIDFn
	previousZone := resolveHostedZoneFn
	previousInspect := inspectBootstrapRequirementsFn
	previousWallet := determineBootstrapWalletFn
	t.Cleanup(func() {
		findRepoRootFn = previousRepoRoot
		userHomeDirFn = previousHome
		mkdirAllFn = previousMkdir
		loadAWSConfigFromProfileFn = previousLoadAWS
		loadAWSConfigForCLIFn = previousLoadAWSCLI
		resolveAWSAccountIDFn = previousAccount
		resolveHostedZoneFn = previousZone
		inspectBootstrapRequirementsFn = previousInspect
		determineBootstrapWalletFn = previousWallet
	})

	findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	userHomeDirFn = func() (string, error) { return t.TempDir(), nil }
	mkdirAllFn = os.MkdirAll

	loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
		return hostedZone{ID: "Z1", Name: "example.com"}, nil
	}
	inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
		return "0xabc", false, nil
	}

	base := upArgs{App: "app", BaseDomain: "example.com", AWSProfile: "profile"}

	t.Run("repo root error", func(t *testing.T) {
		findRepoRootFn = func() (string, error) { return "", errSentinel }
		_, err := prepareUpEnv(context.Background(), base)
		require.ErrorIs(t, err, errSentinel)
		findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }
	})

	t.Run("invalid app name", func(t *testing.T) {
		_, err := prepareUpEnv(context.Background(), upArgs{App: "APP!", BaseDomain: "example.com", AWSProfile: "profile"})
		require.Error(t, err)
	})

	t.Run("invalid base domain", func(t *testing.T) {
		_, err := prepareUpEnv(context.Background(), upArgs{App: "app", BaseDomain: "example.com/", AWSProfile: "profile"})
		require.Error(t, err)
	})

	t.Run("allows ambient credentials when profile empty", func(t *testing.T) {
		loadAWSConfigForCLIFn = func(_ context.Context, profile string) (aws.Config, string, error) {
			require.Empty(t, strings.TrimSpace(profile))
			return aws.Config{Region: "us-east-1"}, "", nil
		}
		_, err := prepareUpEnv(context.Background(), upArgs{App: "app", BaseDomain: "example.com", AWSProfile: " "})
		require.NoError(t, err)
		loadAWSConfigForCLIFn = previousLoadAWSCLI
	})

	t.Run("load aws config error", func(t *testing.T) {
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{}, errSentinel }
		_, err := prepareUpEnv(context.Background(), base)
		require.ErrorIs(t, err, errSentinel)
		loadAWSConfigFromProfileFn = func(context.Context, string) (aws.Config, error) { return aws.Config{Region: "us-east-1"}, nil }
	})

	t.Run("resolve account error", func(t *testing.T) {
		resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "", errSentinel }
		_, err := prepareUpEnv(context.Background(), base)
		require.ErrorIs(t, err, errSentinel)
		resolveAWSAccountIDFn = func(context.Context, aws.Config) (string, error) { return "123456789012", nil }
	})

	t.Run("resolve hosted zone error", func(t *testing.T) {
		resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) { return hostedZone{}, errSentinel }
		_, err := prepareUpEnv(context.Background(), base)
		require.ErrorIs(t, err, errSentinel)
		resolveHostedZoneFn = func(context.Context, aws.Config, string) (hostedZone, error) {
			return hostedZone{ID: "Z1", Name: "example.com"}, nil
		}
	})

	t.Run("inspect bootstrap requirements error", func(t *testing.T) {
		inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
			return "", false, errSentinel
		}
		_, err := prepareUpEnv(context.Background(), base)
		require.ErrorIs(t, err, errSentinel)
		inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
			return "0xabc", false, nil
		}
	})

	t.Run("determine bootstrap wallet error", func(t *testing.T) {
		inspectBootstrapRequirementsFn = func(context.Context, bootstrapDBFactory, string, []naming.Stage) (string, bool, error) {
			return "", true, nil
		}
		determineBootstrapWalletFn = func(string) (bootstrapWallet, error) { return bootstrapWallet{}, errSentinel }
		_, err := prepareUpEnv(context.Background(), base)
		require.ErrorIs(t, err, errSentinel)
	})
}

func TestUpEnv_Run_ErrorPropagation(t *testing.T) {
	previousTools := ensureToolsAvailableFn
	previousReleaseTools := ensureReleaseDeployToolsAvailableFn
	previousBuildZips := buildLambdaZipsFn
	previousInstallRelease := installReleaseDeployAssetsFn
	previousPrepareAssetRoot := prepareLambdaAssetRootFn
	previousStageLocalAssets := stageLocalLambdaAssetsFn
	previousWriteBootstrap := writeBootstrapKeyMaterialFn
	previousCdkBootstrap := cdkBootstrapFn
	previousAPIGW := ensureAPIGatewayCloudWatchLogsRoleFn
	previousCdkDeploy := cdkDeployWithOutputsFn
	previousDeployUI := buildAuthUIFn
	t.Cleanup(func() {
		ensureToolsAvailableFn = previousTools
		ensureReleaseDeployToolsAvailableFn = previousReleaseTools
		buildLambdaZipsFn = previousBuildZips
		installReleaseDeployAssetsFn = previousInstallRelease
		prepareLambdaAssetRootFn = previousPrepareAssetRoot
		stageLocalLambdaAssetsFn = previousStageLocalAssets
		writeBootstrapKeyMaterialFn = previousWriteBootstrap
		cdkBootstrapFn = previousCdkBootstrap
		ensureAPIGatewayCloudWatchLogsRoleFn = previousAPIGW
		cdkDeployWithOutputsFn = previousCdkDeploy
		buildAuthUIFn = previousDeployUI
	})

	baseEnv := func() *upEnv {
		return &upEnv{
			args:       upArgs{OutPath: "/tmp/bootstrap.json"},
			repoRoot:   t.TempDir(),
			app:        "app",
			baseDomain: "example.com",
			awsProfile: "profile",
			awsCfg:     aws.Config{Region: "us-east-1"},
			accountID:  "123456789012",
			hostedZone: hostedZone{ID: "Z1", Name: "example.com"},
			stages:     []naming.Stage{naming.StageDev},
			stateDir:   t.TempDir(),
		}
	}

	prepareLambdaAssetRootFn = func(string) (string, error) { return filepath.Join(t.TempDir(), "lambda-assets"), nil }
	stageLocalLambdaAssetsFn = func(string, string) ([]string, error) { return nil, nil }

	t.Run("tools error", func(t *testing.T) {
		env := baseEnv()
		ensureToolsAvailableFn = func() error { return errSentinel }
		buildLambdaZipsFn = func(string, bool) error { return nil }
		require.ErrorIs(t, env.run(context.Background()), errSentinel)
	})

	t.Run("build zips error", func(t *testing.T) {
		env := baseEnv()
		ensureToolsAvailableFn = func() error { return nil }
		buildLambdaZipsFn = func(string, bool) error { return errSentinel }
		require.ErrorIs(t, env.run(context.Background()), errSentinel)
	})

	t.Run("release asset install error", func(t *testing.T) {
		env := baseEnv()
		env.args.ReleaseDir = "/tmp/release"
		ensureReleaseDeployToolsAvailableFn = func() error { return nil }
		buildLambdaZipsFn = func(string, bool) error {
			t.Fatal("buildLambdaZips should not run when release assets are requested")
			return nil
		}
		installReleaseDeployAssetsFn = func(string, string) (releaseDeployAssetsInstallResult, error) {
			return releaseDeployAssetsInstallResult{}, errSentinel
		}
		require.ErrorIs(t, env.run(context.Background()), errSentinel)
	})

	t.Run("bootstrap output error", func(t *testing.T) {
		env := baseEnv()
		env.bootstrap = bootstrapWallet{Address: "0xabc", Mnemonic: "mnemonic", DerivationPath: defaultBootstrapDerivationPath, ChainID: 1}
		ensureToolsAvailableFn = func() error { return nil }
		buildLambdaZipsFn = func(string, bool) error { return nil }
		writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return errSentinel }
		require.ErrorIs(t, env.run(context.Background()), errSentinel)
	})

	t.Run("deploy error", func(t *testing.T) {
		env := baseEnv()
		ensureToolsAvailableFn = func() error { return nil }
		buildLambdaZipsFn = func(string, bool) error { return nil }
		writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return nil }
		cdkBootstrapFn = func(context.Context, string, string, string, string) error { return errSentinel }
		require.ErrorIs(t, env.run(context.Background()), errSentinel)
	})

	t.Run("deploy UI error", func(t *testing.T) {
		env := baseEnv()
		ensureToolsAvailableFn = func() error { return nil }
		buildLambdaZipsFn = func(string, bool) error { return nil }
		writeBootstrapKeyMaterialFn = func(string, bootstrapWallet) error { return nil }
		cdkBootstrapFn = func(context.Context, string, string, string, string) error { return nil }
		ensureAPIGatewayCloudWatchLogsRoleFn = func(context.Context, aws.Config) error { return nil }
		cdkDeployWithOutputsFn = func(_ context.Context, _ string, _ string, req cdkDeployRequest) (cdkDeployResult, error) {
			return cdkDeployResult{StackName: req.StackName, Outputs: map[string]string{}}, nil
		}
		buildAuthUIFn = func(string) (string, error) { return "", errSentinel }
		require.ErrorIs(t, env.run(context.Background()), errSentinel)
	})

	t.Run("print summary covers mnemonic branch", func(t *testing.T) {
		env := baseEnv()
		env.bootstrap = bootstrapWallet{Mnemonic: "mnemonic"}
		env.printSummary(filepath.Join(t.TempDir(), "state.json"))
	})
}

func TestUpEnv_PrintSummary_ManagedProvisioning(t *testing.T) {
	env := &upEnv{
		args:       upArgs{ProvisioningInputPath: "provision.json"},
		baseDomain: "example.com",
		stages:     []naming.Stage{naming.StageDev},
	}
	env.printSummary("/tmp/state.json")
}

func TestUpEnv_PrintSummary_BootstrapMnemonic(t *testing.T) {
	env := &upEnv{
		baseDomain: "example.com",
		stages:     []naming.Stage{naming.StageDev},
		bootstrap: bootstrapWallet{
			Address:        "0xabc",
			Mnemonic:       "mnemonic",
			DerivationPath: defaultBootstrapDerivationPath,
			ChainID:        1,
		},
	}
	env.printSummary("/tmp/state.json")
}

func TestRunUp_Errors(t *testing.T) {
	t.Run("parse args error", func(t *testing.T) {
		require.Error(t, runUp(nil))
	})

	t.Run("prepare env error", func(t *testing.T) {
		previousRepoRoot := findRepoRootFn
		t.Cleanup(func() { findRepoRootFn = previousRepoRoot })
		findRepoRootFn = func() (string, error) { return testUpRepoRoot(t), nil }

		err := runUp([]string{"--app", "app", "--base-domain", "example.com/", "--aws-profile", "profile"})
		require.Error(t, err)
	})
}

func testUpRepoRoot(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "infra", "cdk", "inventory"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "auth-ui"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "cdk.json"), []byte("{\n  \"app\": \"go run main.go\"\n}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "infra", "cdk", "inventory", "lambdas.go"), []byte("package inventory\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "auth-ui", "package.json"), []byte("{\n  \"name\": \"auth-ui\"\n}\n"), 0o644))
	return repoRoot
}
