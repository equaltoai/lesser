package constructs

import (
	"cdk/inventory"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigateway"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53targets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
)

type APIGatewayProps struct {
	AppName              string
	Environment          string
	Domain               string
	Certificate          awscertificatemanager.ICertificate
	WebSocketCertificate awscertificatemanager.ICertificate
	Functions            *LambdaFunctions
	HostedZone           awsroute53.IHostedZone
	SoulEnabled          bool
}

type APIGateway struct {
	RestApi             apptheorycdk.AppTheoryRestApiRouter
	WebSocketApi        awsapigatewayv2.WebSocketApi
	GraphQLWebSocketApi awsapigatewayv2.WebSocketApi
}

func CreateAPIGateway(scope constructs.Construct, props *APIGatewayProps) *APIGateway {
	gateway := &APIGateway{}
	apiStage := naming.StageForEnvironment(props.Environment)
	appName := strings.TrimSpace(props.AppName)
	if appName == "" {
		appName = naming.DefaultAppName
	}

	// Create access log group
	logGroup := awslogs.NewLogGroup(scope, jsii.String("ApiLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/aws/apigateway/%s", naming.ResourceNameWithApp(appName, "api", props.Environment))),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	streamTimeoutSeconds := 15 * 60

	apiName := naming.ResourceNameWithApp(appName, "api", props.Environment)
	restProps := &apptheorycdk.AppTheoryRestApiRouterProps{
		ApiName:     jsii.String(apiName),
		Description: jsii.String(fmt.Sprintf("Lesser %s REST API", props.Environment)),
		Cors:        jsii.Bool(true),
		Stage: &apptheorycdk.AppTheoryRestApiRouterStageOptions{
			StageName:          jsii.String(string(apiStage)),
			AccessLogging:      logGroup,
			DetailedMetrics:    jsii.Bool(true),
			AccessLogFormat:    awsapigateway.AccessLogFormat_Custom(jsii.String(`{"requestId":"$context.requestId","extendedRequestId":"$context.extendedRequestId","requestTime":"$context.requestTime","ip":"$context.identity.sourceIp","httpMethod":"$context.httpMethod","resourcePath":"$context.resourcePath","path":"$context.path","status":"$context.status","responseLength":"$context.responseLength","integrationStatus":"$context.integration.status","integrationLatency":"$context.integration.latency","integrationError":"$context.integrationErrorMessage","errorMessage":"$context.error.messageString","errorResponseType":"$context.error.responseType"}`)),
			AccessLogRetention: awslogs.RetentionDays_ONE_WEEK,
		},
	}

	// Add custom domain if certificate is provided
	apiDomain := ""
	if props.Domain != "" {
		apiDomain = fmt.Sprintf("api.%s", props.Domain)
	}
	if props.Certificate != nil && apiDomain != "" {
		restProps.Domain = &apptheorycdk.AppTheoryRestApiRouterDomainOptions{
			DomainName:       jsii.String(apiDomain),
			Certificate:      props.Certificate,
			HostedZone:       props.HostedZone,
			CreateAAAARecord: jsii.Bool(true),
		}
	}

	gateway.RestApi = apptheorycdk.NewAppTheoryRestApiRouter(scope, jsii.String("RestApi"), restProps)
	addRestApiGatewayResponses(gateway.RestApi, gateway.RestApi.Api())

	// Add routes
	addRestRoutes(gateway.RestApi, props.Functions, streamTimeoutSeconds)
	if props.SoulEnabled {
		addMcpRoute(scope, gateway.RestApi, appName, apiStage)
	}

	// Shared custom domain for all WebSocket APIs.
	var wsDomainName awsapigatewayv2.DomainName
	if props.WebSocketCertificate != nil && props.Domain != "" {
		wsDomain := fmt.Sprintf("ws.%s", props.Domain)
		wsDomainName = awsapigatewayv2.NewDomainName(scope, jsii.String("WebSocketDomain"), &awsapigatewayv2.DomainNameProps{
			DomainName:  jsii.String(wsDomain),
			Certificate: props.WebSocketCertificate,
		})

		if props.HostedZone != nil {
			recordName := relativeRecordName(wsDomain, props.HostedZone)
			target := awsroute53targets.NewApiGatewayv2DomainProperties(wsDomainName.RegionalDomainName(), wsDomainName.RegionalHostedZoneId())

			awsroute53.NewARecord(scope, jsii.String("WebSocketAliasARecord"), &awsroute53.ARecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})

			awsroute53.NewAaaaRecord(scope, jsii.String("WebSocketAliasAAAARecord"), &awsroute53.AaaaRecordProps{
				Zone:       props.HostedZone,
				RecordName: recordName,
				Target:     awsroute53.RecordTarget_FromAlias(target),
			})
		}
	}

	// Create WebSocket APIs (path-mapped behind ws.<domain>).
	gateway.WebSocketApi = createWebSocketApi(scope, props, wsDomainName)
	gateway.GraphQLWebSocketApi = createGraphQLWebSocketApi(scope, props, wsDomainName)

	return gateway
}

func addMcpRoute(scope constructs.Construct, api apptheorycdk.AppTheoryRestApiRouter, appName string, stage naming.Stage) {
	if scope == nil || api == nil || strings.TrimSpace(appName) == "" || strings.TrimSpace(string(stage)) == "" {
		return
	}

	paramName := fmt.Sprintf("/%s/%s/lesser-body/exports/v1/mcp_lambda_arn", appName, stage)
	lambdaArnParam := awsssm.StringParameter_FromStringParameterName(
		scope,
		jsii.String("LesserBodyMcpLambdaArnParamLookup"),
		jsii.String(paramName),
	)

	mcpLambda := awslambda.Function_FromFunctionArn(scope, jsii.String("ImportedLesserBodyMcpLambda"), lambdaArnParam.StringValue())
	options := &apptheorycdk.AppTheoryRestApiRouterIntegrationOptions{
		Streaming: jsii.Bool(true),
	}
	api.AddLambdaIntegration(jsii.String("/mcp"), &[]*string{jsii.String("POST")}, mcpLambda, options)
	api.AddLambdaIntegration(jsii.String("/.well-known/mcp.json"), &[]*string{jsii.String("GET")}, mcpLambda, nil)
}

func addRestApiGatewayResponses(scope constructs.Construct, api awsapigateway.RestApi) {
	if scope == nil || api == nil {
		return
	}

	commonHeaders := map[string]*string{
		"gatewayresponse.header.Access-Control-Allow-Origin":  jsii.String("'*'"),
		"gatewayresponse.header.Access-Control-Allow-Headers": jsii.String("'*'"),
		"gatewayresponse.header.Access-Control-Allow-Methods": jsii.String("'*'"),
	}

	serviceUnavailableTemplate := map[string]*string{
		"application/json": jsii.String(`{"message":"Service temporarily unavailable","requestId":"$context.requestId"}`),
	}

	awsapigateway.NewGatewayResponse(scope, jsii.String("Default5xxGatewayResponse"), &awsapigateway.GatewayResponseProps{
		RestApi:         api,
		Type:            awsapigateway.ResponseType_DEFAULT_5XX(),
		StatusCode:      jsii.String("503"),
		ResponseHeaders: &commonHeaders,
		Templates:       &serviceUnavailableTemplate,
	})

	awsapigateway.NewGatewayResponse(scope, jsii.String("IntegrationFailureGatewayResponse"), &awsapigateway.GatewayResponseProps{
		RestApi:         api,
		Type:            awsapigateway.ResponseType_INTEGRATION_FAILURE(),
		StatusCode:      jsii.String("503"),
		ResponseHeaders: &commonHeaders,
		Templates:       &serviceUnavailableTemplate,
	})

	awsapigateway.NewGatewayResponse(scope, jsii.String("IntegrationTimeoutGatewayResponse"), &awsapigateway.GatewayResponseProps{
		RestApi:         api,
		Type:            awsapigateway.ResponseType_INTEGRATION_TIMEOUT(),
		StatusCode:      jsii.String("504"),
		ResponseHeaders: &commonHeaders,
		Templates: &map[string]*string{
			"application/json": jsii.String(`{"message":"Service timed out","requestId":"$context.requestId"}`),
		},
	})

	awsapigateway.NewGatewayResponse(scope, jsii.String("ThrottledGatewayResponse"), &awsapigateway.GatewayResponseProps{
		RestApi:         api,
		Type:            awsapigateway.ResponseType_THROTTLED(),
		StatusCode:      jsii.String("429"),
		ResponseHeaders: &commonHeaders,
		Templates: &map[string]*string{
			"application/json": jsii.String(`{"message":"Too Many Requests","requestId":"$context.requestId"}`),
		},
	})

	awsapigateway.NewGatewayResponse(scope, jsii.String("QuotaExceededGatewayResponse"), &awsapigateway.GatewayResponseProps{
		RestApi:         api,
		Type:            awsapigateway.ResponseType_QUOTA_EXCEEDED(),
		StatusCode:      jsii.String("429"),
		ResponseHeaders: &commonHeaders,
		Templates: &map[string]*string{
			"application/json": jsii.String(`{"message":"Quota exceeded","requestId":"$context.requestId"}`),
		},
	})
}

func addRestRoutes(api apptheorycdk.AppTheoryRestApiRouter, functions *LambdaFunctions, streamTimeoutSeconds int) {
	apiFn := functions.Must("api")
	graphqlFn := functions.Must("graphql")
	sseFn := functions.Must("sse")
	healthFn := apiFn

	// Root redirect is implemented in the API Lambda (GET/HEAD "/" → 302 "/l/"), but API Gateway requires an
	// explicit route for "/" (the proxy route does not match the empty path).
	api.AddLambdaIntegration(jsii.String("/"), &[]*string{jsii.String("GET")}, apiFn, nil)
	api.AddLambdaIntegration(jsii.String("/"), &[]*string{jsii.String("HEAD")}, apiFn, nil)

	// Mastodon streaming (SSE) routes.
	sseOptions := &apptheorycdk.AppTheoryRestApiRouterIntegrationOptions{
		Streaming: jsii.Bool(true),
		Timeout:   awscdk.Duration_Seconds(jsii.Number(float64(streamTimeoutSeconds))),
	}
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/health"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/user"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/user/notification"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/public"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/public/local"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/public/remote"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/hashtag"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/hashtag/local"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/list"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/direct"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/oauth/device"), &[]*string{jsii.String("GET")}, sseFn, sseOptions)

	// Mastodon API routes (Lift handles internal routing).
	api.AddLambdaIntegration(jsii.String("/api/v1/{proxy+}"), &[]*string{jsii.String("ANY")}, apiFn, nil)
	api.AddLambdaIntegration(jsii.String("/api/v2/{proxy+}"), &[]*string{jsii.String("ANY")}, apiFn, nil)

	// GraphQL routes.
	api.AddLambdaIntegration(jsii.String("/api/graphql"), &[]*string{jsii.String("GET")}, graphqlFn, nil)
	api.AddLambdaIntegration(jsii.String("/api/graphql"), &[]*string{jsii.String("POST")}, graphqlFn, nil)

	// Federation routes (inventory-driven; Spec 03).
	addInventoryRestRoutes(api, functions, map[string]struct{}{
		"actor":       {},
		"collections": {},
		"inbox":       {},
		"objects":     {},
		"outbox":      {},
		"webfinger":   {},
	})

	// Catch-all fallback to ensure unexpected routes reach the API Lambda.
	api.AddLambdaIntegration(jsii.String("/{proxy+}"), &[]*string{jsii.String("ANY")}, apiFn, nil)

	// Health check.
	api.AddLambdaIntegration(jsii.String("/health"), &[]*string{jsii.String("GET")}, healthFn, nil)
}

func addInventoryRestRoutes(api apptheorycdk.AppTheoryRestApiRouter, functions *LambdaFunctions, include map[string]struct{}) {
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if _, ok := include[spec.Name]; !ok {
			continue
		}

		handler := functions.Must(spec.Name)
		for _, route := range spec.HTTPRoutes {
			api.AddLambdaIntegration(jsii.String(route.Path), &[]*string{jsii.String(route.Method)}, handler, nil)
		}
	}
}

func createWebSocketApi(scope constructs.Construct, props *APIGatewayProps, domainName awsapigatewayv2.DomainName) awsapigatewayv2.WebSocketApi {
	streamingFn := props.Functions.Must("streaming")
	apiStage := naming.StageForEnvironment(props.Environment)
	appName := strings.TrimSpace(props.AppName)
	if appName == "" {
		appName = naming.DefaultAppName
	}

	// Create WebSocket API for streaming
	wsApi := awsapigatewayv2.NewWebSocketApi(scope, jsii.String("WebSocketApi"), &awsapigatewayv2.WebSocketApiProps{
		ApiName:     jsii.String(naming.ResourceNameWithApp(appName, "streaming-ws-v2", props.Environment)),
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
		StageName:    jsii.String(string(apiStage)),
		AutoDeploy:   jsii.Bool(true),
	})

	// Map streaming WebSocket under ws.<domain>/stream
	if domainName != nil {
		awsapigatewayv2.NewApiMapping(scope, jsii.String("StreamingWebSocketApiMapping"), &awsapigatewayv2.ApiMappingProps{
			Api:           wsApi,
			DomainName:    domainName,
			Stage:         stage,
			ApiMappingKey: jsii.String("stream"),
		})
	}

	return wsApi
}

func createGraphQLWebSocketApi(scope constructs.Construct, props *APIGatewayProps, domainName awsapigatewayv2.DomainName) awsapigatewayv2.WebSocketApi {
	graphqlWSFn := props.Functions.Must("graphql-ws")
	apiStage := naming.StageForEnvironment(props.Environment)
	appName := strings.TrimSpace(props.AppName)
	if appName == "" {
		appName = naming.DefaultAppName
	}

	wsApi := awsapigatewayv2.NewWebSocketApi(scope, jsii.String("GraphQLWebSocketApi"), &awsapigatewayv2.WebSocketApiProps{
		ApiName:     jsii.String(naming.ResourceNameWithApp(appName, "graphql-ws", props.Environment)),
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
		StageName:    jsii.String(string(apiStage)),
		AutoDeploy:   jsii.Bool(true),
	})

	// Map GraphQL WebSocket at the root of ws.<domain>
	if domainName != nil {
		awsapigatewayv2.NewApiMapping(scope, jsii.String("GraphQLWebSocketApiMapping"), &awsapigatewayv2.ApiMappingProps{
			Api:        wsApi,
			DomainName: domainName,
			Stage:      stage,
		})
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
