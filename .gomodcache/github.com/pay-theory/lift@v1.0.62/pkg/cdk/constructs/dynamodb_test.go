package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

// testTableKeySchema is a helper to test table key schema creation
func testTableKeySchema(t *testing.T, testName string, props *LiftTableProps, expectedPK, expectedSK string) {
	t.Helper()

	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)
	table := NewLiftTable(stack, jsii.String(testName), props)

	assert.NotNil(t, table)
	assert.NotNil(t, table.Table)

	// Synthesize and check CloudFormation
	template := assertions.Template_FromStack(stack, nil)

	// Build expected key schema
	keySchema := []interface{}{
		map[string]interface{}{
			"AttributeName": expectedPK,
			"KeyType":       "HASH",
		},
	}
	attributeDefs := []interface{}{
		map[string]interface{}{
			"AttributeName": expectedPK,
			"AttributeType": "S",
		},
	}

	if expectedSK != "" {
		keySchema = append(keySchema, map[string]interface{}{
			"AttributeName": expectedSK,
			"KeyType":       "RANGE",
		})
		attributeDefs = append(attributeDefs, map[string]interface{}{
			"AttributeName": expectedSK,
			"AttributeType": "S",
		})
	}

	// Check that table has expected attribute names
	template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]interface{}{
		"KeySchema":            keySchema,
		"AttributeDefinitions": assertions.Match_ArrayWith(&attributeDefs),
	})
}

func TestLiftTable_CreatesTableWithFieldNames(t *testing.T) {
	// Test default behavior (PK/SK)
	t.Run("Default PK/SK attributes", func(t *testing.T) {
		testTableKeySchema(t, "DefaultTable", &LiftTableProps{
			TableName:        jsii.String("test-table"),
			PartitionKeyName: jsii.String("PK"),
			SortKeyName:      jsii.String("SK"),
		}, "PK", "SK")
	})

	// Test custom field names
	t.Run("Custom field name attributes", func(t *testing.T) {
		testTableKeySchema(t, "CustomTable", &LiftTableProps{
			TableName:        jsii.String("custom-table"),
			PartitionKeyName: jsii.String("ID"),
			SortKeyName:      jsii.String("Version"),
		}, "ID", "Version")
	})

	// Test single key table
	t.Run("Single key table", func(t *testing.T) {
		testTableKeySchema(t, "SingleKeyTable", &LiftTableProps{
			TableName:        jsii.String("single-key-table"),
			PartitionKeyName: jsii.String("PK"),
			// No SortKeyName specified
		}, "PK", "")
	})
}

// testWrapperTableUsesPKSK verifies wrapper tables use standard PK/SK fields
func testWrapperTableUsesPKSK(t *testing.T, stackName string, createTable func(constructs.Construct, *string) constructs.Construct) {
	t.Helper()

	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String(stackName), nil)

	table := createTable(stack, jsii.String("TestTable"))
	assert.NotNil(t, table)

	template := assertions.Template_FromStack(stack, nil)

	// Verify it uses PK/SK as field names
	template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), map[string]interface{}{
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
	})
}

func TestWrapperConstructs_UseCorrectFieldNames(t *testing.T) {
	t.Run("ConnectionTable uses PK/SK", func(t *testing.T) {
		testWrapperTableUsesPKSK(t, "ConnectionStack", func(scope constructs.Construct, id *string) constructs.Construct {
			return NewConnectionTable(scope, id, &ConnectionTableProps{
				TableName: jsii.String("connections"),
			})
		})
	})

	t.Run("RateLimitTable uses PK/SK", func(t *testing.T) {
		testWrapperTableUsesPKSK(t, "RateLimitStack", func(scope constructs.Construct, id *string) constructs.Construct {
			return NewRateLimitTable(scope, id, &RateLimitTableProps{
				TableName: jsii.String("rate-limits"),
			})
		})
	})
}
