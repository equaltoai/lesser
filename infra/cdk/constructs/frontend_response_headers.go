package constructs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/constructs-go/constructs/v10"
	_jsii "github.com/aws/jsii-runtime-go"
)

// NewFrontendStaticResponseHeadersPolicy returns a ResponseHeadersPolicy intended for HTML
// served from S3-backed CloudFront behaviors (e.g. /l/* and /auth/* on the stage apex).
//
// The policy is strict and intentionally does not rely on unsafe CSP directives. Inline
// scripts/styles required by the bundled auth UI are allow-listed by hash.
func NewFrontendStaticResponseHeadersPolicy(scope constructs.Construct, domainName *string) awscloudfront.ResponseHeadersPolicy {
	csp := buildFrontendStaticCSP(domainName)

	return awscloudfront.NewResponseHeadersPolicy(scope, _jsii.String("FrontendStaticResponseHeadersPolicy"), &awscloudfront.ResponseHeadersPolicyProps{
		Comment: _jsii.String("Lesser static site security headers (no unsafe CSP directives)"),
		SecurityHeadersBehavior: &awscloudfront.ResponseSecurityHeadersBehavior{
			ContentSecurityPolicy: &awscloudfront.ResponseHeadersContentSecurityPolicy{
				ContentSecurityPolicy: _jsii.String(csp),
				Override:              _jsii.Bool(false),
			},
			ContentTypeOptions: &awscloudfront.ResponseHeadersContentTypeOptions{Override: _jsii.Bool(true)},
			FrameOptions: &awscloudfront.ResponseHeadersFrameOptions{
				FrameOption: awscloudfront.HeadersFrameOption_DENY,
				Override:    _jsii.Bool(true),
			},
			ReferrerPolicy: &awscloudfront.ResponseHeadersReferrerPolicy{
				ReferrerPolicy: awscloudfront.HeadersReferrerPolicy_STRICT_ORIGIN_WHEN_CROSS_ORIGIN,
				Override:       _jsii.Bool(true),
			},
			StrictTransportSecurity: &awscloudfront.ResponseHeadersStrictTransportSecurity{
				AccessControlMaxAge: awscdk.Duration_Days(_jsii.Number(365)),
				IncludeSubdomains:   _jsii.Bool(true),
				Override:            _jsii.Bool(true),
			},
			XssProtection: &awscloudfront.ResponseHeadersXSSProtection{
				Protection: _jsii.Bool(true),
				ModeBlock:  _jsii.Bool(true),
				Override:   _jsii.Bool(true),
			},
		},
		CustomHeadersBehavior: &awscloudfront.ResponseCustomHeadersBehavior{
			CustomHeaders: &[]*awscloudfront.ResponseCustomHeader{
				{
					Header:   _jsii.String("Permissions-Policy"),
					Value:    _jsii.String("camera=(), microphone=(), geolocation=(), payment=()"),
					Override: _jsii.Bool(true),
				},
			},
		},
		RemoveHeaders: &[]*string{
			_jsii.String("Server"),
		},
	})
}

func buildFrontendStaticCSP(domainName *string) string {
	domain := ""
	if domainName != nil {
		domain = strings.TrimSuffix(strings.TrimSpace(*domainName), ".")
	}

	connect := []string{"'self'"}
	if domain != "" {
		connect = append(connect, "https://api."+domain, "wss://ws."+domain)
	}

	styleSources := append([]string{"'self'"}, authUIInlineStyleHashes()...)
	scriptSources := append([]string{"'self'"}, authUIInlineScriptHashes()...)

	parts := []string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"img-src 'self' data: https:",
		"font-src 'self' data: https:",
		fmt.Sprintf("style-src %s", strings.Join(styleSources, " ")),
		fmt.Sprintf("script-src %s", strings.Join(scriptSources, " ")),
		fmt.Sprintf("connect-src %s", strings.Join(connect, " ")),
	}
	return strings.Join(parts, "; ") + ";"
}

func authUIInlineScriptHashes() []string {
	return []string{
		"'sha256-QzWFZi+FLIx23tnm9SBU4aEgx4x8DsuASP07mfqol/c='",
		"'sha256-U7a72oKuFFz8D7GUHLA1NZ0ciymHmDOc9T9aVDg2rWU='",
		"'sha256-eIXWvAmxkr251LJZkjniEK5LcPF3NkapbJepohwYRIc='",
		"'sha256-IV0HjYu959C/EiJIL2l/9Ty8PA4757JXhA/g112YXVE='",
	}
}

func authUIInlineStyleHashes() []string {
	return []string{
		"'sha256-Eq62DWSZ57jJZCUv6mhFsTpy7vjhvM2oZhAX+bvTTN4='",
		"'sha256-Np+0wo4qGeEiSfIZluf224zV9nNLY+nOGde1KcwOB8g='",
		"'sha256-1mD1INqKidrIyg8naC+GEWKaF7uCtCKeBQqXOxf8LM8='",
		"'sha256-iegu0Wb3dMBfhqOyzQ8kCD9W04jq6I7g1SiDlmvSiSg='",
		"'sha256-qkbV7mmkk3RRKy6Ha/Sdwvz6VwoEghyumJdMY397/D0='",
		"'sha256-vv9IoKo7BSLbWcUHr3tNmfNVmm5L/9Cfn2H6LMk7/ow='",
	}
}
