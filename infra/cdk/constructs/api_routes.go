package constructs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type APIGatewayProps struct {
	Environment string
	Domain      string
	Certificate awscertificatemanager.ICertificate
	Functions   *LambdaFunctions
}

type APIGateway struct {
	HttpApi      awsapigatewayv2.HttpApi
	WebSocketApi awsapigatewayv2.WebSocketApi
}

func CreateAPIGateway(scope constructs.Construct, props *APIGatewayProps) *APIGateway {
	gateway := &APIGateway{}

	// Create HTTP API
	gateway.HttpApi = awsapigatewayv2.NewHttpApi(scope, jsii.String("HttpApi"), &awsapigatewayv2.HttpApiProps{
		ApiName:     jsii.String(fmt.Sprintf("lesser-%s-api", props.Environment)),
		Description: jsii.String(fmt.Sprintf("Lesser %s HTTP API", props.Environment)),
		CorsPreflight: &awsapigatewayv2.CorsPreflightOptions{
			AllowOrigins: &[]*string{jsii.String("*")},
			AllowMethods: &[]awsapigatewayv2.CorsHttpMethod{
				awsapigatewayv2.CorsHttpMethod_GET,
				awsapigatewayv2.CorsHttpMethod_POST,
				awsapigatewayv2.CorsHttpMethod_PUT,
				awsapigatewayv2.CorsHttpMethod_DELETE,
				awsapigatewayv2.CorsHttpMethod_PATCH,
				awsapigatewayv2.CorsHttpMethod_OPTIONS,
			},
			AllowHeaders: &[]*string{
				jsii.String("Content-Type"),
				jsii.String("Authorization"),
				jsii.String("Accept"),
				jsii.String("X-Request-ID"),
				jsii.String("Digest"),
				jsii.String("Signature"),
				jsii.String("Date"),
			},
			MaxAge: awscdk.Duration_Hours(jsii.Number(24)),
		},
		DisableExecuteApiEndpoint: jsii.Bool(false),
	})

	// Add custom domain if certificate is provided
	if props.Certificate != nil && props.Domain != "" {
		domainName := awsapigatewayv2.NewDomainName(scope, jsii.String("DomainName"), &awsapigatewayv2.DomainNameProps{
			DomainName:  jsii.String(props.Domain),
			Certificate: props.Certificate,
		})

		awsapigatewayv2.NewApiMapping(scope, jsii.String("ApiMapping"), &awsapigatewayv2.ApiMappingProps{
			Api:        gateway.HttpApi,
			DomainName: domainName,
		})
	}

	// Create access log group
	logGroup := awslogs.NewLogGroup(scope, jsii.String("ApiLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/aws/apigateway/lesser-%s", props.Environment)),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	// Add default stage with logging
	stage := gateway.HttpApi.DefaultStage()
	stage.Node().AddDependency(logGroup)

	// Add routes
	addHttpRoutes(gateway.HttpApi, props.Functions)

	// Create WebSocket API
	gateway.WebSocketApi = createWebSocketApi(scope, props)

	return gateway
}

func addHttpRoutes(api awsapigatewayv2.HttpApi, functions *LambdaFunctions) {
	// Mastodon API routes
	addRoute(api, "GET /api/v1/{proxy+}", functions.APIFunction)
	addRoute(api, "POST /api/v1/{proxy+}", functions.APIFunction)
	addRoute(api, "PUT /api/v1/{proxy+}", functions.APIFunction)
	addRoute(api, "DELETE /api/v1/{proxy+}", functions.APIFunction)
	addRoute(api, "PATCH /api/v1/{proxy+}", functions.APIFunction)

	addRoute(api, "GET /api/v2/{proxy+}", functions.APIFunction)
	addRoute(api, "POST /api/v2/{proxy+}", functions.APIFunction)

	// GraphQL routes
	addRoute(api, "GET /api/graphql", functions.GraphQLFunction)
	addRoute(api, "POST /api/graphql", functions.GraphQLFunction)

	// OAuth routes (handled by native Lift implementation in API)
	addRoute(api, "GET /oauth/{proxy+}", functions.APIFunction)
	addRoute(api, "POST /oauth/{proxy+}", functions.APIFunction)

	// Auth routes (handled by native Lift implementation in API)
	addRoute(api, "GET /auth/{proxy+}", functions.APIFunction)
	addRoute(api, "POST /auth/{proxy+}", functions.APIFunction)

	// ActivityPub routes
	addRoute(api, "GET /.well-known/webfinger", functions.WebfingerFunction)
	addRoute(api, "GET /.well-known/nodeinfo", functions.APIFunction)
	addRoute(api, "GET /nodeinfo/{proxy+}", functions.APIFunction)

	// Instance-level ActivityPub endpoints
	addRoute(api, "GET /inbox", functions.InboxFunction)
	addRoute(api, "POST /inbox", functions.InboxFunction)

	addRoute(api, "GET /users/{username}", functions.APIFunction)
	addRoute(api, "GET /users/{username}/inbox", functions.InboxFunction)
	addRoute(api, "POST /users/{username}/inbox", functions.InboxFunction)
	addRoute(api, "GET /users/{username}/outbox", functions.OutboxFunction)
	addRoute(api, "POST /users/{username}/outbox", functions.OutboxFunction)
	addRoute(api, "GET /users/{username}/followers", functions.APIFunction)
	addRoute(api, "GET /users/{username}/following", functions.APIFunction)

	// Object routes
	addRoute(api, "GET /objects/{id}", functions.APIFunction)
	addRoute(api, "GET /activities/{id}", functions.APIFunction)

	// Instance routes
	addRoute(api, "GET /api/v1/instance", functions.APIFunction)
	addRoute(api, "GET /api/v2/instance", functions.APIFunction)

	// Health check
	addRoute(api, "GET /health", functions.HealthFunction)
}

func addRoute(api awsapigatewayv2.HttpApi, path string, handler awslambda.Function) {
	// Generate a unique but deterministic integration ID based on the path
	integrationId := fmt.Sprintf("Integration%s", sanitizeName(path))
	integration := awsapigatewayv2integrations.NewHttpLambdaIntegration(
		jsii.String(integrationId),
		handler,
		&awsapigatewayv2integrations.HttpLambdaIntegrationProps{
			PayloadFormatVersion: awsapigatewayv2.PayloadFormatVersion_VERSION_2_0(),
		},
	)

	// Parse method and path
	var method awsapigatewayv2.HttpMethod
	var routePath string

	switch {
	case len(path) > 4 && path[:4] == "GET ":
		method = awsapigatewayv2.HttpMethod_GET
		routePath = path[4:]
	case len(path) > 5 && path[:5] == "POST ":
		method = awsapigatewayv2.HttpMethod_POST
		routePath = path[5:]
	case len(path) > 4 && path[:4] == "PUT ":
		method = awsapigatewayv2.HttpMethod_PUT
		routePath = path[4:]
	case len(path) > 7 && path[:7] == "DELETE ":
		method = awsapigatewayv2.HttpMethod_DELETE
		routePath = path[7:]
	case len(path) > 6 && path[:6] == "PATCH ":
		method = awsapigatewayv2.HttpMethod_PATCH
		routePath = path[6:]
	default:
		return
	}

	api.AddRoutes(&awsapigatewayv2.AddRoutesOptions{
		Path:        jsii.String(routePath),
		Methods:     &[]awsapigatewayv2.HttpMethod{method},
		Integration: integration,
	})
}

func createWebSocketApi(scope constructs.Construct, props *APIGatewayProps) awsapigatewayv2.WebSocketApi {
	// Create WebSocket API for streaming
	wsApi := awsapigatewayv2.NewWebSocketApi(scope, jsii.String("WebSocketApi"), &awsapigatewayv2.WebSocketApiProps{
		ApiName:     jsii.String(fmt.Sprintf("lesser-%s-ws", props.Environment)),
		Description: jsii.String("Lesser WebSocket API for streaming"),
		ConnectRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("ConnectIntegration"),
				props.Functions.StreamRouterFunction,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
		DisconnectRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("DisconnectIntegration"),
				props.Functions.StreamRouterFunction,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
		DefaultRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("DefaultIntegration"),
				props.Functions.StreamingFunction,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
	})

	// Create stage
	awsapigatewayv2.NewWebSocketStage(scope, jsii.String("WebSocketStage"), &awsapigatewayv2.WebSocketStageProps{
		WebSocketApi: wsApi,
		StageName:    jsii.String(props.Environment),
		AutoDeploy:   jsii.Bool(true),
	})

	return wsApi
}

// sanitizeName converts a path string to a valid CDK resource identifier
func sanitizeName(path string) string {
	// Replace special characters with alphanumeric equivalents
	name := strings.ReplaceAll(path, "/", "")
	name = strings.ReplaceAll(name, " ", "")
	name = strings.ReplaceAll(name, "{", "")
	name = strings.ReplaceAll(name, "}", "")
	name = strings.ReplaceAll(name, "+", "Plus")
	name = strings.ReplaceAll(name, ".", "Dot")
	name = strings.ReplaceAll(name, "-", "")
	return name
}
