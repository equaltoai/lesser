package stacks

import (
	"sort"
	"strings"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

var configuredSecretARNContextKeys = []string{
	"lesserHostInstanceKeyArn",
	"soulBindingIntegrationKeyArn",
	"vapidSecretArn",
}

func attachConfiguredSecretReadPolicy(scope awscdk.Stack, appName string, environment string, roles []awsiam.IRole, config map[string]interface{}) {
	if scope == nil || len(config) == 0 {
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

	resources := configuredSecretARNs(config)
	if len(resources) == 0 {
		return
	}

	statement := awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
		Effect: awsiam.Effect_ALLOW,
		Actions: &[]*string{
			jsii.String("secretsmanager:GetSecretValue"),
			jsii.String("secretsmanager:DescribeSecret"),
		},
		Resources: &resources,
	})

	policyName := naming.ResourceNameWithApp(appName, "configured-secret-read", environment)
	awsiam.NewPolicy(scope, jsii.String("ConfiguredSecretReadPolicy"), &awsiam.PolicyProps{
		PolicyName: jsii.String(policyName),
		Roles:      &filteredRoles,
		Statements: &[]awsiam.PolicyStatement{statement},
	})
}

func configuredSecretARNs(config map[string]interface{}) []*string {
	if len(config) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	values := make([]string, 0, len(configuredSecretARNContextKeys))
	for _, key := range configuredSecretARNContextKeys {
		raw, ok := config[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}

	sort.Strings(values)

	resources := make([]*string, 0, len(values))
	for _, value := range values {
		resources = append(resources, jsii.String(value))
	}
	return resources
}
