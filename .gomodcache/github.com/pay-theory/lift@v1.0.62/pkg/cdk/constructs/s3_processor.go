package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3notifications"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// S3ProcessorProps defines properties for an S3 processor
type S3ProcessorProps struct {
	// Lambda function properties
	FunctionProps awslambda.FunctionProps

	// S3 bucket properties (optional - creates new bucket if not provided)
	BucketProps *awss3.BucketProps

	// Existing bucket to use (optional - creates new if not provided)
	ExistingBucket awss3.IBucket

	// S3 event types to process (default: ObjectCreated)
	EventTypes *[]awss3.EventType

	// Key prefix filter for S3 events (optional)
	KeyPrefix *string

	// Key suffix filter for S3 events (optional)
	KeySuffix *string

	// Dead letter queue properties (optional)
	DeadLetterQueueProps *awssqs.QueueProps

	// Enable dead letter queue (default: true)
	EnableDeadLetterQueue *bool

	// S3 event source configuration
	EventSourceProps *awslambdaeventsources.S3EventSourceProps

	// Additional S3 processor settings
	BatchSize         *float64        // Default: 10
	MaxBatchingWindow awscdk.Duration // Default: 5 seconds

	// Multi-region support
	CrossRegionReplication *bool
	ReplicationBucket      awss3.IBucket

	// Lifecycle rules
	EnableLifecycleRules *bool
	LifecycleRules       *[]*awss3.LifecycleRule

	// External bucket support
	ExternalBucket awss3.IBucket

	// Event filtering
	EventFilter *S3EventFilter

	// Access logging
	EnableAccessLogging *bool
	AccessLogsBucket    awss3.IBucket
	AccessLogsPrefix    *string

	// Versioning and backup
	EnableVersioning *bool
	EnableBackup     *bool

	// Lift-specific settings
	EnableTracing     *bool
	EnableMultiTenant *bool
	EnableMonitoring  *bool
}

// S3EventFilter defines event filtering options
type S3EventFilter struct {
	Prefix *string
	Suffix *string
}

// S3Processor represents an S3 bucket with Lambda processor
type S3Processor struct {
	constructs.Construct

	// The Lambda function processing S3 events
	Function *LiftFunction

	// The S3 bucket
	Bucket awss3.IBucket

	// Dead letter queue (if enabled)
	DeadLetterQueue awssqs.IQueue

	// Event source mapping
	EventSource awslambdaeventsources.S3EventSource

	// Replication bucket (if cross-region replication is enabled)
	ReplicationBucket awss3.IBucket
}

// NewS3Processor creates a new S3 processor construct
func NewS3Processor(scope constructs.Construct, id *string, props *S3ProcessorProps) *S3Processor {
	this := &S3Processor{}
	constructs.NewConstruct_Override(this, scope, id)

	// Set defaults
	if props == nil {
		props = &S3ProcessorProps{}
	}

	builder := newS3ProcessorBuilder(this, props)
	return builder.build()
}

// s3ProcessorBuilder builds S3 processor components
type s3ProcessorBuilder struct {
	processor *S3Processor
	props     *S3ProcessorProps
	config    *s3ProcessorConfig
}

// s3ProcessorConfig holds resolved configuration values
// Memory optimized: struct with 16 pointer bytes could be 8
type s3ProcessorConfig struct {
	// Slice first (24 bytes - pointer + len + cap)
	eventTypes []awss3.EventType
	// Booleans (1 byte each, packed together)
	enableDLQ            bool
	enableVersioning     bool
	enableLifecycleRules bool
	enableAccessLogging  bool
}

// newS3ProcessorBuilder creates a new S3 processor builder
func newS3ProcessorBuilder(processor *S3Processor, props *S3ProcessorProps) *s3ProcessorBuilder {
	return &s3ProcessorBuilder{
		processor: processor,
		props:     props,
		config:    buildS3ProcessorConfig(props),
	}
}

// buildS3ProcessorConfig resolves configuration values with defaults
func buildS3ProcessorConfig(props *S3ProcessorProps) *s3ProcessorConfig {
	config := &s3ProcessorConfig{
		enableDLQ:            true,
		enableVersioning:     false,
		enableLifecycleRules: false,
		enableAccessLogging:  false,
		eventTypes:           []awss3.EventType{awss3.EventType_OBJECT_CREATED},
	}

	// Apply provided values
	if props.EnableDeadLetterQueue != nil {
		config.enableDLQ = *props.EnableDeadLetterQueue
	}
	if props.EnableVersioning != nil {
		config.enableVersioning = *props.EnableVersioning
	}
	if props.EnableLifecycleRules != nil {
		config.enableLifecycleRules = *props.EnableLifecycleRules
	}
	if props.EnableAccessLogging != nil {
		config.enableAccessLogging = *props.EnableAccessLogging
	}
	if props.EventTypes != nil {
		config.eventTypes = *props.EventTypes
	}

	return config
}

// build constructs the complete S3 processor
func (b *s3ProcessorBuilder) build() *S3Processor {
	// Create or use existing bucket
	b.setupBucket()

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

// setupBucket creates or configures the S3 bucket
func (b *s3ProcessorBuilder) setupBucket() {
	if b.props.ExistingBucket != nil {
		b.processor.Bucket = b.props.ExistingBucket
		return
	}

	bucketBuilder := newS3BucketBuilder(b.processor, b.props, b.config)
	b.processor.Bucket, b.processor.ReplicationBucket = bucketBuilder.build()
}

// setupDeadLetterQueue creates the dead letter queue if enabled
func (b *s3ProcessorBuilder) setupDeadLetterQueue() {
	if !b.config.enableDLQ {
		return
	}

	dlqBuilder := newDeadLetterQueueBuilder(
		b.processor,
		b.props.DeadLetterQueueProps,
		b.props.FunctionProps.FunctionName,
		"-s3-dlq",
	)
	b.processor.DeadLetterQueue = dlqBuilder.build()
}

// setupFunction creates the Lambda function with S3 environment variables
func (b *s3ProcessorBuilder) setupFunction() {
	functionBuilder := newS3FunctionBuilder(b.processor, b.props)
	b.processor.Function = functionBuilder.build()
}

// setupEventSource configures the S3 event source
func (b *s3ProcessorBuilder) setupEventSource() {
	eventSourceBuilder := newS3EventSourceBuilder(b.processor, b.props, b.config)
	b.processor.EventSource = eventSourceBuilder.build()
}

// setupPermissions grants necessary permissions
func (b *s3ProcessorBuilder) setupPermissions() {
	b.processor.Bucket.GrantRead(b.processor.Function.Function, jsii.String("*"))
	b.processor.Bucket.GrantWrite(b.processor.Function.Function, jsii.String("*"), nil)
	if b.processor.DeadLetterQueue != nil {
		b.processor.DeadLetterQueue.GrantSendMessages(b.processor.Function.Function)
	}
	if b.processor.ReplicationBucket != nil {
		b.processor.ReplicationBucket.GrantWrite(b.processor.Function.Function, jsii.String("*"), nil)
	}
}

// setupMonitoring adds monitoring if enabled
func (b *s3ProcessorBuilder) setupMonitoring() {
	if b.props.EnableMonitoring != nil && *b.props.EnableMonitoring {
		b.processor.enableMonitoring()
	}
}

// s3BucketBuilder builds S3 bucket components
type s3BucketBuilder struct {
	processor *S3Processor
	props     *S3ProcessorProps
	config    *s3ProcessorConfig
}

// newS3BucketBuilder creates a new S3 bucket builder
func newS3BucketBuilder(processor *S3Processor, props *S3ProcessorProps, config *s3ProcessorConfig) *s3BucketBuilder {
	return &s3BucketBuilder{
		processor: processor,
		props:     props,
		config:    config,
	}
}

// build creates the main bucket and optional replication bucket
func (bb *s3BucketBuilder) build() (awss3.IBucket, awss3.IBucket) {
	// Create main bucket
	mainBucket := bb.createMainBucket()

	// Create replication bucket if needed
	var replicationBucket awss3.IBucket
	if bb.shouldEnableReplication() {
		replicationBucket = bb.props.ReplicationBucket
		bb.enableCrossRegionReplication(mainBucket)
	}

	return mainBucket, replicationBucket
}

// createMainBucket creates the main S3 bucket
func (bb *s3BucketBuilder) createMainBucket() awss3.IBucket {
	bucketProps := bb.createBaseBucketProps()

	// Apply user-provided bucket props
	bb.applyUserBucketProps(bucketProps)

	// Set default bucket name if needed
	bb.setDefaultBucketName(bucketProps)

	// Configure access logging
	bb.configureAccessLogging(bucketProps)

	// Configure lifecycle rules
	bb.configureLifecycleRules(bucketProps)

	return awss3.NewBucket(bb.processor, jsii.String("Bucket"), bucketProps)
}

// createBaseBucketProps creates base bucket properties with security defaults
func (bb *s3BucketBuilder) createBaseBucketProps() *awss3.BucketProps {
	return &awss3.BucketProps{
		Versioned:          jsii.Bool(bb.config.enableVersioning),
		BlockPublicAccess:  awss3.BlockPublicAccess_BLOCK_ALL(),
		Encryption:         awss3.BucketEncryption_S3_MANAGED,
		EnforceSSL:         jsii.Bool(true),
		EventBridgeEnabled: jsii.Bool(true),
	}
}

// applyUserBucketProps applies user-provided bucket properties
func (bb *s3BucketBuilder) applyUserBucketProps(bucketProps *awss3.BucketProps) {
	if bb.props.BucketProps == nil {
		return
	}
	applyNonNilStructFields(bucketProps, bb.props.BucketProps)
}

// setDefaultBucketName sets a default bucket name if none provided
func (bb *s3BucketBuilder) setDefaultBucketName(bucketProps *awss3.BucketProps) {
	if bucketProps.BucketName != nil || bb.props.FunctionProps.FunctionName == nil {
		return
	}
	bucketProps.BucketName = jsii.String(*bb.props.FunctionProps.FunctionName + "-bucket")
}

// configureAccessLogging configures S3 access logging if enabled
func (bb *s3BucketBuilder) configureAccessLogging(bucketProps *awss3.BucketProps) {
	if !bb.config.enableAccessLogging || bb.props.AccessLogsBucket == nil {
		return
	}

	bucketProps.ServerAccessLogsBucket = bb.props.AccessLogsBucket
	if bb.props.AccessLogsPrefix != nil {
		bucketProps.ServerAccessLogsPrefix = bb.props.AccessLogsPrefix
	}
}

// configureLifecycleRules configures S3 lifecycle rules if enabled
func (bb *s3BucketBuilder) configureLifecycleRules(bucketProps *awss3.BucketProps) {
	if !bb.config.enableLifecycleRules {
		return
	}

	if bb.props.LifecycleRules != nil {
		bucketProps.LifecycleRules = bb.props.LifecycleRules
	} else {
		bucketProps.LifecycleRules = bb.createDefaultLifecycleRules()
	}
}

// createDefaultLifecycleRules creates default lifecycle rules
func (bb *s3BucketBuilder) createDefaultLifecycleRules() *[]*awss3.LifecycleRule {
	defaultRules := []*awss3.LifecycleRule{
		{
			Id:                                  jsii.String("DeleteIncompleteMultipartUploads"),
			Enabled:                             jsii.Bool(true),
			AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(1)),
		},
		{
			Id:      jsii.String("TransitionToIA"),
			Enabled: jsii.Bool(true),
			Transitions: &[]*awss3.Transition{
				{
					StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
					TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
				},
			},
		},
	}
	return &defaultRules
}

// shouldEnableReplication checks if cross-region replication should be enabled
func (bb *s3BucketBuilder) shouldEnableReplication() bool {
	return bb.props.CrossRegionReplication != nil && *bb.props.CrossRegionReplication && bb.props.ReplicationBucket != nil
}

// enableCrossRegionReplication enables cross-region replication
func (bb *s3BucketBuilder) enableCrossRegionReplication(_ awss3.IBucket) {
	bb.processor.enableCrossRegionReplication()
}

// s3FunctionBuilder builds Lambda function components
type s3FunctionBuilder struct {
	processor *S3Processor
	props     *S3ProcessorProps
}

// newS3FunctionBuilder creates a new S3 function builder
func newS3FunctionBuilder(processor *S3Processor, props *S3ProcessorProps) *s3FunctionBuilder {
	return &s3FunctionBuilder{
		processor: processor,
		props:     props,
	}
}

// build creates the Lambda function with S3 environment variables
func (fb *s3FunctionBuilder) build() *LiftFunction {
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
func (fb *s3FunctionBuilder) prepareFunctionEnvironment() map[string]*string {
	functionEnv := make(map[string]*string)

	// Copy existing environment variables
	if fb.props.FunctionProps.Environment != nil {
		for k, v := range *fb.props.FunctionProps.Environment {
			functionEnv[k] = v
		}
	}

	// Add S3-specific environment variables
	functionEnv["S3_BUCKET_NAME"] = fb.processor.Bucket.BucketName()
	functionEnv["S3_BUCKET_ARN"] = fb.processor.Bucket.BucketArn()
	if fb.processor.DeadLetterQueue != nil {
		functionEnv["S3_DLQ_URL"] = fb.processor.DeadLetterQueue.QueueUrl()
	}
	if fb.processor.ReplicationBucket != nil {
		functionEnv["S3_REPLICATION_BUCKET_NAME"] = fb.processor.ReplicationBucket.BucketName()
	}

	return functionEnv
}

// s3EventSourceBuilder builds S3 event source components
type s3EventSourceBuilder struct {
	processor *S3Processor
	props     *S3ProcessorProps
	config    *s3ProcessorConfig
}

// newS3EventSourceBuilder creates a new S3 event source builder
func newS3EventSourceBuilder(processor *S3Processor, props *S3ProcessorProps, config *s3ProcessorConfig) *s3EventSourceBuilder {
	return &s3EventSourceBuilder{
		processor: processor,
		props:     props,
		config:    config,
	}
}

// build creates and configures the S3 event source
func (esb *s3EventSourceBuilder) build() awslambdaeventsources.S3EventSource {
	// Create base event source properties
	eventSourceProps := &awslambdaeventsources.S3EventSourceProps{
		Events: &esb.config.eventTypes,
	}

	// Add key filters if provided
	esb.addKeyFilters(eventSourceProps)

	// Apply user-provided event source properties
	esb.applyUserEventSourceProps(eventSourceProps)

	// Create event source
	return esb.createEventSource(eventSourceProps)
}

// addKeyFilters adds key prefix and suffix filters
func (esb *s3EventSourceBuilder) addKeyFilters(eventSourceProps *awslambdaeventsources.S3EventSourceProps) {
	filters := []*awss3.NotificationKeyFilter{}

	if esb.props.KeyPrefix != nil {
		filters = append(filters, &awss3.NotificationKeyFilter{
			Prefix: esb.props.KeyPrefix,
		})
	}
	if esb.props.KeySuffix != nil {
		filters = append(filters, &awss3.NotificationKeyFilter{
			Suffix: esb.props.KeySuffix,
		})
	}

	if len(filters) > 0 {
		eventSourceProps.Filters = &filters
	}
}

// applyUserEventSourceProps applies user-provided event source properties
func (esb *s3EventSourceBuilder) applyUserEventSourceProps(eventSourceProps *awslambdaeventsources.S3EventSourceProps) {
	if esb.props.EventSourceProps == nil {
		return
	}

	if esb.props.EventSourceProps.Events != nil {
		eventSourceProps.Events = esb.props.EventSourceProps.Events
	}
	if esb.props.EventSourceProps.Filters != nil {
		eventSourceProps.Filters = esb.props.EventSourceProps.Filters
	}
}

// createEventSource creates the appropriate event source based on bucket type
func (esb *s3EventSourceBuilder) createEventSource(eventSourceProps *awslambdaeventsources.S3EventSourceProps) awslambdaeventsources.S3EventSource {
	if bucket, ok := esb.processor.Bucket.(awss3.Bucket); ok {
		eventSource := awslambdaeventsources.NewS3EventSource(bucket, eventSourceProps)
		esb.processor.Function.Function.AddEventSource(eventSource)
		return eventSource
	} else if esb.props.ExternalBucket != nil {
		// Handle external bucket notifications
		esb.configureExternalBucketNotification()
		return nil // External buckets don't return S3EventSource
	}
	return nil
}

// configureExternalBucketNotification configures notifications for external buckets
func (esb *s3EventSourceBuilder) configureExternalBucketNotification() {
	if esb.props.ExternalBucket == nil {
		return
	}

	esb.props.ExternalBucket.AddEventNotification(
		awss3.EventType_OBJECT_CREATED,
		awss3notifications.NewLambdaDestination(esb.processor.Function.Function),
		&awss3.NotificationKeyFilter{
			Prefix: esb.props.EventFilter.Prefix,
			Suffix: esb.props.EventFilter.Suffix,
		},
	)
}

// enableMonitoring adds CloudWatch alarms and metrics for the S3 processor
func (s *S3Processor) enableMonitoring() {
	// Create SNS topic for alerts
	_ = awssns.NewTopic(s, jsii.String("AlarmTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(fmt.Sprintf("%s-alarms", *s.Bucket.BucketName())),
		DisplayName: jsii.String(fmt.Sprintf("Alarms for %s processor", *s.Bucket.BucketName())),
	})

	// Lambda function monitoring
	if s.Function != nil {
		EnableS3LambdaMonitoring(s, s.Bucket.BucketName(), s.Function.Function)
	}

	// S3 bucket metrics
	var client4xxErrorsMetric awscloudwatch.IMetric
	var server5xxErrorsMetric awscloudwatch.IMetric
	var objectSizeMetric awscloudwatch.IMetric
	var objectCountMetric awscloudwatch.IMetric

	if s.Bucket != nil {
		// 4xx errors alarm
		client4xxErrorsMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/S3"),
			MetricName: jsii.String("4xxErrors"),
			DimensionsMap: &map[string]*string{
				"BucketName": s.Bucket.BucketName(),
			},
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			Statistic: awscloudwatch.Stats_SUM(),
		})

		awscloudwatch.NewAlarm(s, jsii.String("Bucket4xxErrorsAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-4xx-errors", *s.Bucket.BucketName())),
			AlarmDescription:   jsii.String("S3 bucket 4xx errors"),
			Metric:             client4xxErrorsMetric,
			Threshold:          jsii.Number(10),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// 5xx errors alarm
		server5xxErrorsMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/S3"),
			MetricName: jsii.String("5xxErrors"),
			DimensionsMap: &map[string]*string{
				"BucketName": s.Bucket.BucketName(),
			},
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			Statistic: awscloudwatch.Stats_SUM(),
		})

		awscloudwatch.NewAlarm(s, jsii.String("Bucket5xxErrorsAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-5xx-errors", *s.Bucket.BucketName())),
			AlarmDescription:   jsii.String("S3 bucket 5xx errors"),
			Metric:             server5xxErrorsMetric,
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// Object size monitoring (for large file detection)
		objectSizeMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/S3"),
			MetricName: jsii.String("BucketSizeBytes"),
			DimensionsMap: &map[string]*string{
				"BucketName":  s.Bucket.BucketName(),
				"StorageType": jsii.String("StandardStorage"),
			},
			Period:    awscdk.Duration_Days(jsii.Number(1)),
			Statistic: awscloudwatch.Stats_AVERAGE(),
		})

		awscloudwatch.NewAlarm(s, jsii.String("BucketSizeAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-bucket-size", *s.Bucket.BucketName())),
			AlarmDescription:   jsii.String("S3 bucket size is large"),
			Metric:             objectSizeMetric,
			Threshold:          jsii.Number(100 * 1024 * 1024 * 1024), // 100 GB
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// Number of objects monitoring
		objectCountMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/S3"),
			MetricName: jsii.String("NumberOfObjects"),
			DimensionsMap: &map[string]*string{
				"BucketName":  s.Bucket.BucketName(),
				"StorageType": jsii.String("AllStorageTypes"),
			},
			Period:    awscdk.Duration_Days(jsii.Number(1)),
			Statistic: awscloudwatch.Stats_AVERAGE(),
		})

		awscloudwatch.NewAlarm(s, jsii.String("ObjectCountAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-object-count", *s.Bucket.BucketName())),
			AlarmDescription:   jsii.String("S3 bucket has many objects"),
			Metric:             objectCountMetric,
			Threshold:          jsii.Number(1000000), // 1 million objects
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// DLQ monitoring if enabled
	if s.DeadLetterQueue != nil {
		awscloudwatch.NewAlarm(s, jsii.String("DLQMessagesAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-processor-dlq-messages", *s.Bucket.BucketName())),
			AlarmDescription: jsii.String("Messages in S3 processor dead letter queue"),
			Metric: s.DeadLetterQueue.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(1),
			EvaluationPeriods:  jsii.Number(1),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_OR_EQUAL_TO_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// Create CloudWatch dashboard
	dashboard := awscloudwatch.NewDashboard(s, jsii.String("ProcessorDashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-processor-dashboard", *s.Bucket.BucketName())),
	})

	// Add widgets to dashboard
	if s.Function != nil {
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

	if s.Bucket != nil {
		dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("S3 Bucket Metrics"),
				Left: &[]awscloudwatch.IMetric{
					client4xxErrorsMetric,
					server5xxErrorsMetric,
				},
			}),
			awscloudwatch.NewSingleValueWidget(&awscloudwatch.SingleValueWidgetProps{
				Title: jsii.String("Bucket Storage"),
				Metrics: &[]awscloudwatch.IMetric{
					objectSizeMetric,
					objectCountMetric,
				},
			}),
		)
	}

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
}

// GrantRead grants permission to read from the bucket
func (s *S3Processor) GrantRead(grantee awslambda.IFunction) {
	s.Bucket.GrantRead(grantee, jsii.String("*"))
}

// GrantWrite grants permission to write to the bucket
func (s *S3Processor) GrantWrite(grantee awslambda.IFunction) {
	s.Bucket.GrantWrite(grantee, jsii.String("*"), nil)
}

// GrantReadWrite grants permission to read and write to the bucket
func (s *S3Processor) GrantReadWrite(grantee awslambda.IFunction) {
	s.Bucket.GrantReadWrite(grantee, jsii.String("*"))
}

// GrantDelete grants permission to delete objects from the bucket
func (s *S3Processor) GrantDelete(grantee awslambda.IFunction) {
	s.Bucket.GrantDelete(grantee, jsii.String("*"))
}

// AddEnvironmentVariable adds an environment variable to the Lambda function
func (s *S3Processor) AddEnvironmentVariable(key string, value string) {
	s.Function.Function.AddEnvironment(jsii.String(key), jsii.String(value), nil)
}

// GetBucketName returns the bucket name
func (s *S3Processor) GetBucketName() *string {
	return s.Bucket.BucketName()
}

// GetBucketArn returns the bucket ARN
func (s *S3Processor) GetBucketArn() *string {
	return s.Bucket.BucketArn()
}

// GetBucketDomainName returns the bucket domain name
func (s *S3Processor) GetBucketDomainName() *string {
	return s.Bucket.BucketDomainName()
}

// AddCorsRule adds a CORS rule to the bucket
func (s *S3Processor) AddCorsRule(rule *awss3.CorsRule) {
	// Note: CORS rules can only be added to concrete Bucket instances
	if bucket, ok := s.Bucket.(awss3.Bucket); ok {
		bucket.AddCorsRule(rule)
	}
}

// EnableCORS enables CORS on the bucket
func (s *S3Processor) EnableCORS(rules []*awss3.CorsRule) {
	// Note: CORS rules need to be set during bucket creation
	// This is a helper method for documentation purposes
	// In practice, CORS should be configured in BucketProps
	for _, rule := range rules {
		s.AddCorsRule(rule)
	}
}

// SetBucketPolicy sets a bucket policy
func (s *S3Processor) SetBucketPolicy(policy map[string]interface{}) {
	// Parse statements from the policy map
	if statements, ok := policy["Statement"].([]interface{}); ok {
		for _, stmt := range statements {
			if stmtMap, ok := stmt.(map[string]interface{}); ok {
				policyStatement := s.parsePolicyStatement(stmtMap)

				// Apply each policy statement to the bucket
				if bucket, ok := s.Bucket.(awss3.Bucket); ok {
					bucket.AddToResourcePolicy(policyStatement)
				}
			}
		}
	}
}

// parsePolicyStatement converts a map to PolicyStatement
func (s *S3Processor) parsePolicyStatement(stmt map[string]interface{}) awsiam.PolicyStatement {
	parser := newPolicyStatementParser(stmt)
	return parser.parse()
}

// policyStatementParser parses IAM policy statements from maps
type policyStatementParser struct {
	stmt  map[string]interface{}
	props *awsiam.PolicyStatementProps
}

// newPolicyStatementParser creates a new policy statement parser
func newPolicyStatementParser(stmt map[string]interface{}) *policyStatementParser {
	return &policyStatementParser{
		stmt:  stmt,
		props: &awsiam.PolicyStatementProps{},
	}
}

// parse builds the PolicyStatement from the map
func (p *policyStatementParser) parse() awsiam.PolicyStatement {
	p.parseEffect()
	p.parseActions()
	p.parseResources()
	p.parsePrincipals()
	return awsiam.NewPolicyStatement(p.props)
}

// parseEffect parses the Effect field
func (p *policyStatementParser) parseEffect() {
	if effect, ok := p.stmt["Effect"].(string); ok {
		if effect == "Allow" {
			p.props.Effect = awsiam.Effect_ALLOW
		} else {
			p.props.Effect = awsiam.Effect_DENY
		}
	}
}

// parseActions parses the Action field(s)
func (p *policyStatementParser) parseActions() {
	actions := p.parseStringOrArray("Action")
	if len(actions) > 0 {
		p.props.Actions = &actions
	}
}

// parseResources parses the Resource field(s)
func (p *policyStatementParser) parseResources() {
	resources := p.parseStringOrArray("Resource")
	if len(resources) > 0 {
		p.props.Resources = &resources
	}
}

// parseStringOrArray parses a field that can be either string or string array
func (p *policyStatementParser) parseStringOrArray(field string) []*string {
	var result []*string

	// Check if field is an array
	if items, ok := p.stmt[field].([]interface{}); ok {
		for _, item := range items {
			if str, ok := item.(string); ok {
				result = append(result, jsii.String(str))
			}
		}
	} else if str, ok := p.stmt[field].(string); ok {
		// Field is a single string
		result = append(result, jsii.String(str))
	}

	return result
}

// parsePrincipals parses the Principal field
func (p *policyStatementParser) parsePrincipals() {
	principal, ok := p.stmt["Principal"].(map[string]interface{})
	if !ok {
		return
	}

	var principals []awsiam.IPrincipal

	if aws, ok := principal["AWS"].(string); ok {
		principals = append(principals, awsiam.NewAccountPrincipal(jsii.String(aws)))
	}

	if service, ok := principal["Service"].(string); ok {
		principals = append(principals, awsiam.NewServicePrincipal(jsii.String(service), nil))
	}

	if len(principals) > 0 {
		p.props.Principals = &principals
	}
}

// enableCrossRegionReplication sets up cross-region replication
func (s *S3Processor) enableCrossRegionReplication() {
	// Create replication role
	replicationRole := awsiam.NewRole(s, jsii.String("ReplicationRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("s3.amazonaws.com"), nil),
		Path:      jsii.String("/"),
	})

	// Grant permissions to read from source bucket
	s.Bucket.GrantRead(replicationRole, jsii.String("*"))

	// Grant permissions to replicate to destination bucket
	if s.ReplicationBucket != nil {
		s.ReplicationBucket.GrantWrite(replicationRole, jsii.String("*"), nil)

		// Add replication configuration
		if sourceBucket, ok := s.Bucket.(awss3.Bucket); ok {
			if cfnBucket, ok := sourceBucket.Node().DefaultChild().(awss3.CfnBucket); ok {

				replicationConfig := &awss3.CfnBucket_ReplicationConfigurationProperty{
					Role: replicationRole.RoleArn(),
					Rules: &[]awss3.CfnBucket_ReplicationRuleProperty{
						{
							Id:       jsii.String("ReplicateAll"),
							Status:   jsii.String("Enabled"),
							Priority: jsii.Number(1),
							Filter:   &awss3.CfnBucket_ReplicationRuleFilterProperty{},
							Destination: &awss3.CfnBucket_ReplicationDestinationProperty{
								Bucket:       s.ReplicationBucket.BucketArn(),
								StorageClass: jsii.String("STANDARD_IA"),
							},
						},
					},
				}

				cfnBucket.SetReplicationConfiguration(replicationConfig)
			}
		}
	}
}
