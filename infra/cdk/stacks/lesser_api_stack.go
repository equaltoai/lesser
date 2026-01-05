package stacks

import (
	localconstructs "cdk/constructs"
	"cdk/inventory"
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type LesserApiStackProps struct {
	awscdk.StackProps
	Environment      string
	Domain           string
	Config           map[string]interface{} // Environment-specific configuration
	HostedZoneDomain string
	HostedZoneId     string
	CloudFrontDomain string
	AppName          string // Used for SSM lookup + deterministic resource naming
	AccountID        string // Used for globally-unique names (e.g. S3)
	Region           string // Used for globally-unique names (e.g. S3)
}

type LesserApiStack struct {
	awscdk.Stack
	MainTable              awsdynamodb.Table
	RateLimitTable         awsdynamodb.Table
	StreamEventsTable      awsdynamodb.Table
	MediaBucket            awss3.Bucket
	StreamingBucket        awss3.Bucket
	TrainingBucket         awss3.Bucket
	MediaDistribution      awscloudfront.Distribution
	FrontendDistribution   awscloudfront.Distribution
	ClientBucket           awss3.Bucket
	AuthUIBucket           awss3.Bucket
	Queues                 map[string]localconstructs.QueuePair
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
	WebSocketCertificate   awscertificatemanager.ICertificate
	CloudFrontKeyPairID    string
	CloudFrontKeyGroupID   string
	LambdaEncryptionRole   awsiam.IRole
	LambdaBasicRole        awsiam.IRole
	AppName                string
	Domain                 string
	AccountID              string
	Region                 string
}

func NewLesserApiStack(scope constructs.Construct, id string, props *LesserApiStackProps) *LesserApiStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	apiStack := &LesserApiStack{
		Stack:            stack,
		Environment:      props.Environment,
		Configuration:    props.Config,
		CloudFrontDomain: props.CloudFrontDomain,
		AppName:          props.AppName,
		Domain:           props.Domain,
		AccountID:        props.AccountID,
		Region:           props.Region,
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

	apiStack.createStageCertificates(props.Domain)
	apiStack.createClientInfrastructure(props.Domain)

	// Create shared resources
	apiStack.createSharedResources()

	// Create S3 and CloudFront (Phase 6.6)
	apiStack.createMediaInfrastructure(props.Domain)

	// Create media streaming and ML infrastructure (Phase 2.2/2.3)
	apiStack.createStreamingAndMLInfrastructure()

	// Create Lambda functions
	apiStack.createLambdaFunctions()

	// Create SQS queues (inventory-driven)
	apiStack.Queues = apiStack.createSQSQueues()
	localconstructs.ApplyQueueEnvironmentVariables(apiStack.Functions, apiStack.Queues)

	// Create API Gateway
	apiStack.createAPIGateway(props.Domain)

	// Create stream processors
	apiStack.createStreamProcessors()

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

func (s *LesserApiStack) createStageCertificates(stageDomain string) {
	if stageDomain == "" || s.HostedZone == nil {
		return
	}

	wildcard := fmt.Sprintf("*.%s", stageDomain)
	validation := awscertificatemanager.CertificateValidation_FromDns(s.HostedZone)

	regionalCert := awscertificatemanager.NewCertificate(s.Stack, jsii.String("RegionalStageCertificate"), &awscertificatemanager.CertificateProps{
		DomainName:              jsii.String(stageDomain),
		SubjectAlternativeNames: &[]*string{jsii.String(wildcard)},
		Validation:              validation,
	})

	// CloudFront certificates must be provisioned in us-east-1.
	//nolint:staticcheck // Required for CloudFront cross-region support.
	cloudFrontCert := awscertificatemanager.NewDnsValidatedCertificate(s.Stack, jsii.String("CloudFrontStageCertificate"), &awscertificatemanager.DnsValidatedCertificateProps{
		DomainName:              jsii.String(stageDomain),
		HostedZone:              s.HostedZone,
		Region:                  jsii.String("us-east-1"),
		SubjectAlternativeNames: &[]*string{jsii.String(wildcard)},
	})

	s.APICertificate = regionalCert
	s.WebSocketCertificate = regionalCert
	s.CDNCertificate = cloudFrontCert
}

func (s *LesserApiStack) createClientInfrastructure(domain string) {
	if domain == "" || s.HostedZone == nil || s.CDNCertificate == nil {
		return
	}

	isProd := naming.IsLiveEnvironment(s.Environment)
	stage := naming.StageForEnvironment(s.Environment)

	apiOrigin := fmt.Sprintf("api.%s", domain)
	clientBucket := naming.S3BucketName(s.AppName, stage, "client", s.AccountID, s.Region)
	authBucket := naming.S3BucketName(s.AppName, stage, "auth-ui", s.AccountID, s.Region)

	frontend := liftcdk.NewPathRoutedFrontendDistribution(s.Stack, jsii.String("ClientFrontend"), &liftcdk.PathRoutedFrontendDistributionProps{
		HostedZone:          s.HostedZone,
		Certificate:         s.CDNCertificate,
		DomainName:          jsii.String(domain),
		ApiOriginDomainName: jsii.String(apiOrigin),
		AppName:             jsii.String(s.AppName),
		Stage:               jsii.String(string(stage)),
		ClientBucketName:    jsii.String(clientBucket),
		AuthBucketName:      jsii.String(authBucket),
		StaticResponseHeadersPolicy: localconstructs.NewFrontendStaticResponseHeadersPolicy(
			s.Stack,
			jsii.String(domain),
		),
		AuthSinglePageApp:   jsii.Bool(false),
		RemovalPolicy:       getRemovalPolicy(isProd),
		AutoDeleteObjects:   jsii.Bool(!isProd),
		PriceClass:          awscloudfront.PriceClass_PRICE_CLASS_100,
		HttpVersion:         awscloudfront.HttpVersion_HTTP2,
	})

	s.FrontendDistribution = frontend.Distribution
	s.ClientBucket = frontend.ClientBucket
	s.AuthUIBucket = frontend.AuthBucket
}

func (s *LesserApiStack) createSharedResources() {
	isProd := naming.IsLiveEnvironment(s.Environment)

	// Create main DynamoDB table with streams
	mainTableName := naming.ResourceNameWithApp(s.AppName, "main-table", s.Environment)
	mainTable := liftcdk.NewLiftTable(s.Stack, jsii.String("LesserTable"), &liftcdk.LiftTableProps{
		TableName:                 jsii.String(mainTableName),
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		EnableStreams:             jsii.Bool(true),
		StreamViewType:            awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
		TimeToLiveAttribute:       jsii.String("ttl"),
		EnablePointInTimeRecovery: jsii.Bool(isProd),
		DeletionProtection:        jsii.Bool(isProd),
		RemovalPolicy:             getRemovalPolicy(isProd),
	})
	s.MainTable = mainTable.Table

	// Stream event log table for Mastodon-compatible SSE endpoints.
	// This table is polled by the SSE Lambda during response streaming invocations.
	streamEventsTable := liftcdk.NewLiftTable(s.Stack, jsii.String("StreamEventsTable"), &liftcdk.LiftTableProps{
		TableName:                 jsii.String(naming.ResourceNameWithApp(s.AppName, "stream-events-table", s.Environment)),
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		TimeToLiveAttribute:       jsii.String("ttl"),
		EnablePointInTimeRecovery: jsii.Bool(isProd),
		DeletionProtection:        jsii.Bool(isProd),
		RemovalPolicy:             getRemovalPolicy(isProd),
	})
	s.StreamEventsTable = streamEventsTable.Table

	// Create rate limit table for Lift's limited library
	// The limited library uses its own table structure for rate limiting
	rateLimitTable := liftcdk.NewLiftTable(s.Stack, jsii.String("RateLimitTable"), &liftcdk.LiftTableProps{
		TableName:                 jsii.String(naming.ResourceNameWithApp(s.AppName, "rate-limits-table", s.Environment)),
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		TimeToLiveAttribute:       jsii.String("ExpiresAt"),
		EnablePointInTimeRecovery: jsii.Bool(isProd),
		DeletionProtection:        jsii.Bool(false),             // Rate limit data is transient
		RemovalPolicy:             awscdk.RemovalPolicy_DESTROY, // Can be recreated
	})
	s.RateLimitTable = rateLimitTable.Table

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
}

func (s *LesserApiStack) createMediaInfrastructure(domain string) {
	isProd := naming.IsLiveEnvironment(s.Environment)
	stage := naming.StageForEnvironment(s.Environment)

	mediaDomain := s.CloudFrontDomain
	if mediaDomain == "" {
		mediaDomain = fmt.Sprintf("media.%s", domain)
	}

	if s.HostedZone == nil {
		panic("Media infrastructure requires HostedZone")
	}

	mediaCDN := liftcdk.NewMediaCDN(s.Stack, jsii.String("MediaCDN"), &liftcdk.MediaCDNProps{
		HostedZone:        s.HostedZone,
		Certificate:       s.CDNCertificate,
		DomainName:        jsii.String(mediaDomain),
		AppName:           jsii.String(s.AppName),
		Stage:             jsii.String(string(stage)),
		BucketName:        jsii.String(naming.S3BucketName(s.AppName, stage, "media", s.AccountID, s.Region)),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
		PriceClass:        awscloudfront.PriceClass_PRICE_CLASS_100, // US and Europe only for cost optimization
		HttpVersion:       awscloudfront.HttpVersion_HTTP2,
		EnableIpv6:        jsii.Bool(true),
	})

	s.MediaBucket = mediaCDN.Bucket
	s.MediaDistribution = mediaCDN.Distribution

	// Enhanced CORS configuration for media uploads and reads
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
}

func (s *LesserApiStack) createStreamingAndMLInfrastructure() {
	isProd := naming.IsLiveEnvironment(s.Environment)
	stage := naming.StageForEnvironment(s.Environment)

	// Create S3 bucket for transcoded streaming outputs (HLS/DASH segments + manifests)
	s.StreamingBucket = awss3.NewBucket(s.Stack, jsii.String("StreamingBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(naming.S3BucketName(s.AppName, stage, "streaming", s.AccountID, s.Region)),
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
		BucketName:        jsii.String(naming.S3BucketName(s.AppName, stage, "training", s.AccountID, s.Region)),
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

	s.ModelMetadataTableName = naming.ResourceNameWithApp(s.AppName, "main-table", s.Environment)

	// Create outputs for integration
	awscdk.NewCfnOutput(s.Stack, jsii.String("StreamingBucketName"), &awscdk.CfnOutputProps{
		Value:       s.StreamingBucket.BucketName(),
		Description: jsii.String("S3 bucket for streaming outputs"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("TrainingBucketName"), &awscdk.CfnOutputProps{
		Value:       s.TrainingBucket.BucketName(),
		Description: jsii.String("S3 bucket for ML training datasets"),
	})

	awscdk.NewCfnOutput(s.Stack, jsii.String("MediaConvertRoleArn"), &awscdk.CfnOutputProps{
		Value:       s.MediaConvertRole.RoleArn(),
		Description: jsii.String("IAM role ARN for MediaConvert"),
	})

	if s.CloudFrontKeyGroupID != "" {
		awscdk.NewCfnOutput(s.Stack, jsii.String("CloudFrontKeyGroupId"), &awscdk.CfnOutputProps{
			Value:       jsii.String(s.CloudFrontKeyGroupID),
			Description: jsii.String("CloudFront key group ID managed by CDK"),
		})
	}

	awscdk.NewCfnOutput(s.Stack, jsii.String("ModelMetadataTable"), &awscdk.CfnOutputProps{
		Value:       jsii.String(s.ModelMetadataTableName),
		Description: jsii.String("DynamoDB table for model metadata (using gsi9)"),
	})
}

func (s *LesserApiStack) createSQSQueues() map[string]localconstructs.QueuePair {
	queuePairs := map[string]localconstructs.QueuePair{}
	defaultVisibility := awscdk.Duration_Minutes(jsii.Number(2))
	defaultRetention := awscdk.Duration_Days(jsii.Number(4))
	defaultMaxReceive := jsii.Number(5)

	primaryConsumerByQueue := map[string]string{}
	primaryTriggerByQueue := map[string]inventory.SQSTrigger{}
	for _, spec := range inventory.LambdaInventory.Lambdas {
		for _, trig := range spec.SQSTriggers {
			if trig.ConsumeDeadLetterQueue {
				continue
			}
			if existing, ok := primaryConsumerByQueue[trig.Queue]; ok && existing != spec.Name {
				panic(fmt.Sprintf("queue %s has multiple primary consumers: %s and %s", trig.Queue, existing, spec.Name))
			}
			primaryConsumerByQueue[trig.Queue] = spec.Name
			primaryTriggerByQueue[trig.Queue] = trig
		}
	}

	for _, lambda := range inventory.LambdaInventory.Lambdas {
		for _, trigger := range lambda.SQSTriggers {
			if _, exists := queuePairs[trigger.Queue]; exists {
				continue
			}

			logical := trigger.Queue
			primaryName := naming.ResourceNameWithApp(s.AppName, logical, s.Environment)

			dlqLogical := trigger.DeadLetterQueue
			if dlqLogical == "" {
				dlqLogical = fmt.Sprintf("%s-dlq", logical)
			}
			dlqName := naming.ResourceNameWithApp(s.AppName, dlqLogical, s.Environment)

			anchorName := primaryConsumerByQueue[logical]
			if anchorName == "" {
				anchorName = "api"
			}
			consumer := s.Functions.Must(anchorName)

			queueProps := &liftcdk.LiftSQSQueueProps{
				Function:                consumer,
				QueueName:               jsii.String(primaryName),
				VisibilityTimeout:       defaultVisibility,
				MessageRetentionPeriod:  defaultRetention,
				ReceiveMessageWaitTime:  awscdk.Duration_Seconds(jsii.Number(20)),
				EnableDeadLetterQueue:   jsii.Bool(true),
				DeadLetterQueueName:     jsii.String(dlqName),
				MaxReceiveCount:         defaultMaxReceive,
				DLQRetentionPeriod:      awscdk.Duration_Days(jsii.Number(14)),
				EnableEventSource:       jsii.Bool(false),
				ReportBatchItemFailures: jsii.Bool(false),
			}

			if primaryTrig, ok := primaryTriggerByQueue[logical]; ok {
				queueProps.EnableEventSource = jsii.Bool(true)
				queueProps.ReportBatchItemFailures = jsii.Bool(primaryTrig.EnablePartialFailure)
				if primaryTrig.BatchSize > 0 {
					queueProps.BatchSize = jsii.Number(float64(primaryTrig.BatchSize))
				}
				if primaryTrig.MaxBatchingWindowSeconds > 0 {
					queueProps.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(float64(primaryTrig.MaxBatchingWindowSeconds)))
				}
			}

			liftQueue := liftcdk.NewLiftSQSQueue(s.Stack, jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueId(logical))), queueProps)

			if liftQueue.Queue != nil {
				awscdk.Tags_Of(liftQueue.Queue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
				awscdk.Tags_Of(liftQueue.Queue).Add(jsii.String("stage"), jsii.String(string(naming.StageForEnvironment(s.Environment))), nil)
			}
			if liftQueue.DeadLetterQueue != nil {
				awscdk.Tags_Of(liftQueue.DeadLetterQueue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
				awscdk.Tags_Of(liftQueue.DeadLetterQueue).Add(jsii.String("stage"), jsii.String(string(naming.StageForEnvironment(s.Environment))), nil)
			}

			queuePairs[logical] = localconstructs.QueuePair{Primary: liftQueue.Queue, DLQ: liftQueue.DeadLetterQueue}
		}
	}

	// Scheduled publishing queue is part of the canonical env-var contract (Spec 05) even though it is not
	// currently wired as an event source mapping (inventory has no scheduled queue consumer).
	if _, exists := queuePairs["scheduled-queue"]; !exists {
		logical := "scheduled-queue"
		primaryName := naming.ResourceNameWithApp(s.AppName, logical, s.Environment)
		dlqLogical := fmt.Sprintf("%s-dlq", logical)
		dlqName := naming.ResourceNameWithApp(s.AppName, dlqLogical, s.Environment)

		anchor := s.Functions.Must("api")
		liftQueue := liftcdk.NewLiftSQSQueue(s.Stack, jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueId(logical))), &liftcdk.LiftSQSQueueProps{
			Function:                anchor,
			QueueName:               jsii.String(primaryName),
			VisibilityTimeout:       defaultVisibility,
			MessageRetentionPeriod:  defaultRetention,
			ReceiveMessageWaitTime:  awscdk.Duration_Seconds(jsii.Number(20)),
			EnableDeadLetterQueue:   jsii.Bool(true),
			DeadLetterQueueName:     jsii.String(dlqName),
			MaxReceiveCount:         defaultMaxReceive,
			DLQRetentionPeriod:      awscdk.Duration_Days(jsii.Number(14)),
			EnableEventSource:       jsii.Bool(false),
			GrantConsumeMessages:    jsii.Bool(false),
			GrantSendMessages:       jsii.Bool(false),
			ReportBatchItemFailures: jsii.Bool(false),
		})

		if liftQueue.Queue != nil {
			awscdk.Tags_Of(liftQueue.Queue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
			awscdk.Tags_Of(liftQueue.Queue).Add(jsii.String("environment"), jsii.String(s.Environment), nil)
		}
		if liftQueue.DeadLetterQueue != nil {
			awscdk.Tags_Of(liftQueue.DeadLetterQueue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
			awscdk.Tags_Of(liftQueue.DeadLetterQueue).Add(jsii.String("environment"), jsii.String(s.Environment), nil)
		}

		queuePairs[logical] = localconstructs.QueuePair{Primary: liftQueue.Queue, DLQ: liftQueue.DeadLetterQueue}
	}

	if qp, ok := queuePairs["federation-delivery-queue"]; ok {
		queuePairs["federation-queue"] = qp
	}
	if qp, ok := queuePairs["push-delivery-queue"]; ok {
		queuePairs["push-notification-queue"] = qp
	}
	if qp, ok := queuePairs["import-processor-queue"]; ok {
		queuePairs["import-export-queue"] = qp
	}

	return queuePairs
}

func sanitizeQueueId(logical string) string {
	clean := strings.ReplaceAll(logical, "-", "")
	clean = strings.ReplaceAll(clean, "_", "")
	if clean == "" {
		return "Queue"
	}
	return clean
}

func (s *LesserApiStack) createLambdaFunctions() {
	// Use secrets passed from SharedStack (no lookup needed)
	// If not passed via props, fall back to lookup by name for backwards compatibility
	if s.PrivateKey == nil {
		s.PrivateKey = awssecretsmanager.Secret_FromSecretNameV2(s.Stack, jsii.String("PrivateKeySecret"), jsii.String(fmt.Sprintf("%s/actor-private-key", s.AppName)))
	}
	if s.JwtSecret == nil {
		s.JwtSecret = awssecretsmanager.Secret_FromSecretNameV2(s.Stack, jsii.String("JwtSecret"), jsii.String(fmt.Sprintf("%s/jwt-secret", s.AppName)))
	}

	s.Functions = localconstructs.CreateLambdaFunctions(s.Stack, &localconstructs.LambdaFunctionsProps{
		AppName:             s.AppName,
		Environment:         s.Environment,
		Domain:              s.Domain,
		Table:               s.MainTable,
		RateLimitTable:      s.RateLimitTable,
		StreamEventsTable:   s.StreamEventsTable,
		MediaBucket:         s.MediaBucket,
		StreamingBucket:     s.StreamingBucket,
		TrainingBucket:      s.TrainingBucket,
		Queues:              s.Queues,
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
		AppName:              s.AppName,
		Environment:          s.Environment,
		Domain:               domain,
		Certificate:          s.APICertificate,
		WebSocketCertificate: s.WebSocketCertificate,
		Functions:            s.Functions,
		HostedZone:           s.HostedZone,
	})

	apis := []awsapigatewayv2.WebSocketApi{s.API.WebSocketApi}
	if s.API.GraphQLWebSocketApi != nil {
		apis = append(apis, s.API.GraphQLWebSocketApi)
	}
	attachWebSocketManageConnectionsPolicy(
		s.Stack,
		s.AppName,
		s.Environment,
		[]awsiam.IRole{s.LambdaBasicRole, s.LambdaEncryptionRole},
		apis,
	)

	// Output API URLs
	awscdk.NewCfnOutput(s.Stack, jsii.String("RestApiUrl"), &awscdk.CfnOutputProps{
		Value:       s.API.RestApi.GetUrl(),
		Description: jsii.String("REST API Gateway URL"),
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
		})
	}
}

func (s *LesserApiStack) createStreamProcessors() {
	// Inventory-driven wiring for streams and SQS (requires queues/table/functions)
	localconstructs.CreateStreamProcessors(s.Stack, &localconstructs.StreamProcessorsProps{
		Table:     s.MainTable,
		Queues:    s.Queues,
		Functions: s.Functions,
	})
}

func (s *LesserApiStack) setupSecurity() {
	// Enhanced security setup (Phase 6.7) - comprehensive IAM policies are
	// now integrated into Lambda functions via security constructs
	// Policies should remain aligned with required service access:
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

	if qp, ok := s.Queues["federation-delivery-queue"]; ok {
		awscdk.NewCfnOutput(s.Stack, jsii.String("FederationQueueUrl"), &awscdk.CfnOutputProps{
			Value:       qp.Primary.QueueUrl(),
			Description: jsii.String("Federation queue URL"),
		})
		awscdk.NewCfnOutput(s.Stack, jsii.String("FederationDLQUrl"), &awscdk.CfnOutputProps{
			Value:       qp.DLQ.QueueUrl(),
			Description: jsii.String("Federation dead letter queue URL"),
		})
	}
	if qp, ok := s.Queues["import-processor-queue"]; ok {
		awscdk.NewCfnOutput(s.Stack, jsii.String("ImportExportQueueUrl"), &awscdk.CfnOutputProps{
			Value:       qp.Primary.QueueUrl(),
			Description: jsii.String("Import/Export queue URL"),
		})
	}
	if qp, ok := s.Queues["push-delivery-queue"]; ok {
		awscdk.NewCfnOutput(s.Stack, jsii.String("PushNotificationQueueUrl"), &awscdk.CfnOutputProps{
			Value:       qp.Primary.QueueUrl(),
			Description: jsii.String("Push notification queue URL"),
		})
	}

	awscdk.NewCfnOutput(s.Stack, jsii.String("Environment"), &awscdk.CfnOutputProps{
		Value:       jsii.String(s.Environment),
		Description: jsii.String("Deployment environment"),
	})

	if s.FrontendDistribution != nil {
		awscdk.NewCfnOutput(s.Stack, jsii.String("FrontendDistributionId"), &awscdk.CfnOutputProps{
			Value:       s.FrontendDistribution.DistributionId(),
			Description: jsii.String("Stage CloudFront distribution ID for single-domain routing"),
		})
		awscdk.NewCfnOutput(s.Stack, jsii.String("FrontendDistributionDomain"), &awscdk.CfnOutputProps{
			Value:       s.FrontendDistribution.DistributionDomainName(),
			Description: jsii.String("Stage CloudFront distribution domain name"),
		})
	}

	if s.ClientBucket != nil {
		awscdk.NewCfnOutput(s.Stack, jsii.String("ClientBucketName"), &awscdk.CfnOutputProps{
			Value:       s.ClientBucket.BucketName(),
			Description: jsii.String("S3 bucket for the client UI (served under /l/*)"),
		})
	}

	if s.AuthUIBucket != nil {
		awscdk.NewCfnOutput(s.Stack, jsii.String("AuthUIBucketName"), &awscdk.CfnOutputProps{
			Value:       s.AuthUIBucket.BucketName(),
			Description: jsii.String("S3 bucket for the auth UI (served under /auth/*)"),
		})
	}
}

func loadEnvironmentConfig(environment string) map[string]interface{} {
	// Default environment tuning used by the CDK app.
	// Note: `infra/cdk/config/` contains reference templates and is not loaded at deploy time.
	config := map[string]interface{}{
		"logLevel":   "INFO",
		"memorySize": 3008.0, // ARM64 Lambda optimized default
		"timeout":    30.0,
		"features": map[string]interface{}{
			"enableMonitoring": true,
		},
	}

	// Environment-specific overrides
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
