package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

func TestRateLimitedFunction(t *testing.T) {
	t.Run("creates function with default rate limiting", func(t *testing.T) {
		// Create test app and stack
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
		// Create rate limited function
		fn := NewRateLimitedFunction(stack, jsii.String("TestFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
					Handler: jsii.String("bootstrap"),
				},
			},
		})

		// Verify function was created
		assert.NotNil(t, fn)
		assert.NotNil(t, fn.Function)
		assert.NotNil(t, fn.RateTable)

		// Synthesize and check template
		template := assertions.Template_FromStack(stack, nil)

		// Check Lambda function
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"Runtime":       "provided.al2023",
			"Architectures": []interface{}{"arm64"},
			"MemorySize":    512,
			"Timeout":       30,
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					"RATE_LIMIT_ENABLED": "true",
					"RATE_LIMIT_TYPE":    "IP",
					"RATE_LIMIT_WINDOW":  "3600",
					"RATE_LIMIT_MAX":     "1000",
				},
			},
		})

		// Check DynamoDB table
		template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
			"BillingMode": "PAY_PER_REQUEST",
			"TimeToLiveSpecification": map[string]interface{}{
				"AttributeName": "expires_at",
				"Enabled":       true,
			},
		})
	})

	t.Run("creates function with custom rate limiting", func(t *testing.T) {
		// Create test app and stack
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

		// Create rate limited function with custom settings
		fn := NewRateLimitedFunction(stack, jsii.String("CustomRateFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
					Handler: jsii.String("bootstrap"),
				},
			},
			RateLimitType: RateLimitTypeUser,
			WindowSeconds: jsii.Number(300), // 5 minutes
			Limit:         jsii.Number(100),
			EnableMetrics: jsii.Bool(false),
		})

		assert.NotNil(t, fn)

		// Synthesize and check template
		template := assertions.Template_FromStack(stack, nil)

		// Check Lambda environment variables
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					"RATE_LIMIT_TYPE":   "USER",
					"RATE_LIMIT_WINDOW": "300",
					"RATE_LIMIT_MAX":    "100",
				},
			},
		})
	})

	t.Run("uses existing DynamoDB table when provided", func(t *testing.T) {
		// Create test app and stack
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

		tableName := jsii.String("existing-rate-table")

		fn := NewRateLimitedFunction(stack, jsii.String("ExistingTableFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
					Handler: jsii.String("bootstrap"),
				},
			},
			TableName: tableName,
		})

		assert.NotNil(t, fn)
		assert.NotNil(t, fn.RateTable)
	})
}

func TestRateLimitedFunctionMethods(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	fn := NewRateLimitedFunction(stack, jsii.String("TestFunction"), &RateLimitedFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
	})

	t.Run("GetFunction returns underlying function", func(t *testing.T) {
		lambdaFn := fn.GetFunction()
		assert.NotNil(t, lambdaFn)
		assert.Equal(t, fn.Function.Function, lambdaFn)
	})

	t.Run("GetTable returns rate limit table", func(t *testing.T) {
		table := fn.GetTable()
		assert.NotNil(t, table)
		assert.Equal(t, fn.RateTable, table)
	})
}
