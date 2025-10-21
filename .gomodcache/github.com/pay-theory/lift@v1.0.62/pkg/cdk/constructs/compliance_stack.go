package constructs

// Package constructs provides high‑level CDK constructs for building a compliance stack.
// It wires together AWS services such as CloudTrail, Config, GuardDuty, Security Hub,
// KMS, S3 and Lambda to deliver a turnkey solution that can be customized via
// `ComplianceStackProps`. The construct is deliberately opinionated but extensible.
//
// Example usage:
//
//   import (
//       "github.com/aws/aws-cdk-go/awscdk/v2"
//       "github.com/yourorg/lift/pkg/cdk/constructs"
//   )
//
//   app := awscdk.NewApp(nil)
//   stack := awscdk.NewStack(app, jsii.String("ComplianceStack"), &awscdk.StackProps{})
//   constructs.NewComplianceStack(stack, "MyCompliance", &constructs.ComplianceStackProps{
//       AppName:              jsii.String(\"myapp\"),
//       ComplianceFrameworks: &[]constructs.ComplianceFramework{constructs.SOC2},
//       EnableCloudTrail:    jsii.Bool(true),
//   })
//

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudtrail"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsconfig"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsguardduty"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecurityhub"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsservicecatalog"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// ComplianceFramework enumerates the supported compliance frameworks that can be enabled
// by the `ComplianceStack`. The value is used to drive AWS Config rule creation and
// Security Hub standard enablement.
//
// Example:
//
//	fw := constructs.SOC2          // Service Organization Control 2
//	props := &constructs.ComplianceStackProps{
//	    ComplianceFrameworks: &[]constructs.ComplianceFramework{fw},
//	}
type ComplianceFramework string

const (
	// SOC2 Service Organization Control 2
	SOC2 ComplianceFramework = "SOC2"
	// HIPAA Health Insurance Portability and Accountability Act
	HIPAA ComplianceFramework = "HIPAA"
	// PCI_DSS Payment Card Industry Data Security Standard
	PCI_DSS ComplianceFramework = "PCI_DSS"
	// ISO27001 Information Security Management System
	ISO27001 ComplianceFramework = "ISO27001"
	// FedRAMP Federal Risk and Authorization Management Program
	FedRAMP ComplianceFramework = "FedRAMP"
	// GDPR General Data Protection Regulation
	GDPR ComplianceFramework = "GDPR"
)

// ComplianceStackProps configures the behavior of a `ComplianceStack`. All fields are
// optional; sensible defaults are applied when values are omitted.
//
// Example:
//
//	props := &constructs.ComplianceStackProps{
//	    AppName:               jsii.String(\"myapp\"),
//	    EnableCloudTrail:      jsii.Bool(true),
//	    ComplianceFrameworks:  &[]constructs.ComplianceFramework{constructs.SOC2, constructs.HIPAA},
//	    DataRetentionDays:     jsii.Number(3650), // ten years
//	}
type ComplianceStackProps struct {
	// Application name for resource naming
	AppName *string

	// Compliance frameworks to implement
	ComplianceFrameworks *[]ComplianceFramework

	// Enable CloudTrail logging
	EnableCloudTrail *bool

	// Enable AWS Config rules
	EnableConfig *bool

	// Enable GuardDuty threat detection
	EnableGuardDuty *bool

	// Enable Security Hub
	EnableSecurityHub *bool

	// Enable data encryption at rest
	EnableEncryption *bool

	// Data retention period in days
	DataRetentionDays *float64

	// Enable compliance reports
	EnableComplianceReports *bool

	// S3 bucket for compliance data
	ComplianceBucket awss3.IBucket

	// KMS key for encryption
	EncryptionKey awskms.IKey

	// CloudWatch log group for compliance logs
	ComplianceLogGroup awslogs.ILogGroup

	// Enable detailed access logging
	EnableDetailedLogging *bool

	// Enable audit trail
	EnableAuditTrail *bool

	// Environment for compliance (dev, staging, prod)
	Environment *string

	// Organization ID for multi-account setup
	OrganizationId *string

	// Enable compliance automation
	EnableAutomation *bool

	// Notification topic ARN for compliance alerts
	NotificationTopicArn *string
}

// ComplianceStack is the concrete CDK construct that aggregates all resources required for
// a compliance‑focused deployment. It exposes references to the underlying AWS services so
// callers can further customize or attach additional permissions.
//
// Example:
//
//	cs := constructs.NewComplianceStack(stack, \"MyCompliance\", props)
//	fmt.Println(\"CloudTrail enabled?\", cs.CloudTrail != nil)
type ComplianceStack struct {
	constructs.Construct
	CloudTrail         awscloudtrail.Trail
	ConfigRecorder     awsconfig.CfnConfigurationRecorder
	GuardDutyDetector  awsguardduty.CfnDetector
	SecurityHub        awssecurityhub.CfnHub
	ComplianceBucket   awss3.Bucket
	EncryptionKey      awskms.Key
	ComplianceLogGroup awslogs.LogGroup
	ComplianceFunction awslambda.Function
}

// NewComplianceStack is the public constructor for the `ComplianceStack` CDK construct.
// It validates input and wires together all sub‑components. The returned value can be
// used directly or stored in a variable for later reference.
//
// Example:
//
//	cs := constructs.NewComplianceStack(app, \"Compliance\", &constructs.ComplianceStackProps{
//	    AppName: jsii.String(\"demo\"),
//	})
func NewComplianceStack(scope constructs.Construct, id string, props *ComplianceStackProps) *ComplianceStack {
	this := constructs.NewConstruct(scope, &id)

	builder := newComplianceStackBuilder(this, props)
	return builder.build()
}

// complianceStackBuilder builds compliance stack components
type complianceStackBuilder struct {
	stack  constructs.Construct
	props  *ComplianceStackProps
	config *complianceStackConfig
}

// complianceStackConfig holds resolved configuration values
// Memory optimized: 48 → 32 bytes (16 bytes saved)
type complianceStackConfig struct {
	// String first (16 bytes)
	environment string
	// Float64 (8 bytes)
	dataRetentionDays float64
	// Booleans (1 byte each, packed together)
	enableCloudTrail        bool
	enableConfig            bool
	enableGuardDuty         bool
	enableSecurityHub       bool
	enableEncryption        bool
	enableComplianceReports bool
	enableAutomation        bool
}

// newComplianceStackBuilder creates a new compliance stack builder
func newComplianceStackBuilder(stack constructs.Construct, props *ComplianceStackProps) *complianceStackBuilder {
	return &complianceStackBuilder{
		stack:  stack,
		props:  props,
		config: buildComplianceStackConfig(props),
	}
}

// buildComplianceStackConfig resolves configuration values with defaults
func buildComplianceStackConfig(props *ComplianceStackProps) *complianceStackConfig {
	config := &complianceStackConfig{
		enableCloudTrail:        true,
		enableConfig:            true,
		enableGuardDuty:         true,
		enableSecurityHub:       true,
		enableEncryption:        true,
		dataRetentionDays:       2555, // 7 years
		enableComplianceReports: true,
		environment:             "prod",
		enableAutomation:        true,
	}

	// Apply provided values
	if props.EnableCloudTrail != nil {
		config.enableCloudTrail = *props.EnableCloudTrail
	}
	if props.EnableConfig != nil {
		config.enableConfig = *props.EnableConfig
	}
	if props.EnableGuardDuty != nil {
		config.enableGuardDuty = *props.EnableGuardDuty
	}
	if props.EnableSecurityHub != nil {
		config.enableSecurityHub = *props.EnableSecurityHub
	}
	if props.EnableEncryption != nil {
		config.enableEncryption = *props.EnableEncryption
	}
	if props.DataRetentionDays != nil {
		config.dataRetentionDays = *props.DataRetentionDays
	}
	if props.EnableComplianceReports != nil {
		config.enableComplianceReports = *props.EnableComplianceReports
	}
	if props.Environment != nil {
		config.environment = *props.Environment
	}
	if props.EnableAutomation != nil {
		config.enableAutomation = *props.EnableAutomation
	}

	return config
}

// build constructs the complete compliance stack
func (b *complianceStackBuilder) build() *ComplianceStack {
	// Create encryption resources
	encryptionKey := b.setupEncryption()

	// Create storage resources
	complianceBucket := b.setupComplianceBucket(encryptionKey)
	complianceLogGroup := b.setupComplianceLogGroup(encryptionKey)

	// Create monitoring and auditing services
	cloudTrail := b.setupCloudTrail(complianceBucket, complianceLogGroup)
	configRecorder := b.setupConfig(complianceBucket)
	guardDutyDetector := b.setupGuardDuty()
	securityHub := b.setupSecurityHub()

	// Create automation and reporting
	complianceFunction := b.setupComplianceFunction(complianceBucket, encryptionKey)
	b.setupComplianceReports(complianceBucket, complianceFunction)

	// Store configuration
	b.storeConfiguration()

	return &ComplianceStack{
		Construct:          b.stack,
		CloudTrail:         cloudTrail,
		ConfigRecorder:     configRecorder,
		GuardDutyDetector:  guardDutyDetector,
		SecurityHub:        securityHub,
		ComplianceBucket:   complianceBucket,
		EncryptionKey:      encryptionKey,
		ComplianceLogGroup: complianceLogGroup,
		ComplianceFunction: complianceFunction,
	}
}

// setupEncryption creates KMS encryption key if enabled
func (b *complianceStackBuilder) setupEncryption() awskms.Key {
	if !b.config.enableEncryption {
		return nil
	}

	if b.props.EncryptionKey != nil {
		if key, ok := b.props.EncryptionKey.(awskms.Key); ok {
			return key
		}
	}

	encryptionBuilder := newEncryptionKeyBuilder(b.stack, b.props)
	return encryptionBuilder.build()
}

// setupComplianceBucket creates S3 bucket for compliance data
func (b *complianceStackBuilder) setupComplianceBucket(encryptionKey awskms.Key) awss3.Bucket {
	if b.props.ComplianceBucket != nil {
		if bucket, ok := b.props.ComplianceBucket.(awss3.Bucket); ok {
			return bucket
		}
	}

	bucketBuilder := newComplianceBucketBuilder(b.stack, b.props, b.config, encryptionKey)
	return bucketBuilder.build()
}

// setupComplianceLogGroup creates CloudWatch log group for compliance logs
func (b *complianceStackBuilder) setupComplianceLogGroup(encryptionKey awskms.Key) awslogs.LogGroup {
	if b.props.ComplianceLogGroup != nil {
		if lg, ok := b.props.ComplianceLogGroup.(awslogs.LogGroup); ok {
			return lg
		}
	}

	return awslogs.NewLogGroup(b.stack, jsii.String("ComplianceLogGroup"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/aws/compliance/%s", *b.props.AppName)),
		Retention:     awslogs.RetentionDays_ONE_YEAR,
		RemovalPolicy: awscdk.RemovalPolicy_RETAIN,
		EncryptionKey: func() awskms.IKey {
			if b.config.enableEncryption {
				return encryptionKey
			}
			return nil
		}(),
	})
}

// setupCloudTrail creates CloudTrail if enabled
func (b *complianceStackBuilder) setupCloudTrail(bucket awss3.Bucket, logGroup awslogs.LogGroup) awscloudtrail.Trail {
	if !b.config.enableCloudTrail {
		return nil
	}

	return awscloudtrail.NewTrail(b.stack, jsii.String("CloudTrail"), &awscloudtrail.TrailProps{
		TrailName:                  jsii.String(fmt.Sprintf("%s-compliance-trail", *b.props.AppName)),
		Bucket:                     bucket,
		S3KeyPrefix:                jsii.String("cloudtrail/"),
		IncludeGlobalServiceEvents: jsii.Bool(true),
		IsMultiRegionTrail:         jsii.Bool(true),
		EnableFileValidation:       jsii.Bool(true),
		SendToCloudWatchLogs:       jsii.Bool(true),
		CloudWatchLogGroup:         logGroup,
	})
}

// setupConfig creates AWS Config configuration recorder if enabled
func (b *complianceStackBuilder) setupConfig(bucket awss3.Bucket) awsconfig.CfnConfigurationRecorder {
	if !b.config.enableConfig {
		return nil
	}

	configBuilder := newConfigRecorderBuilder(b.stack, b.props, bucket)
	return configBuilder.build()
}

// setupGuardDuty creates GuardDuty detector if enabled
func (b *complianceStackBuilder) setupGuardDuty() awsguardduty.CfnDetector {
	if !b.config.enableGuardDuty {
		return nil
	}

	guardDutyBuilder := newGuardDutyDetectorBuilder(b.stack)
	return guardDutyBuilder.build()
}

// setupSecurityHub creates Security Hub if enabled
func (b *complianceStackBuilder) setupSecurityHub() awssecurityhub.CfnHub {
	if !b.config.enableSecurityHub {
		return nil
	}

	securityHubBuilder := newSecurityHubBuilder(b.stack, b.props)
	return securityHubBuilder.build()
}

// setupComplianceFunction creates compliance automation function if enabled
func (b *complianceStackBuilder) setupComplianceFunction(bucket awss3.Bucket, key awskms.Key) awslambda.Function {
	if !b.config.enableAutomation {
		return nil
	}

	return createComplianceFunction(b.stack, b.props, bucket, key)
}

// setupComplianceReports creates compliance reports if enabled
func (b *complianceStackBuilder) setupComplianceReports(bucket awss3.Bucket, function awslambda.Function) {
	if !b.config.enableComplianceReports {
		return
	}

	createComplianceReports(b.stack, b.props, bucket, function)
}

// storeConfiguration stores compliance configuration in SSM Parameter Store
func (b *complianceStackBuilder) storeConfiguration() {
	storeComplianceConfiguration(b.stack, b.props)
}

// encryptionKeyBuilder builds KMS encryption key
type encryptionKeyBuilder struct {
	scope constructs.Construct
	props *ComplianceStackProps
}

// newEncryptionKeyBuilder creates a new encryption key builder
func newEncryptionKeyBuilder(scope constructs.Construct, props *ComplianceStackProps) *encryptionKeyBuilder {
	return &encryptionKeyBuilder{
		scope: scope,
		props: props,
	}
}

// build creates the KMS encryption key
func (ekb *encryptionKeyBuilder) build() awskms.Key {
	encryptionKey := awskms.NewKey(ekb.scope, jsii.String("ComplianceKey"), &awskms.KeyProps{
		Description:       jsii.String(fmt.Sprintf("Compliance encryption key for %s", *ekb.props.AppName)),
		EnableKeyRotation: jsii.Bool(true),
		Policy: awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
			Statements: &[]awsiam.PolicyStatement{
				awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
					Sid:    jsii.String("Enable IAM User Permissions"),
					Effect: awsiam.Effect_ALLOW,
					Principals: &[]awsiam.IPrincipal{
						awsiam.NewAccountRootPrincipal(),
					},
					Actions:   &[]*string{jsii.String("kms:*")},
					Resources: &[]*string{jsii.String("*")},
				}),
				awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
					Sid:    jsii.String("Allow CloudTrail to encrypt logs"),
					Effect: awsiam.Effect_ALLOW,
					Principals: &[]awsiam.IPrincipal{
						awsiam.NewServicePrincipal(jsii.String("cloudtrail.amazonaws.com"), nil),
					},
					Actions: &[]*string{
						jsii.String("kms:GenerateDataKey*"),
						jsii.String("kms:DescribeKey"),
					},
					Resources: &[]*string{jsii.String("*")},
				}),
			},
		}),
	})

	// Add alias for easier identification
	encryptionKey.AddAlias(jsii.String(fmt.Sprintf("alias/%s-compliance", *ekb.props.AppName)))
	return encryptionKey
}

// complianceBucketBuilder builds S3 compliance bucket
type complianceBucketBuilder struct {
	scope         constructs.Construct
	props         *ComplianceStackProps
	config        *complianceStackConfig
	encryptionKey awskms.Key
}

// newComplianceBucketBuilder creates a new compliance bucket builder
func newComplianceBucketBuilder(scope constructs.Construct, props *ComplianceStackProps, config *complianceStackConfig, encryptionKey awskms.Key) *complianceBucketBuilder {
	return &complianceBucketBuilder{
		scope:         scope,
		props:         props,
		config:        config,
		encryptionKey: encryptionKey,
	}
}

// build creates the S3 compliance bucket
func (cbb *complianceBucketBuilder) build() awss3.Bucket {
	return awss3.NewBucket(cbb.scope, jsii.String("ComplianceBucket"), &awss3.BucketProps{
		BucketName: jsii.String(fmt.Sprintf("%s-compliance-%s", *cbb.props.AppName, *awscdk.Stack_Of(cbb.scope).Region())),
		Encryption: func() awss3.BucketEncryption {
			if cbb.config.enableEncryption {
				return awss3.BucketEncryption_KMS
			}
			return awss3.BucketEncryption_S3_MANAGED
		}(),
		EncryptionKey: func() awskms.IKey {
			if cbb.config.enableEncryption {
				return cbb.encryptionKey
			}
			return nil
		}(),
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		Versioned:         jsii.Bool(true),
		LifecycleRules: &[]*awss3.LifecycleRule{
			{
				Id: jsii.String("ComplianceDataLifecycle"),
				Transitions: &[]*awss3.Transition{
					{
						StorageClass:    awss3.StorageClass_INFREQUENT_ACCESS(),
						TransitionAfter: awscdk.Duration_Days(jsii.Number(30)),
					},
					{
						StorageClass:    awss3.StorageClass_GLACIER(),
						TransitionAfter: awscdk.Duration_Days(jsii.Number(90)),
					},
					{
						StorageClass:    awss3.StorageClass_DEEP_ARCHIVE(),
						TransitionAfter: awscdk.Duration_Days(jsii.Number(365)),
					},
				},
				Expiration: awscdk.Duration_Days(jsii.Number(cbb.config.dataRetentionDays)),
			},
		},
		ServerAccessLogsPrefix: jsii.String("access-logs/"),
	})
}

// configRecorderBuilder builds AWS Config recorder
type configRecorderBuilder struct {
	scope  constructs.Construct
	props  *ComplianceStackProps
	bucket awss3.Bucket
}

// newConfigRecorderBuilder creates a new config recorder builder
func newConfigRecorderBuilder(scope constructs.Construct, props *ComplianceStackProps, bucket awss3.Bucket) *configRecorderBuilder {
	return &configRecorderBuilder{
		scope:  scope,
		props:  props,
		bucket: bucket,
	}
}

// build creates the AWS Config configuration recorder
func (crb *configRecorderBuilder) build() awsconfig.CfnConfigurationRecorder {
	// Create Config service role
	configRole := awsiam.NewRole(crb.scope, jsii.String("ConfigRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("config.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/ConfigRole")),
		},
	})

	// Create Config delivery channel
	awsconfig.NewCfnDeliveryChannel(crb.scope, jsii.String("ConfigDeliveryChannel"), &awsconfig.CfnDeliveryChannelProps{
		S3BucketName: crb.bucket.BucketName(),
		S3KeyPrefix:  jsii.String("config/"),
		ConfigSnapshotDeliveryProperties: &awsconfig.CfnDeliveryChannel_ConfigSnapshotDeliveryPropertiesProperty{
			DeliveryFrequency: jsii.String("TwentyFour_Hours"),
		},
	})

	// Create Config recorder
	configRecorder := awsconfig.NewCfnConfigurationRecorder(crb.scope, jsii.String("ConfigRecorder"), &awsconfig.CfnConfigurationRecorderProps{
		RoleArn: configRole.RoleArn(),
		RecordingGroup: &awsconfig.CfnConfigurationRecorder_RecordingGroupProperty{
			AllSupported:               jsii.Bool(true),
			IncludeGlobalResourceTypes: jsii.Bool(true),
			RecordingStrategy: &awsconfig.CfnConfigurationRecorder_RecordingStrategyProperty{
				UseOnly: jsii.String("ALL_SUPPORTED_RESOURCE_TYPES"),
			},
		},
	})

	// Create Config rules for compliance frameworks
	if crb.props.ComplianceFrameworks != nil {
		for _, framework := range *crb.props.ComplianceFrameworks {
			createConfigRulesForFramework(crb.scope, framework)
		}
	}

	return configRecorder
}

// guardDutyDetectorBuilder builds GuardDuty detector
type guardDutyDetectorBuilder struct {
	scope constructs.Construct
}

// newGuardDutyDetectorBuilder creates a new GuardDuty detector builder
func newGuardDutyDetectorBuilder(scope constructs.Construct) *guardDutyDetectorBuilder {
	return &guardDutyDetectorBuilder{
		scope: scope,
	}
}

// build creates the GuardDuty detector
func (gdb *guardDutyDetectorBuilder) build() awsguardduty.CfnDetector {
	return awsguardduty.NewCfnDetector(gdb.scope, jsii.String("GuardDutyDetector"), &awsguardduty.CfnDetectorProps{
		Enable:                     jsii.Bool(true),
		FindingPublishingFrequency: jsii.String("FIFTEEN_MINUTES"),
		Features: &[]interface{}{
			&awsguardduty.CfnDetector_CFNFeatureConfigurationProperty{
				Name:   jsii.String("S3_DATA_EVENTS"),
				Status: jsii.String("ENABLED"),
			},
			&awsguardduty.CfnDetector_CFNFeatureConfigurationProperty{
				Name:   jsii.String("EKS_AUDIT_LOGS"),
				Status: jsii.String("ENABLED"),
			},
			&awsguardduty.CfnDetector_CFNFeatureConfigurationProperty{
				Name:   jsii.String("RDS_LOGIN_EVENTS"),
				Status: jsii.String("ENABLED"),
			},
			&awsguardduty.CfnDetector_CFNFeatureConfigurationProperty{
				Name:   jsii.String("LAMBDA_NETWORK_LOGS"),
				Status: jsii.String("ENABLED"),
			},
		},
	})
}

// securityHubBuilder builds Security Hub
type securityHubBuilder struct {
	scope constructs.Construct
	props *ComplianceStackProps
}

// newSecurityHubBuilder creates a new Security Hub builder
func newSecurityHubBuilder(scope constructs.Construct, props *ComplianceStackProps) *securityHubBuilder {
	return &securityHubBuilder{
		scope: scope,
		props: props,
	}
}

// build creates the Security Hub
func (shb *securityHubBuilder) build() awssecurityhub.CfnHub {
	securityHub := awssecurityhub.NewCfnHub(shb.scope, jsii.String("SecurityHub"), &awssecurityhub.CfnHubProps{
		AutoEnableControls:     jsii.Bool(true),
		EnableDefaultStandards: jsii.Bool(true),
		Tags: map[string]*string{
			"Application": shb.props.AppName,
			"Environment": shb.props.Environment,
		},
	})

	// Enable compliance standards
	if shb.props.ComplianceFrameworks != nil {
		for i, framework := range *shb.props.ComplianceFrameworks {
			enableComplianceStandard(shb.scope, framework, i)
		}
	}

	return securityHub
}

// createConfigRulesForFramework creates AWS Config rules based on the compliance framework
func createConfigRulesForFramework(scope constructs.Construct, framework ComplianceFramework) {
	switch framework {
	case SOC2:
		// Create SOC2-specific Config rules
		awsconfig.NewCfnConfigRule(scope, jsii.String("SOC2RootAccountMFAEnabled"), &awsconfig.CfnConfigRuleProps{
			ConfigRuleName: jsii.String("soc2-root-account-mfa-enabled"),
			Description:    jsii.String("Checks whether MFA is enabled for the root user"),
			Source: &awsconfig.CfnConfigRule_SourceProperty{
				Owner:            jsii.String("AWS"),
				SourceIdentifier: jsii.String("ROOT_ACCOUNT_MFA_ENABLED"),
			},
		})
	case HIPAA:
		// Create HIPAA-specific Config rules
		awsconfig.NewCfnConfigRule(scope, jsii.String("HIPAAEncryptedVolumes"), &awsconfig.CfnConfigRuleProps{
			ConfigRuleName: jsii.String("hipaa-encrypted-volumes"),
			Description:    jsii.String("Checks whether EBS volumes are encrypted"),
			Source: &awsconfig.CfnConfigRule_SourceProperty{
				Owner:            jsii.String("AWS"),
				SourceIdentifier: jsii.String("ENCRYPTED_VOLUMES"),
			},
		})
	case PCI_DSS:
		// Create PCI DSS-specific Config rules
		awsconfig.NewCfnConfigRule(scope, jsii.String("PCIDSSAccessLogsEnabled"), &awsconfig.CfnConfigRuleProps{
			ConfigRuleName: jsii.String("pci-dss-access-logs-enabled"),
			Description:    jsii.String("Checks whether access logs are enabled"),
			Source: &awsconfig.CfnConfigRule_SourceProperty{
				Owner:            jsii.String("AWS"),
				SourceIdentifier: jsii.String("S3_BUCKET_LOGGING_ENABLED"),
			},
		})
	}
}

// enableComplianceStandard enables specific compliance standards in Security Hub
func enableComplianceStandard(scope constructs.Construct, framework ComplianceFramework, index int) {
	// Enable specific standards based on the framework using CfnStandard
	switch framework {
	case SOC2:
		// Enable CIS AWS Foundations Benchmark for SOC2
		awssecurityhub.NewCfnStandard(scope, jsii.String(fmt.Sprintf("SOC2Standard%d", index)), &awssecurityhub.CfnStandardProps{
			StandardsArn: jsii.String("arn:aws:securityhub:::standard/cis-aws-foundations-benchmark/v/1.2.0"),
		})
	case HIPAA:
		// Enable AWS Foundational Security Standard for HIPAA
		awssecurityhub.NewCfnStandard(scope, jsii.String(fmt.Sprintf("HIPAAStandard%d", index)), &awssecurityhub.CfnStandardProps{
			StandardsArn: jsii.String("arn:aws:securityhub:::standard/aws-foundational-security-best-practices/v/1.0.0"),
		})
	case PCI_DSS:
		// Enable PCI DSS standard
		awssecurityhub.NewCfnStandard(scope, jsii.String(fmt.Sprintf("PCIDSSStandard%d", index)), &awssecurityhub.CfnStandardProps{
			StandardsArn: jsii.String("arn:aws:securityhub:::standard/pci-dss/v/3.2.1"),
		})
	case FedRAMP:
		// Enable AWS Foundational Security Standard for FedRAMP
		awssecurityhub.NewCfnStandard(scope, jsii.String(fmt.Sprintf("FedRAMPStandard%d", index)), &awssecurityhub.CfnStandardProps{
			StandardsArn: jsii.String("arn:aws:securityhub:::standard/aws-foundational-security-best-practices/v/1.0.0"),
		})
	}
}

// createComplianceFunction creates a Lambda function for compliance automation
func createComplianceFunction(scope constructs.Construct, props *ComplianceStackProps, bucket awss3.Bucket, key awskms.Key) awslambda.Function {

	// Ensure non-nil environment variables for JSII
	env := props.Environment
	if env == nil {
		env = jsii.String("prod")
	}

	function := CreateStandardLambdaFunction(scope, "ComplianceFunction", bucket, key, LambdaFunctionConfig{
		FunctionName: fmt.Sprintf("%s-compliance-automation", *props.AppName),
		Description:  "Compliance automation and reporting function",
		Timeout:      awscdk.Duration_Minutes(jsii.Number(15)),
		Permissions:  PermissionReadWrite,
		Environment: map[string]*string{
			"COMPLIANCE_BUCKET": bucket.BucketName(),
			"APP_NAME":          props.AppName,
			"ENVIRONMENT":       env,
		},
	})

	// Add additional compliance-specific permissions
	if roleInterface := function.Role(); roleInterface != nil {
		if functionRole, ok := roleInterface.(awsiam.Role); ok {
			functionRole.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Effect: awsiam.Effect_ALLOW,
				Actions: &[]*string{
					jsii.String("config:GetComplianceDetailsByConfigRule"),
					jsii.String("config:GetComplianceDetailsByResource"),
					jsii.String("config:DescribeConfigRules"),
					jsii.String("config:DescribeComplianceByConfigRule"),
					jsii.String("securityhub:GetFindings"),
					jsii.String("securityhub:BatchImportFindings"),
					jsii.String("guardduty:GetFindings"),
					jsii.String("cloudtrail:LookupEvents"),
				},
				Resources: &[]*string{jsii.String("*")},
			}))
		}
	}

	return function
}

// createComplianceReports creates compliance reporting automation
func createComplianceReports(scope constructs.Construct, props *ComplianceStackProps, _ awss3.Bucket, _ awslambda.Function) {
	// Create EventBridge rule for daily compliance reports
	// This would trigger the compliance function daily to generate reports

	// Create Service Catalog portfolio for compliance templates
	awsservicecatalog.NewPortfolio(scope, jsii.String("CompliancePortfolio"), &awsservicecatalog.PortfolioProps{
		DisplayName:  jsii.String(fmt.Sprintf("%s Compliance Templates", *props.AppName)),
		Description:  jsii.String("Pre-approved compliance templates for consistent deployment"),
		ProviderName: jsii.String("Compliance Team"),
	})
}

// storeComplianceConfiguration stores compliance configuration in SSM Parameter Store
func storeComplianceConfiguration(scope constructs.Construct, props *ComplianceStackProps) {
	// Store compliance frameworks
	if props.ComplianceFrameworks != nil {
		frameworks := make([]string, len(*props.ComplianceFrameworks))
		for i, framework := range *props.ComplianceFrameworks {
			frameworks[i] = string(framework)
		}

		awsssm.NewStringParameter(scope, jsii.String("ComplianceFrameworks"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("/%s/compliance/frameworks", *props.AppName)),
			StringValue:   jsii.String(fmt.Sprintf("%v", frameworks)),
			Description:   jsii.String("Enabled compliance frameworks"),
		})
	}

	// Store data retention policy
	// Default to 2555 days (7 years) if not provided
	days := 2555.0
	if props.DataRetentionDays != nil {
		days = *props.DataRetentionDays
	}
	awsssm.NewStringParameter(scope, jsii.String("DataRetentionPolicy"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("/%s/compliance/data-retention-days", *props.AppName)),
		StringValue:   jsii.String(fmt.Sprintf("%.0f", days)),
		Description:   jsii.String("Data retention period in days"),
	})
}

// GetComplianceStatus reports which optional services have been instantiated in the stack.
// The returned map contains boolean flags keyed by service name, useful for health‑checks
// or conditional logic in downstream constructs.
//
// Example:
//
//	status := cs.GetComplianceStatus()
//	if status[\"cloudtrail_enabled\"].(bool) {
//	    // do something
//	}
func (c *ComplianceStack) GetComplianceStatus() map[string]interface{} {
	return map[string]interface{}{
		"cloudtrail_enabled":  c.CloudTrail != nil,
		"config_enabled":      c.ConfigRecorder != nil,
		"guardduty_enabled":   c.GuardDutyDetector != nil,
		"securityhub_enabled": c.SecurityHub != nil,
		"encryption_enabled":  c.EncryptionKey != nil,
		"function_enabled":    c.ComplianceFunction != nil,
	}
}

// AddComplianceRule creates an additional AWS Config rule and attaches it to the stack.
// This method is handy when custom rules need to be introduced after the initial construct
// creation.
//
// Example:
//
//	cs.AddComplianceRule(\"CustomS3Encryption\", \"S3_BUCKET_SERVER_SIDE_ENCRYPTION_ENABLED\")
func (c *ComplianceStack) AddComplianceRule(ruleId string, ruleName string) {
	// Create a Config rule using CfnConfigRule
	awsconfig.NewCfnConfigRule(c.Construct, jsii.String(ruleId), &awsconfig.CfnConfigRuleProps{
		ConfigRuleName: jsii.String(fmt.Sprintf("%s-rule", ruleId)),
		Description:    jsii.String(fmt.Sprintf("Additional compliance rule: %s", ruleName)),
		Source: &awsconfig.CfnConfigRule_SourceProperty{
			Owner:            jsii.String("AWS"),
			SourceIdentifier: jsii.String(ruleName),
		},
	})
}
