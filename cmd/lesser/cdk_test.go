package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

func TestCdkDeployWithOutputs_UsesRunCommand(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

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
		return os.WriteFile(outputsPath, []byte(`{"demo-stack":{"Foo":"Bar"}}`), 0o644)
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
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	repoRoot := t.TempDir()
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		outputsPath := ""
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "--outputs-file" {
				outputsPath = args[i+1]
				break
			}
		}
		if outputsPath != "" {
			return os.WriteFile(outputsPath, []byte(`{"demo":{"Key":"Value"}}`), 0o644)
		}
		return nil
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

func TestCdkDeployWithOutputs_WrapsRunCommandError(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		return os.ErrNotExist
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
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	repoRoot := t.TempDir()
	outputsPath := filepath.Join(t.TempDir(), "deploy", "cdk-outputs", "demo.json")
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		return os.WriteFile(outputsPath, []byte(`{"demo":{"Key":"Value"}}`), 0o644)
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
}

func TestCdkDeployWithOutputs_PrefersExplicitContexts(t *testing.T) {
	previousRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	t.Setenv("LESSER_HOST_URL", "https://env.lesser.host")
	t.Setenv("TRANSLATION_ENABLED", "true")

	repoRoot := t.TempDir()
	var gotArgs []string
	runCommandFn = func(_ context.Context, _ string, args []string, _ execOptions) error {
		gotArgs = append([]string(nil), args...)
		outputsPath := ""
		for i := 0; i < len(args)-1; i++ {
			if args[i] == "--outputs-file" {
				outputsPath = args[i+1]
				break
			}
		}
		if outputsPath != "" {
			return os.WriteFile(outputsPath, []byte(`{"demo":{"Key":"Value"}}`), 0o644)
		}
		return nil
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
	t.Cleanup(func() { runCommandFn = previousRunCommand })

	runCommandFn = func(context.Context, string, []string, execOptions) error {
		t.Fatal("runCommandFn should not be called when contexts are invalid")
		return nil
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
