package constructs

/*
Package constructs provides high-level AWS CDK constructs optimized for Lift applications.

This package builds on top of the AWS CDK for Go (v2) to provide opinionated, production-ready
constructs that implement Lift's best practices for building serverless applications.

Key Features:

  - API Gateway: LiftAPI provides a complete HTTP API Gateway implementation with CORS,
    custom domains, access logging, and throttling support.

  - DynamoDB: LiftTable offers DynamoDB tables with point-in-time recovery, streams,
    auto-scaling, and TTL configured according to Lift's best practices.

  - Lambda: LiftFunction creates Lambda functions with optimized defaults for Lift,
    including tracing, metrics, and multi-tenant support.

  - Security: Security-enhanced constructs with built-in compliance features.

  - Monitoring: Integrated CloudWatch monitoring and X-Ray tracing.

  - Event Processing: Streamlined event processing with DynamoDB streams,
    Kinesis, SNS, SQS, and EventBridge.

  - WebSocket API: LiftWebSocketAPI provides WebSocket API Gateway implementation.

  - Event Processing: DynamoStreamProcessor, KinesisProcessor, SNSProcessor, SQSProcessor
    for processing events from various AWS services.

  - Compliance: ComplianceStack for implementing security and compliance best practices.

  - Monitoring: MonitoredFunction for comprehensive Lambda function monitoring.

  - Security: SecureFunction for enhanced Lambda function security.

Constructs are designed to work together seamlessly, with sensible defaults that can be
customized as needed. Each construct follows AWS best practices for security, scalability,
and observability.

Example:

	import (
		"github.com/aws/aws-cdk-go/awscdk/v2"
		"github.com/aws/constructs-go/constructs/v10"
		"github.com/lift/cdk/constructs"
	)

	func NewMyStack(scope constructs.Construct, id string, props *awscdk.StackProps) awscdk.Stack {
		stack := awscdk.NewStack(scope, &id, props)

		// Create a Lift-optimized API
		api := constructs.NewLiftAPI(stack, jsii.String("MyAPI"), &constructs.LiftAPIProps{
			APICommonProps: constructs.APICommonProps{
				Name:                jsii.String("MyServiceAPI"),
				EnableCORS:          jsii.Bool(true),
				EnableAccessLogging: jsii.Bool(true),
			},
		})

		// Create a Lift-optimized DynamoDB table
		table := constructs.NewLiftTable(stack, jsii.String("MyTable"), &constructs.LiftTableProps{
			TableName: jsii.String("MyServiceData"),
			PartitionKeyName: jsii.String("PK"),
			SortKeyName: jsii.String("SK"),
			EnablePointInTimeRecovery: jsii.Bool(true),
			EnableStreams: jsii.Bool(true),
		})

		// Create a Lift-optimized Lambda function
		fn := constructs.NewLiftFunction(stack, jsii.String("MyFunction"), &constructs.LiftFunctionProps{
			Code:    awslambda.Code_FromAsset(jsii.String("lambda/handler")),
			Handler: jsii.String("main"),
			EnableTracing: jsii.Bool(true),
			EnableMetrics: jsii.Bool(true),
			EnableDynamORM: jsii.Bool(true),
			DynamORMTableName: table.GetTableName(),
		})

		// Grant the Lambda function permissions to access the table
		table.GrantReadWrite(fn)

		// Add a route to the API that invokes the Lambda function
		api.AddLambdaRoute(jsii.String("/items"), awsapigatewayv2.HttpMethod_GET, fn)

		return stack
	}
*/
