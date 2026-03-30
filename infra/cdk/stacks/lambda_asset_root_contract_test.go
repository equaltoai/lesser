package stacks

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
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/jsii-runtime-go"
)

func TestLesserApiStackLambdaFunctionsPropsUsesConfiguredLambdaAssetRoot(t *testing.T) {
	stack := awscdk.NewStack(awscdk.NewApp(nil), jsii.String("TestStack"), nil)

	table := awsdynamodb.NewTable(stack, jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	rateTable := awsdynamodb.NewTable(stack, jsii.String("RateLimitTable"), &awsdynamodb.TableProps{
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
	encryptionRole := awsiam.NewRole(stack, jsii.String("EncryptionRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	basicRole := awsiam.NewRole(stack, jsii.String("BasicRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	apiStack := &LesserApiStack{
		Stack:                  stack,
		AppName:                "app",
		Environment:            "development",
		Domain:                 "dev.example.com",
		Configuration:          map[string]interface{}{"lambdaAssetRoot": " /tmp/release-assets "},
		MainTable:              table,
		RateLimitTable:         rateTable,
		StreamEventsTable:      streamEventsTable,
		MediaBucket:            mediaBucket,
		StreamingBucket:        streamingBucket,
		TrainingBucket:         trainingBucket,
		PrivateKey:             privateKey,
		JwtSecret:              jwtSecret,
		ModelMetadataTableName: "model-metadata",
		LambdaEncryptionRole:   encryptionRole,
		LambdaBasicRole:        basicRole,
		MediaConvertRole:       awsiam.NewRole(stack, jsii.String("MediaConvertRole"), &awsiam.RoleProps{AssumedBy: awsiam.NewServicePrincipal(jsii.String("mediaconvert.amazonaws.com"), nil)}),
	}

	props := apiStack.lambdaFunctionsProps()
	if got, want := props.LambdaAssetRoot, "/tmp/release-assets"; got != want {
		t.Fatalf("LambdaAssetRoot = %q, want %q", got, want)
	}
}

func TestLesserApiStackSynthUsesConfiguredLambdaAssetRootWithoutRepoBin(t *testing.T) {
	assetRoot := writePlaceholderLambdaAssets(t, t.TempDir())
	if err := VerifyLambdaAssetRootWithSynth(assetRoot); err != nil {
		t.Fatalf("VerifyLambdaAssetRootWithSynth: %v", err)
	}
}

func writePlaceholderLambdaAssets(t *testing.T, root string) string {
	t.Helper()

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir %s: %v", binDir, err)
	}
	for _, spec := range inventory.LambdaInventory.Lambdas {
		assetPath := filepath.Join(binDir, spec.Name+".zip")
		if err := os.WriteFile(assetPath, []byte("placeholder:"+spec.Name), 0o644); err != nil {
			t.Fatalf("write placeholder asset %s: %v", assetPath, err)
		}
	}
	return root
}

func resolveLambdaAssetPathMetadata(t *testing.T, fn awslambda.Function) string {
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
