package patterns

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// EventOrchestratorProps defines properties for an event orchestrator pattern
type EventOrchestratorProps struct {
	// Pointers and maps first for better alignment
	DefaultEnvironment     *map[string]*string
	EventRoutingTableProps *liftconstructs.EventRoutingTableProps
	EventBusName           *string
	AppName                *string
	EnableEventArchive     *bool
	EnableEventRouting     *bool
	EnableSagaPattern      *bool
	EnableEventCorrelation *bool
	EnableTracing          *bool
	EnableMultiTenant      *bool
	EnableMonitoring       *bool
	MaxRetryAttempts       *float64
	RetryBackoffRate       *float64
	DefaultMemorySize      *float64
	DefaultTimeout         *float64
	EventRetentionDays     *float64
	ArchiveRetentionDays   *float64
	// Optional: specify the actual DLQ to monitor, or its name, to avoid relying on name conventions
	DLQQueue     awssqs.IQueue
	DLQQueueName *string
	// Non-pointer struct fields
	DefaultFunctionProps awslambda.FunctionProps
	EventSources         []EventSourceConfig
}

// EventSourceConfig defines configuration for an event source
type EventSourceConfig struct {
	SourceName     *string
	HandlerProps   *awslambda.FunctionProps
	ProcessingMode *string
	EventFilters   map[string]interface{}
	EventTypes     []*string
}

// EventOrchestrator represents a multi-source event orchestration pattern
type EventOrchestrator struct {
	constructs.Construct

	// Event routing table (DynamORM-based)
	EventRoutingTable *liftconstructs.EventRoutingTable

	// Event source handlers
	EventHandlers map[string]*liftconstructs.EventBridgeHandler

	// Orchestration function
	OrchestratorFunction *liftconstructs.LiftFunction

	// Correlation function (if enabled)
	CorrelationFunction *liftconstructs.LiftFunction

	// Dead letter handler
	DLQHandler *liftconstructs.LiftFunction

	// Dead letter queue (created by the orchestrator by default)
	DLQQueue awssqs.IQueue
}

// NewEventOrchestrator creates a new event orchestrator pattern using DynamORM
func NewEventOrchestrator(scope constructs.Construct, id *string, props *EventOrchestratorProps) *EventOrchestrator {
	builder := newEventOrchestratorBuilder(scope, id, props)
	return builder.build()
}

// eventOrchestratorBuilder builds event orchestrator components
type eventOrchestratorBuilder struct {
	orchestrator *EventOrchestrator
	props        *EventOrchestratorProps
	config       *eventOrchestratorConfig
}

// eventOrchestratorConfig holds resolved configuration values
type eventOrchestratorConfig struct {
	appName                string
	eventBusName           string
	enableEventRouting     bool
	enableSagaPattern      bool
	enableEventCorrelation bool
	enableMonitoring       bool
}

// newEventOrchestratorBuilder creates a new event orchestrator builder
func newEventOrchestratorBuilder(scope constructs.Construct, id *string, props *EventOrchestratorProps) *eventOrchestratorBuilder {
	orchestrator := &EventOrchestrator{
		EventHandlers: make(map[string]*liftconstructs.EventBridgeHandler),
	}
	constructs.NewConstruct_Override(orchestrator, scope, id)

	return &eventOrchestratorBuilder{
		orchestrator: orchestrator,
		props:        props,
		config:       buildEventOrchestratorConfig(props),
	}
}

// buildEventOrchestratorConfig resolves configuration values with defaults
func buildEventOrchestratorConfig(props *EventOrchestratorProps) *eventOrchestratorConfig {
	if props == nil {
		props = &EventOrchestratorProps{}
	}

	config := &eventOrchestratorConfig{
		appName:                "event-orchestrator",
		eventBusName:           "default",
		enableEventRouting:     true,
		enableSagaPattern:      false,
		enableEventCorrelation: true,
		enableMonitoring:       false,
	}

	// Apply provided values
	if props.AppName != nil {
		config.appName = *props.AppName
	}
	if props.EventBusName != nil {
		config.eventBusName = *props.EventBusName
	}
	if props.EnableEventRouting != nil {
		config.enableEventRouting = *props.EnableEventRouting
	}
	if props.EnableSagaPattern != nil {
		config.enableSagaPattern = *props.EnableSagaPattern
	}
	if props.EnableEventCorrelation != nil {
		config.enableEventCorrelation = *props.EnableEventCorrelation
	}
	if props.EnableMonitoring != nil {
		config.enableMonitoring = *props.EnableMonitoring
	}

	return config
}

// build constructs the complete event orchestrator
func (b *eventOrchestratorBuilder) build() *EventOrchestrator {
	// Create event routing table
	b.setupEventRoutingTable()

	// Create core functions
	b.setupOrchestratorFunction()
	b.setupCorrelationFunction()

	// Create event source handlers
	b.setupEventSourceHandlers()

	// Create a DLQ resource for the orchestrator (unless provided via props)
	b.setupDLQ()

	// Setup monitoring
	b.setupMonitoring()

	return b.orchestrator
}

// setupDLQ creates a dedicated DLQ SQS queue for the orchestrator if not provided
func (b *eventOrchestratorBuilder) setupDLQ() {
	// If user passed a queue via props, use it
	if b.props != nil && b.props.DLQQueue != nil {
		b.orchestrator.DLQQueue = b.props.DLQQueue
		return
	}

	// Otherwise create a new queue with sensible defaults; avoid explicit QueueName to prevent collisions
	qProps := &awssqs.QueueProps{
		RetentionPeriod: awscdk.Duration_Days(jsii.Number(14)),
	}

	// If a queue name override is provided, honor it
	if b.props != nil && b.props.DLQQueueName != nil {
		qProps.QueueName = b.props.DLQQueueName
	}

	b.orchestrator.DLQQueue = awssqs.NewQueue(b.orchestrator, jsii.String("OrchestratorDLQ"), qProps)
}

// setupEventRoutingTable creates event routing table if enabled
func (b *eventOrchestratorBuilder) setupEventRoutingTable() {
	if !b.config.enableEventRouting {
		return
	}

	eventRoutingProps := &liftconstructs.EventRoutingTableProps{
		TableName: jsii.String(b.config.appName + "-routing"),
		// GSIs for source, status, and date indexes are now defined in DynamORM models
		// Example model:
		// type EventRoute struct {
		//     PK         string `dynamorm:"pk"`                    // event#{event_id}
		//     SK         string `dynamorm:"sk"`                    // route#{route_id}
		//     Source     string `dynamorm:"index:source-index,pk"` // For source queries
		//     Status     string `dynamorm:"index:status-index,pk"` // For status queries
		//     Date       string `dynamorm:"index:date-index,pk"`   // For date queries
		// }
	}

	// Override with user-provided props
	if b.props.EventRoutingTableProps != nil {
		eventRoutingProps = b.props.EventRoutingTableProps
	}

	b.orchestrator.EventRoutingTable = liftconstructs.NewEventRoutingTable(b.orchestrator, jsii.String("EventRouting"), eventRoutingProps)
}

// setupOrchestratorFunction creates the main orchestrator function
func (b *eventOrchestratorBuilder) setupOrchestratorFunction() {
	functionBuilder := newOrchestratorFunctionBuilder(b.orchestrator, b.props, b.config)
	b.orchestrator.OrchestratorFunction = functionBuilder.build()

	// Grant permissions to orchestrator
	if b.orchestrator.EventRoutingTable != nil {
		b.orchestrator.EventRoutingTable.GrantEventManagement(b.orchestrator.OrchestratorFunction.Function)
	}
}

// setupCorrelationFunction creates correlation function if enabled
func (b *eventOrchestratorBuilder) setupCorrelationFunction() {
	if !b.config.enableEventCorrelation {
		return
	}

	correlationBuilder := newCorrelationFunctionBuilder(b.orchestrator, b.props, b.config)
	b.orchestrator.CorrelationFunction = correlationBuilder.build()

	// Grant permissions to correlator
	if b.orchestrator.EventRoutingTable != nil {
		b.orchestrator.EventRoutingTable.GrantEventManagement(b.orchestrator.CorrelationFunction.Function)
	}
}

// setupEventSourceHandlers creates handlers for each event source
func (b *eventOrchestratorBuilder) setupEventSourceHandlers() {
	for _, sourceConfig := range b.props.EventSources {
		if sourceConfig.SourceName == nil {
			continue
		}

		handlerBuilder := newEventSourceHandlerBuilder(b.orchestrator, b.props, b.config, sourceConfig)
		handler := handlerBuilder.build()

		if handler != nil {
			b.orchestrator.EventHandlers[*sourceConfig.SourceName] = handler
		}
	}
}

// setupMonitoring enables monitoring if requested
func (b *eventOrchestratorBuilder) setupMonitoring() {
	if b.config.enableMonitoring {
		b.orchestrator.enableMonitoring(b.props)
	}
}

// orchestratorFunctionBuilder builds the main orchestrator function
type orchestratorFunctionBuilder struct {
	orchestrator *EventOrchestrator
	props        *EventOrchestratorProps
	config       *eventOrchestratorConfig
}

// newOrchestratorFunctionBuilder creates a new orchestrator function builder
func newOrchestratorFunctionBuilder(orchestrator *EventOrchestrator, props *EventOrchestratorProps, config *eventOrchestratorConfig) *orchestratorFunctionBuilder {
	return &orchestratorFunctionBuilder{
		orchestrator: orchestrator,
		props:        props,
		config:       config,
	}
}

// build creates the orchestrator function
func (ofb *orchestratorFunctionBuilder) build() *liftconstructs.LiftFunction {
	// Create orchestrator environment
	orchestratorEnv := ofb.buildEnvironment()

	// Create orchestrator function props
	orchestratorProps := ofb.buildFunctionProps(orchestratorEnv)

	return liftconstructs.NewLiftFunction(ofb.orchestrator, jsii.String("Orchestrator"), &liftconstructs.LiftFunctionProps{
		FunctionProps:     orchestratorProps,
		EnableTracing:     ofb.props.EnableTracing,
		EnableMultiTenant: ofb.props.EnableMultiTenant,
	})
}

// buildEnvironment creates environment variables for orchestrator function
func (ofb *orchestratorFunctionBuilder) buildEnvironment() map[string]*string {
	orchestratorEnv := make(map[string]*string)

	// Copy default environment
	if ofb.props.DefaultEnvironment != nil {
		for k, v := range *ofb.props.DefaultEnvironment {
			orchestratorEnv[k] = v
		}
	}

	// Add orchestration-specific environment variables
	orchestratorEnv["EVENT_BUS_NAME"] = jsii.String(ofb.config.eventBusName)
	orchestratorEnv["SAGA_ENABLED"] = jsii.String(fmt.Sprintf("%t", ofb.config.enableSagaPattern))
	orchestratorEnv["CORRELATION_ENABLED"] = jsii.String(fmt.Sprintf("%t", ofb.config.enableEventCorrelation))

	if ofb.orchestrator.EventRoutingTable != nil {
		orchestratorEnv["EVENT_ROUTING_TABLE"] = ofb.orchestrator.EventRoutingTable.GetTableName()
		orchestratorEnv["EVENT_ROUTING_TABLE_ARN"] = ofb.orchestrator.EventRoutingTable.GetTableArn()
	}

	return orchestratorEnv
}

// buildFunctionProps creates function properties for orchestrator
func (ofb *orchestratorFunctionBuilder) buildFunctionProps(env map[string]*string) awslambda.FunctionProps {
	orchestratorProps := ofb.props.DefaultFunctionProps
	orchestratorProps.FunctionName = jsii.String(ofb.config.appName + "-orchestrator")
	orchestratorProps.Environment = &env

	if ofb.props.DefaultMemorySize != nil {
		orchestratorProps.MemorySize = ofb.props.DefaultMemorySize
	}
	if ofb.props.DefaultTimeout != nil {
		orchestratorProps.Timeout = awscdk.Duration_Seconds(ofb.props.DefaultTimeout)
	}

	return orchestratorProps
}

// correlationFunctionBuilder builds the correlation function
type correlationFunctionBuilder struct {
	orchestrator *EventOrchestrator
	props        *EventOrchestratorProps
	config       *eventOrchestratorConfig
}

// newCorrelationFunctionBuilder creates a new correlation function builder
func newCorrelationFunctionBuilder(orchestrator *EventOrchestrator, props *EventOrchestratorProps, config *eventOrchestratorConfig) *correlationFunctionBuilder {
	return &correlationFunctionBuilder{
		orchestrator: orchestrator,
		props:        props,
		config:       config,
	}
}

// build creates the correlation function
func (cfb *correlationFunctionBuilder) build() *liftconstructs.LiftFunction {
	// Create correlation environment (copy from orchestrator)
	correlationEnv := make(map[string]*string)
	correlationEnv["EVENT_BUS_NAME"] = jsii.String(cfb.config.eventBusName)
	correlationEnv["SAGA_ENABLED"] = jsii.String(fmt.Sprintf("%t", cfb.config.enableSagaPattern))
	correlationEnv["CORRELATION_ENABLED"] = jsii.String(fmt.Sprintf("%t", cfb.config.enableEventCorrelation))

	if cfb.orchestrator.EventRoutingTable != nil {
		correlationEnv["EVENT_ROUTING_TABLE"] = cfb.orchestrator.EventRoutingTable.GetTableName()
		correlationEnv["EVENT_ROUTING_TABLE_ARN"] = cfb.orchestrator.EventRoutingTable.GetTableArn()
	}

	// Copy default environment
	if cfb.props.DefaultEnvironment != nil {
		for k, v := range *cfb.props.DefaultEnvironment {
			correlationEnv[k] = v
		}
	}

	correlationProps := cfb.props.DefaultFunctionProps
	correlationProps.FunctionName = jsii.String(cfb.config.appName + "-correlator")
	correlationProps.Environment = &correlationEnv

	return liftconstructs.NewLiftFunction(cfb.orchestrator, jsii.String("Correlator"), &liftconstructs.LiftFunctionProps{
		FunctionProps:     correlationProps,
		EnableTracing:     cfb.props.EnableTracing,
		EnableMultiTenant: cfb.props.EnableMultiTenant,
	})
}

// eventSourceHandlerBuilder builds handlers for event sources
type eventSourceHandlerBuilder struct {
	orchestrator *EventOrchestrator
	props        *EventOrchestratorProps
	config       *eventOrchestratorConfig
	sourceConfig EventSourceConfig
}

// newEventSourceHandlerBuilder creates a new event source handler builder
func newEventSourceHandlerBuilder(orchestrator *EventOrchestrator, props *EventOrchestratorProps, config *eventOrchestratorConfig, sourceConfig EventSourceConfig) *eventSourceHandlerBuilder {
	return &eventSourceHandlerBuilder{
		orchestrator: orchestrator,
		props:        props,
		config:       config,
		sourceConfig: sourceConfig,
	}
}

// build creates an event source handler
func (eshb *eventSourceHandlerBuilder) build() *liftconstructs.EventBridgeHandler {
	sourceName := *eshb.sourceConfig.SourceName

	// Create handler environment
	handlerEnv := eshb.buildHandlerEnvironment(sourceName)

	// Create handler function props
	handlerProps := eshb.buildHandlerProps(sourceName, handlerEnv)

	// Create event pattern
	eventPattern := eshb.buildEventPattern(sourceName)

	// Create EventBridge handler
	handler, err := liftconstructs.NewEventBridgeHandler(eshb.orchestrator, jsii.String(sourceName+"Handler"), &liftconstructs.EventBridgeHandlerProps{
		FunctionProps:     handlerProps,
		EnableTracing:     eshb.props.EnableTracing,
		EnableMultiTenant: eshb.props.EnableMultiTenant,
		RuleProps: &awsevents.RuleProps{
			RuleName:     jsii.String(fmt.Sprintf("%s-%s-rule", eshb.config.appName, sourceName)),
			Description:  jsii.String(fmt.Sprintf("Process %s events", sourceName)),
			EventPattern: eventPattern,
		},
	})

	if err != nil {
		// Log error and return nil
		fmt.Printf("Warning: Failed to create EventBridge handler for %s: %v\n", sourceName, err)
		return nil
	}

	// Grant permissions
	if eshb.orchestrator.EventRoutingTable != nil {
		eshb.orchestrator.EventRoutingTable.GrantEventManagement(handler.Function.Function)
	}

	return handler
}

// buildHandlerEnvironment creates environment variables for handler
func (eshb *eventSourceHandlerBuilder) buildHandlerEnvironment(sourceName string) map[string]*string {
	handlerEnv := make(map[string]*string)

	// Copy default environment
	if eshb.props.DefaultEnvironment != nil {
		for k, v := range *eshb.props.DefaultEnvironment {
			handlerEnv[k] = v
		}
	}

	// Add source-specific environment
	handlerEnv["EVENT_SOURCE"] = jsii.String(sourceName)
	handlerEnv["PROCESSING_MODE"] = eshb.sourceConfig.ProcessingMode

	if eshb.orchestrator.EventRoutingTable != nil {
		handlerEnv["EVENT_ROUTING_TABLE"] = eshb.orchestrator.EventRoutingTable.GetTableName()
	}

	return handlerEnv
}

// buildHandlerProps creates function properties for handler
func (eshb *eventSourceHandlerBuilder) buildHandlerProps(sourceName string, env map[string]*string) awslambda.FunctionProps {
	handlerProps := eshb.props.DefaultFunctionProps

	if eshb.sourceConfig.HandlerProps != nil {
		handlerProps = *eshb.sourceConfig.HandlerProps
	}

	handlerProps.FunctionName = jsii.String(fmt.Sprintf("%s-%s-handler", eshb.config.appName, sourceName))
	handlerProps.Environment = &env

	return handlerProps
}

// buildEventPattern creates event pattern for handler
func (eshb *eventSourceHandlerBuilder) buildEventPattern(sourceName string) *awsevents.EventPattern {
	eventPattern := &awsevents.EventPattern{
		Source: &[]*string{jsii.String(sourceName)},
	}

	if len(eshb.sourceConfig.EventTypes) > 0 {
		eventPattern.DetailType = &eshb.sourceConfig.EventTypes
	}

	if len(eshb.sourceConfig.EventFilters) > 0 {
		eventPattern.Detail = &eshb.sourceConfig.EventFilters
	}

	return eventPattern
}

// enableMonitoring adds CloudWatch alarms and metrics
func (e *EventOrchestrator) enableMonitoring(props *EventOrchestratorProps) {
	appName := "event-orchestrator"
	if props != nil && props.AppName != nil {
		appName = *props.AppName
	}

	// Create SNS topic for alerts
	alertTopic := awssns.NewTopic(e, jsii.String("AlertTopic"), &awssns.TopicProps{
		TopicName:   jsii.String(fmt.Sprintf("%s-orchestrator-alerts", appName)),
		DisplayName: jsii.String(fmt.Sprintf("%s Event Orchestrator Alerts", appName)),
	})

	// 1. Event processing latency by source
	for name, handler := range e.EventHandlers {
		if handler != nil && handler.Function != nil {
			function := handler.Function.Function

			// Create function monitoring alarms using helper
			e.createFunctionAlarms(appName, name, function, alertTopic)
		}
	}

	// 2. Event routing table metrics (if enabled)
	if e.EventRoutingTable != nil {
		routingTableName := e.EventRoutingTable.GetTableName()

		// Create DynamoDB throttling alarms using helper
		e.createDynamoDBThrottleAlarms(appName, routingTableName, alertTopic)
	}

	// 3. Custom metrics for correlation success rates
	correlationMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String(fmt.Sprintf("Lift/EventOrchestrator/%s", appName)),
		MetricName: jsii.String("CorrelationSuccess"),
		Statistic:  jsii.String("Average"),
		Period:     awscdk.Duration_Minutes(jsii.Number(5)),
	})

	correlationAlarm := awscloudwatch.NewAlarm(e, jsii.String("CorrelationFailureAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-correlation-failure", appName)),
		AlarmDescription:   jsii.String("Low event correlation success rate"),
		Metric:             correlationMetric,
		Threshold:          jsii.Number(0.95), // 95% success rate
		ComparisonOperator: awscloudwatch.ComparisonOperator_LESS_THAN_THRESHOLD,
		EvaluationPeriods:  jsii.Number(3),
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	correlationAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))

	// 4. Saga completion metrics
	sagaMetric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String(fmt.Sprintf("Lift/EventOrchestrator/%s", appName)),
		MetricName: jsii.String("SagaCompletion"),
		Statistic:  jsii.String("Average"),
		Period:     awscdk.Duration_Minutes(jsii.Number(15)),
	})

	sagaAlarm := awscloudwatch.NewAlarm(e, jsii.String("SagaFailureAlarm"), &awscloudwatch.AlarmProps{
		AlarmName:          jsii.String(fmt.Sprintf("%s-saga-failure", appName)),
		AlarmDescription:   jsii.String("Low saga completion rate"),
		Metric:             sagaMetric,
		Threshold:          jsii.Number(0.90), // 90% completion rate
		ComparisonOperator: awscloudwatch.ComparisonOperator_LESS_THAN_THRESHOLD,
		EvaluationPeriods:  jsii.Number(3),
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	sagaAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))

	// 5. DLQ metrics (if DLQ handler exists and DLQ is configured)
	if e.DLQHandler != nil {
		var dlqMetric awscloudwatch.IMetric
		switch {
		case e.DLQQueue != nil:
			dlqMetric = e.DLQQueue.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
				Period:    awscdk.Duration_Minutes(jsii.Number(5)),
				Statistic: jsii.String("Maximum"),
			})
		case props != nil && props.DLQQueue != nil:
			dlqMetric = props.DLQQueue.MetricApproximateNumberOfMessagesVisible(&awscloudwatch.MetricOptions{
				Period:    awscdk.Duration_Minutes(jsii.Number(5)),
				Statistic: jsii.String("Maximum"),
			})
		case props != nil && props.DLQQueueName != nil:
			dlqMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
				Namespace:  jsii.String("AWS/SQS"),
				MetricName: jsii.String("ApproximateNumberOfMessages"),
				DimensionsMap: &map[string]*string{
					"QueueName": props.DLQQueueName,
				},
				Statistic: jsii.String("Maximum"),
				Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			})
		default:
			dlqMetric = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
				Namespace:  jsii.String("AWS/SQS"),
				MetricName: jsii.String("ApproximateNumberOfMessages"),
				DimensionsMap: &map[string]*string{
					"QueueName": jsii.String(fmt.Sprintf("%s-dlq", appName)),
				},
				Statistic: jsii.String("Maximum"),
				Period:    awscdk.Duration_Minutes(jsii.Number(5)),
			})
		}

		dlqAlarm := awscloudwatch.NewAlarm(e, jsii.String("DLQAlarm"), &awscloudwatch.AlarmProps{
			AlarmName:          jsii.String(fmt.Sprintf("%s-dlq-messages", appName)),
			AlarmDescription:   jsii.String("Messages in dead letter queue"),
			Metric:             dlqMetric,
			Threshold:          jsii.Number(10),
			ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
			EvaluationPeriods:  jsii.Number(1),
			TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
		})
		dlqAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
	}

	// Create CloudWatch Dashboard
	e.createMonitoringDashboard(appName, alertTopic)
}

// createMonitoringDashboard creates a basic dashboard for the event orchestrator
func (e *EventOrchestrator) createMonitoringDashboard(appName string, _ awssns.ITopic) {
	// Create a basic dashboard
	awscloudwatch.NewDashboard(e, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-event-orchestrator", appName)),
	})
}

// GetEventRoutingTableName returns the event routing table name
func (e *EventOrchestrator) GetEventRoutingTableName() *string {
	if e.EventRoutingTable != nil {
		return e.EventRoutingTable.GetTableName()
	}
	return nil
}

// GrantEventRoutingAccess grants read/write access to the event routing table
func (e *EventOrchestrator) GrantEventRoutingAccess(grantee awslambda.IFunction) {
	if e.EventRoutingTable != nil {
		e.EventRoutingTable.GrantEventManagement(awsiam.IGrantable(grantee))
	}
}

// AddEventSource adds a new event source to the orchestrator
func (e *EventOrchestrator) AddEventSource(_ EventSourceConfig) {
	// Dynamic event source addition is not implemented in this version
	// This would create a new EventBridgeHandler and add it to the orchestrator
	// Implementation would need to handle CDK construct creation at runtime
}

// GetEventHandler returns the handler for a specific event source
func (e *EventOrchestrator) GetEventHandler(sourceName string) *liftconstructs.EventBridgeHandler {
	return e.EventHandlers[sourceName]
}

// lambdaAlarmConfig defines configuration for Lambda function alarms
type lambdaAlarmConfig struct {
	alarmType         string
	alarmSuffix       string
	descriptionSuffix string
	metricFunc        func(awslambda.IFunction, *awscloudwatch.MetricOptions) awscloudwatch.IMetric
	statistic         string
	threshold         float64
	evaluationPeriods float64
}

// createLambdaAlarm creates a standardized CloudWatch alarm for Lambda functions
func (e *EventOrchestrator) createLambdaAlarm(appName, handlerName string, function awslambda.IFunction, alertTopic awssns.ITopic, config lambdaAlarmConfig) {
	alarm := awscloudwatch.NewAlarm(e, jsii.String(fmt.Sprintf("%s%sAlarm", config.alarmType, handlerName)), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-orchestrator-%s-%s", appName, config.alarmSuffix, handlerName)),
		AlarmDescription: jsii.String(fmt.Sprintf("%s for event handler %s", config.descriptionSuffix, handlerName)),
		Metric: config.metricFunc(function, &awscloudwatch.MetricOptions{
			Statistic: jsii.String(config.statistic),
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(config.threshold),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		EvaluationPeriods:  jsii.Number(config.evaluationPeriods),
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	alarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
}

// createFunctionAlarms creates standard monitoring alarms for a Lambda function
func (e *EventOrchestrator) createFunctionAlarms(appName, handlerName string, function awslambda.IFunction, alertTopic awssns.ITopic) {
	// Function duration alarm
	e.createLambdaAlarm(appName, handlerName, function, alertTopic, lambdaAlarmConfig{
		alarmType:         "Duration",
		alarmSuffix:       "duration",
		descriptionSuffix: "High duration",
		metricFunc: func(f awslambda.IFunction, opts *awscloudwatch.MetricOptions) awscloudwatch.IMetric {
			return f.MetricDuration(opts)
		},
		statistic:         "Average",
		threshold:         30000, // 30 seconds
		evaluationPeriods: 2,
	})

	// Function error rate alarm
	e.createLambdaAlarm(appName, handlerName, function, alertTopic, lambdaAlarmConfig{
		alarmType:         "Error",
		alarmSuffix:       "errors",
		descriptionSuffix: "High error rate",
		metricFunc: func(f awslambda.IFunction, opts *awscloudwatch.MetricOptions) awscloudwatch.IMetric {
			return f.MetricErrors(opts)
		},
		statistic:         "Sum",
		threshold:         5,
		evaluationPeriods: 1,
	})
}

// dynamoThrottleAlarmConfig defines configuration for DynamoDB throttling alarms
type dynamoThrottleAlarmConfig struct {
	alarmIDSuffix   string
	alarmNameSuffix string
	description     string
	metricName      string
}

// createDynamoThrottleAlarm creates a standardized DynamoDB throttling alarm
func (e *EventOrchestrator) createDynamoThrottleAlarm(appName string, tableName *string, alertTopic awssns.ITopic, config dynamoThrottleAlarmConfig) {
	alarm := awscloudwatch.NewAlarm(e, jsii.String(fmt.Sprintf("Routing%sThrottleAlarm", config.alarmIDSuffix)), &awscloudwatch.AlarmProps{
		AlarmName:        jsii.String(fmt.Sprintf("%s-routing-%s-throttle", appName, config.alarmNameSuffix)),
		AlarmDescription: jsii.String(config.description),
		Metric: awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/DynamoDB"),
			MetricName: jsii.String(config.metricName),
			DimensionsMap: &map[string]*string{
				"TableName": tableName,
			},
			Statistic: jsii.String("Sum"),
			Period:    awscdk.Duration_Minutes(jsii.Number(5)),
		}),
		Threshold:          jsii.Number(0),
		ComparisonOperator: awscloudwatch.ComparisonOperator_GREATER_THAN_THRESHOLD,
		EvaluationPeriods:  jsii.Number(1),
		TreatMissingData:   awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	alarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(alertTopic))
}

// createDynamoDBThrottleAlarms creates throttling alarms for a DynamoDB table
func (e *EventOrchestrator) createDynamoDBThrottleAlarms(appName string, tableName *string, alertTopic awssns.ITopic) {
	// Read throttle alarm
	e.createDynamoThrottleAlarm(appName, tableName, alertTopic, dynamoThrottleAlarmConfig{
		alarmIDSuffix:   "Read",
		alarmNameSuffix: "read",
		description:     "Event routing table read throttling",
		metricName:      "ReadThrottleEvents",
	})

	// Write throttle alarm
	e.createDynamoThrottleAlarm(appName, tableName, alertTopic, dynamoThrottleAlarmConfig{
		alarmIDSuffix:   "Write",
		alarmNameSuffix: "write",
		description:     "Event routing table write throttling",
		metricName:      "WriteThrottleEvents",
	})
}
