package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// IdempotentKeyExtractor defines how to extract idempotency keys
type IdempotentKeyExtractor string

const (
	// Extract from X-Idempotency-Key header
	IdempotentKeyHeader IdempotentKeyExtractor = "HEADER"
	// Extract from request body field
	IdempotentKeyBody IdempotentKeyExtractor = "BODY"
	// Extract from path parameter
	IdempotentKeyPath IdempotentKeyExtractor = "PATH"
	// Custom extraction logic in Lambda
	IdempotentKeyCustom IdempotentKeyExtractor = "CUSTOM"
)

// IdempotentFunctionProps extends LiftFunctionProps with idempotency configuration
// Memory optimized: 768 → 760 bytes (8 bytes saved)
type IdempotentFunctionProps struct {
	// Embedded struct first (largest)
	LiftFunctionProps
	// Pointers (8 bytes each)
	KeyField              *string
	TTLSeconds            *float64
	TableName             *string
	EnableResponseCaching *bool
	MaxResponseSizeKB     *float64
	// Smaller types last
	KeyExtractor IdempotentKeyExtractor
}

// IdempotentFunction is a Lambda function with built-in idempotency support using DynamORM
type IdempotentFunction struct {
	constructs.Construct
	Function         *LiftFunction
	IdempotencyTable *LiftTable
}

// NewIdempotentFunction creates a Lambda function with idempotency capabilities
func NewIdempotentFunction(scope constructs.Construct, id *string, props *IdempotentFunctionProps) *IdempotentFunction {
	this := constructs.NewConstruct(scope, id)

	// Set defaults
	if props.KeyExtractor == "" {
		props.KeyExtractor = IdempotentKeyHeader
	}
	if props.KeyField == nil {
		switch props.KeyExtractor {
		case IdempotentKeyHeader:
			props.KeyField = jsii.String("X-Idempotency-Key")
		default:
			props.KeyField = jsii.String("idempotencyKey")
		}
	}
	if props.TTLSeconds == nil {
		props.TTLSeconds = jsii.Number(86400) // 24 hours default
	}
	if props.EnableResponseCaching == nil {
		props.EnableResponseCaching = jsii.Bool(true)
	}
	if props.MaxResponseSizeKB == nil {
		props.MaxResponseSizeKB = jsii.Number(400) // 400KB default (DynamoDB item limit)
	}

	// Create or reference the idempotency table
	var idempotencyTable *LiftTable
	tableName := props.TableName
	if tableName == nil {
		tableName = jsii.String(fmt.Sprintf("%s-idempotency", *id))
	}

	// Create DynamORM-compatible idempotency table
	idempotencyTable = NewIdempotencyTable(this, jsii.String("IdempotencyTable"), &IdempotencyTableProps{
		TableName: tableName,
	})

	// Add idempotency environment variables
	if props.Environment == nil {
		props.Environment = &map[string]*string{}
	}
	env := *props.Environment

	// DynamORM table configuration
	env["IDEMPOTENCY_TABLE_NAME"] = idempotencyTable.Table.TableName()
	// AWS_REGION is automatically set by Lambda runtime

	// Idempotency configuration
	env["IDEMPOTENCY_KEY_EXTRACTOR"] = jsii.String(string(props.KeyExtractor))
	env["IDEMPOTENCY_KEY_FIELD"] = props.KeyField
	env["IDEMPOTENCY_TTL_SECONDS"] = jsii.String(fmt.Sprintf("%.0f", *props.TTLSeconds))
	env["IDEMPOTENCY_ENABLED"] = jsii.String("true")

	// DynamORM configuration
	env["DYNAMORM_DEBUG"] = jsii.String("false")
	env["DYNAMORM_RETRY_MAX_ATTEMPTS"] = jsii.String("2")
	env["DYNAMORM_RETRY_BASE_DELAY"] = jsii.String("100")

	// Response caching configuration
	if *props.EnableResponseCaching {
		env["IDEMPOTENCY_CACHE_RESPONSES"] = jsii.String("true")
		env["IDEMPOTENCY_MAX_RESPONSE_KB"] = jsii.String(fmt.Sprintf("%.0f", *props.MaxResponseSizeKB))
	}

	// Function name for tracking
	env["IDEMPOTENCY_FUNCTION_NAME"] = jsii.String(*id)

	props.Environment = &env

	// Create the base Lift function
	liftFn := NewLiftFunction(this, jsii.String("Function"), &props.LiftFunctionProps)

	// Grant table permissions
	idempotencyTable.GrantReadWrite(liftFn.Function)

	// Add CloudWatch metrics permissions
	liftFn.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("cloudwatch:PutMetricData"),
		},
		Resources: &[]*string{jsii.String("*")},
		Conditions: &map[string]interface{}{
			"StringEquals": map[string]interface{}{
				"cloudwatch:namespace": "Lift/Idempotency",
			},
		},
	}))

	return &IdempotentFunction{
		Construct:        this,
		Function:         liftFn,
		IdempotencyTable: idempotencyTable,
	}
}

// GetFunction returns the underlying Lambda function
func (f *IdempotentFunction) GetFunction() awslambda.Function {
	return f.Function.Function
}

// GetTable returns the idempotency tracking table
func (f *IdempotentFunction) GetTable() *LiftTable {
	return f.IdempotencyTable
}

// EnableTransactionSupport adds permissions for DynamoDB transactions
func (f *IdempotentFunction) EnableTransactionSupport() {
	f.Function.Function.AddToRolePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("dynamodb:ConditionCheckItem"),
			jsii.String("dynamodb:TransactWriteItems"),
		},
		Resources: &[]*string{
			f.IdempotencyTable.GetTableArn(),
			jsii.String(fmt.Sprintf("%s/index/*", *f.IdempotencyTable.GetTableArn())),
		},
	}))
}

// AddIdempotencyMetrics adds CloudWatch metrics for idempotency operations
func (f *IdempotentFunction) AddIdempotencyMetrics(namespace *string) {
	if namespace == nil {
		namespace = jsii.String("Lift/Idempotency")
	}

	// Add environment variable for custom namespace
	f.Function.Function.AddEnvironment(jsii.String("IDEMPOTENCY_METRICS_NAMESPACE"), namespace, nil)
}
