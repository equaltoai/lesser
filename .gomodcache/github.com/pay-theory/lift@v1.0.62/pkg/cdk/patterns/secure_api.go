package patterns

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatchactions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awswafv2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
)

// SecureAPIProps defines properties for creating a secure API pattern
type SecureAPIProps struct {
	Vpc                awsec2.IVpc
	Code               awslambda.Code
	AlarmTopic         awssns.ITopic
	EnableWAF          *bool
	EnableRateLimiting *bool
	RateLimitWindow    *float64
	RateLimitMax       *float64
	ApiName            *string
	DomainName         *string
	CertificateArn     *string
	Handler            *string
	MemorySize         *float64
	Timeout            *float64
	Environment        *map[string]*string
	AdditionalPolicies *[]awsiam.PolicyStatement
	RateLimitType      liftconstructs.RateLimitType
}

// SecureAPI is a pattern that creates a secure API with WAF, rate limiting, and VPC
type SecureAPI struct {
	constructs.Construct
	Api             *liftconstructs.LiftAPI
	Function        *liftconstructs.SecureFunction
	RateLimitedFunc *liftconstructs.RateLimitedFunction
	WebACL          awswafv2.CfnWebACL
}

// NewSecureAPI creates a new secure API pattern with enterprise-grade security
func NewSecureAPI(scope constructs.Construct, id *string, props *SecureAPIProps) *SecureAPI {
	this := constructs.NewConstruct(scope, id)

	// Set defaults
	if props.Handler == nil {
		props.Handler = jsii.String("bootstrap")
	}
	if props.EnableRateLimiting == nil {
		props.EnableRateLimiting = jsii.Bool(true)
	}
	if props.EnableWAF == nil {
		props.EnableWAF = jsii.Bool(true)
	}
	if props.RateLimitType == "" {
		props.RateLimitType = liftconstructs.RateLimitTypeIP
	}
	if props.MemorySize == nil {
		props.MemorySize = jsii.Number(1024)
	}

	// For SecureAPI, we'll use either SecureFunction OR RateLimitedFunction, not both
	var lambdaFn awslambda.Function
	var secureFn *liftconstructs.SecureFunction
	var rateLimitedFn *liftconstructs.RateLimitedFunction

	if *props.EnableRateLimiting {
		// Create rate limited function with VPC if provided
		rateLimitedProps := &liftconstructs.RateLimitedFunctionProps{
			LiftFunctionProps: liftconstructs.LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Code:        props.Code,
					Handler:     props.Handler,
					Environment: props.Environment,
					MemorySize:  props.MemorySize,
				},
				EnableTracing: jsii.Bool(true),
				EnableMetrics: jsii.Bool(true),
			},
			RateLimitType: props.RateLimitType,
			WindowSeconds: props.RateLimitWindow,
			Limit:         props.RateLimitMax,
			EnableMetrics: jsii.Bool(true),
		}

		// Add VPC configuration if provided
		if props.Vpc != nil {
			rateLimitedProps.Vpc = props.Vpc
		}

		if props.Timeout != nil {
			rateLimitedProps.Timeout = awscdk.Duration_Seconds(props.Timeout)
		}

		rateLimitedFn = liftconstructs.NewRateLimitedFunction(this, jsii.String("Function"), rateLimitedProps)
		lambdaFn = rateLimitedFn.GetFunction()
	} else {
		// Create secure function
		secureFunctionProps := &liftconstructs.SecureFunctionProps{
			LiftFunctionProps: liftconstructs.LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Code:        props.Code,
					Handler:     props.Handler,
					Environment: props.Environment,
					MemorySize:  props.MemorySize,
				},
				EnableTracing: jsii.Bool(true),
				EnableMetrics: jsii.Bool(true),
			},
			Vpc:                 props.Vpc,
			EnableKMSEncryption: jsii.Bool(true),
			PrivateOnly:         jsii.Bool(false),
			AdditionalPolicies:  props.AdditionalPolicies,
		}

		if props.Timeout != nil {
			secureFunctionProps.Timeout = awscdk.Duration_Seconds(props.Timeout)
		}

		secureFn = liftconstructs.NewSecureFunction(this, jsii.String("Function"), secureFunctionProps)
		lambdaFn = secureFn.GetFunction()
	}

	// Create API Gateway
	api := liftconstructs.NewLiftAPI(this, jsii.String("Api"), &liftconstructs.LiftAPIProps{
		APICommonProps: liftconstructs.APICommonProps{
			Name:                props.ApiName,
			EnableCORS:          jsii.Bool(false), // Typically disabled for secure APIs
			EnableAccessLogging: jsii.Bool(true),
			DomainName:          props.DomainName,
			CertificateArn:      props.CertificateArn,
			ThrottleRateLimit:   jsii.Number(1000), // 1000 requests per second
			ThrottleBurstLimit:  jsii.Number(5000), // 5000 burst
		},
	})

	// Add Lambda integration
	api.AddLambdaRoute(jsii.String("/{proxy+}"), awsapigatewayv2.HttpMethod_ANY, lambdaFn)

	// Create WAF if enabled
	var webACL awswafv2.CfnWebACL
	if *props.EnableWAF {
		webACL = awswafv2.NewCfnWebACL(this, jsii.String("WebACL"), &awswafv2.CfnWebACLProps{
			Scope:            jsii.String("REGIONAL"),
			DefaultAction:    &awswafv2.CfnWebACL_DefaultActionProperty{Allow: &map[string]interface{}{}},
			Description:      jsii.String("WAF protection for " + *props.ApiName),
			Name:             jsii.String(*props.ApiName + "-waf"),
			Rules:            createWAFRules(),
			VisibilityConfig: createWAFVisibilityConfig(props.ApiName),
		})

		// Associate WAF with API Gateway
		// Note: HTTP API ARN construction for WAF association
		apiArn := awscdk.Stack_Of(this).FormatArn(&awscdk.ArnComponents{
			Service:      jsii.String("apigateway"),
			Resource:     jsii.String("apis"),
			ResourceName: api.HttpAPI.HttpApiId(),
		})

		awswafv2.NewCfnWebACLAssociation(this, jsii.String("WebACLAssociation"), &awswafv2.CfnWebACLAssociationProps{
			ResourceArn: apiArn,
			WebAclArn:   webACL.AttrArn(),
		})
	}

	// Create monitoring with alarms
	if props.AlarmTopic != nil {
		// Create alarms for the secure function
		createSecurityAlarms(this, lambdaFn, props.AlarmTopic, props.ApiName)
	}

	return &SecureAPI{
		Construct:       this,
		Api:             api,
		Function:        secureFn,
		RateLimitedFunc: rateLimitedFn,
		WebACL:          webACL,
	}
}

// Helper function to create WAF rules
func createWAFRules() *[]*awswafv2.CfnWebACL_RuleProperty {
	return &[]*awswafv2.CfnWebACL_RuleProperty{
		// SQL Injection protection
		{
			Name:     jsii.String("SQLInjectionRule"),
			Priority: jsii.Number(1),
			Statement: &awswafv2.CfnWebACL_StatementProperty{
				SqliMatchStatement: &awswafv2.CfnWebACL_SqliMatchStatementProperty{
					FieldToMatch: &awswafv2.CfnWebACL_FieldToMatchProperty{
						AllQueryArguments: &map[string]interface{}{},
					},
					TextTransformations: &[]*awswafv2.CfnWebACL_TextTransformationProperty{
						{
							Priority: jsii.Number(0),
							Type:     jsii.String("URL_DECODE"),
						},
						{
							Priority: jsii.Number(1),
							Type:     jsii.String("HTML_ENTITY_DECODE"),
						},
					},
				},
			},
			Action: &awswafv2.CfnWebACL_RuleActionProperty{
				Block: &map[string]interface{}{},
			},
			VisibilityConfig: &awswafv2.CfnWebACL_VisibilityConfigProperty{
				CloudWatchMetricsEnabled: jsii.Bool(true),
				MetricName:               jsii.String("SQLInjectionRule"),
				SampledRequestsEnabled:   jsii.Bool(true),
			},
		},
		// XSS protection
		{
			Name:     jsii.String("XSSRule"),
			Priority: jsii.Number(2),
			Statement: &awswafv2.CfnWebACL_StatementProperty{
				XssMatchStatement: &awswafv2.CfnWebACL_XssMatchStatementProperty{
					FieldToMatch: &awswafv2.CfnWebACL_FieldToMatchProperty{
						Body: &awswafv2.CfnWebACL_BodyProperty{
							OversizeHandling: jsii.String("CONTINUE"),
						},
					},
					TextTransformations: &[]*awswafv2.CfnWebACL_TextTransformationProperty{
						{
							Priority: jsii.Number(0),
							Type:     jsii.String("URL_DECODE"),
						},
						{
							Priority: jsii.Number(1),
							Type:     jsii.String("HTML_ENTITY_DECODE"),
						},
					},
				},
			},
			Action: &awswafv2.CfnWebACL_RuleActionProperty{
				Block: &map[string]interface{}{},
			},
			VisibilityConfig: &awswafv2.CfnWebACL_VisibilityConfigProperty{
				CloudWatchMetricsEnabled: jsii.Bool(true),
				MetricName:               jsii.String("XSSRule"),
				SampledRequestsEnabled:   jsii.Bool(true),
			},
		},
		// Rate limiting at WAF level
		{
			Name:     jsii.String("RateLimitRule"),
			Priority: jsii.Number(3),
			Statement: &awswafv2.CfnWebACL_StatementProperty{
				RateBasedStatement: &awswafv2.CfnWebACL_RateBasedStatementProperty{
					Limit:            jsii.Number(2000), // 2000 requests per 5 minutes
					AggregateKeyType: jsii.String("IP"),
				},
			},
			Action: &awswafv2.CfnWebACL_RuleActionProperty{
				Block: &map[string]interface{}{},
			},
			VisibilityConfig: &awswafv2.CfnWebACL_VisibilityConfigProperty{
				CloudWatchMetricsEnabled: jsii.Bool(true),
				MetricName:               jsii.String("RateLimitRule"),
				SampledRequestsEnabled:   jsii.Bool(true),
			},
		},
	}
}

// Helper function to create WAF visibility config
func createWAFVisibilityConfig(apiName *string) *awswafv2.CfnWebACL_VisibilityConfigProperty {
	return &awswafv2.CfnWebACL_VisibilityConfigProperty{
		CloudWatchMetricsEnabled: jsii.Bool(true),
		MetricName:               jsii.String(*apiName + "-waf"),
		SampledRequestsEnabled:   jsii.Bool(true),
	}
}

// Helper function to create security alarms
func createSecurityAlarms(scope constructs.Construct, fn awslambda.Function, topic awssns.ITopic, apiName *string) {
	// High error rate alarm
	errorAlarm := fn.MetricErrors(nil).CreateAlarm(scope, jsii.String("HighErrorRateAlarm"), &awscloudwatch.CreateAlarmOptions{
		AlarmName:         jsii.String(*apiName + "-high-error-rate"),
		AlarmDescription:  jsii.String("High error rate detected in secure API"),
		Threshold:         jsii.Number(10),
		EvaluationPeriods: jsii.Number(2),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	errorAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(topic))

	// Throttling alarm
	throttleAlarm := fn.MetricThrottles(nil).CreateAlarm(scope, jsii.String("ThrottlingAlarm"), &awscloudwatch.CreateAlarmOptions{
		AlarmName:         jsii.String(*apiName + "-throttling"),
		AlarmDescription:  jsii.String("API throttling detected"),
		Threshold:         jsii.Number(5),
		EvaluationPeriods: jsii.Number(1),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
	throttleAlarm.AddAlarmAction(awscloudwatchactions.NewSnsAction(topic))
}

// GetApiUrl returns the API URL
func (api *SecureAPI) GetApiUrl() *string {
	return api.Api.GetUrl()
}

// GetFunction returns the secure Lambda function
func (api *SecureAPI) GetFunction() *liftconstructs.SecureFunction {
	return api.Function
}

// GetApi returns the API Gateway construct
func (api *SecureAPI) GetApi() *liftconstructs.LiftAPI {
	return api.Api
}

// GetWebACL returns the WAF WebACL
func (api *SecureAPI) GetWebACL() awswafv2.CfnWebACL {
	return api.WebACL
}
