package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/jsii-runtime-go"
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

func main() {
	app := awscdk.NewApp(nil)

	// Get partner and stage from environment
	partner := os.Getenv("PARTNER")
	if partner == "" {
		partner = "testpartner"
	}
	stage := os.Getenv("STAGE")
	if stage == "" {
		stage = "dev"
	}

	// Create primary region stack (us-east-1)
	primaryStack := awscdk.NewStack(app, jsii.String(fmt.Sprintf("KMS-Primary-%s-%s", partner, stage)), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Region:  jsii.String("us-east-1"),
			Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
		},
	})

	// Create multi-region HMAC key in primary region
	hmacKey := liftcdk.NewLiftKMSKey(primaryStack, jsii.String("HMACKey"), &liftcdk.LiftKMSKeyProps{
		Description:        jsii.String("Multi-region HMAC key for payment data hashing"),
		KeySpec:            awskms.KeySpec_HMAC_256,
		KeyUsage:           awskms.KeyUsage_GENERATE_VERIFY_MAC,
		MultiRegion:        jsii.Bool(true),
		AliasName:          jsii.String(fmt.Sprintf("alias/%s/%s/hmac-key-primary", partner, stage)),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String(fmt.Sprintf("/%s/%s/hmac-key-arn", partner, stage)),
		Tags: &map[string]*string{
			"Partner": jsii.String(partner),
			"Stage":   jsii.String(stage),
			"KeyType": jsii.String("HMAC"),
		},
	})

	// Create multi-region token encryption key in primary region
	tokenKey := liftcdk.NewLiftKMSKey(primaryStack, jsii.String("TokenKey"), &liftcdk.LiftKMSKeyProps{
		Description:        jsii.String("Multi-region KMS key for token encryption"),
		KeySpec:            awskms.KeySpec_SYMMETRIC_DEFAULT,
		KeyUsage:           awskms.KeyUsage_ENCRYPT_DECRYPT,
		MultiRegion:        jsii.Bool(true),
		EnableKeyRotation:  jsii.Bool(true),
		AliasName:          jsii.String(fmt.Sprintf("alias/%s/%s/token-key-primary", partner, stage)),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String(fmt.Sprintf("/%s/%s/token-key-arn", partner, stage)),
		Tags: &map[string]*string{
			"Partner": jsii.String(partner),
			"Stage":   jsii.String(stage),
			"KeyType": jsii.String("TokenEncryption"),
		},
	})

	// Create region-specific database encryption key (not multi-region)
	dbKey := liftcdk.NewLiftKMSKey(primaryStack, jsii.String("DbKey"), &liftcdk.LiftKMSKeyProps{
		Description:        jsii.String("KMS key for DynamoDB encryption"),
		KeySpec:            awskms.KeySpec_SYMMETRIC_DEFAULT,
		KeyUsage:           awskms.KeyUsage_ENCRYPT_DECRYPT,
		MultiRegion:        jsii.Bool(false), // Region-specific
		EnableKeyRotation:  jsii.Bool(true),
		AliasName:          jsii.String(fmt.Sprintf("alias/%s/%s/db-key-primary", partner, stage)),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String(fmt.Sprintf("/%s/%s/db-key-arn", partner, stage)),
		Tags: &map[string]*string{
			"Partner": jsii.String(partner),
			"Stage":   jsii.String(stage),
			"KeyType": jsii.String("DatabaseEncryption"),
		},
	})

	// Output primary key ARNs
	awscdk.NewCfnOutput(primaryStack, jsii.String("HMACKeyArn"), &awscdk.CfnOutputProps{
		Value:       hmacKey.GetKeyArn(),
		Description: jsii.String("HMAC Key ARN (Primary)"),
		ExportName:  jsii.String(fmt.Sprintf("%s-%s-hmac-key-arn-primary", partner, stage)),
	})

	awscdk.NewCfnOutput(primaryStack, jsii.String("TokenKeyArn"), &awscdk.CfnOutputProps{
		Value:       tokenKey.GetKeyArn(),
		Description: jsii.String("Token Encryption Key ARN (Primary)"),
		ExportName:  jsii.String(fmt.Sprintf("%s-%s-token-key-arn-primary", partner, stage)),
	})

	awscdk.NewCfnOutput(primaryStack, jsii.String("DbKeyArn"), &awscdk.CfnOutputProps{
		Value:       dbKey.GetKeyArn(),
		Description: jsii.String("Database Encryption Key ARN (Primary)"),
	})

	// Create secondary region stack (us-west-2)
	secondaryStack := awscdk.NewStack(app, jsii.String(fmt.Sprintf("KMS-Secondary-%s-%s", partner, stage)), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Region:  jsii.String("us-west-2"),
			Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
		},
	})

	// Create HMAC replica key in secondary region
	// Note: We need to use Fn.ImportValue or pass the ARN manually
	hmacReplicaKey := liftcdk.NewLiftKMSKey(secondaryStack, jsii.String("HMACReplicaKey"), &liftcdk.LiftKMSKeyProps{
		Description:        jsii.String("Multi-region HMAC key replica for payment data hashing"),
		IsReplicaKey:       jsii.Bool(true),
		PrimaryKeyArn:      hmacKey.GetKeyArn(), // Cross-stack reference
		AliasName:          jsii.String(fmt.Sprintf("alias/%s/%s/hmac-key-secondary", partner, stage)),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String(fmt.Sprintf("/%s/%s/hmac-key-arn-secondary", partner, stage)),
		Tags: &map[string]*string{
			"Partner": jsii.String(partner),
			"Stage":   jsii.String(stage),
			"KeyType": jsii.String("HMAC"),
			"Replica": jsii.String("true"),
		},
	})

	// Create token encryption replica key in secondary region
	tokenReplicaKey := liftcdk.NewLiftKMSKey(secondaryStack, jsii.String("TokenReplicaKey"), &liftcdk.LiftKMSKeyProps{
		Description:        jsii.String("Multi-region token encryption key replica"),
		IsReplicaKey:       jsii.Bool(true),
		PrimaryKeyArn:      tokenKey.GetKeyArn(), // Cross-stack reference
		AliasName:          jsii.String(fmt.Sprintf("alias/%s/%s/token-key-secondary", partner, stage)),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String(fmt.Sprintf("/%s/%s/token-key-arn-secondary", partner, stage)),
		Tags: &map[string]*string{
			"Partner": jsii.String(partner),
			"Stage":   jsii.String(stage),
			"KeyType": jsii.String("TokenEncryption"),
			"Replica": jsii.String("true"),
		},
	})

	// Create region-specific database encryption key for secondary region
	secondaryDbKey := liftcdk.NewLiftKMSKey(secondaryStack, jsii.String("SecondaryDbKey"), &liftcdk.LiftKMSKeyProps{
		Description:        jsii.String("KMS key for DynamoDB encryption (secondary)"),
		KeySpec:            awskms.KeySpec_SYMMETRIC_DEFAULT,
		KeyUsage:           awskms.KeyUsage_ENCRYPT_DECRYPT,
		MultiRegion:        jsii.Bool(false),
		EnableKeyRotation:  jsii.Bool(true),
		AliasName:          jsii.String(fmt.Sprintf("alias/%s/%s/db-key-secondary", partner, stage)),
		EnableSSMParameter: jsii.Bool(true),
		SSMParameterPath:   jsii.String(fmt.Sprintf("/%s/%s/db-key-arn-secondary", partner, stage)),
		Tags: &map[string]*string{
			"Partner": jsii.String(partner),
			"Stage":   jsii.String(stage),
			"KeyType": jsii.String("DatabaseEncryption"),
		},
	})

	// Output secondary key ARNs
	awscdk.NewCfnOutput(secondaryStack, jsii.String("HMACReplicaKeyArn"), &awscdk.CfnOutputProps{
		Value:       hmacReplicaKey.GetKeyArn(),
		Description: jsii.String("HMAC Replica Key ARN (Secondary)"),
	})

	awscdk.NewCfnOutput(secondaryStack, jsii.String("TokenReplicaKeyArn"), &awscdk.CfnOutputProps{
		Value:       tokenReplicaKey.GetKeyArn(),
		Description: jsii.String("Token Encryption Replica Key ARN (Secondary)"),
	})

	awscdk.NewCfnOutput(secondaryStack, jsii.String("SecondaryDbKeyArn"), &awscdk.CfnOutputProps{
		Value:       secondaryDbKey.GetKeyArn(),
		Description: jsii.String("Database Encryption Key ARN (Secondary)"),
	})

	app.Synth(nil)
}
