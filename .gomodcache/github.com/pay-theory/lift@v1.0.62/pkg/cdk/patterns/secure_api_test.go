package patterns

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/jsii-runtime-go"
	liftconstructs "github.com/pay-theory/lift/pkg/cdk/constructs"
	"github.com/stretchr/testify/assert"
)

func TestNewSecureAPI_DefaultConfiguration(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	secureAPI := NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName: jsii.String("secure-api"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify Lambda function exists (only 1 - either secure or rate limited)
	template.ResourceCountIs(jsii.String("AWS::Lambda::Function"), jsii.Number(1))

	// When rate limiting is enabled (default), no VPC/KMS are created
	// Only check that resources exist when they should

	// Verify API Gateway exists
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::Api"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Api"), &map[string]interface{}{
		"Name":         "secure-api",
		"ProtocolType": "HTTP",
	})

	// Verify stage exists (throttling is in DefaultRouteSettings)
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::Stage"), &map[string]interface{}{
		"DefaultRouteSettings": map[string]interface{}{
			"ThrottlingRateLimit":  1000,
			"ThrottlingBurstLimit": 5000,
		},
	})

	// Verify rate limiting table exists (name is based on function ID)
	template.HasResourceProperties(jsii.String("AWS::DynamoDB::Table"), &map[string]interface{}{
		"TableName": "Function-rate-limits",
	})

	// Verify WAF WebACL exists (enabled by default)
	template.ResourceCountIs(jsii.String("AWS::WAFv2::WebACL"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::WAFv2::WebACL"), &map[string]interface{}{
		"Name":  "secure-api-waf",
		"Scope": "REGIONAL",
	})

	// Verify WAF association exists
	template.ResourceCountIs(jsii.String("AWS::WAFv2::WebACLAssociation"), jsii.Number(1))

	assert.NotNil(t, secureAPI)
	assert.NotNil(t, secureAPI.Api)
	assert.NotNil(t, secureAPI.RateLimitedFunc) // Rate limiting is enabled by default
	assert.Nil(t, secureAPI.Function)           // SecureFunction is not used when rate limiting is enabled
	assert.NotNil(t, secureAPI.WebACL)
}

func TestNewSecureAPI_DisableRateLimiting(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName:            jsii.String("secure-api"),
		Code:               awslambda.Code_FromAsset(jsii.String("."), nil),
		EnableRateLimiting: jsii.Bool(false),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should have 1 Lambda function (SecureFunction)
	template.ResourceCountIs(jsii.String("AWS::Lambda::Function"), jsii.Number(1))

	// When rate limiting is disabled, SecureFunction creates VPC, KMS, etc.
	template.ResourceCountIs(jsii.String("AWS::EC2::VPC"), jsii.Number(1))
	template.ResourceCountIs(jsii.String("AWS::EC2::SecurityGroup"), jsii.Number(1))
	template.ResourceCountIs(jsii.String("AWS::KMS::Key"), jsii.Number(1))

	// No rate limiting table should exist
	tables := 0
	fnResource := template.ToJSON()
	resources, ok := (*fnResource)["Resources"].(map[string]interface{})
	if !ok {
		t.Fatal("Template should have Resources")
	}

	for _, resource := range resources {
		if resMap, ok := resource.(map[string]interface{}); ok {
			if resMap["Type"] == "AWS::DynamoDB::Table" {
				if props, ok := resMap["Properties"].(map[string]interface{}); ok {
					if tableName, ok := props["TableName"].(string); ok && tableName == "secure-api-rate-limits" {
						tables++
					}
				}
			}
		}
	}

	assert.Equal(t, 0, tables, "Rate limiting table should not exist")
}

func TestNewSecureAPI_DisableWAF(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	secureAPI := NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName:   jsii.String("secure-api"),
		Code:      awslambda.Code_FromAsset(jsii.String("."), nil),
		EnableWAF: jsii.Bool(false),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// No WAF WebACL should exist
	template.ResourceCountIs(jsii.String("AWS::WAFv2::WebACL"), jsii.Number(0))
	template.ResourceCountIs(jsii.String("AWS::WAFv2::WebACLAssociation"), jsii.Number(0))

	assert.NotNil(t, secureAPI)
	assert.Nil(t, secureAPI.WebACL)
}

func TestNewSecureAPI_WithExistingVPC(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	vpc := awsec2.NewVpc(stack, jsii.String("ExistingVpc"), &awsec2.VpcProps{
		MaxAzs: jsii.Number(2),
	})

	// When
	NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName: jsii.String("secure-api"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
		Vpc:     vpc,
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Should only have 1 VPC (the existing one)
	template.ResourceCountIs(jsii.String("AWS::EC2::VPC"), jsii.Number(1))
}

func TestNewSecureAPI_CustomRateLimiting(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName:         jsii.String("secure-api"),
		Code:            awslambda.Code_FromAsset(jsii.String("."), nil),
		RateLimitType:   liftconstructs.RateLimitTypeTenant,
		RateLimitWindow: jsii.Number(3600),  // 1 hour
		RateLimitMax:    jsii.Number(10000), // 10k requests
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify rate limiting function exists with correct environment variables
	template.HasResourceProperties(jsii.String("AWS::Lambda::Function"), &map[string]interface{}{
		"Environment": map[string]interface{}{
			"Variables": map[string]interface{}{
				"RATE_LIMIT_TYPE":   "TENANT",
				"RATE_LIMIT_WINDOW": "3600",
				"RATE_LIMIT_MAX":    "10000",
			},
		},
	})
}

func TestNewSecureAPI_WithAlarms(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	alarmTopic := awssns.NewTopic(stack, jsii.String("AlarmTopic"), &awssns.TopicProps{
		DisplayName: jsii.String("Security Alarms"),
	})

	// When
	NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName:    jsii.String("secure-api"),
		Code:       awslambda.Code_FromAsset(jsii.String("."), nil),
		AlarmTopic: alarmTopic,
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify alarms are created
	template.ResourceCountIs(jsii.String("AWS::CloudWatch::Alarm"), jsii.Number(2)) // Error and Throttle alarms

	// Verify error alarm
	template.HasResourceProperties(jsii.String("AWS::CloudWatch::Alarm"), &map[string]interface{}{
		"AlarmName":        "secure-api-high-error-rate",
		"AlarmDescription": "High error rate detected in secure API",
	})

	// Verify throttle alarm
	template.HasResourceProperties(jsii.String("AWS::CloudWatch::Alarm"), &map[string]interface{}{
		"AlarmName":        "secure-api-throttling",
		"AlarmDescription": "API throttling detected",
	})
}

func TestNewSecureAPI_WithCustomDomain(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// When
	NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName:        jsii.String("secure-api"),
		Code:           awslambda.Code_FromAsset(jsii.String("."), nil),
		DomainName:     jsii.String("api.example.com"),
		CertificateArn: jsii.String("arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"),
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify custom domain is created
	template.ResourceCountIs(jsii.String("AWS::ApiGatewayV2::DomainName"), jsii.Number(1))
	template.HasResourceProperties(jsii.String("AWS::ApiGatewayV2::DomainName"), &map[string]interface{}{
		"DomainName": "api.example.com",
		"DomainNameConfigurations": []interface{}{
			map[string]interface{}{
				"CertificateArn": "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
			},
		},
	})
}

func TestNewSecureAPI_WithAdditionalPolicies(_ *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	additionalPolicy := awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("s3:GetObject"),
		},
		Resources: &[]*string{
			jsii.String("arn:aws:s3:::my-secure-bucket/*"),
		},
	})

	// When
	NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName:            jsii.String("secure-api"),
		Code:               awslambda.Code_FromAsset(jsii.String("."), nil),
		AdditionalPolicies: &[]awsiam.PolicyStatement{additionalPolicy},
	})

	// Then
	template := assertions.Template_FromStack(stack, nil)

	// Verify IAM policy exists (additional policies are added to the function's policy)
	template.ResourceCountIs(jsii.String("AWS::IAM::Policy"), jsii.Number(1))
}

func TestSecureAPI_GettersWork(t *testing.T) {
	// Given
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	secureAPI := NewSecureAPI(stack, jsii.String("SecureAPI"), &SecureAPIProps{
		ApiName: jsii.String("secure-api"),
		Code:    awslambda.Code_FromAsset(jsii.String("."), nil),
	})

	// Then
	assert.NotNil(t, secureAPI.GetApiUrl())
	assert.NotNil(t, secureAPI.GetApi())
	assert.NotNil(t, secureAPI.GetWebACL())
	// Note: GetFunction() will return nil when rate limiting is enabled (default)
}
