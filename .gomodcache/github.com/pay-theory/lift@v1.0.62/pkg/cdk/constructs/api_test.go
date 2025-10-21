package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLiftAPI(t *testing.T) {
	tests := []struct {
		props      *LiftAPIProps
		assertions func(t *testing.T, template assertions.Template)
		name       string
	}{
		{
			name: "creates basic API",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name:        jsii.String("test-api"),
					Description: jsii.String("Test API"),
				},
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Api"), &map[string]interface{}{
					"Name":         "test-api",
					"Description":  "Test API",
					"ProtocolType": "HTTP",
				})
			},
		},
		{
			name: "enables CORS",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name:       jsii.String("test-api"),
					EnableCORS: jsii.Bool(true),
				},
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Api"), &map[string]interface{}{
					"CorsConfiguration": &map[string]interface{}{
						"AllowOrigins": &[]interface{}{"*"},
						"AllowMethods": assertions.Match_AnyValue(),
						"AllowHeaders": assertions.Match_ArrayWith(&[]interface{}{
							"Content-Type",
							"Authorization",
							"X-Tenant-ID",
							"X-Request-ID",
							"X-Api-Key",
						}),
						"ExposeHeaders": assertions.Match_ArrayWith(&[]interface{}{
							"X-Request-ID",
							"X-Rate-Limit-Limit",
							"X-Rate-Limit-Remaining",
							"X-Rate-Limit-Reset",
						}),
						"MaxAge": 86400,
					},
				})
			},
		},
		{
			name: "creates custom domain",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name:           jsii.String("test-api"),
					DomainName:     jsii.String("api.example.com"),
					CertificateArn: jsii.String("arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"),
				},
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::DomainName"), &map[string]interface{}{
					"DomainName": "api.example.com",
					"DomainNameConfigurations": &[]interface{}{
						&map[string]interface{}{
							"CertificateArn": "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
						},
					},
				})
				template.HasResource(jsii.String("AWS::ApiGatewayV2::ApiMapping"), &map[string]interface{}{})
			},
		},
		{
			name: "enables access logging",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name:                jsii.String("test-api"),
					EnableAccessLogging: jsii.Bool(true),
				},
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				// Check log group is created
				template.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), &map[string]interface{}{
					"LogGroupName":    "/aws/apigateway/test-api",
					"RetentionInDays": 7,
				})
				// Check stage has access log settings
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{
					"AccessLogSettings": &map[string]interface{}{
						"DestinationArn": assertions.Match_AnyValue(),
						"Format":         assertions.Match_AnyValue(),
					},
				})
			},
		},
		{
			name: "configures throttling",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name:               jsii.String("test-api"),
					ThrottleRateLimit:  jsii.Number(100),
					ThrottleBurstLimit: jsii.Number(200),
				},
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{
					"DefaultRouteSettings": &map[string]interface{}{
						"ThrottlingRateLimit":  100,
						"ThrottlingBurstLimit": 200,
					},
				})
			},
		},
		{
			name: "creates custom stage",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name:      jsii.String("test-api"),
					StageName: jsii.String("prod"),
				},
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{
					"StageName":  "prod",
					"AutoDeploy": true,
				})
			},
		},
		{
			name: "enables detailed metrics",
			props: &LiftAPIProps{
				APICommonProps: APICommonProps{
					Name: jsii.String("test-api"),
				},
				EnableDetailedMetrics: jsii.Bool(true),
			},
			assertions: func(_ *testing.T, template assertions.Template) {
				template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{
					"DetailedMetricsEnabled": true,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test stack
			stack := test.NewTestStack()

			// Create API
			NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), tt.props)

			// Get template
			template := assertions.Template_FromStack(stack.Stack(), nil)

			// Run assertions
			tt.assertions(t, template)
		})
	}
}

func TestLiftAPI_AddLambdaRoute(_ *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create API
	api := NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name: jsii.String("test-api"),
		},
	})

	// Create test Lambda function
	fn := awslambda.NewFunction(stack.Stack(), jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2023(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Add route
	api.AddLambdaRoute(jsii.String("/test"), awsapigatewayv2.HttpMethod_GET, fn)

	// Get template
	template := assertions.Template_FromStack(stack.Stack(), nil)

	// Assert route is created
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), &map[string]interface{}{
		"RouteKey": "GET /test",
		"Target":   assertions.Match_AnyValue(),
	})

	// Assert integration is created
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Integration"), &map[string]interface{}{
		"IntegrationType":      "AWS_PROXY",
		"PayloadFormatVersion": "2.0",
		"IntegrationUri":       assertions.Match_AnyValue(),
	})
}

func TestLiftAPI_AddLambdaRouteWithOptions(_ *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create API
	api := NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name: jsii.String("test-api"),
		},
	})

	// Create test Lambda function
	fn := awslambda.NewFunction(stack.Stack(), jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2023(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Add route with options
	api.AddLambdaRouteWithOptions(
		jsii.String("/protected"),
		awsapigatewayv2.HttpMethod_POST,
		fn,
		&RouteOptions{
			// Note: Authorizer would be set here in real usage
		},
	)

	// Get template
	template := assertions.Template_FromStack(stack.Stack(), nil)

	// Assert route is created
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Route"), &map[string]interface{}{
		"RouteKey": "POST /protected",
	})
}

func TestLiftAPI_AddRoutes(_ *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create API
	api := NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name: jsii.String("test-api"),
		},
	})

	// Create test Lambda functions
	fn1 := awslambda.NewFunction(stack.Stack(), jsii.String("Function1"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2023(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	fn2 := awslambda.NewFunction(stack.Stack(), jsii.String("Function2"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2023(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Add multiple routes
	routes := map[string]map[string]awslambda.IFunction{
		"/users": {
			"GET":  fn1,
			"POST": fn2,
		},
		"/items": {
			"GET": fn1,
		},
	}
	api.AddRoutes(routes)

	// Get template
	template := assertions.Template_FromStack(stack.Stack(), nil)

	// Assert routes are created
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Route"), jsii.Number(3))
}

func TestLiftAPI_GetUrl(t *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create API
	api := NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name: jsii.String("test-api"),
		},
	})

	// Get URL
	url := api.GetUrl()
	require.NotNil(t, url)

	// URL should contain Fn::Join for the API URL
	urlStr := *url
	assert.NotEmpty(t, urlStr)
}

func TestLiftAPI_GetUrl_CustomStage(t *testing.T) {
	stack := test.NewTestStack()

	api := NewLiftAPI(stack.Stack(), jsii.String("CustomStageAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name:      jsii.String("custom-stage-api"),
			StageName: jsii.String("prod"),
		},
	})

	url := api.GetUrl()
	require.NotNil(t, url)
	require.Contains(t, *url, "prod")
}

func TestLiftAPI_GetArn(t *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create API
	api := NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name: jsii.String("test-api"),
		},
	})

	// Get ARN
	arn := api.GetArn()
	require.NotNil(t, arn)
}

func TestLiftAPI_Integration(_ *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create API with all features
	api := NewLiftAPI(stack.Stack(), jsii.String("TestAPI"), &LiftAPIProps{
		APICommonProps: APICommonProps{
			Name:                jsii.String("test-api"),
			Description:         jsii.String("Test API with all features"),
			EnableCORS:          jsii.Bool(true),
			EnableAccessLogging: jsii.Bool(true),
			ThrottleRateLimit:   jsii.Number(1000),
			ThrottleBurstLimit:  jsii.Number(2000),
			StageName:           jsii.String("prod"),
		},
		EnableDetailedMetrics: jsii.Bool(true),
	})

	// Create Lambda function
	fn := awslambda.NewFunction(stack.Stack(), jsii.String("TestFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_PROVIDED_AL2023(),
		Handler: jsii.String("bootstrap"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Add multiple routes
	api.AddLambdaRoute(jsii.String("/users"), awsapigatewayv2.HttpMethod_GET, fn)
	api.AddLambdaRoute(jsii.String("/users"), awsapigatewayv2.HttpMethod_POST, fn)
	api.AddLambdaRoute(jsii.String("/users/:id"), awsapigatewayv2.HttpMethod_GET, fn)
	api.AddLambdaRoute(jsii.String("/users/:id"), awsapigatewayv2.HttpMethod_PUT, fn)
	api.AddLambdaRoute(jsii.String("/users/:id"), awsapigatewayv2.HttpMethod_DELETE, fn)

	// Get template
	template := assertions.Template_FromStack(stack.Stack(), nil)

	// Assert API is created with all features
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Api"), &map[string]interface{}{
		"Name":              "test-api",
		"Description":       "Test API with all features",
		"ProtocolType":      "HTTP",
		"CorsConfiguration": assertions.Match_AnyValue(),
	})

	// Assert stage is configured - check that we have a stage with prod name
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{
		"StageName":              "prod",
		"AutoDeploy":             true,
		"DetailedMetricsEnabled": true,
		"DefaultRouteSettings": &map[string]interface{}{
			"ThrottlingRateLimit":  1000,
			"ThrottlingBurstLimit": 2000,
		},
		"AccessLogSettings": assertions.Match_AnyValue(),
	})

	// Assert log group is created
	template.HasResource(jsii.String("AWS::Logs::LogGroup"), &map[string]interface{}{})

	// Assert routes are created
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Route"), jsii.Number(5))
}
