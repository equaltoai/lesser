package constructs

import (
	"fmt"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/constructs-go/constructs/v10"
	_jsii "github.com/aws/jsii-runtime-go"
)

// NewFrontendStaticResponseHeadersPolicy returns a strict response headers policy for the bundled auth UI.
func NewFrontendStaticResponseHeadersPolicy(scope constructs.Construct, domainName *string) awscloudfront.ResponseHeadersPolicy {
	csp := buildFrontendStaticCSP(domainName)

	return awscloudfront.NewResponseHeadersPolicy(scope, _jsii.String("AuthUIResponseHeadersPolicy"), &awscloudfront.ResponseHeadersPolicyProps{
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

// NewClientSSRResponseHeadersPolicy supplies a conservative fallback CSP while leaving origin CSP authoritative.
func NewClientSSRResponseHeadersPolicy(scope constructs.Construct) awscloudfront.ResponseHeadersPolicy {
	return awscloudfront.NewResponseHeadersPolicy(scope, _jsii.String("ClientSSRResponseHeadersPolicy"), &awscloudfront.ResponseHeadersPolicyProps{
		Comment: _jsii.String("Lesser SSR client security headers with non-overriding fallback CSP"),
		SecurityHeadersBehavior: &awscloudfront.ResponseSecurityHeadersBehavior{
			ContentSecurityPolicy: &awscloudfront.ResponseHeadersContentSecurityPolicy{
				ContentSecurityPolicy: _jsii.String(buildClientSSRFallbackCSP()),
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

func buildClientSSRFallbackCSP() string {
	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"img-src 'self' data: https:",
		"font-src 'self' data: https:",
		"style-src 'self'",
		"script-src 'self'",
		"connect-src 'self' https: wss:",
	}, "; ") + ";"
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
		// Astro/Svelte island runtime snippets generated into auth-ui/dist/*.html.
		// Refreshed 2026-07-30 after the Astro 7 auth UI dependency remediation;
		// hashes verified unchanged against a fresh auth-ui production build.
		// CI builds auth-ui from its pinned lockfile and verifies these exact hashes:
		//   bash scripts/verify_auth_ui_csp.sh
		"'sha256-QzWFZi+FLIx23tnm9SBU4aEgx4x8DsuASP07mfqol/c='",
		"'sha256-Ya0pUYrC7nM5Cn/056TyVuEiz6dFGrzmkWzgON0pF0U='",
		"'sha256-eIXWvAmxkr251LJZkjniEK5LcPF3NkapbJepohwYRIc='",
		"'sha256-akD8rFdL+EsozO0lnT/LRcV7tR1XHwagTjg9VZqsgJU='",
	}
}

func authUIInlineStyleHashes() []string {
	return []string{
		"'sha256-vv9IoKo7BSLbWcUHr3tNmfNVmm5L/9Cfn2H6LMk7/ow='",
	}
}
