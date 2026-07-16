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
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
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

	apiFnName := naming.ResourceName("api", "development")
	if got, ok := gotRoutes["GET /"]; !ok || got != apiFnName {
		t.Fatalf("expected root GET route to integrate with %s (got %q)", apiFnName, got)
	}
	if got, ok := gotRoutes["HEAD /"]; !ok || got != apiFnName {
		t.Fatalf("expected root HEAD route to integrate with %s (got %q)", apiFnName, got)
	}
}

func TestBodyEnabledAddsMcpRoute(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	dummy := awslambda.NewFunction(stack, _jsii.String("DummyFn"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: _jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(_jsii.String("exports.handler = async () => ({ statusCode: 200, body: 'ok' });")),
	})

	functions := &LambdaFunctions{
		Functions: map[string]awslambda.Function{
			"api":         dummy,
			"graphql":     dummy,
			"sse":         dummy,
			"streaming":   dummy,
			"graphql-ws":  dummy,
			"actor":       dummy,
			"collections": dummy,
			"inbox":       dummy,
			"objects":     dummy,
			"outbox":      dummy,
			"webfinger":   dummy,
		},
	}

	_ = CreateAPIGateway(stack, &APIGatewayProps{
		Environment: "development",
		Functions:   functions,
		BodyEnabled: true,
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

	gotRoutes := extractHttpRouteToIntegrationURI(t, tpl)

	wantParamName := "/lesser/dev/lesser-body/exports/v1/mcp_lambda_arn"
	for _, routeKey := range []string{
		"POST /mcp",
		"GET /mcp",
		"DELETE /mcp",
		"POST /mcp/{actor}",
		"GET /mcp/{actor}",
		"DELETE /mcp/{actor}",
	} {
		uri, ok := gotRoutes[routeKey]
		if !ok {
			t.Fatalf("expected %s route to exist when bodyEnabled=true", routeKey)
		}
		if !integrationURIReferencesSSMParameterDefault(t, tpl, uri, wantParamName) {
			uriJSON, err := json.Marshal(uri)
			if err != nil {
				t.Fatalf("marshal integration uri: %v", err)
			}
			t.Fatalf("expected %s integration to reference SSM param %q (got %s)", routeKey, wantParamName, string(uriJSON))
		}
	}

	for _, routeKey := range []string{
		"GET /.well-known/mcp.json",
		"GET /.well-known/oauth-protected-resource/mcp/{actor}",
	} {
		uri, ok := gotRoutes[routeKey]
		if !ok {
			t.Fatalf("expected %s route to exist when bodyEnabled=true", routeKey)
		}
		if !integrationURIReferencesSSMParameterDefault(t, tpl, uri, wantParamName) {
			uriJSON, err := json.Marshal(uri)
			if err != nil {
				t.Fatalf("marshal integration uri: %v", err)
			}
			t.Fatalf("expected %s integration to reference SSM param %q (got %s)", routeKey, wantParamName, string(uriJSON))
		}
	}

	optionsProps, ok := findMethodPropertiesByRouteKey(t, tpl, "OPTIONS /mcp")
	if !ok {
		t.Fatalf("expected OPTIONS /mcp route to exist when bodyEnabled=true")
	}

	integration, ok := optionsProps["Integration"].(map[string]any)
	if !ok {
		t.Fatalf("expected OPTIONS /mcp integration properties to exist")
	}

	gotAllowHeaders := firstIntegrationResponseParameterValue(integration, "method.response.header.Access-Control-Allow-Headers")
	wantAllowHeaders := "'Accept,Authorization,Content-Type,User-Agent,X-Forwarded-For,X-Forwarded-Proto,X-Request-Id,X-Tenant-Id,X-Amz-Date,X-Api-Key,X-Amz-Security-Token,mcp-protocol-version,mcp-session-id,last-event-id'"
	if gotAllowHeaders != wantAllowHeaders {
		t.Fatalf("unexpected OPTIONS /mcp allow-headers value: got %q want %q", gotAllowHeaders, wantAllowHeaders)
	}
	gotAllowOrigin := firstIntegrationResponseParameterValue(integration, "method.response.header.Access-Control-Allow-Origin")
	if gotAllowOrigin != "integration.response.body.allowedOrigin" {
		t.Fatalf("expected OPTIONS /mcp allow-origin to be derived from the allowlist template (got %q)", gotAllowOrigin)
	}

	if !templateHasMcpInvokePermission(t, tpl, wantParamName) {
		t.Fatalf("expected API Gateway invoke permission for the MCP Lambda (param %q)", wantParamName)
	}
}

func TestBodyDisabledDoesNotAddMcpRoute(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	dummy := awslambda.NewFunction(stack, _jsii.String("DummyFn"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: _jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(_jsii.String("exports.handler = async () => ({ statusCode: 200, body: 'ok' });")),
	})

	functions := &LambdaFunctions{
		Functions: map[string]awslambda.Function{
			"api":         dummy,
			"graphql":     dummy,
			"sse":         dummy,
			"streaming":   dummy,
			"graphql-ws":  dummy,
			"actor":       dummy,
			"collections": dummy,
			"inbox":       dummy,
			"objects":     dummy,
			"outbox":      dummy,
			"webfinger":   dummy,
		},
	}

	_ = CreateAPIGateway(stack, &APIGatewayProps{
		Environment: "development",
		Functions:   functions,
		BodyEnabled: false,
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

	gotRoutes := extractHttpRouteToIntegrationURI(t, tpl)
	if _, ok := gotRoutes["POST /mcp"]; ok {
		t.Fatalf("unexpected POST /mcp route present when bodyEnabled=false")
	}
	if _, ok := gotRoutes["POST /mcp/{actor}"]; ok {
		t.Fatalf("unexpected POST /mcp/{actor} route present when bodyEnabled=false")
	}
	if _, ok := gotRoutes["GET /mcp"]; ok {
		t.Fatalf("unexpected GET /mcp route present when bodyEnabled=false")
	}
	if _, ok := gotRoutes["GET /mcp/{actor}"]; ok {
		t.Fatalf("unexpected GET /mcp/{actor} route present when bodyEnabled=false")
	}
	if _, ok := gotRoutes["DELETE /mcp"]; ok {
		t.Fatalf("unexpected DELETE /mcp route present when bodyEnabled=false")
	}
	if _, ok := gotRoutes["DELETE /mcp/{actor}"]; ok {
		t.Fatalf("unexpected DELETE /mcp/{actor} route present when bodyEnabled=false")
	}
	for _, routeKey := range []string{
		"GET /.well-known/mcp.json",
		"GET /.well-known/oauth-protected-resource/mcp/{actor}",
	} {
		if _, ok := gotRoutes[routeKey]; ok {
			t.Fatalf("unexpected %s route present when bodyEnabled=false", routeKey)
		}
	}
	if _, ok := gotRoutes["OPTIONS /mcp"]; ok {
		t.Fatalf("unexpected OPTIONS /mcp route present when bodyEnabled=false")
	}
}

func TestInstancePlaneDisabledDoesNotAddRoutes(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment:          "development",
		BodyEnabled:          true,
		InstancePlaneEnabled: false,
	})

	requireNoInstancePlaneRoutes(t, tpl)
	requireNoSSMParameterDefault(t, tpl, "/lesser/dev/lesser-body/exports/v1/instance_mcp_lambda_arn")
}

func TestInstancePlaneRequiresBodyEnabled(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment:          "development",
		BodyEnabled:          false,
		InstancePlaneEnabled: true,
	})

	requireNoInstancePlaneRoutes(t, tpl)
	requireNoSSMParameterDefault(t, tpl, "/lesser/dev/lesser-body/exports/v1/instance_mcp_lambda_arn")
}

func TestInstancePlaneEnabledAddsRoutes(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment:          "development",
		BodyEnabled:          true,
		InstancePlaneEnabled: true,
	})

	gotRoutes := extractHttpRouteToIntegrationURI(t, tpl)
	wantParamName := "/lesser/dev/lesser-body/exports/v1/instance_mcp_lambda_arn"
	for _, routeKey := range instancePlaneRouteKeys() {
		uri, ok := gotRoutes[routeKey]
		if !ok {
			t.Fatalf("expected %s route to exist when instancePlaneEnabled=true and bodyEnabled=true", routeKey)
		}
		if !integrationURIReferencesSSMParameterDefault(t, tpl, uri, wantParamName) {
			uriJSON, err := json.Marshal(uri)
			if err != nil {
				t.Fatalf("marshal integration uri: %v", err)
			}
			t.Fatalf("expected %s integration to reference SSM param %q (got %s)", routeKey, wantParamName, string(uriJSON))
		}
	}

	for _, routeKey := range []string{
		"POST /instance/ptah/mcp",
		"GET /instance/ptah/mcp",
		"POST /instance/ba/mcp",
		"GET /instance/ba/mcp",
	} {
		props, ok := findMethodPropertiesByRouteKey(t, tpl, routeKey)
		if !ok {
			t.Fatalf("expected %s route to exist", routeKey)
		}
		integration := requireIntegrationProps(t, props, routeKey)
		if !valueContainsString(integration["Uri"], "response-streaming-invocations") {
			t.Fatalf("expected %s to use the AppTheory streaming Lambda proxy URI", routeKey)
		}
	}

	for _, routeKey := range []string{
		"DELETE /instance/ptah/mcp",
		"DELETE /instance/ba/mcp",
		"GET /.well-known/oauth-protected-resource/instance/ptah/mcp",
		"GET /.well-known/oauth-protected-resource/instance/ba/mcp",
		"GET /instance/downloads/installer-grants/{grantId}",
	} {
		props, ok := findMethodPropertiesByRouteKey(t, tpl, routeKey)
		if !ok {
			t.Fatalf("expected %s route to exist", routeKey)
		}
		integration := requireIntegrationProps(t, props, routeKey)
		if valueContainsString(integration["Uri"], "response-streaming-invocations") {
			t.Fatalf("expected %s to use the non-streaming Lambda proxy URI", routeKey)
		}
	}

	if !templateHasMcpInvokePermission(t, tpl, wantParamName) {
		t.Fatalf("expected API Gateway invoke permission for the instance MCP Lambda (param %q)", wantParamName)
	}
}

func TestAPIGatewaySuppressesConsoleTestInvokePermissions(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment:          "development",
		BodyEnabled:          true,
		InstancePlaneEnabled: true,
	})

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template Resources missing or wrong type")
	}
	for logicalID, raw := range resources {
		resource, ok := raw.(map[string]any)
		if !ok || resource["Type"] != "AWS::Lambda::Permission" {
			continue
		}
		props, ok := resource["Properties"].(map[string]any)
		if !ok {
			continue
		}
		if valueContainsString(props["SourceArn"], "test-invoke-stage") {
			t.Fatalf("unexpected API Gateway console test invoke permission %s", logicalID)
		}
	}
}

func TestInstallerGrantDownloadRoutePreservesQueryCapabilityURLs(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment:          "development",
		BodyEnabled:          true,
		InstancePlaneEnabled: true,
	})

	const routeKey = "GET /instance/downloads/installer-grants/{grantId}"
	props, ok := findMethodPropertiesByRouteKey(t, tpl, routeKey)
	if !ok {
		t.Fatalf("expected %s route to exist", routeKey)
	}

	integration := requireIntegrationProps(t, props, routeKey)
	if got, _ := integration["Type"].(string); got != "AWS_PROXY" {
		t.Fatalf("expected %s to use Lambda proxy integration, got %q", routeKey, got)
	}
	if _, ok := integration["CacheKeyParameters"]; ok {
		t.Fatalf("%s integration must not configure cache key parameters that could omit the one-time token query string", routeKey)
	}
	if _, ok := integration["RequestParameters"]; ok {
		t.Fatalf("%s integration must not configure request-parameter mappings that could strip query strings", routeKey)
	}
	if _, ok := integration["RequestTemplates"]; ok {
		t.Fatalf("%s integration must not configure request templates; Lambda proxy preserves query strings", routeKey)
	}
	if requestParams, ok := props["RequestParameters"].(map[string]any); ok {
		for key := range requestParams {
			if strings.Contains(strings.ToLower(key), "querystring") {
				t.Fatalf("%s method must not configure query-string request parameter mapping %q", routeKey, key)
			}
		}
	}
	requireNoApiGatewayCaching(t, tpl, routeKey)
}

func TestRestAPIPreflightDefaultsToInstanceOrigin(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment: "development",
		Domain:      "dev.example.com",
	})

	integration := preflightIntegrationForRoute(t, tpl, "OPTIONS /api/v1/{proxy+}")
	gotAllowOrigin := firstIntegrationResponseParameterValue(integration, "method.response.header.Access-Control-Allow-Origin")
	if gotAllowOrigin != "integration.response.body.allowedOrigin" {
		t.Fatalf("expected dynamic allow-origin mapping, got %q", gotAllowOrigin)
	}
	if got := firstIntegrationResponseParameterValue(integration, "method.response.header.Access-Control-Allow-Credentials"); got != "'false'" {
		t.Fatalf("expected credentials to remain disabled, got %q", got)
	}

	template := firstRequestTemplateValue(integration, "application/json")
	if !strings.Contains(template, `#set($defaultOrigin = "https://dev.example.com")`) {
		t.Fatalf("expected default instance origin in preflight template, got:\n%s", template)
	}
	if !strings.Contains(template, `#set($configuredOrigins = "")`) {
		t.Fatalf("expected empty configured origins to use default instance origin, got:\n%s", template)
	}
}

func TestRestAPIPreflightUsesConfiguredAllowlist(t *testing.T) {
	tpl := synthesizeAPIGatewayTemplate(t, &APIGatewayProps{
		Environment:           "development",
		Domain:                "dev.example.com",
		APICORSAllowedOrigins: " https://APP.example.com/ , https://bad.example/path ",
	})

	integration := preflightIntegrationForRoute(t, tpl, "OPTIONS /api/v1/{proxy+}")
	template := firstRequestTemplateValue(integration, "application/json")
	if !strings.Contains(template, `#set($defaultOrigin = "https://dev.example.com")`) {
		t.Fatalf("expected default instance origin to remain available for empty deploy config, got:\n%s", template)
	}
	if !strings.Contains(template, `#set($configuredOrigins = "https://app.example.com")`) {
		t.Fatalf("expected configured origins to be normalized before synthesis, got:\n%s", template)
	}
	if strings.Contains(template, `bad.example/path`) {
		t.Fatalf("invalid configured origin leaked into preflight template:\n%s", template)
	}
	if got := firstIntegrationResponseParameterValue(integration, "method.response.header.Access-Control-Allow-Origin"); got == "'*'" {
		t.Fatalf("configured preflight must not synthesize wildcard allow-origin")
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

func instancePlaneRouteKeys() []string {
	return []string{
		"POST /instance/ptah/mcp",
		"GET /instance/ptah/mcp",
		"DELETE /instance/ptah/mcp",
		"POST /instance/ba/mcp",
		"GET /instance/ba/mcp",
		"DELETE /instance/ba/mcp",
		"GET /.well-known/oauth-protected-resource/instance/ptah/mcp",
		"GET /.well-known/oauth-protected-resource/instance/ba/mcp",
		"GET /instance/downloads/installer-grants/{grantId}",
	}
}

func requireNoInstancePlaneRoutes(t *testing.T, tpl map[string]any) {
	t.Helper()

	gotRoutes := extractHttpRouteToIntegrationURI(t, tpl)
	for _, routeKey := range append(instancePlaneRouteKeys(),
		"OPTIONS /instance/ptah/mcp",
		"OPTIONS /instance/ba/mcp",
		"OPTIONS /.well-known/oauth-protected-resource/instance/ptah/mcp",
		"OPTIONS /.well-known/oauth-protected-resource/instance/ba/mcp",
		"OPTIONS /instance/downloads/installer-grants/{grantId}",
	) {
		if _, ok := gotRoutes[routeKey]; ok {
			t.Fatalf("unexpected %s route present while instance plane is disabled", routeKey)
		}
	}
}

func requireNoSSMParameterDefault(t *testing.T, tpl map[string]any, wantDefault string) {
	t.Helper()

	rawParameters, ok := tpl["Parameters"].(map[string]any)
	if !ok {
		return
	}

	for _, raw := range rawParameters {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		paramType, _ := param["Type"].(string)
		if !strings.HasPrefix(paramType, "AWS::SSM::Parameter::Value") {
			continue
		}
		if gotDefault, _ := param["Default"].(string); gotDefault == wantDefault {
			t.Fatalf("unexpected SSM parameter default %q in template", wantDefault)
		}
	}
}

func extractHttpRouteToIntegrationURI(t *testing.T, tpl map[string]any) map[string]any {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template Resources missing or wrong type")
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

	routeKeyToURI := make(map[string]any)
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

		fullPath := ""
		if resourceLogicalID, ok := findFirstRefLogicalID(props["ResourceId"]); ok {
			fullPath = buildPath(resourceLogicalID)
		} else if restApiLogicalID != "" && isRestApiRootResourceRef(props["ResourceId"], restApiLogicalID) {
			fullPath = "/"
		}
		if fullPath == "" {
			continue
		}

		integration, ok := props["Integration"].(map[string]any)
		if !ok {
			continue
		}

		routeKeyToURI[fmt.Sprintf("%s %s", httpMethod, fullPath)] = integration["Uri"]
	}

	return routeKeyToURI
}

func integrationURIReferencesSSMParameterDefault(t *testing.T, tpl map[string]any, uri any, wantDefault string) bool {
	t.Helper()

	rawParameters, ok := tpl["Parameters"].(map[string]any)
	if !ok {
		return false
	}

	type paramInfo struct {
		Type    string
		Default string
	}
	params := make(map[string]paramInfo, len(rawParameters))
	for logicalID, raw := range rawParameters {
		typed, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		paramType, _ := typed["Type"].(string)
		paramDefault, _ := typed["Default"].(string)
		params[logicalID] = paramInfo{Type: paramType, Default: paramDefault}
	}

	refs := findAllRefLogicalIDs(uri)
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}

		p, ok := params[ref]
		if !ok {
			continue
		}
		if !strings.HasPrefix(p.Type, "AWS::SSM::Parameter::Value") {
			continue
		}
		if p.Default == wantDefault {
			return true
		}
	}

	return false
}

func synthesizeAPIGatewayTemplate(t *testing.T, props *APIGatewayProps) map[string]any {
	t.Helper()

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	dummy := awslambda.NewFunction(stack, _jsii.String("DummyFn"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: _jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(_jsii.String("exports.handler = async () => ({ statusCode: 200, body: 'ok' });")),
	})

	props.Functions = &LambdaFunctions{
		Functions: map[string]awslambda.Function{
			"api":         dummy,
			"graphql":     dummy,
			"sse":         dummy,
			"streaming":   dummy,
			"graphql-ws":  dummy,
			"actor":       dummy,
			"collections": dummy,
			"inbox":       dummy,
			"objects":     dummy,
			"outbox":      dummy,
			"webfinger":   dummy,
		},
	}

	_ = CreateAPIGateway(stack, props)
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
	return tpl
}

func preflightIntegrationForRoute(t *testing.T, tpl map[string]any, route string) map[string]any {
	t.Helper()

	optionsProps, ok := findMethodPropertiesByRouteKey(t, tpl, route)
	if !ok {
		t.Fatalf("expected %s route to exist", route)
	}
	integration, ok := optionsProps["Integration"].(map[string]any)
	if !ok {
		t.Fatalf("expected %s integration properties to exist", route)
	}
	return integration
}

func requireIntegrationProps(t *testing.T, methodProps map[string]any, routeKey string) map[string]any {
	t.Helper()

	integration, ok := methodProps["Integration"].(map[string]any)
	if !ok {
		t.Fatalf("expected %s integration properties to exist", routeKey)
	}
	return integration
}

func valueContainsString(v any, needle string) bool {
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), needle)
}

func requireNoApiGatewayCaching(t *testing.T, tpl map[string]any, routeKey string) {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template Resources missing or wrong type")
	}

	for logicalID, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::ApiGateway::Stage" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		if enabled, _ := props["CacheClusterEnabled"].(bool); enabled {
			t.Fatalf("%s must not enable API Gateway stage cache on %s", routeKey, logicalID)
		}
		methodSettings, _ := props["MethodSettings"].([]any)
		for _, rawSetting := range methodSettings {
			setting, ok := rawSetting.(map[string]any)
			if !ok {
				continue
			}
			if enabled, _ := setting["CachingEnabled"].(bool); !enabled {
				continue
			}
			methodPath, _ := setting["ResourcePath"].(string)
			httpMethod, _ := setting["HttpMethod"].(string)
			t.Fatalf("%s must not enable API Gateway method cache (stage %s setting %s %s)", routeKey, logicalID, httpMethod, methodPath)
		}
	}
}

func templateHasMcpInvokePermission(t *testing.T, tpl map[string]any, wantParamName string) bool {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		return false
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

	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::Lambda::Permission" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		principal, _ := props["Principal"].(string)
		if principal != "apigateway.amazonaws.com" {
			continue
		}
		if !valueReferencesSSMParameterDefault(t, tpl, props["FunctionName"], wantParamName) {
			continue
		}

		sourceArn := props["SourceArn"]
		if sourceArn == nil {
			continue
		}
		if restApiLogicalID != "" && !valueReferencesLogicalID(sourceArn, restApiLogicalID) {
			continue
		}

		return true
	}

	return false
}

func valueReferencesSSMParameterDefault(t *testing.T, tpl map[string]any, v any, wantDefault string) bool {
	t.Helper()

	if integrationURIReferencesSSMParameterDefault(t, tpl, v, wantDefault) {
		return true
	}

	b, err := json.Marshal(v)
	if err != nil {
		return false
	}

	return strings.Contains(string(b), wantDefault)
}

func valueReferencesLogicalID(v any, want string) bool {
	for _, ref := range findAllRefLogicalIDs(v) {
		if ref == want {
			return true
		}
	}
	return false
}

func findAllRefLogicalIDs(v any) []string {
	switch typed := v.(type) {
	case map[string]any:
		out := make([]string, 0)
		if ref, ok := typed["Ref"]; ok {
			if s, ok := ref.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		for _, child := range typed {
			out = append(out, findAllRefLogicalIDs(child)...)
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, child := range typed {
			out = append(out, findAllRefLogicalIDs(child)...)
		}
		return out
	default:
		return nil
	}
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

		fullPath := ""
		if resourceLogicalID, ok := findFirstRefLogicalID(props["ResourceId"]); ok {
			fullPath = buildPath(resourceLogicalID)
		} else if restApiLogicalID != "" && isRestApiRootResourceRef(props["ResourceId"], restApiLogicalID) {
			fullPath = "/"
		}
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

func findMethodPropertiesByRouteKey(t *testing.T, tpl map[string]any, wantRouteKey string) (map[string]any, bool) {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template Resources missing or wrong type")
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

		fullPath := ""
		if resourceLogicalID, ok := findFirstRefLogicalID(props["ResourceId"]); ok {
			fullPath = buildPath(resourceLogicalID)
		} else if restApiLogicalID != "" && isRestApiRootResourceRef(props["ResourceId"], restApiLogicalID) {
			fullPath = "/"
		}
		if fullPath == "" {
			continue
		}

		routeKey := fmt.Sprintf("%s %s", httpMethod, fullPath)
		if routeKey == wantRouteKey {
			return props, true
		}
	}

	return nil, false
}

func firstIntegrationResponseParameterValue(integration map[string]any, key string) string {
	rawResponses, ok := integration["IntegrationResponses"].([]any)
	if !ok || len(rawResponses) == 0 {
		return ""
	}

	firstResponse, ok := rawResponses[0].(map[string]any)
	if !ok {
		return ""
	}

	responseParameters, ok := firstResponse["ResponseParameters"].(map[string]any)
	if !ok {
		return ""
	}

	value, _ := responseParameters[key].(string)
	return value
}

func firstRequestTemplateValue(integration map[string]any, key string) string {
	rawTemplates, ok := integration["RequestTemplates"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := rawTemplates[key].(string)
	return value
}
