package constructs

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk/v3"
)

type representativeFunctionSynth struct {
	Template          map[string]any
	Outdir            string
	FunctionLogicalID string
	FunctionResource  map[string]any
	LogGroupResource  map[string]any
	RoleResource      map[string]any
}

// TestAppTheoryFunctionSynthParityForRepresentativeLambda proves that the
// AppTheoryFunction wrapper can express the same CloudFormation resource
// semantics as the current raw awslambda.Function construction for one
// representative lesser Lambda. This is intentionally a proof only; production
// Lambda construction remains unchanged until the adoption phase gets explicit
// architectural review of the synthesized diff.
func TestAppTheoryFunctionSynthParityForRepresentativeLambda(t *testing.T) {
	moduleRoot := ensureModuleRoot(t)
	ensureLambdaAssets(t, moduleRoot)

	spec := findInventoryLambdaSpec(t, "objects")
	if spec.Role != inventory.RoleClassEncryption {
		t.Fatalf("representative lambda should exercise the encryption role: got %s", spec.Role)
	}
	if len(spec.HTTPRoutes) == 0 {
		t.Fatalf("representative lambda should exercise an HTTP-facing inventory entry")
	}

	current := synthRepresentativeFunction(t, moduleRoot, spec, false)
	appTheory := synthRepresentativeFunction(t, moduleRoot, spec, true)

	assertRepresentativeFunctionResourceParity(t, current, appTheory)
	assertRepresentativeLogGroupParity(t, current, appTheory)
	assertRepresentativeRoleParity(t, current, appTheory)
	assertResourceTypeSetParity(t, current.Template, appTheory.Template, "AWS::Lambda::Permission")
	assertResourceTypeSetParity(t, current.Template, appTheory.Template, "AWS::IAM::Policy")
	assertResourceTypeSetParity(t, current.Template, appTheory.Template, "AWS::Lambda::Alias")
	assertResourceTypeSetParity(t, current.Template, appTheory.Template, "AWS::Lambda::Version")

	assertRepresentativeAssetStagedFromSameZip(t, moduleRoot, current)
	assertRepresentativeAssetStagedFromSameZip(t, moduleRoot, appTheory)
}

func synthRepresentativeFunction(t *testing.T, moduleRoot string, spec inventory.LambdaSpec, useAppTheory bool) representativeFunctionSynth {
	t.Helper()

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	app.Node().SetContext(jsii.String("aws:cdk:enable-asset-metadata"), true)
	stack := awscdk.NewStack(app, jsii.String("RepresentativeStack"), nil)

	environment := "development"
	appName := naming.DefaultAppName
	functionName := naming.ResourceNameWithApp(appName, spec.Name, environment)
	role := awsiam.NewRole(stack, jsii.String("RepresentativeRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	logGroup := awslogs.NewLogGroup(stack, jsii.String("RepresentativeLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/aws/lambda/%s", functionName)),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	lambdaAssetRoot := filepath.Clean(filepath.Join(moduleRoot, "..", ".."))
	assetPath := filepath.ToSlash(filepath.Join(lambdaAssetRoot, "bin", spec.Name+".zip"))
	env := representativeLambdaEnvironment(appName, environment)

	if useAppTheory {
		fn := apptheorycdk.NewAppTheoryFunction(stack, jsii.String("RepresentativeFunction"), &apptheorycdk.AppTheoryFunctionProps{
			Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
			Architecture: awslambda.Architecture_ARM_64(),
			MemorySize:   jsii.Number(inventory.LambdaInventory.Defaults.MemoryMB),
			Timeout:      awscdk.Duration_Seconds(jsii.Number(inventory.LambdaInventory.Defaults.TimeoutSeconds)),
			Environment:  &env,
			Tracing:      awslambda.Tracing_ACTIVE,
			FunctionName: jsii.String(functionName),
			Code:         awslambda.Code_FromAsset(jsii.String(assetPath), nil),
			Handler:      jsii.String("bootstrap"),
			LogGroup:     logGroup,
			Role:         role,
		})
		if fn.Fn() == nil {
			t.Fatal("AppTheoryFunction did not expose the synthesized Lambda function")
		}
	} else {
		awslambda.NewFunction(stack, jsii.String("RepresentativeFunction"), &awslambda.FunctionProps{
			Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
			Architecture: awslambda.Architecture_ARM_64(),
			MemorySize:   jsii.Number(inventory.LambdaInventory.Defaults.MemoryMB),
			Timeout:      awscdk.Duration_Seconds(jsii.Number(inventory.LambdaInventory.Defaults.TimeoutSeconds)),
			Environment:  &env,
			Tracing:      awslambda.Tracing_ACTIVE,
			FunctionName: jsii.String(functionName),
			Code:         awslambda.Code_FromAsset(jsii.String(assetPath), nil),
			Handler:      jsii.String("bootstrap"),
			LogGroup:     logGroup,
			Role:         role,
		})
	}

	app.Synth(nil)
	template := loadTemplate(t, filepath.Join(outdir, "RepresentativeStack.template.json"))
	fnLogical, fnResource := findResourceByTypeAndProperty(t, template, "AWS::Lambda::Function", "FunctionName", functionName)
	_, logGroupResource := findResourceByTypeAndProperty(t, template, "AWS::Logs::LogGroup", "LogGroupName", fmt.Sprintf("/aws/lambda/%s", functionName))
	_, roleResource := findResourceByType(t, template, "AWS::IAM::Role")

	return representativeFunctionSynth{
		Template:          template,
		Outdir:            outdir,
		FunctionLogicalID: fnLogical,
		FunctionResource:  fnResource,
		LogGroupResource:  logGroupResource,
		RoleResource:      roleResource,
	}
}

func representativeLambdaEnvironment(appName, environment string) map[string]*string {
	stage := naming.StageForEnvironment(environment)
	return map[string]*string{
		"ENVIRONMENT":                     jsii.String(environment),
		"STAGE":                           jsii.String(string(stage)),
		"APP_NAME":                        jsii.String(appName),
		"DOMAIN_NAME":                     jsii.String("dev.example.com"),
		"DYNAMODB_TABLE":                  jsii.String("lesser-development"),
		"RATE_LIMIT_TABLE_NAME":           jsii.String("lesser-development-rate-limits"),
		"LIMITED_TABLE_NAME":              jsii.String("lesser-development-rate-limits"),
		"CONNECTIONS_TABLE":               jsii.String("lesser-development"),
		"SUBSCRIPTIONS_TABLE":             jsii.String("lesser-development"),
		"S3_BUCKET_NAME":                  jsii.String("lesser-development-media"),
		"MODERATION_TRAINING_BUCKET_NAME": jsii.String("lesser-development-training"),
		"MODERATION_MODEL_METADATA_TABLE": jsii.String("model-metadata"),
		"PRIVATE_KEY_SECRET":              jsii.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:private-key"),
		"KMS_KEY_ID":                      jsii.String("alias/lesser-shared-encryption"),
		"JWT_SECRET_ARN":                  jsii.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:jwt"),
		"STREAM_EVENTS_TABLE_NAME":        jsii.String("lesser-development-stream-events"),
		"WEBSOCKET_ENDPOINT":              jsii.String("https://ws.dev.example.com/stream"),
		"GRAPHQL_WEBSOCKET_ENDPOINT":      jsii.String("https://ws.dev.example.com"),
	}
}

func assertRepresentativeFunctionResourceParity(t *testing.T, current, appTheory representativeFunctionSynth) {
	t.Helper()

	currentProps := resourceProperties(t, current.FunctionResource)
	appTheoryProps := resourceProperties(t, appTheory.FunctionResource)
	if !reflect.DeepEqual(currentProps, appTheoryProps) {
		t.Fatalf("Lambda function properties diverged:\ncurrent=%s\napptheory=%s", prettyJSON(currentProps), prettyJSON(appTheoryProps))
	}

	currentMetadata := resourceMetadata(current.FunctionResource)
	appTheoryMetadata := resourceMetadata(appTheory.FunctionResource)
	if !reflect.DeepEqual(currentMetadata, appTheoryMetadata) {
		t.Fatalf("Lambda function metadata diverged:\ncurrent=%s\napptheory=%s", prettyJSON(currentMetadata), prettyJSON(appTheoryMetadata))
	}

	if _, ok := currentProps["DeadLetterConfig"]; ok {
		t.Fatalf("current representative Lambda unexpectedly has a function-level DLQ: %s", prettyJSON(currentProps["DeadLetterConfig"]))
	}
	if _, ok := appTheoryProps["DeadLetterConfig"]; ok {
		t.Fatalf("AppTheory representative Lambda unexpectedly has a function-level DLQ: %s", prettyJSON(appTheoryProps["DeadLetterConfig"]))
	}
}

func assertRepresentativeLogGroupParity(t *testing.T, current, appTheory representativeFunctionSynth) {
	t.Helper()
	if !reflect.DeepEqual(current.LogGroupResource, appTheory.LogGroupResource) {
		t.Fatalf("LogGroup resources diverged:\ncurrent=%s\napptheory=%s", prettyJSON(current.LogGroupResource), prettyJSON(appTheory.LogGroupResource))
	}
}

func assertRepresentativeRoleParity(t *testing.T, current, appTheory representativeFunctionSynth) {
	t.Helper()
	if !reflect.DeepEqual(current.RoleResource, appTheory.RoleResource) {
		t.Fatalf("IAM role resources diverged:\ncurrent=%s\napptheory=%s", prettyJSON(current.RoleResource), prettyJSON(appTheory.RoleResource))
	}
}

func assertResourceTypeSetParity(t *testing.T, current, appTheory map[string]any, resourceType string) {
	t.Helper()
	currentSet := collectResourcesByType(current, resourceType)
	appTheorySet := collectResourcesByType(appTheory, resourceType)
	if !reflect.DeepEqual(currentSet, appTheorySet) {
		t.Fatalf("%s resources diverged:\ncurrent=%s\napptheory=%s", resourceType, prettyJSON(currentSet), prettyJSON(appTheorySet))
	}
}

func assertRepresentativeAssetStagedFromSameZip(t *testing.T, moduleRoot string, synth representativeFunctionSynth) {
	t.Helper()

	assetPath, ok := resourceMetadata(synth.FunctionResource)["aws:asset:path"].(string)
	if !ok || assetPath == "" {
		t.Fatalf("aws:asset:path metadata missing from %s", synth.FunctionLogicalID)
	}

	repoRoot := filepath.Clean(filepath.Join(moduleRoot, "..", ".."))
	expectedZip := filepath.Join(repoRoot, "bin", "objects.zip")
	stagedZip := filepath.Join(synth.Outdir, assetPath)
	if got, want := sha256HexFile(t, stagedZip), sha256HexFile(t, expectedZip); got != want {
		t.Fatalf("staged representative asset mismatch: got hash %s want %s (staged=%s expected=%s)", got, want, stagedZip, expectedZip)
	}
}

func findInventoryLambdaSpec(t *testing.T, name string) inventory.LambdaSpec {
	t.Helper()
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("inventory lambda %s not found", name)
	return inventory.LambdaSpec{}
}

func findResourceByTypeAndProperty(t *testing.T, tpl map[string]any, resourceType, propName, propValue string) (string, map[string]any) {
	t.Helper()
	resources := templateResources(t, tpl)
	for logical, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok || resource["Type"] != resourceType {
			continue
		}
		props, ok := resource["Properties"].(map[string]any)
		if !ok {
			continue
		}
		if got, ok := props[propName].(string); ok && got == propValue {
			return logical, resource
		}
	}
	t.Fatalf("resource %s with %s=%s not found", resourceType, propName, propValue)
	return "", nil
}

func findResourceByType(t *testing.T, tpl map[string]any, resourceType string) (string, map[string]any) {
	t.Helper()
	resources := collectResourcesByType(tpl, resourceType)
	if len(resources) != 1 {
		t.Fatalf("expected one %s resource, got %d: %s", resourceType, len(resources), prettyJSON(resources))
	}
	for logical, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s resource %s has unexpected shape: %T", resourceType, logical, raw)
		}
		return logical, resource
	}
	return "", nil
}

func collectResourcesByType(tpl map[string]any, resourceType string) map[string]any {
	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	matches := map[string]any{}
	for logical, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok || resource["Type"] != resourceType {
			continue
		}
		matches[logical] = resource
	}
	return matches
}

func templateResources(t *testing.T, tpl map[string]any) map[string]any {
	t.Helper()
	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template resources missing or wrong type")
	}
	return resources
}

func resourceProperties(t *testing.T, resource map[string]any) map[string]any {
	t.Helper()
	props, ok := resource["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("resource properties missing or wrong type: %s", prettyJSON(resource))
	}
	return props
}

func resourceMetadata(resource map[string]any) map[string]any {
	metadata, ok := resource["Metadata"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return metadata
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%#v", v)
	}
	return string(b)
}
