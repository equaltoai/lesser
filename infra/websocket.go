package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/apigatewayv2"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/dynamodb"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/lambda"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// CreateWebSocketInfrastructure creates the WebSocket API Gateway and related resources
func CreateWebSocketInfrastructure(ctx *pulumi.Context, lambdaRole *iam.Role, tags pulumi.StringMap) error {
	// Get configuration
	cfg := config.New(ctx, "lesser")
	domain := cfg.Require("domain")
	// Create DynamoDB tables for connections and subscriptions
	connectionsTable, err := dynamodb.NewTable(ctx, "lesser-streaming-connections", &dynamodb.TableArgs{
		Name:        pulumi.String("lesser-streaming-connections"),
		BillingMode: pulumi.String("PAY_PER_REQUEST"),
		HashKey:     pulumi.String("PK"),
		RangeKey:    pulumi.String("SK"),
		Attributes: dynamodb.TableAttributeArray{
			&dynamodb.TableAttributeArgs{Name: pulumi.String("PK"), Type: pulumi.String("S")},
			&dynamodb.TableAttributeArgs{Name: pulumi.String("SK"), Type: pulumi.String("S")},
		},
		Ttl: &dynamodb.TableTtlArgs{
			AttributeName: pulumi.String("TTL"),
			Enabled:       pulumi.Bool(true),
		},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	subscriptionsTable, err := dynamodb.NewTable(ctx, "lesser-streaming-subscriptions", &dynamodb.TableArgs{
		Name:        pulumi.String("lesser-streaming-subscriptions"),
		BillingMode: pulumi.String("PAY_PER_REQUEST"),
		HashKey:     pulumi.String("PK"),
		RangeKey:    pulumi.String("SK"),
		Attributes: dynamodb.TableAttributeArray{
			&dynamodb.TableAttributeArgs{Name: pulumi.String("PK"), Type: pulumi.String("S")},
			&dynamodb.TableAttributeArgs{Name: pulumi.String("SK"), Type: pulumi.String("S")},
		},
		Ttl: &dynamodb.TableTtlArgs{
			AttributeName: pulumi.String("TTL"),
			Enabled:       pulumi.Bool(true),
		},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	// Create WebSocket API
	wsApi, err := apigatewayv2.NewApi(ctx, "lesser-websocket-api", &apigatewayv2.ApiArgs{
		Name:                     pulumi.String("lesser-websocket-api"),
		ProtocolType:             pulumi.String("WEBSOCKET"),
		RouteSelectionExpression: pulumi.String("$request.body.type"),
		Tags:                     tags,
	})
	if err != nil {
		return err
	}

	// Create deployment stage
	stage, err := apigatewayv2.NewStage(ctx, "lesser-websocket-stage", &apigatewayv2.StageArgs{
		ApiId:      wsApi.ID(),
		Name:       pulumi.String("prod"),
		AutoDeploy: pulumi.Bool(true),
		DefaultRouteSettings: &apigatewayv2.StageDefaultRouteSettingsArgs{
			ThrottlingBurstLimit: pulumi.Int(5000),
			ThrottlingRateLimit:  pulumi.Float64(1000),
		},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	// Build and create streaming Lambda
	streamingLambda, err := lambda.NewFunction(ctx, "lesser-streaming", &lambda.FunctionArgs{
		Runtime:       pulumi.String("provided.al2023"),
		Handler:       pulumi.String("bootstrap"),
		Role:          lambdaRole.Arn,
		Code:          pulumi.NewFileArchive("../bin/streaming.zip"),
		MemorySize:    pulumi.Int(512),
		Timeout:       pulumi.Int(30),
		Architectures: pulumi.StringArray{pulumi.String("arm64")},
		Environment: &lambda.FunctionEnvironmentArgs{
			Variables: pulumi.StringMap{
				"CONNECTIONS_TABLE":   connectionsTable.Name,
				"SUBSCRIPTIONS_TABLE": subscriptionsTable.Name,
				"DOMAIN":              pulumi.String(domain),
				"AUTH_TABLE":          pulumi.String("lesser-auth"),
				"ACCOUNTS_TABLE":      pulumi.String("lesser-accounts"),
			},
		},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	// Build and create stream router Lambda
	streamRouterLambda, err := lambda.NewFunction(ctx, "lesser-stream-router", &lambda.FunctionArgs{
		Runtime:       pulumi.String("provided.al2023"),
		Handler:       pulumi.String("bootstrap"),
		Role:          lambdaRole.Arn,
		Code:          pulumi.NewFileArchive("../bin/stream-router.zip"),
		MemorySize:    pulumi.Int(512),
		Timeout:       pulumi.Int(60),
		Architectures: pulumi.StringArray{pulumi.String("arm64")},
		Environment: &lambda.FunctionEnvironmentArgs{
			Variables: pulumi.All(wsApi.ApiEndpoint, stage.Name).ApplyT(func(args []interface{}) map[string]string {
				endpoint := args[0].(string)
				stageName := args[1].(string)
				return map[string]string{
					"SUBSCRIPTIONS_TABLE": "lesser-streaming-subscriptions",
					"WEBSOCKET_ENDPOINT":  fmt.Sprintf("%s/%s", endpoint, stageName),
				}
			}).(pulumi.StringMapOutput),
		},
		Tags: tags,
	})
	if err != nil {
		return err
	}

	// Create integrations
	connectIntegration, err := apigatewayv2.NewIntegration(ctx, "lesser-ws-connect-integration", &apigatewayv2.IntegrationArgs{
		ApiId:           wsApi.ID(),
		IntegrationType: pulumi.String("AWS_PROXY"),
		IntegrationUri:  streamingLambda.InvokeArn,
	})
	if err != nil {
		return err
	}

	disconnectIntegration, err := apigatewayv2.NewIntegration(ctx, "lesser-ws-disconnect-integration", &apigatewayv2.IntegrationArgs{
		ApiId:           wsApi.ID(),
		IntegrationType: pulumi.String("AWS_PROXY"),
		IntegrationUri:  streamingLambda.InvokeArn,
	})
	if err != nil {
		return err
	}

	defaultIntegration, err := apigatewayv2.NewIntegration(ctx, "lesser-ws-default-integration", &apigatewayv2.IntegrationArgs{
		ApiId:           wsApi.ID(),
		IntegrationType: pulumi.String("AWS_PROXY"),
		IntegrationUri:  streamingLambda.InvokeArn,
	})
	if err != nil {
		return err
	}

	// Create routes
	_, err = apigatewayv2.NewRoute(ctx, "lesser-ws-connect-route", &apigatewayv2.RouteArgs{
		ApiId:             wsApi.ID(),
		RouteKey:          pulumi.String("$connect"),
		AuthorizationType: pulumi.String("NONE"), // Auth is handled in the Lambda
		Target:            pulumi.Sprintf("integrations/%s", connectIntegration.ID()),
	})
	if err != nil {
		return err
	}

	_, err = apigatewayv2.NewRoute(ctx, "lesser-ws-disconnect-route", &apigatewayv2.RouteArgs{
		ApiId:    wsApi.ID(),
		RouteKey: pulumi.String("$disconnect"),
		Target:   pulumi.Sprintf("integrations/%s", disconnectIntegration.ID()),
	})
	if err != nil {
		return err
	}

	_, err = apigatewayv2.NewRoute(ctx, "lesser-ws-default-route", &apigatewayv2.RouteArgs{
		ApiId:    wsApi.ID(),
		RouteKey: pulumi.String("$default"),
		Target:   pulumi.Sprintf("integrations/%s", defaultIntegration.ID()),
	})
	if err != nil {
		return err
	}

	// Grant Lambda permission to be invoked by API Gateway
	_, err = lambda.NewPermission(ctx, "lesser-streaming-permission", &lambda.PermissionArgs{
		Action:    pulumi.String("lambda:InvokeFunction"),
		Function:  streamingLambda.Name,
		Principal: pulumi.String("apigateway.amazonaws.com"),
		SourceArn: pulumi.Sprintf("%s/*/*", wsApi.ExecutionArn),
	})
	if err != nil {
		return err
	}

	// Note: Event source mappings for existing tables need to be created
	// in the main infrastructure file where those tables are defined

	// Export WebSocket endpoint
	ctx.Export("websocketUrl", pulumi.Sprintf("%s/%s", wsApi.ApiEndpoint, stage.Name))

	// Export the stream router Lambda ARN for use in main.go
	ctx.Export("streamRouterLambdaArn", streamRouterLambda.Arn)

	return nil
}

// UpdateLambdaPolicyForWebSocket adds WebSocket permissions to the Lambda execution role
func UpdateLambdaPolicyForWebSocket(ctx *pulumi.Context, role *iam.Role, wsApi *apigatewayv2.Api, stage *apigatewayv2.Stage) error {
	// Add permissions for API Gateway Management API
	_, err := iam.NewRolePolicy(ctx, "lesser-websocket-policy", &iam.RolePolicyArgs{
		Role: role.ID(),
		Policy: pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": [
						"execute-api:ManageConnections"
					],
					"Resource": "*"
				}
			]
		}`),
	})

	return err
}
