package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/jsii-runtime-go"
)

type LambdaFunctionsProps struct {
	Environment         string
	Table               awsdynamodb.Table
	RateLimitTable      awsdynamodb.Table
	MediaBucket         awss3.Bucket
	StreamingBucket     awss3.Bucket
	TrainingBucket      awss3.Bucket
	FederationQueue     awssqs.Queue
	FederationDLQ       awssqs.Queue
	PushQueue           awssqs.Queue
	PrivateKey          awssecretsmanager.ISecret
	JwtSecret           awssecretsmanager.ISecret
	MediaConvertRoleArn *string
	ModelMetadataTable  *string
	Config              map[string]interface{}
	EncryptionRole      awsiam.IRole
	BasicRole           awsiam.IRole
}

type LambdaFunctions struct {
	// API Functions
	APIFunction       awslambda.Function
	GraphQLFunction   awslambda.Function
	GraphQLWSFunction awslambda.Function

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

	// Note: Policies are attached to roles in SharedStack, not here
	// Roles are imported fully configured from SharedStack

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

	// Common environment variables matching Pulumi config (lines 620-641)
	commonEnv := &map[string]*string{
		"ENVIRONMENT":                 jsii.String(props.Environment),
		"DYNAMO_TABLE_NAME":           props.Table.TableName(),
		"RATE_LIMIT_TABLE_NAME":       props.RateLimitTable.TableName(),
		"LIMITED_TABLE_NAME":          props.RateLimitTable.TableName(), // For limited library
		"CONNECTIONS_TABLE":           props.Table.TableName(),
		"SUBSCRIPTIONS_TABLE":         props.Table.TableName(),
		"S3_BUCKET_NAME":              props.MediaBucket.BucketName(),
		"FEDERATION_QUEUE_URL":        props.FederationQueue.QueueUrl(),
		"FEDERATION_DLQ_URL":          props.FederationDLQ.QueueUrl(),
		"PUSH_QUEUE_URL":              props.PushQueue.QueueUrl(),
		"PUSH_NOTIFICATION_QUEUE_URL": props.PushQueue.QueueUrl(),
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
	(*commonEnv)["WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("https://%s", graphqlWsHost))
	(*commonEnv)["WEBSOCKET_API_URL"] = jsii.String(fmt.Sprintf("https://%s", graphqlWsHost))
	(*commonEnv)["GRAPHQL_WS_URL"] = jsii.String(fmt.Sprintf("wss://%s", graphqlWsHost))
	(*commonEnv)["STREAM_WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("wss://%s", streamWsHost))
	(*commonEnv)["STREAM_WEBSOCKET_API_URL"] = jsii.String(fmt.Sprintf("https://%s", streamWsHost))

	// Set JWT secret ARN from SharedStack (securely passed, never synthesized)
	if props.JwtSecret != nil {
		(*commonEnv)["JWT_SECRET_ARN"] = props.JwtSecret.SecretArn()
	}

	// Determine log retention based on environment
	var logRetention awslogs.RetentionDays
	if props.Environment == "production" {
		logRetention = awslogs.RetentionDays_ONE_MONTH
	} else {
		logRetention = awslogs.RetentionDays_ONE_WEEK
	}

	// Common Lambda configuration (role assigned per function)
	commonProps := awslambda.FunctionProps{
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		MemorySize:   jsii.Number(props.Config["memorySize"].(float64)),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(props.Config["timeout"].(float64))),
		Environment:  commonEnv,
		Tracing:      awslambda.Tracing_ACTIVE,
	}

	// Create Lambda functions - Lift-based implementation
	// Functions needing encryption (use EncryptionRole)
	functions.APIFunction = createFunction(stack, "api", props.Environment, &commonProps, "../../bin/api.zip", logRetention, props.EncryptionRole)
	functions.InboxFunction = createFunction(stack, "inbox", props.Environment, &commonProps, "../../bin/inbox.zip", logRetention, props.EncryptionRole)
	functions.OutboxFunction = createFunction(stack, "outbox", props.Environment, &commonProps, "../../bin/outbox.zip", logRetention, props.EncryptionRole)

	// GraphQL function needs encryption role for JWT secret access
	functions.GraphQLFunction = createFunction(stack, "graphql", props.Environment, &commonProps, "../../bin/graphql.zip", logRetention, props.EncryptionRole)
	
	// GraphQL WebSocket function needs encryption role for JWT secret access (OAuth token validation)
	functions.GraphQLWSFunction = createFunction(stack, "graphql-ws", props.Environment, &commonProps, "../../bin/graphql-ws.zip", logRetention, props.EncryptionRole)
	
	// All other functions use BasicRole

	// Create federation functions (Pulumi lines 668-691)
	functions.WebfingerFunction = createFunction(stack, "webfinger", props.Environment, &commonProps, "../../bin/webfinger.zip", logRetention, props.BasicRole)

	// Create stream processors with higher memory and longer timeout (lines 700-792)
	streamProps := commonProps
	streamProps.MemorySize = jsii.Number(1024)
	streamProps.Timeout = awscdk.Duration_Minutes(jsii.Number(5))

	functions.ActivityProcessor = createFunction(stack, "activity-processor", props.Environment, &streamProps, "../../bin/activity-processor.zip", logRetention, props.BasicRole)
	functions.NotificationProcessor = createFunction(stack, "push-delivery", props.Environment, &commonProps, "../../bin/push-delivery.zip", logRetention, props.BasicRole)
	functions.ModerationProcessor = createFunction(stack, "moderation-processor", props.Environment, &commonProps, "../../bin/moderation-processor.zip", logRetention, props.BasicRole)

	// Severance Processor - handles federation severance detection (Phase 2.4)
	severanceProps := streamProps
	severanceProps.Timeout = awscdk.Duration_Seconds(jsii.Number(30))
	functions.SeveranceProcessor = createFunction(stack, "severance-processor", props.Environment, &severanceProps, "../../bin/severance-processor.zip", logRetention, props.BasicRole)

	// ML Training Processor - handles ML model training job lifecycle (Phase 2.3)
	mlTrainingProps := streamProps
	mlTrainingProps.Timeout = awscdk.Duration_Minutes(jsii.Number(15)) // Longer timeout for Bedrock polling
	functions.MLTrainingProcessor = createFunction(stack, "ml-training-processor", props.Environment, &mlTrainingProps, "../../bin/ml-training-processor.zip", logRetention, props.BasicRole)

	// Create WebSocket functions (Pulumi lines 945-954)
	// Both streaming lambdas read the JWT secret from Secrets Manager, so they need the encryption role for KMS decrypt.
	functions.StreamingFunction = createFunction(stack, "streaming", props.Environment, &commonProps, "../../bin/streaming.zip", logRetention, props.EncryptionRole)
	functions.StreamRouterFunction = createFunction(stack, "stream-router", props.Environment, &commonProps, "../../bin/stream-router.zip", logRetention, props.EncryptionRole)

	// Create specialized processors matching Pulumi
	functions.AIProcessorFunction = createFunction(stack, "ai-processor", props.Environment, &streamProps, "../../bin/ai-processor.zip", logRetention, props.BasicRole)
	functions.SearchIndexerFunction = createFunction(stack, "status-indexer", props.Environment, &commonProps, "../../bin/status-indexer.zip", logRetention, props.BasicRole)
	functions.MediaProcessorFunction = createFunction(stack, "media-processor", props.Environment, &commonProps, "../../bin/media-processor.zip", logRetention, props.BasicRole)
	functions.EmailProcessorFunction = createFunction(stack, "federation-delivery", props.Environment, &streamProps, "../../bin/federation-delivery.zip", logRetention, props.BasicRole)
	functions.TimelineProcessorFunction = createFunction(stack, "trend-aggregator", props.Environment, &commonProps, "../../bin/trend-aggregator.zip", logRetention, props.BasicRole)
	functions.CleanupFunction = createFunction(stack, "cost-aggregator", props.Environment, &commonProps, "../../bin/cost-aggregator.zip", logRetention, props.BasicRole)
	functions.ConfigureInstanceFunction = createFunction(stack, "note-processor", props.Environment, &commonProps, "../../bin/note-processor.zip", logRetention, props.BasicRole)
	functions.HealthFunction = createFunction(stack, "federation-tracker", props.Environment, &commonProps, "../../bin/federation-tracker.zip", logRetention, props.BasicRole)
	functions.RecoveryFunction = createFunction(stack, "import-processor", props.Environment, &streamProps, "../../bin/import-processor.zip", logRetention, props.BasicRole)

	// Note: Secrets Manager permissions are granted via the security role (security.go)
	// We don't use GrantRead() here to avoid circular dependencies between SharedStack and LesserApiStack
	// The security constructs already grant secretsmanager:GetSecretValue for:
	// - lesser/jwt-secret and lesser/jwt-secret-*
	// - lesser/actor-private-key and lesser/actor-private-key-*
	// - lesser/cdn-private-key and lesser/cdn-private-key-*

	return functions
}

func createFunction(stack awscdk.Stack, name string, environment string, props *awslambda.FunctionProps, codePath string, logRetention awslogs.RetentionDays, role awsiam.IRole) awslambda.Function {
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
	funcProps.Role = role

	return awslambda.NewFunction(stack, jsii.String(name+"Function"), &funcProps)
}
