package main

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type DynamORMStreamProcessingStackProps struct {
	awscdk.StackProps
}

// DynamORMStreamProcessingStackProps represents the properties for the DynamORMStreamProcessingStack.
// It extends the awscdk.StackProps to include additional properties specific to the stack.

type DynamORMStreamProcessingStack struct {
	awscdk.Stack
}

// DynamORMStreamProcessingStack represents a stack for processing DynamoDB streams.
// It extends the awscdk.Stack to include methods for setting up the stack.

func NewDynamORMStreamProcessingStack(scope constructs.Construct, id string, props *DynamORMStreamProcessingStackProps) *DynamORMStreamProcessingStack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// Create a table with streaming enabled
	userTable := liftconstructs.NewStreamingTable(stack, jsii.String("UserTable"), &liftconstructs.StreamingTableProps{
		TableName:      jsii.String("users"),
		StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
	})

	// GSIs are now defined in DynamORM model structs using tags like:
	// Email string `dynamorm:"index:email-index,pk"`
	// CreatedAt string `dynamorm:"index:email-index,sk"`

	// Create a stream processor for user events
	userStreamProcessor := liftconstructs.NewStreamProcessor(stack, jsii.String("UserStreamProcessor"), &liftconstructs.StreamProcessorProps{
		StreamingTable: userTable,
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("user-stream-processor"),
			Code:         awslambda.Code_FromAsset(jsii.String("./lambda"), nil),
			Handler:      jsii.String("user-stream-handler"),
			Runtime:      awslambda.Runtime_PROVIDED_AL2(),
			Architecture: awslambda.Architecture_ARM_64(),
			Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
			MemorySize:   jsii.Number(512),
			Environment: &map[string]*string{
				"LOG_LEVEL": jsii.String("INFO"),
			},
		},
		// Streaming configuration
		BatchSize:               jsii.Number(25),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		StartingPosition:        awslambda.StartingPosition_LATEST,
		ParallelizationFactor:   jsii.Number(2),
		ReportBatchItemFailures: jsii.Bool(true),
		// Event filtering and multi-tenancy should be handled in the Lambda function code
		// based on the DynamORM model structure and business logic

		// Dead letter queue
		EnableDeadLetterQueue: jsii.Bool(true),
	})

	// Enable X-Ray tracing on the function
	userStreamProcessor.Function.Function.AddEnvironment(jsii.String("_X_AMZN_TRACE_ID"), jsii.String("enabled"), nil)

	// Multi-tenant filtering should be done in the Lambda function code

	// Create another stream processor for analytics
	_ = liftconstructs.NewStreamProcessor(stack, jsii.String("AnalyticsStreamProcessor"), &liftconstructs.StreamProcessorProps{
		StreamingTable: userTable,
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("user-analytics-processor"),
			Code:         awslambda.Code_FromAsset(jsii.String("./analytics-lambda"), nil),
			Handler:      jsii.String("analytics-handler"),
			Runtime:      awslambda.Runtime_PROVIDED_AL2(),
			Architecture: awslambda.Architecture_ARM_64(),
			Timeout:      awscdk.Duration_Minutes(jsii.Number(10)),
			MemorySize:   jsii.Number(1024),
			Environment: &map[string]*string{
				"LOG_LEVEL":      jsii.String("INFO"),
				"ANALYTICS_MODE": jsii.String("realtime"),
			},
		},
		// Different configuration for analytics
		BatchSize:             jsii.Number(100), // Larger batches for analytics
		MaxBatchingWindow:     awscdk.Duration_Seconds(jsii.Number(30)),
		ParallelizationFactor: jsii.Number(4), // Higher parallelization
		// Event filtering should be handled in the Lambda function code
	})

	// Create a notification stream processor for critical events
	_ = liftconstructs.NewStreamProcessor(stack, jsii.String("NotificationStreamProcessor"), &liftconstructs.StreamProcessorProps{
		StreamingTable: userTable,
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("user-notification-processor"),
			Code:         awslambda.Code_FromAsset(jsii.String("./notification-lambda"), nil),
			Handler:      jsii.String("notification-handler"),
			Runtime:      awslambda.Runtime_PROVIDED_AL2(),
			Architecture: awslambda.Architecture_ARM_64(),
			Timeout:      awscdk.Duration_Minutes(jsii.Number(2)),
			MemorySize:   jsii.Number(256),
		},
		// Fast processing for notifications
		BatchSize:         jsii.Number(1), // Process one at a time for speed
		MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(1)),
		// Event filtering should be handled in the Lambda function code
	})

	// Environment variables are set in the FunctionProps above

	// Grant additional permissions if needed
	// Example: Grant access to external services
	// userStreamProcessor.GrantDynamORMAccess(externalFunction)

	return &DynamORMStreamProcessingStack{
		Stack: stack,
	}
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	NewDynamORMStreamProcessingStack(app, "DynamORMStreamProcessingStack", &DynamORMStreamProcessingStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

// env determines the AWS environment (account+region) in which our stack is to
// be deployed. For more information see: https://docs.aws.amazon.com/cdk/latest/guide/environments.html
func env() *awscdk.Environment {
	return nil
}
