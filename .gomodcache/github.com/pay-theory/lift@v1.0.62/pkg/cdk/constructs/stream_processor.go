package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// StreamProcessorProps defines properties for a stream processor
// Memory optimized: 792 → 784 bytes (8 bytes saved)
type StreamProcessorProps struct {
	// Pointers first (8 bytes each)
	StreamingTable          *StreamingTable
	DeadLetterQueueProps    *awssqs.QueueProps
	EventSourceProps        *awslambdaeventsources.DynamoEventSourceProps
	BatchSize               *float64
	RetryAttempts           *float64
	ParallelizationFactor   *float64
	EnableDeadLetterQueue   *bool
	BisectBatchOnError      *bool
	ReportBatchItemFailures *bool
	// Duration structs (16 bytes each)
	MaxBatchingWindow awscdk.Duration
	MaxRecordAge      awscdk.Duration
	TumblingWindow    awscdk.Duration
	// Large struct
	FunctionProps awslambda.FunctionProps
	// Medium types
	StartingPosition awslambda.StartingPosition
}

// StreamProcessor processes DynamoDB streams with Lambda
type StreamProcessor struct {
	constructs.Construct

	// The Lambda function processing the stream
	Function *LiftFunction

	// The table with streams
	Table *StreamingTable

	// Dead letter queue (if enabled)
	DeadLetterQueue awssqs.IQueue

	// Event source mapping
	EventSource awslambdaeventsources.DynamoEventSource
}

// NewStreamProcessor creates a new stream processor construct
func NewStreamProcessor(scope constructs.Construct, id *string, props *StreamProcessorProps) *StreamProcessor {
	builder := newStreamProcessorBuilder(scope, id, props)
	return builder.build()
}

// streamProcessorBuilder builds stream processors with Lambda and DynamoDB integration
type streamProcessorBuilder struct {
	scope     constructs.Construct
	id        *string
	props     *StreamProcessorProps
	construct constructs.Construct
	processor *StreamProcessor
	config    *streamProcessorConfig
}

// streamProcessorConfig holds resolved configuration values
type streamProcessorConfig struct {
	startingPosition awslambda.StartingPosition
	batchSize        float64
	enableDLQ        bool
}

// newStreamProcessorBuilder creates a new stream processor builder
func newStreamProcessorBuilder(scope constructs.Construct, id *string, props *StreamProcessorProps) *streamProcessorBuilder {
	// Validate required properties
	if props == nil || props.StreamingTable == nil {
		panic("StreamingTable is required for StreamProcessor")
	}

	return &streamProcessorBuilder{
		scope:  scope,
		id:     id,
		props:  props,
		config: buildStreamProcessorConfig(props),
	}
}

// buildStreamProcessorConfig resolves configuration values with defaults
func buildStreamProcessorConfig(props *StreamProcessorProps) *streamProcessorConfig {
	config := &streamProcessorConfig{
		batchSize:        10,
		enableDLQ:        true,
		startingPosition: awslambda.StartingPosition_LATEST,
	}

	if props.BatchSize != nil {
		config.batchSize = *props.BatchSize
	}
	if props.EnableDeadLetterQueue != nil {
		config.enableDLQ = *props.EnableDeadLetterQueue
	}
	if props.StartingPosition != "" {
		config.startingPosition = props.StartingPosition
	}

	return config
}

// build constructs the complete stream processor
func (b *streamProcessorBuilder) build() *StreamProcessor {
	b.construct = constructs.NewConstruct(b.scope, b.id)
	b.processor = &StreamProcessor{
		Construct: b.construct,
		Table:     b.props.StreamingTable,
	}

	b.createDeadLetterQueue()
	b.createFunction()
	b.createEventSource()
	b.grantPermissions()

	return b.processor
}

// createDeadLetterQueue creates DLQ if enabled
func (b *streamProcessorBuilder) createDeadLetterQueue() {
	if !b.config.enableDLQ {
		return
	}

	dlqProps := &awssqs.QueueProps{
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
	}

	if b.props.DeadLetterQueueProps != nil {
		dlqProps = b.props.DeadLetterQueueProps
	}

	b.processor.DeadLetterQueue = awssqs.NewQueue(b.construct, jsii.String("DLQ"), dlqProps)
}

// createFunction creates the Lambda function
func (b *streamProcessorBuilder) createFunction() {
	functionProps := b.props.FunctionProps

	// Configure DLQ if enabled
	if b.config.enableDLQ && b.processor.DeadLetterQueue != nil {
		functionProps.DeadLetterQueueEnabled = jsii.Bool(true)
		functionProps.DeadLetterQueue = b.processor.DeadLetterQueue
	}

	// Setup environment variables
	b.setupEnvironment(&functionProps)

	// Create the Lambda function
	b.processor.Function = NewLiftFunction(b.construct, jsii.String("Function"), &LiftFunctionProps{
		FunctionProps: functionProps,
	})
}

// setupEnvironment configures environment variables for the function
func (b *streamProcessorBuilder) setupEnvironment(functionProps *awslambda.FunctionProps) {
	if functionProps.Environment == nil {
		functionProps.Environment = &map[string]*string{}
	}

	env := *functionProps.Environment
	env["DYNAMODB_STREAM_ARN"] = b.processor.Table.GetStreamArn()
	env["DYNAMODB_TABLE_NAME"] = b.processor.Table.Table.TableName()
	functionProps.Environment = &env
}

// createEventSource creates and configures the DynamoDB event source
func (b *streamProcessorBuilder) createEventSource() {
	eventSourceProps := b.buildEventSourceProps()

	// Override with user-provided props if any
	if b.props.EventSourceProps != nil {
		eventSourceProps = b.props.EventSourceProps
	}

	// Create and add the event source
	b.processor.EventSource = awslambdaeventsources.NewDynamoEventSource(
		b.processor.Table.Table,
		eventSourceProps,
	)
	b.processor.Function.Function.AddEventSource(b.processor.EventSource)
}

// buildEventSourceProps builds event source properties with defaults and overrides
func (b *streamProcessorBuilder) buildEventSourceProps() *awslambdaeventsources.DynamoEventSourceProps {
	eventSourceProps := &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        b.config.startingPosition,
		BatchSize:               jsii.Number(b.config.batchSize),
		Enabled:                 jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
	}

	// Apply optional settings
	b.applyOptionalEventSourceSettings(eventSourceProps)

	return eventSourceProps
}

// applyOptionalEventSourceSettings applies optional event source configuration
func (b *streamProcessorBuilder) applyOptionalEventSourceSettings(props *awslambdaeventsources.DynamoEventSourceProps) {
	if b.props.MaxBatchingWindow != nil {
		props.MaxBatchingWindow = b.props.MaxBatchingWindow
	}
	if b.props.MaxRecordAge != nil {
		props.MaxRecordAge = b.props.MaxRecordAge
	}
	if b.props.BisectBatchOnError != nil {
		props.BisectBatchOnError = b.props.BisectBatchOnError
	}
	if b.props.RetryAttempts != nil {
		props.RetryAttempts = b.props.RetryAttempts
	}
	if b.props.ReportBatchItemFailures != nil {
		props.ReportBatchItemFailures = b.props.ReportBatchItemFailures
	}
	if b.props.TumblingWindow != nil {
		props.TumblingWindow = b.props.TumblingWindow
	}
	if b.props.ParallelizationFactor != nil {
		props.ParallelizationFactor = b.props.ParallelizationFactor
	}
}

// grantPermissions grants necessary permissions to the function
func (b *streamProcessorBuilder) grantPermissions() {
	b.processor.Table.GrantStreamRead(b.processor.Function.Function)
}

// Example usage:
//
// streamingTable := constructs.NewStreamingTable(stack, jsii.String("MyTable"), &constructs.StreamingTableProps{
//     TableName: jsii.String("my-table"),
// })
//
// processor := constructs.NewStreamProcessor(stack, jsii.String("Processor"), &constructs.StreamProcessorProps{
//     StreamingTable: streamingTable,
//     FunctionProps: awslambda.FunctionProps{
//         Runtime: awslambda.Runtime_PROVIDED_AL2023(),
//         Handler: jsii.String("bootstrap"),
//         Code:    awslambda.Code_FromAsset(jsii.String("./handler"), nil),
//     },
// })
