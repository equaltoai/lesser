package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

func TestDynamoStreamProcessor_BasicCreation(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewDynamoStreamProcessor(stack, jsii.String("TestDynamoStreamProcessor"), &DynamoStreamProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-dynamo-stream-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	// Basic verification that constructs were created
	assert.NotNil(t, processor)
	assert.NotNil(t, processor.Function)
	assert.NotNil(t, processor.StreamingTable)
	assert.NotNil(t, processor.EventSource)
	assert.NotNil(t, processor.DeadLetterQueue)
}

func TestDynamoStreamProcessor_WithCustomTable_Basic(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewDynamoStreamProcessor(stack, jsii.String("TestDynamoStreamProcessor"), &DynamoStreamProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("custom-table-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		StreamingTableProps: &StreamingTableProps{
			TableName: jsii.String("custom-streaming-table"),
		},
	})

	// Verify the processor created a streaming table
	assert.NotNil(t, processor.StreamingTable)
	assert.NotNil(t, processor.StreamingTable.Table)
	assert.NotNil(t, processor.Function)
}

func TestDynamoStreamProcessor_DisabledDLQ_Basic(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewDynamoStreamProcessor(stack, jsii.String("TestDynamoStreamProcessor"), &DynamoStreamProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("no-dlq-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableDeadLetterQueue: jsii.Bool(false),
	})

	// Verify DLQ is nil when disabled
	assert.Nil(t, processor.DeadLetterQueue)
	assert.NotNil(t, processor.Function)
	assert.NotNil(t, processor.StreamingTable)
}

func TestDynamoStreamProcessor_HelperMethods_Basic(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewDynamoStreamProcessor(stack, jsii.String("TestDynamoStreamProcessor"), &DynamoStreamProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("helper-methods-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})

	// Test helper methods return non-nil values
	assert.NotNil(t, processor.GetTableName())
	assert.NotNil(t, processor.GetTableArn())
	assert.NotNil(t, processor.GetStreamArn())
	assert.NotNil(t, processor.GetDeadLetterQueueUrl())

	// Test adding environment variable doesn't panic
	processor.AddEnvironmentVariable("TEST_VAR", "test-value")

	// Create another Lambda to test grant methods
	otherFunction := awslambda.NewFunction(stack, jsii.String("OtherFunction"), &awslambda.FunctionProps{
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
		Handler: jsii.String("index.handler"),
		Runtime: awslambda.Runtime_NODEJS_18_X(),
	})

	// Test grant methods don't panic
	processor.GrantReadWriteData(otherFunction)
	processor.GrantStreamRead(otherFunction)
	processor.GrantReadData(otherFunction)
	processor.GrantWriteData(otherFunction)
}

func TestDynamoStreamProcessor_MonitoringEnabled(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	processor := NewDynamoStreamProcessor(stack, jsii.String("MonitoredStreamProcessor"), &DynamoStreamProcessorProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("monitored-stream-processor"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableMonitoring: jsii.Bool(true),
	})

	if processor == nil {
		t.Fatal("Processor should be created")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Should create at least one CloudWatch alarm and exactly one dashboard
	template.HasResourceProperties(jsii.String("AWS::CloudWatch::Alarm"), map[string]interface{}{})
	template.ResourceCountIs(jsii.String("AWS::CloudWatch::Dashboard"), jsii.Number(1))
}

func TestDynamoStreamProcessor_NilProps_Basic(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Test with nil props - should still create resources with defaults
	processor := NewDynamoStreamProcessor(stack, jsii.String("TestDynamoStreamProcessor"), nil)

	assert.NotNil(t, processor.Function)
	assert.NotNil(t, processor.StreamingTable)
}
