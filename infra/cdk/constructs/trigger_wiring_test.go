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
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	_jsii "github.com/aws/jsii-runtime-go"
)

type eventSourceMapping struct {
	FunctionName  string
	SourceLogical string
	SourceAttr    string
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

	queues := buildInventoryQueues(stack, "dev")

	functions := CreateLambdaFunctions(stack, &LambdaFunctionsProps{
		Environment:         "dev",
		Table:               mainTable,
		RateLimitTable:      rateTable,
		MediaBucket:         mediaBucket,
		StreamingBucket:     streamingBucket,
		TrainingBucket:      trainingBucket,
		Queues:              queues,
		PrivateKey:          privateKey,
		JwtSecret:           jwtSecret,
		MediaConvertRoleArn: _jsii.String("arn:aws:iam::123456789012:role/media-convert"),
		ModelMetadataTable:  _jsii.String("model-metadata"),
		Config:              map[string]interface{}{},
		EncryptionRole:      encRole,
		BasicRole:           basicRole,
	})

	CreateStreamProcessors(stack, &StreamProcessorsProps{
		Table:     mainTable,
		Queues:    queues,
		Functions: functions,
	})
	CreateScheduleWiring(stack, &ScheduleWiringProps{
		Functions:   functions,
		Environment: "dev",
	})

	app.Synth(nil)

	tpl := loadTemplate(t, filepath.Join(outdir, "TestStack.template.json"))
	mappings := collectEventSourceMappings(t, tpl)
	queuesMeta := collectQueuesByName(t, tpl)
	rules := collectScheduleRules(t, tpl)

	for _, spec := range inventory.LambdaInventory.Lambdas {
		fnName := fmt.Sprintf("lesser-%s-%s", "dev", spec.Name)

		if len(spec.StreamTriggers) > 0 {
			if countMappingsBySourceAttr(mappings, fnName, "StreamArn") != len(spec.StreamTriggers) {
				t.Fatalf("stream trigger mapping mismatch for %s", spec.Name)
			}
		}

		for _, trig := range spec.SQSTriggers {
			primaryName := fmt.Sprintf("lesser-%s-%s", trig.Queue, "dev")

			dlqLogical := trig.DeadLetterQueue
			if dlqLogical == "" {
				dlqLogical = fmt.Sprintf("%s-dlq", trig.Queue)
			}

			dlqName := fmt.Sprintf("lesser-%s-%s", dlqLogical, "dev")
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
			if !ruleExists(rules, trig.Expression, fnName) {
				t.Fatalf("missing schedule rule for %s with expression %s", spec.Name, trig.Expression)
			}
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
		Environment:         "dev",
		Table:               mainTable,
		RateLimitTable:      rateTable,
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
		Table:     mainTable,
		Queues:    map[string]QueuePair{},
		Functions: functions,
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

	queues := buildInventoryQueues(stack, "dev")
	functions := CreateLambdaFunctions(stack, &LambdaFunctionsProps{
		Environment:         "dev",
		Table:               mainTable,
		RateLimitTable:      rateTable,
		MediaBucket:         mediaBucket,
		StreamingBucket:     streamingBucket,
		TrainingBucket:      trainingBucket,
		Queues:              queues,
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
			t.Fatalf("expected panic for schedule trigger on unsupported lambda type")
		}
	}()
	CreateScheduleWiring(stack, &ScheduleWiringProps{
		Functions:   functions,
		Environment: "dev",
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
			FunctionName:  fnName,
			SourceLogical: srcLogical,
			SourceAttr:    srcAttr,
		})
	}
	return mappings
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

func buildInventoryQueues(stack awscdk.Stack, environment string) map[string]QueuePair {
	queuePairs := map[string]QueuePair{}
	defaultVisibility := awscdk.Duration_Minutes(_jsii.Number(2))
	defaultRetention := awscdk.Duration_Days(_jsii.Number(4))
	defaultMaxReceive := _jsii.Number(5)

	for _, lambda := range inventory.LambdaInventory.Lambdas {
		for _, trigger := range lambda.SQSTriggers {
			if _, exists := queuePairs[trigger.Queue]; exists {
				continue
			}

			logical := trigger.Queue
			primaryName := fmt.Sprintf("lesser-%s-%s", logical, environment)

			dlqLogical := trigger.DeadLetterQueue
			if dlqLogical == "" {
				dlqLogical = fmt.Sprintf("%s-dlq", logical)
			}
			dlqName := fmt.Sprintf("lesser-%s-%s", dlqLogical, environment)

			dlq := awssqs.NewQueue(stack, _jsii.String(fmt.Sprintf("%sDlq", sanitizeQueueLogical(logical))), &awssqs.QueueProps{
				QueueName:       _jsii.String(dlqName),
				RetentionPeriod: awscdk.Duration_Days(_jsii.Number(14)),
			})

			queue := awssqs.NewQueue(stack, _jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueLogical(logical))), &awssqs.QueueProps{
				QueueName:              _jsii.String(primaryName),
				ReceiveMessageWaitTime: awscdk.Duration_Seconds(_jsii.Number(20)),
				VisibilityTimeout:      defaultVisibility,
				RetentionPeriod:        defaultRetention,
				DeadLetterQueue: &awssqs.DeadLetterQueue{
					MaxReceiveCount: defaultMaxReceive,
					Queue:           dlq,
				},
			})

			queuePairs[logical] = QueuePair{Primary: queue, DLQ: dlq}
		}
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
