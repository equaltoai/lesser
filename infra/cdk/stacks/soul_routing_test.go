package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
)

func TestStageExportsPublishedToSSM(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	mainTable := awsdynamodb.NewTable(stack, jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
	})

	mediaBucket := awss3.NewBucket(stack, jsii.String("MediaBucket"), nil)

	apiStack := &LesserApiStack{
		Stack:       stack,
		MainTable:   mainTable,
		MediaBucket: mediaBucket,
		AppName:     "lesser",
		Environment: "development",
		Domain:      "dev.example.com",
	}
	apiStack.publishStageExportsToSSM()

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

	gotNames := extractSSMParameterNames(t, tpl)
	wantNames := []string{
		"/lesser/dev/lesser/exports/v1/table_name",
		"/lesser/dev/lesser/exports/v1/media_bucket_name",
		"/lesser/dev/lesser/exports/v1/domain",
	}

	for _, want := range wantNames {
		var found bool
		for _, got := range gotNames {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing SSM parameter %q (got %v)", want, gotNames)
		}
	}
}

func extractSSMParameterNames(t *testing.T, tpl map[string]any) []string {
	t.Helper()

	resources := mustResources(t, tpl)
	out := make([]string, 0)
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::SSM::Parameter" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := props["Name"].(string)
		if !ok || name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}
