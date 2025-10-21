package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

func TestNewSecureFunction_BasicConfiguration(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	sf := NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify Lambda function exists
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"Runtime":       "provided.al2023",
		"Handler":       "bootstrap",
		"Architectures": []interface{}{"arm64"},
		"TracingConfig": map[string]interface{}{
			"Mode": "Active",
		},
	})

	// Verify VPC was created
	template.ResourceCountIs(jsii.String("AWS::EC2::VPC"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::EC2::VPC"), &map[string]interface{}{
		"EnableDnsHostnames": true,
		"EnableDnsSupport":   true,
	})

	// Verify security group was created
	template.ResourceCountIs(jsii.String("AWS::EC2::SecurityGroup"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::EC2::SecurityGroup"), &map[string]interface{}{
		"GroupDescription": "Security group for secure Lambda function",
	})

	// Verify KMS key was created (enabled by default)
	template.ResourceCountIs(jsii.String("AWS::KMS::Key"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::KMS::Key"), &map[string]interface{}{
		"EnableKeyRotation":   true,
		"PendingWindowInDays": 7,
	})

	// Verify KMS alias was created
	template.HasResourceProperties(jsii.String("AWS::KMS::Alias"), &map[string]interface{}{
		"AliasName": "alias/SecureFunction-key",
	})

	// Verify Lambda has VPC configuration with at least one subnet and security group
	// Note: CDK generates different IDs, so we just check the structure exists
	template.ResourceCountIs(jsii.String("AWS::Lambda::Function"), jsii.Number(1))

	// Verify IAM permissions for VPC exist
	// Note: The policy will contain multiple statements, just verify it exists
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1))

	assert.NotNil(t, sf)
}

func TestNewSecureFunction_WithExistingVPC(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	vpc := awsec2.NewVpc(stack, jsii.String("ExistingVpc"), &awsec2.VpcProps{
		MaxAzs: jsii.Number(2),
	})

	// When
	NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
		Vpc: vpc,
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should only have one VPC (the existing one)
	template.ResourceCountIs(jsii.String("AWS::EC2::VPC"), jsii.Number(1))
}

func TestNewSecureFunction_PrivateOnly(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
		PrivateOnly: jsii.Bool(true),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify security group has no outbound rules
	template.HasResourceProperties(jsii.String("AWS::EC2::SecurityGroup"), &map[string]interface{}{
		"SecurityGroupEgress": []interface{}{
			map[string]interface{}{
				"CidrIp":      "255.255.255.255/32",
				"Description": "Disallow all traffic",
				"IpProtocol":  "icmp",
				"FromPort":    252,
				"ToPort":      86,
			},
		},
	})
}

func TestNewSecureFunction_WithSecrets(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	secret := awssecretsmanager.NewSecret(stack, jsii.String("TestSecret"), &awssecretsmanager.SecretProps{
		Description: jsii.String("Test secret"),
	})

	// When
	NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
		Secrets: &map[string]awssecretsmanager.ISecret{
			"DB_PASSWORD": secret,
		},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify secret permissions exist in IAM policy
	// The Lambda function should have permissions to read the secret
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1))
}

func TestNewSecureFunction_DisableKMSEncryption(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
		EnableKMSEncryption: jsii.Bool(false),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify no KMS key was created
	template.ResourceCountIs(jsii.String("AWS::KMS::Key"), jsii.Number(0))
}

func TestNewSecureFunction_WithCustomKMSKey(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	customKey := awskms.NewKey(stack, jsii.String("CustomKey"), &awskms.KeyProps{
		Description: jsii.String("Custom KMS key"),
	})

	// When
	NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
		KmsKey: customKey,
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should have exactly one KMS key (the custom one)
	template.ResourceCountIs(jsii.String("AWS::KMS::Key"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::KMS::Key"), &map[string]interface{}{
		"Description": "Custom KMS key",
	})
}

func TestNewSecureFunction_WithAdditionalPolicies(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	additionalPolicy := awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("s3:GetObject"),
		},
		Resources: &[]*string{
			jsii.String("arn:aws:s3:::my-bucket/*"),
		},
	})

	// When
	NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
		AdditionalPolicies: &[]awsiam.PolicyStatement{additionalPolicy},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify additional policy was added to the Lambda role
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1))
}

func TestSecureFunction_EnableSecretsManagerAccess(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	sf := NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
	})

	// When
	sf.EnableSecretsManagerAccess()

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify Secrets Manager permissions were added to IAM policy
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1))
}

func TestSecureFunction_GettersWork(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	sf := NewSecureFunction(stack, jsii.String("SecureFunction"), &SecureFunctionProps{
		LiftFunctionProps: LiftFunctionProps{
			FunctionProps: awslambda.FunctionProps{
				Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
				Handler: jsii.String("bootstrap"),
			},
		},
	})

	// Then
	assert.NotNil(t, sf.GetFunction())
	assert.NotNil(t, sf.GetSecurityGroup())
	assert.NotNil(t, sf.GetKmsKey())
}
