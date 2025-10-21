package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

// TestRateLimitedFunctionIntegration tests the complete integration of RateLimitedFunction
// with DynamORM table creation and permission configuration
func TestRateLimitedFunctionIntegration(t *testing.T) {
	t.Run("complete IP rate limiting setup", func(t *testing.T) {
		// Create test stack
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("IntegrationTestStack"), nil)

		// Create rate-limited function
		fn := NewRateLimitedFunction(stack, jsii.String("IPRateLimitedFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Runtime:    awslambda.Runtime_NODEJS_18_X(),
					Handler:    jsii.String("index.handler"),
					Code:       awslambda.Code_FromInline(jsii.String("exports.handler = async (event) => { return { statusCode: 200 }; };")),
					MemorySize: jsii.Number(1024),
					Environment: &map[string]*string{
						"APP_NAME": jsii.String("test-app"),
					},
				},
				EnableTracing:     jsii.Bool(true),
				EnableMetrics:     jsii.Bool(true),
				EnableMultiTenant: jsii.Bool(false),
			},
			RateLimitType: RateLimitTypeIP,
			WindowSeconds: jsii.Number(3600),
			Limit:         jsii.Number(1000),
			EnableMetrics: jsii.Bool(true),
		})

		// Get synthesized template
		template := assertions.Template_FromStack(stack, nil)

		// Verify Lambda function configuration
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"Runtime":       "nodejs18.x",
			"Architectures": []interface{}{"arm64"},
			"MemorySize":    1024,
			"TracingConfig": map[string]interface{}{
				"Mode": "Active",
			},
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					// App-specific vars
					"APP_NAME": "test-app",
					// Lift vars
					"LIFT_VERSION":         "1.0.0",
					"LIFT_METRICS_ENABLED": "true",
					// Rate limiting vars
					"RATE_LIMIT_ENABLED":         "true",
					"RATE_LIMIT_TYPE":            "IP",
					"RATE_LIMIT_WINDOW":          "3600",
					"RATE_LIMIT_MAX":             "1000",
					"RATE_LIMIT_METRICS_ENABLED": "true",
					// DynamORM vars
					"DYNAMORM_DEBUG":              "false",
					"DYNAMORM_RETRY_MAX_ATTEMPTS": "3",
					"DYNAMORM_RETRY_BASE_DELAY":   "100",
					// Limited library vars
					"LIMITED_ENABLED": "true",
					"LIMITED_BACKEND": "dynamorm",
				},
			},
		})

		// Verify DynamoDB table configuration
		// Note: GSIs are now handled by DynamORM through struct tags at runtime
		template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
			"KeySchema": []interface{}{
				map[string]interface{}{
					"AttributeName": "PK",
					"KeyType":       "HASH",
				},
				map[string]interface{}{
					"AttributeName": "SK",
					"KeyType":       "RANGE",
				},
			},
			"AttributeDefinitions": []interface{}{
				map[string]interface{}{
					"AttributeName": "PK",
					"AttributeType": "S",
				},
				map[string]interface{}{
					"AttributeName": "SK",
					"AttributeType": "S",
				},
			},
			"BillingMode": "PAY_PER_REQUEST",
			"TimeToLiveSpecification": map[string]interface{}{
				"AttributeName": "expires_at",
				"Enabled":       true,
			},
		})

		// Verify IAM permissions include DynamoDB access
		template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), &map[string]interface{}{
			"PolicyDocument": map[string]interface{}{
				"Statement": assertions.Match_ArrayWith(&[]interface{}{
					// Check for DynamoDB permissions
					map[string]interface{}{
						"Action": assertions.Match_ArrayWith(&[]interface{}{
							"dynamodb:GetItem",
							"dynamodb:PutItem",
						}),
						"Effect":   "Allow",
						"Resource": assertions.Match_AnyValue(),
					},
				}),
			},
		})

		// Verify function references
		assert.NotNil(t, fn.GetFunction())
		assert.NotNil(t, fn.GetTable())
		assert.NotNil(t, fn.GetTable().GetTableName())
		assert.NotNil(t, fn.GetTable().GetTableArn())
	})

	t.Run("multi-tenant rate limiting with custom table", func(t *testing.T) {
		// Create test stack
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("MultiTenantTestStack"), nil)

		// Create rate-limited function with multi-tenant support
		fn := NewRateLimitedFunction(stack, jsii.String("TenantRateLimitedFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Runtime: awslambda.Runtime_NODEJS_18_X(),
					Handler: jsii.String("index.handler"),
					Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async (event) => { return { statusCode: 200 }; };")),
				},
				EnableMultiTenant: jsii.Bool(true),
			},
			RateLimitType: RateLimitTypeTenant,
			WindowSeconds: jsii.Number(3600),
			Limit:         jsii.Number(5000),
			TableName:     jsii.String("shared-tenant-rate-limits"),
			EnableMetrics: jsii.Bool(true),
		})

		// Get synthesized template
		template := assertions.Template_FromStack(stack, nil)

		// Verify multi-tenant configuration
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					"LIFT_MULTI_TENANT": "true",
					"RATE_LIMIT_TYPE":   "TENANT",
					"RATE_LIMIT_WINDOW": "3600",
					"RATE_LIMIT_MAX":    "5000",
				},
			},
		})

		// Verify custom table name
		template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
			"TableName": "shared-tenant-rate-limits",
		})

		assert.NotNil(t, fn)
	})

	t.Run("user rate limiting with all features", func(t *testing.T) {
		// Create test stack
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("UserRateLimitTestStack"), nil)

		// Create fully-featured rate-limited function
		fn := NewRateLimitedFunction(stack, jsii.String("UserRateLimitedFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Runtime:    awslambda.Runtime_NODEJS_18_X(),
					Handler:    jsii.String("index.handler"),
					Code:       awslambda.Code_FromInline(jsii.String("exports.handler = async (event) => { return { statusCode: 200 }; };")),
					MemorySize: jsii.Number(2048),
					Timeout:    awscdk.Duration_Minutes(jsii.Number(5)),
					Environment: &map[string]*string{
						"API_KEY":   jsii.String("test-key"),
						"LOG_LEVEL": jsii.String("debug"),
					},
				},
				EnableTracing:                jsii.Bool(true),
				EnableMetrics:                jsii.Bool(true),
				EnableMultiTenant:            jsii.Bool(true),
				ReservedConcurrentExecutions: jsii.Number(10),
			},
			RateLimitType: RateLimitTypeUser,
			WindowSeconds: jsii.Number(900), // 15 minutes
			Limit:         jsii.Number(100),
			EnableMetrics: jsii.Bool(true),
		})

		// Get synthesized template
		template := assertions.Template_FromStack(stack, nil)

		// Verify all features are enabled
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"MemorySize":                   2048,
			"Timeout":                      300,
			"ReservedConcurrentExecutions": 10,
			"TracingConfig": map[string]interface{}{
				"Mode": "Active",
			},
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					// Custom vars preserved
					"API_KEY":   "test-key",
					"LOG_LEVEL": "debug",
					// All feature flags
					"LIFT_MULTI_TENANT":          "true",
					"LIFT_METRICS_ENABLED":       "true",
					"RATE_LIMIT_TYPE":            "USER",
					"RATE_LIMIT_WINDOW":          "900",
					"RATE_LIMIT_MAX":             "100",
					"RATE_LIMIT_METRICS_ENABLED": "true",
				},
			},
		})

		// Lambda automatically manages its own LogGroup

		assert.NotNil(t, fn)
	})
}

// TestRateLimitedFunctionEdgeCases tests edge cases and error scenarios
func TestRateLimitedFunctionEdgeCases(t *testing.T) {
	t.Run("handles nil values gracefully", func(t *testing.T) {
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("EdgeCaseStack"), nil)

		// Create function with minimal props
		fn := NewRateLimitedFunction(stack, jsii.String("MinimalFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Runtime: awslambda.Runtime_NODEJS_18_X(),
					Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async (event) => { return { statusCode: 200 }; };")),
					Handler: jsii.String("index.handler"),
				},
			},
		})

		// Should use all defaults without panicking
		assert.NotNil(t, fn)
		assert.NotNil(t, fn.Function)
		assert.NotNil(t, fn.RateTable)

		// Get synthesized template
		template := assertions.Template_FromStack(stack, nil)

		// Verify defaults are applied
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"Runtime":       "nodejs18.x",
			"Architectures": []interface{}{"arm64"},
			"MemorySize":    512,
			"Timeout":       30,
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					"RATE_LIMIT_TYPE":    "IP",
					"RATE_LIMIT_WINDOW":  "3600",
					"RATE_LIMIT_MAX":     "1000",
					"RATE_LIMIT_ENABLED": "true",
				},
			},
		})
	})

	t.Run("preserves existing environment variables", func(t *testing.T) {
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("EnvVarStack"), nil)

		// Create function with conflicting env vars
		existingEnv := &map[string]*string{
			"RATE_LIMIT_TYPE": jsii.String("CUSTOM"), // This should be overwritten
			"CUSTOM_VAR":      jsii.String("value"),  // This should be preserved
		}

		fn := NewRateLimitedFunction(stack, jsii.String("EnvFunction"), &RateLimitedFunctionProps{
			LiftFunctionProps: LiftFunctionProps{
				FunctionProps: awslambda.FunctionProps{
					Runtime:     awslambda.Runtime_NODEJS_18_X(),
					Code:        awslambda.Code_FromInline(jsii.String("exports.handler = async (event) => { return { statusCode: 200 }; };")),
					Handler:     jsii.String("index.handler"),
					Environment: existingEnv,
				},
			},
			RateLimitType: RateLimitTypeUser,
		})

		// Get synthesized template
		template := assertions.Template_FromStack(stack, nil)

		// Verify env var handling
		template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
			"Environment": map[string]interface{}{
				"Variables": map[string]interface{}{
					"RATE_LIMIT_TYPE": "USER",  // Should override
					"CUSTOM_VAR":      "value", // Should preserve
				},
			},
		})

		assert.NotNil(t, fn)
	})
}
