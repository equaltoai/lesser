package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type AuthUIProps struct {
	Environment string
	Domain      string
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

	// S3 bucket for static assets
	bucket := awss3.NewBucket(construct, jsii.String("AuthUIBucket"), &awss3.BucketProps{
		BucketName: jsii.String(fmt.Sprintf("lesser-auth-ui-%s", props.Domain)),
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		AutoDeleteObjects: jsii.Bool(true),
		PublicReadAccess: jsii.Bool(false), // CloudFront OAI will provide access
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		Encryption: awss3.BucketEncryption_S3_MANAGED,
		Versioned: jsii.Bool(false),
		LifecycleRules: &[]*awss3.LifecycleRule{
			{
				AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(1)),
				Enabled: jsii.Bool(true),
			},
		},
	})

	// Origin Access Identity for CloudFront
	oai := awscloudfront.NewOriginAccessIdentity(construct, jsii.String("AuthUIOAI"), &awscloudfront.OriginAccessIdentityProps{
		Comment: jsii.String(fmt.Sprintf("OAI for Lesser Auth UI - %s", props.Environment)),
	})

	// Grant CloudFront OAI read access to bucket
	bucket.GrantRead(oai.GrantPrincipal(), jsii.String("*"))

	// Create CloudFront Function to append /index.html to directory requests
	cfFunction := awscloudfront.NewFunction(construct, jsii.String("AuthUIIndexRewrite"), &awscloudfront.FunctionProps{
		Code: awscloudfront.FunctionCode_FromInline(jsii.String(`
function handler(event) {
    var request = event.request;
    var uri = request.uri;
    
    // If URI ends with /, append index.html
    if (uri.endsWith('/')) {
        request.uri += 'index.html';
    }
    // If URI has no extension, append /index.html
    else if (!uri.includes('.')) {
        request.uri += '/index.html';
    }
    
    return request;
}
		`)),
		Comment: jsii.String("Append index.html to directory requests"),
	})

	// Create S3 origin for CloudFront
	s3Origin := awscloudfrontorigins.NewS3Origin(bucket, &awscloudfrontorigins.S3OriginProps{
		OriginAccessIdentity: oai,
	})

	// CloudFront distribution
	distribution := awscloudfront.NewDistribution(construct, jsii.String("AuthUIDistribution"), &awscloudfront.DistributionProps{
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin: s3Origin,
			ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
			AllowedMethods: awscloudfront.AllowedMethods_ALLOW_GET_HEAD_OPTIONS(),
			CachedMethods: awscloudfront.CachedMethods_CACHE_GET_HEAD_OPTIONS(),
			Compress: jsii.Bool(true),
			CachePolicy: awscloudfront.CachePolicy_CACHING_OPTIMIZED(),
			FunctionAssociations: &[]*awscloudfront.FunctionAssociation{
				{
					Function:  cfFunction,
					EventType: awscloudfront.FunctionEventType_VIEWER_REQUEST,
				},
			},
		},
		DefaultRootObject: jsii.String("index.html"),
		DomainNames: &[]*string{jsii.String(authDomain)},
		Certificate: props.Certificate,
		Comment: jsii.String(fmt.Sprintf("Lesser Auth UI - %s", props.Environment)),
		EnableLogging: jsii.Bool(false), // Disabled for now - would require separate logging bucket with ACLs
		PriceClass: awscloudfront.PriceClass_PRICE_CLASS_100, // US, Canada, Europe
	})

	// Output the distribution domain and auth URL
	awscdk.NewCfnOutput(construct, jsii.String("AuthUIDistributionDomain"), &awscdk.CfnOutputProps{
		Value: distribution.DistributionDomainName(),
		Description: jsii.String("CloudFront distribution domain for Auth UI"),
		ExportName: jsii.String(fmt.Sprintf("%s-auth-ui-distribution", props.Environment)),
	})

	awscdk.NewCfnOutput(construct, jsii.String("AuthUIURL"), &awscdk.CfnOutputProps{
		Value: jsii.String(fmt.Sprintf("https://%s", authDomain)),
		Description: jsii.String("Auth UI URL"),
		ExportName: jsii.String(fmt.Sprintf("%s-auth-ui-url", props.Environment)),
	})

	return &AuthUI{
		Construct:    construct,
		Bucket:       bucket,
		Distribution: distribution,
		URL:          authDomain,
	}
}

