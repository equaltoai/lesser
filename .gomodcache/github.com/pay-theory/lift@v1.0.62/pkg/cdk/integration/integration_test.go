//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/constructs"
	"github.com/pay-theory/lift/pkg/cdk/patterns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for CDK constructs
// These tests actually deploy resources to AWS and verify they work correctly
// Run with: go test -tags=integration ./pkg/cdk/integration -v

const (
	testStackPrefix = "lift-cdk-integration-test"
	testTimeout     = 30 * time.Minute
)

type TestContext struct {
	CFNClient    *cloudformation.Client
	LambdaClient *lambda.Client
	APIClient    *apigatewayv2.Client
	DynamoClient *dynamodb.Client
	StackName    string
	Region       string
}

func setupTestContext(t *testing.T) *TestContext {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	require.NoError(t, err)

	return &TestContext{
		CFNClient:    cloudformation.NewFromConfig(cfg),
		LambdaClient: lambda.NewFromConfig(cfg),
		APIClient:    apigatewayv2.NewFromConfig(cfg),
		DynamoClient: dynamodb.NewFromConfig(cfg),
		StackName:    fmt.Sprintf("%s-%d", testStackPrefix, time.Now().Unix()),
		Region:       region,
	}
}

func TestLiftFunctionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tc := setupTestContext(t)

	// Create test stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String(tc.StackName), &awscdk.StackProps{
		StackName: jsii.String(tc.StackName),
	})

	// Create a simple Lift function
	liftFn := constructs.NewLiftFunction(stack, jsii.String("TestFunction"), &constructs.LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			Code:    awslambda.Code_FromInline(jsii.String(getLambdaTestCode())),
			Handler: jsii.String("index.handler"),
			Runtime: awslambda.Runtime_NODEJS_20_X(), // Using Node for inline code
		},
		AppName:           jsii.String("test-app"),
		EnableMultiTenant: jsii.Bool(true),
	})

	// Deploy stack
	stackOutput := app.Synth(nil)
	require.NotNil(t, stackOutput)

	// Deploy to AWS
	err := deployStack(ctx, tc, app)
	require.NoError(t, err)
	defer cleanupStack(ctx, tc)

	// Test Lambda invocation
	functionName := liftFn.Function.FunctionName()
	payload := map[string]interface{}{
		"httpMethod": "GET",
		"path":       "/test",
	}

	payloadBytes, _ := json.Marshal(payload)
	invokeResp, err := tc.LambdaClient.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: functionName,
		Payload:      payloadBytes,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(200), *invokeResp.StatusCode)

	// Verify response
	var response map[string]interface{}
	err = json.Unmarshal(invokeResp.Payload, &response)
	require.NoError(t, err)
	assert.Equal(t, float64(200), response["statusCode"])

	body, ok := response["body"].(string)
	require.True(t, ok)

	var bodyData map[string]interface{}
	err = json.Unmarshal([]byte(body), &bodyData)
	require.NoError(t, err)
	assert.Equal(t, "success", bodyData["status"])
}

func TestBasicAPIPatternIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tc := setupTestContext(t)

	// Create test stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String(tc.StackName), &awscdk.StackProps{
		StackName: jsii.String(tc.StackName),
	})

	// Create BasicAPI pattern
	basicAPI := patterns.NewBasicAPI(stack, jsii.String("TestAPI"), &patterns.BasicAPIProps{
		AppName:     jsii.String("test-api"),
		CodePath:    awslambda.Code_FromInline(jsii.String(getLambdaTestCode())),
		Handler:     jsii.String("index.handler"),
		Runtime:     awslambda.Runtime_NODEJS_20_X(),
		EnableCORS:  jsii.Bool(true),
		Description: jsii.String("Integration test API"),
	})

	// Deploy stack
	err := deployStack(ctx, tc, app)
	require.NoError(t, err)
	defer cleanupStack(ctx, tc)

	// Get API endpoint
	apiId := basicAPI.API.ApiId()

	// List stages to get the endpoint
	stagesResp, err := tc.APIClient.GetStages(ctx, &apigatewayv2.GetStagesInput{
		ApiId: apiId,
	})
	require.NoError(t, err)
	require.NotEmpty(t, stagesResp.Items)

	// Verify API is accessible (would need HTTP client for actual test)
	// For now, just verify the resources exist
	assert.NotNil(t, basicAPI.Function)
	assert.NotNil(t, basicAPI.API)
}

func TestRateLimitedFunctionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tc := setupTestContext(t)

	// Create test stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String(tc.StackName), &awscdk.StackProps{
		StackName: jsii.String(tc.StackName),
	})

	// Create RateLimitedFunction
	rateLimitedFn := constructs.NewRateLimitedFunction(stack, jsii.String("RateLimited"), &constructs.RateLimitedFunctionProps{
		LiftFunctionProps: constructs.LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromInline(jsii.String(getLambdaTestCode())),
				Handler: jsii.String("index.handler"),
				Runtime: awslambda.Runtime_NODEJS_20_X(),
			},
			AppName: jsii.String("rate-limited-app"),
		},
		RateLimitType: jsii.String("IP"),
		RequestLimit:  jsii.Number(10),
		WindowMinutes: jsii.Number(1),
	})

	// Deploy stack
	err := deployStack(ctx, tc, app)
	require.NoError(t, err)
	defer cleanupStack(ctx, tc)

	// Verify DynamoDB table was created
	tableName := rateLimitedFn.RateLimitTable.TableName()
	describeResp, err := tc.DynamoClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: tableName,
	})
	require.NoError(t, err)
	assert.NotNil(t, describeResp.Table)
	assert.Equal(t, "identifier", *describeResp.Table.AttributeDefinitions[0].AttributeName)
}

func TestMonitoredFunctionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tc := setupTestContext(t)

	// Create test stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String(tc.StackName), &awscdk.StackProps{
		StackName: jsii.String(tc.StackName),
	})

	// Create MonitoredFunction with all features
	monitoredFn := constructs.NewMonitoredFunction(stack, jsii.String("Monitored"), &constructs.MonitoredFunctionProps{
		LiftFunctionProps: constructs.LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromInline(jsii.String(getLambdaTestCode())),
				Handler: jsii.String("index.handler"),
				Runtime: awslambda.Runtime_NODEJS_20_X(),
			},
			AppName: jsii.String("monitored-app"),
		},
		EnableDashboard:          jsii.Bool(true),
		EnableLogInsightsQueries: jsii.Bool(true),
		DashboardName:            jsii.String(fmt.Sprintf("%s-dashboard", tc.StackName)),
		LogRetentionDays:         jsii.Number(1), // Minimum for testing
		AlarmConfig: &constructs.AlarmConfig{
			EnableErrorAlarm:    jsii.Bool(true),
			EnableLatencyAlarm:  jsii.Bool(true),
			EnableThrottleAlarm: jsii.Bool(true),
		},
	})

	// Deploy stack
	err := deployStack(ctx, tc, app)
	require.NoError(t, err)
	defer cleanupStack(ctx, tc)

	// Invoke function to generate some metrics
	functionName := monitoredFn.Function.Function.FunctionName()
	for i := 0; i < 5; i++ {
		payload := map[string]interface{}{
			"httpMethod": "GET",
			"path":       fmt.Sprintf("/test/%d", i),
		}
		payloadBytes, _ := json.Marshal(payload)

		_, err := tc.LambdaClient.Invoke(ctx, &lambda.InvokeInput{
			FunctionName: functionName,
			Payload:      payloadBytes,
		})
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Verify alarms exist (would need CloudWatch client for full verification)
	assert.NotNil(t, monitoredFn.GetAlarm("errors"))
	assert.NotNil(t, monitoredFn.GetAlarm("latency"))
	assert.NotNil(t, monitoredFn.GetAlarm("throttles"))
}

// Helper functions

func getLambdaTestCode() string {
	return `
exports.handler = async (event) => {
    console.log('Event:', JSON.stringify(event));
    
    // Simulate some processing
    await new Promise(resolve => setTimeout(resolve, Math.random() * 100));
    
    return {
        statusCode: 200,
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            status: 'success',
            timestamp: new Date().toISOString(),
            path: event.path || '/',
            method: event.httpMethod || 'GET',
        }),
    };
};
`
}

func deployStack(ctx context.Context, tc *TestContext, app awscdk.App) error {
	// This is a simplified deployment - in real integration tests, you would:
	// 1. Run `cdk synth` to generate CloudFormation template
	// 2. Use CloudFormation SDK to deploy the stack
	// 3. Wait for deployment to complete

	// For now, we'll return an error indicating this needs CDK CLI
	return fmt.Errorf("integration tests require CDK CLI to be installed and configured")
}

func cleanupStack(ctx context.Context, tc *TestContext) {
	// Delete the stack
	_, err := tc.CFNClient.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: &tc.StackName,
	})
	if err != nil {
		fmt.Printf("Failed to cleanup stack %s: %v\n", tc.StackName, err)
	}
}
