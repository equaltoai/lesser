package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

// createTestWebSocketFunctions creates the required Lambda functions for WebSocket tests
func createWSTestFunctions(stack awscdk.Stack, prefix string) (awslambda.IFunction, awslambda.IFunction, awslambda.IFunction) {
	connectFunction := awslambda.NewFunction(stack, jsii.String(prefix+"Connect"), &awslambda.FunctionProps{
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
		Handler: jsii.String("index.handler"),
		Runtime: awslambda.Runtime_NODEJS_18_X(),
	})
	disconnectFunction := awslambda.NewFunction(stack, jsii.String(prefix+"Disconnect"), &awslambda.FunctionProps{
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
		Handler: jsii.String("index.handler"),
		Runtime: awslambda.Runtime_NODEJS_18_X(),
	})
	defaultFunction := awslambda.NewFunction(stack, jsii.String(prefix+"Default"), &awslambda.FunctionProps{
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
		Handler: jsii.String("index.handler"),
		Runtime: awslambda.Runtime_NODEJS_18_X(),
	})
	return connectFunction, disconnectFunction, defaultFunction
}

func TestWebSocketAPI_NewModularPattern(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create Lambda functions externally - this is now required
	connectFunction, disconnectFunction, defaultFunction := createWSTestFunctions(stack, "New")

	wsApi := NewWebSocketAPI(stack, jsii.String("API"), &WebSocketAPIProps{
		ApiName:                 jsii.String("modular-websocket-api"),
		ConnectRouteFunction:    connectFunction,
		DisconnectRouteFunction: disconnectFunction,
		DefaultRouteFunction:    defaultFunction,
	})

	template := synthesizeTemplate(t, stack)

	// Verify WebSocket API is created
	assertResourceExists(t, template, "AWS::ApiGatewayV2::Api")

	// Verify exactly 3 Lambda functions exist (the ones we created)
	functions := findResourcesByType(template, "AWS::Lambda::Function")
	if len(functions) != 3 {
		t.Errorf("Expected exactly 3 Lambda functions, got %d", len(functions))
	}

	// Verify routes are created
	routes := findResourcesByType(template, "AWS::ApiGatewayV2::Route")
	if len(routes) != 3 {
		t.Errorf("Expected 3 routes, got %d", len(routes))
	}

	// Verify construct properties
	if wsApi.WebSocketApi == nil {
		t.Error("WebSocket API should not be nil")
	}
	if wsApi.Stage == nil {
		t.Error("Stage should not be nil")
	}
	if wsApi.ConnectionTable == nil {
		t.Error("Connection table should not be nil")
	}
}

func TestWebSocketAPI_RequiredFunctions(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Test that missing functions cause panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when ConnectRouteFunction is nil")
		}
	}()

	NewWebSocketAPI(stack, jsii.String("API"), &WebSocketAPIProps{
		ApiName: jsii.String("test-api"),
		// Missing required functions - should panic
	})
}

func TestWebSocketAPI_HelperMethodsNew(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	connectFunction, disconnectFunction, defaultFunction := createWSTestFunctions(stack, "Helper")

	wsApi := NewWebSocketAPI(stack, jsii.String("API"), &WebSocketAPIProps{
		ApiName:                 jsii.String("test-api"),
		ConnectRouteFunction:    connectFunction,
		DisconnectRouteFunction: disconnectFunction,
		DefaultRouteFunction:    defaultFunction,
	})

	// Test GetConnectionTableName
	tableName := wsApi.GetConnectionTableName()
	if tableName == nil {
		t.Error("GetConnectionTableName should not return nil")
	}

	// Test GetWebSocketURL (note the uppercase URL)
	wsUrl := wsApi.GetWebSocketURL()
	if wsUrl == nil {
		t.Error("GetWebSocketURL should not return nil")
	}
}
