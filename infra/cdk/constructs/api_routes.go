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
	browsercors "github.com/equaltoai/lesser/pkg/security/cors"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
)

type APIGatewayProps struct {
	AppName               string
	Environment           string
	Domain                string
	Certificate           awscertificatemanager.ICertificate
	WebSocketCertificate  awscertificatemanager.ICertificate
	Functions             *LambdaFunctions
	HostedZone            awsroute53.IHostedZone
	BodyEnabled           bool
	APICORSAllowedOrigins string
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
		Stage: &apptheorycdk.AppTheoryRestApiRouterStageOptions{
			StageName:          jsii.String(string(apiStage)),
			AccessLogging:      logGroup,
			DetailedMetrics:    jsii.Bool(true),
			AccessLogFormat:    awsapigateway.AccessLogFormat_Custom(jsii.String(`{"requestId":"$context.requestId","extendedRequestId":"$context.extendedRequestId","requestTime":"$context.requestTime","ip":"$context.identity.sourceIp","httpMethod":"$context.httpMethod","resourcePath":"$context.resourcePath","path":"$context.resourcePath","status":"$context.status","responseLength":"$context.responseLength","integrationStatus":"$context.integration.status","integrationLatency":"$context.integration.latency","integrationError":"$context.integrationErrorMessage","errorMessage":"$context.error.messageString","errorResponseType":"$context.error.responseType"}`)),
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
	preflight := restAPIPreflightConfigForDomain(props.Domain, props.APICORSAllowedOrigins)
	addRestApiGatewayResponses(gateway.RestApi, gateway.RestApi.Api(), preflight)

	// Add routes
	addRestRoutes(gateway.RestApi, props.Functions, streamTimeoutSeconds, preflight)
	if props.BodyEnabled {
		addMcpRoute(scope, gateway.RestApi, appName, apiStage, preflight)
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

type restAPIPreflightConfig struct {
	DefaultOrigin         string
	ConfiguredOrigins     string
	AllowHeaders          string
	AllowMethods          string
	MaxAgeSeconds         int
	GatewayResponseOrigin string
}

func restAPIPreflightConfigForDomain(domain string, configuredOrigins string) restAPIPreflightConfig {
	defaultOrigin := ""
	if normalized, _, ok := browsercors.NormalizeOrigin("https://" + strings.TrimSpace(domain)); ok {
		defaultOrigin = normalized
	}
	normalizedConfigured := browsercors.NormalizeAllowedOriginsForDeploy(configuredOrigins)

	gatewayResponseOrigin := ""
	if normalizedConfigured == "" {
		gatewayResponseOrigin = defaultOrigin
	} else if normalizedConfigured == "*" {
		gatewayResponseOrigin = "*"
	} else if normalizedConfigured != "" && !strings.Contains(normalizedConfigured, ",") &&
		normalizedConfigured != browsercors.DenyAllAllowlist {
		gatewayResponseOrigin = normalizedConfigured
	}

	return restAPIPreflightConfig{
		DefaultOrigin:     defaultOrigin,
		ConfiguredOrigins: normalizedConfigured,
		AllowHeaders: strings.Join([]string{
			"Accept",
			"Authorization",
			"Content-Type",
			"User-Agent",
			"X-Forwarded-For",
			"X-Forwarded-Proto",
			"X-Request-Id",
			"X-Tenant-Id",
			"X-Amz-Date",
			"X-Api-Key",
			"X-Amz-Security-Token",
			"mcp-protocol-version",
			"mcp-session-id",
			"last-event-id",
		}, ","),
		AllowMethods:          "GET,POST,PUT,DELETE,OPTIONS,PATCH,HEAD",
		MaxAgeSeconds:         600,
		GatewayResponseOrigin: gatewayResponseOrigin,
	}
}

func addMcpRoute(scope constructs.Construct, api apptheorycdk.AppTheoryRestApiRouter, appName string, stage naming.Stage, preflight restAPIPreflightConfig) {
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
	awslambda.NewCfnPermission(scope, jsii.String("McpLambdaInvokeFromApiGateway"), &awslambda.CfnPermissionProps{
		Action:       jsii.String("lambda:InvokeFunction"),
		FunctionName: mcpLambda.FunctionArn(),
		Principal:    jsii.String("apigateway.amazonaws.com"),
		SourceArn:    api.Api().ArnForExecuteApi(jsii.String("*"), jsii.String("/*"), jsii.String(string(stage))),
	})
	options := &apptheorycdk.AppTheoryRestApiRouterIntegrationOptions{
		Streaming: jsii.Bool(true),
	}
	addLambdaIntegrationWithPreflight(api, "/mcp", &[]*string{jsii.String("POST")}, mcpLambda, options, preflight)
	addLambdaIntegrationWithPreflight(api, "/mcp", &[]*string{jsii.String("GET")}, mcpLambda, options, preflight)
	addLambdaIntegrationWithPreflight(api, "/mcp", &[]*string{jsii.String("DELETE")}, mcpLambda, nil, preflight)
	addLambdaIntegrationWithPreflight(api, "/mcp/{actor}", &[]*string{jsii.String("POST")}, mcpLambda, options, preflight)
	addLambdaIntegrationWithPreflight(api, "/mcp/{actor}", &[]*string{jsii.String("GET")}, mcpLambda, options, preflight)
	addLambdaIntegrationWithPreflight(api, "/mcp/{actor}", &[]*string{jsii.String("DELETE")}, mcpLambda, nil, preflight)
	addLambdaIntegrationWithPreflight(api, "/.well-known/mcp.json", &[]*string{jsii.String("GET")}, mcpLambda, nil, preflight)
	addLambdaIntegrationWithPreflight(api, "/.well-known/oauth-protected-resource/mcp/{actor}", &[]*string{jsii.String("GET")}, mcpLambda, nil, preflight)
}

func addRestApiGatewayResponses(scope constructs.Construct, api awsapigateway.RestApi, preflight restAPIPreflightConfig) {
	if scope == nil || api == nil {
		return
	}

	commonHeaders := map[string]*string{
		"gatewayresponse.header.Access-Control-Allow-Headers": jsii.String("'" + preflight.AllowHeaders + "'"),
		"gatewayresponse.header.Access-Control-Allow-Methods": jsii.String("'" + preflight.AllowMethods + "'"),
	}
	if strings.TrimSpace(preflight.GatewayResponseOrigin) != "" {
		commonHeaders["gatewayresponse.header.Access-Control-Allow-Origin"] = jsii.String("'" + preflight.GatewayResponseOrigin + "'")
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

func addRestRoutes(api apptheorycdk.AppTheoryRestApiRouter, functions *LambdaFunctions, streamTimeoutSeconds int, preflight restAPIPreflightConfig) {
	apiFn := functions.Must("api")
	graphqlFn := functions.Must("graphql")
	sseFn := functions.Must("sse")
	healthFn := apiFn

	// Root redirect is implemented in the API Lambda (GET/HEAD "/" → 302 "/l/"), but API Gateway requires an
	// explicit route for "/" (the proxy route does not match the empty path).
	addLambdaIntegrationWithPreflight(api, "/", &[]*string{jsii.String("GET")}, apiFn, nil, preflight)
	addLambdaIntegrationWithPreflight(api, "/", &[]*string{jsii.String("HEAD")}, apiFn, nil, preflight)

	// Mastodon streaming (SSE) routes.
	sseOptions := &apptheorycdk.AppTheoryRestApiRouterIntegrationOptions{
		Streaming: jsii.Bool(true),
		Timeout:   awscdk.Duration_Seconds(jsii.Number(float64(streamTimeoutSeconds))),
	}
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/health", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/user", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/user/notification", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/public", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/public/local", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/public/remote", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/hashtag", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/hashtag/local", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/list", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/direct", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v1/streaming/oauth/device", &[]*string{jsii.String("GET")}, sseFn, sseOptions, preflight)

	// Lesser-exclusive private route gets an explicit API Gateway resource so access logs use
	// a templated resourcePath instead of a raw private conversation ID. Lift still handles
	// the endpoint in the API Lambda.
	addLambdaIntegrationWithPreflight(api, "/api/v1/souls/bound/me/mint-conversations/{conversationId}", &[]*string{jsii.String("GET")}, apiFn, nil, preflight)

	// Mastodon API routes (Lift handles internal routing).
	addLambdaIntegrationWithPreflight(api, "/api/v1/{proxy+}", &[]*string{jsii.String("ANY")}, apiFn, nil, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/v2/{proxy+}", &[]*string{jsii.String("ANY")}, apiFn, nil, preflight)

	// GraphQL routes.
	addLambdaIntegrationWithPreflight(api, "/api/graphql", &[]*string{jsii.String("GET")}, graphqlFn, nil, preflight)
	addLambdaIntegrationWithPreflight(api, "/api/graphql", &[]*string{jsii.String("POST")}, graphqlFn, nil, preflight)

	// Federation routes (inventory-driven; Spec 03).
	addInventoryRestRoutes(api, functions, map[string]struct{}{
		"actor":       {},
		"collections": {},
		"inbox":       {},
		"objects":     {},
		"outbox":      {},
		"webfinger":   {},
	}, preflight)

	// Catch-all fallback to ensure unexpected routes reach the API Lambda.
	addLambdaIntegrationWithPreflight(api, "/{proxy+}", &[]*string{jsii.String("ANY")}, apiFn, nil, preflight)

	// Health check.
	addLambdaIntegrationWithPreflight(api, "/health", &[]*string{jsii.String("GET")}, healthFn, nil, preflight)
}

func addInventoryRestRoutes(api apptheorycdk.AppTheoryRestApiRouter, functions *LambdaFunctions, include map[string]struct{}, preflight restAPIPreflightConfig) {
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if _, ok := include[spec.Name]; !ok {
			continue
		}

		handler := functions.Must(spec.Name)
		for _, route := range spec.HTTPRoutes {
			addLambdaIntegrationWithPreflight(api, route.Path, &[]*string{jsii.String(route.Method)}, handler, nil, preflight)
		}
	}
}

func addLambdaIntegrationWithPreflight(
	api apptheorycdk.AppTheoryRestApiRouter,
	path string,
	methods *[]*string,
	handler awslambda.IFunction,
	options *apptheorycdk.AppTheoryRestApiRouterIntegrationOptions,
	preflight restAPIPreflightConfig,
) {
	api.AddLambdaIntegration(jsii.String(path), methods, handler, options)
	addRestPreflight(api.Api(), path, preflight)
}

func addRestPreflight(api awsapigateway.RestApi, path string, preflight restAPIPreflightConfig) {
	if api == nil {
		return
	}
	resource := restAPIResourceForPath(api, path)
	if resource == nil || resource.Node().TryFindChild(jsii.String("OPTIONS")) != nil {
		return
	}

	resource.AddMethod(
		jsii.String("OPTIONS"),
		awsapigateway.NewMockIntegration(&awsapigateway.IntegrationOptions{
			IntegrationResponses: &[]*awsapigateway.IntegrationResponse{
				{
					StatusCode: jsii.String("200"),
					ResponseParameters: &map[string]*string{
						"method.response.header.Access-Control-Allow-Headers":     jsii.String("'" + preflight.AllowHeaders + "'"),
						"method.response.header.Access-Control-Allow-Methods":     jsii.String("'" + preflight.AllowMethods + "'"),
						"method.response.header.Access-Control-Allow-Origin":      jsii.String("integration.response.body.allowedOrigin"),
						"method.response.header.Access-Control-Allow-Credentials": jsii.String("'false'"),
						"method.response.header.Access-Control-Max-Age":           jsii.String(fmt.Sprintf("'%d'", preflight.MaxAgeSeconds)),
					},
				},
			},
			PassthroughBehavior: awsapigateway.PassthroughBehavior_WHEN_NO_MATCH,
			RequestTemplates: &map[string]*string{
				"application/json": jsii.String(restPreflightRequestTemplate(preflight)),
			},
		}),
		&awsapigateway.MethodOptions{
			MethodResponses: &[]*awsapigateway.MethodResponse{
				{
					StatusCode: jsii.String("200"),
					ResponseParameters: &map[string]*bool{
						"method.response.header.Access-Control-Allow-Headers":     jsii.Bool(true),
						"method.response.header.Access-Control-Allow-Methods":     jsii.Bool(true),
						"method.response.header.Access-Control-Allow-Origin":      jsii.Bool(true),
						"method.response.header.Access-Control-Allow-Credentials": jsii.Bool(true),
						"method.response.header.Access-Control-Max-Age":           jsii.Bool(true),
					},
				},
			},
		},
	)
}

func restAPIResourceForPath(api awsapigateway.RestApi, inputPath string) awsapigateway.IResource {
	current := api.Root()
	trimmed := strings.Trim(strings.TrimSpace(inputPath), "/")
	if trimmed == "" {
		return current
	}
	for _, segment := range strings.Split(trimmed, "/") {
		part := strings.TrimSpace(segment)
		if part == "" {
			continue
		}
		next := current.GetResource(jsii.String(part))
		if next == nil {
			next = current.AddResource(jsii.String(part), nil)
		}
		current = next
	}
	return current
}

func restPreflightRequestTemplate(preflight restAPIPreflightConfig) string {
	defaultOrigin := strings.TrimSpace(preflight.DefaultOrigin)
	configuredOrigins := strings.TrimSpace(preflight.ConfiguredOrigins)
	return fmt.Sprintf(`#set($origin = $input.params('Origin'))
#set($defaultOrigin = "%s")
#set($configuredOrigins = "%s")
#set($allowedOrigin = "")
#if($configuredOrigins == "")
  #if($origin == $defaultOrigin)
    #set($allowedOrigin = $origin)
  #end
#elseif($configuredOrigins == "*")
  #if($origin != "")
    #set($allowedOrigin = "*")
  #end
#else
  #foreach($allowed in $configuredOrigins.split(","))
    #set($candidate = $allowed.trim())
    #if($origin == $candidate)
      #set($allowedOrigin = $origin)
    #end
  #end
#end
{"statusCode": 200, "allowedOrigin": "$util.escapeJavaScript($allowedOrigin)"}`, defaultOrigin, configuredOrigins)
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
