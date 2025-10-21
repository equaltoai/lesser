package constructs

import (
	"fmt"
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

const lambdaDistEnvVar = "LIFT_LAMBDA_DIST_PATH"

// LambdaFunctionConfig defines configuration for creating Lambda functions
type LambdaFunctionConfig struct {
	Environment  map[string]*string // 8 bytes (map)
	Timeout      awscdk.Duration    // 8 bytes (int64)
	FunctionName string             // 16 bytes
	Description  string             // 16 bytes
	Permissions  string             // PermissionRead or PermissionReadWrite - 16 bytes
}

// CreateStandardLambdaFunction creates a Lambda function with common configurations
func CreateStandardLambdaFunction(scope constructs.Construct, id string, bucket awss3.Bucket, encryptionKey awskms.Key, config LambdaFunctionConfig) awslambda.Function {
	// Create IAM role
	role := awsiam.NewRole(scope, jsii.String(id+"Role"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")),
		},
	})

	// Grant permissions
	if config.Permissions == PermissionReadWrite {
		bucket.GrantReadWrite(role, nil)
	} else {
		bucket.GrantRead(role, nil)
	}

	if encryptionKey != nil {
		encryptionKey.GrantEncryptDecrypt(role)
	}

	codePath := resolveLambdaAssetPath()

	return awslambda.NewFunction(scope, jsii.String(id), &awslambda.FunctionProps{
		FunctionName: jsii.String(config.FunctionName),
		Runtime:      awslambda.Runtime_PROVIDED_AL2(),
		Handler:      jsii.String("bootstrap"),
		Code:         awslambda.Code_FromAsset(jsii.String(codePath), nil),
		Role:         role,
		Description:  jsii.String(config.Description),
		Timeout:      config.Timeout,
		Environment:  &config.Environment,
	})
}

func resolveLambdaAssetPath() string {
	if distPath, ok := os.LookupEnv(lambdaDistEnvVar); ok && distPath != "" {
		if _, err := os.Stat(distPath); err == nil {
			return distPath
		}
	}

	const defaultPath = "./dist"
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}

	panic(fmt.Sprintf("lambda asset path not found; set %s or create %s", lambdaDistEnvVar, defaultPath))
}
