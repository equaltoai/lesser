package stacks

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfront"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudfrontorigins"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func addSoulIngressBehavior(dist awscloudfront.Distribution, environment string) {
	if dist == nil {
		return
	}

	soulStage := "lab"
	if naming.IsLiveEnvironment(environment) {
		soulStage = "live"
	}

	functionUrl := awscdk.Fn_ImportValue(jsii.String(
		fmt.Sprintf("lesser-soul-%s-orchestrator-function-url", soulStage),
	))
	withoutScheme := awscdk.Fn_Select(
		jsii.Number(1),
		awscdk.Fn_Split(jsii.String("://"), functionUrl, nil),
	)
	host := awscdk.Fn_Select(jsii.Number(0), awscdk.Fn_Split(jsii.String("/"), withoutScheme, nil))

	origin := awscloudfrontorigins.NewHttpOrigin(host, &awscloudfrontorigins.HttpOriginProps{
		ProtocolPolicy: awscloudfront.OriginProtocolPolicy_HTTPS_ONLY,
	})

	dist.AddBehavior(jsii.String("/soul/*"), origin, &awscloudfront.AddBehaviorOptions{
		AllowedMethods:       awscloudfront.AllowedMethods_ALLOW_ALL(),
		CachedMethods:        awscloudfront.CachedMethods_CACHE_GET_HEAD_OPTIONS(),
		CachePolicy:          awscloudfront.CachePolicy_CACHING_DISABLED(),
		OriginRequestPolicy:  awscloudfront.OriginRequestPolicy_ALL_VIEWER_EXCEPT_HOST_HEADER(),
		ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
		Compress:             jsii.Bool(true),
	})
}
