package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localconstructs "cdk/constructs"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

func TestInstancePlaneEnabledRequiresBodyOrSoulEnablement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config map[string]interface{}
		want   bool
	}{
		{
			name:   "dark by default",
			config: map[string]interface{}{},
			want:   false,
		},
		{
			name: "defaults on with body path",
			config: map[string]interface{}{
				"bodyEnabled": true,
			},
			want: true,
		},
		{
			name: "explicit false disables body path",
			config: map[string]interface{}{
				"bodyEnabled":          true,
				"instancePlaneEnabled": false,
			},
			want: false,
		},
		{
			name: "fails closed without body or soul",
			config: map[string]interface{}{
				"instancePlaneEnabled": true,
			},
			want: false,
		},
		{
			name: "enabled with body path",
			config: map[string]interface{}{
				"bodyEnabled":          true,
				"instancePlaneEnabled": true,
			},
			want: true,
		},
		{
			name: "defaults on with legacy soul path",
			config: map[string]interface{}{
				"soulEnabled": "true",
			},
			want: true,
		},
		{
			name: "enabled with legacy soul path",
			config: map[string]interface{}{
				"soulEnabled":          "true",
				"instancePlaneEnabled": "yes",
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := awscdk.NewApp(nil)
			stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
			apiStack := &LesserApiStack{
				Stack:         stack,
				Configuration: tc.config,
			}

			if got := apiStack.instancePlaneEnabled(); got != tc.want {
				t.Fatalf("instancePlaneEnabled()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestInstancePlaneEnabledRespectsNodeContextDefaults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contexts map[string]interface{}
		want     bool
	}{
		{
			name: "body context defaults instance plane on",
			contexts: map[string]interface{}{
				"bodyEnabled": "true",
			},
			want: true,
		},
		{
			name: "explicit false context disables instance plane",
			contexts: map[string]interface{}{
				"bodyEnabled":          "true",
				"instancePlaneEnabled": "false",
			},
			want: false,
		},
		{
			name: "instance context still requires body",
			contexts: map[string]interface{}{
				"instancePlaneEnabled": "true",
			},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := awscdk.NewApp(nil)
			for key, value := range tc.contexts {
				app.Node().SetContext(jsii.String(key), value)
			}
			stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
			apiStack := &LesserApiStack{Stack: stack}

			if got := apiStack.instancePlaneEnabled(); got != tc.want {
				t.Fatalf("instancePlaneEnabled()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBodyEnabledStackDefaultsInstancePlaneRoutesOn(t *testing.T) {
	tpl := synthesizeStackAPIGatewayTemplate(t, map[string]interface{}{
		"bodyEnabled": true,
	})

	for _, part := range []string{"instance", "ptah", "ba", "downloads", "installer-grants", "{grantId}"} {
		if !stackTemplateHasAPIGatewayPathPart(t, tpl, part) {
			t.Fatalf("expected API Gateway path part %q when bodyEnabled=true and instancePlaneEnabled is omitted", part)
		}
	}
	if !stackTemplateHasSSMParameterDefault(t, tpl, "/lesser/dev/lesser-body/exports/v1/instance_mcp_lambda_arn") {
		t.Fatal("expected default instance MCP Lambda SSM import when bodyEnabled=true and instancePlaneEnabled is omitted")
	}
}

func TestExplicitInstancePlaneFalseDisablesStackRoutes(t *testing.T) {
	tpl := synthesizeStackAPIGatewayTemplate(t, map[string]interface{}{
		"bodyEnabled":          true,
		"instancePlaneEnabled": false,
	})

	for _, part := range []string{"ptah", "ba", "installer-grants", "{grantId}"} {
		if stackTemplateHasAPIGatewayPathPart(t, tpl, part) {
			t.Fatalf("unexpected API Gateway path part %q when instancePlaneEnabled=false", part)
		}
	}
	if stackTemplateHasSSMParameterDefault(t, tpl, "/lesser/dev/lesser-body/exports/v1/instance_mcp_lambda_arn") {
		t.Fatal("unexpected instance MCP Lambda SSM import when instancePlaneEnabled=false")
	}
}

func synthesizeStackAPIGatewayTemplate(t *testing.T, config map[string]interface{}) map[string]any {
	t.Helper()

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	dummy := awslambda.NewFunction(stack, jsii.String("DummyFn"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => ({ statusCode: 200, body: 'ok' });")),
	})
	functions := &localconstructs.LambdaFunctions{
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
	basicRole := awsiam.NewRole(stack, jsii.String("BasicRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	encryptionRole := awsiam.NewRole(stack, jsii.String("EncryptionRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	apiStack := &LesserApiStack{
		Stack:                stack,
		Environment:          "development",
		Configuration:        config,
		AppName:              "lesser",
		Domain:               "dev.example.com",
		Functions:            functions,
		LambdaBasicRole:      basicRole,
		LambdaEncryptionRole: encryptionRole,
	}
	apiStack.createAPIGateway("dev.example.com")
	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestStack.template.json")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template %s: %v", templatePath, err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(data, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	return tpl
}

func stackTemplateHasAPIGatewayPathPart(t *testing.T, tpl map[string]any, want string) bool {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template Resources missing or wrong type")
	}
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok || res["Type"] != "AWS::ApiGateway::Resource" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		if part, _ := props["PathPart"].(string); part == want {
			return true
		}
	}
	return false
}

func stackTemplateHasSSMParameterDefault(t *testing.T, tpl map[string]any, wantDefault string) bool {
	t.Helper()

	parameters, ok := tpl["Parameters"].(map[string]any)
	if !ok {
		return false
	}
	for _, raw := range parameters {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := param["Type"].(string)
		def, _ := param["Default"].(string)
		if def == wantDefault && strings.HasPrefix(typ, "AWS::SSM::Parameter::Value") {
			return true
		}
	}
	return false
}
