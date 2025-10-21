package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscognito"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"

	"github.com/pay-theory/lift/pkg/cdk/patterns"
)

// MultiTenantSaaSStackProps defines properties for a multi-tenant SaaS stack
type MultiTenantSaaSStackProps struct {
	awscdk.StackProps
	// Application name
	AppName string
	// Path to compiled Lambda binary
	CodePath string
	// Custom domain for API
	DomainName string
	// Certificate ARN for custom domain
	CertificateArn string
	// Enable file storage
	EnableFileStorage bool
	// Enable user authentication
	EnableAuth bool
}

// NewMultiTenantSaaSStack creates a complete multi-tenant SaaS stack
func NewMultiTenantSaaSStack(scope constructs.Construct, id string, props *MultiTenantSaaSStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	env := make(map[string]*string)

	// Create Cognito User Pool if auth is enabled
	var userPool awscognito.UserPool
	if props.EnableAuth {
		userPool = awscognito.NewUserPool(stack, jsii.String("UserPool"), &awscognito.UserPoolProps{
			UserPoolName:      jsii.String(props.AppName + "-users"),
			SelfSignUpEnabled: jsii.Bool(true),
			SignInAliases: &awscognito.SignInAliases{
				Email: jsii.Bool(true),
			},
			AutoVerify: &awscognito.AutoVerifiedAttrs{
				Email: jsii.Bool(true),
			},
			PasswordPolicy: &awscognito.PasswordPolicy{
				MinLength:        jsii.Number(8),
				RequireLowercase: jsii.Bool(true),
				RequireUppercase: jsii.Bool(true),
				RequireDigits:    jsii.Bool(true),
				RequireSymbols:   jsii.Bool(true),
			},
			AccountRecovery: awscognito.AccountRecovery_EMAIL_ONLY,
			RemovalPolicy:   awscdk.RemovalPolicy_RETAIN,
		})

		// Create app client
		client := userPool.AddClient(jsii.String("AppClient"), &awscognito.UserPoolClientOptions{
			AuthFlows: &awscognito.AuthFlow{
				UserPassword: jsii.Bool(true),
				UserSrp:      jsii.Bool(true),
			},
			GenerateSecret: jsii.Bool(false),
		})

		// Add auth environment variables
		env["COGNITO_USER_POOL_ID"] = userPool.UserPoolId()
		env["COGNITO_CLIENT_ID"] = client.UserPoolClientId()
	}

	// Create S3 bucket for file storage if enabled
	var storageBucket awss3.Bucket
	if props.EnableFileStorage {
		storageBucket = awss3.NewBucket(stack, jsii.String("Storage"), &awss3.BucketProps{
			BucketName:        jsii.String(props.AppName + "-storage"),
			Versioned:         jsii.Bool(true),
			Encryption:        awss3.BucketEncryption_S3_MANAGED,
			BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
			RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
			LifecycleRules: &[]*awss3.LifecycleRule{
				{
					Id:                          jsii.String("delete-old-versions"),
					NoncurrentVersionExpiration: awscdk.Duration_Days(jsii.Number(90)),
					Enabled:                     jsii.Bool(true),
				},
			},
		})

		env["STORAGE_BUCKET"] = storageBucket.BucketName()
	}

	// Create the main application
	appProps := &patterns.LiftAppProps{
		AppName:             jsii.String(props.AppName),
		CodeAssetPath:       jsii.String(props.CodePath),
		EnableMultiTenant:   jsii.Bool(true),
		EnableDatabase:      jsii.Bool(true),
		EnableRateLimiting:  jsii.Bool(true),
		EnableAccessLogging: jsii.Bool(true),
		Environment:         &env,
		MemorySize:          jsii.Number(1024),
		Timeout:             jsii.Number(300), // 5 minutes in seconds
	}

	// Only set domain name if provided
	if props.DomainName != "" {
		appProps.DomainName = jsii.String(props.DomainName)
		appProps.CertificateArn = jsii.String(props.CertificateArn)
	}

	app := patterns.NewLiftApp(stack, jsii.String("App"), appProps)

	// Grant permissions for file storage
	if storageBucket != nil {
		storageBucket.GrantReadWrite(app.Function.Function, nil)
	}

	// Create outputs
	awscdk.NewCfnOutput(stack, jsii.String("ApiEndpoint"), &awscdk.CfnOutputProps{
		Value:       app.API.HttpAPI.ApiEndpoint(),
		Description: jsii.String("API Gateway endpoint"),
		ExportName:  jsii.String(props.AppName + "-api"),
	})

	if props.DomainName != "" {
		awscdk.NewCfnOutput(stack, jsii.String("CustomDomain"), &awscdk.CfnOutputProps{
			Value:       jsii.String("https://" + props.DomainName),
			Description: jsii.String("Custom domain URL"),
		})
	}

	if userPool != nil {
		awscdk.NewCfnOutput(stack, jsii.String("UserPoolId"), &awscdk.CfnOutputProps{
			Value:       userPool.UserPoolId(),
			Description: jsii.String("Cognito User Pool ID"),
			ExportName:  jsii.String(props.AppName + "-userpool"),
		})
	}

	if storageBucket != nil {
		awscdk.NewCfnOutput(stack, jsii.String("StorageBucket"), &awscdk.CfnOutputProps{
			Value:       storageBucket.BucketName(),
			Description: jsii.String("S3 storage bucket"),
			ExportName:  jsii.String(props.AppName + "-storage"),
		})
	}

	return stack
}
