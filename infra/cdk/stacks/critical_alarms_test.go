package stacks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func TestLesserApiStackSynthCreatesCriticalProcessorAlarms(t *testing.T) {
	appName := "lesser"
	environment := "development"
	tpl := synthLesserApiStageTemplate(t, appName, environment)
	alarms := collectAlarmNames(t, tpl)

	for _, spec := range inventory.LambdaInventory.Lambdas {
		physical := lambdaPhysicalName(appName, environment, spec.Name)
		requireAlarm(t, alarms, fmt.Sprintf("%s-critical-error-rate", physical))
		requireAlarm(t, alarms, fmt.Sprintf("%s-critical-errors", physical))
		if isStreamLambda(spec) {
			requireAlarm(t, alarms, fmt.Sprintf("%s-critical-iterator-age", physical))
		}
		if len(spec.ScheduleTriggers) > 0 {
			requireAlarm(t, alarms, fmt.Sprintf("%s-critical-scheduled-errors", physical))
		}
	}

	for _, q := range deriveInventoryQueues() {
		primary := queuePhysicalName(appName, environment, q.Logical)
		dlq := queuePhysicalName(appName, environment, q.DLQLogical)
		requireAlarm(t, alarms, fmt.Sprintf("%s-critical-age", primary))
		requireAlarm(t, alarms, fmt.Sprintf("%s-critical-depth", primary))
		requireAlarm(t, alarms, fmt.Sprintf("%s-critical-depth", dlq))
	}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		for _, trig := range spec.StreamTriggers {
			poison := queuePhysicalName(appName, environment, trig.PoisonRecordQueue)
			requireAlarm(t, alarms, fmt.Sprintf("%s-critical-depth", poison))
		}
		for idx := range spec.ScheduleTriggers {
			scheduleDLQ := queuePhysicalName(appName, environment, fmt.Sprintf("%s-schedule-%d-dlq", spec.Name, idx))
			requireAlarm(t, alarms, fmt.Sprintf("%s-critical-depth", scheduleDLQ))
		}
	}
}

func synthLesserApiStageTemplate(t *testing.T, appName string, environment string) map[string]any {
	t.Helper()

	outdir := t.TempDir()
	assetRoot := writePlaceholderLambdaAssets(t, t.TempDir())
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})

	NewLesserApiStack(app, "CriticalAlarmStageStack", &LesserApiStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String("123456789012"),
				Region:  jsii.String("us-east-1"),
			},
		},
		Environment:      environment,
		Domain:           naming.StageDomain(naming.StageForEnvironment(environment), "example.com"),
		Config:           map[string]interface{}{"lambdaAssetRoot": assetRoot},
		HostedZoneDomain: "example.com",
		HostedZoneId:     "Z1",
		AppName:          appName,
		AccountID:        "123456789012",
		Region:           "us-east-1",
	})

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "CriticalAlarmStageStack.template.json")
	tpl := readTemplateFile(t, templatePath)
	for _, nestedPath := range nestedTemplatePaths(t, outdir) {
		mergeTemplateResources(t, tpl, readTemplateFile(t, nestedPath), filepath.Base(nestedPath))
	}
	return tpl
}

func nestedTemplatePaths(t *testing.T, outdir string) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(outdir, "*.nested.template.json"))
	if err != nil {
		t.Fatalf("glob nested templates: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected critical alarms nested stack template")
	}
	return paths
}

func readTemplateFile(t *testing.T, templatePath string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(data, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	return tpl
}

func mergeTemplateResources(t *testing.T, into map[string]any, from map[string]any, prefix string) {
	t.Helper()

	intoResources := mustResources(t, into)
	fromResources := mustResources(t, from)
	for logicalID, resource := range fromResources {
		intoResources[prefix+"::"+logicalID] = resource
	}
}
