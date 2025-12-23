package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
	liftnaming "github.com/pay-theory/lift/pkg/naming"
)

type AuthUIProps struct {
	Environment string
	Domain      string
	HostedZone  awsroute53.IHostedZone
	Certificate awscertificatemanager.ICertificate
}

type AuthUI struct {
	constructs.Construct
	Bucket       awss3.Bucket
	Distribution awscloudfront.Distribution
	URL          string
}

// NewAuthUI creates the Auth UI infrastructure (S3 + CloudFront)
func NewAuthUI(scope constructs.Construct, id *string, props *AuthUIProps) *AuthUI {
	construct := constructs.NewConstruct(scope, id)

	// Auth subdomain
	authDomain := fmt.Sprintf("auth.%s", props.Domain)

	if props.HostedZone == nil {
		panic("AuthUI requires HostedZone")
	}

	isProd := props.Environment == "production"
	removalPolicy := awscdk.RemovalPolicy_DESTROY
	if isProd {
		removalPolicy = awscdk.RemovalPolicy_RETAIN
	}

	site := liftcdk.NewStaticSite(construct, jsii.String("AuthUI"), &liftcdk.StaticSiteProps{
		DomainName:        jsii.String(authDomain),
		HostedZone:        props.HostedZone,
		Certificate:       props.Certificate,
		BucketName:        jsii.String(liftnaming.SanitizeS3BucketName(fmt.Sprintf("lesser-auth-ui-%s", props.Domain))),
		RemovalPolicy:     removalPolicy,
		AutoDeleteObjects: jsii.Bool(!isProd),
		EnableWWWRedirect: jsii.Bool(false),
		SinglePageApp:     jsii.Bool(true),
		PriceClass:        awscloudfront.PriceClass_PRICE_CLASS_100,
	})

	// Output the distribution domain and auth URL
	awscdk.NewCfnOutput(construct, jsii.String("AuthUIDistributionDomain"), &awscdk.CfnOutputProps{
		Value:       site.Distribution.DistributionDomainName(),
		Description: jsii.String("CloudFront distribution domain for Auth UI"),
		ExportName:  jsii.String(fmt.Sprintf("%s-auth-ui-distribution", props.Environment)),
	})

	awscdk.NewCfnOutput(construct, jsii.String("AuthUIURL"), &awscdk.CfnOutputProps{
		Value:       jsii.String(fmt.Sprintf("https://%s", authDomain)),
		Description: jsii.String("Auth UI URL"),
		ExportName:  jsii.String(fmt.Sprintf("%s-auth-ui-url", props.Environment)),
	})

	return &AuthUI{
		Construct:    construct,
		Bucket:       site.Bucket,
		Distribution: site.Distribution,
		URL:          authDomain,
	}
}
