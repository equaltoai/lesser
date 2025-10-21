package constructs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftSQSQueueProps defines properties for attaching an SQS queue to an existing Lambda function
type LiftSQSQueueProps struct {
	// Required: Existing Lambda function to attach this queue to
	Function awslambda.Function

	// Queue configuration
	QueueName              *string
	VisibilityTimeout      awscdk.Duration // Default: 5 minutes
	MessageRetentionPeriod awscdk.Duration // Default: 14 days
	ReceiveMessageWaitTime awscdk.Duration // For long polling, default: 20 seconds

	// Dead letter queue configuration
	EnableDeadLetterQueue *bool           // Default: true
	DeadLetterQueueName   *string         // Default: {QueueName}-dlq
	MaxReceiveCount       *float64        // Default: 3
	DLQRetentionPeriod    awscdk.Duration // Default: 14 days

	// Encryption configuration
	EncryptionMasterKey awskms.IKey     // Required for K3 - partner-specific KMS key
	DataKeyReuse        awscdk.Duration // Default: 300 seconds

	// Event source configuration
	EnableEventSource       *bool           // Default: true
	BatchSize               *float64        // Default: 10
	MaxBatchingWindow       awscdk.Duration // Default: 5 seconds
	ReportBatchItemFailures *bool           // Default: true
	MaxConcurrency          *float64        // Default: 5

	// Environment variable configuration
	QueueUrlEnvVar *string // Custom env var name for queue URL (e.g., "K3_PROCESSOR_INSTRUMENT_QUEUE_URL")
	DLQUrlEnvVar   *string // Custom env var name for DLQ URL (optional)

	// SSM Parameter Store configuration
	EnableSSMParameter *bool   // Default: false
	SSMParameterName   *string // SSM parameter name to store queue URL
	SSMDescription     *string // SSM parameter description

	// FIFO queue configuration
	FifoQueue                       *bool
	EnableContentBasedDeduplication *bool

	// Additional permissions
	GrantSendMessages    *bool // Default: true - grant Lambda permission to send messages
	GrantConsumeMessages *bool // Default: true - grant Lambda permission to consume messages
}

// LiftSQSQueue represents an SQS queue attached to an existing Lambda function
type LiftSQSQueue struct {
	constructs.Construct

	// The SQS queue
	Queue awssqs.Queue

	// Dead letter queue (if enabled)
	DeadLetterQueue awssqs.Queue

	// Event source mapping (if enabled)
	EventSource awslambdaeventsources.SqsEventSource

	// SSM Parameter (if enabled)
	SSMParameter awsssm.StringParameter
}

// NewLiftSQSQueue creates a new SQS queue and attaches it to an existing Lambda function
func NewLiftSQSQueue(scope constructs.Construct, id *string, props *LiftSQSQueueProps) *LiftSQSQueue {
	this := &LiftSQSQueue{}
	constructs.NewConstruct_Override(this, scope, id)

	if props.Function == nil {
		panic("Function is required - use existing Lambda function")
	}

	applySQSQueueDefaults(props)

	this.DeadLetterQueue = this.createDeadLetterQueue(props)
	queueProps := buildSQSQueueProps(props, this.DeadLetterQueue)
	this.Queue = awssqs.NewQueue(this, jsii.String("Queue"), queueProps)

	this.configureQueuePermissions(props)
	this.addQueueEnvironment(props)
	this.setupQueueEventSource(props)
	this.createQueueSSMParameter(props)

	return this
}

func applySQSQueueDefaults(props *LiftSQSQueueProps) {
	ensureBoolPtr(&props.EnableDeadLetterQueue, true)
	ensureFloatPtr(&props.MaxReceiveCount, 3)
	ensureDuration(&props.VisibilityTimeout, awscdk.Duration_Minutes(jsii.Number(5)))
	ensureDuration(&props.MessageRetentionPeriod, awscdk.Duration_Days(jsii.Number(14)))
	ensureDuration(&props.ReceiveMessageWaitTime, awscdk.Duration_Seconds(jsii.Number(20)))
	ensureDuration(&props.DLQRetentionPeriod, awscdk.Duration_Days(jsii.Number(14)))
	ensureDuration(&props.DataKeyReuse, awscdk.Duration_Seconds(jsii.Number(300)))
	ensureBoolPtr(&props.EnableEventSource, true)
	ensureFloatPtr(&props.BatchSize, 10)
	ensureDuration(&props.MaxBatchingWindow, awscdk.Duration_Seconds(jsii.Number(5)))
	ensureBoolPtr(&props.ReportBatchItemFailures, true)
	ensureFloatPtr(&props.MaxConcurrency, 5)
	ensureBoolPtr(&props.GrantSendMessages, true)
	ensureBoolPtr(&props.GrantConsumeMessages, true)
	ensureBoolPtr(&props.EnableSSMParameter, false)
}

func ensureBoolPtr(target **bool, value bool) {
	if *target == nil {
		*target = jsii.Bool(value)
	}
}

func ensureFloatPtr(target **float64, value float64) {
	if *target == nil {
		*target = jsii.Number(value)
	}
}

func ensureDuration(target *awscdk.Duration, fallback awscdk.Duration) {
	if *target == nil {
		*target = fallback
	}
}

func (q *LiftSQSQueue) createDeadLetterQueue(props *LiftSQSQueueProps) awssqs.Queue {
	if props.EnableDeadLetterQueue == nil || !*props.EnableDeadLetterQueue {
		return nil
	}

	dlqName := props.DeadLetterQueueName
	if dlqName == nil && props.QueueName != nil {
		dlqName = jsii.String(*props.QueueName + "-dlq")
	}

	dlqProps := &awssqs.QueueProps{
		QueueName:       dlqName,
		RetentionPeriod: props.DLQRetentionPeriod,
	}

	applyQueueEncryption(dlqProps, props)
	configureFifoQueue(dlqProps, props, dlqProps.QueueName, false)

	return awssqs.NewQueue(q, jsii.String("DeadLetterQueue"), dlqProps)
}

func buildSQSQueueProps(props *LiftSQSQueueProps, dlq awssqs.Queue) *awssqs.QueueProps {
	queueProps := &awssqs.QueueProps{
		QueueName:              props.QueueName,
		VisibilityTimeout:      props.VisibilityTimeout,
		RetentionPeriod:        props.MessageRetentionPeriod,
		ReceiveMessageWaitTime: props.ReceiveMessageWaitTime,
	}

	applyQueueEncryption(queueProps, props)

	if dlq != nil && props.EnableDeadLetterQueue != nil && *props.EnableDeadLetterQueue {
		queueProps.DeadLetterQueue = &awssqs.DeadLetterQueue{
			MaxReceiveCount: props.MaxReceiveCount,
			Queue:           dlq,
		}
	}

	configureFifoQueue(queueProps, props, queueProps.QueueName, true)

	return queueProps
}

func applyQueueEncryption(queueProps *awssqs.QueueProps, props *LiftSQSQueueProps) {
	if props.EncryptionMasterKey == nil {
		return
	}

	queueProps.EncryptionMasterKey = props.EncryptionMasterKey
	queueProps.DataKeyReuse = props.DataKeyReuse
}

func configureFifoQueue(queueProps *awssqs.QueueProps, props *LiftSQSQueueProps, originalName *string, applyDedup bool) {
	if props.FifoQueue == nil || !*props.FifoQueue {
		return
	}

	queueProps.Fifo = jsii.Bool(true)
	if applyDedup && props.EnableContentBasedDeduplication != nil {
		queueProps.ContentBasedDeduplication = props.EnableContentBasedDeduplication
	}

	queueProps.QueueName = ensureFifoName(originalName)
}

func ensureFifoName(name *string) *string {
	if name == nil {
		return nil
	}

	if strings.HasSuffix(*name, ".fifo") {
		return name
	}

	return jsii.String(*name + ".fifo")
}

func (q *LiftSQSQueue) configureQueuePermissions(props *LiftSQSQueueProps) {
	if props.GrantSendMessages != nil && *props.GrantSendMessages {
		q.Queue.GrantSendMessages(props.Function)
	}
	if props.GrantConsumeMessages != nil && *props.GrantConsumeMessages {
		q.Queue.GrantConsumeMessages(props.Function)
	}
}

func (q *LiftSQSQueue) addQueueEnvironment(props *LiftSQSQueueProps) {
	envVarName := jsii.String("SQS_QUEUE_URL")
	if props.QueueUrlEnvVar != nil {
		envVarName = jsii.String(*props.QueueUrlEnvVar)
	}

	props.Function.AddEnvironment(envVarName, q.Queue.QueueUrl(), nil)

	if props.DLQUrlEnvVar != nil && q.DeadLetterQueue != nil {
		props.Function.AddEnvironment(jsii.String(*props.DLQUrlEnvVar), q.DeadLetterQueue.QueueUrl(), nil)
	}
}

func (q *LiftSQSQueue) setupQueueEventSource(props *LiftSQSQueueProps) {
	if props.EnableEventSource == nil || !*props.EnableEventSource {
		return
	}

	eventSourceProps := &awslambdaeventsources.SqsEventSourceProps{
		BatchSize:               props.BatchSize,
		ReportBatchItemFailures: props.ReportBatchItemFailures,
		MaxConcurrency:          props.MaxConcurrency,
	}

	if props.FifoQueue == nil || !*props.FifoQueue {
		eventSourceProps.MaxBatchingWindow = props.MaxBatchingWindow
	}

	q.EventSource = awslambdaeventsources.NewSqsEventSource(q.Queue, eventSourceProps)
	q.EventSource.Bind(props.Function)
}

func (q *LiftSQSQueue) createQueueSSMParameter(props *LiftSQSQueueProps) {
	if props.EnableSSMParameter == nil || !*props.EnableSSMParameter || props.SSMParameterName == nil {
		return
	}

	description := props.SSMDescription
	if description == nil {
		description = jsii.String(defaultQueueDescription(props.QueueName))
	}

	q.SSMParameter = awsssm.NewStringParameter(q, jsii.String("SSMParameter"), &awsssm.StringParameterProps{
		ParameterName: props.SSMParameterName,
		StringValue:   q.Queue.QueueUrl(),
		Description:   description,
	})
}

func defaultQueueDescription(name *string) string {
	if name == nil {
		return "Queue URL"
	}
	return fmt.Sprintf("Queue URL for %s", *name)
}

// GetQueueUrl returns the queue URL
func (q *LiftSQSQueue) GetQueueUrl() *string {
	return q.Queue.QueueUrl()
}

// GetQueueArn returns the queue ARN
func (q *LiftSQSQueue) GetQueueArn() *string {
	return q.Queue.QueueArn()
}

// GetQueueName returns the queue name
func (q *LiftSQSQueue) GetQueueName() *string {
	return q.Queue.QueueName()
}

// GetDeadLetterQueueUrl returns the DLQ URL (if enabled)
func (q *LiftSQSQueue) GetDeadLetterQueueUrl() *string {
	if q.DeadLetterQueue != nil {
		return q.DeadLetterQueue.QueueUrl()
	}
	return nil
}

// GrantSendMessages grants additional permission to send messages to the queue
func (q *LiftSQSQueue) GrantSendMessages(grantee awslambda.Function) {
	q.Queue.GrantSendMessages(grantee)
}

// GrantConsumeMessages grants additional permission to consume messages from the queue
func (q *LiftSQSQueue) GrantConsumeMessages(grantee awslambda.Function) {
	q.Queue.GrantConsumeMessages(grantee)
}
