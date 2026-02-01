package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/jsii-runtime-go"
)

func TestExecuteApiManageConnectionsPolicyIsStageScoped(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	role := awsiam.NewRole(stack, jsii.String("Role"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})
	role2 := awsiam.NewRole(stack, jsii.String("Role2"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	ws1 := awsapigatewayv2.NewWebSocketApi(stack, jsii.String("WebSocketApiOne"), &awsapigatewayv2.WebSocketApiProps{
		ApiName: jsii.String("ws-one"),
	})
	ws2 := awsapigatewayv2.NewWebSocketApi(stack, jsii.String("WebSocketApiTwo"), &awsapigatewayv2.WebSocketApiProps{
		ApiName: jsii.String("ws-two"),
	})

	attachWebSocketManageConnectionsPolicy(
		stack,
		"lesser",
		"development",
		[]awsiam.IRole{role, role2},
		[]awsapigatewayv2.WebSocketApi{ws1, ws2},
	)

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestStack.template.json")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(data, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	resources := mustResources(t, tpl)
	policies := collectIAMPolicies(t, resources)
	if len(policies) != 1 {
		t.Fatalf("expected exactly one IAM policy resource, found %d", len(policies))
	}

	doc := extractPolicyDocument(t, policies[0])
	statements := extractPolicyStatements(t, doc)
	if len(statements) != 1 {
		t.Fatalf("expected exactly one policy statement, found %d", len(statements))
	}

	statement := statements[0]
	actions := extractStatementActions(t, statement)
	if !containsString(actions, "execute-api:ManageConnections") {
		t.Fatalf("expected execute-api:ManageConnections action, got %v", actions)
	}
	if containsString(actions, "execute-api:Invoke") {
		t.Fatalf("unexpected execute-api:Invoke action present")
	}

	res := extractStatementResources(t, statement)
	if len(res) != 2 {
		t.Fatalf("expected two execute-api resources, got %d", len(res))
	}
	for _, r := range res {
		base, vars := extractFnSub(t, r)
		if strings.Contains(base, ":execute-api:*:*:*") {
			t.Fatalf("execute-api ARN must not use wildcards: %s", base)
		}
		if !strings.Contains(base, "POST/@connections/*") {
			t.Fatalf("execute-api ARN must scope to @connections: %s", base)
		}
		stage, ok := vars["Stage"].(string)
		if !ok || stage != "dev" {
			t.Fatalf("expected Stage=dev in Fn::Sub vars, got %#v", vars["Stage"])
		}
	}
}

func TestSharedStackDoesNotDefineExecuteApiPolicies(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})

	NewSharedStack(app, "TestSharedStack", &SharedStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String("123456789012"),
				Region:  jsii.String("us-east-1"),
			},
		},
		AppName: "lesser",
	})

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestSharedStack.template.json")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(data, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	resources := mustResources(t, tpl)
	policies := collectIAMPolicies(t, resources)
	for _, policy := range policies {
		doc := extractPolicyDocument(t, policy)
		for _, statement := range extractPolicyStatements(t, doc) {
			actions := extractStatementActions(t, statement)
			for _, action := range actions {
				if strings.HasPrefix(action, "execute-api:") {
					t.Fatalf("shared stack must not define execute-api permissions (found %s)", action)
				}
			}
		}
	}
}

func collectIAMPolicies(t *testing.T, resources map[string]any) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::IAM::Policy" {
			continue
		}
		out = append(out, res)
	}
	return out
}

func extractPolicyDocument(t *testing.T, policy map[string]any) map[string]any {
	t.Helper()

	props, ok := policy["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("policy missing Properties")
	}
	doc, ok := props["PolicyDocument"].(map[string]any)
	if !ok {
		t.Fatalf("policy missing PolicyDocument")
	}
	return doc
}

func extractPolicyStatements(t *testing.T, doc map[string]any) []map[string]any {
	t.Helper()

	raw, ok := doc["Statement"]
	if !ok {
		t.Fatalf("policy document missing Statement")
	}

	switch v := raw.(type) {
	case []any:
		var out []map[string]any
		for _, item := range v {
			s, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("statement has unexpected type %T", item)
			}
			out = append(out, s)
		}
		return out
	case map[string]any:
		return []map[string]any{v}
	default:
		t.Fatalf("statement has unexpected type %T", raw)
	}
	return nil
}

func extractStatementActions(t *testing.T, statement map[string]any) []string {
	t.Helper()

	raw, ok := statement["Action"]
	if !ok {
		t.Fatalf("statement missing Action")
	}

	switch v := raw.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				t.Fatalf("action has unexpected type %T", item)
			}
			out = append(out, s)
		}
		return out
	default:
		t.Fatalf("action has unexpected type %T", raw)
	}
	return nil
}

func extractStatementResources(t *testing.T, statement map[string]any) []any {
	t.Helper()

	raw, ok := statement["Resource"]
	if !ok {
		t.Fatalf("statement missing Resource")
	}
	switch v := raw.(type) {
	case []any:
		return v
	case string:
		return []any{v}
	default:
		t.Fatalf("resource has unexpected type %T", raw)
	}
	return nil
}

func extractFnSub(t *testing.T, raw any) (string, map[string]any) {
	t.Helper()

	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected Fn::Sub object, got %T", raw)
	}
	subRaw, ok := obj["Fn::Sub"]
	if !ok {
		t.Fatalf("expected Fn::Sub, got keys %v", keysOf(obj))
	}
	list, ok := subRaw.([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("expected Fn::Sub to be [string, vars], got %T", subRaw)
	}
	base, ok := list[0].(string)
	if !ok {
		t.Fatalf("expected Fn::Sub base string, got %T", list[0])
	}
	vars, ok := list[1].(map[string]any)
	if !ok {
		t.Fatalf("expected Fn::Sub vars map, got %T", list[1])
	}
	return base, vars
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func containsString(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
