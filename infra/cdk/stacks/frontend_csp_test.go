package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localconstructs "cdk/constructs"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
)

func TestFrontendStaticCSPIsStrictAndBehaviorScoped(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	hostedZone := awsroute53.NewHostedZone(stack, jsii.String("HostedZone"), &awsroute53.HostedZoneProps{
		ZoneName: jsii.String("example.com"),
	})

	staticPolicy := localconstructs.NewFrontendStaticResponseHeadersPolicy(stack, jsii.String("dev.example.com"))

	clientBucket := awss3.NewBucket(stack, jsii.String("ClientBucket"), &awss3.BucketProps{
		BucketName: jsii.String("test-dev-client-123456789012-us-east-1"),
	})
	authBucket := awss3.NewBucket(stack, jsii.String("AuthBucket"), &awss3.BucketProps{
		BucketName: jsii.String("test-dev-auth-ui-123456789012-us-east-1"),
	})

	_ = apptheorycdk.NewAppTheoryPathRoutedFrontend(stack, jsii.String("ClientFrontend"), &apptheorycdk.AppTheoryPathRoutedFrontendProps{
		ApiOriginUrl: jsii.String("api.dev.example.com"),
		Domain: &apptheorycdk.PathRoutedFrontendDomainConfig{
			HostedZone:       hostedZone,
			DomainName:       jsii.String("dev.example.com"),
			CreateAAAARecord: jsii.Bool(true),
		},
		ApiBypassPaths: &[]*apptheorycdk.ApiBypassConfig{
			{PathPattern: jsii.String("/auth/wallet/*")},
		},
		SpaOrigins: &[]*apptheorycdk.SpaOriginConfig{
			{
				Bucket:                  authBucket,
				PathPattern:             jsii.String("/auth/*"),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_NONE,
				StripPrefixBeforeOrigin: jsii.Bool(true),
			},
			{
				Bucket:                  authBucket,
				PathPattern:             jsii.String("/auth"),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_NONE,
				StripPrefixBeforeOrigin: jsii.Bool(true),
			},
			{
				Bucket:                  clientBucket,
				PathPattern:             jsii.String("/l/*"),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_SPA,
				StripPrefixBeforeOrigin: jsii.Bool(true),
			},
			{
				Bucket:                  clientBucket,
				PathPattern:             jsii.String("/l"),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_SPA,
				StripPrefixBeforeOrigin: jsii.Bool(true),
			},
		},
		SpaResponseHeadersPolicy: staticPolicy,
	})

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestStack.template.json")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(data, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	resources := mustResources(t, tpl)
	policyLogicalID, policy := findSingleResponseHeadersPolicy(t, resources)
	csp := extractResponseHeadersPolicyCSP(t, policy)

	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("CSP must not include unsafe directives: %s", csp)
	}
	requireContainsAll(t, csp, []string{
		"'sha256-QzWFZi+FLIx23tnm9SBU4aEgx4x8DsuASP07mfqol/c='",
		"'sha256-QJZDUlo/qa5AJCrG6vHyWcatjwCeWidEHQfJc601lzw='",
		"'sha256-eIXWvAmxkr251LJZkjniEK5LcPF3NkapbJepohwYRIc='",
		"'sha256-IV0HjYu959C/EiJIL2l/9Ty8PA4757JXhA/g112YXVE='",
		"'sha256-vv9IoKo7BSLbWcUHr3tNmfNVmm5L/9Cfn2H6LMk7/ow='",
	})

	dist := findSingleCloudFrontDistribution(t, resources)
	defaultBehavior := extractDefaultCacheBehavior(t, dist)
	if _, ok := defaultBehavior["ResponseHeadersPolicyId"]; ok {
		t.Fatalf("default behavior must not attach ResponseHeadersPolicyId (origin CSP should remain authoritative)")
	}

	cacheBehaviors := extractCacheBehaviors(t, dist)
	needPolicy := map[string]struct{}{
		"/auth":   {},
		"/auth/*": {},
		"/l":      {},
		"/l/*":    {},
	}
	for _, behavior := range cacheBehaviors {
		pathPattern, _ := behavior["PathPattern"].(string)
		_, shouldHave := needPolicy[pathPattern]
		raw, has := behavior["ResponseHeadersPolicyId"]
		if shouldHave && !has {
			t.Fatalf("behavior %q missing ResponseHeadersPolicyId", pathPattern)
		}
		if !shouldHave && has && strings.HasPrefix(pathPattern, "/auth/wallet") {
			t.Fatalf("behavior %q must not attach ResponseHeadersPolicyId (API-owned route)", pathPattern)
		}
		if !shouldHave || !has {
			continue
		}
		if !isRefTo(raw, policyLogicalID) {
			t.Fatalf("behavior %q ResponseHeadersPolicyId does not reference %s", pathPattern, policyLogicalID)
		}
	}
}

func findSingleResponseHeadersPolicy(t *testing.T, resources map[string]any) (string, map[string]any) {
	t.Helper()
	var logicalID string
	var res map[string]any
	for id, raw := range resources {
		typed, ok := raw.(map[string]any)
		if !ok || typed["Type"] != "AWS::CloudFront::ResponseHeadersPolicy" {
			continue
		}
		if logicalID != "" {
			t.Fatalf("expected one ResponseHeadersPolicy, found multiple (%s, %s)", logicalID, id)
		}
		logicalID = id
		res = typed
	}
	if logicalID == "" {
		t.Fatalf("expected ResponseHeadersPolicy resource")
	}
	return logicalID, res
}

func extractResponseHeadersPolicyCSP(t *testing.T, policy map[string]any) string {
	t.Helper()
	props, ok := policy["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("ResponseHeadersPolicy Properties missing or wrong type")
	}
	cfg, ok := props["ResponseHeadersPolicyConfig"].(map[string]any)
	if !ok {
		t.Fatalf("ResponseHeadersPolicyConfig missing or wrong type")
	}
	sec, ok := cfg["SecurityHeadersConfig"].(map[string]any)
	if !ok {
		t.Fatalf("SecurityHeadersConfig missing or wrong type")
	}
	csp, ok := sec["ContentSecurityPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("ContentSecurityPolicy missing or wrong type")
	}
	value, ok := csp["ContentSecurityPolicy"].(string)
	if !ok || strings.TrimSpace(value) == "" {
		t.Fatalf("ContentSecurityPolicy string missing or wrong type")
	}
	override, ok := csp["Override"].(bool)
	if !ok {
		t.Fatalf("ContentSecurityPolicy Override missing or wrong type")
	}
	if override {
		t.Fatalf("ContentSecurityPolicy Override must be false to preserve any origin-provided CSP")
	}
	return value
}

func findSingleCloudFrontDistribution(t *testing.T, resources map[string]any) map[string]any {
	t.Helper()
	var dist map[string]any
	for _, raw := range resources {
		typed, ok := raw.(map[string]any)
		if !ok || typed["Type"] != "AWS::CloudFront::Distribution" {
			continue
		}
		if dist != nil {
			t.Fatalf("expected one CloudFront Distribution, found multiple")
		}
		dist = typed
	}
	if dist == nil {
		t.Fatalf("expected CloudFront Distribution resource")
	}
	return dist
}

func extractDefaultCacheBehavior(t *testing.T, dist map[string]any) map[string]any {
	t.Helper()
	props, ok := dist["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("Distribution Properties missing or wrong type")
	}
	cfg, ok := props["DistributionConfig"].(map[string]any)
	if !ok {
		t.Fatalf("DistributionConfig missing or wrong type")
	}
	behavior, ok := cfg["DefaultCacheBehavior"].(map[string]any)
	if !ok {
		t.Fatalf("DefaultCacheBehavior missing or wrong type")
	}
	return behavior
}

func extractCacheBehaviors(t *testing.T, dist map[string]any) []map[string]any {
	t.Helper()
	props, ok := dist["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("Distribution Properties missing or wrong type")
	}
	cfg, ok := props["DistributionConfig"].(map[string]any)
	if !ok {
		t.Fatalf("DistributionConfig missing or wrong type")
	}

	raw, ok := cfg["CacheBehaviors"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		behavior, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, behavior)
	}
	return out
}

func isRefTo(raw any, want string) bool {
	ref, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	got, ok := ref["Ref"].(string)
	if !ok {
		return false
	}
	return got == want
}

func requireContainsAll(t *testing.T, haystack string, needles []string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			t.Fatalf("expected CSP to contain %q", needle)
		}
	}
}
