package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/jsii-runtime-go"
)

func TestSoulRoutingAddsCloudFrontBehaviorsWhenEnabled(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	dist := awscloudfront.NewDistribution(stack, jsii.String("ClientDistribution"), &awscloudfront.DistributionProps{
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin: awscloudfrontorigins.NewHttpOrigin(jsii.String("api.dev.example.com"), &awscloudfrontorigins.HttpOriginProps{
				ProtocolPolicy: awscloudfront.OriginProtocolPolicy_HTTPS_ONLY,
			}),
		},
	})

	addSoulOrchestratorRouting(stack, dist, "dev.example.com")

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
	distRes := findSingleCloudFrontDistribution(t, resources)

	cacheBehaviors := extractCacheBehaviors(t, distRes)
	wildcard := findCacheBehavior(t, cacheBehaviors, "/soul/*")
	exact := findCacheBehavior(t, cacheBehaviors, "/soul")

	wantCachePolicyID := awscloudfront.CachePolicy_CACHING_DISABLED().CachePolicyId()
	if wantCachePolicyID == nil || strings.TrimSpace(*wantCachePolicyID) == "" {
		t.Fatalf("managed caching-disabled CachePolicyId missing")
	}
	wantOriginRequestPolicyID := awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER().OriginRequestPolicyId()
	if wantOriginRequestPolicyID == nil || strings.TrimSpace(*wantOriginRequestPolicyID) == "" {
		t.Fatalf("managed ALL_VIEWER_EXCEPT_HOST_HEADER OriginRequestPolicyId missing")
	}

	requireBehaviorPolicyIDs(t, wildcard, *wantCachePolicyID, *wantOriginRequestPolicyID)
	requireBehaviorPolicyIDs(t, exact, *wantCachePolicyID, *wantOriginRequestPolicyID)
}

func findCacheBehavior(t *testing.T, behaviors []map[string]any, pathPattern string) map[string]any {
	t.Helper()

	for _, behavior := range behaviors {
		got, _ := behavior["PathPattern"].(string)
		if got == pathPattern {
			return behavior
		}
	}
	t.Fatalf("expected cache behavior for %q", pathPattern)
	return nil
}

func requireBehaviorPolicyIDs(t *testing.T, behavior map[string]any, wantCachePolicyID string, wantOriginRequestPolicyID string) {
	t.Helper()

	gotCachePolicyID, ok := behavior["CachePolicyId"].(string)
	if !ok || strings.TrimSpace(gotCachePolicyID) == "" {
		t.Fatalf("behavior %q missing CachePolicyId", behavior["PathPattern"])
	}
	if gotCachePolicyID != wantCachePolicyID {
		t.Fatalf("behavior %q unexpected CachePolicyId: got %q want %q", behavior["PathPattern"], gotCachePolicyID, wantCachePolicyID)
	}

	gotOriginRequestPolicyID, ok := behavior["OriginRequestPolicyId"].(string)
	if !ok || strings.TrimSpace(gotOriginRequestPolicyID) == "" {
		t.Fatalf("behavior %q missing OriginRequestPolicyId", behavior["PathPattern"])
	}
	if gotOriginRequestPolicyID != wantOriginRequestPolicyID {
		t.Fatalf("behavior %q unexpected OriginRequestPolicyId: got %q want %q", behavior["PathPattern"], gotOriginRequestPolicyID, wantOriginRequestPolicyID)
	}
}
