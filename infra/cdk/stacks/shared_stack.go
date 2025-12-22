package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type SharedStackProps struct {
	awscdk.StackProps
	AppName        string
	RootDomain     string
	HostedZoneId   string
	HostedZoneName string
	Stages         []string
}

type SharedStack struct {
	awscdk.Stack
	EncryptionKey          awskms.IKey
	LambdaEncryptionRole   awsiam.Role
	LambdaBasicRole        awsiam.Role
	ActorPrivateKey        awssecretsmanager.Secret
	JWTSecret              awssecretsmanager.Secret
	HostedZone             awsroute53.IHostedZone
	APICertificate         awscertificatemanager.Certificate
	CDNCertificate         awscertificatemanager.Certificate
	GraphQLWSCertificate   awscertificatemanager.Certificate
	StreamingWSCertificate awscertificatemanager.Certificate
	AuthCertificate        awscertificatemanager.Certificate
	RootDomain             string
	Stages                 []string
}

func NewSharedStack(scope constructs.Construct, id string, props *SharedStackProps) *SharedStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	sharedStack := &SharedStack{
		Stack:      stack,
		RootDomain: props.RootDomain,
		Stages:     props.Stages,
	}

	sharedStack.initHostedZone(props.HostedZoneName, props.HostedZoneId)

	// Create KMS key for encryption
	encryptionKey := liftcdk.NewLiftKMSKey(stack, jsii.String("LesserEncryptionKey"), &liftcdk.LiftKMSKeyProps{
		Description:       jsii.String(fmt.Sprintf("%s encryption key for actor private keys", props.AppName)),
		EnableKeyRotation: jsii.Bool(true),
		AliasName:         jsii.String(fmt.Sprintf("alias/%s-encryption", props.AppName)),
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
	})
	sharedStack.EncryptionKey = encryptionKey.Key

	// Create Lambda execution role for functions needing KMS encryption
	lambdaEncryptionRole := liftcdk.NewLiftLambdaRole(stack, jsii.String("LambdaEncryptionRole"), &liftcdk.LiftLambdaRoleProps{
		RoleName:    jsii.String(fmt.Sprintf("%s-lambda-encryption-role", props.AppName)),
		Description: jsii.String("Role for Lambdas requiring KMS encryption for actor private keys"),
		KMSKeys:     []awskms.IKey{sharedStack.EncryptionKey},
	})
	sharedStack.LambdaEncryptionRole = lambdaEncryptionRole.Role

	// Create Lambda execution role for functions without encryption needs
	lambdaBasicRole := liftcdk.NewLiftLambdaRole(stack, jsii.String("LambdaBasicRole"), &liftcdk.LiftLambdaRoleProps{
		RoleName:    jsii.String(fmt.Sprintf("%s-lambda-basic-role", props.AppName)),
		Description: jsii.String("Role for Lambdas without encryption requirements"),
	})
	sharedStack.LambdaBasicRole = lambdaBasicRole.Role

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
		ExportName:  jsii.String(fmt.Sprintf("%s-encryption-key-arn", props.AppName)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("ActorPrivateKeyArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.ActorPrivateKey.SecretArn(),
		Description: jsii.String("Actor private key secret ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-actor-private-key-arn", props.AppName)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("JWTSecretArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.JWTSecret.SecretArn(),
		Description: jsii.String("JWT secret ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-jwt-secret-arn", props.AppName)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("LambdaEncryptionRoleArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.LambdaEncryptionRole.RoleArn(),
		Description: jsii.String("Lambda encryption role ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-lambda-encryption-role-arn", props.AppName)),
	})

	awscdk.NewCfnOutput(stack, jsii.String("LambdaBasicRoleArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.LambdaBasicRole.RoleArn(),
		Description: jsii.String("Lambda basic role ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-lambda-basic-role-arn", props.AppName)),
	})

	sharedStack.createCertificates()

	// Write all shared resource ARNs to SSM Parameter Store for cross-stack reference
	sharedStack.publishToSSM(props.AppName, props.Stages)

	if sharedStack.APICertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("ApiCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.APICertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for stage API domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-api-certificate-arn", props.AppName)),
		})
	}

	if sharedStack.CDNCertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("CdnCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.CDNCertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for stage CDN domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-cdn-certificate-arn", props.AppName)),
		})
	}

	if sharedStack.GraphQLWSCertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("GraphQLWSCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.GraphQLWSCertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for GraphQL WebSocket domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-graphql-ws-certificate-arn", props.AppName)),
		})
	}

	if sharedStack.StreamingWSCertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("StreamingWSCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.StreamingWSCertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for streaming WebSocket domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-streaming-ws-certificate-arn", props.AppName)),
		})
	}

	if sharedStack.AuthCertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("AuthCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.AuthCertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for auth UI domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-auth-certificate-arn", props.AppName)),
		})
	}

	return sharedStack
}

func (s *SharedStack) initHostedZone(domain string, hostedZoneId string) {
	if domain == "" && hostedZoneId == "" {
		return
	}

	if hostedZoneId != "" && domain != "" {
		s.HostedZone = awsroute53.HostedZone_FromHostedZoneAttributes(s.Stack, jsii.String("SharedHostedZone"), &awsroute53.HostedZoneAttributes{
			HostedZoneId: jsii.String(hostedZoneId),
			ZoneName:     jsii.String(domain),
		})
		return
	}

	if domain != "" {
		s.HostedZone = awsroute53.HostedZone_FromLookup(s.Stack, jsii.String("SharedHostedZone"), &awsroute53.HostedZoneProviderProps{
			DomainName: jsii.String(domain),
		})
	}
}

func (s *SharedStack) createCertificates() {
	if s.RootDomain == "" || len(s.Stages) == 0 || s.HostedZone == nil {
		return
	}

	stageFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		stageFqdns = append(stageFqdns, jsii.String(fmt.Sprintf("%s.%s", stage, s.RootDomain)))
	}

	validation := awscertificatemanager.CertificateValidation_FromDns(s.HostedZone)

	apiPrimary := stageFqdns[0]
	var apiSans []*string
	if len(stageFqdns) > 1 {
		apiSans = stageFqdns[1:]
	}

	s.APICertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedApiCertificate"), &awscertificatemanager.CertificateProps{
		DomainName:              apiPrimary,
		SubjectAlternativeNames: &apiSans,
		Validation:              validation,
	})

	cdnFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		cdnFqdns = append(cdnFqdns, jsii.String(fmt.Sprintf("cdn.%s.%s", stage, s.RootDomain)))
	}

	cdnPrimary := cdnFqdns[0]
	var cdnSans []*string
	if len(cdnFqdns) > 1 {
		cdnSans = cdnFqdns[1:]
	}

	s.CDNCertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedCdnCertificate"), &awscertificatemanager.CertificateProps{
		DomainName:              cdnPrimary,
		SubjectAlternativeNames: &cdnSans,
		Validation:              validation,
	})

	graphqlWsFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		graphqlWsFqdns = append(graphqlWsFqdns, jsii.String(fmt.Sprintf("graphql-ws.%s.%s", stage, s.RootDomain)))
	}

	if len(graphqlWsFqdns) > 0 {
		graphqlWsPrimary := graphqlWsFqdns[0]
		var graphqlWsSans []*string
		if len(graphqlWsFqdns) > 1 {
			graphqlWsSans = graphqlWsFqdns[1:]
		}

		s.GraphQLWSCertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedGraphQLWsCertificate"), &awscertificatemanager.CertificateProps{
			DomainName:              graphqlWsPrimary,
			SubjectAlternativeNames: &graphqlWsSans,
			Validation:              validation,
		})
	}

	streamWsFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		streamWsFqdns = append(streamWsFqdns, jsii.String(fmt.Sprintf("stream.%s.%s", stage, s.RootDomain)))
	}

	if len(streamWsFqdns) > 0 {
		streamWsPrimary := streamWsFqdns[0]
		var streamWsSans []*string
		if len(streamWsFqdns) > 1 {
			streamWsSans = streamWsFqdns[1:]
		}

		s.StreamingWSCertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedStreamingWsCertificate"), &awscertificatemanager.CertificateProps{
			DomainName:              streamWsPrimary,
			SubjectAlternativeNames: &streamWsSans,
			Validation:              validation,
		})
	}

	// Auth UI certificate (auth.dev.lesser.host, auth.live.lesser.host, etc.)
	authFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		authFqdns = append(authFqdns, jsii.String(fmt.Sprintf("auth.%s.%s", stage, s.RootDomain)))
	}

	if len(authFqdns) > 0 {
		authPrimary := authFqdns[0]
		var authSans []*string
		if len(authFqdns) > 1 {
			authSans = authFqdns[1:]
		}

		s.AuthCertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedAuthCertificate"), &awscertificatemanager.CertificateProps{
			DomainName:              authPrimary,
			SubjectAlternativeNames: &authSans,
			Validation:              validation,
		})
	}
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

		// WebSocket connections
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("execute-api:ManageConnections"),
				jsii.String("execute-api:Invoke"),
			},
			Resources: &[]*string{jsii.String("arn:aws:execute-api:*:*:*/*")},
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

func (s *SharedStack) publishToSSM(appName string, stages []string) {
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

	// Certificate ARNs (if they exist)
	if s.APICertificate != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("APICertArnParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/certificates/api-cert-arn", paramPrefix)),
			StringValue:   s.APICertificate.CertificateArn(),
			Description:   jsii.String("API certificate ARN"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}

	if s.CDNCertificate != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("CDNCertArnParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/certificates/cdn-cert-arn", paramPrefix)),
			StringValue:   s.CDNCertificate.CertificateArn(),
			Description:   jsii.String("CDN certificate ARN"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}

	if s.GraphQLWSCertificate != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("GraphQLWSCertArnParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/certificates/graphql-ws-cert-arn", paramPrefix)),
			StringValue:   s.GraphQLWSCertificate.CertificateArn(),
			Description:   jsii.String("GraphQL WebSocket certificate ARN"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}

	if s.StreamingWSCertificate != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("StreamingWSCertArnParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/certificates/streaming-ws-cert-arn", paramPrefix)),
			StringValue:   s.StreamingWSCertificate.CertificateArn(),
			Description:   jsii.String("Streaming WebSocket certificate ARN"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}

	if s.AuthCertificate != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("AuthCertArnParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/certificates/auth-cert-arn", paramPrefix)),
			StringValue:   s.AuthCertificate.CertificateArn(),
			Description:   jsii.String("Auth UI certificate ARN"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}
}
