package stacks

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigatewayv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func attachWebSocketManageConnectionsPolicy(scope awscdk.Stack, appName string, environment string, roles []awsiam.IRole, apis []awsapigatewayv2.WebSocketApi) {
	if scope == nil {
		return
	}
	if len(roles) == 0 || len(apis) == 0 {
		return
	}

	var filteredRoles []awsiam.IRole
	for _, role := range roles {
		if role != nil {
			filteredRoles = append(filteredRoles, role)
		}
	}
	if len(filteredRoles) == 0 {
		return
	}

	stage := string(naming.StageForEnvironment(environment))
	if stage == "" {
		stage = string(naming.StageDev)
	}

	var resources []*string
	for _, api := range apis {
		if api == nil {
			continue
		}
		resources = append(resources, executeAPIManageConnectionsResource(api.ApiId(), stage))
	}
	if len(resources) == 0 {
		return
	}

	statement := awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("execute-api:ManageConnections"),
		},
		Resources: &resources,
	})

	policyName := naming.ResourceNameWithApp(appName, "execute-api-manage-connections", environment)
	awsiam.NewPolicy(scope, jsii.String("ExecuteApiManageConnectionsPolicy"), &awsiam.PolicyProps{
		PolicyName: jsii.String(policyName),
		Roles:      &filteredRoles,
		Statements: &[]awsiam.PolicyStatement{statement},
	})
}

func executeAPIManageConnectionsResource(apiID *string, stage string) *string {
	return awscdk.Fn_Sub(
		jsii.String("arn:${AWS::Partition}:execute-api:${AWS::Region}:${AWS::AccountId}:${ApiId}/${Stage}/POST/@connections/*"),
		&map[string]*string{
			"ApiId": apiID,
			"Stage": jsii.String(stage),
		},
	)
}
