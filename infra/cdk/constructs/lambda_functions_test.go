package constructs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/jsii-runtime-go"
)

func TestLambdaFunctionsGeneratedFromInventory(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	moduleRoot := filepath.Clean(filepath.Join(originalWD, ".."))
	if err := os.Chdir(moduleRoot); err != nil {
		t.Fatalf("chdir to module root %s: %v", moduleRoot, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalWD)
	})

	// Create placeholder zip assets when they don't exist so this test can run
	// without requiring `make build-lambdas` first.
	repoRoot := filepath.Clean(filepath.Join(moduleRoot, "..", ".."))
	binDir := filepath.Join(repoRoot, "bin")
	createdAssets := make([]string, 0, len(inventory.LambdaInventory.Lambdas))
	for _, spec := range inventory.LambdaInventory.Lambdas {
		assetPath := filepath.Join(binDir, spec.Name+".zip")
		_, statErr := os.Stat(assetPath)
		if statErr == nil {
			continue
		}
		if !os.IsNotExist(statErr) {
			t.Fatalf("stat asset %s: %v", assetPath, statErr)
		}
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatalf("mkdir bin dir %s: %v", binDir, err)
		}
		if err := os.WriteFile(assetPath, []byte("placeholder:"+spec.Name), 0o644); err != nil {
			t.Fatalf("write placeholder asset %s: %v", assetPath, err)
		}
		createdAssets = append(createdAssets, assetPath)
	}
	t.Cleanup(func() {
		for _, assetPath := range createdAssets {
			_ = os.Remove(assetPath)
		}
	})

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{
		Outdir: jsii.String(outdir),
	})
	app.Node().SetContext(jsii.String("aws:cdk:enable-asset-metadata"), true)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	mainTable := awsdynamodb.NewTable(stack, jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	rateTable := awsdynamodb.NewTable(stack, jsii.String("RateTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	streamEventsTable := awsdynamodb.NewTable(stack, jsii.String("StreamEventsTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	mediaBucket := awss3.NewBucket(stack, jsii.String("MediaBucket"), nil)
	streamingBucket := awss3.NewBucket(stack, jsii.String("StreamingBucket"), nil)
	trainingBucket := awss3.NewBucket(stack, jsii.String("TrainingBucket"), nil)
	privateKey := awssecretsmanager.NewSecret(stack, jsii.String("PrivateKey"), nil)
	jwtSecret := awssecretsmanager.NewSecret(stack, jsii.String("JwtSecret"), nil)
	encRole := awsiam.NewRole(stack, jsii.String("EncRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	basicRole := awsiam.NewRole(stack, jsii.String("BasicRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	functions := CreateLambdaFunctions(stack, &LambdaFunctionsProps{
		Environment:         "dev",
		Table:               mainTable,
		RateLimitTable:      rateTable,
		StreamEventsTable:   streamEventsTable,
		MediaBucket:         mediaBucket,
		StreamingBucket:     streamingBucket,
		TrainingBucket:      trainingBucket,
		Queues:              map[string]QueuePair{},
		PrivateKey:          privateKey,
		JwtSecret:           jwtSecret,
		MediaConvertRoleArn: jsii.String("arn:aws:iam::123456789012:role/media-convert"),
		ModelMetadataTable:  jsii.String("model-metadata"),
		Config:              map[string]interface{}{},
		EncryptionRole:      encRole,
		BasicRole:           basicRole,
	})

	expectedCount := len(inventory.LambdaInventory.Lambdas)
	if got := len(functions.Functions); got != expectedCount {
		t.Fatalf("function count mismatch: got %d, want %d", got, expectedCount)
	}

	// Synthesis triggers asset binding and metadata attachment.
	app.Synth(nil)

	for _, spec := range inventory.LambdaInventory.Lambdas {
		fn := functions.Must(spec.Name)
		wantName := fmt.Sprintf("lesser-%s-%s", "dev", spec.Name)
		if got := resolveConfiguredFunctionName(t, fn); got != wantName {
			t.Fatalf("unexpected function name for %s: %s", spec.Name, got)
		}

		logGroupName := resolveLogGroupName(t, stack, spec.Name)
		wantLogGroup := fmt.Sprintf("/aws/lambda/%s", wantName)
		if logGroupName != wantLogGroup {
			t.Fatalf("unexpected log group for %s: %s", spec.Name, logGroupName)
		}

		assetPath := resolveAssetPathMetadata(t, fn)
		stagedHash := sha256HexFile(t, filepath.Join(outdir, assetPath))
		expectedHash := sha256HexFile(t, filepath.Join(binDir, spec.Name+".zip"))
		if stagedHash != expectedHash {
			t.Fatalf("unexpected staged asset for %s: %s (staged=%s)", spec.Name, assetPath, filepath.Join(outdir, assetPath))
		}
	}

	// Spot-check runtime and architecture remain consistent with Lift defaults
	apiFn := functions.Must("api")
	if apiFn.Runtime() != awslambda.Runtime_PROVIDED_AL2023() {
		t.Fatalf("unexpected runtime: %v", apiFn.Runtime())
	}
	if apiFn.Architecture() != awslambda.Architecture_ARM_64() {
		t.Fatalf("unexpected architecture: %v", apiFn.Architecture())
	}
}

func resolveConfiguredFunctionName(t *testing.T, fn awslambda.Function) string {
	t.Helper()
	child := fn.Node().DefaultChild()
	if child == nil {
		t.Fatal("lambda default child not found")
	}
	cfn, ok := child.(awslambda.CfnFunction)
	if !ok {
		t.Fatalf("unexpected lambda default child type: %T", child)
	}
	return mustString(t, cfn.FunctionName())
}

func resolveLogGroupName(t *testing.T, stack awscdk.Stack, name string) string {
	t.Helper()

	lg := stack.Node().FindChild(jsii.String(name + "LogGroup"))
	if lg == nil {
		t.Fatalf("log group construct not found for %s", name)
	}
	child := lg.Node().DefaultChild()
	if child == nil {
		t.Fatalf("log group default child not found for %s", name)
	}
	cfn, ok := child.(awslogs.CfnLogGroup)
	if !ok {
		t.Fatalf("unexpected log group default child type for %s: %T", name, child)
	}
	return mustString(t, cfn.LogGroupName())
}

func resolveAssetPathMetadata(t *testing.T, fn awslambda.Function) string {
	t.Helper()

	child := fn.Node().DefaultChild()
	if child == nil {
		t.Fatal("lambda default child not found")
	}
	cfn, ok := child.(awslambda.CfnFunction)
	if !ok {
		t.Fatalf("unexpected lambda default child type: %T", child)
	}

	value := cfn.GetMetadata(jsii.String("aws:asset:path"))
	if value == nil {
		t.Fatalf("aws:asset:path metadata missing for %s", mustString(t, cfn.FunctionName()))
	}
	switch typed := value.(type) {
	case string:
		return typed
	case *string:
		return mustString(t, typed)
	default:
		t.Fatalf("unexpected aws:asset:path type for %s: %T", mustString(t, cfn.FunctionName()), value)
	}
	return ""
}

func sha256HexFile(t *testing.T, path string) string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		t.Fatalf("hash %s: %v", path, err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func mustString(t *testing.T, v *string) string {
	t.Helper()
	if v == nil {
		t.Fatal("unexpected nil string pointer")
	}
	return *v
}
