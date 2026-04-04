package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/jsii-runtime-go"
)

func TestConfiguredSecretReadPolicyUsesExactConfiguredSecretARNs(t *testing.T) {
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

	attachConfiguredSecretReadPolicy(stack, "lesser", "development", []awsiam.IRole{role, role2}, map[string]interface{}{
		"lesserHostInstanceKeyArn": " arn:aws:secretsmanager:us-east-1:123456789012:secret:lesser-host/lab/instances/theory/instance-key-abc123 ",
		"vapidSecretArn":           "arn:aws:secretsmanager:us-east-1:123456789012:secret:vapid-xyz789",
	})

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
	if !containsString(actions, "secretsmanager:GetSecretValue") || !containsString(actions, "secretsmanager:DescribeSecret") {
		t.Fatalf("expected secretsmanager read actions, got %v", actions)
	}

	res := extractStatementResources(t, statement)
	if len(res) != 2 {
		t.Fatalf("expected two exact secret resources, got %d", len(res))
	}

	if got, ok := res[0].(string); !ok || got != "arn:aws:secretsmanager:us-east-1:123456789012:secret:lesser-host/lab/instances/theory/instance-key-abc123" {
		t.Fatalf("unexpected first secret resource: %#v", res[0])
	}
	if got, ok := res[1].(string); !ok || got != "arn:aws:secretsmanager:us-east-1:123456789012:secret:vapid-xyz789" {
		t.Fatalf("unexpected second secret resource: %#v", res[1])
	}
}

func TestConfiguredSecretReadPolicySkipsWhenNoConfiguredSecretARNs(t *testing.T) {
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

	attachConfiguredSecretReadPolicy(stack, "lesser", "development", []awsiam.IRole{role}, map[string]interface{}{
		"lesserHostUrl": "https://lab.lesser.host",
	})

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
	if len(policies) != 0 {
		t.Fatalf("expected no IAM policy resources, found %d", len(policies))
	}
}
