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
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53targets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

func TestFrontendDistributionForwardsOAuthQueryStringsAndHandlesBasePaths(t *testing.T) {
	resources := synthClientFrontendResources(t)

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
	if _, has := defaultBehavior["FunctionAssociations"]; !has {
		t.Fatalf("default behavior must attach the rewrite function to preserve forwarded host headers")
	}

	cacheBehaviors := extractCacheBehaviors(t, dist)
	bypass := findCacheBehaviorByPathPattern(t, cacheBehaviors, "/auth/wallet/*")
	got, ok := bypass["OriginRequestPolicyId"].(string)
	if !ok || strings.TrimSpace(got) == "" {
		t.Fatalf("behavior %q missing OriginRequestPolicyId", "/auth/wallet/*")
	}
	if wantPolicyID != nil && got != *wantPolicyID {
		t.Fatalf("behavior %q unexpected OriginRequestPolicyId: got %q want %q", "/auth/wallet/*", got, *wantPolicyID)
	}
	if _, has := bypass["FunctionAssociations"]; !has {
		t.Fatalf("behavior %q must attach the rewrite function to preserve forwarded host headers", "/auth/wallet/*")
	}

	findCacheBehaviorByPathPattern(t, cacheBehaviors, "/auth")
	findCacheBehaviorByPathPattern(t, cacheBehaviors, "/auth/*")
	findCacheBehaviorByPathPattern(t, cacheBehaviors, "/l")
	findCacheBehaviorByPathPattern(t, cacheBehaviors, "/l/*")
	findCacheBehaviorByPathPattern(t, cacheBehaviors, "/l/_assets/*")
	requirePathPatternPrecedes(t, cacheBehaviors, "/l/_assets/*", "/l/*")

	// Issue #54/#587: rewrite function must normalize /l and preserve directory-index semantics for auth-ui routes.
	fn := findSingleCloudFrontFunction(t, resources)
	code := extractCloudFrontFunctionCode(t, fn)
	requireContainsAll(t, code, []string{
		"var host = request.headers.host;",
		"request.headers['x-forwarded-host'] = { value: host.value };",
		"request.headers['x-forwarded-proto'] = { value: 'https' };",
		"if (uri === '/l')",
		"request.uri = '/l/';",
		"if (uri === '/auth')",
		"if (uri.indexOf('/auth/') === 0)",
		"request.uri = '/auth/index.html';",
		"uri + '/index.html'",
	})
}

func TestClientFrontendLogicalIDsRemainStable(t *testing.T) {
	resources := synthClientFrontendResources(t)

	requireResourceLogicalID(t, resources, "ClientFrontendDistribution015B354C")
	requireResourceLogicalID(t, resources, "ClientFrontendAliasRecordD250F593")
	requireResourceLogicalID(t, resources, "ClientFrontendAliasRecordAAAACA5BB675")

	requireResourceLogicalIDAbsent(t, resources, "ClientFrontend6814B143")
	requireResourceLogicalIDPrefixAbsent(t, resources, "FrontendAliasARecord")
	requireResourceLogicalIDPrefixAbsent(t, resources, "FrontendAliasAAAARecord")
}

func TestClientSSRFunctionURLOriginPermissions(t *testing.T) {
	resources := synthClientFrontendResources(t)

	invokeURLPermission := findLambdaPermissionByAction(t, resources, "lambda:InvokeFunctionUrl")
	invokeURLProps := extractLambdaPermissionProperties(t, invokeURLPermission)
	if got, _ := invokeURLProps["Principal"].(string); got != "cloudfront.amazonaws.com" {
		t.Fatalf("InvokeFunctionUrl permission principal = %q, want cloudfront.amazonaws.com", got)
	}
	requireContainsAll(t, mustJSON(t, invokeURLProps["SourceArn"]), []string{
		"cloudfront",
		"distribution",
		"ClientFrontendDistribution015B354C",
	})

	invokePermission := findLambdaPermissionByAction(t, resources, "lambda:InvokeFunction")
	invokeProps := extractLambdaPermissionProperties(t, invokePermission)
	if got, _ := invokeProps["Principal"].(string); got != "cloudfront.amazonaws.com" {
		t.Fatalf("InvokeFunction permission principal = %q, want cloudfront.amazonaws.com", got)
	}
	invokedViaURL, ok := invokeProps["InvokedViaFunctionUrl"].(bool)
	if !ok || !invokedViaURL {
		t.Fatalf("InvokeFunction permission must set InvokedViaFunctionUrl=true")
	}
	requireContainsAll(t, mustJSON(t, invokeProps["SourceArn"]), []string{
		"cloudfront",
		"distribution",
		"ClientFrontendDistribution015B354C",
	})
}

func synthClientFrontendResources(t *testing.T) map[string]any {
	t.Helper()

	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})
	stack := awscdk.NewStack(app, jsii.String("TestStack"), &awscdk.StackProps{
		Env: &awscdk.Environment{
			Account: jsii.String("123456789012"),
			Region:  jsii.String("us-east-1"),
		},
	})

	authPolicy := localconstructs.NewFrontendStaticResponseHeadersPolicy(stack, jsii.String("dev.example.com"))
	clientPolicy := localconstructs.NewClientSSRResponseHeadersPolicy(stack)
	rewriteFn := newClientFrontendRewriteFunction(stack)
	hostedZone := awsroute53.NewHostedZone(stack, jsii.String("HostedZone"), &awsroute53.HostedZoneProps{
		ZoneName: jsii.String("example.com"),
	})

	clientBucket := awss3.NewBucket(stack, jsii.String("ClientBucket"), &awss3.BucketProps{
		BucketName: jsii.String("test-dev-client-123456789012-us-east-1"),
	})
	authBucket := awss3.NewBucket(stack, jsii.String("AuthBucket"), &awss3.BucketProps{
		BucketName: jsii.String("test-dev-auth-ui-123456789012-us-east-1"),
	})

	clientSSRFn := awslambda.NewFunction(stack, jsii.String("ClientSSRHostFunction"), &awslambda.FunctionProps{
		Runtime: awslambda.Runtime_NODEJS_22_X(),
		Handler: jsii.String("index.handler"),
		Code:    awslambda.Code_FromInline(jsii.String("exports.handler = async () => ({ statusCode: 200, body: 'ok' });")),
	})
	clientSSRURL := clientSSRFn.AddFunctionUrl(&awslambda.FunctionUrlOptions{
		AuthType: awslambda.FunctionUrlAuthType_AWS_IAM,
	})

	apiOriginTarget := awscloudfrontorigins.NewHttpOrigin(jsii.String("api.dev.example.com"), nil)
	authOrigin := awscloudfrontorigins.S3BucketOrigin_WithOriginAccessControl(authBucket, nil)
	clientAssetOrigin := awscloudfrontorigins.S3BucketOrigin_WithOriginAccessControl(clientBucket, nil)
	clientSSROrigin := awscloudfrontorigins.FunctionUrlOrigin_WithOriginAccessControl(clientSSRURL, &awscloudfrontorigins.FunctionUrlOriginWithOACProps{
		ReadTimeout: awscdk.Duration_Seconds(jsii.Number(30)),
	})
	functionAssociations := &[]*awscloudfront.FunctionAssociation{
		{
			EventType: awscloudfront.FunctionEventType_VIEWER_REQUEST,
			Function:  rewriteFn,
		},
	}

	clientFrontend := constructs.NewConstruct(stack, jsii.String("ClientFrontend"))
	dist := awscloudfront.NewDistribution(clientFrontend, jsii.String("Distribution"), &awscloudfront.DistributionProps{
		DefaultBehavior: &awscloudfront.BehaviorOptions{
			Origin:               apiOriginTarget,
			ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
			OriginRequestPolicy:  awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
			AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_ALL(),
			CachePolicy:          awscloudfront.CachePolicy_CACHING_DISABLED(),
			FunctionAssociations: functionAssociations,
		},
		PriceClass: awscloudfront.PriceClass_PRICE_CLASS_100,
	})
	grantCloudFrontFunctionURLInvokePermission(clientSSRFn, dist)
	dist.AddBehavior(jsii.String("/auth/wallet/*"), apiOriginTarget, &awscloudfront.AddBehaviorOptions{
		ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		OriginRequestPolicy:  awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
		AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_ALL(),
		CachePolicy:          awscloudfront.CachePolicy_CACHING_DISABLED(),
		FunctionAssociations: functionAssociations,
	})
	dist.AddBehavior(jsii.String("/auth"), authOrigin, &awscloudfront.AddBehaviorOptions{
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		ResponseHeadersPolicy: authPolicy,
		FunctionAssociations:  functionAssociations,
	})
	dist.AddBehavior(jsii.String("/auth/*"), authOrigin, &awscloudfront.AddBehaviorOptions{
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		ResponseHeadersPolicy: authPolicy,
		FunctionAssociations:  functionAssociations,
	})
	dist.AddBehavior(jsii.String("/l"), clientSSROrigin, &awscloudfront.AddBehaviorOptions{
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		AllowedMethods:        awscloudfront.AllowedMethods_ALLOW_ALL(),
		CachePolicy:           awscloudfront.CachePolicy_CACHING_DISABLED(),
		OriginRequestPolicy:   awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
		ResponseHeadersPolicy: clientPolicy,
		FunctionAssociations:  functionAssociations,
	})
	dist.AddBehavior(jsii.String("/l/_assets/*"), clientAssetOrigin, &awscloudfront.AddBehaviorOptions{
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		CachePolicy:           awscloudfront.CachePolicy_CACHING_OPTIMIZED(),
		ResponseHeadersPolicy: clientPolicy,
		FunctionAssociations:  functionAssociations,
	})
	dist.AddBehavior(jsii.String("/l/*"), clientSSROrigin, &awscloudfront.AddBehaviorOptions{
		ViewerProtocolPolicy:  awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		AllowedMethods:        awscloudfront.AllowedMethods_ALLOW_ALL(),
		CachePolicy:           awscloudfront.CachePolicy_CACHING_DISABLED(),
		OriginRequestPolicy:   awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
		ResponseHeadersPolicy: clientPolicy,
		FunctionAssociations:  functionAssociations,
	})
	recordName := relativeRecordName("dev.example.com", hostedZone)
	target := awsroute53targets.NewCloudFrontTarget(dist)
	awsroute53.NewARecord(clientFrontend, jsii.String("AliasRecord"), &awsroute53.ARecordProps{
		Zone:       hostedZone,
		RecordName: recordName,
		Target:     awsroute53.RecordTarget_FromAlias(target),
	})
	awsroute53.NewAaaaRecord(clientFrontend, jsii.String("AliasRecordAAAA"), &awsroute53.AaaaRecordProps{
		Zone:       hostedZone,
		RecordName: recordName,
		Target:     awsroute53.RecordTarget_FromAlias(target),
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

	return mustResources(t, tpl)
}

func requireResourceLogicalID(t *testing.T, resources map[string]any, logicalID string) {
	t.Helper()
	if _, ok := resources[logicalID]; !ok {
		t.Fatalf("expected logical ID %q in synthesized template", logicalID)
	}
}

func requireResourceLogicalIDAbsent(t *testing.T, resources map[string]any, logicalID string) {
	t.Helper()
	if _, ok := resources[logicalID]; ok {
		t.Fatalf("expected logical ID %q to be absent from synthesized template", logicalID)
	}
}

func requireResourceLogicalIDPrefixAbsent(t *testing.T, resources map[string]any, prefix string) {
	t.Helper()
	for logicalID := range resources {
		if strings.HasPrefix(logicalID, prefix) {
			t.Fatalf("expected no logical IDs with prefix %q, found %q", prefix, logicalID)
		}
	}
}

func findLambdaPermissionByAction(t *testing.T, resources map[string]any, action string) map[string]any {
	t.Helper()
	var permission map[string]any
	for _, raw := range resources {
		typed, ok := raw.(map[string]any)
		if !ok || typed["Type"] != "AWS::Lambda::Permission" {
			continue
		}
		props := extractLambdaPermissionProperties(t, typed)
		got, _ := props["Action"].(string)
		if got != action {
			continue
		}
		if permission != nil {
			t.Fatalf("expected one Lambda permission for %q, found multiple", action)
		}
		permission = typed
	}
	if permission == nil {
		t.Fatalf("expected Lambda permission for %q", action)
	}
	return permission
}

func extractLambdaPermissionProperties(t *testing.T, permission map[string]any) map[string]any {
	t.Helper()
	props, ok := permission["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("Lambda Permission Properties missing or wrong type")
	}
	return props
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return string(data)
}

func findCacheBehaviorByPathPattern(t *testing.T, behaviors []map[string]any, pathPattern string) map[string]any {
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

func requirePathPatternPrecedes(t *testing.T, behaviors []map[string]any, earlier string, later string) {
	t.Helper()

	earlierIndex := -1
	laterIndex := -1
	for i, behavior := range behaviors {
		got, _ := behavior["PathPattern"].(string)
		switch got {
		case earlier:
			earlierIndex = i
		case later:
			laterIndex = i
		}
	}

	if earlierIndex == -1 {
		t.Fatalf("expected cache behavior for %q", earlier)
	}
	if laterIndex == -1 {
		t.Fatalf("expected cache behavior for %q", later)
	}
	if earlierIndex >= laterIndex {
		t.Fatalf("expected cache behavior %q to precede %q, got indexes %d >= %d", earlier, later, earlierIndex, laterIndex)
	}
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
