package stacks

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

// VerifyLambdaAssetRootWithSynth proves the configured lambda asset root is the
// source synth stages for every inventory lambda, even when ../../bin is absent.
func VerifyLambdaAssetRootWithSynth(assetRoot string) error {
	assetRoot = strings.TrimSpace(assetRoot)
	if assetRoot == "" {
		return fmt.Errorf("lambda asset root is required")
	}

	tempRoot, err := os.MkdirTemp("", "lesser-lambda-asset-root.")
	if err != nil {
		return fmt.Errorf("create certification workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	workspace := filepath.Join(tempRoot, "infra", "cdk")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create certification workspace dir: %w", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	if err := os.Chdir(workspace); err != nil {
		return fmt.Errorf("chdir %s: %w", workspace, err)
	}
	defer func() { _ = os.Chdir(originalWD) }()

	outdir := filepath.Join(tempRoot, "cdk.out")
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	app.Node().SetContext(jsii.String("aws:cdk:enable-asset-metadata"), true)

	apiStack := NewLesserApiStack(app, "ArtifactDeployCertification", &LesserApiStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String("123456789012"),
				Region:  jsii.String("us-east-1"),
			},
		},
		Environment:      "development",
		Domain:           "dev.example.com",
		Config:           map[string]interface{}{"lambdaAssetRoot": assetRoot},
		HostedZoneDomain: "example.com",
		HostedZoneId:     "Z1",
		AppName:          "app",
		AccountID:        "123456789012",
		Region:           "us-east-1",
	})

	app.Synth(nil)

	for _, spec := range inventory.LambdaInventory.Lambdas {
		fn := apiStack.Functions.Must(spec.Name)
		stagedAssetPath, err := lambdaAssetMetadataPath(fn)
		if err != nil {
			return fmt.Errorf("resolve staged asset metadata for %s: %w", spec.Name, err)
		}

		stagedHash, err := sha256HexFileAtPath(filepath.Join(outdir, stagedAssetPath))
		if err != nil {
			return fmt.Errorf("hash staged asset for %s: %w", spec.Name, err)
		}
		expectedPath := filepath.Join(assetRoot, "bin", spec.Name+".zip")
		expectedHash, err := sha256HexFileAtPath(expectedPath)
		if err != nil {
			return fmt.Errorf("hash configured asset for %s: %w", spec.Name, err)
		}
		if stagedHash != expectedHash {
			return fmt.Errorf("staged asset for %s does not match %s", spec.Name, expectedPath)
		}
	}

	return nil
}

func lambdaAssetMetadataPath(fn awslambda.Function) (string, error) {
	child := fn.Node().DefaultChild()
	if child == nil {
		return "", fmt.Errorf("lambda default child not found")
	}

	cfn, ok := child.(awslambda.CfnFunction)
	if !ok {
		return "", fmt.Errorf("unexpected lambda default child type: %T", child)
	}

	value := cfn.GetMetadata(jsii.String("aws:asset:path"))
	switch typed := value.(type) {
	case string:
		return typed, nil
	case *string:
		if typed == nil {
			return "", fmt.Errorf("aws:asset:path metadata missing")
		}
		return *typed, nil
	case nil:
		return "", fmt.Errorf("aws:asset:path metadata missing")
	default:
		return "", fmt.Errorf("unexpected aws:asset:path type: %T", value)
	}
}

func sha256HexFileAtPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
