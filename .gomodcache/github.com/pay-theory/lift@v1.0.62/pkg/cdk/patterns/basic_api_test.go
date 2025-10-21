package patterns

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

func TestNewBasicAPI_DefaultConfiguration(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	basicAPI := NewBasicAPI(stack, jsii.String("BasicAPI"), &BasicAPIProps{
		ApiName: jsii.String("test-api"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify Lambda function exists
	template.ResourceCountIs(jsii.String("AWS::Lambda::Function"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"Runtime": "provided.al2023",
		"Handler": "bootstrap",
		"TracingConfig": map[string]interface{}{
			"Mode": "Active",
		},
	})

	// Verify API Gateway exists
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Api"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Api"), &map[string]interface{}{
		"Name":         "test-api",
		"ProtocolType": "HTTP",
		"CorsConfiguration": map[string]interface{}{
			"AllowOrigins": []interface{}{"*"},
			"AllowMethods": []interface{}{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		},
	})

	// Verify route exists
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Route"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), &map[string]interface{}{
		"RouteKey": "ANY /{proxy+}",
	})

	// Verify integration exists
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Integration"), jsii.Number(1))

	// Verify stage exists (API Gateway may create multiple stages)
	// Just verify they exist
	template.HasResource(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{})

	// Verify CloudWatch dashboard exists (monitoring enabled by default)
	template.ResourceCountIs(jsii.String("AWS::CloudWatch::Dashboard"), jsii.Number(1))

	assert.NotNil(t, basicAPI)
	assert.NotNil(t, basicAPI.Api)
	assert.NotNil(t, basicAPI.Function)
}

func TestNewBasicAPI_DisableCORS(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewBasicAPI(stack, jsii.String("BasicAPI"), &BasicAPIProps{
		ApiName:    jsii.String("test-api"),
		Code:       awslambda.Code_FromAsset(jsii.String("."), nil),
		EnableCORS: jsii.Bool(false),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify API exists without CORS
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Api"), jsii.Number(1))

	// Check that CORS is not configured
	fnResource := template.ToJSON()
	resources, ok := (*fnResource)["Resources"].(map[string]interface{})
	if !ok {
		t.Fatal("Template should have Resources")
	}

	hasCORS := false
	for _, resource := range resources {
		if resMap, ok := resource.(map[string]interface{}); ok {
			if resMap["Type"] == "AWS::ApiGatewayV2::Api" {
				if props, ok := resMap["Properties"].(map[string]interface{}); ok {
					if _, ok := props["CorsConfiguration"]; ok {
						hasCORS = true
					}
				}
			}
		}
	}

	assert.False(t, hasCORS, "CORS should not be configured")
}

func TestNewBasicAPI_DisableMonitoring(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewBasicAPI(stack, jsii.String("BasicAPI"), &BasicAPIProps{
		ApiName:          jsii.String("test-api"),
		Code:             awslambda.Code_FromAsset(jsii.String("."), nil),
		EnableMonitoring: jsii.Bool(false),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify no dashboard is created
	template.ResourceCountIs(jsii.String("AWS::CloudWatch::Dashboard"), jsii.Number(0))
}

func TestNewBasicAPI_CustomConfiguration(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	env := map[string]*string{
		"ENV_VAR": jsii.String("value"),
	}

	// When
	NewBasicAPI(stack, jsii.String("BasicAPI"), &BasicAPIProps{
		ApiName:     jsii.String("test-api"),
		Code:        awslambda.Code_FromAsset(jsii.String("."), nil),
		Handler:     jsii.String("main"),
		MemorySize:  jsii.Number(1024),
		Timeout:     jsii.Number(60),
		Environment: &env,
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify Lambda configuration
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"Handler":    "main",
		"MemorySize": 1024,
		"Timeout":    60,
		"Environment": map[string]interface{}{
			"Variables": map[string]interface{}{
				"ENV_VAR": "value",
			},
		},
	})

	// Verify no dead letter queue
	template.ResourceCountIs(jsii.String("AWS::SQS::Queue"), jsii.Number(0))

	// Verify log retention
	template.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), &map[string]interface{}{
		"RetentionInDays": 7,
	})
}

func TestBasicAPI_AddRoute(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	basicAPI := NewBasicAPI(stack, jsii.String("BasicAPI"), &BasicAPIProps{
		ApiName: jsii.String("test-api"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Create another Lambda function for the new route
	additionalFn := awslambda.NewFunction(stack, jsii.String("AdditionalFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2023(),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
		Handler: jsii.String("bootstrap"),
	})

	// When
	basicAPI.AddRoute(jsii.String("/custom"), awsapigatewayv2.HttpMethod_GET, additionalFn)

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should have 2 routes now (default + custom)
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Route"), jsii.Number(2))

	// Verify custom route exists
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), &map[string]interface{}{
		"RouteKey": "GET /custom",
	})
}

func TestBasicAPI_GettersWork(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	basicAPI := NewBasicAPI(stack, jsii.String("BasicAPI"), &BasicAPIProps{
		ApiName: jsii.String("test-api"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Then
	assert.NotNil(t, basicAPI.GetApiUrl())
	assert.NotNil(t, basicAPI.GetFunction())
	assert.NotNil(t, basicAPI.GetApi())
}
