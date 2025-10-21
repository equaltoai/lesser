package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigateway"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// LiftRestAPIProps defines properties for creating a Lift REST API Gateway (v1)
//
//nolint:govet // Embedding maintains logical grouping with minimal benefit from reordering.
type LiftRestAPIProps struct {
	APICommonProps
	// Enable detailed CloudWatch metrics (REST API only)
	EnableDetailedMetrics *bool
	// API Key configuration
	RequireApiKey *bool
	// Endpoint configuration (REGIONAL, EDGE, PRIVATE)
	EndpointType awsapigateway.EndpointType
	// Default authorizer for all routes
	DefaultAuthorizer awsapigateway.IAuthorizer
}

// LiftRestAPI is a REST API Gateway (v1) construct for Lift applications
type LiftRestAPI struct {
	constructs.Construct
	RestAPI  awsapigateway.RestApi
	LogGroup awslogs.ILogGroup
}

// GetResourceName returns the API name
func (l *LiftRestAPI) GetResourceName() *string {
	return l.RestAPI.RestApiId()
}

// NewLiftRestAPI creates a new REST API Gateway optimized for Lift
func NewLiftRestAPI(scope constructs.Construct, id *string, props *LiftRestAPIProps) *LiftRestAPI {
	this := constructs.NewConstruct(scope, id)

	builder := newLiftRestAPIBuilder(this, props)
	return builder.build()
}

// liftRestAPIBuilder builds Lift REST API components
type liftRestAPIBuilder struct {
	construct constructs.Construct
	props     *LiftRestAPIProps
}

// newLiftRestAPIBuilder creates a new Lift REST API builder
func newLiftRestAPIBuilder(construct constructs.Construct, props *LiftRestAPIProps) *liftRestAPIBuilder {
	return &liftRestAPIBuilder{
		construct: construct,
		props:     props,
	}
}

// build constructs the complete Lift REST API
func (b *liftRestAPIBuilder) build() *LiftRestAPI {
	// Create log group for access logging
	logGroup := b.createLogGroup()

	// Create REST API
	restApi := b.createRestAPI(logGroup)

	return &LiftRestAPI{
		Construct: b.construct,
		RestAPI:   restApi,
		LogGroup:  logGroup,
	}
}

// createLogGroup creates the access log group if needed
func (b *liftRestAPIBuilder) createLogGroup() awslogs.ILogGroup {
	if b.props.EnableAccessLogging == nil || !*b.props.EnableAccessLogging {
		return nil
	}

	return CreateAPILogGroup(b.construct, b.props.Name, b.props.AccessLogGroup)
}

// createRestAPI creates the REST API with configuration
func (b *liftRestAPIBuilder) createRestAPI(logGroup awslogs.ILogGroup) awsapigateway.RestApi {
	// Set defaults
	stageName := "prod"
	if b.props.StageName != nil {
		stageName = *b.props.StageName
	}

	endpointType := awsapigateway.EndpointType_REGIONAL
	if b.props.EndpointType != "" {
		endpointType = b.props.EndpointType
	}

	apiProps := &awsapigateway.RestApiProps{
		RestApiName: b.props.Name,
		Description: b.props.Description,
		DeployOptions: &awsapigateway.StageOptions{
			StageName:        jsii.String(stageName),
			MetricsEnabled:   b.props.EnableDetailedMetrics,
			LoggingLevel:     awsapigateway.MethodLoggingLevel_INFO,
			DataTraceEnabled: jsii.Bool(false),
		},
		EndpointConfiguration: &awsapigateway.EndpointConfiguration{
			Types: &[]awsapigateway.EndpointType{endpointType},
		},
	}

	// Configure CORS if enabled
	if b.props.EnableCORS != nil && *b.props.EnableCORS {
		apiProps.DefaultCorsPreflightOptions = b.createCORSConfig()
	}

	// Configure access logging if enabled
	if logGroup != nil {
		apiProps.DeployOptions.AccessLogDestination = awsapigateway.NewLogGroupLogDestination(logGroup)
		apiProps.DeployOptions.AccessLogFormat = awsapigateway.AccessLogFormat_JsonWithStandardFields(&awsapigateway.JsonWithStandardFieldProps{
			Caller:         jsii.Bool(true),
			HttpMethod:     jsii.Bool(true),
			Ip:             jsii.Bool(true),
			Protocol:       jsii.Bool(true),
			RequestTime:    jsii.Bool(true),
			ResourcePath:   jsii.Bool(true),
			ResponseLength: jsii.Bool(true),
			Status:         jsii.Bool(true),
			User:           jsii.Bool(true),
		})
	}

	// Configure throttling if specified
	if b.props.ThrottleRateLimit != nil || b.props.ThrottleBurstLimit != nil {
		apiProps.DeployOptions.ThrottlingRateLimit = b.props.ThrottleRateLimit
		apiProps.DeployOptions.ThrottlingBurstLimit = b.props.ThrottleBurstLimit
	}

	// Configure custom domain if provided
	if b.props.DomainName != nil && b.props.CertificateArn != nil {
		cert := awscertificatemanager.Certificate_FromCertificateArn(b.construct, jsii.String("Certificate"), b.props.CertificateArn)
		apiProps.DomainName = &awsapigateway.DomainNameOptions{
			DomainName:  b.props.DomainName,
			Certificate: cert,
		}
	}

	// Configure default authorizer if provided
	if b.props.DefaultAuthorizer != nil {
		apiProps.DefaultMethodOptions = &awsapigateway.MethodOptions{
			Authorizer: b.props.DefaultAuthorizer,
		}
	}

	return awsapigateway.NewRestApi(b.construct, jsii.String("RestApi"), apiProps)
}

// createCORSConfig creates CORS preflight configuration
func (b *liftRestAPIBuilder) createCORSConfig() *awsapigateway.CorsOptions {
	// Use custom origins if provided, otherwise default to wildcard
	allowOrigins := &[]*string{jsii.String("*")}
	if b.props.AllowOrigins != nil {
		allowOrigins = b.props.AllowOrigins
	}

	// Convert methods to pointers
	methods := CORSMethods()
	methodPtrs := make([]*string, len(methods))
	for i, m := range methods {
		methodPtrs[i] = jsii.String(m)
	}

	return &awsapigateway.CorsOptions{
		AllowOrigins:  allowOrigins,
		AllowMethods:  &methodPtrs,
		AllowHeaders:  CORSHeaders(),
		ExposeHeaders: CORSExposeHeaders(),
		MaxAge:        awscdk.Duration_Hours(jsii.Number(24)),
	}
}

// AddLambdaIntegration adds a Lambda function as a method to the API
func (api *LiftRestAPI) AddLambdaIntegration(path *string, method *string, fn awslambda.IFunction) {
	api.AddLambdaIntegrationWithOptions(path, method, fn, nil)
}

// IntegrationOptions defines options for API integrations
type IntegrationOptions struct {
	// Authorizer for this method
	Authorizer awsapigateway.IAuthorizer
	// Request validator
	RequestValidator awsapigateway.IRequestValidator
	// API key required
	ApiKeyRequired *bool
}

// AddLambdaIntegrationWithOptions adds a Lambda function with additional options
func (api *LiftRestAPI) AddLambdaIntegrationWithOptions(path *string, method *string, fn awslambda.IFunction, options *IntegrationOptions) {
	// Get or create resource
	resource := api.getOrCreateResource(path)

	// Create Lambda integration
	integration := awsapigateway.NewLambdaIntegration(fn, &awsapigateway.LambdaIntegrationOptions{
		Proxy: jsii.Bool(true),
	})

	// Create method options
	methodOptions := &awsapigateway.MethodOptions{}
	if options != nil {
		if options.Authorizer != nil {
			methodOptions.Authorizer = options.Authorizer
		}
		if options.RequestValidator != nil {
			methodOptions.RequestValidator = options.RequestValidator
		}
		if options.ApiKeyRequired != nil {
			methodOptions.ApiKeyRequired = options.ApiKeyRequired
		}
	}

	// Add method to resource
	resource.AddMethod(method, integration, methodOptions)
}

// getOrCreateResource gets or creates a resource for the given path
func (api *LiftRestAPI) getOrCreateResource(path *string) awsapigateway.IResource {
	if path == nil || *path == "/" {
		return api.RestAPI.Root()
	}

	// Split path into segments
	segments := SplitPath(*path)

	// Start from root
	current := api.RestAPI.Root()

	// Create each segment
	for _, segment := range segments {
		current = current.ResourceForPath(jsii.String(segment))
	}

	return current
}

// CreateAPIKey creates an API key for the REST API
func (api *LiftRestAPI) CreateAPIKey(name *string) awsapigateway.IApiKey {
	return awsapigateway.NewApiKey(api, jsii.String("ApiKey"), &awsapigateway.ApiKeyProps{
		ApiKeyName: name,
	})
}

// CreateUsagePlan creates a usage plan with throttling and quota
func (api *LiftRestAPI) CreateUsagePlan(name *string, throttle *awsapigateway.ThrottleSettings, quota *awsapigateway.QuotaSettings) awsapigateway.UsagePlan {
	return awsapigateway.NewUsagePlan(api, jsii.String("UsagePlan"), &awsapigateway.UsagePlanProps{
		Name:     name,
		Throttle: throttle,
		Quota:    quota,
		ApiStages: &[]*awsapigateway.UsagePlanPerApiStage{
			{
				Api:   api.RestAPI,
				Stage: api.RestAPI.DeploymentStage(),
			},
		},
	})
}

// GetUrl returns the URL of the API
func (api *LiftRestAPI) GetUrl() *string {
	return api.RestAPI.Url()
}

// GetArn returns the ARN of the API
func (api *LiftRestAPI) GetArn() *string {
	return api.RestAPI.ArnForExecuteApi(jsii.String("*"), jsii.String("*"), jsii.String("*"))
}

// GrantInvoke grants invoke permissions to a principal
func (api *LiftRestAPI) GrantInvoke(grantee awsiam.IGrantable) awsiam.Grant {
	return awsiam.Grant_AddToPrincipal(&awsiam.GrantOnPrincipalOptions{
		Grantee:      grantee,
		Actions:      &[]*string{jsii.String("execute-api:Invoke")},
		ResourceArns: &[]*string{api.GetArn()},
	})
}

// GetStage returns the deployment stage
func (api *LiftRestAPI) GetStage() awsapigateway.IStage {
	return api.RestAPI.DeploymentStage()
}
