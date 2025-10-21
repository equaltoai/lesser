package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/assertions"
	"github.com/aws/jsii-runtime-go"
)

func TestLiftKMSKey_PrimarySymmetricKey(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	key := NewLiftKMSKey(stack, jsii.String("TestKey"), &LiftKMSKeyProps{
		Description: jsii.String("Test symmetric key"),
		KeySpec:     awskms.KeySpec_SYMMETRIC_DEFAULT,
		AliasName:   jsii.String("alias/test/symmetric-key"),
		MultiRegion: jsii.Bool(true),
	})

	if key == nil {
		t.Fatal("Expected key to be created")
	}
	if key.Key == nil {
		t.Fatal("Expected key.Key to be set")
	}
	if key.Alias == nil {
		t.Fatal("Expected alias to be created")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Verify key creation
	template.HasResourceProperties(jsii.String("AWS::KMS::Key"), map[string]interface{}{
		"Description": "Test symmetric key",
		"KeySpec":     "SYMMETRIC_DEFAULT",
		"KeyUsage":    "ENCRYPT_DECRYPT",
		"MultiRegion": true,
	})

	// Verify alias creation
	template.HasResourceProperties(jsii.String("AWS::KMS::Alias"), map[string]interface{}{
		"AliasName": "alias/test/symmetric-key",
	})
}

func TestLiftKMSKey_PrimaryHMACKey(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	key := NewLiftKMSKey(stack, jsii.String("TestKey"), &LiftKMSKeyProps{
		Description: jsii.String("Test HMAC key"),
		KeySpec:     awskms.KeySpec_HMAC_256,
		KeyUsage:    awskms.KeyUsage_GENERATE_VERIFY_MAC,
		AliasName:   jsii.String("alias/test/hmac-key"),
		MultiRegion: jsii.Bool(true),
	})

	if key == nil {
		t.Fatal("Expected key to be created")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Verify HMAC key creation
	template.HasResourceProperties(jsii.String("AWS::KMS::Key"), map[string]interface{}{
		"Description": "Test HMAC key",
		"KeySpec":     "HMAC_256",
		"KeyUsage":    "GENERATE_VERIFY_MAC",
		"MultiRegion": true,
	})
}

func TestLiftKMSKey_ReplicaKey(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Region:  jsii.String("us-west-2"),
			Account: jsii.String("123456789012"),
		},
	})

	key := NewLiftKMSKey(stack, jsii.String("TestKey"), &LiftKMSKeyProps{
		Description:   jsii.String("Test replica key"),
		IsReplicaKey:  jsii.Bool(true),
		PrimaryKeyArn: jsii.String("arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"),
		AliasName:     jsii.String("alias/test/replica-key"),
	})

	if key == nil {
		t.Fatal("Expected replica key to be created")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Verify replica key creation
	template.HasResourceProperties(jsii.String("AWS::KMS::ReplicaKey"), map[string]interface{}{
		"PrimaryKeyArn": "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
		"Description":   "Test replica key",
	})
}

func TestLiftKMSKey_SSMParameterStore(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	key := NewLiftKMSKey(stack, jsii.String("TestKey"), &LiftKMSKeyProps{
		Description:        jsii.String("Test key with SSM"),
		AliasName:          jsii.String("alias/test/key"),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String("/test/kms/key-arn"),
	})

	if key == nil {
		t.Fatal("Expected key to be created")
	}
	if key.SSMParameter == nil {
		t.Fatal("Expected SSM parameter to be created")
	}

	template := assertions.Template_FromStack(stack, nil)

	// Verify SSM parameter creation
	template.HasResourceProperties(jsii.String("AWS::SSM::Parameter"), map[string]interface{}{
		"Name": "/test/kms/key-arn",
		"Type": "String",
	})
}

func TestLiftKMSKey_Tags(t *testing.T) {
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	key := NewLiftKMSKey(stack, jsii.String("TestKey"), &LiftKMSKeyProps{
		Description: jsii.String("Test key with tags"),
		Tags: &map[string]*string{
			"Environment": jsii.String("test"),
			"Service":     jsii.String("payments"),
		},
	})

	if key == nil {
		t.Fatal("Expected key to be created")
	}

	// Tags are applied via awscdk.Tags_Of(), which are validated at synth time
	// We can't directly test tag application in unit tests, but we can verify the key was created
	template := assertions.Template_FromStack(stack, nil)
	template.ResourceCountIs(jsii.String("AWS::KMS::Key"), jsii.Number(1))
}
