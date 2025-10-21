package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/pay-theory/lift/pkg/cdk/patterns"
)

// MicroserviceStackProps defines properties for a microservice stack
type MicroserviceStackProps struct {
	awscdk.StackProps
	Environment    map[string]string
	ServiceName    string
	CodePath       string
	MemorySize     float64
	EnableDatabase bool
	EnableCache    bool
}

// NewMicroserviceStack creates a stack for a single microservice
func NewMicroserviceStack(scope constructs.Construct, id string, props *MicroserviceStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// Convert environment map
	env := make(map[string]*string)
	for k, v := range props.Environment {
		env[k] = jsii.String(v)
	}

	// Create the Lift application
	app := patterns.NewLiftApp(stack, jsii.String("Service"), &patterns.LiftAppProps{
		AppName:            jsii.String(props.ServiceName),
		CodeAssetPath:      jsii.String(props.CodePath),
		EnableDatabase:     jsii.Bool(props.EnableDatabase),
		EnableRateLimiting: jsii.Bool(true),
		Environment:        &env,
		MemorySize:         jsii.Number(props.MemorySize),
		Timeout:            jsii.Number(300), // 5 minutes in seconds
	})

	// Add stack outputs
	awscdk.NewCfnOutput(stack, jsii.String("ServiceEndpoint"), &awscdk.CfnOutputProps{
		Value:       app.API.HttpAPI.ApiEndpoint(),
		Description: jsii.String("Service API endpoint"),
		ExportName:  jsii.String(props.ServiceName + "-endpoint"),
	})

	if app.Database != nil {
		awscdk.NewCfnOutput(stack, jsii.String("DatabaseTable"), &awscdk.CfnOutputProps{
			Value:       app.Database.Table.TableName(),
			Description: jsii.String("DynamoDB table name"),
			ExportName:  jsii.String(props.ServiceName + "-table"),
		})
	}

	return stack
}
