package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// EventBridgeHandlerProps defines properties for an EventBridge handler
type EventBridgeHandlerProps struct {
	// Lambda function properties
	FunctionProps awslambda.FunctionProps

	// Event rule properties (optional - creates new rule if not provided)
	RuleProps *awsevents.RuleProps

	// Existing rule to use (optional - creates new if not provided)
	ExistingRule awsevents.Rule

	// Existing event bus to use (optional - uses default if not provided)
	ExistingEventBus awsevents.IEventBus

	// Event bus properties for creating a custom event bus
	EventBusProps *awsevents.EventBusProps

	// Event pattern for filtering events
	EventPattern *awsevents.EventPattern

	// Schedule expression for scheduled events (conflicts with EventPattern)
	ScheduleExpression *string

	// Lambda target properties
	TargetProps *awseventstargets.LambdaFunctionProps

	// Dead letter queue properties (optional)
	DeadLetterQueueProps *awssqs.QueueProps

	// Enable dead letter queue (default: true)
	EnableDeadLetterQueue *bool

	// Maximum event age in seconds (default: 3600)
	MaxEventAge awscdk.Duration

	// Retry attempts for failed invocations (default: 3)
	RetryAttempts *float64

	// Enable input transformation
	InputTransformation *awsevents.RuleTargetInput

	// Lift-specific settings
	EnableTracing     *bool
	EnableMultiTenant *bool
	EnableMonitoring  *bool

	// Cross-account event bus support
	CrossAccountEventBusArn *string
}

// EventBridgeHandler represents an EventBridge rule with Lambda handler
type EventBridgeHandler struct {
	constructs.Construct

	// The Lambda function handling events
	Function *LiftFunction

	// The EventBridge rule
	Rule awsevents.Rule

	// The event bus (default or custom)
	EventBus awsevents.IEventBus

	// Dead letter queue (if enabled)
	DeadLetterQueue awssqs.IQueue

	// Lambda target
	Target awseventstargets.LambdaFunction
}

// NewEventBridgeHandler creates a new EventBridge handler construct
func NewEventBridgeHandler(scope constructs.Construct, id *string, props *EventBridgeHandlerProps) (*EventBridgeHandler, error) {
	this := &EventBridgeHandler{}
	constructs.NewConstruct_Override(this, scope, id)

	builder := newEventBridgeHandlerBuilder(this, props)
	return builder.build()
}

// eventBridgeHandlerBuilder builds EventBridge handler components
type eventBridgeHandlerBuilder struct {
	handler *EventBridgeHandler
	props   *EventBridgeHandlerProps
	config  *eventBridgeHandlerConfig
}

// eventBridgeHandlerConfig holds resolved configuration values
type eventBridgeHandlerConfig struct {
	maxEventAge   awscdk.Duration
	retryAttempts float64
	enableDLQ     bool
}

// newEventBridgeHandlerBuilder creates a new EventBridge handler builder
func newEventBridgeHandlerBuilder(handler *EventBridgeHandler, props *EventBridgeHandlerProps) *eventBridgeHandlerBuilder {
	return &eventBridgeHandlerBuilder{
		handler: handler,
		props:   props,
		config:  buildEventBridgeHandlerConfig(props),
	}
}

// buildEventBridgeHandlerConfig resolves configuration values with defaults
func buildEventBridgeHandlerConfig(props *EventBridgeHandlerProps) *eventBridgeHandlerConfig {
	if props == nil {
		props = &EventBridgeHandlerProps{}
	}

	config := &eventBridgeHandlerConfig{
		maxEventAge:   awscdk.Duration_Hours(jsii.Number(1)),
		retryAttempts: float64(3),
		enableDLQ:     true,
	}

	// Apply provided values
	if props.MaxEventAge != nil {
		config.maxEventAge = props.MaxEventAge
	}
	if props.RetryAttempts != nil {
		config.retryAttempts = *props.RetryAttempts
	}
	if props.EnableDeadLetterQueue != nil {
		config.enableDLQ = *props.EnableDeadLetterQueue
	}

	return config
}

// build constructs the complete EventBridge handler
func (b *eventBridgeHandlerBuilder) build() (*EventBridgeHandler, error) {
	// Setup event bus
	b.setupEventBus()

	// Setup dead letter queue
	b.setupDeadLetterQueue()

	// Setup Lambda function
	b.setupFunction()

	// Setup EventBridge rule
	if err := b.setupRule(); err != nil {
		return nil, err
	}

	// Setup Lambda target
	b.setupTarget()

	// Grant permissions
	b.setupPermissions()

	// Setup monitoring
	b.setupMonitoring()

	return b.handler, nil
}

// setupEventBus creates or configures the event bus
func (b *eventBridgeHandlerBuilder) setupEventBus() {
	switch {
	case b.props.ExistingEventBus != nil:
		b.handler.EventBus = b.props.ExistingEventBus
	case b.props.EventBusProps != nil:
		b.handler.EventBus = awsevents.NewEventBus(b.handler, jsii.String("EventBus"), b.props.EventBusProps)
	case b.props.CrossAccountEventBusArn != nil:
		// Reference cross-account event bus
		b.handler.EventBus = awsevents.EventBus_FromEventBusArn(b.handler, jsii.String("CrossAccountEventBus"), b.props.CrossAccountEventBusArn)
	default:
		// Use default event bus
		b.handler.EventBus = awsevents.EventBus_FromEventBusName(b.handler, jsii.String("DefaultEventBus"), jsii.String("default"))
	}
}

// setupDeadLetterQueue creates the dead letter queue if enabled
func (b *eventBridgeHandlerBuilder) setupDeadLetterQueue() {
	if !b.config.enableDLQ {
		return
	}

	dlqBuilder := newDeadLetterQueueBuilder(
		b.handler,
		b.props.DeadLetterQueueProps,
		b.props.FunctionProps.FunctionName,
		"-eventbridge-dlq",
	)
	b.handler.DeadLetterQueue = dlqBuilder.build()
}

// setupFunction creates the Lambda function with EventBridge environment variables
func (b *eventBridgeHandlerBuilder) setupFunction() {
	// Create environment variables
	functionEnv := make(map[string]*string)
	if b.props.FunctionProps.Environment != nil {
		for k, v := range *b.props.FunctionProps.Environment {
			functionEnv[k] = v
		}
	}

	// Add EventBridge-specific environment variables
	functionEnv["EVENT_BUS_NAME"] = b.handler.EventBus.EventBusName()
	functionEnv["EVENT_BUS_ARN"] = b.handler.EventBus.EventBusArn()
	if b.handler.DeadLetterQueue != nil {
		functionEnv["EVENTBRIDGE_DLQ_URL"] = b.handler.DeadLetterQueue.QueueUrl()
	}

	// Create LiftFunction with enhanced properties
	liftProps := &LiftFunctionProps{
		FunctionProps: b.props.FunctionProps,
	}

	// Override environment
	liftProps.Environment = &functionEnv

	// Set Lift-specific properties
	if b.props.EnableTracing != nil {
		liftProps.EnableTracing = b.props.EnableTracing
	}
	if b.props.EnableMultiTenant != nil {
		liftProps.EnableMultiTenant = b.props.EnableMultiTenant
	}

	b.handler.Function = NewLiftFunction(b.handler, jsii.String("Function"), liftProps)
}

// setupRule creates or uses existing EventBridge rule
func (b *eventBridgeHandlerBuilder) setupRule() error {
	if b.props.ExistingRule != nil {
		b.handler.Rule = b.props.ExistingRule
		return nil
	}

	// Validate event pattern and schedule
	if b.props.EventPattern != nil && b.props.ScheduleExpression != nil {
		return fmt.Errorf("EventPattern and ScheduleExpression cannot both be specified")
	}

	// Create new rule
	ruleBuilder := newEventBridgeRuleBuilder(b.handler, b.props)
	b.handler.Rule = ruleBuilder.build()

	return nil
}

// setupTarget configures the Lambda target
func (b *eventBridgeHandlerBuilder) setupTarget() {
	targetProps := &awseventstargets.LambdaFunctionProps{
		MaxEventAge:   b.config.maxEventAge,
		RetryAttempts: jsii.Number(b.config.retryAttempts),
	}

	// Add dead letter queue to target if enabled
	if b.handler.DeadLetterQueue != nil {
		targetProps.DeadLetterQueue = b.handler.DeadLetterQueue
	}

	// Override with user-provided target props
	b.applyUserTargetProps(targetProps)

	// Apply input transformation if provided
	if b.props.InputTransformation != nil {
		targetProps.Event = *b.props.InputTransformation
	}

	// Create and add target
	b.handler.Target = awseventstargets.NewLambdaFunction(b.handler.Function.Function, targetProps)
	b.handler.Rule.AddTarget(b.handler.Target)
}

// applyUserTargetProps applies user-provided target properties
func (b *eventBridgeHandlerBuilder) applyUserTargetProps(targetProps *awseventstargets.LambdaFunctionProps) {
	if b.props.TargetProps == nil {
		return
	}
	applyNonNilStructFields(targetProps, b.props.TargetProps)
}

// setupPermissions grants necessary permissions
func (b *eventBridgeHandlerBuilder) setupPermissions() {
	b.handler.EventBus.GrantPutEventsTo(b.handler.Function.Function, jsii.String("eventbridge"))
	if b.handler.DeadLetterQueue != nil {
		b.handler.DeadLetterQueue.GrantSendMessages(b.handler.Function.Function)
	}
}

// setupMonitoring adds monitoring if enabled
func (b *eventBridgeHandlerBuilder) setupMonitoring() {
	if b.props.EnableMonitoring != nil && *b.props.EnableMonitoring {
		b.handler.enableMonitoring()
	}
}

// eventBridgeRuleBuilder builds EventBridge rule components
type eventBridgeRuleBuilder struct {
	handler *EventBridgeHandler
	props   *EventBridgeHandlerProps
}

// newEventBridgeRuleBuilder creates a new EventBridge rule builder
func newEventBridgeRuleBuilder(handler *EventBridgeHandler, props *EventBridgeHandlerProps) *eventBridgeRuleBuilder {
	return &eventBridgeRuleBuilder{
		handler: handler,
		props:   props,
	}
}

// build creates the EventBridge rule
func (rb *eventBridgeRuleBuilder) build() awsevents.Rule {
	ruleProps := &awsevents.RuleProps{}

	// Apply user-provided rule props
	rb.applyUserRuleProps(ruleProps)

	// Configure event pattern or schedule
	rb.configureRulePattern(ruleProps)

	// Set default rule name if not provided
	if ruleProps.RuleName == nil && rb.props.FunctionProps.FunctionName != nil {
		ruleProps.RuleName = jsii.String(*rb.props.FunctionProps.FunctionName + "-rule")
	}

	return awsevents.NewRule(rb.handler, jsii.String("Rule"), ruleProps)
}

// applyUserRuleProps applies user-provided rule properties
func (rb *eventBridgeRuleBuilder) applyUserRuleProps(ruleProps *awsevents.RuleProps) {
	if rb.props.RuleProps == nil {
		return
	}

	if rb.props.RuleProps.RuleName != nil {
		ruleProps.RuleName = rb.props.RuleProps.RuleName
	}
	if rb.props.RuleProps.Description != nil {
		ruleProps.Description = rb.props.RuleProps.Description
	}
	if rb.props.RuleProps.Enabled != nil {
		ruleProps.Enabled = rb.props.RuleProps.Enabled
	}
}

// configureRulePattern configures the rule's event pattern or schedule
func (rb *eventBridgeRuleBuilder) configureRulePattern(ruleProps *awsevents.RuleProps) {
	switch {
	case rb.props.EventPattern != nil:
		ruleProps.EventPattern = rb.props.EventPattern
		// Only set event bus for event pattern rules
		ruleProps.EventBus = rb.handler.EventBus
	case rb.props.ScheduleExpression != nil:
		ruleProps.Schedule = awsevents.Schedule_Expression(rb.props.ScheduleExpression)
		// Scheduled rules don't use event buses
	default:
		// Default to match all events if neither pattern nor schedule is provided
		ruleProps.EventPattern = &awsevents.EventPattern{
			Source: &[]*string{jsii.String("*")},
		}
		ruleProps.EventBus = rb.handler.EventBus
	}
}

// enableMonitoring adds CloudWatch alarms and metrics for the EventBridge handler
func (e *EventBridgeHandler) enableMonitoring() {
	if e.Function != nil {
		function := e.Function.Function

		// Function error rate alarm
		awscloudwatch.NewAlarm(e, jsii.String("FunctionErrorAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-errors", *e.Rule.RuleName())),
			AlarmDescription: jsii.String("EventBridge handler function errors"),
			Metric: function.MetricErrors(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(3),
			EvaluationPeriods:  jsii.Number(2),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})

		// Function duration alarm
		awscloudwatch.NewAlarm(e, jsii.String("FunctionDurationAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-duration", *e.Rule.RuleName())),
			AlarmDescription: jsii.String("EventBridge handler function duration"),
			Metric: function.MetricDuration(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(30000), // 30 seconds
			EvaluationPeriods:  jsii.Number(3),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// EventBridge rule invocation metrics (not used but kept for reference)
	_ = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("AWS/Events"),
		MetricName: jsii.String("InvocationsCount"),
		DimensionsMap: &map[string]*string{
			"RuleName": e.Rule.RuleName(),
		},
		Period: awscdk.Duration_Minutes(jsii.Number(5)),
	})

	// Helper function to create metric alarms
	createMetricAlarm := func(id, alarmNameSuffix, description, namespace, metricName string, dimensions *map[string]*string, threshold, evaluationPeriods float64) {
		awscloudwatch.NewAlarm(e, jsii.String(id), &awscloudwatch.AlarmProps{
			AlarmName:        jsii.String(fmt.Sprintf("%s-%s", *e.Rule.RuleName(), alarmNameSuffix)),
			AlarmDescription: jsii.String(description),
			Metric: awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
				Namespace:     jsii.String(namespace),
				MetricName:    jsii.String(metricName),
				DimensionsMap: dimensions,
				Period:        awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:          jsii.Number(threshold),
			EvaluationPeriods:  jsii.Number(evaluationPeriods),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
	}

	// Rule invocation failure alarm
	createMetricAlarm(
		"RuleFailureAlarm",
		"rule-failures",
		"EventBridge rule invocation failures",
		"AWS/Events",
		"FailedInvocations",
		&map[string]*string{"RuleName": e.Rule.RuleName()},
		5,
		2,
	)

	// DLQ monitoring if DLQ exists
	if e.DeadLetterQueue != nil {
		createMetricAlarm(
			"DLQAlarm",
			"dlq-messages",
			"Messages in EventBridge handler DLQ",
			"AWS/SQS",
			"ApproximateNumberOfMessages",
			&map[string]*string{"QueueName": e.DeadLetterQueue.QueueName()},
			10,
			1,
		)
	}
}

// GrantPutEvents grants permission to put events to the event bus
func (e *EventBridgeHandler) GrantPutEvents(grantee awslambda.IFunction) {
	e.EventBus.GrantPutEventsTo(grantee, jsii.String("eventbridge"))
}

// AddEnvironmentVariable adds an environment variable to the Lambda function
func (e *EventBridgeHandler) AddEnvironmentVariable(key string, value string) {
	e.Function.Function.AddEnvironment(jsii.String(key), jsii.String(value), nil)
}

// GetEventBusName returns the event bus name
func (e *EventBridgeHandler) GetEventBusName() *string {
	return e.EventBus.EventBusName()
}

// GetEventBusArn returns the event bus ARN
func (e *EventBridgeHandler) GetEventBusArn() *string {
	return e.EventBus.EventBusArn()
}

// GetRuleName returns the rule name
func (e *EventBridgeHandler) GetRuleName() *string {
	return e.Rule.RuleName()
}

// GetRuleArn returns the rule ARN
func (e *EventBridgeHandler) GetRuleArn() *string {
	return e.Rule.RuleArn()
}

// AddEventPattern adds an event pattern to the rule
// Note: This method is deprecated as EventBridge patterns cannot be modified after rule creation.
// Create a new EventBridgeHandler with the desired pattern instead.
func (e *EventBridgeHandler) AddEventPattern(_ *awsevents.EventPattern) error {
	// For existing rules, we need to recreate or update the pattern
	// This is a limitation of EventBridge - patterns cannot be modified after creation
	// Users should create a new rule with the desired pattern
	return fmt.Errorf("event patterns cannot be modified after rule creation - create a new EventBridgeHandler with the desired pattern")
}

// EnableRule enables the EventBridge rule
// Note: This method configures the rule to be enabled during deployment.
// To change rule state after deployment, use AWS CLI or AWS Console.
func (e *EventBridgeHandler) EnableRule() error {
	// CDK constructs are immutable at runtime, but we can configure the deployment state
	// The rule state is set during CDK construction time
	// For runtime changes, users should use AWS CLI: aws events enable-rule --name <rule-name>
	return fmt.Errorf("rule state cannot be changed after CDK deployment - use AWS CLI: aws events enable-rule --name %s", *e.Rule.RuleName())
}

// DisableRule disables the EventBridge rule
// Note: This method provides guidance for disabling the rule after deployment.
// To change rule state after deployment, use AWS CLI or AWS Console.
func (e *EventBridgeHandler) DisableRule() error {
	// CDK constructs are immutable at runtime, but we can configure the deployment state
	// The rule state is set during CDK construction time
	// For runtime changes, users should use AWS CLI: aws events disable-rule --name <rule-name>
	return fmt.Errorf("rule state cannot be changed after CDK deployment - use AWS CLI: aws events disable-rule --name %s", *e.Rule.RuleName())
}
