package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/jsii-runtime-go"
)

func TestLiftEventSourceMapping_DirectMapping(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a mock Lambda function
	fn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_18_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	esm := NewLiftEventSourceMapping(stack, jsii.String("EventSourceMapping"), &LiftEventSourceMappingProps{
		TargetFunction:        fn,
		EventSourceArn:        jsii.String("arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000"),
		StartingPosition:      awslambda.StartingPosition_LATEST,
		BatchSize:             jsii.Number(10),
		RetryAttempts:         jsii.Number(3),
		ParallelizationFactor: jsii.Number(1),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify event source mapping was created
	template.ResourceCountIs(jsii.String("AWS::Lambda::EventSourceMapping"), jsii.Number(1))

	// Verify mapping properties
	template.HasResourceProperties(jsii.String("AWS::Lambda::EventSourceMapping"), map[string]any{
		"EventSourceArn":        "arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000",
		"StartingPosition":      "LATEST",
		"BatchSize":             float64(10),
		"MaximumRetryAttempts":  float64(3),
		"ParallelizationFactor": float64(1),
	})

	if esm.EventSourceMapping == nil {
		t.Error("Expected EventSourceMapping to be created")
	}
	if esm.CustomResource != nil {
		t.Error("Expected CustomResource to be nil for direct mapping")
	}
}

func TestLiftEventSourceMapping_CustomResourceMapping(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a mock Lambda function
	fn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_18_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	esm := NewLiftEventSourceMapping(stack, jsii.String("EventSourceMapping"), &LiftEventSourceMappingProps{
		TargetFunction:        fn,
		TableName:             jsii.String("test-table"),
		UseCustomResource:     jsii.Bool(true),
		StartingPosition:      awslambda.StartingPosition_LATEST,
		BatchSize:             jsii.Number(10),
		RetryAttempts:         jsii.Number(3),
		ParallelizationFactor: jsii.Number(1),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify custom resource handler Lambda was created
	template.ResourceCountIs(jsii.String("AWS::Lambda::Function"), jsii.Number(2)) // Target + handler

	// Verify IAM role for custom resource
	template.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]any{
		"AssumeRolePolicyDocument": map[string]any{
			"Statement": []any{
				map[string]any{
					"Action": "sts:AssumeRole",
					"Effect": "Allow",
					"Principal": map[string]any{
						"Service": "lambda.amazonaws.com",
					},
				},
			},
		},
	})

	// Verify custom resource was created
	template.ResourceCountIs(jsii.String("AWS::CloudFormation::CustomResource"), jsii.Number(1))

	if esm.CustomResource == nil {
		t.Error("Expected CustomResource to be created")
	}
	if esm.CustomResourceHandler == nil {
		t.Error("Expected CustomResourceHandler to be created")
	}
	if esm.EventSourceMapping != nil {
		t.Error("Expected EventSourceMapping to be nil for custom resource mapping")
	}
}

func TestLiftEventSourceMapping_WithOptionalParams(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	fn := awslambda.NewFunction(stack, jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_18_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => {}")),
	})

	// WHEN
	esm := NewLiftEventSourceMapping(stack, jsii.String("EventSourceMapping"), &LiftEventSourceMappingProps{
		TargetFunction:          fn,
		EventSourceArn:          jsii.String("arn:aws:dynamodb:us-east-1:123456789012:table/test-table/stream/2024-01-01T00:00:00.000"),
		StartingPosition:        awslambda.StartingPosition_LATEST,
		BatchSize:               jsii.Number(20),
		RetryAttempts:           jsii.Number(5),
		ParallelizationFactor:   jsii.Number(2),
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(10)),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		MaxRecordAge:            awscdk.Duration_Hours(jsii.Number(24)),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify event source mapping with all optional properties
	template.HasResourceProperties(jsii.String("AWS::Lambda::EventSourceMapping"), map[string]any{
		"BatchSize":                      float64(20),
		"MaximumRetryAttempts":           float64(5),
		"ParallelizationFactor":          float64(2),
		"MaximumBatchingWindowInSeconds": float64(10),
		"BisectBatchOnFunctionError":     true,
		"FunctionResponseTypes":          []any{"ReportBatchItemFailures"},
		"MaximumRecordAgeInSeconds":      float64(86400),
	})

	if esm.EventSourceMapping == nil {
		t.Error("Expected EventSourceMapping to be created")
	}
}
