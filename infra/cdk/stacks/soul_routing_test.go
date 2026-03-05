package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/jsii-runtime-go"
)

func TestSoulRuntimeEnabledDoesNotAddSoulRoutingBehaviors(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stageDomain := "dev.example.com"

	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	hostedZone := awsroute53.NewHostedZone(stack, jsii.String("HostedZone"), &awsroute53.HostedZoneProps{
		ZoneName: jsii.String("example.com"),
	})

	apiStack := &LesserApiStack{
		Stack:         stack,
		Environment:   "development",
		Domain:        stageDomain,
		Configuration: map[string]interface{}{"soulRuntimeEnabled": true},
		HostedZone:    hostedZone,
		AppName:       "test",
		AccountID:     "123456789012",
		Region:        "us-east-1",
	}

	apiStack.createStageCertificates(stageDomain)
	apiStack.createClientInfrastructure(stageDomain)

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

	// /soul orchestrator routing is deprecated and should not exist anywhere in the stack template.
	wantSSMDefault := "/soul/" + stageDomain + "/exports/v1/orchestrator_origin_domain"
	requireNoSSMStringParameterValueDefault(t, tpl, wantSSMDefault)

	frontendDist := findSingleCloudFrontDistribution(t, resources)
	cacheBehaviors := extractCacheBehaviors(t, frontendDist)
	for _, behavior := range cacheBehaviors {
		path, _ := behavior["PathPattern"].(string)
		switch path {
		case "/soul", "/soul/*":
			t.Fatalf("unexpected /soul routing CacheBehavior: %q", path)
		}
	}
}

func requireNoSSMStringParameterValueDefault(t *testing.T, tpl map[string]any, wantDefault string) {
	t.Helper()

	raw, ok := tpl["Parameters"].(map[string]any)
	if !ok {
		return
	}

	for _, entryAny := range raw {
		entry, ok := entryAny.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := entry["Type"].(string); typ != "AWS::SSM::Parameter::Value<String>" {
			continue
		}
		if def, _ := entry["Default"].(string); def != wantDefault {
			continue
		}
		t.Fatalf("unexpected SSM Parameter::Value<String> with Default=%q", wantDefault)
	}
}
