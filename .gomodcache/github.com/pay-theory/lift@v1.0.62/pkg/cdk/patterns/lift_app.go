package patterns

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// LiftAppProps defines properties for a complete Lift application
type LiftAppProps struct {
	// Application name
	AppName *string
	// Lambda code asset path
	CodeAssetPath *string
	// Enable multi-tenant support
	EnableMultiTenant *bool
	// Enable API Gateway access logging
	EnableAccessLogging *bool
	// Custom domain configuration
	DomainName     *string
	CertificateArn *string
	// Environment variables for Lambda
	Environment *map[string]*string
	// Memory size for Lambda function
	MemorySize *float64
	// Timeout for Lambda function
	Timeout *float64
	// Enable DynamoDB table
	EnableDatabase *bool
	// Database table name (only used if DatabaseTable is not provided)
	DatabaseTableName *string
	// Database partition key field name (defaults to "ID" for simple models)
	DatabasePartitionKey *string
	// Database sort key field name (optional)
	DatabaseSortKey *string
	// Existing table to use (if provided, other database options are ignored)
	DatabaseTable *liftconstructs.LiftTable
	// Enable rate limiting table
	EnableRateLimiting *bool
	// Rate limiting table name
	RateLimitTableName *string
	// Enable idempotency
	EnableIdempotency *bool
}

// LiftApp is a complete Lift application pattern with API Gateway, Lambda, and DynamoDB
type LiftApp struct {
	constructs.Construct
	API            *liftconstructs.LiftAPI
	Function       *liftconstructs.LiftFunction
	Database       *liftconstructs.LiftTable
	RateLimitTable *liftconstructs.LiftTable
}

// NewLiftApp creates a complete Lift application stack
func NewLiftApp(scope constructs.Construct, id *string, props *LiftAppProps) *LiftApp {
	builder := newLiftAppBuilder(scope, id, props)
	return builder.build()
}

// liftAppBuilder builds complete Lift application stacks
type liftAppBuilder struct {
	scope     constructs.Construct
	id        *string
	props     *LiftAppProps
	construct constructs.Construct
	app       *LiftApp
	env       map[string]*string
}

// newLiftAppBuilder creates a new Lift app builder
func newLiftAppBuilder(scope constructs.Construct, id *string, props *LiftAppProps) *liftAppBuilder {
	return &liftAppBuilder{
		scope: scope,
		id:    id,
		props: props,
		env:   make(map[string]*string),
	}
}

// build constructs the complete Lift application
func (b *liftAppBuilder) build() *LiftApp {
	b.construct = constructs.NewConstruct(b.scope, b.id)
	b.app = &LiftApp{Construct: b.construct}

	b.prepareEnvironment()
	b.createFunction()
	b.setupDatabase()
	b.setupRateLimiting()
	b.createAPI()
	b.setupRoutes()
	b.createOutputs()

	return b.app
}

// prepareEnvironment prepares environment variables for the Lambda function
func (b *liftAppBuilder) prepareEnvironment() {
	// Copy user-provided environment variables
	if b.props.Environment != nil {
		for k, v := range *b.props.Environment {
			b.env[k] = v
		}
	}

	// Add database table name if enabled
	if b.props.EnableDatabase != nil && *b.props.EnableDatabase {
		tableName := b.props.DatabaseTableName
		if tableName == nil {
			tableName = jsii.String(*b.props.AppName + "-table")
		}
		b.env["DYNAMODB_TABLE"] = tableName
	}

	// Add rate limit table name if enabled
	if b.props.EnableRateLimiting != nil && *b.props.EnableRateLimiting {
		tableName := b.props.RateLimitTableName
		if tableName == nil {
			tableName = jsii.String(*b.props.AppName + "-rate-limits")
		}
		b.env["RATE_LIMIT_TABLE"] = tableName
	}
}

// createFunction creates the Lambda function
func (b *liftAppBuilder) createFunction() {
	timeout := awscdk.Duration_Seconds(jsii.Number(30))
	if b.props.Timeout != nil {
		timeout = awscdk.Duration_Seconds(b.props.Timeout)
	}

	fnProps := &liftconstructs.LiftFunctionProps{
		FunctionProps: awslambda.FunctionProps{
			FunctionName: b.props.AppName,
			Code:         awslambda.Code_FromAsset(b.props.CodeAssetPath, nil),
			Handler:      jsii.String("bootstrap"),
			Environment:  &b.env,
			MemorySize:   b.props.MemorySize,
			Timeout:      timeout,
		},
		EnableTracing:     jsii.Bool(true),
		EnableMultiTenant: b.props.EnableMultiTenant,
	}

	b.app.Function = liftconstructs.NewLiftFunction(b.construct, jsii.String("Function"), fnProps)
}

// setupDatabase configures the database table
func (b *liftAppBuilder) setupDatabase() {
	if b.props.DatabaseTable != nil {
		b.useExistingDatabase()
	} else if b.props.EnableDatabase != nil && *b.props.EnableDatabase {
		b.createNewDatabase()
	}
}

// useExistingDatabase uses the provided database table
func (b *liftAppBuilder) useExistingDatabase() {
	b.app.Database = b.props.DatabaseTable
	b.app.Database.Table.GrantReadWriteData(b.app.Function.Function)
	b.env["DYNAMODB_TABLE"] = b.app.Database.Table.TableName()
}

// createNewDatabase creates a new database table
func (b *liftAppBuilder) createNewDatabase() {
	tableName := b.props.DatabaseTableName
	if tableName == nil {
		tableName = jsii.String(*b.props.AppName + "-table")
	}

	partitionKey := b.props.DatabasePartitionKey
	if partitionKey == nil {
		partitionKey = jsii.String("ID") // Common default for simple models
	}

	tableProps := &liftconstructs.LiftTableProps{
		TableName:                 tableName,
		PartitionKeyName:          partitionKey,
		EnablePointInTimeRecovery: jsii.Bool(true),
		EnableStreams:             jsii.Bool(true),
		TimeToLiveAttribute:       jsii.String("ttl"),
		EnableAutoScaling:         jsii.Bool(true),
	}

	if b.props.DatabaseSortKey != nil {
		tableProps.SortKeyName = b.props.DatabaseSortKey
	}

	b.app.Database = liftconstructs.NewLiftTable(b.construct, jsii.String("Database"), tableProps)
	b.app.Database.Table.GrantReadWriteData(b.app.Function.Function)
}

// setupRateLimiting creates rate limiting table if enabled
func (b *liftAppBuilder) setupRateLimiting() {
	if b.props.EnableRateLimiting == nil || !*b.props.EnableRateLimiting {
		return
	}

	tableName := b.props.RateLimitTableName
	if tableName == nil {
		tableName = jsii.String(*b.props.AppName + "-rate-limits")
	}

	b.app.RateLimitTable = liftconstructs.NewLiftTable(b.construct, jsii.String("RateLimitTable"), &liftconstructs.LiftTableProps{
		TableName:           tableName,
		PartitionKeyName:    jsii.String("PK"), // RateLimit struct uses PK/SK
		SortKeyName:         jsii.String("SK"),
		TimeToLiveAttribute: jsii.String("expires"),
	})

	b.app.RateLimitTable.Table.GrantReadWriteData(b.app.Function.Function)
}

// createAPI creates the API Gateway
func (b *liftAppBuilder) createAPI() {
	apiProps := &liftconstructs.LiftAPIProps{
		APICommonProps: liftconstructs.APICommonProps{
			Name:                jsii.String(*b.props.AppName + "-api"),
			Description:         jsii.String("API Gateway for " + *b.props.AppName),
			EnableCORS:          jsii.Bool(true),
			EnableAccessLogging: b.props.EnableAccessLogging,
			DomainName:          b.props.DomainName,
			CertificateArn:      b.props.CertificateArn,
		},
	}

	b.app.API = liftconstructs.NewLiftAPI(b.construct, jsii.String("API"), apiProps)
}

// setupRoutes configures API Gateway routes
func (b *liftAppBuilder) setupRoutes() {
	// Add catch-all route to Lambda
	b.app.API.AddLambdaRoute(
		jsii.String("/{proxy+}"),
		awsapigatewayv2.HttpMethod_ANY,
		b.app.Function.Function,
	)

	// Also add root route
	b.app.API.AddLambdaRoute(
		jsii.String("/"),
		awsapigatewayv2.HttpMethod_ANY,
		b.app.Function.Function,
	)
}

// createOutputs creates CloudFormation outputs
func (b *liftAppBuilder) createOutputs() {
	stack := awscdk.Stack_Of(b.construct)

	awscdk.NewCfnOutput(stack, jsii.String("ApiUrl"), &awscdk.CfnOutputProps{
		Value:       b.app.API.GetUrl(),
		Description: jsii.String("API Gateway endpoint URL"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("FunctionName"), &awscdk.CfnOutputProps{
		Value:       b.app.Function.Function.FunctionName(),
		Description: jsii.String("Lambda function name"),
	})

	if b.app.Database != nil {
		awscdk.NewCfnOutput(stack, jsii.String("DatabaseTableName"), &awscdk.CfnOutputProps{
			Value:       b.app.Database.Table.TableName(),
			Description: jsii.String("DynamoDB table name"),
		})
	}
}
