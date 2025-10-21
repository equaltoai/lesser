package main

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type BasicLiftStackProps struct {
	awscdk.StackProps
}

func NewBasicLiftStack(scope constructs.Construct, id string, props *BasicLiftStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// Create a Lift Lambda function
	liftFn := liftconstructs.NewLiftFunction(stack, jsii.String("MyLiftFunction"), &liftconstructs.LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			Code:    awslambda.Code_FromAsset(jsii.String("../../lambda"), nil),
			Handler: jsii.String("bootstrap"),
			Environment: &map[string]*string{
				"LOG_LEVEL": jsii.String("info"),
			},
			MemorySize: jsii.Number(512),
		},
		EnableTracing: jsii.Bool(true),
	})

	// Create a Lift API Gateway
	api := liftconstructs.NewLiftAPI(stack, jsii.String("MyLiftAPI"), &liftconstructs.LiftAPIProps{
		APICommonProps: liftconstructs.APICommonProps{
			Name:        jsii.String("my-lift-api"),
			Description: jsii.String("Example Lift API deployed with CDK"),
			EnableCORS:  jsii.Bool(true),
		},
	})

	// Add routes
	api.AddLambdaRoute(jsii.String("/users"), awsapigatewayv2.HttpMethod_GET, liftFn.Function)
	api.AddLambdaRoute(jsii.String("/users"), awsapigatewayv2.HttpMethod_POST, liftFn.Function)
	api.AddLambdaRoute(jsii.String("/users/{id}"), awsapigatewayv2.HttpMethod_GET, liftFn.Function)
	api.AddLambdaRoute(jsii.String("/users/{id}"), awsapigatewayv2.HttpMethod_PUT, liftFn.Function)
	api.AddLambdaRoute(jsii.String("/users/{id}"), awsapigatewayv2.HttpMethod_DELETE, liftFn.Function)

	// Output the API URL
	awscdk.NewCfnOutput(stack, jsii.String("ApiUrl"), &awscdk.CfnOutputProps{
		Value:       api.GetUrl(),
		Description: jsii.String("API Gateway endpoint URL"),
	})

	return stack
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	NewBasicLiftStack(app, "BasicLiftStack", &BasicLiftStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

func env() *awscdk.Environment {
	return nil
}
