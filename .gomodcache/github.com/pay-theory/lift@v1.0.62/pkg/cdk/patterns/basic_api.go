package patterns

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// BasicAPIProps defines properties for creating a basic API pattern
type BasicAPIProps struct {
	// API name
	ApiName *string
	// Lambda function code
	Code awslambda.Code
	// Handler (defaults to "bootstrap" for Go)
	Handler *string
	// Enable CORS
	EnableCORS *bool
	// Enable monitoring with CloudWatch dashboard
	EnableMonitoring *bool
	// Memory size in MB (default: 512)
	MemorySize *float64
	// Timeout in seconds (default: 30)
	Timeout *float64
	// Environment variables
	Environment *map[string]*string
}

// BasicAPI is a pattern that creates an API Gateway with a Lambda function backend
type BasicAPI struct {
	constructs.Construct
	Api      *liftconstructs.LiftAPI
	Function *liftconstructs.LiftFunction
}

// NewBasicAPI creates a new basic API pattern with sensible defaults
func NewBasicAPI(scope constructs.Construct, id *string, props *BasicAPIProps) *BasicAPI {
	this := constructs.NewConstruct(scope, id)

	// Set defaults
	if props.Handler == nil {
		props.Handler = jsii.String("bootstrap")
	}
	if props.EnableCORS == nil {
		props.EnableCORS = jsii.Bool(true)
	}
	if props.EnableMonitoring == nil {
		props.EnableMonitoring = jsii.Bool(true)
	}

	// Create Lambda function
	functionProps := &liftconstructs.LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			Code:        props.Code,
			Handler:     props.Handler,
			Environment: props.Environment,
		},
		EnableTracing: jsii.Bool(true),
		EnableMetrics: jsii.Bool(true),
	}

	if props.MemorySize != nil {
		functionProps.MemorySize = props.MemorySize
	}
	if props.Timeout != nil {
		functionProps.Timeout = awscdk.Duration_Seconds(props.Timeout)
	}

	fn := liftconstructs.NewLiftFunction(this, jsii.String("Function"), functionProps)

	// Create API Gateway
	api := liftconstructs.NewLiftAPI(this, jsii.String("Api"), &liftconstructs.LiftAPIProps{
		APICommonProps: liftconstructs.APICommonProps{
			Name:                props.ApiName,
			EnableCORS:          props.EnableCORS,
			EnableAccessLogging: jsii.Bool(true),
		},
	})

	// Add Lambda integration
	api.AddLambdaRoute(jsii.String("/{proxy+}"), awsapigatewayv2.HttpMethod_ANY, fn.Function)

	// Add monitoring if enabled
	if *props.EnableMonitoring {
		// Create CloudWatch dashboard
		dashboardName := *props.ApiName + "-dashboard"
		dashboard := awscloudwatch.NewDashboard(this, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
			DashboardName: jsii.String(dashboardName),
		})

		// Add widgets for API metrics
		dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("API Requests"),
				Left: &[]awscloudwatch.IMetric{
					api.HttpAPI.Metric(jsii.String("Count"), nil),
				},
			}),
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("API Latency"),
				Left: &[]awscloudwatch.IMetric{
					api.HttpAPI.Metric(jsii.String("Latency"), &awscloudwatch.MetricOptions{
						Statistic: jsii.String("Average"),
					}),
				},
			}),
		)

		// Add Lambda metrics
		dashboard.AddWidgets(
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("Lambda Invocations"),
				Left: &[]awscloudwatch.IMetric{
					fn.Function.MetricInvocations(nil),
				},
			}),
			awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
				Title: jsii.String("Lambda Errors"),
				Left: &[]awscloudwatch.IMetric{
					fn.Function.MetricErrors(nil),
				},
			}),
		)
	}

	return &BasicAPI{
		Construct: this,
		Api:       api,
		Function:  fn,
	}
}

// GetApiUrl returns the API URL
func (api *BasicAPI) GetApiUrl() *string {
	return api.Api.GetUrl()
}

// GetFunction returns the Lambda function
func (api *BasicAPI) GetFunction() awslambda.Function {
	return api.Function.Function
}

// GetApi returns the API Gateway construct
func (api *BasicAPI) GetApi() *liftconstructs.LiftAPI {
	return api.Api
}

// AddRoute adds a new route to the API
func (api *BasicAPI) AddRoute(path *string, method awsapigatewayv2.HttpMethod, handler awslambda.IFunction) {
	api.Api.AddLambdaRoute(path, method, handler)
}
