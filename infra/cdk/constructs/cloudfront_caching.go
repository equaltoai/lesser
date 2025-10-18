package constructs

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// CloudFrontConfig defines CloudFront distribution configuration for caching
type CloudFrontConfig struct {
	Environment string
	Domain      string
	Certificate awscertificatemanager.ICertificate
	HttpApi     awsapigatewayv2.HttpApi
	S3Bucket    awss3.Bucket
}

// CreateCloudFrontDistribution creates a CloudFront distribution with caching
func CreateCloudFrontDistribution(scope constructs.Construct, config *CloudFrontConfig) awscloudfront.Distribution {
	// Get caching configuration
	cacheConfig := GetCachingConfig(config.Environment)

	// Create cache policies
	defaultCachePolicy := createDefaultCachePolicy(scope, config.Environment)
	noCachePolicy := createNoCachePolicy(scope, config.Environment)
	staticCachePolicy := createStaticCachePolicy(scope, config.Environment)

	// Create origin request policy
	originRequestPolicy := createOriginRequestPolicy(scope, config.Environment)

	// Create response headers policy
	responseHeadersPolicy := createResponseHeadersPolicy(scope, config.Environment)

	// Get API Gateway domain
	apiDomain := fmt.Sprintf("%s.execute-api.%s.amazonaws.com",
		*config.HttpApi.ApiId(),
		*awscdk.Stack_Of(scope).Region())

	// Create API origin
	apiOrigin := awscloudfrontorigins.NewHttpOrigin(
		jsii.String(apiDomain),
		&awscloudfrontorigins.HttpOriginProps{
			ProtocolPolicy: awscloudfront.OriginProtocolPolicy_HTTPS_ONLY,
			HttpPort:       jsii.Number(80),
			HttpsPort:      jsii.Number(443),
			OriginPath:     jsii.String(""),
			CustomHeaders: &map[string]*string{
				"X-Environment": jsii.String(config.Environment),
			},
		},
	)

	// Create S3 origin for media
	s3Origin := awscloudfrontorigins.NewS3Origin(
		config.S3Bucket,
		&awscloudfrontorigins.S3OriginProps{
			OriginAccessIdentity: awscloudfront.NewOriginAccessIdentity(scope, jsii.String("OAI"), &awscloudfront.OriginAccessIdentityProps{
				Comment: jsii.String(fmt.Sprintf("OAI for %s S3 bucket", config.Environment)),
			}),
		},
	)

	// Define cache behaviors (not used directly but shows what could be configured)
	_ = createCacheBehaviors(
		apiOrigin,
		defaultCachePolicy,
		noCachePolicy,
		staticCachePolicy,
		originRequestPolicy,
		responseHeadersPolicy,
		cacheConfig.CachingEnabled,
	)

	// Create distribution
	distribution := awscloudfront.NewDistribution(scope, jsii.String("CloudFrontDistribution"), &awscloudfront.DistributionProps{
		Comment:           jsii.String(fmt.Sprintf("Lesser %s CloudFront Distribution", config.Environment)),
		DefaultRootObject: jsii.String("index.html"),
		DomainNames: func() *[]*string {
			if config.Domain != "" && config.Certificate != nil {
				return &[]*string{jsii.String(config.Domain)}
			}
			return nil
		}(),
		Certificate: config.Certificate,
		HttpVersion: awscloudfront.HttpVersion_HTTP2_AND_3,
		PriceClass:  awscloudfront.PriceClass_PRICE_CLASS_100, // Use only North America and Europe

		// Default behavior for API routes
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin:                apiOrigin,
			AllowedMethods:        awscloudfront.AllowedMethods_ALLOW_ALL(),
			CachedMethods:         awscloudfront.CachedMethods_CACHE_GET_HEAD_OPTIONS(),
			CachePolicy:           defaultCachePolicy,
			OriginRequestPolicy:   originRequestPolicy,
			ResponseHeadersPolicy: responseHeadersPolicy,
			ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
			Compress:              jsii.Bool(true),
		},

		// Additional behaviors for specific paths
		AdditionalBehaviors: &map[string]*awscloudfront.BehaviorOptions{
			// Media files from S3
			"/media/*": {
				Origin:                s3Origin,
				AllowedMethods:        awscloudfront.AllowedMethods_ALLOW_GET_HEAD_OPTIONS(),
				CachedMethods:         awscloudfront.CachedMethods_CACHE_GET_HEAD_OPTIONS(),
				CachePolicy:           staticCachePolicy,
				ResponseHeadersPolicy: responseHeadersPolicy,
				ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
				Compress:              jsii.Bool(true),
			},
			// Instance information - long cache
			"/api/v1/instance":      createCacheBehavior(apiOrigin, staticCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/api/v2/instance":      createCacheBehavior(apiOrigin, staticCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/.well-known/nodeinfo": createCacheBehavior(apiOrigin, staticCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/nodeinfo/*":           createCacheBehavior(apiOrigin, staticCachePolicy, originRequestPolicy, responseHeadersPolicy),

			// Public timelines - short cache
			"/api/v1/timelines/public": createCacheBehavior(apiOrigin, defaultCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/api/v1/timelines/tag/*":  createCacheBehavior(apiOrigin, defaultCachePolicy, originRequestPolicy, responseHeadersPolicy),

			// No cache for auth and user-specific endpoints
			"/oauth/*":               createNoCacheBehavior(apiOrigin, noCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/auth/*":                createNoCacheBehavior(apiOrigin, noCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/api/v1/timelines/home": createNoCacheBehavior(apiOrigin, noCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/api/v1/notifications*": createNoCacheBehavior(apiOrigin, noCachePolicy, originRequestPolicy, responseHeadersPolicy),
			"/api/v1/streaming/*":    createNoCacheBehavior(apiOrigin, noCachePolicy, originRequestPolicy, responseHeadersPolicy),
		},

		// Error pages
		ErrorResponses: &[]*awscloudfront.ErrorResponse{
			{
				HttpStatus:         jsii.Number(404),
				ResponseHttpStatus: jsii.Number(404),
				ResponsePagePath:   jsii.String("/404.html"),
				Ttl:                awscdk.Duration_Minutes(jsii.Number(5)),
			},
			{
				HttpStatus:         jsii.Number(500),
				ResponseHttpStatus: jsii.Number(500),
				ResponsePagePath:   jsii.String("/500.html"),
				Ttl:                awscdk.Duration_Seconds(jsii.Number(10)),
			},
			{
				HttpStatus:         jsii.Number(502),
				ResponseHttpStatus: jsii.Number(502),
				ResponsePagePath:   jsii.String("/502.html"),
				Ttl:                awscdk.Duration_Seconds(jsii.Number(10)),
			},
			{
				HttpStatus:         jsii.Number(503),
				ResponseHttpStatus: jsii.Number(503),
				ResponsePagePath:   jsii.String("/503.html"),
				Ttl:                awscdk.Duration_Seconds(jsii.Number(10)),
			},
		},

		// Logging
		EnableLogging: jsii.Bool(config.Environment == "production"),
		LogBucket: func() awss3.IBucket {
			if config.Environment == "production" {
				logBucket := awss3.NewBucket(scope, jsii.String("CloudFrontLogBucket"), &awss3.BucketProps{
					BucketName:    jsii.String(fmt.Sprintf("lesser-%s-cloudfront-logs", config.Environment)),
					Encryption:    awss3.BucketEncryption_S3_MANAGED,
					RemovalPolicy: awscdk.RemovalPolicy_RETAIN,
				})

				// Apply comprehensive CloudFront log lifecycle policies
				ApplyS3LifecyclePolicies(&S3LifecycleConfig{
					Environment: config.Environment,
					Bucket:      logBucket,
					BucketType:  "cloudfront-logs",
				})

				return logBucket
			}
			return nil
		}(),
		LogFilePrefix: jsii.String("cloudfront/"),

		// Enable IPv6
		EnableIpv6: jsii.Bool(true),

		// WAF Web ACL (if needed)
		WebAclId: func() *string {
			if config.Environment == "production" {
				// This would reference a WAF WebACL ARN if configured
				return nil
			}
			return nil
		}(),
	})

	// Add outputs
	awscdk.NewCfnOutput(scope, jsii.String("CloudFrontURL"), &awscdk.CfnOutputProps{
		Value:       distribution.DistributionDomainName(),
		Description: jsii.String("CloudFront distribution URL"),
	})

	awscdk.NewCfnOutput(scope, jsii.String("CloudFrontDistributionId"), &awscdk.CfnOutputProps{
		Value:       distribution.DistributionId(),
		Description: jsii.String("CloudFront distribution ID"),
	})

	return distribution
}

// createDefaultCachePolicy creates the default cache policy
func createDefaultCachePolicy(scope constructs.Construct, environment string) awscloudfront.CachePolicy {
	config := GetCachingConfig(environment)

	return awscloudfront.NewCachePolicy(scope, jsii.String("DefaultCachePolicy"), &awscloudfront.CachePolicyProps{
		CachePolicyName:            jsii.String(fmt.Sprintf("lesser-%s-default-cache", environment)),
		Comment:                    jsii.String("Default cache policy with short TTL"),
		DefaultTtl:                 awscdk.Duration_Seconds(jsii.Number(config.CacheTTLSeconds)),
		MaxTtl:                     awscdk.Duration_Seconds(jsii.Number(config.CacheTTLSeconds * 2)),
		MinTtl:                     awscdk.Duration_Seconds(jsii.Number(0)),
		EnableAcceptEncodingGzip:   jsii.Bool(true),
		EnableAcceptEncodingBrotli: jsii.Bool(true),
		QueryStringBehavior: awscloudfront.CacheQueryStringBehavior_AllowList(
			jsii.String("local"),
			jsii.String("only_media"),
			jsii.String("max_id"),
			jsii.String("since_id"),
			jsii.String("min_id"),
			jsii.String("limit"),
			jsii.String("offset"),
		),
		HeaderBehavior: awscloudfront.CacheHeaderBehavior_AllowList(
			jsii.String("Authorization"),
			jsii.String("Accept"),
			jsii.String("Accept-Language"),
		),
		CookieBehavior: awscloudfront.CacheCookieBehavior_None(),
	})
}

// createNoCachePolicy creates a no-cache policy for dynamic content
func createNoCachePolicy(scope constructs.Construct, environment string) awscloudfront.CachePolicy {
	return awscloudfront.NewCachePolicy(scope, jsii.String("NoCachePolicy"), &awscloudfront.CachePolicyProps{
		CachePolicyName:            jsii.String(fmt.Sprintf("lesser-%s-no-cache", environment)),
		Comment:                    jsii.String("No cache policy for dynamic content"),
		DefaultTtl:                 awscdk.Duration_Seconds(jsii.Number(0)),
		MaxTtl:                     awscdk.Duration_Seconds(jsii.Number(0)),
		MinTtl:                     awscdk.Duration_Seconds(jsii.Number(0)),
		EnableAcceptEncodingGzip:   jsii.Bool(true),
		EnableAcceptEncodingBrotli: jsii.Bool(true),
		QueryStringBehavior:        awscloudfront.CacheQueryStringBehavior_All(),
		HeaderBehavior:             awscloudfront.CacheHeaderBehavior_None(),
		CookieBehavior:             awscloudfront.CacheCookieBehavior_All(),
	})
}

// createStaticCachePolicy creates a cache policy for static content
func createStaticCachePolicy(scope constructs.Construct, environment string) awscloudfront.CachePolicy {
	return awscloudfront.NewCachePolicy(scope, jsii.String("StaticCachePolicy"), &awscloudfront.CachePolicyProps{
		CachePolicyName:            jsii.String(fmt.Sprintf("lesser-%s-static-cache", environment)),
		Comment:                    jsii.String("Cache policy for static content"),
		DefaultTtl:                 awscdk.Duration_Hours(jsii.Number(1)),
		MaxTtl:                     awscdk.Duration_Days(jsii.Number(1)),
		MinTtl:                     awscdk.Duration_Minutes(jsii.Number(1)),
		EnableAcceptEncodingGzip:   jsii.Bool(true),
		EnableAcceptEncodingBrotli: jsii.Bool(true),
		QueryStringBehavior:        awscloudfront.CacheQueryStringBehavior_None(),
		HeaderBehavior:             awscloudfront.CacheHeaderBehavior_None(),
		CookieBehavior:             awscloudfront.CacheCookieBehavior_None(),
	})
}

// createOriginRequestPolicy creates an origin request policy
func createOriginRequestPolicy(scope constructs.Construct, environment string) awscloudfront.OriginRequestPolicy {
	return awscloudfront.NewOriginRequestPolicy(scope, jsii.String("OriginRequestPolicy"), &awscloudfront.OriginRequestPolicyProps{
		OriginRequestPolicyName: jsii.String(fmt.Sprintf("lesser-%s-origin-request", environment)),
		Comment:                 jsii.String("Origin request policy for API Gateway"),
		QueryStringBehavior:     awscloudfront.OriginRequestQueryStringBehavior_All(),
		HeaderBehavior: awscloudfront.OriginRequestHeaderBehavior_AllowList(
			jsii.String("Accept"),
			jsii.String("Accept-Language"),
			jsii.String("Content-Type"),
			jsii.String("Authorization"),
			jsii.String("X-Request-ID"),
			jsii.String("Digest"),
			jsii.String("Signature"),
			jsii.String("Date"),
			jsii.String("CloudFront-Viewer-Country"),
			jsii.String("CloudFront-Is-Mobile-Viewer"),
			jsii.String("CloudFront-Is-Desktop-Viewer"),
		),
		CookieBehavior: awscloudfront.OriginRequestCookieBehavior_All(),
	})
}

// createResponseHeadersPolicy creates a response headers policy
func createResponseHeadersPolicy(scope constructs.Construct, environment string) awscloudfront.ResponseHeadersPolicy {
	return awscloudfront.NewResponseHeadersPolicy(scope, jsii.String("ResponseHeadersPolicy"), &awscloudfront.ResponseHeadersPolicyProps{
		ResponseHeadersPolicyName: jsii.String(fmt.Sprintf("lesser-%s-response-headers", environment)),
		Comment:                   jsii.String("Response headers policy for security and caching"),

		SecurityHeadersBehavior: &awscloudfront.ResponseSecurityHeadersBehavior{
			ContentTypeOptions: &awscloudfront.ResponseHeadersContentTypeOptions{
				Override: jsii.Bool(true),
			},
			FrameOptions: &awscloudfront.ResponseHeadersFrameOptions{
				FrameOption: awscloudfront.HeadersFrameOption_DENY,
				Override:    jsii.Bool(true),
			},
			ReferrerPolicy: &awscloudfront.ResponseHeadersReferrerPolicy{
				ReferrerPolicy: awscloudfront.HeadersReferrerPolicy_STRICT_ORIGIN_WHEN_CROSS_ORIGIN,
				Override:       jsii.Bool(true),
			},
			XssProtection: &awscloudfront.ResponseHeadersXSSProtection{
				ModeBlock:  jsii.Bool(true),
				Protection: jsii.Bool(true),
				Override:   jsii.Bool(true),
			},
			StrictTransportSecurity: &awscloudfront.ResponseHeadersStrictTransportSecurity{
				AccessControlMaxAge: awscdk.Duration_Seconds(jsii.Number(63072000)), // 2 years
				IncludeSubdomains:   jsii.Bool(true),
				Override:            jsii.Bool(true),
			},
			ContentSecurityPolicy: &awscloudfront.ResponseHeadersContentSecurityPolicy{
				ContentSecurityPolicy: jsii.String("default-src 'self'; img-src 'self' data: https:; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self' wss: https:"),
				Override:              jsii.Bool(true),
			},
		},

		CorsBehavior: &awscloudfront.ResponseHeadersCorsBehavior{
			AccessControlAllowCredentials: jsii.Bool(false),
			AccessControlAllowHeaders: &[]*string{
				jsii.String("Content-Type"),
				jsii.String("Authorization"),
				jsii.String("Accept"),
				jsii.String("X-Request-ID"),
			},
			AccessControlAllowMethods: &[]*string{
				jsii.String("GET"),
				jsii.String("POST"),
				jsii.String("PUT"),
				jsii.String("DELETE"),
				jsii.String("PATCH"),
				jsii.String("OPTIONS"),
			},
			AccessControlAllowOrigins: &[]*string{
				jsii.String("*"),
			},
			AccessControlMaxAge: awscdk.Duration_Hours(jsii.Number(24)),
			OriginOverride:      jsii.Bool(false),
		},

		CustomHeadersBehavior: &awscloudfront.ResponseCustomHeadersBehavior{
			CustomHeaders: &[]*awscloudfront.ResponseCustomHeader{
				{
					Header:   jsii.String("X-Environment"),
					Value:    jsii.String(environment),
					Override: jsii.Bool(false),
				},
				{
					Header:   jsii.String("X-Powered-By"),
					Value:    jsii.String("Lesser"),
					Override: jsii.Bool(false),
				},
			},
		},
	})
}

// Helper functions for creating cache behaviors
func createCacheBehavior(
	origin awscloudfront.IOrigin,
	cachePolicy awscloudfront.ICachePolicy,
	originRequestPolicy awscloudfront.IOriginRequestPolicy,
	responseHeadersPolicy awscloudfront.IResponseHeadersPolicy,
) *awscloudfront.BehaviorOptions {
	return &awscloudfront.BehaviorOptions{
		Origin:                origin,
		AllowedMethods:        awscloudfront.AllowedMethods_ALLOW_GET_HEAD_OPTIONS(),
		CachedMethods:         awscloudfront.CachedMethods_CACHE_GET_HEAD_OPTIONS(),
		CachePolicy:           cachePolicy,
		OriginRequestPolicy:   originRequestPolicy,
		ResponseHeadersPolicy: responseHeadersPolicy,
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		Compress:              jsii.Bool(true),
	}
}

func createNoCacheBehavior(
	origin awscloudfront.IOrigin,
	cachePolicy awscloudfront.ICachePolicy,
	originRequestPolicy awscloudfront.IOriginRequestPolicy,
	responseHeadersPolicy awscloudfront.IResponseHeadersPolicy,
) *awscloudfront.BehaviorOptions {
	return &awscloudfront.BehaviorOptions{
		Origin:                origin,
		AllowedMethods:        awscloudfront.AllowedMethods_ALLOW_ALL(),
		CachedMethods:         awscloudfront.CachedMethods_CACHE_GET_HEAD_OPTIONS(),
		CachePolicy:           cachePolicy,
		OriginRequestPolicy:   originRequestPolicy,
		ResponseHeadersPolicy: responseHeadersPolicy,
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		Compress:              jsii.Bool(true),
	}
}

func createCacheBehaviors(
	apiOrigin awscloudfront.IOrigin,
	defaultCachePolicy awscloudfront.ICachePolicy,
	noCachePolicy awscloudfront.ICachePolicy,
	staticCachePolicy awscloudfront.ICachePolicy,
	originRequestPolicy awscloudfront.IOriginRequestPolicy,
	responseHeadersPolicy awscloudfront.IResponseHeadersPolicy,
	cachingEnabled bool,
) map[string]*awscloudfront.BehaviorOptions {
	behaviors := make(map[string]*awscloudfront.BehaviorOptions)

	if !cachingEnabled {
		// If caching is disabled, use no-cache policy for everything
		return behaviors
	}

	// Add cache behaviors for cacheable routes
	for _, route := range GetCacheableRoutes() {
		behaviors[route.Path] = createCacheBehavior(
			apiOrigin,
			defaultCachePolicy,
			originRequestPolicy,
			responseHeadersPolicy,
		)
	}

	// Add no-cache behaviors for non-cacheable routes
	for _, path := range GetNonCacheableRoutes() {
		behaviors[path] = createNoCacheBehavior(
			apiOrigin,
			noCachePolicy,
			originRequestPolicy,
			responseHeadersPolicy,
		)
	}

	return behaviors
}
