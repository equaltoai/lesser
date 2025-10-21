/*
Package cdk provides AWS CDK constructs for deploying Lift applications with infrastructure as code.

# Overview

The Lift CDK package provides high-level constructs optimized for deploying serverless Go applications
built with the Lift framework. It includes constructs for Lambda functions, API Gateway, DynamoDB tables,
and other AWS resources commonly used with Lift.

# Key Constructs

## LiftFunction

Optimized Lambda function for Lift applications with the following features:
- ARM64 architecture for better price/performance
- 512MB memory by default
- 30 second timeout
- PROVIDED_AL2023 runtime for Go
- X-Ray tracing support

Example:
```go

	fn := constructs.NewLiftFunction(stack, jsii.String("MyFunction"), &constructs.LiftFunctionProps{
	    FunctionProps: awslambda.FunctionProps{
	        Code:    awslambda.Code_FromAsset(jsii.String("./lambda"), nil),
	        Handler: jsii.String("bootstrap"),
	    },
	    EnableTracing:     jsii.Bool(true),
	    EnableMultiTenant: jsii.Bool(true),
	})

```

## LiftAPI

API Gateway HTTP API with Lift integrations:
- CORS configuration for Lift headers
- Custom domain support
- Access logging
- Rate limiting configuration

Example:
```go

	api := constructs.NewLiftAPI(stack, jsii.String("MyAPI"), &constructs.LiftAPIProps{
	    APICommonProps: constructs.APICommonProps{
	        Name:        jsii.String("my-api"),
	        EnableCORS:  jsii.Bool(true),
	        DomainName:  jsii.String("api.example.com"),
	        CertificateArn: jsii.String("arn:aws:acm:..."),
	    },
	})

// Add routes
api.AddLambdaRoute(jsii.String("/users"), awsapigatewayv2.HttpMethod_GET, fn)
```

## LiftTable

DynamoDB table optimized for Lift's single-table design:
- Single-table design with pk/sk keys
- Multi-tenant partitioning support
- Global secondary index (gsi1)
- Auto-scaling configuration
- Point-in-time recovery
- TTL support

Example:
```go

	table := constructs.NewLiftTable(stack, jsii.String("MyTable"), &constructs.LiftTableProps{
	    TableName:         jsii.String("my-table"),
	    EnableMultiTenant: jsii.Bool(true),
	    EnableStreams:     jsii.Bool(true),
	    EnableAutoScaling: jsii.Bool(true),
	})

```

## LiftApp Pattern

Complete application deployment that includes:
- Lambda function with Lift runtime
- API Gateway with catch-all routing
- DynamoDB table for application data
- Rate limiting table
- CloudWatch logs
- All necessary IAM permissions

Example:
```go

	app := patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
	    AppName:            jsii.String("my-app"),
	    CodeAssetPath:      jsii.String("./dist"),
	    EnableMultiTenant:  jsii.Bool(true),
	    EnableDatabase:     jsii.Bool(true),
	    EnableRateLimiting: jsii.Bool(true),
	    MemorySize:         jsii.Number(1024),
	    Timeout:            awscdk.Duration_Minutes(jsii.Number(5)),
	})

```

# Best Practices

1. Use the LiftApp pattern for standard applications
2. Enable multi-tenant support if building SaaS applications
3. Configure auto-scaling for production workloads
4. Enable tracing for observability
5. Set appropriate memory based on your workload (512MB-3008MB)
6. Use custom domains for production APIs

# Integration with Lift

The CDK constructs automatically configure:
- Environment variables expected by Lift
- IAM permissions for AWS services
- Optimized Lambda settings
- API Gateway routing compatible with Lift's router

# Examples

See the examples directory for complete examples of different deployment patterns.

# Contributing

When adding new constructs:
1. Follow the existing patterns
2. Include comprehensive documentation
3. Add examples showing usage
4. Ensure compatibility with Lift framework assumptions
*/
package cdk
