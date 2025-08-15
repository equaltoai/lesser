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
	Environment     string
	Table           awsdynamodb.Table
	MediaBucket     awss3.Bucket
	FederationQueue awssqs.Queue
	FederationDLQ   awssqs.Queue
	PushQueue       awssqs.Queue
	PrivateKey      awssecretsmanager.ISecret
	Config          map[string]interface{}
}

type LambdaFunctions struct {
	// API Functions
	APIFunction      awslambda.Function
	GraphQLFunction  awslambda.Function
	
	// Federation Functions
	InboxFunction     awslambda.Function
	OutboxFunction    awslambda.Function
	WebfingerFunction awslambda.Function
	
	// Stream Processors
	ActivityProcessor     awslambda.Function
	NotificationProcessor awslambda.Function
	ModerationProcessor   awslambda.Function
	
	// WebSocket Functions
	StreamingFunction    awslambda.Function
	StreamRouterFunction awslambda.Function
	
	// Specialized Processors
	AIProcessorFunction        awslambda.Function
	SearchIndexerFunction      awslambda.Function
	MediaProcessorFunction     awslambda.Function
	EmailProcessorFunction     awslambda.Function
	TimelineProcessorFunction  awslambda.Function
	CleanupFunction            awslambda.Function
	ConfigureInstanceFunction  awslambda.Function
	HealthFunction             awslambda.Function
	RecoveryFunction           awslambda.Function
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
	
	// Common environment variables matching Pulumi config (lines 620-641)
	commonEnv := &map[string]*string{
		"DYNAMO_TABLE_NAME":      props.Table.TableName(),
		"S3_BUCKET_NAME":         props.MediaBucket.BucketName(),
		"FEDERATION_QUEUE_URL":   props.FederationQueue.QueueUrl(),
		"FEDERATION_DLQ_URL":     props.FederationDLQ.QueueUrl(),
		"PUSH_QUEUE_URL":         props.PushQueue.QueueUrl(),
		"DOMAIN":                 jsii.String("REPLACE_WITH_DOMAIN"),  // Set by CDK context
		"JWT_SECRET":             jsii.String("REPLACE_WITH_JWT_SECRET"), // Set by CDK context  
		"KMS_KEY_ID":             jsii.String("alias/lesser-encryption"), // SharedStack KMS key
		"CDN_DOMAIN":             jsii.String("REPLACE_WITH_MEDIA_DOMAIN"), // Set by CDK context
		"INSTANCE_TITLE":         jsii.String("Lesser Instance"),
		"INSTANCE_SHORT_DESC":    jsii.String("A personal ActivityPub server"),
		"INSTANCE_DESCRIPTION":   jsii.String("A lightweight, serverless ActivityPub implementation"),
		"INSTANCE_ADMIN_EMAIL":   jsii.String("REPLACE_WITH_ADMIN_EMAIL"), // Set by CDK context
		"REGISTRATIONS_OPEN":     jsii.String("false"),
		"APPROVAL_REQUIRED":      jsii.String("true"),
		"INVITES_ENABLED":        jsii.String("false"),
		"FEDERATION_ENABLED":     jsii.String("true"),
	}
	
	// Common Lambda configuration with security role
	commonProps := awslambda.FunctionProps{
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		MemorySize:   jsii.Number(props.Config["memorySize"].(float64)),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(props.Config["timeout"].(float64))),
		Environment:  commonEnv,
		LogRetention: awslogs.RetentionDays_ONE_WEEK,
		Tracing:      awslambda.Tracing_ACTIVE,
		Role:         security.LambdaRole, // Use comprehensive security role
	}
	
	// Create Lambda functions - Lift-based implementation
	functions.APIFunction = createFunction(stack, "api", &commonProps, "../../bin/api.zip")
	functions.GraphQLFunction = createFunction(stack, "graphql", &commonProps, "../../bin/graphql.zip")
	
	// Create federation functions (Pulumi lines 668-691)
	functions.InboxFunction = createFunction(stack, "inbox", &commonProps, "../../bin/inbox.zip")
	functions.OutboxFunction = createFunction(stack, "outbox", &commonProps, "../../bin/outbox.zip") 
	functions.WebfingerFunction = createFunction(stack, "webfinger", &commonProps, "../../bin/webfinger.zip")
	
	// Create stream processors with higher memory and longer timeout (lines 700-792)
	streamProps := commonProps
	streamProps.MemorySize = jsii.Number(1024)
	streamProps.Timeout = awscdk.Duration_Minutes(jsii.Number(5))
	
	functions.ActivityProcessor = createFunction(stack, "activity-processor", &streamProps, "../../bin/activity-processor.zip")
	functions.NotificationProcessor = createFunction(stack, "push-delivery", &commonProps, "../../bin/push-delivery.zip")
	functions.ModerationProcessor = createFunction(stack, "moderation-processor", &commonProps, "../../bin/moderation-processor.zip")
	
	// Create WebSocket functions (Pulumi lines 945-954)
	functions.StreamingFunction = createFunction(stack, "streaming", &commonProps, "../../bin/streaming.zip")
	functions.StreamRouterFunction = createFunction(stack, "stream-router", &commonProps, "../../bin/stream-router.zip")
	
	// Create specialized processors matching Pulumi
	functions.AIProcessorFunction = createFunction(stack, "ai-processor", &streamProps, "../../bin/ai-processor.zip")
	functions.SearchIndexerFunction = createFunction(stack, "status-indexer", &commonProps, "../../bin/status-indexer.zip")
	functions.MediaProcessorFunction = createFunction(stack, "media-processor", &commonProps, "../../bin/media-processor.zip")
	functions.EmailProcessorFunction = createFunction(stack, "federation-delivery", &streamProps, "../../bin/federation-delivery.zip")
	functions.TimelineProcessorFunction = createFunction(stack, "trend-aggregator", &commonProps, "../../bin/trend-aggregator.zip")
	functions.CleanupFunction = createFunction(stack, "cost-aggregator", &commonProps, "../../bin/cost-aggregator.zip")
	functions.ConfigureInstanceFunction = createFunction(stack, "note-processor", &commonProps, "../../bin/note-processor.zip")
	functions.HealthFunction = createFunction(stack, "federation-tracker", &commonProps, "../../bin/federation-tracker.zip")
	functions.RecoveryFunction = createFunction(stack, "import-processor", &streamProps, "../../bin/import-processor.zip")
	
	// Grant additional Secrets Manager permissions to federation functions
	props.PrivateKey.GrantRead(functions.InboxFunction, nil)
	props.PrivateKey.GrantRead(functions.OutboxFunction, nil)
	props.PrivateKey.GrantRead(functions.APIFunction, nil)
	
	return functions
}

func createFunction(stack awscdk.Stack, name string, props *awslambda.FunctionProps, codePath string) awslambda.Function {
	funcProps := *props
	funcProps.FunctionName = jsii.String(fmt.Sprintf("lesser-%s", name)) // Match Pulumi naming
	funcProps.Code = awslambda.Code_FromAsset(jsii.String(codePath), nil)
	funcProps.Handler = jsii.String("bootstrap")
	
	return awslambda.NewFunction(stack, jsii.String(name+"Function"), &funcProps)
}