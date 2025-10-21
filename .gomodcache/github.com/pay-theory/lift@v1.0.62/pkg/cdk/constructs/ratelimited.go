package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// RateLimitType defines the type of rate limiting
type RateLimitType string

const (
	RateLimitTypeIP     RateLimitType = "IP"
	RateLimitTypeUser   RateLimitType = "USER"
	RateLimitTypeTenant RateLimitType = "TENANT"
)

// RateLimitedFunctionProps extends LiftFunctionProps with rate limiting configuration
// Memory optimized: 760 → 752 bytes (8 bytes saved)
type RateLimitedFunctionProps struct {
	// Embedded struct first (largest)
	LiftFunctionProps
	// Pointers (8 bytes each)
	WindowSeconds *float64
	Limit         *float64
	TableName     *string
	EnableMetrics *bool
	// Smaller types last
	RateLimitType RateLimitType
}

// RateLimitedFunction is a Lambda function with built-in rate limiting using DynamORM
type RateLimitedFunction struct {
	constructs.Construct
	Function      *LiftFunction
	RateTable     *LiftTable
	rateLimitType RateLimitType
}

// NewRateLimitedFunction creates a Lambda function with rate limiting capabilities
func NewRateLimitedFunction(scope constructs.Construct, id *string, props *RateLimitedFunctionProps) *RateLimitedFunction {
	this := constructs.NewConstruct(scope, id)

	// Set defaults
	if props.WindowSeconds == nil {
		props.WindowSeconds = jsii.Number(3600) // 1 hour default
	}
	if props.Limit == nil {
		props.Limit = jsii.Number(1000) // 1000 requests per window default
	}
	if props.RateLimitType == "" {
		props.RateLimitType = RateLimitTypeIP
	}
	if props.EnableMetrics == nil {
		props.EnableMetrics = jsii.Bool(true)
	}

	// Create or reference the rate limiting table
	var rateTable *LiftTable
	tableName := props.TableName
	if tableName == nil {
		tableName = jsii.String(fmt.Sprintf("%s-rate-limits", *id))
	}

	// Create DynamORM-compatible rate limit table
	rateTable = NewRateLimitTable(this, jsii.String("RateTable"), &RateLimitTableProps{
		TableName: tableName,
	})

	// Add rate limiting environment variables
	if props.Environment == nil {
		props.Environment = &map[string]*string{}
	}
	env := *props.Environment

	// DynamORM table configuration
	env["RATE_LIMIT_TABLE_NAME"] = rateTable.Table.TableName()
	env["DYNAMORM_REGION"] = awscdk.Stack_Of(this).Region()

	// Rate limiting configuration
	env["RATE_LIMIT_TYPE"] = jsii.String(string(props.RateLimitType))
	env["RATE_LIMIT_WINDOW"] = jsii.String(fmt.Sprintf("%.0f", *props.WindowSeconds))
	env["RATE_LIMIT_MAX"] = jsii.String(fmt.Sprintf("%.0f", *props.Limit))
	env["RATE_LIMIT_ENABLED"] = jsii.String("true")

	// DynamORM configuration
	env["DYNAMORM_DEBUG"] = jsii.String("false")
	env["DYNAMORM_RETRY_MAX_ATTEMPTS"] = jsii.String("3")
	env["DYNAMORM_RETRY_BASE_DELAY"] = jsii.String("100")

	// Limited library configuration
	env["LIMITED_ENABLED"] = jsii.String("true")
	env["LIMITED_BACKEND"] = jsii.String("dynamorm")

	if *props.EnableMetrics {
		env["RATE_LIMIT_METRICS_ENABLED"] = jsii.String("true")
	}
	props.Environment = &env

	// Create the base Lift function
	liftFn := NewLiftFunction(this, jsii.String("Function"), &props.LiftFunctionProps)

	// Grant table permissions
	rateTable.GrantReadWrite(liftFn.Function)

	// Add CloudWatch metrics permissions if enabled
	if *props.EnableMetrics {
		liftFn.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Actions: &[]*string{
				jsii.String("cloudwatch:PutMetricData"),
			},
			Resources: &[]*string{jsii.String("*")},
			Conditions: &map[string]interface{}{
				"StringEquals": map[string]interface{}{
					"cloudwatch:namespace": "Lift/RateLimits",
				},
			},
		}))
	}

	return &RateLimitedFunction{
		Construct:     this,
		Function:      liftFn,
		RateTable:     rateTable,
		rateLimitType: props.RateLimitType,
	}
}

// GetFunction returns the underlying Lambda function
func (f *RateLimitedFunction) GetFunction() awslambda.Function {
	return f.Function.Function
}

// GetTable returns the rate limiting table
func (f *RateLimitedFunction) GetTable() *LiftTable {
	return f.RateTable
}

// AddRateLimitAlarm adds a CloudWatch alarm for rate limit violations
func (f *RateLimitedFunction) AddRateLimitAlarm(alarmName *string, threshold *float64) awscloudwatch.IAlarm {
	// Default values
	if alarmName == nil {
		alarmName = jsii.String(fmt.Sprintf("%s-RateLimitAlarm", *f.Function.Function.FunctionName()))
	}
	if threshold == nil {
		threshold = jsii.Number(10) // Default to 10 rate limit violations
	}

	// Create a metric for rate limit violations
	metric := awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
		Namespace:  jsii.String("LiftApp/RateLimiting"),
		MetricName: jsii.String("RateLimitExceeded"),
		DimensionsMap: &map[string]*string{
			"FunctionName":  f.Function.Function.FunctionName(),
			"RateLimitType": jsii.String(string(f.rateLimitType)),
		},
		Statistic: jsii.String("Sum"),
		Period:    awscdk.Duration_Minutes(jsii.Number(5)),
	})

	// Create the alarm
	alarm := awscloudwatch.NewAlarm(f, alarmName, &awscloudwatch.AlarmProps{
		Metric:            metric,
		Threshold:         threshold,
		EvaluationPeriods: jsii.Number(1),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
		AlarmDescription:  jsii.String(fmt.Sprintf("Alarm when rate limit violations exceed %v in 5 minutes", *threshold)),
	})

	// Note: Add alarm actions to an SNS topic if needed

	return alarm
}
