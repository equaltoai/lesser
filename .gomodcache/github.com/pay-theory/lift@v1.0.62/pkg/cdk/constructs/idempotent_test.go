package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test helpers to reduce duplication across assertions
func assertIdempotencyTableStructure(t *testing.T, template assertions.Template) {
	t.Helper()
	template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
		"TableName":   assertions.Match_StringLikeRegexp(jsii.String(".*-idempotency")),
		"BillingMode": "PAY_PER_REQUEST",
		"AttributeDefinitions": assertions.Match_ArrayWith(&[]interface{}{
			&map[string]interface{}{
				"AttributeName": "PK",
				"AttributeType": "S",
			},
			&map[string]interface{}{
				"AttributeName": "SK",
				"AttributeType": "S",
			},
		}),
		"KeySchema": &[]interface{}{
			&map[string]interface{}{
				"AttributeName": "PK",
				"KeyType":       "HASH",
			},
			&map[string]interface{}{
				"AttributeName": "SK",
				"KeyType":       "RANGE",
			},
		},
		"TimeToLiveSpecification": &map[string]interface{}{
			"AttributeName": "expires_at",
			"Enabled":       true,
		},
	})
}

func assertLambdaHasIdempotencyPolicy(t *testing.T, template assertions.Template) {
	t.Helper()
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), &map[string]interface{}{
		"PolicyDocument": &map[string]interface{}{
			"Statement": assertions.Match_ArrayWith(&[]interface{}{
				&map[string]interface{}{
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
}

// createKeyExtractorTestCases generates test cases for different key extractors
func createKeyExtractorTestCases() []struct {
	props      *IdempotentFunctionProps
	assertions func(t *testing.T, template assertions.Template, fn *IdempotentFunction)
	name       string
} {
	// Intentionally avoid sharing Code assets across stacks/tests.
	// Each test case will construct its own FunctionProps with a fresh Code instance.

	extractorConfigs := []struct {
		name         string
		extractor    IdempotentKeyExtractor
		keyField     string
		extractorStr string
	}{
		{
			name:         "creates function with body key extractor",
			extractor:    IdempotentKeyBody,
			keyField:     "requestId",
			extractorStr: "BODY",
		},
		{
			name:         "creates function with path parameter key extractor",
			extractor:    IdempotentKeyPath,
			keyField:     "orderId",
			extractorStr: "PATH",
		},
		{
			name:         "creates function with custom key extractor",
			extractor:    IdempotentKeyCustom,
			keyField:     "customExtractorFunction",
			extractorStr: "CUSTOM",
		},
	}

	var testCases []struct {
		props      *IdempotentFunctionProps
		assertions func(t *testing.T, template assertions.Template, fn *IdempotentFunction)
		name       string
	}

	for _, config := range extractorConfigs {
		config := config // capture range variable
		testCases = append(testCases, struct {
			props      *IdempotentFunctionProps
			assertions func(t *testing.T, template assertions.Template, fn *IdempotentFunction)
			name       string
		}{
			name: config.name,
			props: &IdempotentFunctionProps{
				LiftFunctionProps: LiftFunctionProps{
					FunctionProps: awslambda.FunctionProps{
						Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
						Runtime: awslambda.Runtime_PROVIDED_AL2023(),
						Handler: jsii.String("bootstrap"),
					},
				},
				KeyExtractor: config.extractor,
				KeyField:     jsii.String(config.keyField),
			},
			assertions: func(_ *testing.T, template assertions.Template, _ *IdempotentFunction) {
				template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
					"Environment": &map[string]interface{}{
						"Variables": &map[string]interface{}{
							"IDEMPOTENCY_KEY_EXTRACTOR": config.extractorStr,
							"IDEMPOTENCY_KEY_FIELD":     config.keyField,
						},
					},
				})
			},
		})
	}

	return testCases
}

func TestNewIdempotentFunction(t *testing.T) {
	tests := []struct {
		props      *IdempotentFunctionProps
		assertions func(t *testing.T, template assertions.Template, fn *IdempotentFunction)
		name       string
	}{
		{
			name: "creates function with default idempotency settings",
			props: &IdempotentFunctionProps{
				LiftFunctionProps: LiftFunctionProps{
					FunctionProps: awslambda.FunctionProps{
						Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
						Runtime: awslambda.Runtime_PROVIDED_AL2023(),
						Handler: jsii.String("bootstrap"),
					},
				},
			},
			assertions: func(_ *testing.T, template assertions.Template, fn *IdempotentFunction) {
				// Check function and table created
				assert.NotNil(t, fn.Function)
				assert.NotNil(t, fn.IdempotencyTable)

				// Shared assertions
				assertIdempotencyTableStructure(t, template)
				assertLambdaHasIdempotencyPolicy(t, template)
			},
		},
		{
			name: "creates function with custom idempotency configuration",
			props: &IdempotentFunctionProps{
				LiftFunctionProps: LiftFunctionProps{
					FunctionProps: awslambda.FunctionProps{
						Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
						Runtime: awslambda.Runtime_PROVIDED_AL2023(),
						Handler: jsii.String("bootstrap"),
					},
				},
				TableName:    jsii.String("custom-idempotency-table"),
				KeyExtractor: IdempotentKeyHeader,
				KeyField:     jsii.String("X-Idempotency-Key"),
				TTLSeconds:   jsii.Number(48 * 3600),
			},
			assertions: func(_ *testing.T, template assertions.Template, _ *IdempotentFunction) {
				// Check custom table name
				template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
					"TableName": "custom-idempotency-table",
				})

				// Check environment variables are set
				template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
					"Environment": &map[string]interface{}{
						"Variables": &map[string]interface{}{
							"IDEMPOTENCY_TABLE_NAME":    assertions.Match_AnyValue(),
							"IDEMPOTENCY_KEY_EXTRACTOR": "HEADER",
							"IDEMPOTENCY_KEY_FIELD":     "X-Idempotency-Key",
							"IDEMPOTENCY_TTL_SECONDS":   "172800",
							"IDEMPOTENCY_ENABLED":       "true",
						},
					},
				})
			},
		},
	}

	// Append key extractor test cases
	tests = append(tests, createKeyExtractorTestCases()...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test stack
			stack := test.NewTestStack()

			// Create idempotent function
			fn := NewIdempotentFunction(stack.Stack(), jsii.String("TestFunction"), tt.props)

			// Get template
			template := assertions.Template_FromStack(stack.Stack(), nil)

			// Run assertions
			tt.assertions(t, template, fn)
		})
	}
}

func TestIdempotentFunction_Methods(t *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create idempotent function
	fn := NewIdempotentFunction(stack.Stack(), jsii.String("TestFunction"), &IdempotentFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Runtime: awslambda.Runtime_PROVIDED_AL2023(),
				Handler: jsii.String("bootstrap"),
			},
		},
		TableName: jsii.String("test-idempotency-table"),
	})

	t.Run("GetFunction returns underlying function", func(t *testing.T) {
		lambdaFn := fn.GetFunction()
		require.NotNil(t, lambdaFn)
		assert.Equal(t, fn.Function.Function, lambdaFn)
	})

	t.Run("GetTable returns idempotency table", func(t *testing.T) {
		table := fn.GetTable()
		require.NotNil(t, table)
		assert.Equal(t, fn.IdempotencyTable, table)
	})

}

func TestIdempotentFunction_Integration(t *testing.T) {
	// Create test stack
	stack := test.NewTestStack()

	// Create idempotent function with all features
	fn := NewIdempotentFunction(stack.Stack(), jsii.String("TestFunction"), &IdempotentFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				FunctionName: jsii.String("test-idempotent-function"),
				Code:         awslambda.Code_FromAsset(jsii.String("."), nil),
				Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
				Handler:      jsii.String("bootstrap"),
				Timeout:      awscdk.Duration_Seconds(jsii.Number(30)),
				MemorySize:   jsii.Number(512),
			},
			EnableTracing: jsii.Bool(true),
		},
		TableName:    jsii.String("test-idempotency"),
		KeyExtractor: IdempotentKeyHeader,
		KeyField:     jsii.String("X-Request-ID"),
		TTLSeconds:   jsii.Number(24 * 3600),
	})

	// Get template
	template := assertions.Template_FromStack(stack.Stack(), nil)

	// Assert Lambda function is created with all properties
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"FunctionName": "test-idempotent-function",
		"Runtime":      "provided.al2023",
		"Handler":      "bootstrap",
		"Timeout":      30,
		"MemorySize":   512,
		"TracingConfig": &map[string]interface{}{
			"Mode": "Active",
		},
		"Environment": &map[string]interface{}{
			"Variables": &map[string]interface{}{
				"IDEMPOTENCY_TABLE_NAME":    assertions.Match_AnyValue(),
				"IDEMPOTENCY_KEY_EXTRACTOR": "HEADER",
				"IDEMPOTENCY_KEY_FIELD":     "X-Request-ID",
				"IDEMPOTENCY_TTL_SECONDS":   "86400",
				"IDEMPOTENCY_ENABLED":       "true",
			},
		},
	})

	// Assert DynamoDB table is created
	template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
		"TableName":   "test-idempotency",
		"BillingMode": "PAY_PER_REQUEST",
	})

	// Assert IAM permissions
	assertLambdaHasIdempotencyPolicy(t, template)

	// Verify the function is properly configured
	assert.NotNil(t, fn.Function)
	assert.NotNil(t, fn.IdempotencyTable)
}
