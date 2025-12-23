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
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type AuthUIProps struct {
	AppName     string
	Environment string
	Domain      string
	AccountID   string
	Region      string
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

	isProd := naming.IsLiveEnvironment(props.Environment)
	removalPolicy := awscdk.RemovalPolicy_DESTROY
	if isProd {
		removalPolicy = awscdk.RemovalPolicy_RETAIN
	}

	site := liftcdk.NewStaticSite(construct, jsii.String("AuthUI"), &liftcdk.StaticSiteProps{
		DomainName:        jsii.String(authDomain),
		HostedZone:        props.HostedZone,
		Certificate:       props.Certificate,
		BucketName:        jsii.String(naming.S3BucketName(props.AppName, naming.StageForEnvironment(props.Environment), "auth-ui", props.AccountID, props.Region)),
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
	})

	awscdk.NewCfnOutput(construct, jsii.String("AuthUIURL"), &awscdk.CfnOutputProps{
		Value:       jsii.String(fmt.Sprintf("https://%s", authDomain)),
		Description: jsii.String("Auth UI URL"),
	})

	return &AuthUI{
		Construct:    construct,
		Bucket:       site.Bucket,
		Distribution: site.Distribution,
		URL:          authDomain,
	}
}
