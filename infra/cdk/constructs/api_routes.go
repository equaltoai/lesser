package constructs

import (
	"cdk/inventory"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2integrations"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53targets"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type APIGatewayProps struct {
	AppName              string
	Environment          string
	Domain               string
	Certificate          awscertificatemanager.ICertificate
	WebSocketCertificate awscertificatemanager.ICertificate
	Functions            *LambdaFunctions
	HostedZone           awsroute53.IHostedZone
}

type APIGateway struct {
	RestApi             *liftcdk.LiftRestAPI
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
	restProps := &liftcdk.LiftRestAPIProps{
		APICommonProps: liftcdk.APICommonProps{
			Name:                jsii.String(apiName),
			Description:         jsii.String(fmt.Sprintf("Lesser %s REST API", props.Environment)),
			EnableCORS:          jsii.Bool(true),
			EnableAccessLogging: jsii.Bool(true),
			AccessLogGroup:      logGroup,
			StageName:           jsii.String(string(apiStage)),
		},
		EnableStreaming:       jsii.Bool(false), // enable per-method for SSE endpoints
		StreamingTimeout:      &streamTimeoutSeconds,
		EnableDetailedMetrics: jsii.Bool(true),
	}

	// Add custom domain if certificate is provided
	apiDomain := ""
	if props.Domain != "" {
		apiDomain = fmt.Sprintf("api.%s", props.Domain)
	}
	if props.Certificate != nil && apiDomain != "" {
		restProps.DomainName = jsii.String(apiDomain)
		restProps.Certificate = props.Certificate
	}

	gateway.RestApi = liftcdk.NewLiftRestAPI(scope, jsii.String("RestApi"), restProps)

	if props.HostedZone != nil && apiDomain != "" && gateway.RestApi.RestAPI.DomainName() != nil {
		recordName := relativeRecordName(apiDomain, props.HostedZone)
		target := awsroute53targets.NewApiGatewayDomain(gateway.RestApi.RestAPI.DomainName())

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

	// Add routes
	addRestRoutes(gateway.RestApi, props.Functions, streamTimeoutSeconds)

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

func addRestRoutes(api *liftcdk.LiftRestAPI, functions *LambdaFunctions, streamTimeoutSeconds int) {
	apiFn := functions.Must("api")
	graphqlFn := functions.Must("graphql")
	sseFn := functions.Must("sse")
	healthFn := apiFn

	// Mastodon streaming (SSE) routes.
	// NOTE: Response-streaming integrations are currently disabled for REST APIs to avoid
	// CloudFormation/API Gateway provisioning issues. These endpoints still exist but use
	// standard Lambda proxy integrations.
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/health"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/user"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/user/notification"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/public"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/public/local"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/public/remote"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/hashtag"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/hashtag/local"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/list"), jsii.String("GET"), sseFn)
	api.AddLambdaIntegration(jsii.String("/api/v1/streaming/direct"), jsii.String("GET"), sseFn)

	// Mastodon API routes (Lift handles internal routing).
	api.AddLambdaIntegration(jsii.String("/api/v1/{proxy+}"), jsii.String("ANY"), apiFn)
	api.AddLambdaIntegration(jsii.String("/api/v2/{proxy+}"), jsii.String("ANY"), apiFn)

	// GraphQL routes.
	api.AddLambdaIntegration(jsii.String("/api/graphql"), jsii.String("GET"), graphqlFn)
	api.AddLambdaIntegration(jsii.String("/api/graphql"), jsii.String("POST"), graphqlFn)

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
	api.AddLambdaIntegration(jsii.String("/{proxy+}"), jsii.String("ANY"), apiFn)

	// Health check.
	api.AddLambdaIntegration(jsii.String("/health"), jsii.String("GET"), healthFn)
}

func addInventoryRestRoutes(api *liftcdk.LiftRestAPI, functions *LambdaFunctions, include map[string]struct{}) {
	for _, spec := range inventory.LambdaInventory.Lambdas {
		if _, ok := include[spec.Name]; !ok {
			continue
		}

		handler := functions.Must(spec.Name)
		for _, route := range spec.HTTPRoutes {
			api.AddLambdaIntegration(jsii.String(route.Path), jsii.String(route.Method), handler)
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
