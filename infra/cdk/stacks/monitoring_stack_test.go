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
)

func TestMonitoringStackSynthCreatesInventoryLambdaAlarms(t *testing.T) {
	tpl := synthMonitoringTemplate(t, "development")
	alarms := collectAlarmNames(t, tpl)

	for _, spec := range inventory.LambdaInventory.Lambdas {
		physical := lambdaPhysicalName("development", spec.Name)
		requireAlarm(t, alarms, fmt.Sprintf("%s-error-rate", physical))
		requireAlarm(t, alarms, fmt.Sprintf("%s-duration", physical))
		requireAlarm(t, alarms, fmt.Sprintf("%s-throttles", physical))
		if isStreamLambda(spec) {
			requireAlarm(t, alarms, fmt.Sprintf("%s-iterator-age", physical))
		}
	}
}

func TestMonitoringStackSynthCreatesQueueAndTableAlarms(t *testing.T) {
	tpl := synthMonitoringTemplate(t, "development")
	alarms := collectAlarmNames(t, tpl)

	for _, q := range deriveInventoryQueues() {
		primary := queuePhysicalName("development", q.Logical)
		dlq := queuePhysicalName("development", q.DLQLogical)
		requireAlarm(t, alarms, fmt.Sprintf("%s-age", primary))
		requireAlarm(t, alarms, fmt.Sprintf("%s-depth", dlq))
	}

	requireAlarm(t, alarms, "lesser-development-read-throttles")
	requireAlarm(t, alarms, "lesser-development-write-throttles")
	requireAlarm(t, alarms, "lesser-rate-limits-development-read-throttles")
	requireAlarm(t, alarms, "lesser-rate-limits-development-write-throttles")
}

func TestMonitoringStackSynthDoesNotProvisionApplicationWiring(t *testing.T) {
	tpl := synthMonitoringTemplate(t, "development")
	resourceTypes := collectResourceTypes(t, tpl)

	disallowed := []string{
		"AWS::SQS::Queue",
		"AWS::Lambda::EventSourceMapping",
		"AWS::Events::Rule",
	}

	for _, typ := range disallowed {
		if count := resourceTypes[typ]; count > 0 {
			t.Fatalf("monitoring stack must not provision %s (found %d)", typ, count)
		}
	}
}

func synthMonitoringTemplate(t *testing.T, environment string) map[string]any {
	t.Helper()

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})

	NewMonitoringStack(app, "TestMonitoringStack", &MonitoringStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String("123456789012"),
				Region:  jsii.String("us-east-1"),
			},
		},
		AppName:     "lesser",
		Environment: environment,
		AlertEmail:  "",
	})

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestMonitoringStack.template.json")
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

func collectAlarmNames(t *testing.T, tpl map[string]any) map[string]struct{} {
	t.Helper()

	out := map[string]struct{}{}
	resources := mustResources(t, tpl)
	for _, resAny := range resources {
		res, ok := resAny.(map[string]any)
		if !ok {
			continue
		}
		if res["Type"] != "AWS::CloudWatch::Alarm" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := props["AlarmName"].(string)
		if !ok || name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func collectResourceTypes(t *testing.T, tpl map[string]any) map[string]int {
	t.Helper()

	out := map[string]int{}
	resources := mustResources(t, tpl)
	for _, resAny := range resources {
		res, ok := resAny.(map[string]any)
		if !ok {
			continue
		}
		typ, ok := res["Type"].(string)
		if !ok || typ == "" {
			continue
		}
		out[typ]++
	}
	return out
}

func mustResources(t *testing.T, tpl map[string]any) map[string]any {
	t.Helper()

	resourcesAny, ok := tpl["Resources"]
	if !ok {
		t.Fatalf("template missing Resources")
	}
	resources, ok := resourcesAny.(map[string]any)
	if !ok {
		t.Fatalf("template Resources has unexpected type %T", resourcesAny)
	}
	return resources
}

func requireAlarm(t *testing.T, alarms map[string]struct{}, name string) {
	t.Helper()
	if _, ok := alarms[name]; !ok {
		t.Fatalf("missing expected alarm: %s", name)
	}
}
