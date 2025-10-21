package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// SNSProcessorProps defines the properties for creating an SNS processor
type SNSProcessorProps struct {
	// The Lambda function configuration
	FunctionProps *LiftFunctionProps `field:"required"`

	// Optional: Topic configuration
	TopicProps *awssns.TopicProps `field:"optional"`

	// Optional: Use an existing topic instead of creating a new one
	ExistingTopic awssns.ITopic `field:"optional"`

	// Optional: SNS subscription configuration
	SubscriptionProps *awslambdaeventsources.SnsEventSourceProps `field:"optional"`

	// Optional: Enable dead letter queue for failed messages
	EnableDLQ *bool `field:"optional"`

	// Optional: DLQ configuration
	DLQProps *awssqs.QueueProps `field:"optional"`

	// Optional: Message filtering policy
	FilterPolicy *map[string]awssns.SubscriptionFilter `field:"optional"`

	// Optional: Enable FIFO topic
	EnableFifo *bool `field:"optional"`

	// Optional: Enable content-based deduplication
	ContentBasedDeduplication *bool `field:"optional"`

	// Optional: Message retention period in seconds (1 hour to 14 days)
	MessageRetentionSeconds *float64 `field:"optional"`

	// Optional: Display name for the topic
	DisplayName *string `field:"optional"`

	// Optional: Subscription protocol (defaults to lambda)
	Protocol *string `field:"optional"`

	// Optional: Raw message delivery
	RawMessageDelivery *bool `field:"optional"`
}

// SNSProcessor creates an SNS topic with Lambda processor and optional DLQ
type SNSProcessor struct {
	constructs.Construct
	Topic    awssns.ITopic
	Function LiftFunction
	DLQ      awssqs.IQueue
}

// NewSNSProcessor creates a new SNS processor with Lambda function
func NewSNSProcessor(scope constructs.Construct, id *string, props *SNSProcessorProps) *SNSProcessor {
	builder := newSNSProcessorBuilder(scope, id, props)
	return builder.build()
}

// snsProcessorBuilder builds SNS processor infrastructure
type snsProcessorBuilder struct {
	scope        constructs.Construct
	id           *string
	props        *SNSProcessorProps
	construct    constructs.Construct
	topic        awssns.ITopic
	function     *LiftFunction
	dlq          awssqs.IQueue
	subscription *awslambdaeventsources.SnsEventSourceProps
}

// newSNSProcessorBuilder creates a new SNS processor builder
func newSNSProcessorBuilder(scope constructs.Construct, id *string, props *SNSProcessorProps) *snsProcessorBuilder {
	return &snsProcessorBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete SNS processor
func (b *snsProcessorBuilder) build() *SNSProcessor {
	b.construct = constructs.NewConstruct(b.scope, b.id)

	b.setupTopic()
	b.createFunction()
	b.configureEnvironment()
	b.setupDLQ()
	b.configureSubscription()
	b.grantPermissions()

	return &SNSProcessor{
		Construct: b.construct,
		Topic:     b.topic,
		Function:  *b.function,
		DLQ:       b.dlq,
	}
}

// setupTopic creates or uses existing SNS topic
func (b *snsProcessorBuilder) setupTopic() {
	if b.props.ExistingTopic != nil {
		b.topic = b.props.ExistingTopic
		return
	}

	b.topic = b.createNewTopic()
}

// createNewTopic creates a new SNS topic
func (b *snsProcessorBuilder) createNewTopic() awssns.ITopic {
	topicProps := b.getTopicProps()
	b.applyDisplayName(topicProps)
	b.applyFifoConfig(topicProps)

	return awssns.NewTopic(b.construct, jsii.String("Topic"), topicProps)
}

// getTopicProps gets or creates topic properties
func (b *snsProcessorBuilder) getTopicProps() *awssns.TopicProps {
	if b.props.TopicProps != nil {
		return b.props.TopicProps
	}
	return &awssns.TopicProps{}
}

// applyDisplayName sets the display name if provided
func (b *snsProcessorBuilder) applyDisplayName(topicProps *awssns.TopicProps) {
	if topicProps.DisplayName == nil && b.props.DisplayName != nil {
		topicProps.DisplayName = b.props.DisplayName
	}
}

// applyFifoConfig applies FIFO configuration if enabled
func (b *snsProcessorBuilder) applyFifoConfig(topicProps *awssns.TopicProps) {
	if b.props.EnableFifo == nil || !*b.props.EnableFifo {
		return
	}

	topicProps.Fifo = jsii.Bool(true)
	if b.props.ContentBasedDeduplication != nil {
		topicProps.ContentBasedDeduplication = b.props.ContentBasedDeduplication
	}
}

// createFunction creates the Lambda function
func (b *snsProcessorBuilder) createFunction() {
	b.function = NewLiftFunction(b.construct, jsii.String("Function"), b.props.FunctionProps)
}

// configureEnvironment sets up environment variables
func (b *snsProcessorBuilder) configureEnvironment() {
	b.function.Function.AddEnvironment(jsii.String("SNS_TOPIC_ARN"), b.topic.TopicArn(), nil)
	b.function.Function.AddEnvironment(jsii.String("SNS_TOPIC_NAME"), b.topic.TopicName(), nil)
}

// setupDLQ creates dead letter queue if enabled
func (b *snsProcessorBuilder) setupDLQ() {
	if !b.isDLQEnabled() {
		return
	}

	dlqProps := b.getDLQProps()
	b.ensureFifoDLQ(dlqProps)

	b.dlq = awssqs.NewQueue(b.construct, jsii.String("DLQ"), dlqProps)
	b.function.Function.AddEnvironment(jsii.String("SNS_DLQ_URL"), b.dlq.QueueUrl(), nil)
}

// isDLQEnabled checks if DLQ should be created
func (b *snsProcessorBuilder) isDLQEnabled() bool {
	if b.props.EnableDLQ == nil {
		return true // Default to enabled
	}
	return *b.props.EnableDLQ
}

// getDLQProps gets or creates DLQ properties
func (b *snsProcessorBuilder) getDLQProps() *awssqs.QueueProps {
	if b.props.DLQProps != nil {
		return b.props.DLQProps
	}
	return &awssqs.QueueProps{
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
	}
}

// ensureFifoDLQ ensures DLQ is FIFO for FIFO topics
func (b *snsProcessorBuilder) ensureFifoDLQ(dlqProps *awssqs.QueueProps) {
	if b.props.EnableFifo != nil && *b.props.EnableFifo {
		dlqProps.Fifo = jsii.Bool(true)
	}
}

// configureSubscription sets up SNS subscription
func (b *snsProcessorBuilder) configureSubscription() {
	b.subscription = b.getSubscriptionProps()
	b.applyFilterPolicy()
	b.applyDLQToSubscription()

	eventSource := awslambdaeventsources.NewSnsEventSource(b.topic, b.subscription)
	b.function.Function.AddEventSource(eventSource)
}

// getSubscriptionProps gets or creates subscription properties
func (b *snsProcessorBuilder) getSubscriptionProps() *awslambdaeventsources.SnsEventSourceProps {
	if b.props.SubscriptionProps != nil {
		return b.props.SubscriptionProps
	}
	return &awslambdaeventsources.SnsEventSourceProps{}
}

// applyFilterPolicy sets filter policy if provided
func (b *snsProcessorBuilder) applyFilterPolicy() {
	if b.props.FilterPolicy != nil {
		b.subscription.FilterPolicy = b.props.FilterPolicy
	}
}

// applyDLQToSubscription adds DLQ to subscription if available
func (b *snsProcessorBuilder) applyDLQToSubscription() {
	if b.dlq != nil {
		b.subscription.DeadLetterQueue = b.dlq
	}
}

// grantPermissions grants necessary permissions
func (b *snsProcessorBuilder) grantPermissions() {
	b.topic.GrantPublish(b.function.Function.GrantPrincipal())
}

// GrantPublish grants SNS publish permissions to a principal
func (s *SNSProcessor) GrantPublish(grantee awsiam.IGrantable) awsiam.Grant {
	return s.Topic.GrantPublish(grantee)
}

// GrantSubscribe grants SNS subscribe permissions to a principal
func (s *SNSProcessor) GrantSubscribe(grantee awsiam.IGrantable) awsiam.Grant {
	return s.Topic.GrantSubscribe(grantee)
}

// AddSubscription adds a new subscription to the topic
func (s *SNSProcessor) AddSubscription(subscription awssns.ITopicSubscription) awssns.Subscription {
	return s.Topic.AddSubscription(subscription)
}

// GetTopicArn returns the SNS topic ARN
func (s *SNSProcessor) GetTopicArn() *string {
	return s.Topic.TopicArn()
}

// GetTopicName returns the SNS topic name
func (s *SNSProcessor) GetTopicName() *string {
	return s.Topic.TopicName()
}

// GetDLQUrl returns the DLQ URL if DLQ is enabled
func (s *SNSProcessor) GetDLQUrl() *string {
	if s.DLQ != nil {
		return s.DLQ.QueueUrl()
	}
	return nil
}
