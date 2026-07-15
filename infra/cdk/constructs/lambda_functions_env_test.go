package constructs

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	_jsii "github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func TestLambdaEnvironmentsIncludeBaselineAndInventoryVars(t *testing.T) {
	moduleRoot := ensureModuleRoot(t)
	ensureLambdaAssets(t, moduleRoot)

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
		Config: map[string]interface{}{
			"vapidPublicKey": "test-public-key",
			"vapidSubject":   "mailto:test@example.com",
			"vapidSecretArn": "arn:aws:secretsmanager:us-east-1:123456789012:secret:test",
			"lesserVersion":  "v1.5.20",
		},
		EncryptionRole: encRole,
		BasicRole:      basicRole,
	})
	queues := buildInventoryQueues(stack, functions, environment)
	ApplyQueueEnvironmentVariables(functions, queues)

	app.Synth(nil)

	tpl := loadTemplate(t, filepath.Join(outdir, "TestStack.template.json"))
	envByName := collectLambdaEnvironments(t, tpl)

	baselineKeys := []string{
		"ENVIRONMENT", "STAGE", "APP_NAME",
		"VERSION",
		"DOMAIN_NAME",
		"DYNAMODB_TABLE",
		"RATE_LIMIT_TABLE_NAME", "LIMITED_TABLE_NAME",
		"CONNECTIONS_TABLE", "SUBSCRIPTIONS_TABLE",
		"STREAM_EVENTS_TABLE_NAME",
		"S3_BUCKET_NAME",
		"PRIVATE_KEY_SECRET", "JWT_SECRET_ARN", "KMS_KEY_ID",
		"WEBSOCKET_ENDPOINT",
		"GRAPHQL_WEBSOCKET_ENDPOINT",
		"IMPORT_QUEUE_URL", "EXPORT_QUEUE_URL", "MEDIA_QUEUE_URL",
		"SCHEDULED_QUEUE_URL", "FEDERATION_DELIVERY_QUEUE_URL", "PUSH_NOTIFICATION_QUEUE_URL",
	}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		fnName := naming.ResourceName(spec.Name, environment)
		env, ok := envByName[fnName]
		if !ok {
			t.Fatalf("environment not found for %s", fnName)
		}

		for _, key := range baselineKeys {
			val, present := env[key]
			if !present || val == "" {
				t.Fatalf("baseline env var %s missing for %s", key, fnName)
			}
		}

		for _, key := range spec.RequiredEnvVars {
			val, present := env[key]
			if !present || val == "" {
				t.Fatalf("required env var %s missing for %s", key, fnName)
			}
		}
	}

	federationAggregatorName := naming.ResourceName("federation-aggregator", environment)
	federationAggregatorEnv, ok := envByName[federationAggregatorName]
	if !ok {
		t.Fatalf("environment not found for %s", federationAggregatorName)
	}
	for _, key := range []string{"APP_NAME", "STAGE", "DYNAMODB_TABLE", "FEDERATION_AGGREGATOR_QUEUE_URL"} {
		if val := federationAggregatorEnv[key]; val == "" {
			t.Fatalf("federation aggregator env var %s missing", key)
		}
	}
	if _, present := federationAggregatorEnv["AWS_ACCOUNT_ID"]; present {
		t.Fatalf("federation aggregator should not depend on AWS_ACCOUNT_ID in the synthesized env")
	}
}

func collectLambdaEnvironments(t *testing.T, tpl map[string]any) map[string]map[string]string {
	t.Helper()

	resources, ok := tpl["Resources"].(map[string]any)
	if !ok {
		t.Fatalf("template resources missing or wrong type")
	}
	result := make(map[string]map[string]string)
	for _, raw := range resources {
		res, ok := raw.(map[string]any)
		if !ok || res["Type"] != "AWS::Lambda::Function" {
			continue
		}
		props, ok := res["Properties"].(map[string]any)
		if !ok {
			continue
		}
		fnName, _ := props["FunctionName"].(string)
		if fnName == "" {
			continue
		}
		env := make(map[string]string)
		if envBlock, ok := props["Environment"].(map[string]any); ok {
			if vars, ok := envBlock["Variables"].(map[string]any); ok {
				for k, v := range vars {
					if s := stringifyCFNValue(v); s != "" {
						env[k] = s
					}
				}
			}
		}
		result[fnName] = env
	}
	return result
}

func stringifyCFNValue(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case *string:
		if typed != nil {
			return *typed
		}
	case map[string]any, []any:
		if b, err := json.Marshal(typed); err == nil && len(b) > 0 {
			return string(b)
		}
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
