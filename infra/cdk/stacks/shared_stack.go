package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
)

type SharedStackProps struct {
	awscdk.StackProps
	AppName string
}

type SharedStack struct {
	awscdk.Stack
	EncryptionKey        awskms.IKey
	LambdaEncryptionRole awsiam.Role
	LambdaBasicRole      awsiam.Role
	ActorPrivateKey      awssecretsmanager.Secret
	JWTSecret            awssecretsmanager.Secret
}

func NewSharedStack(scope constructs.Construct, id string, props *SharedStackProps) *SharedStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	sharedStack := &SharedStack{
		Stack: stack,
	}

	// Create KMS key for encryption
	encryptionKey := apptheorycdk.NewAppTheoryKmsKey(stack, jsii.String("LesserEncryptionKey"), &apptheorycdk.AppTheoryKmsKeyProps{
		Description:       jsii.String(fmt.Sprintf("%s encryption key for actor private keys", props.AppName)),
		EnableKeyRotation: jsii.Bool(true),
		AliasName:         jsii.String(fmt.Sprintf("alias/%s", naming.SharedResourceName(props.AppName, "encryption"))),
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
	})
	sharedStack.EncryptionKey = encryptionKey.Key()

	// Create Lambda execution role for functions needing KMS encryption
	lambdaEncryptionRole := apptheorycdk.NewAppTheoryLambdaRole(stack, jsii.String("LambdaEncryptionRole"), &apptheorycdk.AppTheoryLambdaRoleProps{
		RoleName:    jsii.String(naming.SharedResourceName(props.AppName, "lambda-encryption-role")),
		Description: jsii.String("Role for Lambdas requiring KMS encryption for actor private keys"),
		EnableXRay:  jsii.Bool(true),
		ApplicationKmsKeys: &[]awskms.IKey{
			sharedStack.EncryptionKey,
		},
	})
	sharedStack.LambdaEncryptionRole = lambdaEncryptionRole.Role()

	// Create Lambda execution role for functions without encryption needs
	lambdaBasicRole := apptheorycdk.NewAppTheoryLambdaRole(stack, jsii.String("LambdaBasicRole"), &apptheorycdk.AppTheoryLambdaRoleProps{
		RoleName:    jsii.String(naming.SharedResourceName(props.AppName, "lambda-basic-role")),
		Description: jsii.String("Role for Lambdas without encryption requirements"),
		EnableXRay:  jsii.Bool(true),
	})
	sharedStack.LambdaBasicRole = lambdaBasicRole.Role()

	// Attach all application policies to roles using wildcard patterns
	sharedStack.attachApplicationPolicies(props.AppName)

	// Create secret for ActivityPub actor private key
	sharedStack.ActorPrivateKey = awssecretsmanager.NewSecret(stack, jsii.String("ActorPrivateKey"), &awssecretsmanager.SecretProps{
		Description:   jsii.String("ActivityPub actor private key for federation"),
		SecretName:    jsii.String(fmt.Sprintf("%s/actor-private-key", props.AppName)),
		EncryptionKey: sharedStack.EncryptionKey,
		GenerateSecretString: &awssecretsmanager.SecretStringGenerator{
			SecretStringTemplate: jsii.String(`{}`),
			GenerateStringKey:    jsii.String("private_key"),
			PasswordLength:       jsii.Number(2048),
			ExcludeCharacters:    jsii.String(" \t\n"),
		},
	})

	// Create JWT secret (shared across all environments)
	sharedStack.JWTSecret = awssecretsmanager.NewSecret(stack, jsii.String("JWTSecret"), &awssecretsmanager.SecretProps{
		Description:   jsii.String("JWT signing secret for authentication (shared by all environments)"),
		SecretName:    jsii.String(fmt.Sprintf("%s/jwt-secret", props.AppName)),
		EncryptionKey: sharedStack.EncryptionKey,
		GenerateSecretString: &awssecretsmanager.SecretStringGenerator{
			SecretStringTemplate: jsii.String(`{}`),
			GenerateStringKey:    jsii.String("secret"),
			PasswordLength:       jsii.Number(64),
			ExcludeCharacters:    jsii.String(" \t\n\"'\\"),
		},
	})

	// Create outputs
	awscdk.NewCfnOutput(stack, jsii.String("EncryptionKeyArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.EncryptionKey.KeyArn(),
		Description: jsii.String("KMS encryption key ARN"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("ActorPrivateKeyArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.ActorPrivateKey.SecretArn(),
		Description: jsii.String("Actor private key secret ARN"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("JWTSecretArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.JWTSecret.SecretArn(),
		Description: jsii.String("JWT secret ARN"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("LambdaEncryptionRoleArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.LambdaEncryptionRole.RoleArn(),
		Description: jsii.String("Lambda encryption role ARN"),
	})

	awscdk.NewCfnOutput(stack, jsii.String("LambdaBasicRoleArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.LambdaBasicRole.RoleArn(),
		Description: jsii.String("Lambda basic role ARN"),
	})

	// Write all shared resource ARNs to SSM Parameter Store for cross-stack reference
	sharedStack.publishToSSM(props.AppName)

	return sharedStack
}

func (s *SharedStack) attachApplicationPolicies(appName string) {
	// Attach all application policies to both roles using wildcard ARN patterns
	// This avoids circular dependencies with environment-specific stacks
	roles := []awsiam.Role{s.LambdaEncryptionRole, s.LambdaBasicRole}

	for _, role := range roles {
		// DynamoDB access - wildcard pattern for all lesser tables
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("dynamodb:GetItem"),
				jsii.String("dynamodb:PutItem"),
				jsii.String("dynamodb:UpdateItem"),
				jsii.String("dynamodb:DeleteItem"),
				jsii.String("dynamodb:Query"),
				jsii.String("dynamodb:Scan"),
				jsii.String("dynamodb:BatchGetItem"),
				jsii.String("dynamodb:BatchWriteItem"),
				jsii.String("dynamodb:DescribeStream"),
				jsii.String("dynamodb:GetRecords"),
				jsii.String("dynamodb:GetShardIterator"),
				jsii.String("dynamodb:ListStreams"),
			},
			Resources: &[]*string{
				jsii.String(fmt.Sprintf("arn:aws:dynamodb:*:*:table/%s-*", appName)),
				jsii.String(fmt.Sprintf("arn:aws:dynamodb:*:*:table/%s-*/index/*", appName)),
				jsii.String(fmt.Sprintf("arn:aws:dynamodb:*:*:table/%s-*/stream/*", appName)),
			},
		}))

		// S3 access - wildcard pattern for all lesser buckets
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("s3:GetObject"),
				jsii.String("s3:PutObject"),
				jsii.String("s3:DeleteObject"),
				jsii.String("s3:PutObjectAcl"),
			},
			Resources: &[]*string{
				jsii.String(fmt.Sprintf("arn:aws:s3:::%s-*/*", appName)),
			},
		}))

		// SQS access - wildcard pattern for all lesser queues
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("sqs:SendMessage"),
				jsii.String("sqs:ReceiveMessage"),
				jsii.String("sqs:DeleteMessage"),
				jsii.String("sqs:GetQueueAttributes"),
				jsii.String("sqs:ChangeMessageVisibility"),
				jsii.String("sqs:GetQueueUrl"),
			},
			Resources: &[]*string{
				jsii.String(fmt.Sprintf("arn:aws:sqs:*:*:%s-*", appName)),
			},
		}))

		// Secrets Manager access
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("secretsmanager:GetSecretValue"),
				jsii.String("secretsmanager:DescribeSecret"),
			},
			Resources: &[]*string{
				jsii.String(fmt.Sprintf("arn:aws:secretsmanager:*:*:secret:%s/*", appName)),
			},
		}))

		// KMS decrypt for application secrets encrypted with the shared key.
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("kms:Decrypt"),
				jsii.String("kms:DescribeKey"),
			},
			Resources: &[]*string{
				s.EncryptionKey.KeyArn(),
			},
		}))

		// CloudWatch Logs
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("cloudwatch:PutMetricData"),
			},
			Resources: &[]*string{jsii.String("*")},
			Conditions: &map[string]interface{}{
				"StringLike": map[string]interface{}{
					"cloudwatch:namespace": []string{
						"Lesser/*",
						"lesser/*",
					},
				},
			},
		}))

		// Bedrock AI
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("bedrock:InvokeModel"),
				jsii.String("bedrock:InvokeModelWithResponseStream"),
				jsii.String("bedrock:CreateModelCustomizationJob"),
				jsii.String("bedrock:GetModelCustomizationJob"),
				jsii.String("bedrock:ListModelCustomizationJobs"),
				jsii.String("bedrock:StopModelCustomizationJob"),
				jsii.String("bedrock:GetFoundationModel"),
				jsii.String("bedrock:ListFoundationModels"),
			},
			Resources: &[]*string{
				jsii.String("arn:aws:bedrock:*::foundation-model/*"),
				jsii.String("arn:aws:bedrock:*:*:model-customization-job/*"),
				jsii.String("arn:aws:bedrock:*:*:custom-model/*"),
			},
		}))

		// Comprehend
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("comprehend:DetectDominantLanguage"),
				jsii.String("comprehend:DetectEntities"),
				jsii.String("comprehend:DetectKeyPhrases"),
				jsii.String("comprehend:DetectSentiment"),
			},
			Resources: &[]*string{jsii.String("*")},
		}))
	}
}

func (s *SharedStack) publishToSSM(appName string) {
	// Write shared resource ARNs to SSM Parameter Store
	// Use well-known naming convention: /lesser/shared/{resource-type}/{resource-name}
	paramPrefix := fmt.Sprintf("/%s/shared", appName)

	// KMS Key ARN
	awsssm.NewStringParameter(s.Stack, jsii.String("KMSKeyArnParam"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("%s/kms/encryption-key-arn", paramPrefix)),
		StringValue:   s.EncryptionKey.KeyArn(),
		Description:   jsii.String("KMS encryption key ARN for actor private keys"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})

	// IAM Role ARNs
	awsssm.NewStringParameter(s.Stack, jsii.String("EncryptionRoleArnParam"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("%s/iam/lambda-encryption-role-arn", paramPrefix)),
		StringValue:   s.LambdaEncryptionRole.RoleArn(),
		Description:   jsii.String("Lambda encryption role ARN"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})

	awsssm.NewStringParameter(s.Stack, jsii.String("BasicRoleArnParam"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("%s/iam/lambda-basic-role-arn", paramPrefix)),
		StringValue:   s.LambdaBasicRole.RoleArn(),
		Description:   jsii.String("Lambda basic role ARN"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})

	// Secret ARNs
	awsssm.NewStringParameter(s.Stack, jsii.String("JWTSecretArnParam"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("%s/secrets/jwt-secret-arn", paramPrefix)),
		StringValue:   s.JWTSecret.SecretArn(),
		Description:   jsii.String("JWT secret ARN"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})

	awsssm.NewStringParameter(s.Stack, jsii.String("ActorPrivateKeyArnParam"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("%s/secrets/actor-private-key-arn", paramPrefix)),
		StringValue:   s.ActorPrivateKey.SecretArn(),
		Description:   jsii.String("Actor private key secret ARN"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})
}
