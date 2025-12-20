package constructs

import (
	"cdk/inventory"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53targets"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type APIGatewayProps struct {
	Environment            string
	Domain                 string
	Certificate            awscertificatemanager.ICertificate
	GraphQLWSCertificate   awscertificatemanager.ICertificate
	StreamingWSCertificate awscertificatemanager.ICertificate
	Functions              *LambdaFunctions
	HostedZone             awsroute53.IHostedZone
}

type APIGateway struct {
	HttpApi             awsapigatewayv2.HttpApi
	WebSocketApi        awsapigatewayv2.WebSocketApi
	GraphQLWebSocketApi awsapigatewayv2.WebSocketApi
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

		if props.HostedZone != nil {
			recordName := relativeRecordName(props.Domain, props.HostedZone)
			target := awsroute53targets.NewApiGatewayv2DomainProperties(domainName.RegionalDomainName(), domainName.RegionalHostedZoneId())

			awsroute53.NewARecord(scope, jsii.String("ApiAliasARecord"), &awsroute53.ARecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})

			awsroute53.NewAaaaRecord(scope, jsii.String("ApiAliasAAAARecord"), &awsroute53.AaaaRecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})
		}
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

	// Create GraphQL WebSocket API
	gateway.GraphQLWebSocketApi = createGraphQLWebSocketApi(scope, props)

	return gateway
}

func addHttpRoutes(api awsapigatewayv2.HttpApi, functions *LambdaFunctions) {
	apiFn := functions.Must("api")
	graphqlFn := functions.Must("graphql")
	healthFn := apiFn

	// Mastodon API routes
	addRoute(api, "GET /api/v1/{proxy+}", apiFn)
	addRoute(api, "POST /api/v1/{proxy+}", apiFn)
	addRoute(api, "PUT /api/v1/{proxy+}", apiFn)
	addRoute(api, "DELETE /api/v1/{proxy+}", apiFn)
	addRoute(api, "PATCH /api/v1/{proxy+}", apiFn)

	addRoute(api, "GET /api/v2/{proxy+}", apiFn)
	addRoute(api, "POST /api/v2/{proxy+}", apiFn)

	// GraphQL routes
	addRoute(api, "GET /api/graphql", graphqlFn)
	addRoute(api, "POST /api/graphql", graphqlFn)
	addRoute(api, "OPTIONS /api/graphql", graphqlFn)

	// Account registration endpoint (Mastodon-compatible)
	addRoute(api, "POST /api/v1/accounts", apiFn)

	// Admin routes
	addRoute(api, "GET /api/v1/admin/{proxy+}", apiFn)
	addRoute(api, "POST /api/v1/admin/{proxy+}", apiFn)
	addRoute(api, "PUT /api/v1/admin/{proxy+}", apiFn)
	addRoute(api, "DELETE /api/v1/admin/{proxy+}", apiFn)
	addRoute(api, "PATCH /api/v1/admin/{proxy+}", apiFn)

	// OAuth routes (handled by native Lift implementation in API)
	addRoute(api, "GET /oauth/{proxy+}", apiFn)
	addRoute(api, "POST /oauth/{proxy+}", apiFn)

	// Auth routes (handled by native Lift implementation in API)
	addRoute(api, "GET /auth/{proxy+}", apiFn)
	addRoute(api, "POST /auth/{proxy+}", apiFn)

	// Federation routes (inventory-driven; Spec 03)
	addInventoryHttpRoutes(api, functions, map[string]struct{}{
		"actor":       {},
		"collections": {},
		"inbox":       {},
		"objects":     {},
		"outbox":      {},
		"webfinger":   {},
	})

	// Instance routes
	addRoute(api, "GET /api/v1/instance", apiFn)
	addRoute(api, "GET /api/v2/instance", apiFn)

	// Catch-all fallback to ensure unexpected routes reach the API Lambda
	addRoute(api, "ANY /{proxy+}", apiFn)

	// Health check
	addRoute(api, "GET /health", healthFn)
}

func addInventoryHttpRoutes(api awsapigatewayv2.HttpApi, functions *LambdaFunctions, include map[string]struct{}) {
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if _, ok := include[spec.Name]; !ok {
			continue
		}

		handler := functions.Must(spec.Name)
		for _, route := range spec.HTTPRoutes {
			addRoute(api, fmt.Sprintf("%s %s", route.Method, route.Path), handler)
		}
	}
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
	var (
		method    awsapigatewayv2.HttpMethod
		routePath string
	)

	switch {
	case strings.HasPrefix(path, "GET "):
		method = awsapigatewayv2.HttpMethod_GET
		routePath = path[4:]
	case strings.HasPrefix(path, "POST "):
		method = awsapigatewayv2.HttpMethod_POST
		routePath = path[5:]
	case strings.HasPrefix(path, "PUT "):
		method = awsapigatewayv2.HttpMethod_PUT
		routePath = path[4:]
	case strings.HasPrefix(path, "DELETE "):
		method = awsapigatewayv2.HttpMethod_DELETE
		routePath = path[7:]
	case strings.HasPrefix(path, "PATCH "):
		method = awsapigatewayv2.HttpMethod_PATCH
		routePath = path[6:]
	case strings.HasPrefix(path, "OPTIONS "):
		method = awsapigatewayv2.HttpMethod_OPTIONS
		routePath = path[8:]
	case strings.HasPrefix(path, "ANY "):
		method = awsapigatewayv2.HttpMethod_ANY
		routePath = path[4:]
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
	streamingFn := props.Functions.Must("streaming")

	// Create WebSocket API for streaming
	wsApi := awsapigatewayv2.NewWebSocketApi(scope, jsii.String("WebSocketApi"), &awsapigatewayv2.WebSocketApiProps{
		ApiName:     jsii.String(fmt.Sprintf("lesser-%s-streaming-ws-v2", props.Environment)),
		Description: jsii.String("Lesser WebSocket API for streaming"),
		ConnectRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("ConnectIntegration"),
				streamingFn,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
		DisconnectRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("DisconnectIntegration"),
				streamingFn,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
		DefaultRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("DefaultIntegration"),
				streamingFn,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
	})

	// Create stage
	stage := awsapigatewayv2.NewWebSocketStage(scope, jsii.String("WebSocketStage"), &awsapigatewayv2.WebSocketStageProps{
		WebSocketApi: wsApi,
		StageName:    jsii.String(props.Environment),
		AutoDeploy:   jsii.Bool(true),
	})

	// Attach custom domain if certificate is provided
	if props.StreamingWSCertificate != nil && props.Domain != "" {
		streamDomain := fmt.Sprintf("stream.%s", props.Domain)

		domainName := awsapigatewayv2.NewDomainName(scope, jsii.String("StreamingWebSocketDomain"), &awsapigatewayv2.DomainNameProps{
			DomainName:  jsii.String(streamDomain),
			Certificate: props.StreamingWSCertificate,
		})

		awsapigatewayv2.NewApiMapping(scope, jsii.String("StreamingWebSocketApiMapping"), &awsapigatewayv2.ApiMappingProps{
			Api:        wsApi,
			DomainName: domainName,
			Stage:      stage,
		})

		if props.HostedZone != nil {
			recordName := relativeRecordName(streamDomain, props.HostedZone)
			target := awsroute53targets.NewApiGatewayv2DomainProperties(domainName.RegionalDomainName(), domainName.RegionalHostedZoneId())

			awsroute53.NewARecord(scope, jsii.String("StreamingWebSocketAliasARecord"), &awsroute53.ARecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})

			awsroute53.NewAaaaRecord(scope, jsii.String("StreamingWebSocketAliasAAAARecord"), &awsroute53.AaaaRecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})
		}
	}

	return wsApi
}

func createGraphQLWebSocketApi(scope constructs.Construct, props *APIGatewayProps) awsapigatewayv2.WebSocketApi {
	graphqlWSFn := props.Functions.Must("graphql-ws")

	wsApi := awsapigatewayv2.NewWebSocketApi(scope, jsii.String("GraphQLWebSocketApi"), &awsapigatewayv2.WebSocketApiProps{
		ApiName:     jsii.String(fmt.Sprintf("lesser-%s-graphql-ws", props.Environment)),
		Description: jsii.String("GraphQL WebSocket API for subscriptions"),
		ConnectRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("GraphQLWSConnectIntegration"),
				graphqlWSFn,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
		DisconnectRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("GraphQLWSDisconnectIntegration"),
				graphqlWSFn,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
		DefaultRouteOptions: &awsapigatewayv2.WebSocketRouteOptions{
			Integration: awsapigatewayv2integrations.NewWebSocketLambdaIntegration(
				jsii.String("GraphQLWSDefaultIntegration"),
				graphqlWSFn,
				&awsapigatewayv2integrations.WebSocketLambdaIntegrationProps{},
			),
		},
	})

	stage := awsapigatewayv2.NewWebSocketStage(scope, jsii.String("GraphQLWebSocketStage"), &awsapigatewayv2.WebSocketStageProps{
		WebSocketApi: wsApi,
		StageName:    jsii.String(props.Environment),
		AutoDeploy:   jsii.Bool(true),
	})

	if props.GraphQLWSCertificate != nil && props.Domain != "" {
		graphqlWsDomain := fmt.Sprintf("graphql-ws.%s", props.Domain)

		domainName := awsapigatewayv2.NewDomainName(scope, jsii.String("GraphQLWebSocketDomain"), &awsapigatewayv2.DomainNameProps{
			DomainName:  jsii.String(graphqlWsDomain),
			Certificate: props.GraphQLWSCertificate,
		})

		awsapigatewayv2.NewApiMapping(scope, jsii.String("GraphQLWebSocketApiMapping"), &awsapigatewayv2.ApiMappingProps{
			Api:        wsApi,
			DomainName: domainName,
			Stage:      stage,
		})

		if props.HostedZone != nil {
			recordName := relativeRecordName(graphqlWsDomain, props.HostedZone)
			target := awsroute53targets.NewApiGatewayv2DomainProperties(domainName.RegionalDomainName(), domainName.RegionalHostedZoneId())

			awsroute53.NewARecord(scope, jsii.String("GraphQLWebSocketAliasARecord"), &awsroute53.ARecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})

			awsroute53.NewAaaaRecord(scope, jsii.String("GraphQLWebSocketAliasAAAARecord"), &awsroute53.AaaaRecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})
		}
	}

	return wsApi
}

func relativeRecordName(domain string, zone awsroute53.IHostedZone) *string {
	if zone == nil {
		return jsii.String(domain)
	}

	zoneNamePtr := zone.ZoneName()
	if zoneNamePtr == nil {
		return jsii.String(domain)
	}

	zoneName := strings.TrimSuffix(*zoneNamePtr, ".")
	if domain == "" || domain == zoneName {
		return jsii.String("")
	}

	if strings.HasSuffix(domain, "."+zoneName) {
		return jsii.String(strings.TrimSuffix(domain, "."+zoneName))
	}

	return jsii.String(domain)
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
