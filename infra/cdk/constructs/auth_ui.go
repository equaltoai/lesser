package constructs

import (
	"fmt"
	"strings"

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
	stage := naming.StageForEnvironment(props.Environment)
	removalPolicy := awscdk.RemovalPolicy_DESTROY
	if isProd {
		removalPolicy = awscdk.RemovalPolicy_RETAIN
	}

	site := liftcdk.NewStaticSite(construct, jsii.String("AuthUI"), &liftcdk.StaticSiteProps{
		DomainName:            jsii.String(authDomain),
		HostedZone:            props.HostedZone,
		Certificate:           props.Certificate,
		ResponseHeadersPolicy: authUIResponseHeadersPolicy(construct, props.Domain),
		AppName:               jsii.String(props.AppName),
		Stage:                 jsii.String(string(stage)),
		BucketName:            jsii.String(naming.S3BucketName(props.AppName, stage, "auth-ui", props.AccountID, props.Region)),
		RemovalPolicy:         removalPolicy,
		AutoDeleteObjects:     jsii.Bool(!isProd),
		EnableWWWRedirect:     jsii.Bool(false),
		SinglePageApp:         jsii.Bool(true),
		PriceClass:            awscloudfront.PriceClass_PRICE_CLASS_100,
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

func authUIResponseHeadersPolicy(scope constructs.Construct, stageDomain string) awscloudfront.IResponseHeadersPolicy {
	domain := strings.TrimSuffix(strings.TrimSpace(stageDomain), ".")

	connect := []string{"'self'"}
	if domain != "" {
		connect = append(connect, "https://"+domain, "https://api."+domain, "wss://ws."+domain)
	}

	csp := strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"img-src 'self' data: https:",
		"font-src 'self' data: https:",
		"style-src 'self' 'unsafe-inline'",
		"script-src 'self' 'unsafe-inline'",
		"connect-src " + strings.Join(connect, " "),
	}, "; ") + ";"

	return awscloudfront.NewResponseHeadersPolicy(scope, jsii.String("AuthUIResponseHeadersPolicy"), &awscloudfront.ResponseHeadersPolicyProps{
		Comment: jsii.String("Lesser auth UI security headers"),
		SecurityHeadersBehavior: &awscloudfront.ResponseSecurityHeadersBehavior{
			ContentSecurityPolicy: &awscloudfront.ResponseHeadersContentSecurityPolicy{
				ContentSecurityPolicy: jsii.String(csp),
				Override:              jsii.Bool(true),
			},
			ContentTypeOptions: &awscloudfront.ResponseHeadersContentTypeOptions{Override: jsii.Bool(true)},
			FrameOptions: &awscloudfront.ResponseHeadersFrameOptions{
				FrameOption: awscloudfront.HeadersFrameOption_DENY,
				Override:    jsii.Bool(true),
			},
			ReferrerPolicy: &awscloudfront.ResponseHeadersReferrerPolicy{
				ReferrerPolicy: awscloudfront.HeadersReferrerPolicy_STRICT_ORIGIN_WHEN_CROSS_ORIGIN,
				Override:       jsii.Bool(true),
			},
			StrictTransportSecurity: &awscloudfront.ResponseHeadersStrictTransportSecurity{
				AccessControlMaxAge: awscdk.Duration_Days(jsii.Number(365)),
				IncludeSubdomains:   jsii.Bool(true),
				Override:            jsii.Bool(true),
			},
			XssProtection: &awscloudfront.ResponseHeadersXSSProtection{
				Protection: jsii.Bool(true),
				ModeBlock:  jsii.Bool(true),
				Override:   jsii.Bool(true),
			},
		},
		CustomHeadersBehavior: &awscloudfront.ResponseCustomHeadersBehavior{
			CustomHeaders: &[]*awscloudfront.ResponseCustomHeader{
				{
					Header:   jsii.String("Permissions-Policy"),
					Value:    jsii.String("camera=(), microphone=(), geolocation=(), payment=()"),
					Override: jsii.Bool(true),
				},
			},
		},
		RemoveHeaders: &[]*string{
			jsii.String("Server"),
		},
	})
}
