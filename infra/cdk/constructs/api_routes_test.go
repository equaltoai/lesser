package constructs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	_jsii "github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func TestFederationHttpRoutesGeneratedFromInventory(t *testing.T) {
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
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	mainTable := awsdynamodb.NewTable(stack, _jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: _jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: _jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	rateTable := awsdynamodb.NewTable(stack, _jsii.String("RateTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: _jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: _jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	streamEventsTable := awsdynamodb.NewTable(stack, _jsii.String("StreamEventsTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: _jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: _jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})
	mediaBucket := awss3.NewBucket(stack, _jsii.String("MediaBucket"), nil)
	streamingBucket := awss3.NewBucket(stack, _jsii.String("StreamingBucket"), nil)
	trainingBucket := awss3.NewBucket(stack, _jsii.String("TrainingBucket"), nil)
	privateKey := awssecretsmanager.NewSecret(stack, _jsii.String("PrivateKey"), nil)
	jwtSecret := awssecretsmanager.NewSecret(stack, _jsii.String("JwtSecret"), nil)
	encRole := awsiam.NewRole(stack, _jsii.String("EncRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(_jsii.String("lambda.amazonaws.com"), nil),
	})
	basicRole := awsiam.NewRole(stack, _jsii.String("BasicRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(_jsii.String("lambda.amazonaws.com"), nil),
	})

	functions := CreateLambdaFunctions(stack, &LambdaFunctionsProps{
		Environment:         "development",
		Table:               mainTable,
		RateLimitTable:      rateTable,
		StreamEventsTable:   streamEventsTable,
		MediaBucket:         mediaBucket,
		StreamingBucket:     streamingBucket,
		TrainingBucket:      trainingBucket,
		Queues:              map[string]QueuePair{},
		PrivateKey:          privateKey,
		JwtSecret:           jwtSecret,
		MediaConvertRoleArn: _jsii.String("arn:aws:iam::123456789012:role/media-convert"),
		ModelMetadataTable:  _jsii.String("model-metadata"),
		Config:              map[string]interface{}{},
		EncryptionRole:      encRole,
		BasicRole:           basicRole,
	})

	_ = CreateAPIGateway(stack, &APIGatewayProps{
		Environment: "development",
		Functions:   functions,
	})

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestStack.template.json")
	b, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template %s: %v", templatePath, err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(b, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	gotRoutes := extractHttpRouteToFunctionName(t, tpl)
	wantRoutes := expectedFederationHttpRoutes(t, "development")

	for routeKey, wantFnName := range wantRoutes {
		gotFnName, ok := gotRoutes[routeKey]
		if !ok {
			t.Fatalf("missing route %q", routeKey)
		}
		if gotFnName != wantFnName {
			t.Fatalf("unexpected integration for %q: got %s want %s", routeKey, gotFnName, wantFnName)
		}
	}

	if _, ok := gotRoutes["GET /activities/{id}"]; ok {
		t.Fatalf("unexpected activity dereference route present")
	}
}

func expectedFederationHttpRoutes(t *testing.T, environment string) map[string]string {
	t.Helper()

	include := map[string]struct{}{
		"actor":       {},
		"collections": {},
		"inbox":       {},
		"objects":     {},
		"outbox":      {},
		"webfinger":   {},
	}

	want := make(map[string]string)
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if _, ok := include[spec.Name]; !ok {
			continue
		}
		for _, route := range spec.HTTPRoutes {
			routeKey := fmt.Sprintf("%s %s", route.Method, route.Path)
			want[routeKey] = naming.ResourceName(spec.Name, environment)
		}
	}
	return want
}

func extractHttpRouteToFunctionName(t *testing.T, tpl map[string]any) map[string]string {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template Resources missing or wrong type")
	}

	lambdaLogicalIDToName := make(map[string]string)
	for logicalID, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::Lambda::Function" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		if fnName, ok := props["FunctionName"].(string); ok && fnName != "" {
			lambdaLogicalIDToName[logicalID] = fnName
		}
	}

	restApiLogicalID := ""
	for logicalID, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] == "AWS::ApiGateway::RestApi" {
			restApiLogicalID = logicalID
			break
		}
	}

	resourceToPathPart := make(map[string]string)
	resourceToParent := make(map[string]string)
	for logicalID, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::ApiGateway::Resource" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		part, ok := props["PathPart"].(string)
		if !ok || part == "" {
			continue
		}
		resourceToPathPart[logicalID] = part

		parentID := props["ParentId"]
		if restApiLogicalID != "" && isRestApiRootResourceRef(parentID, restApiLogicalID) {
			resourceToParent[logicalID] = ""
			continue
		}
		if parentLogicalID, ok := findFirstRefLogicalID(parentID); ok {
			resourceToParent[logicalID] = parentLogicalID
		}
	}

	resourceLogicalIDToFullPath := make(map[string]string)
	var buildPath func(logicalID string) string
	buildPath = func(logicalID string) string {
		if logicalID == "" {
			return ""
		}
		if existing, ok := resourceLogicalIDToFullPath[logicalID]; ok {
			return existing
		}
		part := resourceToPathPart[logicalID]
		parent := resourceToParent[logicalID]
		full := ""
		if parent == "" {
			full = "/" + part
		} else {
			full = buildPath(parent) + "/" + part
		}
		resourceLogicalIDToFullPath[logicalID] = full
		return full
	}

	routeKeyToFunctionName := make(map[string]string)
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::ApiGateway::Method" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		httpMethod, ok := props["HttpMethod"].(string)
		if !ok || httpMethod == "" {
			continue
		}

		resourceLogicalID, ok := findFirstRefLogicalID(props["ResourceId"])
		if !ok {
			continue
		}
		fullPath := buildPath(resourceLogicalID)
		if fullPath == "" {
			continue
		}

		integration, ok := props["Integration"].(map[string]any)
		if !ok {
			continue
		}
		lambdaLogicalID, ok := findFirstGetAttLogicalID(integration["Uri"])
		if !ok {
			continue
		}
		fnName, ok := lambdaLogicalIDToName[lambdaLogicalID]
		if !ok {
			continue
		}

		routeKeyToFunctionName[fmt.Sprintf("%s %s", httpMethod, fullPath)] = fnName
	}

	return routeKeyToFunctionName
}

func isRestApiRootResourceRef(v any, restApiLogicalID string) bool {
	switch typed := v.(type) {
	case map[string]any:
		if getAtt, ok := typed["Fn::GetAtt"]; ok {
			switch att := getAtt.(type) {
			case []any:
				if len(att) == 2 {
					apiID, _ := att[0].(string)
					attr, _ := att[1].(string)
					return apiID == restApiLogicalID && attr == "RootResourceId"
				}
			case string:
				return att == restApiLogicalID+".RootResourceId"
			}
		}
		for _, child := range typed {
			if isRestApiRootResourceRef(child, restApiLogicalID) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if isRestApiRootResourceRef(child, restApiLogicalID) {
				return true
			}
		}
	}
	return false
}

func findFirstRefLogicalID(v any) (string, bool) {
	switch typed := v.(type) {
	case map[string]any:
		if ref, ok := typed["Ref"]; ok {
			if s, ok := ref.(string); ok && s != "" {
				return s, true
			}
		}
		for _, child := range typed {
			if s, ok := findFirstRefLogicalID(child); ok {
				return s, true
			}
		}
	case []any:
		for _, child := range typed {
			if s, ok := findFirstRefLogicalID(child); ok {
				return s, true
			}
		}
	}
	return "", false
}

func findFirstGetAttLogicalID(v any) (string, bool) {
	switch typed := v.(type) {
	case map[string]any:
		if getAtt, ok := typed["Fn::GetAtt"]; ok {
			switch att := getAtt.(type) {
			case []any:
				if len(att) == 2 {
					if logicalID, ok := att[0].(string); ok && logicalID != "" {
						return logicalID, true
					}
				}
			case string:
				parts := strings.Split(att, ".")
				if len(parts) > 0 && parts[0] != "" {
					return parts[0], true
				}
			}
		}
		for _, child := range typed {
			if s, ok := findFirstGetAttLogicalID(child); ok {
				return s, true
			}
		}
	case []any:
		for _, child := range typed {
			if s, ok := findFirstGetAttLogicalID(child); ok {
				return s, true
			}
		}
	}
	return "", false
}
