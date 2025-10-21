package constructs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
	"github.com/stretchr/testify/assert"
)

// createComplianceFrameworkTestStack creates a test stack for compliance framework testing
func createComplianceFrameworkTestStack(appName string, frameworks []ComplianceFramework) (awscdk.Stack, assertions.Template) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:              jsii.String(appName),
		ComplianceFrameworks: &frameworks,
		EnableConfig:         jsii.Bool(true),
		EnableSecurityHub:    jsii.Bool(true),
	})

	template := assertions.Template_FromStack(stack, nil)
	return stack, template
}

func TestComplianceStack_Creation(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	complianceStack := NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName: jsii.String("test-app"),
		ComplianceFrameworks: &[]ComplianceFramework{
			SOC2,
			HIPAA,
		},
		EnableCloudTrail:        jsii.Bool(true),
		EnableConfig:            jsii.Bool(true),
		EnableGuardDuty:         jsii.Bool(true),
		EnableSecurityHub:       jsii.Bool(true),
		EnableEncryption:        jsii.Bool(true),
		DataRetentionDays:       jsii.Number(2555),
		EnableComplianceReports: jsii.Bool(true),
		Environment:             jsii.String("test"),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify KMS key is created
	template.HasResourceProperties(jsii.String("AWS::KMS::Key"), map[string]interface{}{
		"Description":       "Compliance encryption key for test-app",
		"EnableKeyRotation": true,
	})

	// Verify S3 bucket is created with encryption and lifecycle
	template.HasResourceProperties(jsii.String("AWS::S3::Bucket"), map[string]interface{}{
		"BucketEncryption": map[string]interface{}{
			"ServerSideEncryptionConfiguration": []interface{}{
				map[string]interface{}{
					"ServerSideEncryptionByDefault": map[string]interface{}{
						"SSEAlgorithm": "aws:kms",
					},
				},
			},
		},
		"PublicAccessBlockConfiguration": map[string]interface{}{
			"BlockPublicAcls":       true,
			"BlockPublicPolicy":     true,
			"IgnorePublicAcls":      true,
			"RestrictPublicBuckets": true,
		},
		"VersioningConfiguration": map[string]interface{}{
			"Status": "Enabled",
		},
	})

	// Verify CloudTrail is created
	template.HasResourceProperties(jsii.String("AWS::CloudTrail::Trail"), map[string]interface{}{
		"IncludeGlobalServiceEvents": true,
		"IsMultiRegionTrail":         true,
		"EnableLogFileValidation":    true,
	})

	// Verify Config Configuration Recorder is created
	template.HasResourceProperties(jsii.String("AWS::Config::ConfigurationRecorder"), map[string]interface{}{
		"RecordingGroup": map[string]interface{}{
			"AllSupported": true,
		},
	})

	// Verify GuardDuty detector is created
	template.HasResourceProperties(jsii.String("AWS::GuardDuty::Detector"), map[string]interface{}{
		"Enable":                     true,
		"FindingPublishingFrequency": "FIFTEEN_MINUTES",
	})

	// Verify Security Hub is created
	template.HasResourceProperties(jsii.String("AWS::SecurityHub::Hub"), map[string]interface{}{
		"EnableDefaultStandards": true,
	})

	// Verify CloudWatch Log Group is created
	template.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), map[string]interface{}{
		"LogGroupName": "/aws/compliance/test-app",
	})

	// Verify compliance function is created
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), map[string]interface{}{
		"Handler": "bootstrap",
		"Runtime": "provided.al2",
	})

	// Verify SSM parameters are created
	template.HasResourceProperties(jsii.String("AWS::SSM::Parameter"), map[string]interface{}{
		"Type": "String",
	})

	// Verify compliance stack properties
	assert.NotNil(t, complianceStack.CloudTrail)
	assert.NotNil(t, complianceStack.ConfigRecorder)
	assert.NotNil(t, complianceStack.GuardDutyDetector)
	assert.NotNil(t, complianceStack.SecurityHub)
	assert.NotNil(t, complianceStack.ComplianceBucket)
	assert.NotNil(t, complianceStack.EncryptionKey)
	assert.NotNil(t, complianceStack.ComplianceLogGroup)
	assert.NotNil(t, complianceStack.ComplianceFunction)
}

func TestComplianceStack_MinimalConfiguration(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	complianceStack := NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:                 jsii.String("minimal-app"),
		EnableCloudTrail:        jsii.Bool(false),
		EnableConfig:            jsii.Bool(false),
		EnableGuardDuty:         jsii.Bool(false),
		EnableSecurityHub:       jsii.Bool(false),
		EnableEncryption:        jsii.Bool(false),
		EnableComplianceReports: jsii.Bool(false),
		EnableAutomation:        jsii.Bool(false),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify minimal resources are created
	template.ResourceCountIs(jsii.String("AWS::CloudTrail::Trail"), jsii.Number(0))
	template.ResourceCountIs(jsii.String("AWS::Config::ConfigurationRecorder"), jsii.Number(0))
	template.ResourceCountIs(jsii.String("AWS::GuardDuty::Detector"), jsii.Number(0))
	template.ResourceCountIs(jsii.String("AWS::SecurityHub::Hub"), jsii.Number(0))
	template.ResourceCountIs(jsii.String("AWS::Lambda::Function"), jsii.Number(0))

	// S3 bucket should still be created for compliance data
	template.ResourceCountIs(jsii.String("AWS::S3::Bucket"), jsii.Number(1))
	template.ResourceCountIs(jsii.String("AWS::Logs::LogGroup"), jsii.Number(1))

	// Verify compliance stack properties
	assert.Nil(t, complianceStack.CloudTrail)
	assert.Nil(t, complianceStack.ConfigRecorder)
	assert.Nil(t, complianceStack.GuardDutyDetector)
	assert.Nil(t, complianceStack.SecurityHub)
	assert.NotNil(t, complianceStack.ComplianceBucket)
	assert.NotNil(t, complianceStack.ComplianceLogGroup)
	assert.Nil(t, complianceStack.ComplianceFunction)
}

func TestComplianceStack_WithExistingResources(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create existing resources
	existingBucket := awss3.NewBucket(stack, jsii.String("ExistingBucket"), &awss3.BucketProps{
		BucketName: jsii.String("existing-compliance-bucket"),
	})

	// WHEN
	complianceStack := NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:          jsii.String("test-app"),
		ComplianceBucket: existingBucket,
		EnableCloudTrail: jsii.Bool(true),
		EnableConfig:     jsii.Bool(true),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify existing bucket is used (only 1 bucket total)
	template.ResourceCountIs(jsii.String("AWS::S3::Bucket"), jsii.Number(1))

	// Verify compliance stack uses existing bucket
	assert.Equal(t, existingBucket, complianceStack.ComplianceBucket)
}

func TestComplianceStack_SOC2Framework(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName: jsii.String("soc2-app"),
		ComplianceFrameworks: &[]ComplianceFramework{
			SOC2,
		},
		EnableConfig: jsii.Bool(true),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify SOC2-specific Config rules are created
	template.HasResourceProperties(jsii.String("AWS::Config::ConfigRule"), map[string]interface{}{
		"Source": map[string]interface{}{
			"Owner": "AWS",
		},
	})

	// Should have Config rules for SOC2
	template.ResourceCountIs(jsii.String("AWS::Config::ConfigRule"), jsii.Number(1))
}

func TestComplianceStack_HIPAAFramework(t *testing.T) {
	// GIVEN, WHEN
	_, template := createComplianceFrameworkTestStack("hipaa-app", []ComplianceFramework{HIPAA})

	// THEN
	// Verify HIPAA-specific Config rules are created
	template.HasResourceProperties(jsii.String("AWS::Config::ConfigRule"), map[string]interface{}{
		"Source": map[string]interface{}{
			"Owner": "AWS",
		},
	})

	// Verify Security Hub standards subscription
	template.HasResourceProperties(jsii.String("AWS::SecurityHub::Standard"), map[string]interface{}{
		"StandardsArn": assertions.Match_StringLikeRegexp(jsii.String(".*aws-foundational-security.*")),
	})
}

func TestComplianceStack_PCIDSSFramework(t *testing.T) {
	// GIVEN, WHEN
	_, template := createComplianceFrameworkTestStack("pci-app", []ComplianceFramework{PCI_DSS})

	// THEN
	// Verify PCI DSS-specific Config rules are created
	template.HasResourceProperties(jsii.String("AWS::Config::ConfigRule"), map[string]interface{}{
		"Source": map[string]interface{}{
			"Owner": "AWS",
		},
	})

	// Verify PCI DSS Security Hub standard
	template.HasResourceProperties(jsii.String("AWS::SecurityHub::Standard"), map[string]interface{}{
		"StandardsArn": assertions.Match_StringLikeRegexp(jsii.String(".*pci-dss.*")),
	})
}

func TestComplianceStack_DataRetentionPolicy(_ *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:           jsii.String("retention-app"),
		DataRetentionDays: jsii.Number(365), // 1 year
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify S3 bucket lifecycle policy
	template.HasResourceProperties(jsii.String("AWS::S3::Bucket"), map[string]interface{}{
		"LifecycleConfiguration": map[string]interface{}{
			"Rules": []interface{}{
				map[string]interface{}{
					"Id":               "ComplianceDataLifecycle",
					"Status":           "Enabled",
					"ExpirationInDays": 365,
				},
			},
		},
	})

	// Verify CloudWatch log group retention
	template.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), map[string]interface{}{
		"RetentionInDays": 365,
	})
}

func TestComplianceStack_EncryptionConfiguration(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	complianceStack := NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:          jsii.String("encryption-app"),
		EnableEncryption: jsii.Bool(true),
		EnableCloudTrail: jsii.Bool(true),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify KMS key is created
	template.HasResourceProperties(jsii.String("AWS::KMS::Key"), map[string]interface{}{
		"EnableKeyRotation": true,
	})

	// Verify S3 bucket uses KMS encryption
	template.HasResourceProperties(jsii.String("AWS::S3::Bucket"), map[string]interface{}{
		"BucketEncryption": map[string]interface{}{
			"ServerSideEncryptionConfiguration": []interface{}{
				map[string]interface{}{
					"ServerSideEncryptionByDefault": map[string]interface{}{
						"SSEAlgorithm": "aws:kms",
					},
				},
			},
		},
	})

	// Verify CloudTrail is created (KMS encryption handled separately)
	template.HasResourceProperties(jsii.String("AWS::CloudTrail::Trail"), map[string]interface{}{
		"EnableLogFileValidation": true,
	})

	// Verify CloudWatch log group is created (KMS encryption handled separately)
	template.HasResourceProperties(jsii.String("AWS::Logs::LogGroup"), map[string]interface{}{
		"LogGroupName": "/aws/compliance/encryption-app",
	})

	// Verify encryption key is accessible
	assert.NotNil(t, complianceStack.EncryptionKey)
}

func TestComplianceStack_ComplianceStatus(t *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	complianceStack := NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:           jsii.String("status-app"),
		EnableCloudTrail:  jsii.Bool(true),
		EnableConfig:      jsii.Bool(true),
		EnableGuardDuty:   jsii.Bool(true),
		EnableSecurityHub: jsii.Bool(true),
		EnableEncryption:  jsii.Bool(true),
	})

	// WHEN
	status := complianceStack.GetComplianceStatus()

	// THEN
	cloudtrailEnabledVal := status["cloudtrail_enabled"]
	cloudtrailEnabled, ok := cloudtrailEnabledVal.(bool)
	assert.True(t, ok && cloudtrailEnabled)

	configEnabledVal := status["config_enabled"]
	configEnabled, ok := configEnabledVal.(bool)
	assert.True(t, ok && configEnabled)

	guarddutyEnabledVal := status["guardduty_enabled"]
	guarddutyEnabled, ok := guarddutyEnabledVal.(bool)
	assert.True(t, ok && guarddutyEnabled)

	securityhubEnabledVal := status["securityhub_enabled"]
	securityhubEnabled, ok := securityhubEnabledVal.(bool)
	assert.True(t, ok && securityhubEnabled)

	encryptionEnabledVal := status["encryption_enabled"]
	encryptionEnabled, ok := encryptionEnabledVal.(bool)
	assert.True(t, ok && encryptionEnabled)

	functionEnabledVal := status["function_enabled"]
	functionEnabled, ok := functionEnabledVal.(bool)
	assert.True(t, ok && functionEnabled)
}

func TestComplianceStack_AddComplianceRule(_ *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	complianceStack := NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:      jsii.String("add-rule-app"),
		EnableConfig: jsii.Bool(true),
	})

	// WHEN
	complianceStack.AddComplianceRule("AdditionalRule", "ADDITIONAL_RULE_NAME")

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify additional Config rule was created
	template.HasResourceProperties(jsii.String("AWS::Config::ConfigRule"), map[string]interface{}{
		"ConfigRuleName": "AdditionalRule-rule",
		"Source": map[string]interface{}{
			"Owner": "AWS",
		},
	})
}

func TestComplianceStack_MultipleFrameworks(_ *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName: jsii.String("multi-framework-app"),
		ComplianceFrameworks: &[]ComplianceFramework{
			SOC2,
			HIPAA,
			PCI_DSS,
			FedRAMP,
		},
		EnableConfig:      jsii.Bool(true),
		EnableSecurityHub: jsii.Bool(true),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify multiple standards are created
	template.ResourceCountIs(jsii.String("AWS::SecurityHub::Standard"), jsii.Number(4))

	// Verify Config recorder is created for multiple frameworks
	template.ResourceCountIs(jsii.String("AWS::Config::ConfigurationRecorder"), jsii.Number(1))
}

func TestComplianceStack_OrganizationSettings(_ *testing.T) {
	// GIVEN
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// WHEN
	NewComplianceStack(stack, "TestComplianceStack", &ComplianceStackProps{
		AppName:        jsii.String("org-app"),
		OrganizationId: jsii.String("o-1234567890"),
		EnableConfig:   jsii.Bool(true),
	})

	// THEN
	template := assertions.Template_FromStack(stack, nil)

	// Verify organization-specific resources are created
	template.HasResourceProperties(jsii.String("AWS::Config::ConfigurationRecorder"), map[string]interface{}{
		"RecordingGroup": map[string]interface{}{
			"AllSupported": true,
		},
	})
}

// Benchmark tests
func BenchmarkComplianceStack_Creation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		app := awscdk.NewApp(nil)
		stack := awscdk.NewStack(app, jsii.String("BenchmarkStack"), nil)

		NewComplianceStack(stack, "BenchmarkComplianceStack", &ComplianceStackProps{
			AppName: jsii.String("benchmark-app"),
			ComplianceFrameworks: &[]ComplianceFramework{
				SOC2,
				HIPAA,
			},
			EnableCloudTrail:  jsii.Bool(true),
			EnableConfig:      jsii.Bool(true),
			EnableGuardDuty:   jsii.Bool(true),
			EnableSecurityHub: jsii.Bool(true),
			EnableEncryption:  jsii.Bool(true),
		})
	}
}

func BenchmarkComplianceStack_AddRule(b *testing.B) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("BenchmarkStack"), nil)

	complianceStack := NewComplianceStack(stack, "BenchmarkComplianceStack", &ComplianceStackProps{
		AppName:      jsii.String("benchmark-app"),
		EnableConfig: jsii.Bool(true),
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		complianceStack.AddComplianceRule("BenchmarkRule", "BENCHMARK_RULE_NAME")
	}
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lift-lambda-dist")
	if err != nil {
		panic(err)
	}

	bootstrapPath := filepath.Join(dir, "bootstrap")
	const bootstrapScript = "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(bootstrapPath, []byte(bootstrapScript), 0o755); err != nil {
		panic(err)
	}

	if err := os.Setenv(lambdaDistEnvVar, dir); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
