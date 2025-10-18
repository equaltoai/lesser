package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/jsii-runtime-go"
)

type LambdaFunctionsProps struct {
	Environment          string
	Table                awsdynamodb.Table
	MediaBucket          awss3.Bucket
	StreamingBucket      awss3.Bucket
	TrainingBucket       awss3.Bucket
	FederationQueue      awssqs.Queue
	FederationDLQ        awssqs.Queue
	PushQueue            awssqs.Queue
	PrivateKey           awssecretsmanager.ISecret
	CloudFrontPrivateKey awssecretsmanager.ISecret
	MediaConvertRoleArn  *string
	ModelMetadataTable   *string
	Config               map[string]interface{}
}

type LambdaFunctions struct {
	// API Functions
	APIFunction     awslambda.Function
	GraphQLFunction awslambda.Function

	// Federation Functions
	InboxFunction     awslambda.Function
	OutboxFunction    awslambda.Function
	WebfingerFunction awslambda.Function

	// Stream Processors
	ActivityProcessor     awslambda.Function
	NotificationProcessor awslambda.Function
	ModerationProcessor   awslambda.Function
	SeveranceProcessor    awslambda.Function

	// WebSocket Functions
	StreamingFunction    awslambda.Function
	StreamRouterFunction awslambda.Function

	// Specialized Processors
	AIProcessorFunction       awslambda.Function
	SearchIndexerFunction     awslambda.Function
	MediaProcessorFunction    awslambda.Function
	EmailProcessorFunction    awslambda.Function
	TimelineProcessorFunction awslambda.Function
	MLTrainingProcessor       awslambda.Function
	CleanupFunction           awslambda.Function
	ConfigureInstanceFunction awslambda.Function
	HealthFunction            awslambda.Function
	RecoveryFunction          awslambda.Function
}

func CreateLambdaFunctions(stack awscdk.Stack, props *LambdaFunctionsProps) *LambdaFunctions {
	functions := &LambdaFunctions{}

	// Create security constructs with comprehensive IAM policies (Phase 6.7)
	security := CreateSecurityConstructs(stack, &SecurityProps{
		Environment:     props.Environment,
		Table:           props.Table,
		MediaBucket:     props.MediaBucket,
		FederationQueue: props.FederationQueue,
		FederationDLQ:   props.FederationDLQ,
		PushQueue:       props.PushQueue,
	})

	// Helper to get config values
	getConfigString := func(key string, defaultVal string) *string {
		if props.Config != nil {
			if val, ok := props.Config[key].(string); ok {
				return jsii.String(val)
			}
		}
		return jsii.String(defaultVal)
	}

	// Common environment variables matching Pulumi config (lines 620-641)
	commonEnv := &map[string]*string{
		"DYNAMO_TABLE_NAME":     props.Table.TableName(),
		"S3_BUCKET_NAME":        props.MediaBucket.BucketName(),
		"FEDERATION_QUEUE_URL":  props.FederationQueue.QueueUrl(),
		"FEDERATION_DLQ_URL":    props.FederationDLQ.QueueUrl(),
		"PUSH_QUEUE_URL":        props.PushQueue.QueueUrl(),
		"DOMAIN":                jsii.String("REPLACE_WITH_DOMAIN"),                                          // Set by CDK context
		"JWT_SECRET_ARN":        jsii.String("arn:aws:secretsmanager:*:*:secret:lesser/jwt-secret-*"),        // Reference to auto-generated JWT secret in SharedStack
		"ACTOR_PRIVATE_KEY_ARN": jsii.String("arn:aws:secretsmanager:*:*:secret:lesser/actor-private-key-*"), // Reference to actor key in SharedStack
		"KMS_KEY_ID":            jsii.String("alias/lesser-encryption"),                                      // SharedStack KMS key
		"CDN_DOMAIN":            jsii.String("REPLACE_WITH_MEDIA_DOMAIN"),                                    // Set by CDK context
		"INSTANCE_TITLE":        jsii.String("Lesser Instance"),
		"INSTANCE_SHORT_DESC":   jsii.String("A personal ActivityPub server"),
		"INSTANCE_DESCRIPTION":  jsii.String("A lightweight, serverless ActivityPub implementation"),
		"INSTANCE_ADMIN_EMAIL":  jsii.String("REPLACE_WITH_ADMIN_EMAIL"), // Set by CDK context
		"REGISTRATIONS_OPEN":    jsii.String("false"),
		"APPROVAL_REQUIRED":     jsii.String("true"),
		"INVITES_ENABLED":       jsii.String("false"),
		"FEDERATION_ENABLED":    jsii.String("true"),

		// Media Streaming Configuration (Phase 2.2)
		"MEDIA_SOURCE_BUCKET_NAME":    props.MediaBucket.BucketName(),
		"MEDIA_STREAMING_BUCKET_NAME": props.StreamingBucket.BucketName(),
		"MEDIA_CONVERT_ENDPOINT":      getConfigString("mediaConvertEndpoint", ""),
		"MEDIA_CONVERT_ROLE_ARN":      props.MediaConvertRoleArn,
		"CLOUDFRONT_DOMAIN":           getConfigString("cloudfrontDomain", ""),
		"CLOUDFRONT_PRIVATE_KEY_PATH": props.CloudFrontPrivateKey.SecretArn(),
		"CLOUDFRONT_KEY_PAIR_ID":      getConfigString("cloudfrontKeyPairId", ""), // Set after manual upload
		"MANIFEST_TTL_HOURS":          getConfigString("manifestTTLHours", "24"),

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

	// Determine log retention based on environment
	var logRetention awslogs.RetentionDays
	if props.Environment == "production" {
		logRetention = awslogs.RetentionDays_ONE_MONTH
	} else {
		logRetention = awslogs.RetentionDays_ONE_WEEK
	}

	// Common Lambda configuration with security role
	commonProps := awslambda.FunctionProps{
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		MemorySize:   jsii.Number(props.Config["memorySize"].(float64)),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(props.Config["timeout"].(float64))),
		Environment:  commonEnv,
		Tracing:      awslambda.Tracing_ACTIVE,
		Role:         security.LambdaRole, // Use comprehensive security role
	}

	// Create Lambda functions - Lift-based implementation
	functions.APIFunction = createFunction(stack, "api", props.Environment, &commonProps, "../../bin/api.zip", logRetention)
	functions.GraphQLFunction = createFunction(stack, "graphql", props.Environment, &commonProps, "../../bin/graphql.zip", logRetention)

	// Create federation functions (Pulumi lines 668-691)
	functions.InboxFunction = createFunction(stack, "inbox", props.Environment, &commonProps, "../../bin/inbox.zip", logRetention)
	functions.OutboxFunction = createFunction(stack, "outbox", props.Environment, &commonProps, "../../bin/outbox.zip", logRetention)
	functions.WebfingerFunction = createFunction(stack, "webfinger", props.Environment, &commonProps, "../../bin/webfinger.zip", logRetention)

	// Create stream processors with higher memory and longer timeout (lines 700-792)
	streamProps := commonProps
	streamProps.MemorySize = jsii.Number(1024)
	streamProps.Timeout = awscdk.Duration_Minutes(jsii.Number(5))

	functions.ActivityProcessor = createFunction(stack, "activity-processor", props.Environment, &streamProps, "../../bin/activity-processor.zip", logRetention)
	functions.NotificationProcessor = createFunction(stack, "push-delivery", props.Environment, &commonProps, "../../bin/push-delivery.zip", logRetention)
	functions.ModerationProcessor = createFunction(stack, "moderation-processor", props.Environment, &commonProps, "../../bin/moderation-processor.zip", logRetention)

	// Severance Processor - handles federation severance detection (Phase 2.4)
	severanceProps := streamProps
	severanceProps.Timeout = awscdk.Duration_Seconds(jsii.Number(30))
	functions.SeveranceProcessor = createFunction(stack, "severance-processor", props.Environment, &severanceProps, "../../bin/severance-processor.zip", logRetention)

	// ML Training Processor - handles ML model training job lifecycle (Phase 2.3)
	mlTrainingProps := streamProps
	mlTrainingProps.Timeout = awscdk.Duration_Minutes(jsii.Number(15)) // Longer timeout for Bedrock polling
	functions.MLTrainingProcessor = createFunction(stack, "ml-training-processor", props.Environment, &mlTrainingProps, "../../bin/ml-training-processor.zip", logRetention)

	// Create WebSocket functions (Pulumi lines 945-954)
	functions.StreamingFunction = createFunction(stack, "streaming", props.Environment, &commonProps, "../../bin/streaming.zip", logRetention)
	functions.StreamRouterFunction = createFunction(stack, "stream-router", props.Environment, &commonProps, "../../bin/stream-router.zip", logRetention)

	// Create specialized processors matching Pulumi
	functions.AIProcessorFunction = createFunction(stack, "ai-processor", props.Environment, &streamProps, "../../bin/ai-processor.zip", logRetention)
	functions.SearchIndexerFunction = createFunction(stack, "status-indexer", props.Environment, &commonProps, "../../bin/status-indexer.zip", logRetention)
	functions.MediaProcessorFunction = createFunction(stack, "media-processor", props.Environment, &commonProps, "../../bin/media-processor.zip", logRetention)
	functions.EmailProcessorFunction = createFunction(stack, "federation-delivery", props.Environment, &streamProps, "../../bin/federation-delivery.zip", logRetention)
	functions.TimelineProcessorFunction = createFunction(stack, "trend-aggregator", props.Environment, &commonProps, "../../bin/trend-aggregator.zip", logRetention)
	functions.CleanupFunction = createFunction(stack, "cost-aggregator", props.Environment, &commonProps, "../../bin/cost-aggregator.zip", logRetention)
	functions.ConfigureInstanceFunction = createFunction(stack, "note-processor", props.Environment, &commonProps, "../../bin/note-processor.zip", logRetention)
	functions.HealthFunction = createFunction(stack, "federation-tracker", props.Environment, &commonProps, "../../bin/federation-tracker.zip", logRetention)
	functions.RecoveryFunction = createFunction(stack, "import-processor", props.Environment, &streamProps, "../../bin/import-processor.zip", logRetention)

	// Grant additional Secrets Manager permissions to federation functions
	props.PrivateKey.GrantRead(functions.InboxFunction, nil)
	props.PrivateKey.GrantRead(functions.OutboxFunction, nil)
	props.PrivateKey.GrantRead(functions.APIFunction, nil)

	return functions
}

func createFunction(stack awscdk.Stack, name string, environment string, props *awslambda.FunctionProps, codePath string, logRetention awslogs.RetentionDays) awslambda.Function {
	// Create log group with explicit retention (replaces deprecated logRetention)
	// Include environment in log group name for isolation between environments
	logGroup := awslogs.NewLogGroup(stack, jsii.String(name+"LogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/aws/lambda/lesser-%s-%s", environment, name)),
		Retention:     logRetention,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	funcProps := *props
	funcProps.FunctionName = jsii.String(fmt.Sprintf("lesser-%s-%s", environment, name)) // Include environment for isolation
	funcProps.Code = awslambda.Code_FromAsset(jsii.String(codePath), nil)
	funcProps.Handler = jsii.String("bootstrap")
	funcProps.LogGroup = logGroup // Use new logGroup property instead of deprecated logRetention

	return awslambda.NewFunction(stack, jsii.String(name+"Function"), &funcProps)
}
