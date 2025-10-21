package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/jsii-runtime-go"
)

func TestLiftLambdaRole_BasicRole(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a basic Lambda role
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		RoleName: jsii.String("test-lambda-role"),
	})

	// Assert role was created
	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert basic execution policy is attached
	template.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]interface{}{
		"AssumeRolePolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": "sts:AssumeRole",
					"Effect": "Allow",
					"Principal": map[string]interface{}{
						"Service": "lambda.amazonaws.com",
					},
				},
			},
		},
		"ManagedPolicyArns": []interface{}{
			map[string]interface{}{
				"Fn::Join": []interface{}{
					"",
					[]interface{}{
						"arn:",
						map[string]interface{}{"Ref": "AWS::Partition"},
						":iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
					},
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithDynamoDB(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a DynamoDB table
	table := awsdynamodb.NewTable(stack, jsii.String("TestTable"), &awsdynamodb.TableProps{
		TableName: jsii.String("test-table"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("pk"),
			Type: awsdynamodb.AttributeType_STRING,
		},
	})

	// Create role with DynamoDB access
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		DynamoDBTables: []awsdynamodb.ITable{table},
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert DynamoDB policy is attached with expected actions
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []interface{}{
						"dynamodb:DescribeTable",
						"dynamodb:Query",
						"dynamodb:Scan",
						"dynamodb:GetItem",
						"dynamodb:PutItem",
						"dynamodb:UpdateItem",
						"dynamodb:DeleteItem",
						"dynamodb:BatchGetItem",
						"dynamodb:BatchWriteItem",
					},
					"Effect": "Allow",
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithKMS(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create a KMS key
	key := awskms.NewKey(stack, jsii.String("TestKey"), &awskms.KeyProps{
		Description: jsii.String("Test key"),
	})

	// Create role with KMS access
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		KMSKeys: []awskms.IKey{key},
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert KMS policy is attached with expected actions
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []interface{}{
						"kms:Decrypt",
						"kms:Encrypt",
						"kms:GenerateDataKey",
						"kms:DescribeKey",
					},
					"Effect": "Allow",
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithMultiRegionKMS(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role with multi-region KMS access
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		EnableMultiRegionKMS: jsii.Bool(true),
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert multi-region KMS policy includes GenerateMac and VerifyMac
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []interface{}{
						"kms:GenerateMac",
						"kms:VerifyMac",
					},
					"Effect": "Allow",
					"Resource": map[string]interface{}{
						"Fn::Join": []interface{}{
							"",
							[]interface{}{
								"arn:aws:kms:*:",
								map[string]interface{}{
									"Ref": "AWS::AccountId",
								},
								":key/mrk-*",
							},
						},
					},
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithSecretsManager(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role with Secrets Manager access
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		SecretsManagerArns: []string{
			"arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret",
		},
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert Secrets Manager policy is attached
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []interface{}{
						"secretsmanager:GetSecretValue",
						"secretsmanager:DescribeSecret",
					},
					"Effect": "Allow",
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithSSMParameterStore(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role with SSM access
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		SSMParameterPaths: []string{
			"/k3/config/*",
			"/k3/secrets/*",
		},
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert SSM policy is attached
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []interface{}{
						"ssm:GetParameter",
						"ssm:GetParameters",
						"ssm:GetParametersByPath",
					},
					"Effect": "Allow",
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithPaymentCryptography(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role with Payment Cryptography access
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		EnablePaymentCrypto: jsii.Bool(true),
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert Payment Cryptography policy is attached
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": []interface{}{
				map[string]interface{}{
					"Action": []interface{}{
						"payment-cryptography:DecryptData",
						"payment-cryptography:EncryptData",
						"payment-cryptography:GetAlias",
					},
					"Effect":   "Allow",
					"Resource": "*",
				},
			},
		},
	})
}

func TestLiftLambdaRole_WithCustomManagedPolicies(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role with custom managed policies
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		ManagedPolicyArns: []string{
			"arn:aws:iam::058264189048:policy/kernel-common-sqs-policy",
			"arn:aws:iam::058264189048:policy/kernel-common-service-policy",
		},
		EnableCloudWatchInsights: jsii.Bool(true),
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert CloudWatch Insights policy is attached
	template.HasResourceProperties(jsii.String("AWS::IAM::Role"), map[string]interface{}{
		"ManagedPolicyArns": assertions.Match_ArrayWith(&[]interface{}{
			map[string]interface{}{
				"Fn::Join": []interface{}{
					"",
					[]interface{}{
						"arn:",
						map[string]interface{}{"Ref": "AWS::Partition"},
						":iam::aws:policy/CloudWatchLambdaInsightsExecutionRolePolicy",
					},
				},
			},
		}),
	})
}

func TestLiftLambdaRole_WithInlinePolicy(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create custom inline policy
	inlinePolicy := awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
		Statements: &[]awsiam.PolicyStatement{
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Effect: awsiam.Effect_ALLOW,
				Actions: &[]*string{
					jsii.String("logs:CreateLogGroup"),
				},
				Resources: &[]*string{
					jsii.String("*"),
				},
			}),
		},
	})

	// Create role with inline policy
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		InlinePolicies: map[string]awsiam.PolicyDocument{
			"CustomPolicy": inlinePolicy,
		},
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert inline policy is attached
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1)) // 1 inline policy
}

func TestLiftLambdaRole_WithAdditionalStatements(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role with additional policy statements
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{
		AdditionalPolicyStatements: []awsiam.PolicyStatement{
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Effect: awsiam.Effect_ALLOW,
				Actions: &[]*string{
					jsii.String("ec2:DescribeInstances"),
				},
				Resources: &[]*string{
					jsii.String("*"),
				},
			}),
		},
	})

	if liftRole.Role == nil {
		t.Fatal("Role should not be nil")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Assert additional statement is in the policy
	template.HasResourceProperties(jsii.String("AWS::IAM::Policy"), map[string]interface{}{
		"PolicyDocument": map[string]interface{}{
			"Statement": assertions.Match_ArrayWith(&[]interface{}{
				map[string]interface{}{
					"Action":   "ec2:DescribeInstances",
					"Effect":   "Allow",
					"Resource": "*",
				},
			}),
		},
	})
}

func TestLiftLambdaRole_GrantMethods(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create role
	liftRole := NewLiftLambdaRole(stack, jsii.String("TestRole"), &LiftLambdaRoleProps{})

	// Create resources to grant access to
	table := awsdynamodb.NewTable(stack, jsii.String("Table"), &awsdynamodb.TableProps{
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("pk"),
			Type: awsdynamodb.AttributeType_STRING,
		},
	})

	key := awskms.NewKey(stack, jsii.String("Key"), &awskms.KeyProps{
		Description: jsii.String("Test key"),
	})

	// Grant access using helper methods
	liftRole.GrantDynamoDBAccess(table)
	liftRole.GrantKMSAccess(key)

	template := assertions.Template_FromStack(stack, nil)

	// Assert permissions were granted
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1))
}
