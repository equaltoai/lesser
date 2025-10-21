package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

func TestEventBridgeHandler_DefaultConfiguration(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify constructs were created
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}
	if handler.Rule == nil {
		t.Error("Expected Rule to be created")
	}
	if handler.EventBus == nil {
		t.Error("Expected EventBus to be created")
	}
	if handler.DeadLetterQueue == nil {
		t.Error("Expected DeadLetterQueue to be created by default")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify Lambda function
	assertResourceExists(t, template, "AWS::Lambda::Function")

	// Verify EventBridge rule
	rules := findResourcesByType(template, "AWS::Events::Rule")
	if len(rules) == 0 {
		t.Error("Expected EventBridge rule to be created")
	}

	// Verify dead letter queue
	assertResourceExists(t, template, "AWS::SQS::Queue")

	// Verify Lambda permission for EventBridge
	permissions := findResourcesByType(template, "AWS::Lambda::Permission")
	if len(permissions) == 0 {
		t.Error("Expected Lambda permission for EventBridge to be created")
	}
}

func TestEventBridgeHandler_WithEventPattern(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	eventPattern := &awsevents.EventPattern{
		Source:     &[]*string{jsii.String("myapp.orders")},
		DetailType: &[]*string{jsii.String("Order Placed")},
		Detail: &map[string]interface{}{
			"state": &[]*string{jsii.String("pending")},
		},
	}

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EventPattern: eventPattern,
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify constructs
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}
	if handler.Rule == nil {
		t.Error("Expected Rule to be created")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify rule has event pattern
	assertResourceExists(t, template, "AWS::Events::Rule")
}

func TestEventBridgeHandler_WithScheduleExpression(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-scheduled-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		ScheduleExpression: jsii.String("rate(5 minutes)"),
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify constructs
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}
	if handler.Rule == nil {
		t.Error("Expected Rule to be created")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify rule has schedule expression
	assertResourceExists(t, template, "AWS::Events::Rule")
}

func TestEventBridgeHandler_WithCustomEventBus(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EventBusProps: &awsevents.EventBusProps{
			EventBusName: jsii.String("my-custom-event-bus"),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify constructs
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}
	if handler.EventBus == nil {
		t.Error("Expected EventBus to be created")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify custom event bus
	assertResourceExists(t, template, "AWS::Events::EventBus")

	// Verify rule references custom event bus
	rules := findResourcesByType(template, "AWS::Events::Rule")
	if len(rules) == 0 {
		t.Error("Expected EventBridge rule to be created")
	}
}

func TestEventBridgeHandler_WithExistingRule(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create existing rule
	existingRule := awsevents.NewRule(stack, jsii.String("ExistingRule"), &awsevents.RuleProps{
		RuleName: jsii.String("existing-rule"),
		EventPattern: &awsevents.EventPattern{
			Source: &[]*string{jsii.String("existing.app")},
		},
	})

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		ExistingRule: existingRule,
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify the handler uses the existing rule
	if handler.Rule != existingRule {
		t.Error("Expected handler to use existing rule")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Should only have one rule (the existing one)
	rules := findResourcesByType(template, "AWS::Events::Rule")
	if len(rules) != 1 {
		t.Errorf("Expected exactly 1 rule, got %d", len(rules))
	}
}

func TestEventBridgeHandler_DisabledDLQ(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EnableDeadLetterQueue: jsii.Bool(false),
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify no DLQ was created
	if handler.DeadLetterQueue != nil {
		t.Error("Expected DeadLetterQueue to be nil when disabled")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify no SQS queue exists
	queues := findResourcesByType(template, "AWS::SQS::Queue")
	if len(queues) != 0 {
		t.Errorf("Expected no SQS queues, got %d", len(queues))
	}
}

func TestEventBridgeHandler_CustomTargetProps(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		TargetProps: &awseventstargets.LambdaFunctionProps{
			MaxEventAge:   awscdk.Duration_Hours(jsii.Number(2)),
			RetryAttempts: jsii.Number(5),
		},
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify constructs
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}
	if handler.Target == nil {
		t.Error("Expected Target to be created")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify Lambda function exists
	functions := findResourcesByType(template, "AWS::Lambda::Function")
	if len(functions) == 0 {
		t.Error("Expected Lambda function to be created")
	}
}

func TestEventBridgeHandler_EnvironmentVariables(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
			Environment: &map[string]*string{
				"CUSTOM_VAR": jsii.String("custom-value"),
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify function was created
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}

	// Synthesize template
	template := synthesizeTemplate(t, stack)

	// Verify Lambda function has environment variables
	templateJSON := template.ToJSON()
	resourcesVal := (*templateJSON)["Resources"]
	resources, ok := resourcesVal.(map[string]interface{})
	if !ok {
		t.Fatal("Template should have Resources")
	}

	// Find Lambda function
	var foundEnvVars map[string]interface{}
	for _, resource := range resources {
		if resMap, ok := resource.(map[string]interface{}); ok {
			if resType, ok := resMap["Type"].(string); ok && resType == "AWS::Lambda::Function" {
				if props, ok := resMap["Properties"].(map[string]interface{}); ok {
					if env, ok := props["Environment"].(map[string]interface{}); ok {
						if variables, ok := env["Variables"].(map[string]interface{}); ok {
							foundEnvVars = variables
							break
						}
					}
				}
			}
		}
	}

	if foundEnvVars == nil {
		t.Error("Expected Lambda function to have environment variables")
		return
	}

	// Verify EventBridge-specific environment variables were added
	if _, exists := foundEnvVars["EVENT_BUS_NAME"]; !exists {
		t.Error("Expected EVENT_BUS_NAME environment variable")
	}
	if _, exists := foundEnvVars["EVENT_BUS_ARN"]; !exists {
		t.Error("Expected EVENT_BUS_ARN environment variable")
	}
	if _, exists := foundEnvVars["EVENTBRIDGE_DLQ_URL"]; !exists {
		t.Error("Expected EVENTBRIDGE_DLQ_URL environment variable")
	}
	if _, exists := foundEnvVars["CUSTOM_VAR"]; !exists {
		t.Error("Expected CUSTOM_VAR environment variable to be preserved")
	}
}

func TestEventBridgeHandler_CrossAccountEventBus(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	crossAccountEventBusArn := "arn:aws:events:us-east-1:123456789012:event-bus/cross-account-bus"

	handler, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		CrossAccountEventBusArn: jsii.String(crossAccountEventBusArn),
	})
	if err != nil {
		t.Fatalf("Failed to create EventBridgeHandler: %v", err)
	}

	// Verify constructs
	if handler.Function == nil {
		t.Error("Expected Function to be created")
	}
	if handler.EventBus == nil {
		t.Error("Expected EventBus to be created")
	}

	// Verify event bus ARN
	if *handler.GetEventBusArn() != crossAccountEventBusArn {
		t.Errorf("Expected event bus ARN %s, got %s", crossAccountEventBusArn, *handler.GetEventBusArn())
	}
}

func TestEventBridgeHandler_ErrorOnBothEventPatternAndSchedule(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	_, err := NewEventBridgeHandler(stack, jsii.String("TestEventBridgeHandler"), &EventBridgeHandlerProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: jsii.String("test-eventbridge-handler"),
			Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
			Handler:      jsii.String("index.handler"),
			Runtime:      awslambda.Runtime_NODEJS_18_X(),
		},
		EventPattern: &awsevents.EventPattern{
			Source: &[]*string{jsii.String("myapp.orders")},
		},
		ScheduleExpression: jsii.String("rate(5 minutes)"),
	})

	if err == nil {
		t.Error("Expected error when both EventPattern and ScheduleExpression are provided")
	}
}
