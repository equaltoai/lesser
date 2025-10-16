package constructs

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type CloudFrontKeyPairProps struct {
	Environment string
	SecretName  string
}

type CloudFrontKeyPair struct {
	Secret          awssecretsmanager.ISecret
	PublicKeyOutput awscdk.CfnOutput
	Function        awslambda.Function
}

// CreateCloudFrontKeyPair generates an RSA-2048 key pair at deployment time
// using a Go Lambda custom resource and stores it in Secrets Manager
func CreateCloudFrontKeyPair(scope constructs.Construct, id string, props *CloudFrontKeyPairProps) *CloudFrontKeyPair {
	// Create IAM role for the key generation Lambda
	lambdaRole := awsiam.NewRole(scope, jsii.String(id+"Role"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
	})

	// Add basic Lambda execution policy
	lambdaRole.AddManagedPolicy(awsiam.ManagedPolicy_FromAwsManagedPolicyName(
		jsii.String("service-role/AWSLambdaBasicExecutionRole"),
	))

	// Add Secrets Manager write permissions
	lambdaRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("secretsmanager:CreateSecret"),
			jsii.String("secretsmanager:PutSecretValue"),
			jsii.String("secretsmanager:UpdateSecret"),
			jsii.String("secretsmanager:DescribeSecret"),
		},
		Resources: &[]*string{
			jsii.String(fmt.Sprintf("arn:aws:secretsmanager:*:*:secret:%s*", props.SecretName)),
		},
	}))

	// Path to the compiled binary
	// CDK app runs from infra/cdk directory (see Makefile: cd infra/cdk && cdk deploy)
	// Asset paths are resolved relative to the working directory where cdk runs
	// Binary is at <repo_root>/bin/cloudfront-keygen, cdk runs from <repo_root>/infra/cdk
	// Therefore: infra/cdk -> (up) -> infra -> (up) -> repo_root -> (down) -> bin
	binaryPath := filepath.Join("..", "..", "bin", "cloudfront-keygen")

	// Create Lambda function from pre-built Go binary
	keyGenFunction := awslambda.NewFunction(scope, jsii.String(id+"Function"), &awslambda.FunctionProps{
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		Handler:      jsii.String("bootstrap"),
		Code:         awslambda.Code_FromAsset(jsii.String(binaryPath), nil),
		Role:         lambdaRole,
		Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
		MemorySize:   jsii.Number(256),
		Description:  jsii.String("Generates CloudFront RSA key pairs at deployment time"),
	})

	// Create Custom Resource using the Lambda function directly
	customResource := awscdk.NewCustomResource(scope, jsii.String(id+"Resource"), &awscdk.CustomResourceProps{
		ServiceToken: keyGenFunction.FunctionArn(),
		Properties: &map[string]interface{}{
			"SecretName": props.SecretName,
		},
		// Ensure resource is replaced if secret name changes
		ResourceType: jsii.String("Custom::CloudFrontKeyPair"),
	})

	// Grant the Lambda permission to be invoked by CloudFormation
	keyGenFunction.GrantInvoke(awsiam.NewServicePrincipal(jsii.String("cloudformation.amazonaws.com"), nil))

	// Reference the secret that will be created by the Lambda
	secret := awssecretsmanager.Secret_FromSecretNameV2(
		scope,
		jsii.String(id+"Secret"),
		jsii.String(props.SecretName),
	)

	// Add dependency to ensure secret is created before it's referenced
	secret.Node().AddDependency(customResource)

	// Output the public key for manual CloudFront configuration
	publicKeyOutput := awscdk.NewCfnOutput(scope, jsii.String(id+"PublicKey"), &awscdk.CfnOutputProps{
		Value:       customResource.GetAttString(jsii.String("PublicKey")),
		Description: jsii.String("CloudFront public key (PEM) - upload to CloudFront Key Management"),
		ExportName:  jsii.String(fmt.Sprintf("lesser-%s-cloudfront-public-key", props.Environment)),
	})

	return &CloudFrontKeyPair{
		Secret:          secret,
		PublicKeyOutput: publicKeyOutput,
		Function:        keyGenFunction,
	}
}
