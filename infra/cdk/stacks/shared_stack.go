package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type SharedStackProps struct {
	awscdk.StackProps
	AppName string
}

type SharedStack struct {
	awscdk.Stack
	EncryptionKey   awskms.Key
	ActorPrivateKey awssecretsmanager.Secret
}

func NewSharedStack(scope constructs.Construct, id string, props *SharedStackProps) *SharedStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	sharedStack := &SharedStack{
		Stack: stack,
	}

	// Create KMS key for encryption
	sharedStack.EncryptionKey = awskms.NewKey(stack, jsii.String("LesserEncryptionKey"), &awskms.KeyProps{
		Description:       jsii.String(fmt.Sprintf("%s encryption key for actor private keys", props.AppName)),
		EnableKeyRotation: jsii.Bool(true),
		Alias:             jsii.String(fmt.Sprintf("alias/%s-encryption", props.AppName)),
		RemovalPolicy:     awscdk.RemovalPolicy_RETAIN,
	})

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

	return sharedStack
}
