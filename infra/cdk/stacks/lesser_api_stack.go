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
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
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

	clientAssetsBucket := awss3.NewBucket(s.Stack, jsii.String("ClientBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(clientBucket),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		EnforceSSL:        jsii.Bool(true),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
	})

	authAssetsBucket := awss3.NewBucket(s.Stack, jsii.String("AuthBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(authBucket),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		EnforceSSL:        jsii.Bool(true),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
	})

	staticPolicy := localconstructs.NewFrontendStaticResponseHeadersPolicy(s.Stack, jsii.String(domain))

	frontend := apptheorycdk.NewAppTheoryPathRoutedFrontend(s.Stack, jsii.String("ClientFrontend"), &apptheorycdk.AppTheoryPathRoutedFrontendProps{
		ApiOriginUrl: jsii.String(apiOrigin),
		// Forward all query strings to the API origin (required for OAuth /oauth/authorize PKCE params).
		// Exclude Host to avoid origin domain mismatch issues.
		ApiOriginRequestPolicy: awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
		Domain: &apptheorycdk.PathRoutedFrontendDomainConfig{
			DomainName:       jsii.String(domain),
			Certificate:      s.CDNCertificate,
			HostedZone:       s.HostedZone,
			CreateAAAARecord: jsii.Bool(true),
		},
		ApiBypassPaths: &[]*apptheorycdk.ApiBypassConfig{
			{PathPattern: jsii.String("/auth/wallet/*")},
		},
		SpaOrigins: &[]*apptheorycdk.SpaOriginConfig{
			{
				Bucket:                  clientAssetsBucket,
				PathPattern:             jsii.String("/l/*"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_SPA,
			},
			{
				Bucket:                  authAssetsBucket,
				PathPattern:             jsii.String("/auth/*"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_NONE,
			},
			// Exact prefix paths (no wildcard) for behavior-scoped CSP parity with Lift.
			{
				Bucket:                  clientAssetsBucket,
				PathPattern:             jsii.String("/l"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_SPA,
			},
			{
				Bucket:                  authAssetsBucket,
				PathPattern:             jsii.String("/auth"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_NONE,
			},
		},
		SpaResponseHeadersPolicy: staticPolicy,
		RemovalPolicy:            getRemovalPolicy(isProd),
		AutoDeleteObjects:        jsii.Bool(!isProd),
		PriceClass:               awscloudfront.PriceClass_PRICE_CLASS_100,
	})

	overridePathRoutedFrontendRewriteFunction(frontend)

	dist := frontend.Distribution()

	s.FrontendDistribution = dist
	s.ClientBucket = clientAssetsBucket
	s.AuthUIBucket = authAssetsBucket
}

func (s *LesserApiStack) bodyEnabled() bool {
	if s.Configuration != nil {
		if v, ok := s.Configuration["bodyEnabled"]; ok && isTruthyConfigValue(v) {
			return true
		}
		// Backwards compatibility: "soulEnabled" historically controlled MCP wiring.
		if v, ok := s.Configuration["soulEnabled"]; ok && isTruthyConfigValue(v) {
			return true
		}
	}
	if isTruthyConfigValue(s.Node().TryGetContext(jsii.String("bodyEnabled"))) {
		return true
	}
	return isTruthyConfigValue(s.Node().TryGetContext(jsii.String("soulEnabled")))
}

func isTruthyConfigValue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		clean := strings.TrimSpace(strings.ToLower(v))
		switch clean {
		case "true", "1", "yes", "y":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func overridePathRoutedFrontendRewriteFunction(frontend apptheorycdk.AppTheoryPathRoutedFrontend) {
	if frontend == nil {
		return
	}

	// The upstream rewrite function doesn't handle exact-prefix paths (e.g. "/l" and "/auth") and doesn't
	// support directory-index semantics for multi-page static sites (e.g. "/auth/login" → "/auth/login/index.html").
	// We override the function code so:
	// - "/l" redirects/rewrites consistently with "/l/" and extensionless routes fall back to "/l/index.html" (SPA)
	// - "/auth" serves "/auth/index.html" and extensionless routes map to "/auth/<path>/index.html" (directory index)
	rewriteFn := frontend.SpaRewriteFunction()
	if rewriteFn == nil {
		return
	}

	raw := rewriteFn.Node().DefaultChild()
	cfnFn, ok := raw.(awscloudfront.CfnFunction)
	if !ok || cfnFn == nil {
		return
	}

	// NOTE: Keep this function CSP-safe: it only rewrites the request URI at CloudFront and does not inject content.
	cfnFn.SetFunctionCode(jsii.String(strings.TrimSpace(`
function handler(event) {
  var request = event.request;
  var uri = request.uri;

  var prefixes = [
    { cleanPrefix: '/auth', prefix: '/auth/', rewriteMode: 'dir', stripPrefixBeforeOrigin: true, indexPath: '/auth/index.html' },
    { cleanPrefix: '/l', prefix: '/l/', rewriteMode: 'spa', stripPrefixBeforeOrigin: true, indexPath: '/l/index.html' }
  ];

  for (var i = 0; i < prefixes.length; i++) {
    var cfg = prefixes[i];

    // Normalize "/prefix" → "/prefix/" so the rest of the logic can treat it uniformly.
    if (uri === cfg.cleanPrefix) {
      uri = cfg.prefix;
    }

    if (uri.indexOf(cfg.prefix) !== 0) {
      continue;
    }

    var uriWithoutPrefix = uri.substring(cfg.prefix.length);
    var lastSlash = uriWithoutPrefix.lastIndexOf('/');
    var lastSegment = lastSlash >= 0 ? uriWithoutPrefix.substring(lastSlash + 1) : uriWithoutPrefix;

    if (cfg.rewriteMode === 'spa') {
      // SPA behavior: any extensionless route falls back to the SPA index.
      if (lastSegment.indexOf('.') === -1) {
        request.uri = cfg.indexPath;
      } else {
        request.uri = uri;
      }
    } else {
      // Directory-index behavior (multi-page static site):
      // - "/auth" or "/auth/" → "/auth/index.html"
      // - "/auth/login" → "/auth/login/index.html"
      // - "/auth/login/" → "/auth/login/index.html"
      if (uriWithoutPrefix === '') {
        request.uri = cfg.indexPath;
      } else if (lastSegment === '') {
        request.uri = uri + 'index.html';
      } else if (lastSegment.indexOf('.') === -1) {
        request.uri = uri + '/index.html';
      } else {
        request.uri = uri;
      }
    }

    if (cfg.stripPrefixBeforeOrigin) {
      var cleanPrefixWithSlash = cfg.cleanPrefix + '/';
      if (request.uri.indexOf(cleanPrefixWithSlash) === 0) {
        request.uri = request.uri.substring(cfg.cleanPrefix.length);
      }
    }

    break;
  }

  return request;
}
`)))
}

func (s *LesserApiStack) createSharedResources() {
	isProd := naming.IsLiveEnvironment(s.Environment)

	// Create main DynamoDB table with streams
	mainTableName := naming.ResourceNameWithApp(s.AppName, "main-table", s.Environment)
	mainTable := apptheorycdk.NewAppTheoryDynamoTable(s.Stack, jsii.String("LesserTable"), &apptheorycdk.AppTheoryDynamoTableProps{
		TableName:                 jsii.String(mainTableName),
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		BillingMode:               awsdynamodb.BillingMode_PAY_PER_REQUEST,
		EnableStream:              jsii.Bool(true),
		StreamViewType:            awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
		TimeToLiveAttribute:       jsii.String("ttl"),
		EnablePointInTimeRecovery: jsii.Bool(isProd),
		DeletionProtection:        jsii.Bool(isProd),
		RemovalPolicy:             getRemovalPolicy(isProd),
	})
	s.MainTable = mainTable.Table()

	// Stream event log table for Mastodon-compatible SSE endpoints.
	// This table is polled by the SSE Lambda during response streaming invocations.
	streamEventsTable := apptheorycdk.NewAppTheoryDynamoTable(s.Stack, jsii.String("StreamEventsTable"), &apptheorycdk.AppTheoryDynamoTableProps{
		TableName:                 jsii.String(naming.ResourceNameWithApp(s.AppName, "stream-events-table", s.Environment)),
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		BillingMode:               awsdynamodb.BillingMode_PAY_PER_REQUEST,
		TimeToLiveAttribute:       jsii.String("ttl"),
		EnablePointInTimeRecovery: jsii.Bool(isProd),
		DeletionProtection:        jsii.Bool(isProd),
		RemovalPolicy:             getRemovalPolicy(isProd),
	})
	s.StreamEventsTable = streamEventsTable.Table()

	// Create rate limit table for Lift's limited library
	// The limited library uses its own table structure for rate limiting
	rateLimitTable := apptheorycdk.NewAppTheoryDynamoTable(s.Stack, jsii.String("RateLimitTable"), &apptheorycdk.AppTheoryDynamoTableProps{
		TableName:                 jsii.String(naming.ResourceNameWithApp(s.AppName, "rate-limits-table", s.Environment)),
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		BillingMode:               awsdynamodb.BillingMode_PAY_PER_REQUEST,
		TimeToLiveAttribute:       jsii.String("ExpiresAt"),
		EnablePointInTimeRecovery: jsii.Bool(isProd),
		DeletionProtection:        jsii.Bool(false),             // Rate limit data is transient
		RemovalPolicy:             awscdk.RemovalPolicy_DESTROY, // Can be recreated
	})
	s.RateLimitTable = rateLimitTable.Table()

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

	mediaBucket := awss3.NewBucket(s.Stack, jsii.String("MediaBucket"), &awss3.BucketProps{
		BucketName:        jsii.String(naming.S3BucketName(s.AppName, stage, "media", s.AccountID, s.Region)),
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		EnforceSSL:        jsii.Bool(true),
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
	})

	mediaCDN := apptheorycdk.NewAppTheoryMediaCdn(s.Stack, jsii.String("MediaCDN"), &apptheorycdk.AppTheoryMediaCdnProps{
		Bucket: mediaBucket,
		Domain: &apptheorycdk.MediaCdnDomainConfig{
			HostedZone:       s.HostedZone,
			Certificate:      s.CDNCertificate,
			DomainName:       jsii.String(mediaDomain),
			CreateAAAARecord: jsii.Bool(true),
		},
		RemovalPolicy:     getRemovalPolicy(isProd),
		AutoDeleteObjects: jsii.Bool(!isProd),
		PriceClass:        awscloudfront.PriceClass_PRICE_CLASS_100, // US and Europe only for cost optimization
	})

	s.MediaBucket = mediaBucket
	s.MediaDistribution = mediaCDN.Distribution()

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
	isProd := naming.IsLiveEnvironment(s.Environment)
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

			queue := apptheorycdk.NewAppTheoryQueue(s.Stack, jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueId(logical))), &apptheorycdk.AppTheoryQueueProps{
				QueueName:              jsii.String(primaryName),
				VisibilityTimeout:      defaultVisibility,
				RetentionPeriod:        defaultRetention,
				ReceiveMessageWaitTime: awscdk.Duration_Seconds(jsii.Number(20)),
				EnableDlq:              jsii.Bool(true),
				MaxReceiveCount:        defaultMaxReceive,
				DlqRetentionPeriod:     awscdk.Duration_Days(jsii.Number(14)),
				RemovalPolicy:          getRemovalPolicy(isProd),
			})

			primaryQueue := queue.Queue()
			deadLetterQueue := queue.DeadLetterQueue()

			if primaryTrig, ok := primaryTriggerByQueue[logical]; ok {
				consumerName := primaryConsumerByQueue[logical]
				if consumerName == "" {
					panic(fmt.Sprintf("queue %s has trigger but no consumer mapping", logical))
				}
				consumer := s.Functions.Must(consumerName)

				consumerProps := &apptheorycdk.AppTheoryQueueConsumerProps{
					Queue:                   primaryQueue,
					Consumer:                consumer,
					ReportBatchItemFailures: jsii.Bool(primaryTrig.EnablePartialFailure),
				}
				if primaryTrig.BatchSize > 0 {
					consumerProps.BatchSize = jsii.Number(float64(primaryTrig.BatchSize))
				}
				if primaryTrig.MaxBatchingWindowSeconds > 0 {
					consumerProps.MaxBatchingWindow = awscdk.Duration_Seconds(jsii.Number(float64(primaryTrig.MaxBatchingWindowSeconds)))
				}
				apptheorycdk.NewAppTheoryQueueConsumer(s.Stack, jsii.String(fmt.Sprintf("%sConsumer", sanitizeQueueId(logical))), consumerProps)
			}

			if primaryQueue != nil {
				awscdk.Tags_Of(primaryQueue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
				awscdk.Tags_Of(primaryQueue).Add(jsii.String("stage"), jsii.String(string(naming.StageForEnvironment(s.Environment))), nil)
			}
			if deadLetterQueue != nil {
				awscdk.Tags_Of(deadLetterQueue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
				awscdk.Tags_Of(deadLetterQueue).Add(jsii.String("stage"), jsii.String(string(naming.StageForEnvironment(s.Environment))), nil)
			}

			queuePairs[logical] = localconstructs.QueuePair{Primary: primaryQueue, DLQ: deadLetterQueue}
		}
	}

	// Scheduled publishing queue is part of the canonical env-var contract (Spec 05) even though it is not
	// currently wired as an event source mapping (inventory has no scheduled queue consumer).
	if _, exists := queuePairs["scheduled-queue"]; !exists {
		logical := "scheduled-queue"
		primaryName := naming.ResourceNameWithApp(s.AppName, logical, s.Environment)
		queue := apptheorycdk.NewAppTheoryQueue(s.Stack, jsii.String(fmt.Sprintf("%sQueue", sanitizeQueueId(logical))), &apptheorycdk.AppTheoryQueueProps{
			QueueName:              jsii.String(primaryName),
			VisibilityTimeout:      defaultVisibility,
			RetentionPeriod:        defaultRetention,
			ReceiveMessageWaitTime: awscdk.Duration_Seconds(jsii.Number(20)),
			EnableDlq:              jsii.Bool(true),
			MaxReceiveCount:        defaultMaxReceive,
			DlqRetentionPeriod:     awscdk.Duration_Days(jsii.Number(14)),
			RemovalPolicy:          getRemovalPolicy(isProd),
		})

		primaryQueue := queue.Queue()
		deadLetterQueue := queue.DeadLetterQueue()

		if primaryQueue != nil {
			awscdk.Tags_Of(primaryQueue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
			awscdk.Tags_Of(primaryQueue).Add(jsii.String("environment"), jsii.String(s.Environment), nil)
		}
		if deadLetterQueue != nil {
			awscdk.Tags_Of(deadLetterQueue).Add(jsii.String("app"), jsii.String(s.AppName), nil)
			awscdk.Tags_Of(deadLetterQueue).Add(jsii.String("environment"), jsii.String(s.Environment), nil)
		}

		queuePairs[logical] = localconstructs.QueuePair{Primary: primaryQueue, DLQ: deadLetterQueue}
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
		BodyEnabled:          s.bodyEnabled(),
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
		Value:       s.API.RestApi.Api().Url(),
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

	s.publishStageExportsToSSM()
}

func (s *LesserApiStack) publishStageExportsToSSM() {
	if s.Stack == nil {
		return
	}

	appName := strings.TrimSpace(s.AppName)
	if appName == "" {
		appName = naming.DefaultAppName
	}
	stage := naming.StageForEnvironment(s.Environment)
	paramPrefix := fmt.Sprintf("/%s/%s/lesser/exports/v1", appName, stage)

	if s.MainTable != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("LesserStageTableNameParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/table_name", paramPrefix)),
			StringValue:   s.MainTable.TableName(),
			Description:   jsii.String("Lesser stage DynamoDB table name"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}

	if s.MediaBucket != nil {
		awsssm.NewStringParameter(s.Stack, jsii.String("LesserStageMediaBucketNameParam"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("%s/media_bucket_name", paramPrefix)),
			StringValue:   s.MediaBucket.BucketName(),
			Description:   jsii.String("Lesser stage media bucket name"),
			Tier:          awsssm.ParameterTier_STANDARD,
		})
	}

	awsssm.NewStringParameter(s.Stack, jsii.String("LesserStageDomainParam"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("%s/domain", paramPrefix)),
		StringValue:   jsii.String(s.Domain),
		Description:   jsii.String("Lesser stage domain"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})
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
