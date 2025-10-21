package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

const (
	fifoSuffix = ".fifo"
)

// SQSProcessorProps defines properties for an SQS processor
type SQSProcessorProps struct {
	// Lambda function properties
	FunctionProps awslambda.FunctionProps

	// SQS queue properties (optional - creates new queue if not provided)
	QueueProps *awssqs.QueueProps

	// Existing queue to use (optional - creates new if not provided)
	ExistingQueue awssqs.IQueue

	// Dead letter queue properties (optional)
	DeadLetterQueueProps *awssqs.QueueProps

	// Enable dead letter queue (default: true)
	EnableDeadLetterQueue *bool

	// SQS event source configuration
	EventSourceProps *awslambdaeventsources.SqsEventSourceProps

	// Additional SQS processor settings
	BatchSize                       *float64        // Default: 10
	MaxBatchingWindow               awscdk.Duration // Default: 5 seconds
	VisibilityTimeout               awscdk.Duration // Default: 6 times function timeout
	MessageRetentionPeriod          awscdk.Duration // Default: 14 days
	MaxReceiveCount                 *float64        // Default: 3
	EnableContentBasedDeduplication *bool           // For FIFO queues
	FifoQueue                       *bool           // Default: false
	ReceiveMessageWaitTimeSeconds   *float64        // For long polling (0-20)

	// Lift-specific settings
	EnableTracing     *bool
	EnableMultiTenant *bool
	EnableMonitoring  *bool
}

// SQSProcessor represents an SQS queue with Lambda processor
type SQSProcessor struct {
	constructs.Construct

	// The Lambda function processing SQS messages
	Function *LiftFunction

	// The SQS queue
	Queue awssqs.IQueue

	// Dead letter queue (if enabled)
	DeadLetterQueue awssqs.IQueue

	// Event source mapping
	EventSource awslambdaeventsources.SqsEventSource
}

// NewSQSProcessor creates a new SQS processor construct
func NewSQSProcessor(scope constructs.Construct, id *string, props *SQSProcessorProps) *SQSProcessor {
	this := &SQSProcessor{}
	constructs.NewConstruct_Override(this, scope, id)

	// Set defaults
	if props == nil {
		props = &SQSProcessorProps{}
	}

	builder := newSQSProcessorBuilder(this, props)
	return builder.build()
}

// sqsProcessorBuilder builds SQS processor components
type sqsProcessorBuilder struct {
	processor *SQSProcessor
	props     *SQSProcessorProps
	config    *sqsProcessorConfig
}

// sqsProcessorConfig holds resolved configuration values
// Memory optimized: struct with 56 pointer bytes could be 48
type sqsProcessorConfig struct {
	// Durations first (16 bytes each)
	maxBatchingWindow      awscdk.Duration
	visibilityTimeout      awscdk.Duration
	messageRetentionPeriod awscdk.Duration
	// Float64s (8 bytes each)
	batchSize           float64
	maxReceiveCount     float64
	longPollingWaitTime float64
	// Booleans (1 byte each, packed together)
	enableDLQ bool
	fifoQueue bool
}

// newSQSProcessorBuilder creates a new SQS processor builder
func newSQSProcessorBuilder(processor *SQSProcessor, props *SQSProcessorProps) *sqsProcessorBuilder {
	return &sqsProcessorBuilder{
		processor: processor,
		props:     props,
		config:    buildSQSProcessorConfig(props),
	}
}

// buildSQSProcessorConfig resolves configuration values with defaults
func buildSQSProcessorConfig(props *SQSProcessorProps) *sqsProcessorConfig {
	config := &sqsProcessorConfig{
		batchSize:              float64(10),
		maxBatchingWindow:      awscdk.Duration_Seconds(jsii.Number(5)),
		visibilityTimeout:      awscdk.Duration_Minutes(jsii.Number(5)),
		messageRetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
		maxReceiveCount:        float64(3),
		enableDLQ:              true,
		fifoQueue:              false,
		longPollingWaitTime:    float64(0),
	}

	// Apply provided values
	if props.BatchSize != nil {
		config.batchSize = *props.BatchSize
	}
	if props.MaxBatchingWindow != nil {
		config.maxBatchingWindow = props.MaxBatchingWindow
	}
	if props.VisibilityTimeout != nil {
		config.visibilityTimeout = props.VisibilityTimeout
	}
	if props.MessageRetentionPeriod != nil {
		config.messageRetentionPeriod = props.MessageRetentionPeriod
	}
	if props.MaxReceiveCount != nil {
		config.maxReceiveCount = *props.MaxReceiveCount
	}
	if props.EnableDeadLetterQueue != nil {
		config.enableDLQ = *props.EnableDeadLetterQueue
	}
	if props.FifoQueue != nil {
		config.fifoQueue = *props.FifoQueue
	}
	if props.ReceiveMessageWaitTimeSeconds != nil {
		config.longPollingWaitTime = *props.ReceiveMessageWaitTimeSeconds
	}

	return config
}

// build constructs the complete SQS processor
func (b *sqsProcessorBuilder) build() *SQSProcessor {
	// Create or use existing queue
	b.setupQueue()

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

// setupQueue creates or configures the SQS queue
func (b *sqsProcessorBuilder) setupQueue() {
	if b.props.ExistingQueue != nil {
		b.processor.Queue = b.props.ExistingQueue
		return
	}

	queueBuilder := newSQSQueueBuilder(b.processor, b.props, b.config)
	b.processor.Queue, b.processor.DeadLetterQueue = queueBuilder.build()
}

// setupFunction creates the Lambda function
func (b *sqsProcessorBuilder) setupFunction() {
	functionBuilder := newSQSFunctionBuilder(b.processor, b.props)
	b.processor.Function = functionBuilder.build()
}

// setupEventSource configures the SQS event source
func (b *sqsProcessorBuilder) setupEventSource() {
	eventSourceBuilder := newSQSEventSourceBuilder(b.processor, b.props, b.config)
	b.processor.EventSource = eventSourceBuilder.build()
}

// setupPermissions grants necessary permissions
func (b *sqsProcessorBuilder) setupPermissions() {
	b.processor.Queue.GrantConsumeMessages(b.processor.Function.Function)
	if b.processor.DeadLetterQueue != nil {
		b.processor.DeadLetterQueue.GrantSendMessages(b.processor.Function.Function)
	}
}

// setupMonitoring adds monitoring if enabled
func (b *sqsProcessorBuilder) setupMonitoring() {
	if b.props.EnableMonitoring != nil && *b.props.EnableMonitoring {
		b.processor.enableMonitoring()
	}
}

// sqsQueueBuilder builds SQS queue components
type sqsQueueBuilder struct {
	processor *SQSProcessor
	props     *SQSProcessorProps
	config    *sqsProcessorConfig
}

// newSQSQueueBuilder creates a new SQS queue builder
func newSQSQueueBuilder(processor *SQSProcessor, props *SQSProcessorProps, config *sqsProcessorConfig) *sqsQueueBuilder {
	return &sqsQueueBuilder{
		processor: processor,
		props:     props,
		config:    config,
	}
}

// build creates the main queue and optional dead letter queue
func (qb *sqsQueueBuilder) build() (awssqs.Queue, awssqs.Queue) {
	var dlq awssqs.Queue
	var dlqConfig *awssqs.DeadLetterQueue

	// Create dead letter queue if enabled
	if qb.config.enableDLQ {
		dlq = qb.createDeadLetterQueue()
		dlqConfig = &awssqs.DeadLetterQueue{
			MaxReceiveCount: jsii.Number(qb.config.maxReceiveCount),
			Queue:           dlq,
		}
	}

	// Create main queue
	mainQueue := qb.createMainQueue(dlqConfig)

	return mainQueue, dlq
}

// createDeadLetterQueue creates the dead letter queue
func (qb *sqsQueueBuilder) createDeadLetterQueue() awssqs.Queue {
	// For FIFO queues, we need special handling
	if qb.config.fifoQueue {
		dlqProps := &awssqs.QueueProps{
			RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
		}
		if qb.props.DeadLetterQueueProps != nil {
			dlqProps = qb.props.DeadLetterQueueProps
			if dlqProps.RetentionPeriod == nil {
				dlqProps.RetentionPeriod = awscdk.Duration_Days(jsii.Number(14))
			}
		}
		if dlqProps.QueueName == nil && qb.props.FunctionProps.FunctionName != nil {
			dlqProps.QueueName = jsii.String(*qb.props.FunctionProps.FunctionName + "-dlq")
		}
		qb.applyFIFOConfig(dlqProps)
		return awssqs.NewQueue(qb.processor, jsii.String("DeadLetterQueue"), dlqProps)
	}

	// For regular queues, use the shared DLQ builder
	dlqBuilder := newDeadLetterQueueBuilder(
		qb.processor,
		qb.props.DeadLetterQueueProps,
		qb.props.FunctionProps.FunctionName,
		"-dlq",
	)
	dlq := dlqBuilder.build()
	// The shared DLQ builder returns awssqs.IQueue, but we know it's actually awssqs.Queue
	if queue, ok := dlq.(awssqs.Queue); ok {
		return queue
	}
	// Fallback - shouldn't happen but handle gracefully
	panic("unexpected: DLQ builder did not return awssqs.Queue")
}

// createMainQueue creates the main SQS queue
func (qb *sqsQueueBuilder) createMainQueue(dlqConfig *awssqs.DeadLetterQueue) awssqs.Queue {
	queueProps := &awssqs.QueueProps{
		VisibilityTimeout:      qb.config.visibilityTimeout,
		RetentionPeriod:        qb.config.messageRetentionPeriod,
		DeadLetterQueue:        dlqConfig,
		ReceiveMessageWaitTime: awscdk.Duration_Seconds(jsii.Number(qb.config.longPollingWaitTime)),
	}

	// Apply user-provided queue props
	qb.applyUserQueueProps(queueProps)

	// Apply FIFO configuration
	qb.applyFIFOConfig(queueProps)

	// Set default queue name if needed
	qb.setDefaultQueueName(queueProps)

	return awssqs.NewQueue(qb.processor, jsii.String("Queue"), queueProps)
}

// applyUserQueueProps applies user-provided queue properties
func (qb *sqsQueueBuilder) applyUserQueueProps(queueProps *awssqs.QueueProps) {
	if qb.props.QueueProps == nil {
		return
	}
	applyNonNilStructFields(queueProps, qb.props.QueueProps)
}

// applyFIFOConfig applies FIFO queue configuration
func (qb *sqsQueueBuilder) applyFIFOConfig(queueProps *awssqs.QueueProps) {
	if !qb.config.fifoQueue {
		return
	}

	queueProps.Fifo = jsii.Bool(true)
	if qb.props.EnableContentBasedDeduplication != nil {
		queueProps.ContentBasedDeduplication = qb.props.EnableContentBasedDeduplication
	}

	// Ensure FIFO queue name ends with .fifo
	if queueProps.QueueName != nil {
		queueName := *queueProps.QueueName
		if len(queueName) < 5 || queueName[len(queueName)-5:] != fifoSuffix {
			queueProps.QueueName = jsii.String(queueName + fifoSuffix)
		}
	}
}

// setDefaultQueueName sets a default queue name if none provided
func (qb *sqsQueueBuilder) setDefaultQueueName(queueProps *awssqs.QueueProps) {
	if queueProps.QueueName != nil || qb.props.FunctionProps.FunctionName == nil {
		return
	}

	suffix := ""
	if qb.config.fifoQueue {
		suffix = fifoSuffix
	}
	queueProps.QueueName = jsii.String(*qb.props.FunctionProps.FunctionName + "-queue" + suffix)
}

// sqsFunctionBuilder builds Lambda function components
type sqsFunctionBuilder struct {
	processor *SQSProcessor
	props     *SQSProcessorProps
}

// newSQSFunctionBuilder creates a new SQS function builder
func newSQSFunctionBuilder(processor *SQSProcessor, props *SQSProcessorProps) *sqsFunctionBuilder {
	return &sqsFunctionBuilder{
		processor: processor,
		props:     props,
	}
}

// build creates the Lambda function with SQS environment variables
func (fb *sqsFunctionBuilder) build() *LiftFunction {
	// Prepare environment variables
	functionEnv := fb.prepareFunctionEnvironment()

	// Create LiftFunction properties
	liftProps := &LiftFunctionProps{
		FunctionProps: fb.props.FunctionProps,
	}

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
func (fb *sqsFunctionBuilder) prepareFunctionEnvironment() map[string]*string {
	functionEnv := make(map[string]*string)

	// Copy existing environment variables
	if fb.props.FunctionProps.Environment != nil {
		for k, v := range *fb.props.FunctionProps.Environment {
			functionEnv[k] = v
		}
	}

	// Add SQS-specific environment variables
	functionEnv["SQS_QUEUE_URL"] = fb.processor.Queue.QueueUrl()
	if fb.processor.DeadLetterQueue != nil {
		functionEnv["SQS_DLQ_URL"] = fb.processor.DeadLetterQueue.QueueUrl()
	}

	return functionEnv
}

// sqsEventSourceBuilder builds SQS event source components
type sqsEventSourceBuilder struct {
	processor *SQSProcessor
	props     *SQSProcessorProps
	config    *sqsProcessorConfig
}

// newSQSEventSourceBuilder creates a new SQS event source builder
func newSQSEventSourceBuilder(processor *SQSProcessor, props *SQSProcessorProps, config *sqsProcessorConfig) *sqsEventSourceBuilder {
	return &sqsEventSourceBuilder{
		processor: processor,
		props:     props,
		config:    config,
	}
}

// build creates and configures the SQS event source
func (esb *sqsEventSourceBuilder) build() awslambdaeventsources.SqsEventSource {
	// Create base event source properties
	eventSourceProps := &awslambdaeventsources.SqsEventSourceProps{
		BatchSize:               jsii.Number(esb.config.batchSize),
		ReportBatchItemFailures: jsii.Bool(true),
	}

	// Set batching window for non-FIFO queues
	if !esb.config.fifoQueue {
		eventSourceProps.MaxBatchingWindow = esb.config.maxBatchingWindow
	}

	// Apply user-provided event source properties
	esb.applyUserEventSourceProps(eventSourceProps)

	// Create event source and add to function
	eventSource := awslambdaeventsources.NewSqsEventSource(esb.processor.Queue, eventSourceProps)
	esb.processor.Function.Function.AddEventSource(eventSource)

	return eventSource
}

// applyUserEventSourceProps applies user-provided event source properties
func (esb *sqsEventSourceBuilder) applyUserEventSourceProps(eventSourceProps *awslambdaeventsources.SqsEventSourceProps) {
	if esb.props.EventSourceProps == nil {
		return
	}

	if esb.props.EventSourceProps.BatchSize != nil {
		eventSourceProps.BatchSize = esb.props.EventSourceProps.BatchSize
	}

	// Only set batching window for non-FIFO queues
	if esb.props.EventSourceProps.MaxBatchingWindow != nil && !esb.config.fifoQueue {
		eventSourceProps.MaxBatchingWindow = esb.props.EventSourceProps.MaxBatchingWindow
	}

	if esb.props.EventSourceProps.ReportBatchItemFailures != nil {
		eventSourceProps.ReportBatchItemFailures = esb.props.EventSourceProps.ReportBatchItemFailures
	}
	if esb.props.EventSourceProps.MaxConcurrency != nil {
		eventSourceProps.MaxConcurrency = esb.props.EventSourceProps.MaxConcurrency
	}
	if esb.props.EventSourceProps.Enabled != nil {
		eventSourceProps.Enabled = esb.props.EventSourceProps.Enabled
	}
	if esb.props.EventSourceProps.Filters != nil {
		eventSourceProps.Filters = esb.props.EventSourceProps.Filters
	}
}

// enableMonitoring adds CloudWatch alarms and metrics for the SQS processor
func (s *SQSProcessor) enableMonitoring() {
	// Create SNS topic for alerts
	_ = awssns.NewTopic(s, jsii.String("AlarmTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(fmt.Sprintf("%s-alarms", *s.Queue.QueueName())),
		DisplayName: jsii.String(fmt.Sprintf("Alarms for %s", *s.Queue.QueueName())),
	})

	// Queue depth alarm - warns when messages accumulate
	awscloudwatch.NewAlarm(s, jsii.String("QueueDepthAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-queue-depth", *s.Queue.QueueName())),
		AlarmDescription: jsii.String("Queue depth is too high"),
		Metric: s.Queue.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(1000),
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	// Message age alarm - warns when messages are not processed quickly
	awscloudwatch.NewAlarm(s, jsii.String("MessageAgeAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-message-age", *s.Queue.QueueName())),
		AlarmDescription: jsii.String("Messages are aging in queue"),
		Metric: s.Queue.MetricApproximateAgeOfOldestMessage(&awscloudwatch.MetricOptions{
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(300), // 5 minutes
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	// DLQ message count alarm
	if s.DeadLetterQueue != nil {
		awscloudwatch.NewAlarm(s, jsii.String("DLQMessagesAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-dlq-messages", *s.Queue.QueueName())),
			AlarmDescription: jsii.String("Messages in dead letter queue"),
			Metric: s.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// Lambda function monitoring
	if s.Function != nil && s.Function.Function != nil {
		function := s.Function.Function

		// Function error rate alarm
		awscloudwatch.NewAlarm(s, jsii.String("FunctionErrorAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-processor-errors", *s.Queue.QueueName())),
			AlarmDescription: jsii.String("SQS processor function errors"),
			Metric: function.MetricErrors(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(5),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// Function throttles alarm
		awscloudwatch.NewAlarm(s, jsii.String("FunctionThrottleAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-processor-throttles", *s.Queue.QueueName())),
			AlarmDescription: jsii.String("SQS processor function throttled"),
			Metric: function.MetricThrottles(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// Function duration alarm
		awscloudwatch.NewAlarm(s, jsii.String("FunctionDurationAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-processor-duration", *s.Queue.QueueName())),
			AlarmDescription: jsii.String("SQS processor taking too long"),
			Metric: function.MetricDuration(&awscloudwatch.MetricOptions{
				Period:    awscdk.Duration_Minutes(jsii.Number(5)),
				Statistic: awscloudwatch.Stats_AVERAGE(),
			}),
			Threshold:          jsii.Number(30000), // 30 seconds
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// Queue in-flight messages alarm
	awscloudwatch.NewAlarm(s, jsii.String("InFlightMessagesAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-in-flight-messages", *s.Queue.QueueName())),
		AlarmDescription: jsii.String("Too many messages in flight"),
		Metric: s.Queue.MetricApproximateNumberOfMessagesNotVisible(&awscloudwatch.MetricOptions{
			Period: awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(5000),
		EvaluationPeriods:  jsii.Number(2),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})

	// Create CloudWatch dashboard
	dashboard := awscloudwatch.NewDashboard(s, jsii.String("ProcessorDashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-processor-dashboard", *s.Queue.QueueName())),
	})

	// Add widgets to dashboard
	dashboard.AddWidgets(
		awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
			Title: jsii.String("Queue Metrics"),
			Left: &[]awscloudwatch.IMetric{
				s.Queue.MetricApproximateNumberOfMessagesVisible(nil),
				s.Queue.MetricApproximateNumberOfMessagesNotVisible(nil),
			},
			Right: &[]awscloudwatch.IMetric{
				s.Queue.MetricApproximateAgeOfOldestMessage(nil),
			},
		}),
	)

	if s.DeadLetterQueue != nil {
		dashboard.AddWidgets(
			awscloudwatch.NewSingleValueWidget(&awscloudwatch.SingleValueWidgetProps{
				Title: jsii.String("Dead Letter Queue"),
				Metrics: &[]awscloudwatch.IMetric{
					s.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(nil),
				},
			}),
		)
	}

	if s.Function != nil && s.Function.Function != nil {
		dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("Lambda Function Metrics"),
				Left: &[]awscloudwatch.IMetric{
					s.Function.Function.MetricInvocations(nil),
					s.Function.Function.MetricErrors(nil),
					s.Function.Function.MetricThrottles(nil),
				},
				Right: &[]awscloudwatch.IMetric{
					s.Function.Function.MetricDuration(nil),
				},
			}),
		)
	}
}

// GrantSendMessages grants permission to send messages to the queue
func (s *SQSProcessor) GrantSendMessages(grantee awslambda.IFunction) {
	s.Queue.GrantSendMessages(grantee)
}

// GrantConsumeMessages grants permission to consume messages from the queue
func (s *SQSProcessor) GrantConsumeMessages(grantee awslambda.IFunction) {
	s.Queue.GrantConsumeMessages(grantee)
}

// AddEnvironmentVariable adds an environment variable to the Lambda function
func (s *SQSProcessor) AddEnvironmentVariable(key string, value string) {
	s.Function.Function.AddEnvironment(jsii.String(key), jsii.String(value), nil)
}

// GetQueueName returns the queue name
func (s *SQSProcessor) GetQueueName() *string {
	return s.Queue.QueueName()
}

// GetQueueUrl returns the queue URL
func (s *SQSProcessor) GetQueueUrl() *string {
	return s.Queue.QueueUrl()
}

// GetQueueArn returns the queue ARN
func (s *SQSProcessor) GetQueueArn() *string {
	return s.Queue.QueueArn()
}
