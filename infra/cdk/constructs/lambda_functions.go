package constructs

import (
	"fmt"
	"strings"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/jsii-runtime-go"
)

type LambdaFunctionsProps struct {
	Environment         string
	Table               awsdynamodb.Table
	RateLimitTable      awsdynamodb.Table
	MediaBucket         awss3.Bucket
	StreamingBucket     awss3.Bucket
	TrainingBucket      awss3.Bucket
	Queues              map[string]QueuePair
	PrivateKey          awssecretsmanager.ISecret
	JwtSecret           awssecretsmanager.ISecret
	MediaConvertRoleArn *string
	ModelMetadataTable  *string
	Config              map[string]interface{}
	EncryptionRole      awsiam.IRole
	BasicRole           awsiam.IRole
}

type LambdaFunctions struct {
	Functions map[string]awslambda.Function
}

// Must returns the named function or panics if missing.
func (l *LambdaFunctions) Must(name string) awslambda.Function {
	if fn, ok := l.Functions[name]; ok {
		return fn
	}
	panic(fmt.Sprintf("lambda %s not found", name))
}

func CreateLambdaFunctions(stack awscdk.Stack, props *LambdaFunctionsProps) *LambdaFunctions {
	functions := &LambdaFunctions{Functions: make(map[string]awslambda.Function)}

	// Helper to get config values
	getConfigString := func(key string, defaultVal string) *string {
		if props.Config != nil {
			if val, ok := props.Config[key].(string); ok {
				return jsii.String(val)
			}
		}
		return jsii.String(defaultVal)
	}

	domainValue := "lesser.host"
	if domainPtr := getConfigString("domain", "lesser.host"); domainPtr != nil && *domainPtr != "" {
		domainValue = *domainPtr
	}

	// Common environment variables shared across functions
	commonEnv := map[string]*string{
		"ENVIRONMENT":                 jsii.String(props.Environment),
		"DYNAMO_TABLE_NAME":           props.Table.TableName(),
		"RATE_LIMIT_TABLE_NAME":       props.RateLimitTable.TableName(),
		"LIMITED_TABLE_NAME":          props.RateLimitTable.TableName(), // For limited library
		"CONNECTIONS_TABLE":           props.Table.TableName(),
		"SUBSCRIPTIONS_TABLE":         props.Table.TableName(),
		"S3_BUCKET_NAME":              props.MediaBucket.BucketName(),
		"FEDERATION_QUEUE_URL":        queueURL(props.Queues, "federation-delivery-queue"),
		"FEDERATION_DLQ_URL":          dlqURL(props.Queues, "federation-delivery-queue"),
		"PUSH_QUEUE_URL":              queueURL(props.Queues, "push-delivery-queue"),
		"PUSH_NOTIFICATION_QUEUE_URL": queueURL(props.Queues, "push-delivery-queue"),
		"DOMAIN":                      jsii.String(domainValue),
		"ACTOR_PRIVATE_KEY_ARN":       props.PrivateKey.SecretArn(),             // Reference to actor key in SharedStack
		"KMS_KEY_ID":                  jsii.String("alias/lesser-encryption"),   // KMS key for encrypting actor private keys
		"CDN_DOMAIN":                  jsii.String("REPLACE_WITH_MEDIA_DOMAIN"), // Set by CDK context
		"INSTANCE_TITLE":              jsii.String("Lesser Instance"),
		"INSTANCE_SHORT_DESC":         jsii.String("A personal ActivityPub server"),
		"INSTANCE_DESCRIPTION":        jsii.String("A lightweight, serverless ActivityPub implementation"),
		"INSTANCE_ADMIN_EMAIL":        jsii.String("REPLACE_WITH_ADMIN_EMAIL"), // Set by CDK context
		"REGISTRATIONS_OPEN":          jsii.String("false"),
		"APPROVAL_REQUIRED":           jsii.String("true"),
		"INVITES_ENABLED":             jsii.String("false"),
		"FEDERATION_ENABLED":          jsii.String("true"),

		// Media Streaming Configuration (Phase 2.2)
		"MEDIA_SOURCE_BUCKET_NAME":    props.MediaBucket.BucketName(),
		"MEDIA_STREAMING_BUCKET_NAME": props.StreamingBucket.BucketName(),
		"MEDIA_CONVERT_ENDPOINT":      getConfigString("mediaConvertEndpoint", ""),
		"MEDIA_CONVERT_ROLE_ARN":      props.MediaConvertRoleArn,
		"CLOUDFRONT_DOMAIN":           getConfigString("cloudfrontDomain", ""),
		"CLOUDFRONT_PRIVATE_KEY_PATH": getConfigString("cloudfrontPrivateKeySecret", ""),
		"CLOUDFRONT_KEY_PAIR_ID":      getConfigString("cloudfrontKeyPairId", ""), // Set after manual upload
		"MANIFEST_TTL_HOURS":          getConfigString("manifestTTLHours", "24"),
		"VAPID_PUBLIC_KEY":            getConfigString("vapidPublicKey", ""),
		"VAPID_SUBJECT":               getConfigString("vapidSubject", ""),
		"VAPID_SECRET_ARN":            getConfigString("vapidSecretArn", ""),

		// ML Moderation Configuration (Phase 2.3)
		"MODERATION_TRAINING_BUCKET_NAME": props.TrainingBucket.BucketName(),
		"MODERATION_MODEL_METADATA_TABLE": props.ModelMetadataTable,
		"BEDROCK_TRAINING_REGION":         getConfigString("bedrockRegion", "us-east-1"),
		"BEDROCK_INFERENCE_MODEL_ID":      getConfigString("bedrockInferenceModelId", ""),
		"BEDROCK_CUSTOMIZATION_ROLE_ARN":  getConfigString("bedrockCustomizationRoleArn", ""),
		"BEDROCK_GUARDRAIL_ID":            getConfigString("bedrockGuardrailId", ""),
		"BEDROCK_GUARDRAIL_VERSION":       getConfigString("bedrockGuardrailVersion", "DRAFT"),
		"MODERATION_ML_ENABLED":           getConfigString("moderationMLEnabled", "false"),
		"MODERATION_ML_TENANTS":           getConfigString("moderationMLTenants", ""),
	}

	// WebSocket endpoints (GraphQL subscriptions + streaming)
	graphqlWsHost := fmt.Sprintf("graphql-ws.%s", domainValue)
	streamWsHost := fmt.Sprintf("stream.%s", domainValue)
	commonEnv["WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("https://%s", graphqlWsHost))
	commonEnv["WEBSOCKET_API_URL"] = jsii.String(fmt.Sprintf("https://%s", graphqlWsHost))
	commonEnv["GRAPHQL_WS_URL"] = jsii.String(fmt.Sprintf("wss://%s", graphqlWsHost))
	commonEnv["STREAM_WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("wss://%s", streamWsHost))
	commonEnv["STREAM_WEBSOCKET_API_URL"] = jsii.String(fmt.Sprintf("https://%s", streamWsHost))

	// Set JWT secret ARN from SharedStack (securely passed, never synthesized)
	if props.JwtSecret != nil {
		commonEnv["JWT_SECRET_ARN"] = props.JwtSecret.SecretArn()
	}

	// Select baseline defaults by environment (Lift-aligned)
	defaults := inventory.LambdaInventory.Defaults
	if props.Environment == "production" || props.Environment == "prod" {
		defaults = inventory.ProductionDefaults
	}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		memory := defaults.MemoryMB
		if spec.Overrides.MemoryMB != nil {
			memory = *spec.Overrides.MemoryMB
		}
		timeout := defaults.TimeoutSeconds
		if spec.Overrides.TimeoutSeconds != nil {
			timeout = *spec.Overrides.TimeoutSeconds
		}
		retentionDays := defaults.LogRetentionDays
		if spec.Overrides.LogRetentionDays != nil {
			retentionDays = *spec.Overrides.LogRetentionDays
		}

		retention := awslogs.RetentionDays_ONE_WEEK
		if retentionDays >= 30 {
			retention = awslogs.RetentionDays_ONE_MONTH
		}

		role := props.BasicRole
		if spec.Role == inventory.RoleClassEncryption {
			role = props.EncryptionRole
		}

		logGroup := awslogs.NewLogGroup(stack, jsii.String(spec.Name+"LogGroup"), &awslogs.LogGroupProps{
			LogGroupName:  jsii.String(fmt.Sprintf("/aws/lambda/lesser-%s-%s", props.Environment, spec.Name)),
			Retention:     retention,
			RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		})

		// Clone and extend env per-Lambda with queue URLs derived from inventory
		env := copyEnv(commonEnv)
		for _, trig := range spec.SQSTriggers {
			envKey := fmt.Sprintf("%s_QUEUE_URL", strings.ToUpper(strings.ReplaceAll(trig.Queue, "-", "_")))
			if trig.ConsumeDeadLetterQueue {
				env[envKey] = dlqURL(props.Queues, trig.Queue)
			} else {
				env[envKey] = queueURL(props.Queues, trig.Queue)
			}
			if trig.DeadLetterQueue != "" {
				dlqKey := fmt.Sprintf("%s_DLQ_URL", strings.ToUpper(strings.ReplaceAll(trig.DeadLetterQueue, "-", "_")))
				env[dlqKey] = dlqURL(props.Queues, trig.Queue)
			}
		}
		for _, key := range spec.RequiredEnvVars {
			if _, exists := env[key]; exists {
				continue
			}
			switch key {
			case "EXPORT_PROCESSOR_QUEUE_URL":
				env[key] = queueURL(props.Queues, "export-processor-queue")
			case "IMPORT_PROCESSOR_QUEUE_URL":
				env[key] = queueURL(props.Queues, "import-processor-queue")
			case "MEDIA_PROCESSOR_QUEUE_URL":
				env[key] = queueURL(props.Queues, "media-processor-queue")
			default:
				// leave untouched for non-queue vars
			}
		}

		fnProps := awslambda.FunctionProps{
			Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
			Architecture: awslambda.Architecture_ARM_64(),
			MemorySize:   jsii.Number(memory),
			Timeout:      awscdk.Duration_Seconds(jsii.Number(timeout)),
			Environment:  &env,
			Tracing:      awslambda.Tracing_ACTIVE,
			FunctionName: jsii.String(fmt.Sprintf("lesser-%s-%s", props.Environment, spec.Name)),
			Code:         awslambda.Code_FromAsset(jsii.String(fmt.Sprintf("../../bin/%s.zip", spec.Name)), nil),
			Handler:      jsii.String("bootstrap"),
			LogGroup:     logGroup,
			Role:         role,
		}

		functions.Functions[spec.Name] = awslambda.NewFunction(stack, jsii.String(spec.Name+"Function"), &fnProps)
	}

	return functions
}

func copyEnv(src map[string]*string) map[string]*string {
	dst := make(map[string]*string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func queueURL(queueMap map[string]QueuePair, logical string) *string {
	if qp, ok := queueMap[logical]; ok && qp.Primary != nil {
		return qp.Primary.QueueUrl()
	}
	return jsii.String("")
}

func dlqURL(queueMap map[string]QueuePair, logical string) *string {
	if qp, ok := queueMap[logical]; ok && qp.DLQ != nil {
		return qp.DLQ.QueueUrl()
	}
	return jsii.String("")
}
