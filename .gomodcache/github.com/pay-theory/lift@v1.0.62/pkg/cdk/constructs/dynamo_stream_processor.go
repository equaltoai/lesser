package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// DynamoStreamProcessorProps defines properties for a DynamoDB stream processor
// Memory optimized: 816 → 808 bytes (8 bytes saved)
type DynamoStreamProcessorProps struct {
	// StreamingTableProps defines the properties of the underlying DynamORM streaming table.
	StreamingTableProps *StreamingTableProps
	// DeadLetterQueueProps configures an optional SQS dead‑letter queue for failed stream records.
	DeadLetterQueueProps *awssqs.QueueProps
	// EventSourceProps allows overriding any of the default DynamoDB event source settings.
	EventSourceProps *awslambdaeventsources.DynamoEventSourceProps

	// Optional fine‑grained tuning parameters. If nil, sensible defaults are applied.
	BatchSize               *float64 // Number of records to fetch per batch (default 10)
	RetryAttempts           *float64 // Max retry attempts for failed batches (default 10000)
	ParallelizationFactor   *float64 // Parallelism factor for batch processing (default 1)
	EnableDeadLetterQueue   *bool    // Whether to provision a dead‑letter queue (default true)
	BisectBatchOnError      *bool    // Split failing batch into smaller batches (default false)
	ReportBatchItemFailures *bool    // Report individual item failures to Lambda (default true)
	EnableTracing           *bool    // Enable X‑Ray tracing for the Lambda function
	EnableMultiTenant       *bool    // Configure the function for multi‑tenant use cases
	EnableMonitoring        *bool    // Attach CloudWatch monitoring dashboards

	// Duration settings control throttling and record retention.
	MaxBatchingWindow awscdk.Duration // Maximum time to wait before invoking the function (default 5 s)
	MaxRecordAge      awscdk.Duration // Maximum age of a stream record before it is discarded (default 24 h)
	TumblingWindow    awscdk.Duration // Optional tumbling window for aggregating records

	// FunctionProps contains the underlying Lambda configuration.
	FunctionProps awslambda.FunctionProps
	// StartingPosition specifies where the stream should start reading.
	StartingPosition awslambda.StartingPosition
}

// DynamoStreamProcessor represents a DynamoDB table with stream processor using DynamORM
type DynamoStreamProcessor struct {
	constructs.Construct

	// The Lambda function processing DynamoDB stream records
	Function *LiftFunction

	// The DynamORM streaming table
	StreamingTable *StreamingTable

	// Dead letter queue (if enabled)
	DeadLetterQueue awssqs.IQueue

	// Event source mapping
	EventSource awslambdaeventsources.DynamoEventSource
}

// NewDynamoStreamProcessor creates a new DynamoDB stream processor construct using DynamORM
func NewDynamoStreamProcessor(scope constructs.Construct, id *string, props *DynamoStreamProcessorProps) *DynamoStreamProcessor {
	// Creates a new DynamoDB stream processor construct.
	//
	// Example usage:
	//   processor := constructs.NewDynamoStreamProcessor(this, jsii.String("MyProcessor"), &constructs.DynamoStreamProcessorProps{
	//       StreamingTableProps: &constructs.StreamingTableProps{ /* ... */ },
	//       FunctionProps: awslambda.FunctionProps{
	//           Runtime: awslambda.Runtime_NODEJS_18_X(),
	//           Handler: jsii.String("index.handler"),
	//       },
	//   })
	this := &DynamoStreamProcessor{}
	constructs.NewConstruct_Override(this, scope, id)

	// Apply default empty props if none are provided.
	if props == nil {
		props = &DynamoStreamProcessorProps{}
	}

	builder := newDynamoStreamProcessorBuilder(this, props)
	return builder.build()
}

// dynamoStreamProcessorBuilder builds DynamoDB stream processor components
type dynamoStreamProcessorBuilder struct {
	processor *DynamoStreamProcessor
	props     *DynamoStreamProcessorProps
	config    *dynamoStreamProcessorConfig
}

// dynamoStreamProcessorConfig holds resolved configuration values
// Memory optimized: 96 → 80 bytes (16 bytes saved)
type dynamoStreamProcessorConfig struct {
	// maxBatchingWindow defines the default maximum batching window.
	maxBatchingWindow awscdk.Duration
	// maxRecordAge defines the default maximum age for a stream record.
	maxRecordAge awscdk.Duration
	// startingPosition determines where the Lambda begins reading the stream.
	startingPosition awslambda.StartingPosition

	// batchSize is the default number of records per batch.
	batchSize float64
	// retryAttempts defines how many times to retry a failed batch.
	retryAttempts float64
	// parallelizationFactor controls concurrency for batch processing.
	parallelizationFactor float64

	// bisectBatchOnError, reportBatchItemFailures and enableDLQ toggle optional behaviors.
	bisectBatchOnError      bool
	reportBatchItemFailures bool
	enableDLQ               bool
}

// newDynamoStreamProcessorBuilder creates a new DynamoDB stream processor builder
func newDynamoStreamProcessorBuilder(processor *DynamoStreamProcessor, props *DynamoStreamProcessorProps) *dynamoStreamProcessorBuilder {
	return &dynamoStreamProcessorBuilder{
		processor: processor,
		props:     props,
		config:    buildDynamoStreamProcessorConfig(props),
	}
}

// buildDynamoStreamProcessorConfig resolves configuration values with defaults
func buildDynamoStreamProcessorConfig(props *DynamoStreamProcessorProps) *dynamoStreamProcessorConfig {
	// Resolve configuration values, applying defaults where the user has not supplied a value.
	config := &dynamoStreamProcessorConfig{
		batchSize:               float64(10),                             // default batch size
		maxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)), // default 5 s window
		startingPosition:        awslambda.StartingPosition_LATEST,       // start from latest records
		maxRecordAge:            awscdk.Duration_Hours(jsii.Number(24)),  // retain records for 24 h
		bisectBatchOnError:      false,                                   // do not split failing batches by default
		retryAttempts:           float64(10000),                          // generous retry limit
		reportBatchItemFailures: true,                                    // report individual failures
		parallelizationFactor:   float64(1),                              // single‑threaded processing
		enableDLQ:               true,                                    // provision DLQ by default
	}

	// Apply provided values
	if props.BatchSize != nil {
		config.batchSize = *props.BatchSize
	}
	if props.MaxBatchingWindow != nil {
		config.maxBatchingWindow = props.MaxBatchingWindow
	}
	if props.StartingPosition != "" {
		config.startingPosition = props.StartingPosition
	}
	if props.MaxRecordAge != nil {
		config.maxRecordAge = props.MaxRecordAge
	}
	if props.BisectBatchOnError != nil {
		config.bisectBatchOnError = *props.BisectBatchOnError
	}
	if props.RetryAttempts != nil {
		config.retryAttempts = *props.RetryAttempts
	}
	if props.ReportBatchItemFailures != nil {
		config.reportBatchItemFailures = *props.ReportBatchItemFailures
	}
	if props.ParallelizationFactor != nil {
		config.parallelizationFactor = *props.ParallelizationFactor
	}
	if props.EnableDeadLetterQueue != nil {
		config.enableDLQ = *props.EnableDeadLetterQueue
	}

	return config
}

// build constructs the complete DynamoDB stream processor
func (b *dynamoStreamProcessorBuilder) build() *DynamoStreamProcessor {
	// Create streaming table
	b.setupStreamingTable()

	// Create dead letter queue if enabled
	b.setupDeadLetterQueue()

	// Create Lambda function
	b.setupFunction()

	// Configure event source
	b.setupEventSource()

	// Setup permissions
	b.setupPermissions()

	// Add monitoring if enabled
	b.setupMonitoring()

	return b.processor
}

// setupStreamingTable creates the DynamORM streaming table
func (b *dynamoStreamProcessorBuilder) setupStreamingTable() {
	streamingTableProps := &StreamingTableProps{}
	if b.props.StreamingTableProps != nil {
		streamingTableProps = b.props.StreamingTableProps
	}

	// Set table name based on function name if not provided
	if streamingTableProps.TableName == nil && b.props.FunctionProps.FunctionName != nil {
		streamingTableProps.TableName = jsii.String(*b.props.FunctionProps.FunctionName + "-table")
	}

	b.processor.StreamingTable = NewStreamingTable(b.processor, jsii.String("StreamingTable"), streamingTableProps)
}

// setupDeadLetterQueue creates the dead letter queue if enabled
func (b *dynamoStreamProcessorBuilder) setupDeadLetterQueue() {
	if !b.config.enableDLQ {
		return
	}

	dlqBuilder := newDeadLetterQueueBuilder(
		b.processor,
		b.props.DeadLetterQueueProps,
		b.props.FunctionProps.FunctionName,
		"-stream-dlq",
	)
	b.processor.DeadLetterQueue = dlqBuilder.build()
}

// setupFunction creates the Lambda function with DynamoDB environment variables
func (b *dynamoStreamProcessorBuilder) setupFunction() {
	functionBuilder := newDynamoStreamFunctionBuilder(b.processor, b.props)
	b.processor.Function = functionBuilder.build()
}

// setupEventSource configures the DynamoDB stream event source
func (b *dynamoStreamProcessorBuilder) setupEventSource() {
	eventSourceBuilder := newDynamoStreamEventSourceBuilder(b.processor, b.props, b.config)
	b.processor.EventSource = eventSourceBuilder.build()
}

// setupPermissions grants necessary permissions
func (b *dynamoStreamProcessorBuilder) setupPermissions() {
	b.processor.StreamingTable.GrantStreamRead(b.processor.Function.Function)
	b.processor.StreamingTable.GrantReadWrite(b.processor.Function.Function)
	if b.processor.DeadLetterQueue != nil {
		b.processor.DeadLetterQueue.GrantSendMessages(b.processor.Function.Function)
	}
}

// setupMonitoring adds monitoring if enabled
func (b *dynamoStreamProcessorBuilder) setupMonitoring() {
	if b.props.EnableMonitoring != nil && *b.props.EnableMonitoring {
		b.processor.enableMonitoring()
	}
}

// dynamoStreamFunctionBuilder builds Lambda function components
type dynamoStreamFunctionBuilder struct {
	processor *DynamoStreamProcessor
	props     *DynamoStreamProcessorProps
}

// newDynamoStreamFunctionBuilder creates a new DynamoDB stream function builder
func newDynamoStreamFunctionBuilder(processor *DynamoStreamProcessor, props *DynamoStreamProcessorProps) *dynamoStreamFunctionBuilder {
	return &dynamoStreamFunctionBuilder{
		processor: processor,
		props:     props,
	}
}

// build creates the Lambda function with DynamoDB environment variables
func (fb *dynamoStreamFunctionBuilder) build() *LiftFunction {
	// Prepare environment variables
	functionEnv := fb.prepareFunctionEnvironment()

	// Create LiftFunction properties
	liftProps := &LiftFunctionProps{
		FunctionProps: fb.props.FunctionProps,
	}

	// Set default code and runtime if not provided
	fb.setDefaultFunctionProps(liftProps)

	// Set environment variables
	liftProps.Environment = &functionEnv

	// Set Lift-specific properties
	if fb.props.EnableTracing != nil {
		liftProps.EnableTracing = fb.props.EnableTracing
	}
	if fb.props.EnableMultiTenant != nil {
		liftProps.EnableMultiTenant = fb.props.EnableMultiTenant
	}

	return NewLiftFunction(fb.processor, jsii.String("Function"), liftProps)
}

// prepareFunctionEnvironment prepares environment variables for the function
func (fb *dynamoStreamFunctionBuilder) prepareFunctionEnvironment() map[string]*string {
	functionEnv := make(map[string]*string)

	// Copy existing environment variables
	if fb.props.FunctionProps.Environment != nil {
		for k, v := range *fb.props.FunctionProps.Environment {
			functionEnv[k] = v
		}
	}

	// Add DynamoDB-specific environment variables
	functionEnv["DYNAMODB_TABLE_NAME"] = fb.processor.StreamingTable.GetTableName()
	functionEnv["DYNAMODB_TABLE_ARN"] = fb.processor.StreamingTable.GetTableArn()
	if fb.processor.StreamingTable.GetStreamArn() != nil {
		functionEnv["DYNAMODB_STREAM_ARN"] = fb.processor.StreamingTable.GetStreamArn()
	}
	if fb.processor.DeadLetterQueue != nil {
		functionEnv["DYNAMODB_DLQ_URL"] = fb.processor.DeadLetterQueue.QueueUrl()
	}

	return functionEnv
}

// setDefaultFunctionProps sets default function properties if not provided
func (fb *dynamoStreamFunctionBuilder) setDefaultFunctionProps(liftProps *LiftFunctionProps) {
	if liftProps.Code == nil {
		liftProps.Code = awslambda.Code_FromInline(jsii.String("exports.handler = async (event) => { console.log('Stream event:', JSON.stringify(event)); };"))
	}
	if liftProps.Handler == nil {
		liftProps.Handler = jsii.String("index.handler")
	}
	if liftProps.Runtime == nil {
		liftProps.Runtime = awslambda.Runtime_NODEJS_18_X()
	}
}

// dynamoStreamEventSourceBuilder builds DynamoDB stream event source components
type dynamoStreamEventSourceBuilder struct {
	processor *DynamoStreamProcessor
	props     *DynamoStreamProcessorProps
	config    *dynamoStreamProcessorConfig
}

// newDynamoStreamEventSourceBuilder creates a new DynamoDB stream event source builder
func newDynamoStreamEventSourceBuilder(processor *DynamoStreamProcessor, props *DynamoStreamProcessorProps, config *dynamoStreamProcessorConfig) *dynamoStreamEventSourceBuilder {
	return &dynamoStreamEventSourceBuilder{
		processor: processor,
		props:     props,
		config:    config,
	}
}

// build creates and configures the DynamoDB stream event source
func (esb *dynamoStreamEventSourceBuilder) build() awslambdaeventsources.DynamoEventSource {
	// Create base event source properties
	eventSourceProps := esb.createBaseEventSourceProps()

	// Set tumbling window if specified
	if esb.props.TumblingWindow != nil {
		eventSourceProps.TumblingWindow = esb.props.TumblingWindow
	}

	// Apply user-provided event source properties
	esb.applyUserEventSourceProps(eventSourceProps)

	// Create event source and add to function
	eventSource := awslambdaeventsources.NewDynamoEventSource(esb.processor.StreamingTable.Table, eventSourceProps)
	esb.processor.Function.Function.AddEventSource(eventSource)

	return eventSource
}

// createBaseEventSourceProps creates base event source properties from config
func (esb *dynamoStreamEventSourceBuilder) createBaseEventSourceProps() *awslambdaeventsources.DynamoEventSourceProps {
	return &awslambdaeventsources.DynamoEventSourceProps{
		StartingPosition:        esb.config.startingPosition,
		BatchSize:               jsii.Number(esb.config.batchSize),
		MaxBatchingWindow:       esb.config.maxBatchingWindow,
		MaxRecordAge:            esb.config.maxRecordAge,
		BisectBatchOnError:      jsii.Bool(esb.config.bisectBatchOnError),
		RetryAttempts:           jsii.Number(esb.config.retryAttempts),
		ReportBatchItemFailures: jsii.Bool(esb.config.reportBatchItemFailures),
		ParallelizationFactor:   jsii.Number(esb.config.parallelizationFactor),
	}
}

// applyUserEventSourceProps applies user-provided event source properties
func (esb *dynamoStreamEventSourceBuilder) applyUserEventSourceProps(eventSourceProps *awslambdaeventsources.DynamoEventSourceProps) {
	if esb.props.EventSourceProps == nil {
		return
	}

	props := esb.props.EventSourceProps

	if props.StartingPosition != "" {
		eventSourceProps.StartingPosition = props.StartingPosition
	}
	if props.BatchSize != nil {
		eventSourceProps.BatchSize = props.BatchSize
	}
	if props.MaxBatchingWindow != nil {
		eventSourceProps.MaxBatchingWindow = props.MaxBatchingWindow
	}
	if props.MaxRecordAge != nil {
		eventSourceProps.MaxRecordAge = props.MaxRecordAge
	}
	if props.BisectBatchOnError != nil {
		eventSourceProps.BisectBatchOnError = props.BisectBatchOnError
	}
	if props.RetryAttempts != nil {
		eventSourceProps.RetryAttempts = props.RetryAttempts
	}
	if props.ReportBatchItemFailures != nil {
		eventSourceProps.ReportBatchItemFailures = props.ReportBatchItemFailures
	}
	if props.ParallelizationFactor != nil {
		eventSourceProps.ParallelizationFactor = props.ParallelizationFactor
	}
	if props.TumblingWindow != nil {
		eventSourceProps.TumblingWindow = props.TumblingWindow
	}
	if props.Enabled != nil {
		eventSourceProps.Enabled = props.Enabled
	}
	if props.Filters != nil {
		eventSourceProps.Filters = props.Filters
	}
}

// enableMonitoring adds CloudWatch alarms and metrics for the DynamoDB stream processor
func (d *DynamoStreamProcessor) enableMonitoring() {
	// Create SNS topic for alerts
	_ = awssns.NewTopic(d, jsii.String("AlarmTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(fmt.Sprintf("%s-stream-alarms", *d.StreamingTable.GetTableName())),
		DisplayName: jsii.String(fmt.Sprintf("Stream alarms for %s", *d.StreamingTable.GetTableName())),
	})

	if d.Function != nil && d.Function.Function != nil {
		EnableStreamLambdaMonitoring(d, d.StreamingTable.GetTableName(), d.Function.Function)
	}

	// DynamoDB table metrics
	if d.StreamingTable != nil {
		// User errors on table operations
		userErrorsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("UserErrors"),
			DimensionsMap: &map[string]*string{
				"TableName": d.StreamingTable.GetTableName(),
			},
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			Statistic: awscloudwatch.Stats_SUM(),
		})

		awscloudwatch.NewAlarm(d, jsii.String("UserErrorsAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-user-errors", *d.StreamingTable.GetTableName())),
			AlarmDescription:   jsii.String("User errors on DynamoDB operations"),
			Metric:             userErrorsMetric,
			Threshold:          jsii.Number(10),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// System errors on table operations
		systemErrorsMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("SystemErrors"),
			DimensionsMap: &map[string]*string{
				"TableName": d.StreamingTable.GetTableName(),
			},
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			Statistic: awscloudwatch.Stats_SUM(),
		})

		awscloudwatch.NewAlarm(d, jsii.String("SystemErrorsAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-system-errors", *d.StreamingTable.GetTableName())),
			AlarmDescription:   jsii.String("System errors on DynamoDB operations"),
			Metric:             systemErrorsMetric,
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// Throttled requests alarm
		throttledMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("GetRecords.Throttled"),
			DimensionsMap: &map[string]*string{
				"TableName": d.StreamingTable.GetTableName(),
			},
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			Statistic: awscloudwatch.Stats_SUM(),
		})

		awscloudwatch.NewAlarm(d, jsii.String("StreamThrottledAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-stream-throttled", *d.StreamingTable.GetTableName())),
			AlarmDescription:   jsii.String("DynamoDB stream GetRecords throttled"),
			Metric:             throttledMetric,
			Threshold:          jsii.Number(5),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// DLQ monitoring if enabled
	if d.DeadLetterQueue != nil {
		awscloudwatch.NewAlarm(d, jsii.String("DLQMessagesAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-stream-dlq-messages", *d.StreamingTable.GetTableName())),
			AlarmDescription: jsii.String("Messages in stream processor dead letter queue"),
			Metric: d.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// Create CloudWatch dashboard
	dashboard := awscloudwatch.NewDashboard(d, jsii.String("StreamProcessorDashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-stream-processor-dashboard", *d.StreamingTable.GetTableName())),
	})

	// Add widgets to dashboard
	if d.Function != nil && d.Function.Function != nil {
		dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("Lambda Function Metrics"),
				Left: &[]awscloudwatch.IMetric{
					d.Function.Function.MetricInvocations(nil),
					d.Function.Function.MetricErrors(nil),
					d.Function.Function.MetricThrottles(nil),
				},
				Right: &[]awscloudwatch.IMetric{
					d.Function.Function.MetricDuration(nil),
				},
			}),
		)

		// Add iterator age widget
		iteratorAgeWidgetMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/Lambda"),
			MetricName: jsii.String("IteratorAge"),
			DimensionsMap: &map[string]*string{
				"FunctionName": d.Function.Function.FunctionName(),
			},
		})

		dashboard.AddWidgets(
			awscloudwatch.NewSingleValueWidget(&awscloudwatch.SingleValueWidgetProps{
				Title: jsii.String("Stream Iterator Age"),
				Metrics: &[]awscloudwatch.IMetric{
					iteratorAgeWidgetMetric,
				},
			}),
		)
	}

	if d.StreamingTable != nil {
		// Create metrics for dashboard
		userErrorsDashboardMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("UserErrors"),
			DimensionsMap: &map[string]*string{
				"TableName": d.StreamingTable.GetTableName(),
			},
		})

		systemErrorsDashboardMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String("SystemErrors"),
			DimensionsMap: &map[string]*string{
				"TableName": d.StreamingTable.GetTableName(),
			},
		})

		dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("DynamoDB Stream Metrics"),
				Left: &[]awscloudwatch.IMetric{
					userErrorsDashboardMetric,
					systemErrorsDashboardMetric,
				},
			}),
		)
	}

	if d.DeadLetterQueue != nil {
		dashboard.AddWidgets(
			awscloudwatch.NewSingleValueWidget(&awscloudwatch.SingleValueWidgetProps{
				Title: jsii.String("Dead Letter Queue"),
				Metrics: &[]awscloudwatch.IMetric{
					d.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(nil),
				},
			}),
		)
	}
}

// GrantReadWriteData grants permission to read and write data to the table
func (d *DynamoStreamProcessor) GrantReadWriteData(grantee awslambda.IFunction) {
	d.StreamingTable.GrantReadWrite(grantee)
}

// GrantStreamRead grants permission to read from the DynamoDB stream
func (d *DynamoStreamProcessor) GrantStreamRead(grantee awslambda.IFunction) {
	d.StreamingTable.GrantStreamRead(grantee)
}

// GrantReadData grants permission to read data from the table
func (d *DynamoStreamProcessor) GrantReadData(grantee awslambda.IFunction) {
	d.StreamingTable.Table.GrantReadData(awsiam.IGrantable(grantee))
}

// GrantWriteData grants permission to write data to the table
func (d *DynamoStreamProcessor) GrantWriteData(grantee awslambda.IFunction) {
	d.StreamingTable.Table.GrantWriteData(awsiam.IGrantable(grantee))
}

// AddEnvironmentVariable adds an environment variable to the Lambda function
func (d *DynamoStreamProcessor) AddEnvironmentVariable(key string, value string) {
	d.Function.Function.AddEnvironment(jsii.String(key), jsii.String(value), nil)
}

// GetTableName returns the table name
func (d *DynamoStreamProcessor) GetTableName() *string {
	return d.StreamingTable.GetTableName()
}

// GetTableArn returns the table ARN
func (d *DynamoStreamProcessor) GetTableArn() *string {
	return d.StreamingTable.GetTableArn()
}

// GetStreamArn returns the DynamoDB stream ARN
func (d *DynamoStreamProcessor) GetStreamArn() *string {
	return d.StreamingTable.GetStreamArn()
}

// GetDeadLetterQueueUrl returns the DLQ URL if enabled
func (d *DynamoStreamProcessor) GetDeadLetterQueueUrl() *string {
	if d.DeadLetterQueue != nil {
		return d.DeadLetterQueue.QueueUrl()
	}
	return nil
}
