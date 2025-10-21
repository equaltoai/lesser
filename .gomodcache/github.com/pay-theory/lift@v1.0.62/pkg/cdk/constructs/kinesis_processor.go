package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskinesis"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// KinesisProcessorProps defines the properties for creating a Kinesis processor
type KinesisProcessorProps struct {
	// The Lambda function configuration
	FunctionProps *LiftFunctionProps `field:"required"`

	// Optional: Stream configuration
	StreamProps *awskinesis.StreamProps `field:"optional"`

	// Optional: Use an existing stream instead of creating a new one
	ExistingStream awskinesis.IStream `field:"optional"`

	// Optional: Event source configuration
	EventSourceProps *awslambdaeventsources.KinesisEventSourceProps `field:"optional"`

	// Optional: Enable dead letter queue for failed records
	EnableDLQ *bool `field:"optional"`

	// Optional: DLQ configuration
	DLQProps *awssqs.QueueProps `field:"optional"`

	// Optional: Stream mode (provisioned or on-demand)
	StreamMode *awskinesis.StreamMode `field:"optional"`

	// Optional: Number of shards (for provisioned mode)
	ShardCount *float64 `field:"optional"`

	// Optional: Data retention period in hours (24-8760 hours)
	RetentionPeriodHours *float64 `field:"optional"`

	// Optional: Enable encryption
	Encryption *awskinesis.StreamEncryption `field:"optional"`

	// Optional: Enable enhanced fan-out
	EnableEnhancedFanOut *bool `field:"optional"`

	// Optional: Consumer name for enhanced fan-out
	ConsumerName *string `field:"optional"`

	// Optional: Batch size for processing (1-10000)
	BatchSize *float64 `field:"optional"`

	// Optional: Maximum batching window in seconds
	MaxBatchingWindowSeconds *float64 `field:"optional"`

	// Optional: Parallelization factor (1-10)
	ParallelizationFactor *float64 `field:"optional"`

	// Optional: Starting position
	StartingPosition *awslambda.StartingPosition `field:"optional"`

	// Optional: Maximum record age in seconds
	MaxRecordAgeSeconds *float64 `field:"optional"`

	// Optional: Bisect batch on function error
	BisectBatchOnError *bool `field:"optional"`

	// Optional: Maximum retry attempts
	RetryAttempts *float64 `field:"optional"`

	// Optional: Tumbling window in seconds
	TumblingWindowSeconds *float64 `field:"optional"`

	// Optional: Report batch item failures
	ReportBatchItemFailures *bool `field:"optional"`
}

// KinesisProcessor creates a Kinesis stream with Lambda processor
type KinesisProcessor struct {
	constructs.Construct
	Stream   awskinesis.IStream
	Function LiftFunction
	DLQ      awssqs.IQueue
	Consumer awskinesis.IStreamConsumer
}

// NewKinesisProcessor creates a new Kinesis processor with Lambda function
func NewKinesisProcessor(scope constructs.Construct, id *string, props *KinesisProcessorProps) *KinesisProcessor {
	builder := newKinesisProcessorBuilder(scope, id, props)
	return builder.build()
}

// kinesisProcessorBuilder builds Kinesis processors with Lambda functions
type kinesisProcessorBuilder struct {
	scope     constructs.Construct
	id        *string
	props     *KinesisProcessorProps
	construct constructs.Construct
}

// newKinesisProcessorBuilder creates a new Kinesis processor builder
func newKinesisProcessorBuilder(scope constructs.Construct, id *string, props *KinesisProcessorProps) *kinesisProcessorBuilder {
	return &kinesisProcessorBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete Kinesis processor
func (b *kinesisProcessorBuilder) build() *KinesisProcessor {
	b.construct = constructs.NewConstruct(b.scope, b.id)

	stream := b.createOrGetStream()
	function := b.createFunction(stream)
	dlq := b.createDLQ(function)
	consumer := b.createConsumer(stream)
	b.configureEventSource(stream, function, dlq)
	b.grantPermissions(stream, function, dlq)

	return &KinesisProcessor{
		Construct: b.construct,
		Stream:    stream,
		Function:  *function,
		DLQ:       dlq,
		Consumer:  consumer,
	}
}

// createOrGetStream creates a new stream or uses existing one
func (b *kinesisProcessorBuilder) createOrGetStream() awskinesis.IStream {
	if b.props.ExistingStream != nil {
		return b.props.ExistingStream
	}

	streamBuilder := newKinesisStreamBuilder(b.construct, b.props)
	return streamBuilder.build()
}

// createFunction creates the Lambda function with environment variables
func (b *kinesisProcessorBuilder) createFunction(stream awskinesis.IStream) *LiftFunction {
	function := NewLiftFunction(b.construct, jsii.String("Function"), b.props.FunctionProps)

	function.Function.AddEnvironment(jsii.String("KINESIS_STREAM_ARN"), stream.StreamArn(), nil)
	function.Function.AddEnvironment(jsii.String("KINESIS_STREAM_NAME"), stream.StreamName(), nil)

	return function
}

// createDLQ creates the dead letter queue if enabled
func (b *kinesisProcessorBuilder) createDLQ(function *LiftFunction) awssqs.IQueue {
	enableDLQ := true
	if b.props.EnableDLQ != nil {
		enableDLQ = *b.props.EnableDLQ
	}

	if !enableDLQ {
		return nil
	}

	dlqBuilder := newDeadLetterQueueBuilder(
		b.construct,
		b.props.DLQProps,
		b.props.FunctionProps.FunctionName,
		"-kinesis-dlq",
	)
	dlq := dlqBuilder.build()

	function.Function.AddEnvironment(jsii.String("KINESIS_DLQ_URL"), dlq.QueueUrl(), nil)
	return dlq
}

// createConsumer creates the enhanced fan-out consumer if enabled
func (b *kinesisProcessorBuilder) createConsumer(stream awskinesis.IStream) awskinesis.IStreamConsumer {
	enableEnhancedFanOut := false
	if b.props.EnableEnhancedFanOut != nil {
		enableEnhancedFanOut = *b.props.EnableEnhancedFanOut
	}

	if !enableEnhancedFanOut {
		return nil
	}

	return awskinesis.NewStreamConsumer(b.construct, jsii.String("Consumer"), &awskinesis.StreamConsumerProps{
		Stream: stream,
	})
}

// kinesisStreamBuilder builds Kinesis streams
type kinesisStreamBuilder struct {
	construct constructs.Construct
	props     *KinesisProcessorProps
}

// newKinesisStreamBuilder creates a new Kinesis stream builder
func newKinesisStreamBuilder(construct constructs.Construct, props *KinesisProcessorProps) *kinesisStreamBuilder {
	return &kinesisStreamBuilder{
		construct: construct,
		props:     props,
	}
}

// build creates the Kinesis stream with configured properties
func (sb *kinesisStreamBuilder) build() awskinesis.IStream {
	streamProps := sb.createStreamProps()
	return awskinesis.NewStream(sb.construct, jsii.String("Stream"), streamProps)
}

// createStreamProps creates stream properties with defaults
func (sb *kinesisStreamBuilder) createStreamProps() *awskinesis.StreamProps {
	streamProps := sb.props.StreamProps
	if streamProps == nil {
		streamProps = &awskinesis.StreamProps{}
	}

	sb.configureStreamMode(streamProps)
	sb.configureShardCount(streamProps)
	sb.configureRetention(streamProps)
	sb.configureEncryption(streamProps)

	return streamProps
}

// configureStreamMode sets the stream mode
func (sb *kinesisStreamBuilder) configureStreamMode(streamProps *awskinesis.StreamProps) {
	if sb.props.StreamMode != nil {
		streamProps.StreamMode = *sb.props.StreamMode
	} else {
		streamProps.StreamMode = awskinesis.StreamMode_ON_DEMAND
	}
}

// configureShardCount sets the shard count for provisioned mode
func (sb *kinesisStreamBuilder) configureShardCount(streamProps *awskinesis.StreamProps) {
	if sb.props.ShardCount != nil {
		streamProps.ShardCount = sb.props.ShardCount
	}
}

// configureRetention sets the retention period
func (sb *kinesisStreamBuilder) configureRetention(streamProps *awskinesis.StreamProps) {
	if sb.props.RetentionPeriodHours != nil {
		streamProps.RetentionPeriod = awscdk.Duration_Hours(sb.props.RetentionPeriodHours)
	} else if streamProps.RetentionPeriod == nil {
		streamProps.RetentionPeriod = awscdk.Duration_Hours(jsii.Number(24))
	}
}

// configureEncryption sets the encryption configuration
func (sb *kinesisStreamBuilder) configureEncryption(streamProps *awskinesis.StreamProps) {
	if sb.props.Encryption != nil {
		streamProps.Encryption = *sb.props.Encryption
	}
}

// configureEventSource configures the Kinesis event source for the Lambda function
func (b *kinesisProcessorBuilder) configureEventSource(stream awskinesis.IStream, function *LiftFunction, _ awssqs.IQueue) {
	eventSourceBuilder := newKinesisEventSourceBuilder(b.props)
	eventSource := eventSourceBuilder.build(stream)
	function.Function.AddEventSource(eventSource)
}

// grantPermissions grants necessary permissions
func (b *kinesisProcessorBuilder) grantPermissions(stream awskinesis.IStream, function *LiftFunction, dlq awssqs.IQueue) {
	stream.GrantRead(function.Function)
	if dlq != nil {
		dlq.GrantSendMessages(function.Function)
	}
}

// kinesisEventSourceBuilder builds Kinesis event sources
type kinesisEventSourceBuilder struct {
	props *KinesisProcessorProps
}

// newKinesisEventSourceBuilder creates a new Kinesis event source builder
func newKinesisEventSourceBuilder(props *KinesisProcessorProps) *kinesisEventSourceBuilder {
	return &kinesisEventSourceBuilder{
		props: props,
	}
}

// build creates the Kinesis event source with configured properties
func (esb *kinesisEventSourceBuilder) build(stream awskinesis.IStream) awslambdaeventsources.KinesisEventSource {
	eventSourceProps := esb.createEventSourceProps()
	return awslambdaeventsources.NewKinesisEventSource(stream, eventSourceProps)
}

// createEventSourceProps creates event source properties with defaults
func (esb *kinesisEventSourceBuilder) createEventSourceProps() *awslambdaeventsources.KinesisEventSourceProps {
	eventSourceProps := esb.props.EventSourceProps
	if eventSourceProps == nil {
		eventSourceProps = &awslambdaeventsources.KinesisEventSourceProps{}
	}

	esb.configureBatching(eventSourceProps)
	esb.configureProcessing(eventSourceProps)
	esb.configureErrorHandling(eventSourceProps)

	return eventSourceProps
}

// configureBatching configures batch processing settings
func (esb *kinesisEventSourceBuilder) configureBatching(props *awslambdaeventsources.KinesisEventSourceProps) {
	if esb.props.BatchSize != nil {
		props.BatchSize = esb.props.BatchSize
	} else if props.BatchSize == nil {
		props.BatchSize = jsii.Number(100)
	}

	if esb.props.MaxBatchingWindowSeconds != nil {
		props.MaxBatchingWindow = awscdk.Duration_Seconds(esb.props.MaxBatchingWindowSeconds)
	} else if props.MaxBatchingWindow == nil {
		props.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(5))
	}

	if esb.props.ParallelizationFactor != nil {
		props.ParallelizationFactor = esb.props.ParallelizationFactor
	}
}

// configureProcessing configures processing settings
func (esb *kinesisEventSourceBuilder) configureProcessing(props *awslambdaeventsources.KinesisEventSourceProps) {
	if esb.props.StartingPosition != nil {
		props.StartingPosition = *esb.props.StartingPosition
	}
}

// configureErrorHandling configures error handling settings
func (esb *kinesisEventSourceBuilder) configureErrorHandling(props *awslambdaeventsources.KinesisEventSourceProps) {
	if esb.props.RetryAttempts != nil {
		props.RetryAttempts = esb.props.RetryAttempts
	}

	if esb.props.MaxRecordAgeSeconds != nil {
		props.MaxRecordAge = awscdk.Duration_Seconds(esb.props.MaxRecordAgeSeconds)
	}

	if esb.props.BisectBatchOnError != nil {
		props.BisectBatchOnError = esb.props.BisectBatchOnError
	}

	if esb.props.ReportBatchItemFailures != nil {
		props.ReportBatchItemFailures = esb.props.ReportBatchItemFailures
	}
}

// GrantWrite grants permission to write to the Kinesis stream
func (k *KinesisProcessor) GrantWrite(grantee awslambda.IFunction) {
	k.Stream.GrantWrite(grantee)
}

// GrantRead grants permission to read from the Kinesis stream
func (k *KinesisProcessor) GrantRead(grantee awslambda.IFunction) {
	k.Stream.GrantRead(grantee)
}

// GrantReadWrite grants permission to read and write to the Kinesis stream
func (k *KinesisProcessor) GrantReadWrite(grantee awslambda.IFunction) {
	k.Stream.GrantReadWrite(grantee)
}

// AddEnvironmentVariable adds an environment variable to the Lambda function
func (k *KinesisProcessor) AddEnvironmentVariable(key string, value string) {
	k.Function.Function.AddEnvironment(jsii.String(key), jsii.String(value), nil)
}

// GetStreamName returns the stream name
func (k *KinesisProcessor) GetStreamName() *string {
	return k.Stream.StreamName()
}

// GetStreamArn returns the stream ARN
func (k *KinesisProcessor) GetStreamArn() *string {
	return k.Stream.StreamArn()
}

// GetDeadLetterQueueUrl returns the DLQ URL if enabled
func (k *KinesisProcessor) GetDeadLetterQueueUrl() *string {
	if k.DLQ != nil {
		return k.DLQ.QueueUrl()
	}
	return nil
}
