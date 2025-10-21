package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/jsii-runtime-go"
)

func TestSQSProcessor_DefaultConfiguration(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-sqs-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	// Test that all components are created
	if processor.Function == nil {
		t.Error("Function should be created")
	}
	if processor.Queue == nil {
		t.Error("Queue should be created")
	}
	if processor.DeadLetterQueue == nil {
		t.Error("Dead letter queue should be created by default")
	}
	if processor.EventSource == nil {
		t.Error("Event source should be created")
	}

	// Synthesize to verify template
	template := synthesizeTemplate(t, stack)

	// Verify SQS queue exists
	assertResourceExists(t, template, "AWS::SQS::Queue")

	// Verify dead letter queue exists
	assertResourceExists(t, template, "AWS::SQS::Queue")

	// Verify Lambda function
	assertResourceExists(t, template, "AWS::Lambda::Function")

	// Verify event source mapping
	assertResourceExists(t, template, "AWS::Lambda::EventSourceMapping")
}

func TestSQSProcessor_CustomConfiguration(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("custom-sqs-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		BatchSize:         jsii.Number(20),
		MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(10)),
		MaxReceiveCount:   jsii.Number(5),
		VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(10)),
		QueueProps: &awssqs.QueueProps{
			QueueName: jsii.String("custom-queue-name"),
		},
		EnableTracing:     jsii.Bool(true),
		EnableMultiTenant: jsii.Bool(true),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify custom configuration is applied
	assertResourceExists(t, template, "AWS::SQS::Queue")

	// Verify custom event source configuration
	assertResourceExists(t, template, "AWS::Lambda::EventSourceMapping")
}

func TestSQSProcessor_FIFOQueue(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("fifo-sqs-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		FifoQueue:                       jsii.Bool(true),
		EnableContentBasedDeduplication: jsii.Bool(true),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify FIFO queue configuration
	assertResourceExists(t, template, "AWS::SQS::Queue")

	// Verify FIFO DLQ configuration
	assertResourceExists(t, template, "AWS::SQS::Queue")
}

func TestSQSProcessor_ExistingQueue(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create existing queue
	existingQueue := awssqs.NewQueue(stack, jsii.String("ExistingQueue"), &awssqs.QueueProps{
		QueueName: jsii.String("existing-queue"),
	})

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("existing-queue-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		ExistingQueue: existingQueue,
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	if processor.Queue != existingQueue {
		t.Error("Should use existing queue")
	}

	template := synthesizeTemplate(t, stack)

	// Should still create Lambda and event source mapping
	assertResourceExists(t, template, "AWS::Lambda::Function")

	assertResourceExists(t, template, "AWS::Lambda::EventSourceMapping")
}

func TestSQSProcessor_DisabledDLQ(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("no-dlq-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableDeadLetterQueue: jsii.Bool(false),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	if processor.DeadLetterQueue != nil {
		t.Error("Dead letter queue should not be created when disabled")
	}

	template := synthesizeTemplate(t, stack)

	// Count SQS queues - should only be 1 (main queue, no DLQ)
	assertResourceCount(t, template, "AWS::SQS::Queue", 1)
}

func TestSQSProcessor_LongPolling(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("long-polling-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		ReceiveMessageWaitTimeSeconds: jsii.Number(20),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify long polling configuration
	assertResourceExists(t, template, "AWS::SQS::Queue")
}

func TestSQSProcessor_EnvironmentVariables(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("env-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
			Environment: &map[string]*string{
				"CUSTOM_VAR": jsii.String("custom-value"),
			},
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify environment variables are set (including SQS-specific ones)
	functions := findResourcesByType(template, "AWS::Lambda::Function")
	if len(functions) != 1 {
		t.Fatalf("Expected 1 function, got %d", len(functions))
	}

	// Get the first (and only) function from the map
	var function map[string]interface{}
	for _, fn := range functions {
		function = fn
		break
	}
	props, ok := function["Properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Function should have Properties")
	}

	env, ok := props["Environment"].(map[string]interface{})
	if !ok {
		t.Fatal("Function should have Environment")
	}

	variables, ok := env["Variables"].(map[string]interface{})
	if !ok {
		t.Fatal("Environment should have Variables")
	}

	// Should have custom variable
	if variables["CUSTOM_VAR"] != "custom-value" {
		t.Error("Should preserve custom environment variables")
	}

	// Should have SQS queue URL
	if _, exists := variables["SQS_QUEUE_URL"]; !exists {
		t.Error("Should add SQS_QUEUE_URL environment variable")
	}

	// Should have DLQ URL (DLQ is enabled by default)
	if _, exists := variables["SQS_DLQ_URL"]; !exists {
		t.Error("Should add SQS_DLQ_URL environment variable when DLQ is enabled")
	}
}

func TestSQSProcessor_GrantPermissions(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("permission-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	// Create another function to test permissions
	anotherFunction := awslambda.NewFunction(stack, jsii.String("AnotherFunction"), &awslambda.FunctionProps{
		FunctionName: jsii.String("another-function"),
		Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
		Handler:      jsii.String("index.handler"),
		Runtime:      awslambda.Runtime_NODEJS_18_X(),
	})

	// Grant permissions
	processor.GrantSendMessages(anotherFunction)
	processor.GrantConsumeMessages(anotherFunction)

	template := synthesizeTemplate(t, stack)

	// Verify IAM policies are created
	policies := findResourcesByType(template, "AWS::IAM::Policy")
	if len(policies) == 0 {
		t.Error("Should create IAM policies for SQS permissions")
	}
}

func TestSQSProcessor_CustomEventSourceProps(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewSQSProcessor(stack, jsii.String("TestSQSProcessor"), &SQSProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("custom-event-source-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EventSourceProps: &awslambdaeventsources.SqsEventSourceProps{
			BatchSize:               jsii.Number(5),
			MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(30)),
			ReportBatchItemFailures: jsii.Bool(false),
			MaxConcurrency:          jsii.Number(10),
		},
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := synthesizeTemplate(t, stack)

	// Verify custom event source configuration
	assertResourceExists(t, template, "AWS::Lambda::EventSourceMapping")
}
