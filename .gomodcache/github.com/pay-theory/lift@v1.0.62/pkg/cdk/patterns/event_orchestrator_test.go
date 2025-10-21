package patterns

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
)

func TestEventOrchestrator_CreatesDLQAndMonitoring(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Minimal default function props for handler creation
	defaultFn := awslambda.FunctionProps{
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {};")),
		Handler: jsii.String("index.handler"),
		Runtime: awslambda.Runtime_NODEJS_18_X(),
	}

	orch := NewEventOrchestrator(stack, jsii.String("TestOrchestrator"), &EventOrchestratorProps{
		AppName:              jsii.String("test-orch"),
		DefaultFunctionProps: defaultFn,
		EnableMonitoring:     jsii.Bool(true),
		// Provide at least one event source so a handler is created
		EventSources: []EventSourceConfig{
			{SourceName: jsii.String("orders"), ProcessingMode: jsii.String("standard")},
		},
		// Provide an explicit DLQ name so we can assert it deterministically
		DLQQueueName: jsii.String("test-orch-dlq"),
	})

	if orch == nil {
		t.Fatal("Orchestrator should be created")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert DLQ queue is created with the provided name
	template.HasResourceProperties(jsii.String("AWS::SQS::Queue"), map[string]interface{}{
		"QueueName": "test-orch-dlq",
	})

	// Assert function alarms exist for the 'orders' handler
	template.HasResourceProperties(jsii.String("AWS::CloudWatch::Alarm"), map[string]interface{}{
		"AlarmName": assertions.Match_StringLikeRegexp(jsii.String(".*orchestrator-errors-orders.*")),
	})

	// If event routing is enabled by default, throttle alarms for routing table should exist
	template.HasResourceProperties(jsii.String("AWS::CloudWatch::Alarm"), map[string]interface{}{
		"AlarmName": assertions.Match_StringLikeRegexp(jsii.String(".*routing-read-throttle")),
	})
}
