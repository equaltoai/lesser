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
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
)

type eventSourceMapping struct {
	FunctionName               string
	SourceLogical              string
	SourceAttr                 string
	DestinationLogical         string
	MaximumRetryAttempts       float64
	MaximumRecordAgeInSeconds  float64
	BisectBatchOnFunctionError bool
	ReportsBatchItemFailures   bool
}

type queueDetails struct {
	LogicalID     string
	QueueName     string
	RedriveTarget string
	MaxReceive    float64
}

type scheduleRule struct {
	Expression string
	TargetName string
}

func TestInventoryTriggersMaterializeResources(t *testing.T) {
	moduleRoot := ensureModuleRoot(t)
	ensureLambdaAssets(t, moduleRoot)

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	mainTable := awsdynamodb.NewTable(stack, _jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: _jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: _jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
		Stream:       awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
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

	environment := "development"
	functions := CreateLambdaFunctions(stack, &LambdaFunctionsProps{
		Environment:         environment,
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
	queues := buildInventoryQueues(stack, functions, environment)
	ApplyQueueEnvironmentVariables(functions, queues)

	CreateStreamProcessors(stack, &StreamProcessorsProps{
		AppName:       naming.DefaultAppName,
		Environment:   environment,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		Table:         mainTable,
		Queues:        queues,
		Functions:     functions,
	})

	app.Synth(nil)

	tpl := loadTemplate(t, filepath.Join(outdir, "TestStack.template.json"))
	mappings := collectEventSourceMappings(t, tpl)
	queuesMeta := collectQueuesByName(t, tpl)
	rules := collectScheduleRules(t, tpl)
	expectedSqsSources := map[string][]string{}
	expectedStreamCounts := map[string]int{}
	expectedScheduleExprs := map[string][]string{}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		fnName := naming.ResourceName(spec.Name, environment)
		expectedStreamCounts[fnName] = len(spec.StreamTriggers)

		if len(spec.StreamTriggers) > 0 {
			if countMappingsBySourceAttr(mappings, fnName, "StreamArn") != len(spec.StreamTriggers) {
				t.Fatalf("stream trigger mapping mismatch for %s", spec.Name)
			}
			for _, trig := range spec.StreamTriggers {
				poisonName := naming.ResourceName(trig.PoisonRecordQueue, environment)
				poisonQueue, ok := queuesMeta[poisonName]
				if !ok {
					t.Fatalf("stream poison queue %s not created for %s", poisonName, spec.Name)
				}
				mapping := requireStreamMapping(t, mappings, fnName)
				if mapping.MaximumRetryAttempts != float64(trig.MaxRetryAttempts) {
					t.Fatalf("%s retry attempts = %v, want %d", spec.Name, mapping.MaximumRetryAttempts, trig.MaxRetryAttempts)
				}
				if mapping.MaximumRecordAgeInSeconds != float64(trig.MaxRecordAgeSeconds) {
					t.Fatalf("%s max record age = %v, want %d", spec.Name, mapping.MaximumRecordAgeInSeconds, trig.MaxRecordAgeSeconds)
				}
				if mapping.DestinationLogical != poisonQueue.LogicalID {
					t.Fatalf("%s poison destination = %s, want %s", spec.Name, mapping.DestinationLogical, poisonQueue.LogicalID)
				}
				if mapping.BisectBatchOnFunctionError != trig.EnableBisectOnError {
					t.Fatalf("%s bisect setting = %v, want %v", spec.Name, mapping.BisectBatchOnFunctionError, trig.EnableBisectOnError)
				}
				if mapping.ReportsBatchItemFailures != trig.ReportBatchItemFailures {
					t.Fatalf("%s partial failure setting = %v, want %v", spec.Name, mapping.ReportsBatchItemFailures, trig.ReportBatchItemFailures)
				}
			}
		}

		for _, trig := range spec.SQSTriggers {
			primaryName := naming.ResourceName(trig.Queue, environment)

			dlqLogical := trig.DeadLetterQueue
			if dlqLogical == "" {
				dlqLogical = fmt.Sprintf("%s-dlq", trig.Queue)
			}

			dlqName := naming.ResourceName(dlqLogical, environment)
			dlq, ok := queuesMeta[dlqName]
			if !ok {
				t.Fatalf("dlq %s not created", dlqName)
			}
			q, ok := queuesMeta[primaryName]
			if !ok {
				t.Fatalf("queue %s not created", primaryName)
			}

			expectedMappingQueue := q
			if trig.ConsumeDeadLetterQueue {
				expectedMappingQueue = dlq
			}
			expectedSqsSources[fnName] = append(expectedSqsSources[fnName], expectedMappingQueue.LogicalID)
			if countMappings(mappings, fnName, expectedMappingQueue.LogicalID, "Arn") == 0 {
				t.Fatalf("missing SQS mapping for %s -> %s", spec.Name, trig.Queue)
			}

			if q.RedriveTarget != dlq.LogicalID {
				t.Fatalf("queue %s redrive target mismatch: got %s want %s", q.QueueName, q.RedriveTarget, dlq.LogicalID)
			}
			if q.MaxReceive < 5 {
				t.Fatalf("queue %s redrive maxReceive too low: %v", q.QueueName, q.MaxReceive)
			}
		}

		for _, trig := range spec.ScheduleTriggers {
			expectedScheduleExprs[fnName] = append(expectedScheduleExprs[fnName], trig.Expression)
			if !ruleExists(rules, trig.Expression, fnName) {
				t.Fatalf("missing schedule rule for %s with expression %s", spec.Name, trig.Expression)
			}
		}
	}

	actualSqsSources := map[string][]string{}
	actualStreamSources := map[string][]string{}
	for _, m := range mappings {
		switch m.SourceAttr {
		case "Arn":
			actualSqsSources[m.FunctionName] = append(actualSqsSources[m.FunctionName], m.SourceLogical)
		case "StreamArn":
			actualStreamSources[m.FunctionName] = append(actualStreamSources[m.FunctionName], m.SourceLogical)
		}
	}

	actualScheduleExprs := map[string][]string{}
	for _, rule := range rules {
		actualScheduleExprs[rule.TargetName] = append(actualScheduleExprs[rule.TargetName], rule.Expression)
	}

	for fn, expected := range expectedSqsSources {
		actual := actualSqsSources[fn]
		missing, extra := diffStringSets(expected, actual)
		if len(missing) > 0 || len(extra) > 0 {
			t.Fatalf("sqs mapping mismatch for %s: missing=%v extra=%v actual=%v expected=%v", fn, missing, extra, actual, expected)
		}
	}
	for fn, actual := range actualSqsSources {
		if len(expectedSqsSources[fn]) == 0 && len(actual) > 0 {
			t.Fatalf("unexpected sqs mappings for %s: %v", fn, actual)
		}
	}

	for fn, expCount := range expectedStreamCounts {
		actualCount := len(actualStreamSources[fn])
		if actualCount != expCount {
			t.Fatalf("stream mapping mismatch for %s: expected %d got %d (sources=%v)", fn, expCount, actualCount, actualStreamSources[fn])
		}
	}
	for fn, streamSources := range actualStreamSources {
		if expectedStreamCounts[fn] == 0 && len(streamSources) > 0 {
			t.Fatalf("unexpected stream mappings for %s: %v", fn, streamSources)
		}
	}

	for fn, expected := range expectedScheduleExprs {
		actual := actualScheduleExprs[fn]
		missing, extra := diffStringSets(expected, actual)
		if len(missing) > 0 || len(extra) > 0 {
			t.Fatalf("schedule rule mismatch for %s: missing=%v extra=%v actual=%v expected=%v", fn, missing, extra, actual, expected)
		}
	}
	for fn, actual := range actualScheduleExprs {
		if len(expectedScheduleExprs[fn]) == 0 && len(actual) > 0 {
			t.Fatalf("unexpected schedule rules for %s: %v", fn, actual)
		}
	}
}

func TestMissingQueuePanics(t *testing.T) {
	moduleRoot := ensureModuleRoot(t)
	ensureLambdaAssets(t, moduleRoot)

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	mainTable := awsdynamodb.NewTable(stack, _jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: _jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: _jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
		Stream:       awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
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

	environment := "development"
	functions := CreateLambdaFunctions(stack, &LambdaFunctionsProps{
		Environment:         environment,
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

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic when queue is missing")
		}
	}()
	CreateStreamProcessors(stack, &StreamProcessorsProps{
		AppName:       naming.DefaultAppName,
		Environment:   environment,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		Table:         mainTable,
		Queues:        map[string]QueuePair{},
		Functions:     functions,
	})
}

func TestScheduleTypeValidationPanics(t *testing.T) {
	orig := inventory.LambdaInventory
	badSpec := inventory.LambdaSpec{
		Name: "bad-schedule",
		Type: inventory.LambdaTypeAPIHTTP,
		ScheduleTriggers: []inventory.ScheduleTrigger{
			{Expression: "rate(5 minutes)"},
		},
	}
	inventory.LambdaInventory = inventory.Inventory{Defaults: orig.Defaults, Lambdas: append(orig.Lambdas, badSpec)}
	t.Cleanup(func() { inventory.LambdaInventory = orig })

	moduleRoot := ensureModuleRoot(t)
	ensureLambdaAssets(t, moduleRoot)

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	mainTable := awsdynamodb.NewTable(stack, _jsii.String("MainTable"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{Name: _jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:      &awsdynamodb.Attribute{Name: _jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		BillingMode:  awsdynamodb.BillingMode_PAY_PER_REQUEST,
		Stream:       awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
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

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for schedule trigger on unsupported lambda type")
		}
	}()

	_ = CreateLambdaFunctions(stack, &LambdaFunctionsProps{
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
}

func collectEventSourceMappings(t *testing.T, tpl map[string]any) []eventSourceMapping {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template resources missing")
	}
	lambdaNames := resolveLambdaLogicalToName(resources)

	mappings := make([]eventSourceMapping, 0)
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok || res["Type"] != "AWS::Lambda::EventSourceMapping" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		fnLogical, ok := findFirstRefLogicalIDDeep(props["FunctionName"])
		if !ok {
			continue
		}
		fnName := lambdaNames[fnLogical]
		srcLogical, srcAttr := extractLogicalAndAttr(props["EventSourceArn"])
		if srcLogical == "" {
			continue
		}
		mappings = append(mappings, eventSourceMapping{
			FunctionName:               fnName,
			SourceLogical:              srcLogical,
			SourceAttr:                 srcAttr,
			DestinationLogical:         extractEventSourceMappingDestinationLogical(props),
			MaximumRetryAttempts:       numericProperty(props, "MaximumRetryAttempts"),
			MaximumRecordAgeInSeconds:  numericProperty(props, "MaximumRecordAgeInSeconds"),
			BisectBatchOnFunctionError: boolProperty(props, "BisectBatchOnFunctionError"),
			ReportsBatchItemFailures:   containsStringAny(props["FunctionResponseTypes"], "ReportBatchItemFailures"),
		})
	}
	return mappings
}

func requireStreamMapping(t *testing.T, mappings []eventSourceMapping, fnName string) eventSourceMapping {
	t.Helper()
	var found []eventSourceMapping
	for _, m := range mappings {
		if m.FunctionName == fnName && m.SourceAttr == "StreamArn" {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly one stream mapping for %s, got %d", fnName, len(found))
	}
	return found[0]
}

func extractEventSourceMappingDestinationLogical(props map[string]any) string {
	cfg, ok := props["DestinationConfig"].(map[string]any)
	if !ok {
		return ""
	}
	onFailure, ok := cfg["OnFailure"].(map[string]any)
	if !ok {
		return ""
	}
	logical, _ := extractLogicalAndAttr(onFailure["Destination"])
	return logical
}

func numericProperty(props map[string]any, key string) float64 {
	switch v := props[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}

func boolProperty(props map[string]any, key string) bool {
	v, _ := props[key].(bool)
	return v
}

func containsStringAny(v any, needle string) bool {
	items, ok := v.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if s, ok := item.(string); ok && s == needle {
			return true
		}
	}
	return false
}

func collectQueuesByName(t *testing.T, tpl map[string]any) map[string]queueDetails {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template resources missing")
	}
	result := make(map[string]queueDetails)
	for logical, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok || res["Type"] != "AWS::SQS::Queue" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		qd := queueDetails{LogicalID: logical}
		if name, ok := props["QueueName"].(string); ok {
			qd.QueueName = name
		}
		if rp, ok := props["RedrivePolicy"].(map[string]any); ok {
			if target, ok := rp["deadLetterTargetArn"]; ok {
				dlqLogical, _ := extractLogicalAndAttr(target)
				qd.RedriveTarget = dlqLogical
			}
			switch mrc := rp["maxReceiveCount"].(type) {
			case float64:
				qd.MaxReceive = mrc
			case int:
				qd.MaxReceive = float64(mrc)
			}
		}
		if qd.QueueName == "" {
			continue
		}
		result[qd.QueueName] = qd
	}
	return result
}

func collectScheduleRules(t *testing.T, tpl map[string]any) []scheduleRule {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template resources missing")
	}
	lambdaNames := resolveLambdaLogicalToName(resources)

	rules := make([]scheduleRule, 0)
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok || res["Type"] != "AWS::Events::Rule" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		expr, _ := props["ScheduleExpression"].(string)
		targets, ok := props["Targets"].([]any)
		if !ok {
			continue
		}
		for _, tgt := range targets {
			tmap, ok := tgt.(map[string]any)
			if !ok {
				continue
			}
			logical, _ := extractLogicalAndAttr(tmap["Arn"])
			if logical == "" {
				continue
			}
			fnName := lambdaNames[logical]
			rules = append(rules, scheduleRule{
				Expression: expr,
				TargetName: fnName,
			})
		}
	}
	return rules
}

func resolveLambdaLogicalToName(resources map[string]any) map[string]string {
	result := make(map[string]string)
	for logical, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok || res["Type"] != "AWS::Lambda::Function" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		if name, ok := props["FunctionName"].(string); ok && name != "" {
			result[logical] = name
		}
	}
	return result
}

func countMappings(mappings []eventSourceMapping, fnName, logical, attr string) int {
	count := 0
	for _, m := range mappings {
		if m.FunctionName == fnName && m.SourceLogical == logical && m.SourceAttr == attr {
			count++
		}
	}
	return count
}

func countMappingsBySourceAttr(mappings []eventSourceMapping, fnName, attr string) int {
	count := 0
	for _, m := range mappings {
		if m.FunctionName == fnName && m.SourceAttr == attr {
			count++
		}
	}
	return count
}

func ruleExists(rules []scheduleRule, expression, fnName string) bool {
	for _, r := range rules {
		if r.Expression == expression && r.TargetName == fnName {
			return true
		}
	}
	return false
}

func diffStringSets(expected, actual []string) (missing []string, extra []string) {
	exp := map[string]struct{}{}
	act := map[string]struct{}{}
	for _, e := range expected {
		exp[e] = struct{}{}
	}
	for _, a := range actual {
		act[a] = struct{}{}
	}
	for e := range exp {
		if _, ok := act[e]; !ok {
			missing = append(missing, e)
		}
	}
	for a := range act {
		if _, ok := exp[a]; !ok {
			extra = append(extra, a)
		}
	}
	return
}

func findFirstRefLogicalIDDeep(v any) (string, bool) {
	switch typed := v.(type) {
	case map[string]any:
		if ref, ok := typed["Ref"].(string); ok && ref != "" {
			return ref, true
		}
		if logical, _ := extractLogicalAndAttr(typed); logical != "" {
			return logical, true
		}
		for _, child := range typed {
			if logical, ok := findFirstRefLogicalIDDeep(child); ok {
				return logical, true
			}
		}
	case []any:
		for _, child := range typed {
			if logical, ok := findFirstRefLogicalIDDeep(child); ok {
				return logical, true
			}
		}
	case string:
		if typed != "" {
			return typed, true
		}
	}
	return "", false
}

func extractLogicalAndAttr(v any) (string, string) {
	switch typed := v.(type) {
	case map[string]any:
		if getAtt, ok := typed["Fn::GetAtt"]; ok {
			switch att := getAtt.(type) {
			case []any:
				if len(att) == 2 {
					logical, _ := att[0].(string)
					attr, _ := att[1].(string)
					return logical, attr
				}
			case string:
				parts := strings.Split(att, ".")
				if len(parts) == 2 {
					return parts[0], parts[1]
				}
			}
		}
		for _, child := range typed {
			if logical, attr := extractLogicalAndAttr(child); logical != "" {
				return logical, attr
			}
		}
	case []any:
		for _, child := range typed {
			if logical, attr := extractLogicalAndAttr(child); logical != "" {
				return logical, attr
			}
		}
	}
	return "", ""
}

func sanitizeQueueLogical(queue string) string {
	clean := strings.ReplaceAll(queue, "-", "")
	clean = strings.ReplaceAll(clean, "_", "")
	if clean == "" {
		return "Queue"
	}
	return clean
}

func buildInventoryQueues(stack awscdk.Stack, functions *LambdaFunctions, environment string) map[string]QueuePair {
	queuePairs := map[string]QueuePair{}
	defaultVisibility := awscdk.Duration_Minutes(_jsii.Number(2))
	defaultRetention := awscdk.Duration_Days(_jsii.Number(4))
	defaultMaxReceive := _jsii.Number(5)

	primaryConsumerByQueue := map[string]string{}
	primaryTriggerByQueue := map[string]inventory.SQSTrigger{}
	for _, spec := range inventory.LambdaInventory.Lambdas {
		for _, trig := range spec.SQSTriggers {
			if trig.ConsumeDeadLetterQueue {
				continue
			}
			if existing, ok := primaryConsumerByQueue[trig.Queue]; ok && existing != spec.Name {
				panic(fmt.Sprintf("queue %s has multiple primary consumers: %s and %s", trig.Queue, existing, spec.Name))
			}
			primaryConsumerByQueue[trig.Queue] = spec.Name
			primaryTriggerByQueue[trig.Queue] = trig
		}
	}

	for _, lambda := range inventory.LambdaInventory.Lambdas {
		for _, trigger := range lambda.SQSTriggers {
			if _, exists := queuePairs[trigger.Queue]; exists {
				continue
			}

			logical := trigger.Queue
			primaryName := naming.ResourceName(logical, environment)

			queue := apptheorycdk.NewAppTheoryQueue(stack, _jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueLogical(logical))), &apptheorycdk.AppTheoryQueueProps{
				QueueName:              _jsii.String(primaryName),
				VisibilityTimeout:      defaultVisibility,
				RetentionPeriod:        defaultRetention,
				ReceiveMessageWaitTime: awscdk.Duration_Seconds(_jsii.Number(20)),
				EnableDlq:              _jsii.Bool(true),
				MaxReceiveCount:        defaultMaxReceive,
				DlqRetentionPeriod:     awscdk.Duration_Days(_jsii.Number(14)),
				RemovalPolicy:          awscdk.RemovalPolicy_DESTROY,
			})

			primaryQueue := queue.Queue()
			deadLetterQueue := queue.DeadLetterQueue()

			if primaryTrig, ok := primaryTriggerByQueue[logical]; ok {
				consumerName := primaryConsumerByQueue[logical]
				if consumerName == "" {
					panic(fmt.Sprintf("queue %s has trigger but no consumer mapping", logical))
				}
				consumer := functions.Must(consumerName)

				consumerProps := &apptheorycdk.AppTheoryQueueConsumerProps{
					Queue:                   primaryQueue,
					Consumer:                consumer,
					ReportBatchItemFailures: _jsii.Bool(primaryTrig.EnablePartialFailure),
				}
				if primaryTrig.BatchSize > 0 {
					consumerProps.BatchSize = _jsii.Number(float64(primaryTrig.BatchSize))
				}
				if primaryTrig.MaxBatchingWindowSeconds > 0 {
					consumerProps.MaxBatchingWindow = awscdk.Duration_Seconds(_jsii.Number(float64(primaryTrig.MaxBatchingWindowSeconds)))
				}
				apptheorycdk.NewAppTheoryQueueConsumer(stack, _jsii.String(fmt.Sprintf("%sConsumer", sanitizeQueueLogical(logical))), consumerProps)
			}

			queuePairs[logical] = QueuePair{Primary: primaryQueue, DLQ: deadLetterQueue}
		}
	}

	// Scheduled publishing queue is part of the canonical env-var contract (Spec 05) even when not used as an
	// inventory-declared event source mapping.
	if _, exists := queuePairs["scheduled-queue"]; !exists {
		logical := "scheduled-queue"
		primaryName := naming.ResourceName(logical, environment)

		queue := apptheorycdk.NewAppTheoryQueue(stack, _jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueLogical(logical))), &apptheorycdk.AppTheoryQueueProps{
			QueueName:              _jsii.String(primaryName),
			VisibilityTimeout:      defaultVisibility,
			RetentionPeriod:        defaultRetention,
			ReceiveMessageWaitTime: awscdk.Duration_Seconds(_jsii.Number(20)),
			EnableDlq:              _jsii.Bool(true),
			MaxReceiveCount:        defaultMaxReceive,
			DlqRetentionPeriod:     awscdk.Duration_Days(_jsii.Number(14)),
			RemovalPolicy:          awscdk.RemovalPolicy_DESTROY,
		})

		queuePairs[logical] = QueuePair{Primary: queue.Queue(), DLQ: queue.DeadLetterQueue()}
	}
	return queuePairs
}

func ensureModuleRoot(t *testing.T) string {
	t.Helper()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	moduleRoot := filepath.Clean(filepath.Join(originalWD, ".."))
	if err := os.Chdir(moduleRoot); err != nil {
		t.Fatalf("chdir to module root %s: %v", moduleRoot, err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	return moduleRoot
}

func ensureLambdaAssets(t *testing.T, moduleRoot string) {
	t.Helper()

	repoRoot := filepath.Clean(filepath.Join(moduleRoot, "..", ".."))
	binDir := filepath.Join(repoRoot, "bin")
	createdAssets := make([]string, 0, len(inventory.LambdaInventory.Lambdas))
	for _, spec := range inventory.LambdaInventory.Lambdas {
		assetPath := filepath.Join(binDir, spec.Name+".zip")
		if _, err := os.Stat(assetPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat asset %s: %v", assetPath, err)
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
}

func loadTemplate(t *testing.T, templatePath string) map[string]any {
	t.Helper()

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
