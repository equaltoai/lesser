package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awskms"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type SharedStackProps struct {
	awscdk.StackProps
	AppName        string
	RootDomain     string
	HostedZoneId   string
	HostedZoneName string
	Stages         []string
}

type SharedStack struct {
	awscdk.Stack
	EncryptionKey   awskms.Key
	ActorPrivateKey awssecretsmanager.Secret
	JWTSecret       awssecretsmanager.Secret
	HostedZone      awsroute53.IHostedZone
	APICertificate  awscertificatemanager.Certificate
	CDNCertificate  awscertificatemanager.Certificate
	RootDomain      string
	Stages          []string
}

func NewSharedStack(scope constructs.Construct, id string, props *SharedStackProps) *SharedStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	sharedStack := &SharedStack{
		Stack:      stack,
		RootDomain: props.RootDomain,
		Stages:     props.Stages,
	}

	sharedStack.initHostedZone(props.HostedZoneName, props.HostedZoneId)

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

	// Create JWT secret (shared across all environments)
	sharedStack.JWTSecret = awssecretsmanager.NewSecret(stack, jsii.String("JWTSecret"), &awssecretsmanager.SecretProps{
		Description:   jsii.String("JWT signing secret for authentication (shared by all environments)"),
		SecretName:    jsii.String(fmt.Sprintf("%s/jwt-secret", props.AppName)),
		EncryptionKey: sharedStack.EncryptionKey,
		GenerateSecretString: &awssecretsmanager.SecretStringGenerator{
			SecretStringTemplate: jsii.String(`{}`),
			GenerateStringKey:    jsii.String("secret"),
			PasswordLength:       jsii.Number(64),
			ExcludeCharacters:    jsii.String(" \t\n\"'\\"),
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

	awscdk.NewCfnOutput(stack, jsii.String("JWTSecretArn"), &awscdk.CfnOutputProps{
		Value:       sharedStack.JWTSecret.SecretArn(),
		Description: jsii.String("JWT secret ARN"),
		ExportName:  jsii.String(fmt.Sprintf("%s-jwt-secret-arn", props.AppName)),
	})

	sharedStack.createCertificates()

	if sharedStack.APICertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("ApiCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.APICertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for stage API domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-api-certificate-arn", props.AppName)),
		})
	}

	if sharedStack.CDNCertificate != nil {
		awscdk.NewCfnOutput(stack, jsii.String("CdnCertificateArn"), &awscdk.CfnOutputProps{
			Value:       sharedStack.CDNCertificate.CertificateArn(),
			Description: jsii.String("ACM certificate ARN for stage CDN domains"),
			ExportName:  jsii.String(fmt.Sprintf("%s-cdn-certificate-arn", props.AppName)),
		})
	}

	return sharedStack
}

func (s *SharedStack) initHostedZone(domain string, hostedZoneId string) {
	if domain == "" && hostedZoneId == "" {
		return
	}

	if hostedZoneId != "" && domain != "" {
		s.HostedZone = awsroute53.HostedZone_FromHostedZoneAttributes(s.Stack, jsii.String("SharedHostedZone"), &awsroute53.HostedZoneAttributes{
			HostedZoneId: jsii.String(hostedZoneId),
			ZoneName:     jsii.String(domain),
		})
		return
	}

	if domain != "" {
		s.HostedZone = awsroute53.HostedZone_FromLookup(s.Stack, jsii.String("SharedHostedZone"), &awsroute53.HostedZoneProviderProps{
			DomainName: jsii.String(domain),
		})
	}
}

func (s *SharedStack) createCertificates() {
	if s.RootDomain == "" || len(s.Stages) == 0 || s.HostedZone == nil {
		return
	}

	stageFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		stageFqdns = append(stageFqdns, jsii.String(fmt.Sprintf("%s.%s", stage, s.RootDomain)))
	}

	validation := awscertificatemanager.CertificateValidation_FromDns(s.HostedZone)

	apiPrimary := stageFqdns[0]
	var apiSans []*string
	if len(stageFqdns) > 1 {
		apiSans = stageFqdns[1:]
	}

	s.APICertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedApiCertificate"), &awscertificatemanager.CertificateProps{
		DomainName:              apiPrimary,
		SubjectAlternativeNames: &apiSans,
		Validation:              validation,
	})

	cdnFqdns := make([]*string, 0, len(s.Stages))
	for _, stage := range s.Stages {
		cdnFqdns = append(cdnFqdns, jsii.String(fmt.Sprintf("cdn.%s.%s", stage, s.RootDomain)))
	}

	cdnPrimary := cdnFqdns[0]
	var cdnSans []*string
	if len(cdnFqdns) > 1 {
		cdnSans = cdnFqdns[1:]
	}

	s.CDNCertificate = awscertificatemanager.NewCertificate(s.Stack, jsii.String("SharedCdnCertificate"), &awscertificatemanager.CertificateProps{
		DomainName:              cdnPrimary,
		SubjectAlternativeNames: &cdnSans,
		Validation:              validation,
	})
}
