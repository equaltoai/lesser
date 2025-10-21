package patterns

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// EventDrivenAPIProps defines properties for an event-driven API pattern
type EventDrivenAPIProps struct {
	// Application name
	AppName *string

	// API configuration
	ApiName             *string
	Description         *string
	EnableCORS          *bool
	EnableAccessLogging *bool
	ThrottleRateLimit   *float64
	ThrottleBurstLimit  *float64

	// Lambda function configuration
	FunctionProps awslambda.FunctionProps
	MemorySize    *float64
	Timeout       *float64
	Environment   *map[string]*string

	// EventBridge configuration
	EventBusName *string
	EventSource  *string
	DetailType   *string

	// Request tracking configuration
	RequestTrackingTableProps *liftconstructs.RequestTrackingTableProps
	EnableRequestTracking     *bool
	RequestRetentionDays      *float64

	// Lift-specific settings
	EnableTracing     *bool
	EnableMultiTenant *bool
	EnableMonitoring  *bool
}

// EventDrivenAPI represents an API Gateway + EventBridge pattern for async processing
type EventDrivenAPI struct {
	constructs.Construct

	// The HTTP API
	API *liftconstructs.LiftAPI

	// API handler function
	APIFunction *liftconstructs.LiftFunction

	// EventBridge handler
	EventHandler *liftconstructs.EventBridgeHandler

	// Request tracking table (DynamORM-based)
	RequestTrackingTable *liftconstructs.RequestTrackingTable
}

// NewEventDrivenAPI creates a new event-driven API pattern using DynamORM
func NewEventDrivenAPI(scope constructs.Construct, id *string, props *EventDrivenAPIProps) *EventDrivenAPI {
	builder := newEventDrivenAPIBuilder(scope, id, props)
	return builder.build()
}

// eventDrivenAPIBuilder builds event-driven API components
type eventDrivenAPIBuilder struct {
	scope  constructs.Construct
	id     *string
	props  *EventDrivenAPIProps
	api    *EventDrivenAPI
	config *eventDrivenAPIConfig
}

// eventDrivenAPIConfig holds resolved configuration values
type eventDrivenAPIConfig struct {
	appName               string
	apiName               string
	eventBusName          string
	eventSource           string
	detailType            string
	enableRequestTracking bool
}

// newEventDrivenAPIBuilder creates a new event-driven API builder
func newEventDrivenAPIBuilder(scope constructs.Construct, id *string, props *EventDrivenAPIProps) *eventDrivenAPIBuilder {
	return &eventDrivenAPIBuilder{
		scope:  scope,
		id:     id,
		props:  props,
		config: buildEventDrivenAPIConfig(props),
	}
}

// buildEventDrivenAPIConfig resolves configuration values with defaults
func buildEventDrivenAPIConfig(props *EventDrivenAPIProps) *eventDrivenAPIConfig {
	if props == nil {
		props = &EventDrivenAPIProps{}
	}

	config := &eventDrivenAPIConfig{
		appName:               "event-driven-api",
		eventBusName:          "default",
		detailType:            "APIRequest",
		enableRequestTracking: true,
	}

	// Apply provided values
	if props.AppName != nil {
		config.appName = *props.AppName
	}
	if props.EventBusName != nil {
		config.eventBusName = *props.EventBusName
	}
	if props.DetailType != nil {
		config.detailType = *props.DetailType
	}
	if props.EnableRequestTracking != nil {
		config.enableRequestTracking = *props.EnableRequestTracking
	}

	// Derive other values
	config.apiName = config.appName + "-api"
	if props.ApiName != nil {
		config.apiName = *props.ApiName
	}
	config.eventSource = config.appName
	if props.EventSource != nil {
		config.eventSource = *props.EventSource
	}

	return config
}

// build constructs the complete event-driven API
func (b *eventDrivenAPIBuilder) build() *EventDrivenAPI {
	b.api = &EventDrivenAPI{}
	constructs.NewConstruct_Override(b.api, b.scope, b.id)

	b.setupRequestTracking()
	b.setupAPIFunction()
	b.setupAPI()
	b.setupEventHandler()
	b.setupMonitoring()

	return b.api
}

// setupRequestTracking creates the request tracking table if enabled
func (b *eventDrivenAPIBuilder) setupRequestTracking() {
	if !b.config.enableRequestTracking {
		return
	}

	requestTrackingProps := &liftconstructs.RequestTrackingTableProps{
		TableName: jsii.String(b.config.appName + "-requests"),
	}

	// Override with user-provided props
	if b.props.RequestTrackingTableProps != nil {
		requestTrackingProps = b.props.RequestTrackingTableProps
	}

	b.api.RequestTrackingTable = liftconstructs.NewRequestTrackingTable(b.api, jsii.String("RequestTracking"), requestTrackingProps)
}

// setupAPIFunction creates the API handler Lambda function
func (b *eventDrivenAPIBuilder) setupAPIFunction() {
	// Create environment variables
	apiEnv := b.buildAPIEnvironment()

	// Create API function properties
	apiFunctionProps := b.props.FunctionProps
	apiFunctionProps.FunctionName = jsii.String(b.config.appName + "-api-handler")
	apiFunctionProps.Environment = &apiEnv
	b.applyFunctionConfig(&apiFunctionProps)

	// Create Lift function
	b.api.APIFunction = liftconstructs.NewLiftFunction(b.api, jsii.String("APIFunction"), &liftconstructs.LiftFunctionProps{
		FunctionProps:     apiFunctionProps,
		EnableTracing:     b.props.EnableTracing,
		EnableMultiTenant: b.props.EnableMultiTenant,
	})

	// Grant permissions
	if b.api.RequestTrackingTable != nil {
		b.api.RequestTrackingTable.GrantReadWrite(b.api.APIFunction.Function)
	}
}

// setupAPI creates the HTTP API Gateway
func (b *eventDrivenAPIBuilder) setupAPI() {
	b.api.API = liftconstructs.NewLiftAPI(b.api, jsii.String("API"), &liftconstructs.LiftAPIProps{
		APICommonProps: liftconstructs.APICommonProps{
			Name:                jsii.String(b.config.apiName),
			Description:         jsii.String("Event-driven API with async processing"),
			EnableCORS:          b.props.EnableCORS,
			EnableAccessLogging: b.props.EnableAccessLogging,
			ThrottleRateLimit:   b.props.ThrottleRateLimit,
			ThrottleBurstLimit:  b.props.ThrottleBurstLimit,
		},
	})

	// Add routes to API
	b.api.API.AddLambdaRoute(jsii.String("/submit"), "POST", b.api.APIFunction.Function)
	b.api.API.AddLambdaRoute(jsii.String("/status/{requestId}"), "GET", b.api.APIFunction.Function)
}

// setupEventHandler creates the EventBridge handler
func (b *eventDrivenAPIBuilder) setupEventHandler() {
	// Create environment variables
	eventEnv := b.buildEventEnvironment()

	// Create event function properties
	eventFunctionProps := b.props.FunctionProps
	eventFunctionProps.FunctionName = jsii.String(b.config.appName + "-event-processor")
	eventFunctionProps.Environment = &eventEnv
	b.applyFunctionConfig(&eventFunctionProps)

	// Create EventBridge handler
	eventHandler, err := liftconstructs.NewEventBridgeHandler(b.api, jsii.String("EventHandler"), &liftconstructs.EventBridgeHandlerProps{
		FunctionProps:     eventFunctionProps,
		EnableTracing:     b.props.EnableTracing,
		EnableMultiTenant: b.props.EnableMultiTenant,
		RuleProps: &awsevents.RuleProps{
			RuleName:    jsii.String(b.config.appName + "-processor-rule"),
			Description: jsii.String("Process async API requests"),
			EventPattern: &awsevents.EventPattern{
				Source:     &[]*string{jsii.String(b.config.eventSource)},
				DetailType: &[]*string{jsii.String(b.config.detailType)},
			},
		},
	})

	if err != nil {
		fmt.Printf("Warning: Failed to create EventBridge handler: %v\n", err)
		b.api.EventHandler = nil
	} else {
		b.api.EventHandler = eventHandler
		// Grant permissions
		if b.api.RequestTrackingTable != nil {
			b.api.RequestTrackingTable.GrantReadWrite(b.api.EventHandler.Function.Function)
		}
	}
}

// setupMonitoring enables monitoring if requested
func (b *eventDrivenAPIBuilder) setupMonitoring() {
	if b.props.EnableMonitoring != nil && *b.props.EnableMonitoring {
		b.api.enableMonitoring(b.props)
	}
}

// buildAPIEnvironment creates environment variables for the API function
func (b *eventDrivenAPIBuilder) buildAPIEnvironment() map[string]*string {
	apiEnv := make(map[string]*string)

	// Copy user environment variables
	if b.props.Environment != nil {
		for k, v := range *b.props.Environment {
			apiEnv[k] = v
		}
	}

	// Add event-driven pattern variables
	apiEnv["EVENT_BUS_NAME"] = jsii.String(b.config.eventBusName)
	apiEnv["EVENT_SOURCE"] = jsii.String(b.config.eventSource)
	apiEnv["EVENT_DETAIL_TYPE"] = jsii.String(b.config.detailType)

	// Add request tracking variables
	if b.api.RequestTrackingTable != nil {
		apiEnv["REQUEST_TRACKING_TABLE"] = b.api.RequestTrackingTable.GetTableName()
		apiEnv["REQUEST_TRACKING_TABLE_ARN"] = b.api.RequestTrackingTable.GetTableArn()
	}

	return apiEnv
}

// buildEventEnvironment creates environment variables for the event function
func (b *eventDrivenAPIBuilder) buildEventEnvironment() map[string]*string {
	eventEnv := make(map[string]*string)

	// Copy user environment variables
	if b.props.Environment != nil {
		for k, v := range *b.props.Environment {
			eventEnv[k] = v
		}
	}

	// Add request tracking variables
	if b.api.RequestTrackingTable != nil {
		eventEnv["REQUEST_TRACKING_TABLE"] = b.api.RequestTrackingTable.GetTableName()
		eventEnv["REQUEST_TRACKING_TABLE_ARN"] = b.api.RequestTrackingTable.GetTableArn()
	}

	return eventEnv
}

// applyFunctionConfig applies memory size and timeout configuration
func (b *eventDrivenAPIBuilder) applyFunctionConfig(functionProps *awslambda.FunctionProps) {
	if b.props.MemorySize != nil {
		functionProps.MemorySize = b.props.MemorySize
	}
	if b.props.Timeout != nil {
		functionProps.Timeout = awscdk.Duration_Seconds(b.props.Timeout)
	}
}

// enableMonitoring adds CloudWatch alarms and metrics
func (e *EventDrivenAPI) enableMonitoring(_ *EventDrivenAPIProps) {
	// Basic monitoring implementation with Lambda function metrics only
	if e.APIFunction != nil {
		function := e.APIFunction.Function

		// Function error alarm
		awscloudwatch.NewAlarm(e, jsii.String("FunctionErrorAlarm"), &awscloudwatch.AlarmProps{
			AlarmName: jsii.String("api-function-errors"),
			Metric: function.MetricErrors(&awscloudwatch.MetricOptions{
				Period: awscdk.Duration_Minutes(jsii.Number(5)),
			}),
			Threshold:         jsii.Number(5),
			EvaluationPeriods: jsii.Number(2),
		})
	}
}

// GetAPIEndpoint returns the API endpoint URL
func (e *EventDrivenAPI) GetAPIEndpoint() *string {
	return e.API.GetUrl()
}

// GetRequestTrackingTableName returns the request tracking table name
func (e *EventDrivenAPI) GetRequestTrackingTableName() *string {
	if e.RequestTrackingTable != nil {
		return e.RequestTrackingTable.GetTableName()
	}
	return nil
}

// GrantRequestTrackingAccess grants read/write access to the request tracking table
func (e *EventDrivenAPI) GrantRequestTrackingAccess(grantee awslambda.IFunction) {
	if e.RequestTrackingTable != nil {
		e.RequestTrackingTable.GrantReadWrite(grantee)
	}
}

// AddAPIRoute adds a new route to the API
func (e *EventDrivenAPI) AddAPIRoute(path *string, method string, handler awslambda.IFunction) {
	httpMethod := awsapigatewayv2.HttpMethod(method)
	e.API.AddLambdaRoute(path, httpMethod, handler)
}
