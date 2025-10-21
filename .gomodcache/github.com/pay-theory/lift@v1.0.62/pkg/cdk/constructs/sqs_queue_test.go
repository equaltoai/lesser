package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

func TestLiftSQSQueue_WithExistingFunction(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a test Lambda function
	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	queue := NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:       testFn,
		QueueName:      jsii.String("test-queue"),
		QueueUrlEnvVar: jsii.String("TEST_QUEUE_URL"),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify queue created
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName": "test-queue",
	})

	// Verify DLQ created
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName": "test-queue-dlq",
	})

	// Verify event source mapping created
	template.ResourceCountIs(jsii.String("AWS::Lambda::EventSourceMapping"), jsii.Number(1))

	// Verify queue URL is accessible
	if queue.GetQueueUrl() == nil {
		t.Error("Queue URL should not be nil")
	}
}

func TestLiftSQSQueue_WithCustomEnvironmentVariable(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:       testFn,
		QueueName:      jsii.String("processor-instrument-queue"),
		QueueUrlEnvVar: jsii.String("K3_PROCESSOR_INSTRUMENT_QUEUE_URL"),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify Lambda has custom environment variable
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]interface{}{
		"Environment": map[string]interface{}{
			"Variables": map[string]interface{}{
				"K3_PROCESSOR_INSTRUMENT_QUEUE_URL": assertions.Match_AnyValue(),
			},
		},
	})
}

func TestLiftSQSQueue_WithKMSEncryption(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	kmsKey := awskms.NewKey(stack, jsii.String("TestKey"), &awskms.KeyProps{
		Description: jsii.String("Test KMS key"),
	})

	// WHEN
	NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:            testFn,
		QueueName:           jsii.String("encrypted-queue"),
		EncryptionMasterKey: kmsKey,
		DataKeyReuse:        awscdk.Duration_Seconds(jsii.Number(300)),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify queue has KMS encryption
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName": "encrypted-queue",
		"KmsMasterKeyId": map[string]interface{}{
			"Fn::GetAtt": []interface{}{
				assertions.Match_StringLikeRegexp(jsii.String(".*Key.*")),
				jsii.String("Arn"),
			},
		},
		"KmsDataKeyReusePeriodSeconds": float64(300),
	})
}

func TestLiftSQSQueue_WithCustomBatchSize(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN - Create queue with custom batch size
	NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:  testFn,
		QueueName: jsii.String("custom-batch-queue"),
		BatchSize: jsii.Number(20),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify event source mapping created with custom batch size
	template.ResourceCountIs(jsii.String("AWS::Lambda::EventSourceMapping"), jsii.Number(1))

	// Verify queue created
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName": "custom-batch-queue",
	})
}

func TestLiftSQSQueue_WithSSMParameter(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	queue := NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:           testFn,
		QueueName:          jsii.String("test-queue"),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterName:   jsii.String("/k3/qakernel/paytheorylab/test-queue-url"),
		SSMDescription:     jsii.String("Test queue URL"),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify SSM parameter created
	template.HasResourceProperties(jsii.String("AWS::SSM::Parameter"), map[string]interface{}{
		"Name":        "/k3/qakernel/paytheorylab/test-queue-url",
		"Description": "Test queue URL",
		"Type":        "String",
	})

	// Verify SSM parameter is accessible
	if queue.SSMParameter == nil {
		t.Error("SSM parameter should not be nil when enabled")
	}
}

func TestLiftSQSQueue_WithFIFOQueue(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:                        testFn,
		QueueName:                       jsii.String("test-fifo-queue"),
		FifoQueue:                       jsii.Bool(true),
		EnableContentBasedDeduplication: jsii.Bool(true),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify FIFO queue created with .fifo suffix
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName":                 "test-fifo-queue.fifo",
		"FifoQueue":                 true,
		"ContentBasedDeduplication": true,
	})

	// Verify FIFO DLQ also has .fifo suffix
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName": "test-fifo-queue-dlq.fifo",
		"FifoQueue": true,
	})
}

func TestLiftSQSQueue_WithCustomVisibilityTimeout(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	testFn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN - Create queue with custom visibility timeout (like AuthVoid's 5 minutes)
	NewLiftSQSQueue(stack, jsii.String("TestQueue"), &LiftSQSQueueProps{
		Function:          testFn,
		QueueName:         jsii.String("auth-void-queue"),
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(300)),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify visibility timeout is set
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName":         "auth-void-queue",
		"VisibilityTimeout": float64(300),
	})
}

func TestLiftSQSQueue_MultipleQueuesOnSameFunction(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a single Lambda function
	testFn := awslambda.NewFunction(stack, jsii.String("K3Function"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_20_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN - Attach three different queues to the same function (K3 pattern)
	queue1 := NewLiftSQSQueue(stack, jsii.String("ProcessorInstrument"), &LiftSQSQueueProps{
		Function:          testFn,
		QueueName:         jsii.String("processor-instrument"),
		QueueUrlEnvVar:    jsii.String("K3_PROCESSOR_INSTRUMENT_QUEUE_URL"),
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(60)),
	})

	queue2 := NewLiftSQSQueue(stack, jsii.String("AuthVoid"), &LiftSQSQueueProps{
		Function:               testFn,
		QueueName:              jsii.String("auth-void"),
		QueueUrlEnvVar:         jsii.String("K3_AUTH_VOID_QUEUE_URL"),
		VisibilityTimeout:      awscdk.Duration_Seconds(jsii.Number(300)),
		MessageRetentionPeriod: awscdk.Duration_Hours(jsii.Number(24)),
	})

	queue3 := NewLiftSQSQueue(stack, jsii.String("RapidConnect"), &LiftSQSQueueProps{
		Function:          testFn,
		QueueName:         jsii.String("rapid-connect-tor"),
		QueueUrlEnvVar:    jsii.String("K3_RAPID_CONNECT_TOR_QUEUE_URL"),
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(120)),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify all three queues created
	template.ResourceCountIs(jsii.String("AWS::SQS::Queue"), jsii.Number(6)) // 3 main + 3 DLQ

	// Verify all three event source mappings (all queues trigger the same Lambda)
	template.ResourceCountIs(jsii.String("AWS::Lambda::EventSourceMapping"), jsii.Number(3))

	// Verify all queues are accessible
	if queue1.GetQueueUrl() == nil || queue2.GetQueueUrl() == nil || queue3.GetQueueUrl() == nil {
		t.Error("All queue URLs should be accessible")
	}

	// Verify Lambda function has all three environment variables
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]interface{}{
		"Environment": map[string]interface{}{
			"Variables": map[string]interface{}{
				"K3_PROCESSOR_INSTRUMENT_QUEUE_URL": assertions.Match_AnyValue(),
				"K3_AUTH_VOID_QUEUE_URL":            assertions.Match_AnyValue(),
				"K3_RAPID_CONNECT_TOR_QUEUE_URL":    assertions.Match_AnyValue(),
			},
		},
	})
}
