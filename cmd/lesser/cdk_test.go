package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/require"
)

func TestParseCdkOutputs(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "outputs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"stack":{"Key":"Value"}}`), 0o644))

	out, err := parseCdkOutputs(path)
	require.NoError(t, err)
	require.Equal(t, map[string]map[string]string{"stack": {"Key": "Value"}}, out)

	t.Run("missing file errors", func(t *testing.T) {
		_, err := parseCdkOutputs(filepath.Join(tmp, "missing.json"))
		require.Error(t, err)
	})

	t.Run("invalid json errors", func(t *testing.T) {
		bad := filepath.Join(tmp, "bad.json")
		require.NoError(t, os.WriteFile(bad, []byte("{"), 0o644))
		_, err := parseCdkOutputs(bad)
		require.Error(t, err)
	})
}

func parseCdkOutputs(path string) (map[string]map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- test helper reads fixture paths
	if err != nil {
		return nil, err
	}

	out := map[string]map[string]string{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func TestCdkDeployWithOutputs_UsesRunCommand(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	repoRoot := t.TempDir()
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		outputsPath := ""
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "--outputs-file" {
				outputsPath = args[i+1]
				break
			}
		}
		if outputsPath == "" {
			return nil
		}
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"Foo": "Bar"}, nil
	}

	res, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:    "demo-stack",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
	})
	require.NoError(t, err)
	require.Equal(t, "demo-stack", res.StackName)
	require.Equal(t, map[string]string{"Foo": "Bar"}, res.Outputs)
}

func TestCdkDeployWithOutputs_IncludesStageAndStagingFlags(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	repoRoot := t.TempDir()
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"Key": "Value"}, nil
	}

	_, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:       "demo",
		App:             "app",
		BaseDomain:      "example.com",
		HostedZoneID:    "Z1",
		Region:          "us-east-1",
		LambdaAssetRoot: "/tmp/lambda-assets",
		StageFilter:     "DEV",
		WithStaging:     true,
	})
	require.NoError(t, err)
	require.Contains(t, gotArgs, "--context")
	require.Contains(t, gotArgs, "stage=dev")
	require.Contains(t, gotArgs, "withStaging=true")
	require.Contains(t, gotArgs, "lambdaAssetRoot=/tmp/lambda-assets")
}

func TestCdkDeployWithOutputs_NormalizesAPICORSContext(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	repoRoot := t.TempDir()
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"Key": "Value"}, nil
	}

	_, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
		Contexts: map[string]string{
			"apiCorsAllowedOrigins": " https://APP.example.com/ , https://bad.example/path ",
		},
	})
	require.NoError(t, err)
	require.Contains(t, gotArgs, "apiCorsAllowedOrigins=https://app.example.com")
	require.NotContains(t, gotArgs, "https://bad.example/path")
}

func TestCdkDeployWithOutputs_UsesAPICORSEnvFallback(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})
	t.Setenv("API_CORS_ALLOWED_ORIGINS", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://LEGACY.example.com/ ")

	repoRoot := t.TempDir()
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"Key": "Value"}, nil
	}

	_, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
	})
	require.NoError(t, err)
	require.Contains(t, gotArgs, "apiCorsAllowedOrigins=https://legacy.example.com")
}

func TestCdkDeployWithOutputs_WrapsRunCommandError(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		return os.ErrNotExist
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		t.Fatal("resolveStackOutputsFn should not be called when deploy fails")
		return nil, nil
	}

	_, err := cdkDeployWithOutputs(context.Background(), t.TempDir(), "profile", cdkDeployRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cdk deploy demo")
}

func TestCdkDeployWithOutputs_UsesExplicitOutputsPath(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	repoRoot := t.TempDir()
	outputsPath := filepath.Join(t.TempDir(), "deploy", "cdk-outputs", "demo.json")
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"Key": "Value"}, nil
	}

	res, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
		OutputsPath:  outputsPath,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"Key": "Value"}, res.Outputs)
	require.Contains(t, gotArgs, outputsPath)
	out, err := parseCdkOutputs(outputsPath)
	require.NoError(t, err)
	require.Equal(t, map[string]map[string]string{"demo": {"Key": "Value"}}, out)
}

func TestCdkDeployWithOutputs_PrefersExplicitContexts(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	t.Setenv("LESSER_HOST_URL", "https://env.lesser.host")
	t.Setenv("TRANSLATION_ENABLED", "true")

	repoRoot := t.TempDir()
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"Key": "Value"}, nil
	}

	_, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
		Contexts: map[string]string{
			"lesserHostUrl":      "https://override.lesser.host/",
			"translationEnabled": "false",
		},
	})
	require.NoError(t, err)
	require.Contains(t, gotArgs, "lesserHostUrl=https://override.lesser.host")
	require.NotContains(t, gotArgs, "lesserHostUrl=https://env.lesser.host")
	require.Contains(t, gotArgs, "translationEnabled=false")
	require.NotContains(t, gotArgs, "translationEnabled=true")
}

func TestCdkDeployWithOutputs_RejectsLambdaFunctionURLHost(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		t.Fatal("runCommandFn should not be called when contexts are invalid")
		return nil
	}
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		t.Fatal("resolveStackOutputsFn should not be called when contexts are invalid")
		return nil, nil
	}

	_, err := cdkDeployWithOutputs(context.Background(), t.TempDir(), "profile", cdkDeployRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
		Contexts: map[string]string{
			"lesserHostUrl": "https://abc.lambda-url.us-east-1.on.aws",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "LESSER_HOST_URL")
}

func TestCdkDeployWithOutputs_PersistsResolvedOutputsWhenCDKOmitsOutputsFile(t *testing.T) {
	previousRunCommand := runCommandFn
	previousResolveOutputs := resolveStackOutputsFn
	t.Cleanup(func() {
		runCommandFn = previousRunCommand
		resolveStackOutputsFn = previousResolveOutputs
	})

	repoRoot := t.TempDir()
	runCommandFn = func(_ context.Context, _ string, _ []string, _ execOptions) error {
		return nil
	}

	outputsPath := filepath.Join(repoRoot, "deploy", "cdk-outputs", "demo-stack.json")
	resolveStackOutputsFn = func(context.Context, string, cdkDeployRequest) (map[string]string, error) {
		return map[string]string{"ReleaseAssetBucketName": "bucket"}, nil
	}

	res, err := cdkDeployWithOutputs(context.Background(), repoRoot, "profile", cdkDeployRequest{
		StackName:    "demo-stack",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
		OutputsPath:  outputsPath,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"ReleaseAssetBucketName": "bucket"}, res.Outputs)

	out, err := parseCdkOutputs(outputsPath)
	require.NoError(t, err)
	require.Equal(t, map[string]map[string]string{
		"demo-stack": {
			"ReleaseAssetBucketName": "bucket",
		},
	}, out)
}

func TestResolveStackOutputs_UsesConfiguredProfileAndRegion(t *testing.T) {
	previousLoadAWS := loadAWSConfigForCLIFn
	previousNewCloudFormationClient := newCloudFormationClientFn
	previousDescribeOutputs := describeCloudFormationOutputsFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWS
		newCloudFormationClientFn = previousNewCloudFormationClient
		describeCloudFormationOutputsFn = previousDescribeOutputs
	})

	loadAWSConfigForCLIFn = func(context.Context, string) (aws.Config, string, error) {
		return aws.Config{Region: "us-west-2"}, "profile", nil
	}

	var gotCfg aws.Config
	newCloudFormationClientFn = func(cfg aws.Config, _ ...func(*cloudformation.Options)) *cloudformation.Client {
		gotCfg = cfg
		return nil
	}

	describeCloudFormationOutputsFn = func(_ context.Context, _ *cloudformation.Client, stackName string) (map[string]string, error) {
		require.Equal(t, "demo-stack", stackName)
		return map[string]string{"Key": "Value"}, nil
	}

	out, err := resolveStackOutputs(context.Background(), "profile", cdkDeployRequest{
		StackName: "demo-stack",
		Region:    "us-east-1",
	})
	require.NoError(t, err)
	require.Equal(t, "us-east-1", gotCfg.Region)
	require.Equal(t, map[string]string{"Key": "Value"}, out)
}

func TestWriteCdkOutputs_ValidatesRequiredInputs(t *testing.T) {
	err := writeCdkOutputs("   ", "demo", map[string]string{"Key": "Value"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cdk outputs path is required")

	err = writeCdkOutputs(filepath.Join(t.TempDir(), "outputs.json"), "   ", map[string]string{"Key": "Value"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cdk stack name is required")
}

func TestCloneStringMap_NilInputReturnsEmptyMap(t *testing.T) {
	cloned := cloneStringMap(nil)
	require.NotNil(t, cloned)
	require.Empty(t, cloned)
}

func TestCdkBootstrap_RunsCdkBootstrap(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	var gotArgs []string
	var gotEnv map[string]string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		require.Equal(t, "cdk", name)
		gotArgs = append([]string(nil), args...)
		gotEnv = opts.Env
		return nil
	}

	require.NoError(t, cdkBootstrap(context.Background(), t.TempDir(), "profile", "123", "us-east-1"))
	require.Contains(t, gotArgs, "bootstrap")
	require.Contains(t, gotArgs, "aws://123/us-east-1")
	require.Equal(t, "profile", gotEnv["AWS_PROFILE"])
}

func TestCdkDestroyStack_IncludesContexts(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	var gotArgs []string
	var gotEnv map[string]string
	runCommandFn = func(_ context.Context, name string, args []string, opts execOptions) error {
		require.Equal(t, "cdk", name)
		gotArgs = append([]string(nil), args...)
		gotEnv = opts.Env
		return nil
	}

	err := cdkDestroyStack(context.Background(), t.TempDir(), "profile", cdkDestroyRequest{
		StackName:    "demo",
		App:          "app",
		BaseDomain:   "example.com",
		HostedZoneID: "Z1",
		Region:       "us-east-1",
		StageFilter:  "LIVE",
		WithStaging:  true,
	})
	require.NoError(t, err)
	require.Contains(t, gotArgs, "destroy")
	require.Contains(t, gotArgs, "--force")
	require.Contains(t, gotArgs, "hostedZoneId=Z1")
	require.Contains(t, gotArgs, "stage=live")
	require.Contains(t, gotArgs, "withStaging=true")
	require.Equal(t, "profile", gotEnv["AWS_PROFILE"])
}

func TestCdkDestroyStack_WrapsError(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		return os.ErrPermission
	}

	err := cdkDestroyStack(context.Background(), t.TempDir(), "profile", cdkDestroyRequest{
		StackName:  "demo",
		App:        "app",
		BaseDomain: "example.com",
		Region:     "us-east-1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cdk destroy demo")
}
