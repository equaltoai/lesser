package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

const (
	trueStr = "true"
)

// LiftFunctionProps extends standard Lambda function properties with Lift-specific configuration
// This struct contains all configurable properties for creating a Lift-optimized
// Lambda function. It extends the standard AWS CDK Lambda function properties
// with additional Lift-specific features like tracing, metrics, multi-tenant support,
// and DynamORM configuration.
type LiftFunctionProps struct {
	awslambda.FunctionProps
	// EnableTracing enables X-Ray tracing for the function
	EnableTracing *bool
	// EnableMetrics enables CloudWatch metrics
	EnableMetrics *bool
	// EnableMultiTenant enables multi-tenant support
	EnableMultiTenant *bool
	// ReservedConcurrentExecutions to limit concurrent executions
	ReservedConcurrentExecutions *float64
	// EnableDynamORM configures DynamORM environment variables
	EnableDynamORM *bool
	// DynamORM table name (optional - for when using DynamORM)
	DynamORMTableName *string
	// DynamORM debug mode
	DynamORMDebug *bool
}

// LiftFunction is a Lambda function construct optimized for Lift applications
// This construct creates a Lambda function with Lift-optimized defaults including:
// - X-Ray tracing (if enabled)
// - CloudWatch metrics (if enabled)
// - Multi-tenant support (if enabled)
// - DynamORM environment variables (if enabled)
type LiftFunction struct {
	constructs.Construct
	Function awslambda.Function
}

// GetResourceName returns the function name
// This method returns the name of the Lambda function. This is useful for
// monitoring and identification purposes.
func (l *LiftFunction) GetResourceName() *string {
	return l.Function.FunctionName()
}

// NewLiftFunction creates a new Lift Lambda function with optimized defaults
// This function creates a new Lambda function with all Lift-optimized features including:
// - Default runtime (PROVIDED_AL2023)
// - ARM64 architecture
// - Memory size (512MB)
// - Timeout (30 seconds)
// - Tracing (if enabled)
// - Metrics (if enabled)
// - Multi-tenant support (if enabled)
// - DynamORM environment variables (if enabled)
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Configuration properties
//
// Returns:
//   - A new LiftFunction instance
func NewLiftFunction(scope constructs.Construct, id *string, props *LiftFunctionProps) *LiftFunction {
	builder := newLiftFunctionBuilder(scope, id, props)
	return builder.build()
}

// liftFunctionBuilder builds optimized Lambda functions with Lift defaults
type liftFunctionBuilder struct {
	scope     constructs.Construct
	id        *string
	props     *LiftFunctionProps
	construct constructs.Construct
}

// newLiftFunctionBuilder creates a new Lift function builder
func newLiftFunctionBuilder(scope constructs.Construct, id *string, props *LiftFunctionProps) *liftFunctionBuilder {
	return &liftFunctionBuilder{
		scope: scope,
		id:    id,
		props: props,
	}
}

// build constructs the complete Lift function
func (b *liftFunctionBuilder) build() *LiftFunction {
	b.construct = constructs.NewConstruct(b.scope, b.id)

	b.setLiftDefaults()
	b.configureTracing()
	b.configureConcurrency()
	b.configureEnvironment()
	b.configureDynamORM()

	function := awslambda.NewFunction(b.construct, jsii.String("Resource"), &b.props.FunctionProps)

	return &LiftFunction{
		Construct: b.construct,
		Function:  function,
	}
}

// setLiftDefaults applies Lift-optimized defaults
func (b *liftFunctionBuilder) setLiftDefaults() {
	if b.props.Runtime == nil {
		b.props.Runtime = awslambda.Runtime_PROVIDED_AL2023()
	}
	if b.props.Architecture == nil {
		b.props.Architecture = awslambda.Architecture_ARM_64()
	}
	if b.props.MemorySize == nil {
		b.props.MemorySize = jsii.Number(512)
	}
	if b.props.Timeout == nil {
		b.props.Timeout = awscdk.Duration_Seconds(jsii.Number(30))
	}
}

// configureTracing configures AWS X-Ray tracing
func (b *liftFunctionBuilder) configureTracing() {
	if b.props.EnableTracing != nil && *b.props.EnableTracing {
		b.props.Tracing = awslambda.Tracing_ACTIVE
	}
}

// configureConcurrency configures concurrent execution limits
func (b *liftFunctionBuilder) configureConcurrency() {
	if b.props.ReservedConcurrentExecutions != nil {
		b.props.FunctionProps.ReservedConcurrentExecutions = b.props.ReservedConcurrentExecutions
	}
}

// configureEnvironment sets up Lift-specific environment variables
func (b *liftFunctionBuilder) configureEnvironment() {
	if b.props.Environment == nil {
		b.props.Environment = &map[string]*string{}
	}

	env := *b.props.Environment
	env["LIFT_VERSION"] = jsii.String("1.0.0")

	if b.props.EnableMultiTenant != nil && *b.props.EnableMultiTenant {
		env["LIFT_MULTI_TENANT"] = jsii.String(trueStr)
	}
	if b.props.EnableMetrics != nil && *b.props.EnableMetrics {
		env["LIFT_METRICS_ENABLED"] = jsii.String(trueStr)
	}
}

// configureDynamORM configures DynamORM environment variables
func (b *liftFunctionBuilder) configureDynamORM() {
	if b.props.EnableDynamORM == nil || !*b.props.EnableDynamORM {
		return
	}

	env := *b.props.Environment
	env["DYNAMORM_REGION"] = awscdk.Stack_Of(b.construct).Region()

	if b.props.DynamORMTableName != nil {
		env["DYNAMODB_TABLE_NAME"] = b.props.DynamORMTableName
	}

	// Set debug mode
	debugMode := "false"
	if b.props.DynamORMDebug != nil && *b.props.DynamORMDebug {
		debugMode = trueStr
	}
	env["DYNAMORM_DEBUG"] = jsii.String(debugMode)

	// Set default retry configuration
	env["DYNAMORM_RETRY_MAX_ATTEMPTS"] = jsii.String("3")
	env["DYNAMORM_RETRY_BASE_DELAY"] = jsii.String("100")

	b.props.Environment = &env
}
