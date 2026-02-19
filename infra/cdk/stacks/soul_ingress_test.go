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

func TestFrontendDistributionAddsSoulBehaviorWithNoCachingAndAuthForwarding(t *testing.T) {
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
		// Forward all viewer headers (including Authorization) + query strings; exclude Host to avoid origin mismatch.
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
		},
		SpaResponseHeadersPolicy: staticPolicy,
	})

	addSoulIngressBehavior(frontend.Distribution(), "development")

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
	dist := findSingleCloudFrontDistribution(t, resources)
	cacheBehaviors := extractCacheBehaviors(t, dist)

	var soulBehavior map[string]any
	for _, behavior := range cacheBehaviors {
		pathPattern, _ := behavior["PathPattern"].(string)
		if pathPattern == "/soul/*" {
			soulBehavior = behavior
			break
		}
	}
	if soulBehavior == nil {
		t.Fatalf("expected cache behavior for /soul/*")
	}

	wantCachePolicyID := awscloudfront.CachePolicy_CACHING_DISABLED().CachePolicyId()
	gotCachePolicyID, ok := soulBehavior["CachePolicyId"].(string)
	if !ok || strings.TrimSpace(gotCachePolicyID) == "" {
		t.Fatalf("behavior /soul/* missing CachePolicyId")
	}
	if wantCachePolicyID != nil && gotCachePolicyID != *wantCachePolicyID {
		t.Fatalf("behavior /soul/* unexpected CachePolicyId: got %q want %q", gotCachePolicyID, *wantCachePolicyID)
	}

	wantOriginRequestPolicyID := awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER().OriginRequestPolicyId()
	gotOriginRequestPolicyID, ok := soulBehavior["OriginRequestPolicyId"].(string)
	if !ok || strings.TrimSpace(gotOriginRequestPolicyID) == "" {
		t.Fatalf("behavior /soul/* missing OriginRequestPolicyId")
	}
	if wantOriginRequestPolicyID != nil && gotOriginRequestPolicyID != *wantOriginRequestPolicyID {
		t.Fatalf("behavior /soul/* unexpected OriginRequestPolicyId: got %q want %q", gotOriginRequestPolicyID, *wantOriginRequestPolicyID)
	}

	allowedMethodsRaw, ok := soulBehavior["AllowedMethods"]
	if !ok {
		t.Fatalf("behavior /soul/* missing AllowedMethods")
	}

	var itemsAny []any
	switch typed := allowedMethodsRaw.(type) {
	case []any:
		itemsAny = typed
	case map[string]any:
		got, ok := typed["Items"].([]any)
		if !ok {
			t.Fatalf("behavior /soul/* AllowedMethods Items missing or wrong type")
		}
		itemsAny = got
	default:
		t.Fatalf("behavior /soul/* AllowedMethods has unexpected type %T", allowedMethodsRaw)
	}

	var hasPost bool
	for _, item := range itemsAny {
		if method, ok := item.(string); ok && method == "POST" {
			hasPost = true
			break
		}
	}
	if !hasPost {
		t.Fatalf("behavior /soul/* AllowedMethods must include POST")
	}
}
