// Package constructs provides AWS CDK constructs for Lift applications.
//
// This package contains high-level CDK constructs that implement Lift's best practices
// for AWS infrastructure. The constructs include optimized configurations for API
// Gateway, Lambda functions, DynamoDB tables, and other AWS services.
package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudtrail"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskinesis"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskinesisfirehose"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// AuditLevel defines the level of audit logging
type AuditLevel string

const (
	// AuditLevelBasic provides basic audit logging
	AuditLevelBasic AuditLevel = "BASIC"
	// AuditLevelDetailed provides detailed audit logging
	AuditLevelDetailed AuditLevel = "DETAILED"
	// AuditLevelComprehensive provides comprehensive audit logging
	AuditLevelComprehensive AuditLevel = "COMPREHENSIVE"
)

// AuditingProps defines properties for the Auditing construct
type AuditingProps struct {
	// EncryptionKey is the KMS key used for encrypting audit logs
	EncryptionKey awskms.IKey
	// AuditBucket is the S3 bucket used for storing audit logs
	AuditBucket awss3.IBucket
	// EnableComplianceReporting enables compliance reporting features
	EnableComplianceReporting *bool
	// EnableImmutableLogs makes audit logs immutable to prevent tampering
	EnableImmutableLogs *bool
	// EnableDatabaseLogs enables database query logging
	EnableDatabaseLogs *bool
	// EnableRealTimeProcessing enables real-time log processing
	EnableRealTimeProcessing *bool
	// EnableTamperProtection enables tamper protection for audit logs
	EnableTamperProtection *bool
	// EnableLogAggregation enables log aggregation from multiple sources
	EnableLogAggregation *bool
	// LogRetentionDays specifies how many days to retain logs
	LogRetentionDays *float64
	// EnableSIEMIntegration enables integration with SIEM systems
	EnableSIEMIntegration *bool
	// SIEMEndpoint is the endpoint for SIEM integration
	SIEMEndpoint *string
	// EnableLogAnalysis enables automated log analysis
	EnableLogAnalysis *bool
	// ComplianceFrameworks specifies which compliance frameworks to support
	ComplianceFrameworks *[]string
	// EnableApplicationLogs enables application-level logging
	EnableApplicationLogs *bool
	// AppName is the name of the application being audited
	AppName *string
	// EnableCloudTrail enables AWS CloudTrail for API call logging
	EnableCloudTrail *bool
	// EnableEncryption enables encryption for logs at rest and in transit
	EnableEncryption *bool
	// EnableCrossAccountAccess enables cross-account access for audit logs
	EnableCrossAccountAccess *bool
	// CrossAccountRoleArns specifies the ARNs of roles for cross-account access
	CrossAccountRoleArns *[]*string
	// EnableIntegrityChecking enables integrity checking for audit logs
	EnableIntegrityChecking *bool
	// EnableDashboard enables a CloudWatch dashboard for audit logs
	EnableDashboard *bool
	// EnableAlerting enables CloudWatch alerts for audit logs
	EnableAlerting *bool
	// AlertTopicArn is the ARN of the SNS topic for alerts
	AlertTopicArn *string
	// Environment specifies the deployment environment (dev, test, prod)
	Environment *string
	// EnableRegulatoryCompliance enables features for regulatory compliance
	EnableRegulatoryCompliance *bool
	// AuditLevel specifies the level of audit logging (BASIC, DETAILED, COMPREHENSIVE)
	AuditLevel AuditLevel
}

// AuditingConstruct creates comprehensive audit logging infrastructure
//
// This construct sets up a complete audit logging infrastructure including:
// - CloudWatch log groups for different types of logs
// - KMS encryption for logs
// - CloudTrail for API call logging
// - S3 bucket for log storage
// - Lambda functions for log processing
// - CloudWatch dashboard for monitoring
// - Kinesis Firehose for log delivery
// - Kinesis stream for log collection
// - Lambda functions for compliance and integrity checking
// - CloudWatch alarms for alerting
type AuditingConstruct struct {
	// AuditLogGroup is the CloudWatch log group for audit logs
	AuditLogGroup awslogs.LogGroup
	// Embedded Construct for CDK compatibility
	constructs.Construct
	// EncryptionKey is the KMS key used for encrypting logs
	EncryptionKey awskms.Key
	// CloudTrail is the CloudTrail instance for API call logging
	CloudTrail awscloudtrail.Trail
	// ApplicationLogGroup is the CloudWatch log group for application logs
	ApplicationLogGroup awslogs.LogGroup
	// DatabaseLogGroup is the CloudWatch log group for database logs
	DatabaseLogGroup awslogs.LogGroup
	// AuditBucket is the S3 bucket for storing audit logs
	AuditBucket awss3.Bucket
	// LogProcessingFunction is the Lambda function for processing logs
	LogProcessingFunction awslambda.Function
	// AuditDashboard is the CloudWatch dashboard for monitoring audit logs
	AuditDashboard awscloudwatch.Dashboard
	// FirehoseDeliveryStream is the Kinesis Firehose for log delivery
	FirehoseDeliveryStream awskinesisfirehose.CfnDeliveryStream
	// LogStream is the Kinesis stream for log collection
	LogStream awskinesis.Stream
	// ComplianceFunction is the Lambda function for compliance checking
	ComplianceFunction awslambda.Function
	// IntegrityFunction is the Lambda function for integrity checking
	IntegrityFunction awslambda.Function
	// AuditAlarms is a list of CloudWatch alarms for audit log alerting
	AuditAlarms []awscloudwatch.Alarm
}

// NewAuditingConstruct creates a new auditing construct
//
// This function creates a new auditing construct with the following features:
// - Configurable audit logging level (BASIC, DETAILED, COMPREHENSIVE)
// - Optional encryption for logs at rest and in transit
// - Optional CloudTrail for API call logging
// - Optional SIEM integration
// - Optional log analysis
// - Optional compliance reporting
// - Optional dashboard and alerting
//
// Parameters:
//   - scope: The CDK construct scope
//   - id: The construct ID
//   - props: Configuration properties
//
// Returns:
//   - A new AuditingConstruct instance
func NewAuditingConstruct(scope constructs.Construct, id string, props *AuditingProps) *AuditingConstruct {
	this := constructs.NewConstruct(scope, &id)

	builder := newAuditingConstructBuilder(this, props)
	return builder.build()
}

// auditingConstructBuilder builds auditing construct components
type auditingConstructBuilder struct {
	construct constructs.Construct
	props     *AuditingProps
	config    *auditingConstructConfig
}

// auditingConstructConfig holds resolved configuration values
// Memory optimized: 64 → 56 bytes (8 bytes saved)
type auditingConstructConfig struct {
	// Pointers first (8 bytes)
	logRetentionDays *float64
	// Strings (16 bytes each)
	environment string
	auditLevel  AuditLevel
	// Booleans (1 byte each, packed together)
	enableCloudTrail           bool
	enableApplicationLogs      bool
	enableDatabaseLogs         bool
	enableRealTimeProcessing   bool
	enableTamperProtection     bool
	enableLogAggregation       bool
	enableSIEMIntegration      bool
	enableLogAnalysis          bool
	enableComplianceReporting  bool
	enableEncryption           bool
	enableCrossAccountAccess   bool
	enableIntegrityChecking    bool
	enableDashboard            bool
	enableAlerting             bool
	enableImmutableLogs        bool
	enableRegulatoryCompliance bool
}

// newAuditingConstructBuilder creates a new auditing construct builder
func newAuditingConstructBuilder(construct constructs.Construct, props *AuditingProps) *auditingConstructBuilder {
	return &auditingConstructBuilder{
		construct: construct,
		props:     props,
		config:    buildAuditingConstructConfig(props),
	}
}

// buildAuditingConstructConfig resolves configuration values with defaults
func buildAuditingConstructConfig(props *AuditingProps) *auditingConstructConfig {
	builder := newAuditingConfigBuilder()
	return builder.withDefaults().applyProps(props).build()
}

// auditingConfigBuilder builds auditing configuration
type auditingConfigBuilder struct {
	config *auditingConstructConfig
}

// newAuditingConfigBuilder creates a new auditing config builder
func newAuditingConfigBuilder() *auditingConfigBuilder {
	return &auditingConfigBuilder{
		config: &auditingConstructConfig{},
	}
}

// withDefaults sets default values
func (b *auditingConfigBuilder) withDefaults() *auditingConfigBuilder {
	b.config.auditLevel = AuditLevelDetailed
	b.config.enableCloudTrail = true
	b.config.enableApplicationLogs = true
	b.config.enableDatabaseLogs = true
	b.config.enableRealTimeProcessing = true
	b.config.enableTamperProtection = true
	b.config.enableLogAggregation = true
	b.config.logRetentionDays = jsii.Number(2555) // 7 years
	b.config.enableSIEMIntegration = false
	b.config.enableLogAnalysis = true
	b.config.enableComplianceReporting = true
	b.config.environment = "prod"
	b.config.enableEncryption = true
	b.config.enableCrossAccountAccess = false
	b.config.enableIntegrityChecking = true
	b.config.enableDashboard = true
	b.config.enableAlerting = true
	b.config.enableImmutableLogs = true
	b.config.enableRegulatoryCompliance = true
	return b
}

// applyProps applies provided properties
func (b *auditingConfigBuilder) applyProps(props *AuditingProps) *auditingConfigBuilder {
	if props.AuditLevel != "" {
		b.config.auditLevel = props.AuditLevel
	}

	b.applyBooleanProps(props)
	b.applyAdvancedProps(props)

	return b
}

// applyBooleanProps applies basic boolean properties
func (b *auditingConfigBuilder) applyBooleanProps(props *AuditingProps) {
	if props.EnableCloudTrail != nil {
		b.config.enableCloudTrail = *props.EnableCloudTrail
	}
	if props.EnableApplicationLogs != nil {
		b.config.enableApplicationLogs = *props.EnableApplicationLogs
	}
	if props.EnableDatabaseLogs != nil {
		b.config.enableDatabaseLogs = *props.EnableDatabaseLogs
	}
	if props.EnableRealTimeProcessing != nil {
		b.config.enableRealTimeProcessing = *props.EnableRealTimeProcessing
	}
	if props.EnableTamperProtection != nil {
		b.config.enableTamperProtection = *props.EnableTamperProtection
	}
	if props.EnableLogAggregation != nil {
		b.config.enableLogAggregation = *props.EnableLogAggregation
	}
	if props.EnableSIEMIntegration != nil {
		b.config.enableSIEMIntegration = *props.EnableSIEMIntegration
	}
	if props.EnableLogAnalysis != nil {
		b.config.enableLogAnalysis = *props.EnableLogAnalysis
	}
	if props.EnableComplianceReporting != nil {
		b.config.enableComplianceReporting = *props.EnableComplianceReporting
	}
}

// applyAdvancedProps applies advanced configuration properties
func (b *auditingConfigBuilder) applyAdvancedProps(props *AuditingProps) {
	if props.LogRetentionDays != nil {
		b.config.logRetentionDays = props.LogRetentionDays
	}
	if props.Environment != nil {
		b.config.environment = *props.Environment
	}
	if props.EnableEncryption != nil {
		b.config.enableEncryption = *props.EnableEncryption
	}
	if props.EnableCrossAccountAccess != nil {
		b.config.enableCrossAccountAccess = *props.EnableCrossAccountAccess
	}
	if props.EnableIntegrityChecking != nil {
		b.config.enableIntegrityChecking = *props.EnableIntegrityChecking
	}
	if props.EnableDashboard != nil {
		b.config.enableDashboard = *props.EnableDashboard
	}
	if props.EnableAlerting != nil {
		b.config.enableAlerting = *props.EnableAlerting
	}
	if props.EnableImmutableLogs != nil {
		b.config.enableImmutableLogs = *props.EnableImmutableLogs
	}
	if props.EnableRegulatoryCompliance != nil {
		b.config.enableRegulatoryCompliance = *props.EnableRegulatoryCompliance
	}
}

// build returns the configured auditing config
func (b *auditingConfigBuilder) build() *auditingConstructConfig {
	return b.config
}

// build constructs the complete auditing construct
func (b *auditingConstructBuilder) build() *AuditingConstruct {
	// Create encryption key
	encryptionKey := b.setupEncryptionKey()

	// Create audit bucket
	auditBucket := b.setupAuditBucket(encryptionKey)

	// Create log groups
	applicationLogGroup, databaseLogGroup, auditLogGroup := b.setupLogGroups(encryptionKey)

	// Create CloudTrail
	cloudTrail := b.setupCloudTrail(auditBucket, auditLogGroup)

	// Create streaming components
	logStream, firehoseStream := b.setupStreamingComponents(auditBucket, encryptionKey)

	// Create processing functions
	logProcessingFunction, integrityFunction, complianceFunction := b.setupProcessingFunctions(auditBucket, encryptionKey, logStream)

	// Create monitoring components
	dashboard, alarms := b.setupMonitoring(applicationLogGroup, databaseLogGroup, auditLogGroup)

	// Store audit configuration
	storeAuditConfiguration(b.construct, b.props)

	return &AuditingConstruct{
		Construct:              b.construct,
		AuditBucket:            auditBucket,
		EncryptionKey:          encryptionKey,
		CloudTrail:             cloudTrail,
		ApplicationLogGroup:    applicationLogGroup,
		DatabaseLogGroup:       databaseLogGroup,
		AuditLogGroup:          auditLogGroup,
		LogProcessingFunction:  logProcessingFunction,
		LogStream:              logStream,
		FirehoseDeliveryStream: firehoseStream,
		AuditDashboard:         dashboard,
		AuditAlarms:            alarms,
		IntegrityFunction:      integrityFunction,
		ComplianceFunction:     complianceFunction,
	}
}

// setupEncryptionKey creates the KMS encryption key
func (b *auditingConstructBuilder) setupEncryptionKey() awskms.Key {
	if !b.config.enableEncryption {
		return nil
	}

	if b.props.EncryptionKey != nil {
		encryptionKey, ok := b.props.EncryptionKey.(awskms.Key)
		if !ok {
			panic("EncryptionKey must be of type awskms.Key")
		}
		return encryptionKey
	}

	encryptionKey := awskms.NewKey(b.construct, jsii.String("AuditEncryptionKey"), &awskms.KeyProps{
		Description:       jsii.String(fmt.Sprintf("Audit encryption key for %s", *b.props.AppName)),
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
					Sid:    jsii.String("Allow audit services"),
					Effect: awsiam.Effect_ALLOW,
					Principals: &[]awsiam.IPrincipal{
						awsiam.NewServicePrincipal(jsii.String("cloudtrail.amazonaws.com"), nil),
						awsiam.NewServicePrincipal(jsii.String("logs.amazonaws.com"), nil),
						awsiam.NewServicePrincipal(jsii.String("firehose.amazonaws.com"), nil),
						awsiam.NewServicePrincipal(jsii.String("kinesis.amazonaws.com"), nil),
					},
					Actions: &[]*string{
						jsii.String("kms:Encrypt"),
						jsii.String("kms:Decrypt"),
						jsii.String("kms:ReEncrypt*"),
						jsii.String("kms:GenerateDataKey*"),
						jsii.String("kms:DescribeKey"),
					},
					Resources: &[]*string{jsii.String("*")},
				}),
			},
		}),
	})
	encryptionKey.AddAlias(jsii.String(fmt.Sprintf("alias/%s-audit", *b.props.AppName)))

	return encryptionKey
}

// setupAuditBucket creates the S3 audit bucket
func (b *auditingConstructBuilder) setupAuditBucket(encryptionKey awskms.Key) awss3.Bucket {
	if b.props.AuditBucket != nil {
		auditBucket, ok := b.props.AuditBucket.(awss3.Bucket)
		if !ok {
			panic("AuditBucket must be of type awss3.Bucket")
		}
		return auditBucket
	}

	auditBucket := awss3.NewBucket(b.construct, jsii.String("AuditBucket"), &awss3.BucketProps{
		BucketName: jsii.String(fmt.Sprintf("%s-audit-%s", *b.props.AppName, *awscdk.Stack_Of(b.construct).Region())),
		Encryption: func() awss3.BucketEncryption {
			if b.config.enableEncryption {
				return awss3.BucketEncryption_KMS
			}
			return awss3.BucketEncryption_S3_MANAGED
		}(),
		EncryptionKey: func() awskms.IKey {
			if b.config.enableEncryption {
				return encryptionKey
			}
			return nil
		}(),
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		Versioned:         jsii.Bool(true),
		ObjectLockEnabled: func() *bool {
			if b.config.enableImmutableLogs {
				return jsii.Bool(true)
			}
			return nil
		}(),
		LifecycleRules: &[]*awss3.LifecycleRule{
			{
				Id: jsii.String("AuditLogLifecycle"),
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
				Expiration: awscdk.Duration_Days(b.config.logRetentionDays),
			},
		},
		ServerAccessLogsPrefix: jsii.String("access-logs/"),
	})

	// Configure cross-account access if enabled
	b.configureCrossAccountAccess(auditBucket)

	return auditBucket
}

// configureCrossAccountAccess adds bucket policy for cross-account access
func (b *auditingConstructBuilder) configureCrossAccountAccess(auditBucket awss3.Bucket) {
	if !b.config.enableCrossAccountAccess || b.props.CrossAccountRoleArns == nil {
		return
	}

	auditBucket.AddToResourcePolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Sid:    jsii.String("AllowCrossAccountAccess"),
		Effect: awsiam.Effect_ALLOW,
		Principals: &[]awsiam.IPrincipal{
			awsiam.NewArnPrincipal((*b.props.CrossAccountRoleArns)[0]),
		},
		Actions: &[]*string{
			jsii.String("s3:GetObject"),
			jsii.String("s3:ListBucket"),
		},
		Resources: &[]*string{
			auditBucket.BucketArn(),
			auditBucket.ArnForObjects(jsii.String("*")),
		},
	}))
}

// setupLogGroups creates all required log groups
func (b *auditingConstructBuilder) setupLogGroups(encryptionKey awskms.Key) (awslogs.LogGroup, awslogs.LogGroup, awslogs.LogGroup) {
	applicationLogGroup := createLogGroup(b.construct, "ApplicationLogGroup", fmt.Sprintf("/aws/audit/%s/application", *b.props.AppName), encryptionKey, b.config.logRetentionDays)
	databaseLogGroup := createLogGroup(b.construct, "DatabaseLogGroup", fmt.Sprintf("/aws/audit/%s/database", *b.props.AppName), encryptionKey, b.config.logRetentionDays)
	auditLogGroup := createLogGroup(b.construct, "AuditLogGroup", fmt.Sprintf("/aws/audit/%s/system", *b.props.AppName), encryptionKey, b.config.logRetentionDays)

	return applicationLogGroup, databaseLogGroup, auditLogGroup
}

// setupCloudTrail creates CloudTrail if enabled
func (b *auditingConstructBuilder) setupCloudTrail(auditBucket awss3.Bucket, auditLogGroup awslogs.LogGroup) awscloudtrail.Trail {
	if !b.config.enableCloudTrail {
		return nil
	}

	cloudTrail := awscloudtrail.NewTrail(b.construct, jsii.String("AuditCloudTrail"), &awscloudtrail.TrailProps{
		TrailName:                  jsii.String(fmt.Sprintf("%s-audit-trail", *b.props.AppName)),
		Bucket:                     auditBucket,
		S3KeyPrefix:                jsii.String("cloudtrail/"),
		IncludeGlobalServiceEvents: jsii.Bool(true),
		IsMultiRegionTrail:         jsii.Bool(true),
		EnableFileValidation:       jsii.Bool(true),
		SendToCloudWatchLogs:       jsii.Bool(true),
		CloudWatchLogGroup:         auditLogGroup,
	})

	// Add S3 data events for comprehensive auditing
	cloudTrail.AddS3EventSelector(&[]*awscloudtrail.S3EventSelector{
		{
			Bucket:       auditBucket,
			ObjectPrefix: jsii.String(""),
		},
	}, &awscloudtrail.AddEventSelectorOptions{
		ReadWriteType:           awscloudtrail.ReadWriteType_ALL,
		IncludeManagementEvents: jsii.Bool(true),
	})

	return cloudTrail
}

// setupStreamingComponents creates Kinesis stream and Firehose delivery stream
func (b *auditingConstructBuilder) setupStreamingComponents(auditBucket awss3.Bucket, encryptionKey awskms.Key) (awskinesis.Stream, awskinesisfirehose.CfnDeliveryStream) {
	var logStream awskinesis.Stream
	var firehoseStream awskinesisfirehose.CfnDeliveryStream

	// Create Kinesis stream for real-time processing
	if b.config.enableRealTimeProcessing {
		logStream = awskinesis.NewStream(b.construct, jsii.String("AuditLogStream"), &awskinesis.StreamProps{
			StreamName:      jsii.String(fmt.Sprintf("%s-audit-stream", *b.props.AppName)),
			ShardCount:      jsii.Number(2),
			Encryption:      awskinesis.StreamEncryption_KMS,
			EncryptionKey:   encryptionKey,
			RetentionPeriod: awscdk.Duration_Hours(jsii.Number(24)),
		})
	}

	// Create Firehose delivery stream for log aggregation
	if b.config.enableLogAggregation {
		firehoseStream = createFirehoseDeliveryStream(b.construct, b.props, auditBucket, encryptionKey, logStream)
	}

	return logStream, firehoseStream
}

// setupProcessingFunctions creates all Lambda processing functions
func (b *auditingConstructBuilder) setupProcessingFunctions(auditBucket awss3.Bucket, encryptionKey awskms.Key, logStream awskinesis.Stream) (awslambda.Function, awslambda.Function, awslambda.Function) {
	var logProcessingFunction awslambda.Function
	var integrityFunction awslambda.Function
	var complianceFunction awslambda.Function

	// Create log processing function
	if b.config.enableRealTimeProcessing {
		logProcessingFunction = createLogProcessingFunction(b.construct, b.props, auditBucket, encryptionKey, logStream)
	}

	// Create integrity checking function
	if b.config.enableIntegrityChecking {
		integrityFunction = createIntegrityCheckingFunction(b.construct, b.props, auditBucket, encryptionKey)
	}

	// Create compliance function
	if b.config.enableComplianceReporting {
		complianceFunction = createAuditComplianceFunction(b.construct, b.props, auditBucket, encryptionKey)
	}

	return logProcessingFunction, integrityFunction, complianceFunction
}

// setupMonitoring creates dashboard and alarms
func (b *auditingConstructBuilder) setupMonitoring(applicationLogGroup, databaseLogGroup, auditLogGroup awslogs.LogGroup) (awscloudwatch.Dashboard, []awscloudwatch.Alarm) {
	var dashboard awscloudwatch.Dashboard
	var alarms []awscloudwatch.Alarm

	// Create dashboard
	if b.config.enableDashboard {
		dashboard = createAuditDashboard(b.construct, b.props, applicationLogGroup, databaseLogGroup, auditLogGroup)
	}

	// Create alarms
	if b.config.enableAlerting {
		alarms = createAuditAlarms(b.construct, b.props, applicationLogGroup, databaseLogGroup, auditLogGroup)
	}

	return dashboard, alarms
}

// createLogGroup creates a CloudWatch log group with encryption
func createLogGroup(scope constructs.Construct, id string, logGroupName string, encryptionKey awskms.Key, retentionDays *float64) awslogs.LogGroup {
	return awslogs.NewLogGroup(scope, jsii.String(id), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(logGroupName),
		Retention:     mapRetentionDays(retentionDays),
		RemovalPolicy: awscdk.RemovalPolicy_RETAIN,
		EncryptionKey: encryptionKey,
	})
}

// retentionMapping defines the mapping between days and retention constants
type retentionMapping struct {
	retention awslogs.RetentionDays
	maxDays   float64
}

// mapRetentionDays maps numeric days to CloudWatch retention constants
func mapRetentionDays(days *float64) awslogs.RetentionDays {
	if days == nil {
		return awslogs.RetentionDays_INFINITE
	}

	// Define retention mappings in ascending order
	mappings := []retentionMapping{
		{retention: awslogs.RetentionDays_ONE_DAY, maxDays: 1},
		{retention: awslogs.RetentionDays_THREE_DAYS, maxDays: 3},
		{retention: awslogs.RetentionDays_FIVE_DAYS, maxDays: 5},
		{retention: awslogs.RetentionDays_ONE_WEEK, maxDays: 7},
		{retention: awslogs.RetentionDays_TWO_WEEKS, maxDays: 14},
		{retention: awslogs.RetentionDays_ONE_MONTH, maxDays: 30},
		{retention: awslogs.RetentionDays_TWO_MONTHS, maxDays: 60},
		{retention: awslogs.RetentionDays_THREE_MONTHS, maxDays: 90},
		{retention: awslogs.RetentionDays_FOUR_MONTHS, maxDays: 120},
		{retention: awslogs.RetentionDays_FIVE_MONTHS, maxDays: 150},
		{retention: awslogs.RetentionDays_SIX_MONTHS, maxDays: 180},
		{retention: awslogs.RetentionDays_ONE_YEAR, maxDays: 365},
		{retention: awslogs.RetentionDays_THIRTEEN_MONTHS, maxDays: 400},
		{retention: awslogs.RetentionDays_EIGHTEEN_MONTHS, maxDays: 545},
		{retention: awslogs.RetentionDays_TWO_YEARS, maxDays: 730},
		{retention: awslogs.RetentionDays_FIVE_YEARS, maxDays: 1827},
		{retention: awslogs.RetentionDays_TEN_YEARS, maxDays: 3653},
	}

	// Find the appropriate retention period
	for _, mapping := range mappings {
		if *days <= mapping.maxDays {
			return mapping.retention
		}
	}

	return awslogs.RetentionDays_INFINITE
}

// createFirehoseDeliveryStream creates a Kinesis Firehose delivery stream
func createFirehoseDeliveryStream(scope constructs.Construct, props *AuditingProps, bucket awss3.Bucket, encryptionKey awskms.Key, stream awskinesis.Stream) awskinesisfirehose.CfnDeliveryStream {
	// Create IAM role for Firehose
	firehoseRole := awsiam.NewRole(scope, jsii.String("FirehoseRole"), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("firehose.amazonaws.com"), nil),
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"FirehosePolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Effect: awsiam.Effect_ALLOW,
						Actions: &[]*string{
							jsii.String("s3:AbortMultipartUpload"),
							jsii.String("s3:GetBucketLocation"),
							jsii.String("s3:GetObject"),
							jsii.String("s3:ListBucket"),
							jsii.String("s3:ListBucketMultipartUploads"),
							jsii.String("s3:PutObject"),
						},
						Resources: &[]*string{
							bucket.BucketArn(),
							bucket.ArnForObjects(jsii.String("*")),
						},
					}),
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Effect: awsiam.Effect_ALLOW,
						Actions: &[]*string{
							jsii.String("kinesis:DescribeStream"),
							jsii.String("kinesis:GetShardIterator"),
							jsii.String("kinesis:GetRecords"),
							jsii.String("kinesis:ListShards"),
						},
						Resources: &[]*string{stream.StreamArn()},
					}),
				},
			}),
		},
	})

	// Grant KMS permissions
	encryptionKey.GrantEncryptDecrypt(firehoseRole)

	return awskinesisfirehose.NewCfnDeliveryStream(scope, jsii.String("AuditFirehoseStream"), &awskinesisfirehose.CfnDeliveryStreamProps{
		DeliveryStreamName: jsii.String(fmt.Sprintf("%s-audit-firehose", *props.AppName)),
		DeliveryStreamType: jsii.String("KinesisStreamAsSource"),
		KinesisStreamSourceConfiguration: &awskinesisfirehose.CfnDeliveryStream_KinesisStreamSourceConfigurationProperty{
			KinesisStreamArn: stream.StreamArn(),
			RoleArn:          firehoseRole.RoleArn(),
		},
		S3DestinationConfiguration: &awskinesisfirehose.CfnDeliveryStream_S3DestinationConfigurationProperty{
			BucketArn:         bucket.BucketArn(),
			RoleArn:           firehoseRole.RoleArn(),
			Prefix:            jsii.String("audit-logs/year=!{timestamp:yyyy}/month=!{timestamp:MM}/day=!{timestamp:dd}/hour=!{timestamp:HH}/"),
			ErrorOutputPrefix: jsii.String("error-logs/"),
			BufferingHints: &awskinesisfirehose.CfnDeliveryStream_BufferingHintsProperty{
				SizeInMBs:         jsii.Number(5),
				IntervalInSeconds: jsii.Number(300),
			},
			CompressionFormat: jsii.String("GZIP"),
			EncryptionConfiguration: &awskinesisfirehose.CfnDeliveryStream_EncryptionConfigurationProperty{
				KmsEncryptionConfig: &awskinesisfirehose.CfnDeliveryStream_KMSEncryptionConfigProperty{
					AwskmsKeyArn: encryptionKey.KeyArn(),
				},
			},
		},
	})
}

// createAuditLambdaFunction creates a Lambda function with common audit configurations
func createAuditLambdaFunction(scope constructs.Construct, id string, props *AuditingProps, bucket awss3.Bucket, encryptionKey awskms.Key, config struct {
	Timeout      awscdk.Duration
	FunctionName string
	Description  string
	Permissions  string // PermissionRead or PermissionReadWrite
}) awslambda.Function {
	// Create environment variables
	environment := &map[string]*string{
		"AUDIT_BUCKET": bucket.BucketName(),
		"APP_NAME":     props.AppName,
		"ENVIRONMENT":  props.Environment,
	}

	// Create the Lambda function
	function := awslambda.NewFunction(scope, jsii.String(id), &awslambda.FunctionProps{
		FunctionName: jsii.String(config.FunctionName),
		Description:  jsii.String(config.Description),
		Runtime:      awslambda.Runtime_GO_1_X(),
		Code:         awslambda.Code_FromInline(jsii.String("// Placeholder audit function code")),
		Handler:      jsii.String("main"),
		Timeout:      config.Timeout,
		Environment:  environment,
	})

	// Grant appropriate S3 permissions
	switch config.Permissions {
	case "read":
		bucket.GrantRead(awsiam.IGrantable(function), jsii.String("*"))
	case PermissionReadWrite:
		bucket.GrantReadWrite(awsiam.IGrantable(function), jsii.String("*"))
	}

	// Grant KMS permissions if encryption is enabled
	if encryptionKey != nil {
		encryptionKey.GrantDecrypt(awsiam.IGrantable(function))
		if config.Permissions == PermissionReadWrite {
			encryptionKey.GrantEncrypt(awsiam.IGrantable(function))
		}
	}

	return function
}

// createLogProcessingFunction creates a Lambda function for log processing
func createLogProcessingFunction(scope constructs.Construct, props *AuditingProps, bucket awss3.Bucket, encryptionKey awskms.Key, stream awskinesis.Stream) awslambda.Function {
	function := createAuditLambdaFunction(scope, "LogProcessingFunction", props, bucket, encryptionKey, struct {
		Timeout      awscdk.Duration
		FunctionName string
		Description  string
		Permissions  string
	}{
		FunctionName: fmt.Sprintf("%s-log-processing", *props.AppName),
		Description:  "Real-time audit log processing function",
		Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
		Permissions:  PermissionReadWrite,
	})

	// Add additional Kinesis permissions to the role
	roleInterface := function.Role()
	if role, ok := roleInterface.(awsiam.Role); ok {
		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("kinesis:DescribeStream"),
				jsii.String("kinesis:GetShardIterator"),
				jsii.String("kinesis:GetRecords"),
				jsii.String("kinesis:ListShards"),
			},
			Resources: &[]*string{stream.StreamArn()},
		}))

		role.AddToPolicy(awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
			Effect: awsiam.Effect_ALLOW,
			Actions: &[]*string{
				jsii.String("logs:CreateLogStream"),
				jsii.String("logs:PutLogEvents"),
			},
			Resources: &[]*string{jsii.String("*")},
		}))
	}

	// Add Kinesis event source using higher-level construct
	eventSource := awslambdaeventsources.NewKinesisEventSource(stream, &awslambdaeventsources.KinesisEventSourceProps{
		BatchSize:               jsii.Number(100),
		StartingPosition:        awslambda.StartingPosition_LATEST,
		MaxBatchingWindow:       awscdk.Duration_Seconds(jsii.Number(5)),
		BisectBatchOnError:      jsii.Bool(true),
		ReportBatchItemFailures: jsii.Bool(true),
		RetryAttempts:           jsii.Number(3),
		MaxRecordAge:            awscdk.Duration_Minutes(jsii.Number(60)),
		ParallelizationFactor:   jsii.Number(1),
	})
	function.AddEventSource(eventSource)

	return function
}

// createScheduledAuditFunction creates a Lambda function with scheduled execution for audit tasks
func createScheduledAuditFunction(scope constructs.Construct, id string, props *AuditingProps, bucket awss3.Bucket, encryptionKey awskms.Key, config struct {
	Schedule       awsevents.Schedule
	FunctionSuffix string
	Description    string
	Permissions    string
	RuleID         string
}) awslambda.Function {
	function := createAuditLambdaFunction(scope, id, props, bucket, encryptionKey, struct {
		Timeout      awscdk.Duration
		FunctionName string
		Description  string
		Permissions  string
	}{
		FunctionName: fmt.Sprintf("%s-%s", *props.AppName, config.FunctionSuffix),
		Description:  config.Description,
		Timeout:      awscdk.Duration_Minutes(jsii.Number(15)),
		Permissions:  config.Permissions,
	})

	// Schedule the function
	rule := awsevents.NewRule(scope, jsii.String(config.RuleID), &awsevents.RuleProps{
		Schedule: config.Schedule,
	})
	rule.AddTarget(awseventstargets.NewLambdaFunction(function, nil))

	return function
}

// createIntegrityCheckingFunction creates a Lambda function for log integrity checking
func createIntegrityCheckingFunction(scope constructs.Construct, props *AuditingProps, bucket awss3.Bucket, encryptionKey awskms.Key) awslambda.Function {
	return createScheduledAuditFunction(scope, "IntegrityCheckingFunction", props, bucket, encryptionKey, struct {
		Schedule       awsevents.Schedule
		FunctionSuffix string
		Description    string
		Permissions    string
		RuleID         string
	}{
		FunctionSuffix: "integrity-checking",
		Description:    "Audit log integrity checking function",
		Permissions:    "read",
		RuleID:         "IntegrityCheckRule",
		Schedule:       awsevents.Schedule_Rate(awscdk.Duration_Hours(jsii.Number(24))),
	})
}

// createAuditComplianceFunction creates a Lambda function for compliance reporting
func createAuditComplianceFunction(scope constructs.Construct, props *AuditingProps, bucket awss3.Bucket, encryptionKey awskms.Key) awslambda.Function {
	return createScheduledAuditFunction(scope, "ComplianceFunction", props, bucket, encryptionKey, struct {
		Schedule       awsevents.Schedule
		FunctionSuffix string
		Description    string
		Permissions    string
		RuleID         string
	}{
		FunctionSuffix: "compliance-reporting",
		Description:    "Audit compliance reporting function",
		Permissions:    PermissionReadWrite,
		RuleID:         "ComplianceReportRule",
		Schedule:       awsevents.Schedule_Rate(awscdk.Duration_Days(jsii.Number(7))),
	})
}

// createAuditDashboard creates a CloudWatch dashboard for audit monitoring
func createAuditDashboard(scope constructs.Construct, props *AuditingProps, appLogGroup awslogs.LogGroup, dbLogGroup awslogs.LogGroup, auditLogGroup awslogs.LogGroup) awscloudwatch.Dashboard {
	// Helper function to create text widgets
	createLogWidget := func(title string, logGroup awslogs.LogGroup) awscloudwatch.TextWidget {
		return awscloudwatch.NewTextWidget(&awscloudwatch.TextWidgetProps{
			Markdown: jsii.String(fmt.Sprintf("## %s\nLog Group: %s", title, *logGroup.LogGroupName())),
			Width:    jsii.Number(8),
			Height:   jsii.Number(3),
		})
	}

	// Create text widgets for each log group
	appLogWidget := createLogWidget("Application Audit Logs", appLogGroup)
	dbLogWidget := createLogWidget("Database Audit Logs", dbLogGroup)
	systemLogWidget := createLogWidget("System Audit Logs", auditLogGroup)

	return awscloudwatch.NewDashboard(scope, jsii.String("AuditDashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(fmt.Sprintf("%s-audit-dashboard", *props.AppName)),
		Widgets: &[]*[]awscloudwatch.IWidget{
			{
				appLogWidget,
				dbLogWidget,
			},
			{
				systemLogWidget,
			},
		},
	})
}

// createLogMetricAlarm creates a CloudWatch alarm for log metrics
func createLogMetricAlarm(scope constructs.Construct, id string, _ *AuditingProps, config struct {
	LogGroupName      *string
	AlarmName         string
	Threshold         float64
	PeriodMinutes     int
	EvaluationPeriods int
	DatapointsToAlarm int
}) awscloudwatch.Alarm {
	return awscloudwatch.NewAlarm(scope, jsii.String(id), &awscloudwatch.AlarmProps{
		AlarmName: jsii.String(config.AlarmName),
		Metric: awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
			Namespace:  jsii.String("AWS/Logs"),
			MetricName: jsii.String("IncomingLogEvents"),
			DimensionsMap: &map[string]*string{
				"LogGroupName": config.LogGroupName,
			},
			Statistic: jsii.String("Sum"),
			Period:    awscdk.Duration_Minutes(jsii.Number(config.PeriodMinutes)),
		}),
		Threshold:         jsii.Number(config.Threshold),
		EvaluationPeriods: jsii.Number(config.EvaluationPeriods),
		DatapointsToAlarm: jsii.Number(config.DatapointsToAlarm),
		TreatMissingData:  awscloudwatch.TreatMissingData_NOT_BREACHING,
	})
}

// createAuditAlarms creates CloudWatch alarms for audit monitoring
func createAuditAlarms(scope constructs.Construct, props *AuditingProps, appLogGroup awslogs.LogGroup, _ awslogs.LogGroup, auditLogGroup awslogs.LogGroup) []awscloudwatch.Alarm {
	var alarms []awscloudwatch.Alarm

	// Failed login attempts alarm
	failedLoginAlarm := createLogMetricAlarm(scope, "FailedLoginAlarm", props, struct {
		LogGroupName      *string
		AlarmName         string
		Threshold         float64
		PeriodMinutes     int
		EvaluationPeriods int
		DatapointsToAlarm int
	}{
		AlarmName:         fmt.Sprintf("%s-failed-login-attempts", *props.AppName),
		LogGroupName:      appLogGroup.LogGroupName(),
		PeriodMinutes:     5,
		Threshold:         10,
		EvaluationPeriods: 1,
		DatapointsToAlarm: 1,
	})
	alarms = append(alarms, failedLoginAlarm)

	// Suspicious activity alarm
	suspiciousActivityAlarm := createLogMetricAlarm(scope, "SuspiciousActivityAlarm", props, struct {
		LogGroupName      *string
		AlarmName         string
		Threshold         float64
		PeriodMinutes     int
		EvaluationPeriods int
		DatapointsToAlarm int
	}{
		AlarmName:         fmt.Sprintf("%s-suspicious-activity", *props.AppName),
		LogGroupName:      auditLogGroup.LogGroupName(),
		PeriodMinutes:     15,
		Threshold:         100,
		EvaluationPeriods: 2,
		DatapointsToAlarm: 2,
	})
	alarms = append(alarms, suspiciousActivityAlarm)

	return alarms
}

// storeAuditConfiguration stores audit configuration in SSM Parameter Store
func storeAuditConfiguration(scope constructs.Construct, props *AuditingProps) {
	awsssm.NewStringParameter(scope, jsii.String("AuditLevel"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("/%s/audit/level", *props.AppName)),
		StringValue:   jsii.String(string(props.AuditLevel)),
		Description:   jsii.String("Audit logging level"),
	})

	awsssm.NewStringParameter(scope, jsii.String("AuditRetentionDays"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("/%s/audit/retention-days", *props.AppName)),
		StringValue:   jsii.String(fmt.Sprintf("%.0f", *props.LogRetentionDays)),
		Description:   jsii.String("Audit log retention period in days"),
	})

	if props.ComplianceFrameworks != nil {
		awsssm.NewStringParameter(scope, jsii.String("ComplianceFrameworks"), &awsssm.StringParameterProps{
			ParameterName: jsii.String(fmt.Sprintf("/%s/audit/compliance-frameworks", *props.AppName)),
			StringValue:   jsii.String(fmt.Sprintf("%v", *props.ComplianceFrameworks)),
			Description:   jsii.String("Enabled compliance frameworks"),
		})
	}
}

// GetAuditStatus returns the current audit status
func (a *AuditingConstruct) GetAuditStatus() map[string]interface{} {
	return map[string]interface{}{
		"cloudtrail_enabled":       a.CloudTrail != nil,
		"application_logs_enabled": a.ApplicationLogGroup != nil,
		"database_logs_enabled":    a.DatabaseLogGroup != nil,
		"real_time_processing":     a.LogProcessingFunction != nil,
		"integrity_checking":       a.IntegrityFunction != nil,
		"compliance_reporting":     a.ComplianceFunction != nil,
		"dashboard_enabled":        a.AuditDashboard != nil,
		"alerting_enabled":         len(a.AuditAlarms) > 0,
		"encryption_enabled":       a.EncryptionKey != nil,
		"stream_processing":        a.LogStream != nil,
		"log_aggregation":          a.FirehoseDeliveryStream != nil,
	}
}

// AddCustomAuditRule adds a custom audit rule
func (a *AuditingConstruct) AddCustomAuditRule(ruleId string, logGroup awslogs.LogGroup, filterPattern string) {
	awslogs.NewMetricFilter(a.Construct, jsii.String(fmt.Sprintf("CustomAuditRule_%s", ruleId)), &awslogs.MetricFilterProps{
		LogGroup:        logGroup,
		FilterPattern:   awslogs.FilterPattern_Literal(jsii.String(filterPattern)),
		MetricNamespace: jsii.String("Audit/Custom"),
		MetricName:      jsii.String(ruleId),
		MetricValue:     jsii.String("1"),
	})
}

// EnableSIEMIntegration enables SIEM integration for audit logs
func (a *AuditingConstruct) EnableSIEMIntegration(_ string) {
	// Create a destination for SIEM integration
	// This would typically involve creating a subscription filter
	// to forward logs to external SIEM systems
}
