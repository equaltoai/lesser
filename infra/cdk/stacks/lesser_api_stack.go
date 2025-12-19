package stacks

import (
	localconstructs "cdk/constructs"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53targets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type LesserApiStackProps struct {
	awscdk.StackProps
	Environment      string
	Domain           string
	Config           map[string]interface{} // Environment-specific configuration
	HostedZoneDomain string
	HostedZoneId     string
	CloudFrontDomain string
	AppName          string // For SSM parameter lookup
}

type LesserApiStack struct {
	awscdk.Stack
	MainTable              awsdynamodb.Table
	RateLimitTable         awsdynamodb.Table
	MediaBucket            awss3.Bucket
	StreamingBucket        awss3.Bucket
	TrainingBucket         awss3.Bucket
	MediaDistribution      awscloudfront.Distribution
	FederationQueue        awssqs.Queue
	FederationDLQ          awssqs.Queue
	PushQueue              awssqs.Queue
	ImportExportQueue      awssqs.Queue
	ImportExportDLQ        awssqs.Queue
	PrivateKey             awssecretsmanager.ISecret
	JwtSecret              awssecretsmanager.ISecret
	Functions              *localconstructs.LambdaFunctions
	API                    *localconstructs.APIGateway
	Environment            string
	Configuration          map[string]interface{}
	MediaConvertRole       awsiam.Role
	ModelMetadataTableName string
	HostedZone             awsroute53.IHostedZone
	CloudFrontDomain       string
	APICertificate         awscertificatemanager.ICertificate
	CDNCertificate         awscertificatemanager.ICertificate
	GraphQLWSCertificate   awscertificatemanager.ICertificate
	StreamingWSCertificate awscertificatemanager.ICertificate
	AuthCertificate        awscertificatemanager.ICertificate
	CloudFrontKeyPairID    string
	CloudFrontKeyGroupID   string
	LambdaEncryptionRole   awsiam.IRole
	LambdaBasicRole        awsiam.IRole
}

func NewLesserApiStack(scope constructs.Construct, id string, props *LesserApiStackProps) *LesserApiStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	apiStack := &LesserApiStack{
		Stack:            stack,
		Environment:      props.Environment,
		Configuration:    props.Config,
		CloudFrontDomain: props.CloudFrontDomain,
	}

	// Import shared resources from SSM Parameter Store
	apiStack.loadSharedResourcesFromSSM(props.AppName)

	if props.Config != nil {
		if val, ok := props.Config["cloudfrontKeyPairId"].(string); ok {
			apiStack.CloudFrontKeyPairID = val
		}
		if val, ok := props.Config["cloudfrontKeyGroupId"].(string); ok {
			apiStack.CloudFrontKeyGroupID = val
		}
	}

	apiStack.initHostedZone(props.HostedZoneDomain, props.HostedZoneId)

	// Create shared resources
	apiStack.createSharedResources()

	// Create S3 and CloudFront (Phase 6.6)
	apiStack.createMediaInfrastructure(props.Domain)

	// Create Auth UI infrastructure (passwordless OAuth)
	apiStack.createAuthUIInfrastructure(props.Domain, apiStack.AuthCertificate)

	// Create media streaming and ML infrastructure (Phase 2.2/2.3)
	apiStack.createStreamingAndMLInfrastructure()

	// Create SQS queues (Phase 6.6)
	apiStack.createSQSQueues()

	// Create Lambda functions
	apiStack.createLambdaFunctions()

	// Create API Gateway
	apiStack.createAPIGateway(props.Domain)

	// Create stream processors
	apiStack.createStreamProcessors()

	// Setup monitoring
	if features, ok := apiStack.Configuration["features"].(map[string]interface{}); ok {
		if enableMonitoring, ok := features["enableMonitoring"].(bool); ok && enableMonitoring {
			apiStack.setupMonitoring()
		}
	}

	// Setup security
	apiStack.setupSecurity()

	// Create outputs
	apiStack.createOutputs()

	return apiStack
}

func (s *LesserApiStack) loadSharedResourcesFromSSM(appName string) {
	// Load shared resource ARNs from SSM Parameter Store
	// Use well-known naming convention: /lesser/shared/{resource-type}/{resource-name}
	paramPrefix := fmt.Sprintf("/%s/shared", appName)

	// Import IAM roles by ARN
	encryptionRoleArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("EncryptionRoleArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/iam/lambda-encryption-role-arn", paramPrefix)),
	)
	s.LambdaEncryptionRole = awsiam.Role_FromRoleArn(
		s.Stack,
		jsii.String("ImportedEncryptionRole"),
		encryptionRoleArnParam.StringValue(),
		nil,
	)

	basicRoleArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("BasicRoleArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/iam/lambda-basic-role-arn", paramPrefix)),
	)
	s.LambdaBasicRole = awsiam.Role_FromRoleArn(
		s.Stack,
		jsii.String("ImportedBasicRole"),
		basicRoleArnParam.StringValue(),
		nil,
	)

	// Import secrets by ARN
	jwtSecretArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("JWTSecretArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/secrets/jwt-secret-arn", paramPrefix)),
	)
	s.JwtSecret = awssecretsmanager.Secret_FromSecretCompleteArn(
		s.Stack,
		jsii.String("ImportedJWTSecret"),
		jwtSecretArnParam.StringValue(),
	)

	actorKeyArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("ActorKeyArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/secrets/actor-private-key-arn", paramPrefix)),
	)
	s.PrivateKey = awssecretsmanager.Secret_FromSecretCompleteArn(
		s.Stack,
		jsii.String("ImportedActorPrivateKey"),
		actorKeyArnParam.StringValue(),
	)

	// Import certificates by ARN
	apiCertArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("APICertArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/certificates/api-cert-arn", paramPrefix)),
	)
	s.APICertificate = awscertificatemanager.Certificate_FromCertificateArn(
		s.Stack,
		jsii.String("ImportedAPICert"),
		apiCertArnParam.StringValue(),
	)

	cdnCertArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("CDNCertArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/certificates/cdn-cert-arn", paramPrefix)),
	)
	s.CDNCertificate = awscertificatemanager.Certificate_FromCertificateArn(
		s.Stack,
		jsii.String("ImportedCDNCert"),
		cdnCertArnParam.StringValue(),
	)

	graphqlWSCertArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("GraphQLWSCertArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/certificates/graphql-ws-cert-arn", paramPrefix)),
	)
	s.GraphQLWSCertificate = awscertificatemanager.Certificate_FromCertificateArn(
		s.Stack,
		jsii.String("ImportedGraphQLWSCert"),
		graphqlWSCertArnParam.StringValue(),
	)

	streamingWSCertArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("StreamingWSCertArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/certificates/streaming-ws-cert-arn", paramPrefix)),
	)
	s.StreamingWSCertificate = awscertificatemanager.Certificate_FromCertificateArn(
		s.Stack,
		jsii.String("ImportedStreamingWSCert"),
		streamingWSCertArnParam.StringValue(),
	)

	authCertArnParam := awsssm.StringParameter_FromStringParameterName(
		s.Stack,
		jsii.String("AuthCertArnParamLookup"),
		jsii.String(fmt.Sprintf("%s/certificates/auth-cert-arn", paramPrefix)),
	)
	s.AuthCertificate = awscertificatemanager.Certificate_FromCertificateArn(
		s.Stack,
		jsii.String("ImportedAuthCert"),
		authCertArnParam.StringValue(),
	)
}

func (s *LesserApiStack) initHostedZone(domain string, zoneId string) {
	if domain == "" {
		return
	}

	if zoneId != "" {
		s.HostedZone = awsroute53.HostedZone_FromHostedZoneAttributes(s.Stack, jsii.String("HostedZone"), &awsroute53.HostedZoneAttributes{
			HostedZoneId: jsii.String(zoneId),
			ZoneName:     jsii.String(domain),
		})
		return
	}

	s.HostedZone = awsroute53.HostedZone_FromLookup(s.Stack, jsii.String("HostedZone"), &awsroute53.HostedZoneProviderProps{
		DomainName: jsii.String(domain),
	})
}

func (s *LesserApiStack) createSharedResources() {
	isProd := s.Environment == "production"

	// Create main DynamoDB table with streams
	s.MainTable = awsdynamodb.NewTable(s.Stack, jsii.String("LesserTable"), &awsdynamodb.TableProps{
		TableName: jsii.String(fmt.Sprintf("lesser-%s", s.Environment)),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("PK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("SK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		BillingMode:         awsdynamodb.BillingMode_PAY_PER_REQUEST,
		Stream:              awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
		TimeToLiveAttribute: jsii.String("ttl"),
		PointInTimeRecoverySpecification: &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(isProd),
		},
		DeletionProtection: jsii.Bool(isProd),
		RemovalPolicy:      getRemovalPolicy(isProd),
	})

	// Create rate limit table for Lift's limited library
	// The limited library uses its own table structure for rate limiting
	s.RateLimitTable = awsdynamodb.NewTable(s.Stack, jsii.String("RateLimitTable"), &awsdynamodb.TableProps{
		TableName: jsii.String(fmt.Sprintf("lesser-rate-limits-%s", s.Environment)),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("PK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("SK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		BillingMode:         awsdynamodb.BillingMode_PAY_PER_REQUEST,
		TimeToLiveAttribute: jsii.String("ExpiresAt"),
		PointInTimeRecoverySpecification: &awsdynamodb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: jsii.Bool(isProd),
		},
		DeletionProtection: jsii.Bool(false),             // Rate limit data is transient
		RemovalPolicy:      awscdk.RemovalPolicy_DESTROY, // Can be recreated
	})

	// Add GSI1-GSI8 (generic pattern-based GSIs)
	// Using camelCase attribute names to match DynamORM conventions (gsi1PK, gsi2PK, etc.)
	for i := 1; i <= 8; i++ {
		s.MainTable.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
			IndexName: jsii.String(fmt.Sprintf("gsi%d", i)),
			PartitionKey: &awsdynamodb.Attribute{
				Name: jsii.String(fmt.Sprintf("gsi%dPK", i)),
				Type: awsdynamodb.AttributeType_STRING,
			},
			SortKey: &awsdynamodb.Attribute{
				Name: jsii.String(fmt.Sprintf("gsi%dSK", i)),
				Type: awsdynamodb.AttributeType_STRING,
			},
			ProjectionType: awsdynamodb.ProjectionType_ALL,
		})
	}

	// Note: GSI2 and GSI3 are now used for relationship domain queries (Phase 2.4)
	// GSI2: FOLLOWER_DOMAIN#{domain} → FOLLOWING#{username} (remote users following local)
	// GSI3: FOLLOWING_DOMAIN#{domain} → FOLLOWER#{username} (local users following remote)
	// The generic loop above already creates these; attributes are defined via dynamorm tags

	// Dedicated index for OAuth client pagination (global listing newest-first)
	s.MainTable.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName: jsii.String("oauth-clients-index"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("oauthClientsPK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("oauthClientsSK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		ProjectionType: awsdynamodb.ProjectionType_ALL,
	})

	// Basic S3 bucket setup - CloudFront integration moved to createMediaInfrastructure
	s.MediaBucket = awss3.NewBucket(s.Stack, jsii.String("MediaBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-media-%s", s.Environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
		// CORS and policies configured in createMediaInfrastructure
	})
}

func (s *LesserApiStack) createMediaInfrastructure(domain string) {
	// Create Origin Access Identity for CloudFront
	oai := awscloudfront.NewOriginAccessIdentity(s.Stack, jsii.String("MediaOAI"), &awscloudfront.OriginAccessIdentityProps{
		Comment: jsii.String("Lesser Media OAI"),
	})

	// Grant read access to the OAI directly on the bucket
	s.MediaBucket.GrantRead(oai, jsii.String("*"))

	// Enhanced CORS configuration matching Pulumi
	s.MediaBucket.AddCorsRule(&awss3.CorsRule{
		AllowedMethods: &[]awss3.HttpMethods{
			awss3.HttpMethods_GET,
			awss3.HttpMethods_PUT,
			awss3.HttpMethods_POST,
			awss3.HttpMethods_HEAD,
		},
		AllowedOrigins: &[]*string{jsii.String("*")},
		AllowedHeaders: &[]*string{jsii.String("*")},
		ExposedHeaders: &[]*string{jsii.String("ETag")},
		MaxAge:         jsii.Number(3000),
	})

	// Apply comprehensive lifecycle policies
	localconstructs.ApplyS3LifecyclePolicies(&localconstructs.S3LifecycleConfig{
		Environment: s.Environment,
		Bucket:      s.MediaBucket,
		BucketType:  "media",
	})

	// Create CloudFront distribution for media.{domain}
	mediaDomain := s.CloudFrontDomain
	if mediaDomain == "" {
		mediaDomain = fmt.Sprintf("media.%s", domain)
	}

	domainNames := func() *[]*string {
		if mediaDomain == "" || s.CDNCertificate == nil {
			return nil
		}
		return &[]*string{jsii.String(mediaDomain)}
	}()

	s.MediaDistribution = awscloudfront.NewDistribution(s.Stack, jsii.String("MediaDistribution"), &awscloudfront.DistributionProps{
		Enabled:                jsii.Bool(true),
		HttpVersion:            awscloudfront.HttpVersion_HTTP2,
		Comment:                jsii.String("Lesser Media CDN"),
		DefaultRootObject:      jsii.String("index.html"),
		DomainNames:            domainNames,
		Certificate:            s.CDNCertificate,
		MinimumProtocolVersion: awscloudfront.SecurityPolicyProtocol_TLS_V1_2_2021,
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin: awscloudfrontorigins.S3BucketOrigin_WithOriginAccessIdentity(s.MediaBucket, &awscloudfrontorigins.S3BucketOriginWithOAIProps{
				OriginAccessIdentity: oai,
			}),
			ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
			AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_GET_HEAD_OPTIONS(),
			CachedMethods:        awscloudfront.CachedMethods_CACHE_GET_HEAD(),
			CachePolicy:          awscloudfront.CachePolicy_CACHING_OPTIMIZED(),
			OriginRequestPolicy:  awscloudfront.OriginRequestPolicy_CORS_S3_ORIGIN(),
			Compress:             jsii.Bool(true),
		},
		PriceClass: awscloudfront.PriceClass_PRICE_CLASS_100, // US and Europe only for cost optimization
	})

	if s.HostedZone != nil && mediaDomain != "" && s.CDNCertificate != nil {
		recordName := relativeRecordName(mediaDomain, s.HostedZone)
		target := awsroute53targets.NewCloudFrontTarget(s.MediaDistribution)

		awsroute53.NewARecord(s.Stack, jsii.String("MediaCdnAliasARecord"), &awsroute53.ARecordProps{
			Zone:       s.HostedZone,
			RecordName: recordName,
			Target:     awsroute53.RecordTarget_FromAlias(target),
		})

		awsroute53.NewAaaaRecord(s.Stack, jsii.String("MediaCdnAliasAAAARecord"), &awsroute53.AaaaRecordProps{
			Zone:       s.HostedZone,
			RecordName: recordName,
			Target:     awsroute53.RecordTarget_FromAlias(target),
		})
	}
}

func relativeRecordName(domain string, zone awsroute53.IHostedZone) *string {
	if zone == nil {
		return jsii.String(domain)
	}

	zoneNamePtr := zone.ZoneName()
	if zoneNamePtr == nil {
		return jsii.String(domain)
	}

	zoneName := strings.TrimSuffix(*zoneNamePtr, ".")
	if domain == "" || domain == zoneName {
		return jsii.String("")
	}

	if strings.HasSuffix(domain, "."+zoneName) {
		trimmed := strings.TrimSuffix(domain, "."+zoneName)
		return jsii.String(trimmed)
	}

	return jsii.String(domain)
}

// createAuthUIInfrastructure creates S3 + CloudFront for the passwordless OAuth UI
func (s *LesserApiStack) createAuthUIInfrastructure(domain string, certificate awscertificatemanager.ICertificate) {
	authUI := localconstructs.NewAuthUI(s.Stack, jsii.String("AuthUI"), &localconstructs.AuthUIProps{
		Environment: s.Environment,
		Domain:      domain,
		Certificate: certificate,
	})

	// Create DNS record for auth.{domain}
	if s.HostedZone != nil && certificate != nil {
		authDomain := fmt.Sprintf("auth.%s", domain)
		recordName := relativeRecordName(authDomain, s.HostedZone)
		target := awsroute53targets.NewCloudFrontTarget(authUI.Distribution)

		awsroute53.NewARecord(s.Stack, jsii.String("AuthUIARecord"), &awsroute53.ARecordProps{
			Zone:       s.HostedZone,
			RecordName: recordName,
			Target:     awsroute53.RecordTarget_FromAlias(target),
		})

		awsroute53.NewAaaaRecord(s.Stack, jsii.String("AuthUIAAAARecord"), &awsroute53.AaaaRecordProps{
			Zone:       s.HostedZone,
			RecordName: recordName,
			Target:     awsroute53.RecordTarget_FromAlias(target),
		})
	}
}

func (s *LesserApiStack) createStreamingAndMLInfrastructure() {
	isProd := s.Environment == "production"

	// Create S3 bucket for transcoded streaming outputs (HLS/DASH segments + manifests)
	s.StreamingBucket = awss3.NewBucket(s.Stack, jsii.String("StreamingBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-streaming-%s", s.Environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
		Versioned:         jsii.Bool(false),
	})

	// Add CORS for streaming bucket
	s.StreamingBucket.AddCorsRule(&awss3.CorsRule{
		AllowedMethods: &[]awss3.HttpMethods{awss3.HttpMethods_GET, awss3.HttpMethods_HEAD},
		AllowedOrigins: &[]*string{jsii.String("*")},
		AllowedHeaders: &[]*string{jsii.String("*")},
		MaxAge:         jsii.Number(3000),
	})

	// Create S3 bucket for ML training datasets
	s.TrainingBucket = awss3.NewBucket(s.Stack, jsii.String("TrainingBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(fmt.Sprintf("lesser-training-%s", s.Environment)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
		Versioned:         jsii.Bool(isProd),
		LifecycleRules: &[]*awss3.LifecycleRule{
			{
				Id:         jsii.String("ArchiveOldTrainingData"),
				Enabled:    jsii.Bool(true),
				Expiration: awscdk.Duration_Days(jsii.Number(90)),
				Transitions: &[]*awss3.Transition{
					{
						StorageClass:    awss3.StorageClass_INTELLIGENT_TIERING(),
						TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
					},
				},
			},
		},
	})

	// Create IAM role for MediaConvert
	s.MediaConvertRole = awsiam.NewRole(s.Stack, jsii.String("MediaConvertRole"), &awsiam.RoleProps{
		AssumedBy:   awsiam.NewServicePrincipal(jsii.String("mediaconvert.amazonaws.com"), nil),
		Description: jsii.String("IAM role for MediaConvert transcoding jobs"),
	})

	// Grant MediaConvert role access to read from source bucket and write to streaming bucket
	s.MediaBucket.GrantRead(s.MediaConvertRole, jsii.String("*"))
	s.StreamingBucket.GrantReadWrite(s.MediaConvertRole, jsii.String("*"))

	// Add CloudWatch Logs permissions for MediaConvert
	s.MediaConvertRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("logs:CreateLogGroup"),
			jsii.String("logs:CreateLogStream"),
			jsii.String("logs:PutLogEvents"),
		},
		Resources: &[]*string{
			jsii.String("arn:aws:logs:*:*:log-group:/aws/mediaconvert/*"),
		},
	}))

	// Add GSI9 to main table for model metadata tracking
	s.MainTable.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName: jsii.String("gsi9"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("gsi9PK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("gsi9SK"),
			Type: awsdynamodb.AttributeType_STRING,
		},
	})

	s.ModelMetadataTableName = fmt.Sprintf("lesser-%s", s.Environment)

	// Create outputs for integration
	awscdk.NewCfnOutput(s.Stack, jsii.String("StreamingBucketName"), &awscdk.CfnOutputProps{
		Value:       s.StreamingBucket.BucketName(),
		Description: jsii.String("S3 bucket for streaming outputs"),
		ExportName:  jsii.String(fmt.Sprintf("lesser-%s-streaming-bucket", s.Environment)),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("TrainingBucketName"), &awscdk.CfnOutputProps{
		Value:       s.TrainingBucket.BucketName(),
		Description: jsii.String("S3 bucket for ML training datasets"),
		ExportName:  jsii.String(fmt.Sprintf("lesser-%s-training-bucket", s.Environment)),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("MediaConvertRoleArn"), &awscdk.CfnOutputProps{
		Value:       s.MediaConvertRole.RoleArn(),
		Description: jsii.String("IAM role ARN for MediaConvert"),
		ExportName:  jsii.String(fmt.Sprintf("lesser-%s-mediaconvert-role", s.Environment)),
	})

	if s.CloudFrontKeyGroupID != "" {
		awscdk.NewCfnOutput(s.Stack, jsii.String("CloudFrontKeyGroupId"), &awscdk.CfnOutputProps{
			Value:       jsii.String(s.CloudFrontKeyGroupID),
			Description: jsii.String("CloudFront key group ID managed by CDK"),
			ExportName:  jsii.String(fmt.Sprintf("lesser-%s-cloudfront-keygroup-id", s.Environment)),
		})
	}

	awscdk.NewCfnOutput(s.Stack, jsii.String("ModelMetadataTable"), &awscdk.CfnOutputProps{
		Value:       jsii.String(s.ModelMetadataTableName),
		Description: jsii.String("DynamoDB table for model metadata (using gsi9)"),
		ExportName:  jsii.String(fmt.Sprintf("lesser-%s-model-metadata-table", s.Environment)),
	})
}

func (s *LesserApiStack) createSQSQueues() {
	// Create federation dead letter queue
	s.FederationDLQ = awssqs.NewQueue(s.Stack, jsii.String("FederationDLQ"), &awssqs.QueueProps{
		QueueName:         jsii.String(fmt.Sprintf("lesser-federation-dlq-%s", s.Environment)),
		RetentionPeriod:   awscdk.Duration_Days(jsii.Number(14)), // 14 days
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(30)),
	})

	// Create federation queue with DLQ redrive policy
	s.FederationQueue = awssqs.NewQueue(s.Stack, jsii.String("FederationQueue"), &awssqs.QueueProps{
		QueueName:              jsii.String(fmt.Sprintf("lesser-federation-queue-%s", s.Environment)),
		VisibilityTimeout:      awscdk.Duration_Minutes(jsii.Number(5)),  // 5 minutes
		RetentionPeriod:        awscdk.Duration_Days(jsii.Number(4)),     // 4 days
		ReceiveMessageWaitTime: awscdk.Duration_Seconds(jsii.Number(20)), // Long polling
		DeadLetterQueue: &awssqs.DeadLetterQueue{
			MaxReceiveCount: jsii.Number(5), // After 5 failed attempts, send to DLQ
			Queue:           s.FederationDLQ,
		},
	})

	// Create push notification queue
	s.PushQueue = awssqs.NewQueue(s.Stack, jsii.String("PushNotificationQueue"), &awssqs.QueueProps{
		QueueName:              jsii.String(fmt.Sprintf("lesser-push-notification-queue-%s", s.Environment)),
		VisibilityTimeout:      awscdk.Duration_Minutes(jsii.Number(1)),  // 1 minute
		RetentionPeriod:        awscdk.Duration_Days(jsii.Number(1)),     // 1 day
		ReceiveMessageWaitTime: awscdk.Duration_Seconds(jsii.Number(20)), // Long polling
	})

	// Create import/export dead letter queue
	s.ImportExportDLQ = awssqs.NewQueue(s.Stack, jsii.String("ImportExportDLQ"), &awssqs.QueueProps{
		QueueName:         jsii.String(fmt.Sprintf("lesser-import-export-dlq-%s", s.Environment)),
		RetentionPeriod:   awscdk.Duration_Days(jsii.Number(14)), // 14 days
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(30)),
	})

	// Create import/export queue with DLQ redrive policy
	s.ImportExportQueue = awssqs.NewQueue(s.Stack, jsii.String("ImportExportQueue"), &awssqs.QueueProps{
		QueueName:              jsii.String(fmt.Sprintf("lesser-import-export-queue-%s", s.Environment)),
		VisibilityTimeout:      awscdk.Duration_Minutes(jsii.Number(15)), // 15 minutes for processing time
		RetentionPeriod:        awscdk.Duration_Days(jsii.Number(7)),     // 7 days
		ReceiveMessageWaitTime: awscdk.Duration_Seconds(jsii.Number(20)), // Long polling
		DeadLetterQueue: &awssqs.DeadLetterQueue{
			MaxReceiveCount: jsii.Number(3), // After 3 failed attempts, send to DLQ
			Queue:           s.ImportExportDLQ,
		},
	})
}

func (s *LesserApiStack) createLambdaFunctions() {
	// Use secrets passed from SharedStack (no lookup needed)
	// If not passed via props, fall back to lookup by name for backwards compatibility
	if s.PrivateKey == nil {
		s.PrivateKey = awssecretsmanager.Secret_FromSecretNameV2(s.Stack, jsii.String("PrivateKeySecret"), jsii.String("lesser/actor-private-key"))
	}
	if s.JwtSecret == nil {
		s.JwtSecret = awssecretsmanager.Secret_FromSecretNameV2(s.Stack, jsii.String("JwtSecret"), jsii.String("lesser/jwt-secret"))
	}

	s.Functions = localconstructs.CreateLambdaFunctions(s.Stack, &localconstructs.LambdaFunctionsProps{
		Environment:         s.Environment,
		Table:               s.MainTable,
		RateLimitTable:      s.RateLimitTable,
		MediaBucket:         s.MediaBucket,
		StreamingBucket:     s.StreamingBucket,
		TrainingBucket:      s.TrainingBucket,
		FederationQueue:     s.FederationQueue,
		FederationDLQ:       s.FederationDLQ,
		PushQueue:           s.PushQueue,
		PrivateKey:          s.PrivateKey,
		JwtSecret:           s.JwtSecret,
		MediaConvertRoleArn: s.MediaConvertRole.RoleArn(),
		ModelMetadataTable:  jsii.String(s.ModelMetadataTableName),
		Config:              s.Configuration,
		EncryptionRole:      s.LambdaEncryptionRole,
		BasicRole:           s.LambdaBasicRole,
	})
}

func (s *LesserApiStack) createAPIGateway(domain string) {
	s.API = localconstructs.CreateAPIGateway(s.Stack, &localconstructs.APIGatewayProps{
		Environment:            s.Environment,
		Domain:                 domain,
		Certificate:            s.APICertificate,
		GraphQLWSCertificate:   s.GraphQLWSCertificate,
		StreamingWSCertificate: s.StreamingWSCertificate,
		Functions:              s.Functions,
		HostedZone:             s.HostedZone,
	})

	// Output API URLs
	awscdk.NewCfnOutput(s.Stack, jsii.String("HttpApiUrl"), &awscdk.CfnOutputProps{
		Value:       s.API.HttpApi.Url(),
		Description: jsii.String("HTTP API Gateway URL"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("WebSocketApiUrl"), &awscdk.CfnOutputProps{
		Value:       s.API.WebSocketApi.ApiEndpoint(),
		Description: jsii.String("WebSocket API Gateway URL"),
	})

	if s.API.GraphQLWebSocketApi != nil {
		awscdk.NewCfnOutput(s.Stack, jsii.String("GraphQLWebSocketApiUrl"), &awscdk.CfnOutputProps{
			Value:       s.API.GraphQLWebSocketApi.ApiEndpoint(),
			Description: jsii.String("GraphQL WebSocket API Gateway URL"),
		})
	}

	if s.CloudFrontKeyPairID != "" {
		awscdk.NewCfnOutput(s.Stack, jsii.String("CloudFrontKeyPairId"), &awscdk.CfnOutputProps{
			Value:       jsii.String(s.CloudFrontKeyPairID),
			Description: jsii.String("CloudFront public key ID used for signed URLs"),
			ExportName:  jsii.String(fmt.Sprintf("lesser-%s-cloudfront-keypair-id", s.Environment)),
		})
	}
}

func (s *LesserApiStack) createStreamProcessors() {
	localconstructs.CreateStreamProcessors(s.Stack, &localconstructs.StreamProcessorsProps{
		Table:     s.MainTable,
		PushQueue: s.PushQueue,
		Functions: s.Functions,
	})
}

func (s *LesserApiStack) setupMonitoring() {
	// Implementation will use monitoring_stack.go
}

func (s *LesserApiStack) setupSecurity() {
	// Enhanced security setup (Phase 6.7) - comprehensive IAM policies are
	// now integrated into Lambda functions via security constructs
	// All policies match Pulumi configuration exactly:
	// - DynamoDB: Full table + GSI + streams access
	// - S3: GetObject, PutObject, DeleteObject, PutObjectAcl
	// - SQS: Full queue operations for federation and push notifications
	// - Bedrock: InvokeModel for amazon.titan-embed-text-v1
	// - KMS: Encrypt/Decrypt with SharedStack key (alias/lesser-encryption)
	// - Comprehend: AI text analysis capabilities
}

func (s *LesserApiStack) createOutputs() {
	awscdk.NewCfnOutput(s.Stack, jsii.String("TableName"), &awscdk.CfnOutputProps{
		Value:       s.MainTable.TableName(),
		Description: jsii.String("DynamoDB table name"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("MediaBucketName"), &awscdk.CfnOutputProps{
		Value:       s.MediaBucket.BucketName(),
		Description: jsii.String("S3 media bucket name"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("MediaDistributionDomain"), &awscdk.CfnOutputProps{
		Value:       s.MediaDistribution.DistributionDomainName(),
		Description: jsii.String("CloudFront distribution domain name for media"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("FederationQueueUrl"), &awscdk.CfnOutputProps{
		Value:       s.FederationQueue.QueueUrl(),
		Description: jsii.String("Federation queue URL"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("ImportExportQueueUrl"), &awscdk.CfnOutputProps{
		Value:       s.ImportExportQueue.QueueUrl(),
		Description: jsii.String("Import/Export queue URL"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("FederationDLQUrl"), &awscdk.CfnOutputProps{
		Value:       s.FederationDLQ.QueueUrl(),
		Description: jsii.String("Federation dead letter queue URL"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("PushNotificationQueueUrl"), &awscdk.CfnOutputProps{
		Value:       s.PushQueue.QueueUrl(),
		Description: jsii.String("Push notification queue URL"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("Environment"), &awscdk.CfnOutputProps{
		Value:       jsii.String(s.Environment),
		Description: jsii.String("Deployment environment"),
	})
}

func loadEnvironmentConfig(environment string) map[string]interface{} {
	// Configuration from Pulumi legacy - uses external domain config
	// These will be overridden by CDK context or environment variables
	config := map[string]interface{}{
		"logLevel":   "INFO",
		"memorySize": 3008.0, // ARM64 Lambda optimized (from Pulumi line 650)
		"timeout":    30.0,
		"features": map[string]interface{}{
			"enableMonitoring": true,
		},
	}

	// Environment-specific overrides (matching Pulumi behavior)
	switch environment {
	case "development":
		config["logLevel"] = "DEBUG"
		config["memorySize"] = 1024.0
	case "staging":
		config["memorySize"] = 1024.0
	case "production":
		config["memorySize"] = 3008.0 // Max memory for production
	}

	return config
}

func getRemovalPolicy(isProd bool) awscdk.RemovalPolicy {
	if isProd {
		return awscdk.RemovalPolicy_RETAIN
	}
	return awscdk.RemovalPolicy_DESTROY
}
