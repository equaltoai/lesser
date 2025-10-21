package constructs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

const (
	defaultRoute = "$default"
)

// WebSocketRouteConfig defines configuration for WebSocket routes
type WebSocketRouteConfig struct {
	// Route key (e.g., "$connect", "$disconnect", defaultRoute, "custom")
	RouteKey *string
	// Lambda function for this route
	Function awslambda.IFunction
	// Whether this route requires authorization
	RequireAuthorization *bool
	// Custom authorizer for this route
	Authorizer awsapigatewayv2.IWebSocketRouteAuthorizer
}

// WebSocketAPIProps defines properties for a WebSocket API
// Memory optimized: 216 → 200 bytes (16 bytes saved)
type WebSocketAPIProps struct {
	AccessLogGroup             awslogs.ILogGroup
	DefaultAuthorizer          awsapigatewayv2.IWebSocketRouteAuthorizer
	DefaultRouteFunction       awslambda.IFunction
	ConnectRouteFunction       awslambda.IFunction
	DisconnectRouteFunction    awslambda.IFunction
	StageName                  *string
	ThrottleBurstLimit         *float64
	ApiName                    *string
	Description                *string
	RouteSelectionExpression   *string
	EnableDeadLetterQueue      *bool
	ThrottleRateLimit          *float64
	ConnectionTableProps       *ConnectionTableProps
	EnableConnectionManagement *bool
	AutoDeploy                 *bool
	EnableAccessLogging        *bool
	EnableTracing              *bool
	EnableMultiTenant          *bool
	EnableMonitoring           *bool
	Routes                     []*WebSocketRouteConfig
}

// WebSocketAPI represents a WebSocket API Gateway with Lambda integration
type WebSocketAPI struct {
	constructs.Construct

	// The WebSocket API
	WebSocketApi awsapigatewayv2.WebSocketApi

	// The stage
	Stage awsapigatewayv2.WebSocketStage

	// Lambda functions for different routes - REMOVED: Functions must be created externally
	// ConnectFunction    *LiftFunction
	// DisconnectFunction *LiftFunction
	// DefaultFunction    *LiftFunction

	// Connection management table (DynamORM-based)
	ConnectionTable *ConnectionTable

	// Routes map
	Routes map[string]awsapigatewayv2.WebSocketRoute

	// Access log group
	AccessLogGroup awslogs.ILogGroup
}

// NewWebSocketAPI creates a new WebSocket API construct using DynamORM
func NewWebSocketAPI(scope constructs.Construct, id *string, props *WebSocketAPIProps) *WebSocketAPI {
	this := &WebSocketAPI{}
	constructs.NewConstruct_Override(this, scope, id)

	builder := newWebSocketAPIBuilder(this, props)
	return builder.build()
}

// webSocketAPIBuilder builds WebSocket API components
type webSocketAPIBuilder struct {
	api    *WebSocketAPI
	props  *WebSocketAPIProps
	config *webSocketAPIConfig
}

// webSocketAPIConfig holds resolved configuration values
type webSocketAPIConfig struct {
	apiName                    string
	description                string
	routeSelectionExpression   string
	stageName                  string
	enableConnectionManagement bool
	autoDeploy                 bool
	enableAccessLogging        bool
}

// newWebSocketAPIBuilder creates a new WebSocket API builder
func newWebSocketAPIBuilder(api *WebSocketAPI, props *WebSocketAPIProps) *webSocketAPIBuilder {
	return &webSocketAPIBuilder{
		api:    api,
		props:  props,
		config: buildWebSocketAPIConfig(props),
	}
}

// buildWebSocketAPIConfig resolves configuration values with defaults
func buildWebSocketAPIConfig(props *WebSocketAPIProps) *webSocketAPIConfig {
	if props == nil {
		props = &WebSocketAPIProps{}
	}

	config := &webSocketAPIConfig{
		apiName:                    "WebSocketAPI",
		description:                "Lift WebSocket API with DynamORM",
		routeSelectionExpression:   "$request.body.action",
		stageName:                  "prod",
		enableConnectionManagement: true,
		autoDeploy:                 true,
		enableAccessLogging:        true,
	}

	// Apply provided values
	if props.ApiName != nil {
		config.apiName = *props.ApiName
	}
	if props.Description != nil {
		config.description = *props.Description
	}
	if props.RouteSelectionExpression != nil {
		config.routeSelectionExpression = *props.RouteSelectionExpression
	}
	if props.StageName != nil {
		config.stageName = *props.StageName
	}
	if props.EnableConnectionManagement != nil {
		config.enableConnectionManagement = *props.EnableConnectionManagement
	}
	if props.AutoDeploy != nil {
		config.autoDeploy = *props.AutoDeploy
	}
	if props.EnableAccessLogging != nil {
		config.enableAccessLogging = *props.EnableAccessLogging
	}

	return config
}

// build constructs the complete WebSocket API
func (b *webSocketAPIBuilder) build() *WebSocketAPI {
	// Setup access logging
	b.setupAccessLogging()

	// Setup WebSocket API
	b.setupWebSocketAPI()

	// Setup connection table
	b.setupConnectionTable()

	// Validate and setup routes
	b.validateRequiredFunctions()
	b.setupRoutes()

	// Setup stage
	b.setupStage()

	// Grant permissions
	b.api.grantApiGatewayInvokePermissions()

	return b.api
}

// setupAccessLogging creates access log group if enabled
func (b *webSocketAPIBuilder) setupAccessLogging() {
	if !b.config.enableAccessLogging {
		return
	}

	if b.props.AccessLogGroup != nil {
		b.api.AccessLogGroup = b.props.AccessLogGroup
	} else {
		b.api.AccessLogGroup = awslogs.NewLogGroup(b.api, jsii.String("AccessLogGroup"), &awslogs.LogGroupProps{
			LogGroupName:  jsii.String(fmt.Sprintf("/aws/apigateway/websocket/%s", b.config.apiName)),
			Retention:     awslogs.RetentionDays_ONE_MONTH,
			RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		})
	}
}

// setupWebSocketAPI creates the WebSocket API
func (b *webSocketAPIBuilder) setupWebSocketAPI() {
	apiProps := &awsapigatewayv2.WebSocketApiProps{
		ApiName:                  jsii.String(b.config.apiName),
		Description:              jsii.String(b.config.description),
		RouteSelectionExpression: jsii.String(b.config.routeSelectionExpression),
	}

	// Add default authorizer if provided
	if b.props.DefaultAuthorizer != nil {
		apiProps.DefaultRouteOptions = &awsapigatewayv2.WebSocketRouteOptions{
			Authorizer: b.props.DefaultAuthorizer,
		}
	}

	b.api.WebSocketApi = awsapigatewayv2.NewWebSocketApi(b.api, jsii.String("Api"), apiProps)
}

// setupConnectionTable creates connection management table if enabled
func (b *webSocketAPIBuilder) setupConnectionTable() {
	if !b.config.enableConnectionManagement {
		return
	}

	// Set defaults for connection table
	connectionTableProps := &ConnectionTableProps{}
	if b.props.ConnectionTableProps != nil {
		connectionTableProps = b.props.ConnectionTableProps
	}

	// Set table name based on API name if not provided
	if connectionTableProps.TableName == nil {
		connectionTableProps.TableName = jsii.String(fmt.Sprintf("%s-connections", b.config.apiName))
	}

	// Create the DynamORM-based connection table with minimal ID
	b.api.ConnectionTable = NewConnectionTable(b.api, jsii.String("T"), connectionTableProps)
}

// validateRequiredFunctions validates that required Lambda functions are provided
func (b *webSocketAPIBuilder) validateRequiredFunctions() {
	if b.props.ConnectRouteFunction == nil {
		panic("ConnectRouteFunction is required. Create Lambda function externally and pass via props to avoid long CloudFormation resource names.")
	}
	if b.props.DisconnectRouteFunction == nil {
		panic("DisconnectRouteFunction is required. Create Lambda function externally and pass via props to avoid long CloudFormation resource names.")
	}
	if b.props.DefaultRouteFunction == nil {
		panic("DefaultRouteFunction is required. Create Lambda function externally and pass via props to avoid long CloudFormation resource names.")
	}
}

// setupRoutes creates all WebSocket routes
func (b *webSocketAPIBuilder) setupRoutes() {
	// Initialize routes map
	b.api.Routes = make(map[string]awsapigatewayv2.WebSocketRoute)

	// Add standard routes
	b.addStandardRoutes()

	// Add custom routes
	b.addCustomRoutes()
}

// addStandardRoutes adds the standard WebSocket routes
func (b *webSocketAPIBuilder) addStandardRoutes() {
	if b.props.ConnectRouteFunction != nil {
		b.api.AddRoute("$connect", b.props.ConnectRouteFunction, &WebSocketRouteConfig{
			RouteKey: jsii.String("$connect"),
			Function: b.props.ConnectRouteFunction,
		})
	}

	if b.props.DisconnectRouteFunction != nil {
		b.api.AddRoute("$disconnect", b.props.DisconnectRouteFunction, &WebSocketRouteConfig{
			RouteKey: jsii.String("$disconnect"),
			Function: b.props.DisconnectRouteFunction,
		})
	}

	if b.props.DefaultRouteFunction != nil {
		b.api.AddRoute(defaultRoute, b.props.DefaultRouteFunction, &WebSocketRouteConfig{
			RouteKey: jsii.String(defaultRoute),
			Function: b.props.DefaultRouteFunction,
		})
	}
}

// addCustomRoutes adds custom routes if provided
func (b *webSocketAPIBuilder) addCustomRoutes() {
	if b.props.Routes == nil {
		return
	}

	for _, routeConfig := range b.props.Routes {
		if routeConfig.RouteKey != nil && routeConfig.Function != nil {
			b.api.AddRoute(*routeConfig.RouteKey, routeConfig.Function, routeConfig)
		}
	}
}

// setupStage creates the WebSocket stage with throttling
func (b *webSocketAPIBuilder) setupStage() {
	stageProps := &awsapigatewayv2.WebSocketStageProps{
		WebSocketApi: b.api.WebSocketApi,
		StageName:    jsii.String(b.config.stageName),
		AutoDeploy:   jsii.Bool(b.config.autoDeploy),
	}

	// Configure throttling if specified
	b.configureThrottling(stageProps)

	b.api.Stage = awsapigatewayv2.NewWebSocketStage(b.api, jsii.String("Stage"), stageProps)
}

// configureThrottling configures stage throttling settings
func (b *webSocketAPIBuilder) configureThrottling(stageProps *awsapigatewayv2.WebSocketStageProps) {
	if b.props.ThrottleRateLimit == nil && b.props.ThrottleBurstLimit == nil {
		return
	}

	throttleSettings := &awsapigatewayv2.ThrottleSettings{}
	if b.props.ThrottleRateLimit != nil {
		throttleSettings.RateLimit = b.props.ThrottleRateLimit
	}
	if b.props.ThrottleBurstLimit != nil {
		throttleSettings.BurstLimit = b.props.ThrottleBurstLimit
	}
	stageProps.Throttle = throttleSettings
}

// REMOVED: createStandardFunctions - Functions must now be created externally to avoid deep nesting

// AddRoute adds a new route to the WebSocket API
func (w *WebSocketAPI) AddRoute(routeKey string, function awslambda.IFunction, config *WebSocketRouteConfig) awsapigatewayv2.WebSocketRoute {
	// Sanitize route key for naming
	sanitizedName := strings.ReplaceAll(routeKey, "$", "")
	sanitizedName = strings.ReplaceAll(sanitizedName, "/", "")

	// Create Lambda integration - use minimal ID
	// Use single letter for standard routes to minimize nesting
	shortId := ""
	switch routeKey {
	case "$connect":
		shortId = "C"
	case "$disconnect":
		shortId = "D"
	case defaultRoute:
		shortId = "X"
	default:
		// For custom routes, use first letter or two
		if len(sanitizedName) > 0 {
			shortId = string(sanitizedName[0])
		} else {
			shortId = "R"
		}
	}

	integration := awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
		jsii.String(shortId),
		function,
		nil,
	)

	// Build route options
	routeOptions := &awsapigatewayv2.WebSocketRouteOptions{
		Integration: integration,
	}

	// Add authorizer if specified
	if config != nil && config.Authorizer != nil {
		routeOptions.Authorizer = config.Authorizer
	}

	// Create the route
	route := w.WebSocketApi.AddRoute(jsii.String(routeKey), routeOptions)

	// Store in routes map
	w.Routes[routeKey] = route

	return route
}

// GrantConnectionManagement grants permissions to manage WebSocket connections
func (w *WebSocketAPI) GrantConnectionManagement(grantee awsiam.IGrantable) awsiam.Grant {
	// Grant API Gateway management permissions
	apiPolicy := awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: jsii.Strings(
			"execute-api:ManageConnections",
			"execute-api:Invoke",
		),
		Resources: jsii.Strings(fmt.Sprintf("arn:aws:execute-api:*:*:%s/*/*", *w.WebSocketApi.ApiId())),
	})

	grantee.GrantPrincipal().AddToPrincipalPolicy(apiPolicy)

	// Grant connection table permissions
	if w.ConnectionTable != nil {
		w.ConnectionTable.GrantConnectionManagement(grantee)
	}

	// Return a simple grant
	return awsiam.Grant_AddToPrincipal(&awsiam.GrantOnPrincipalOptions{
		Grantee:      grantee,
		Actions:      &[]*string{jsii.String("execute-api:ManageConnections")},
		ResourceArns: &[]*string{jsii.String(fmt.Sprintf("arn:aws:execute-api:*:*:%s/*/*", *w.WebSocketApi.ApiId()))},
	})
}

// GrantApiInvoke grants permission to invoke the WebSocket API
func (w *WebSocketAPI) GrantApiInvoke(grantee awsiam.IGrantable) awsiam.Grant {
	// Add policy statement for API invoke permissions
	apiPolicy := awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect:    awsiam.Effect_ALLOW,
		Actions:   jsii.Strings("execute-api:Invoke"),
		Resources: jsii.Strings(fmt.Sprintf("arn:aws:execute-api:*:*:%s/*/*", *w.WebSocketApi.ApiId())),
	})

	grantee.GrantPrincipal().AddToPrincipalPolicy(apiPolicy)

	return awsiam.Grant_AddToPrincipal(&awsiam.GrantOnPrincipalOptions{
		Grantee:      grantee,
		Actions:      &[]*string{jsii.String("execute-api:Invoke")},
		ResourceArns: &[]*string{jsii.String(fmt.Sprintf("arn:aws:execute-api:*:*:%s/*/*", *w.WebSocketApi.ApiId()))},
	})
}

// REMOVED: AddEnvironmentVariable - Functions are now managed externally
// Add environment variables directly to the Lambda functions you create

// grantApiGatewayInvokePermissions grants API Gateway permission to invoke Lambda functions
func (w *WebSocketAPI) grantApiGatewayInvokePermissions() {
	// Permissions are now created automatically by WebSocketLambdaIntegration
	// when routes are added. This avoids duplicate permissions.
}

// REMOVED: setupEnvironmentVariables - Set environment variables directly on your Lambda functions
// Use GetWebSocketURL(), GetConnectionTableName(), etc. to get values to set on your functions

// GetConnectionTableName returns the connection table name
func (w *WebSocketAPI) GetConnectionTableName() *string {
	if w.ConnectionTable != nil {
		return w.ConnectionTable.Table.TableName()
	}
	return nil
}

// GetWebSocketURL returns the WebSocket URL
func (w *WebSocketAPI) GetWebSocketURL() *string {
	url := fmt.Sprintf("wss://%s.execute-api.%s.amazonaws.com/%s",
		*w.WebSocketApi.ApiId(),
		*w.WebSocketApi.Stack().Region(),
		*w.Stage.StageName(),
	)
	return jsii.String(url)
}
