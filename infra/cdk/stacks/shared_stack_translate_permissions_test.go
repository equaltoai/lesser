package stacks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
)

func TestSharedStackIncludesTranslatePermissions(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: jsii.String(outdir)})

	NewSharedStack(app, "TestSharedStack", &SharedStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String("123456789012"),
				Region:  jsii.String("us-east-1"),
			},
		},
		AppName: "lesser",
	})

	app.Synth(nil)

	templatePath := filepath.Join(outdir, "TestSharedStack.template.json")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	var tpl map[string]any
	if err := json.Unmarshal(data, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}

	resources := mustResources(t, tpl)
	policies := collectIAMPolicies(t, resources)
	if len(policies) == 0 {
		t.Fatalf("expected IAM policy resources for shared stack, found none")
	}

	found := false
	for _, policy := range policies {
		doc := extractPolicyDocument(t, policy)
		for _, statement := range extractPolicyStatements(t, doc) {
			actions := extractStatementActions(t, statement)
			if !containsString(actions, "translate:TranslateText") || !containsString(actions, "translate:ListLanguages") {
				continue
			}

			res := extractStatementResources(t, statement)
			for _, r := range res {
				if s, ok := r.(string); ok && s == "*" {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		t.Fatalf("expected shared stack roles to allow translate:TranslateText and translate:ListLanguages on '*'")
	}
}
