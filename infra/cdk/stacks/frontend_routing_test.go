package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	localconstructs "cdk/constructs"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk"
)

func TestFrontendDistributionForwardsOAuthQueryStringsAndHandlesBasePaths(t *testing.T) {
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

	frontend := apptheorycdk.NewAppTheoryPathRoutedFrontend(stack, jsii.String("ClientFrontend"), &apptheorycdk.AppTheoryPathRoutedFrontendProps{
		ApiOriginUrl: jsii.String("api.dev.example.com"),
		// Forward all query strings to the API origin (required for OAuth /oauth/authorize PKCE params).
		ApiOriginRequestPolicy: awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
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
				Bucket:                  clientBucket,
				PathPattern:             jsii.String("/l/*"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_SPA,
			},
			{
				Bucket:                  authBucket,
				PathPattern:             jsii.String("/auth/*"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_NONE,
			},
			{
				Bucket:                  clientBucket,
				PathPattern:             jsii.String("/l"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_SPA,
			},
			{
				Bucket:                  authBucket,
				PathPattern:             jsii.String("/auth"),
				ResponseHeadersPolicy:   staticPolicy,
				StripPrefixBeforeOrigin: jsii.Bool(true),
				RewriteMode:             apptheorycdk.AppTheorySpaRewriteMode_NONE,
			},
		},
		SpaResponseHeadersPolicy: staticPolicy,
	})

	overridePathRoutedFrontendRewriteFunction(frontend)

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

	// Issue #53: API origin must forward query strings (CloudFront OriginRequestPolicy).
	dist := findSingleCloudFrontDistribution(t, resources)
	defaultBehavior := extractDefaultCacheBehavior(t, dist)
	wantPolicyID := awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER().OriginRequestPolicyId()
	gotPolicyID, ok := defaultBehavior["OriginRequestPolicyId"].(string)
	if !ok || strings.TrimSpace(gotPolicyID) == "" {
		t.Fatalf("default behavior missing OriginRequestPolicyId")
	}
	if wantPolicyID != nil && gotPolicyID != *wantPolicyID {
		t.Fatalf("unexpected OriginRequestPolicyId: got %q want %q", gotPolicyID, *wantPolicyID)
	}

	cacheBehaviors := extractCacheBehaviors(t, dist)
	var foundBypass bool
	for _, behavior := range cacheBehaviors {
		pathPattern, _ := behavior["PathPattern"].(string)
		if pathPattern != "/auth/wallet/*" {
			continue
		}
		foundBypass = true
		got, ok := behavior["OriginRequestPolicyId"].(string)
		if !ok || strings.TrimSpace(got) == "" {
			t.Fatalf("behavior %q missing OriginRequestPolicyId", pathPattern)
		}
		if wantPolicyID != nil && got != *wantPolicyID {
			t.Fatalf("behavior %q unexpected OriginRequestPolicyId: got %q want %q", pathPattern, got, *wantPolicyID)
		}
	}
	if !foundBypass {
		t.Fatalf("expected API bypass behavior for /auth/wallet/*")
	}

	// Issue #54: rewrite function must handle exact-prefix paths and directory-index semantics for auth-ui routes.
	fn := findSingleCloudFrontFunction(t, resources)
	code := extractCloudFrontFunctionCode(t, fn)
	requireContainsAll(t, code, []string{
		"cleanPrefix: '/auth'",
		"cleanPrefix: '/l'",
		"uri === cfg.cleanPrefix",
		"uri.indexOf(cfg.prefix) !== 0",
		"uri + '/index.html'",
	})
}

func findSingleCloudFrontFunction(t *testing.T, resources map[string]any) map[string]any {
	t.Helper()
	var fn map[string]any
	for _, raw := range resources {
		typed, ok := raw.(map[string]any)
		if !ok || typed["Type"] != "AWS::CloudFront::Function" {
			continue
		}
		if fn != nil {
			t.Fatalf("expected one CloudFront Function, found multiple")
		}
		fn = typed
	}
	if fn == nil {
		t.Fatalf("expected CloudFront Function resource")
	}
	return fn
}

func extractCloudFrontFunctionCode(t *testing.T, fn map[string]any) string {
	t.Helper()
	props, ok := fn["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("CloudFront Function Properties missing or wrong type")
	}
	raw, ok := props["FunctionCode"]
	if !ok {
		t.Fatalf("CloudFront Function missing FunctionCode")
	}
	code, ok := raw.(string)
	if ok {
		return code
	}

	// CDK can represent large strings via Fn::Join; normalize to a string for matching.
	if joined, ok := raw.(map[string]any); ok {
		if joinAny, ok := joined["Fn::Join"]; ok {
			parts, ok := joinAny.([]any)
			if ok && len(parts) == 2 {
				sep, _ := parts[0].(string)
				list, _ := parts[1].([]any)
				out := make([]string, 0, len(list))
				for _, entry := range list {
					if s, ok := entry.(string); ok {
						out = append(out, s)
					}
				}
				return strings.Join(out, sep)
			}
		}
	}

	t.Fatalf("CloudFront Function FunctionCode has unexpected type %T", raw)
	return ""
}
