package constructs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	_jsii "github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
)

func TestAPIGatewayAccessLogsUseTemplatedResourcePath(t *testing.T) {
	outdir := t.TempDir()
	app := awscdk.NewApp(&awscdk.AppProps{Outdir: _jsii.String(outdir)})
	stack := awscdk.NewStack(app, _jsii.String("TestStack"), nil)

	dummy := awslambda.NewFunction(stack, _jsii.String("DummyFn"), &awslambda.FunctionProps{
		FunctionName: _jsii.String(naming.ResourceName("api", "development")),
		Runtime:      awslambda.Runtime_NODEJS_20_X(),
		Handler:      _jsii.String("index.handler"),
		Code:         awslambda.Code_FromInline(_jsii.String("exports.handler = async () => ({ statusCode: 200, body: 'ok' });")),
	})

	functions := &LambdaFunctions{
		Functions: map[string]awslambda.Function{
			"api":         dummy,
			"graphql":     dummy,
			"graphql-ws":  dummy,
			"sse":         dummy,
			"streaming":   dummy,
			"actor":       dummy,
			"collections": dummy,
			"inbox":       dummy,
			"objects":     dummy,
			"outbox":      dummy,
			"webfinger":   dummy,
		},
	}

	_ = CreateAPIGateway(stack, &APIGatewayProps{
		AppName:     naming.DefaultAppName,
		Environment: "development",
		Functions:   functions,
	})

	template := synthStackTemplate(t, app, outdir, "TestStack")
	encoded, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("marshal template: %v", err)
	}
	templateText := string(encoded)
	if strings.Contains(templateText, "$context.path") {
		t.Fatalf("API Gateway access logs must not persist raw $context.path: %s", templateText)
	}
	if !strings.Contains(templateText, `\"path\":\"$context.resourcePath\"`) {
		t.Fatalf("expected access log path field to use $context.resourcePath")
	}

	gotRoutes := extractHttpRouteToFunctionName(t, template)
	routeKey := "GET /api/v1/souls/bound/me/mint-conversations/{conversationId}"
	if got, ok := gotRoutes[routeKey]; !ok || got != naming.ResourceName("api", "development") {
		t.Fatalf("expected explicit private mint-conversation route to integrate with api lambda (got %q, present=%v)", got, ok)
	}
}

func synthStackTemplate(t *testing.T, app awscdk.App, outdir string, stackName string) map[string]any {
	t.Helper()

	app.Synth(nil)
	templatePath := filepath.Join(outdir, stackName+".template.json")
	b, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template %s: %v", templatePath, err)
	}

	var template map[string]any
	if err := json.Unmarshal(b, &template); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	return template
}
