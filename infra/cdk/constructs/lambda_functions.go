package constructs

import (
	"fmt"
	"path/filepath"
	"strings"

	"cdk/inventory"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsevents"
	"github.com/aws/aws-cdk-go/awscdk/v2/awseventstargets"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssecretsmanager"
	"github.com/aws/jsii-runtime-go"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	apptheorycdk "github.com/theory-cloud/apptheory/cdk-go/apptheorycdk/v3"
)

type LambdaFunctionsProps struct {
	AppName             string
	Environment         string
	Domain              string
	LambdaAssetRoot     string
	Table               awsdynamodb.Table
	RateLimitTable      awsdynamodb.Table
	StreamEventsTable   awsdynamodb.Table
	MediaBucket         awss3.Bucket
	MediaDomain         string
	StreamingBucket     awss3.Bucket
	TrainingBucket      awss3.Bucket
	Queues              map[string]QueuePair
	PrivateKey          awssecretsmanager.ISecret
	JwtSecret           awssecretsmanager.ISecret
	MediaConvertRoleArn *string
	ModelMetadataTable  *string
	Config              map[string]interface{}
	EncryptionRole      awsiam.IRole
	BasicRole           awsiam.IRole
}

type LambdaFunctions struct {
	Functions map[string]awslambda.Function
}

// preAppTheoryLambdaLogicalIDs records the CloudFormation logical IDs generated
// by the native CDK Lambda construct before the AppTheoryFunction adoption.
//
// AppTheoryFunction wraps the underlying AWS::Lambda::Function under a nested
// "Function" child. That is idiomatic for new constructs, but migrating existing
// lesser Lambdas without preserving these logical IDs would ask CloudFormation to
// replace every function even though physical FunctionName values are unchanged.
// Keep this map for the pre-existing inventory Lambdas only; new inventory
// Lambdas should use AppTheoryFunction's natural logical IDs.
const preAppTheoryLambdaLogicalIDCount = 38

var preAppTheoryLambdaLogicalIDs = map[string]string{
	"activity-processor":            "activityprocessorFunction363D35B9",
	"actor":                         "actorFunction4F1B5B35",
	"ai-processor":                  "aiprocessorFunction80F9FEE9",
	"api":                           "apiFunction73599D16",
	"cms-scheduler":                 "cmsschedulerFunction7389FCD5",
	"collections":                   "collectionsFunctionB3956451",
	"cost-aggregator":               "costaggregatorFunction6A1DA294",
	"dlq-processor":                 "dlqprocessorFunction1B32771C",
	"enhanced-federation-processor": "enhancedfederationprocessorFunction1B66F94B",
	"export-generator":              "exportgeneratorFunctionF1137BBF",
	"federation-aggregator":         "federationaggregatorFunction2454A6CE",
	"federation-delivery":           "federationdeliveryFunction0574D778",
	"federation-timeseries":         "federationtimeseriesFunction5655FEB8",
	"federation-tracker":            "federationtrackerFunction998CD9D6",
	"graphql":                       "graphqlFunction067C62D4",
	"graphql-ws":                    "graphqlwsFunction2605C748",
	"import-processor":              "importprocessorFunction03DACDBF",
	"inbox":                         "inboxFunction22ED6D87",
	"media-processor":               "mediaprocessorFunction55166D54",
	"metrics-aggregator":            "metricsaggregatorFunction61E5EE49",
	"metrics-processor":             "metricsprocessorFunction985F65E4",
	"ml-training-processor":         "mltrainingprocessorFunction1F30198A",
	"moderation-processor":          "moderationprocessorFunction33714409",
	"note-processor":                "noteprocessorFunction71938A48",
	"notification-processor":        "notificationprocessorFunctionC222A047",
	"objects":                       "objectsFunction46AD58EB",
	"outbox":                        "outboxFunction51296F97",
	"push-delivery":                 "pushdeliveryFunctionB7BE0F3C",
	"report-trust-updater":          "reporttrustupdaterFunctionA287D274",
	"search-indexer":                "searchindexerFunction2D70A8F2",
	"severance-processor":           "severanceprocessorFunction879E1935",
	"sse":                           "sseFunction277E5041",
	"status-indexer":                "statusindexerFunction70FCBEBE",
	"stream-router":                 "streamrouterFunction3BCEAE5E",
	"streaming":                     "streamingFunction6A4849EF",
	"trend-aggregator":              "trendaggregatorFunction8820FAD6",
	"webfinger":                     "webfingerFunctionB71806D6",
	"websocket-cost-aggregator":     "websocketcostaggregatorFunction0068CAA1",
}

// Must returns the named function or panics if missing.
func (l *LambdaFunctions) Must(name string) awslambda.Function {
	if fn, ok := l.Functions[name]; ok {
		return fn
	}
	panic(fmt.Sprintf("lambda %s not found", name))
}

func CreateLambdaFunctions(stack awscdk.Stack, props *LambdaFunctionsProps) *LambdaFunctions {
	functions := &LambdaFunctions{Functions: make(map[string]awslambda.Function)}

	stage := naming.StageForEnvironment(props.Environment)

	appName := strings.TrimSpace(props.AppName)

	getConfigString := func(key string) string {
		if props.Config == nil {
			return ""
		}
		if val, ok := props.Config[key].(string); ok {
			return strings.TrimSpace(val)
		}
		return ""
	}

	if appName == "" {
		appName = naming.DefaultAppName
	}
	lambdaAssetRoot := strings.TrimSpace(props.LambdaAssetRoot)
	if lambdaAssetRoot == "" {
		lambdaAssetRoot = filepath.Clean(filepath.Join("..", ".."))
	}
	if absLambdaAssetRoot, err := filepath.Abs(lambdaAssetRoot); err == nil {
		lambdaAssetRoot = absLambdaAssetRoot
	}

	domainValue := strings.TrimSpace(props.Domain)
	if domainValue == "" {
		domainValue = "lesser.host"
	}

	// Common environment variables shared across functions.
	//
	// IMPORTANT: Lambda env vars have a strict 4KB limit. Keep this baseline as small as possible.
	// Add feature-specific values only when the runtime actually requires them.
	commonEnv := map[string]*string{
		// Environment selectors
		"ENVIRONMENT": jsii.String(props.Environment),
		"STAGE":       jsii.String(string(stage)),
		"APP_NAME":    jsii.String(appName),

		// Domain
		"DOMAIN_NAME": jsii.String(domainValue),

		// DynamoDB tables
		"DYNAMODB_TABLE":        props.Table.TableName(),
		"RATE_LIMIT_TABLE_NAME": props.RateLimitTable.TableName(),
		"LIMITED_TABLE_NAME":    props.RateLimitTable.TableName(), // limited library reads either key

		// Streaming table overrides (used by streaming/graphql-ws helpers)
		"CONNECTIONS_TABLE":   props.Table.TableName(),
		"SUBSCRIPTIONS_TABLE": props.Table.TableName(),

		// Media bucket
		"S3_BUCKET_NAME": props.MediaBucket.BucketName(),

		// ML moderation configuration (optional at runtime, but the resources always exist)
		"MODERATION_TRAINING_BUCKET_NAME": props.TrainingBucket.BucketName(),
		"MODERATION_MODEL_METADATA_TABLE": props.ModelMetadataTable,

		// Secrets
		"PRIVATE_KEY_SECRET": props.PrivateKey.SecretArn(),
		"KMS_KEY_ID":         jsii.String(fmt.Sprintf("alias/%s", naming.SharedResourceName(appName, "encryption"))), // KMS key for encrypting actor private keys
	}
	if mediaDomain := strings.TrimSpace(props.MediaDomain); mediaDomain != "" {
		commonEnv["CLOUDFRONT_DOMAIN"] = jsii.String(mediaDomain)
	}

	if v := getConfigString("lesserVersion"); v != "" {
		commonEnv["VERSION"] = jsii.String(v)
	}

	// Tips (TipSplitter integration). These are public, non-secret values and are only set when configured.
	if v := getConfigString("tipEnabled"); v != "" {
		commonEnv["TIP_ENABLED"] = jsii.String(v)
	}
	if v := getConfigString("tipChainId"); v != "" {
		commonEnv["TIP_CHAIN_ID"] = jsii.String(v)
	}
	if v := getConfigString("tipContractAddress"); v != "" {
		commonEnv["TIP_CONTRACT_ADDRESS"] = jsii.String(v)
	}

	// lesser.host trust services (optional; managed instances). Values are non-secret unless explicitly noted.
	if v := getConfigString("lesserHostUrl"); v != "" {
		commonEnv["LESSER_HOST_URL"] = jsii.String(v)
	}
	// NOTE: The ARN itself is not secret, but it points at the stored instance key.
	if v := getConfigString("lesserHostInstanceKeyArn"); v != "" {
		commonEnv["LESSER_HOST_INSTANCE_KEY_ARN"] = jsii.String(v)
	}
	if v := getConfigString("lesserHostAttestationsUrl"); v != "" {
		commonEnv["LESSER_HOST_ATTESTATIONS_URL"] = jsii.String(v)
	}
	// NOTE: Dedicated body/Ptah -> Lesser binding credential ARN; the ARN is non-secret,
	// but the resolved value must remain server-side only.
	if v := getConfigString("soulBindingIntegrationKeyArn"); v != "" {
		commonEnv["SOUL_BINDING_INTEGRATION_KEY_ARN"] = jsii.String(v)
	}

	// Optional tables
	if props.StreamEventsTable != nil {
		commonEnv["STREAM_EVENTS_TABLE_NAME"] = props.StreamEventsTable.TableName()
	}

	// WebSocket endpoint (used by streaming + notifications).
	wsHost := fmt.Sprintf("ws.%s", domainValue)
	// Streaming WebSocket API is path-mapped under /stream (see constructs/api_routes.go).
	commonEnv["WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("https://%s/stream", wsHost))
	// GraphQL WebSocket API is mapped at the root of ws.<domain> (see constructs/api_routes.go).
	commonEnv["GRAPHQL_WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("https://%s", wsHost))

	// Optional feature flags (set only when explicitly configured).
	if v := getConfigString("translationEnabled"); v != "" {
		commonEnv["TRANSLATION_ENABLED"] = jsii.String(v)
	}
	if v := getConfigString("allowAgents"); v != "" {
		commonEnv["ALLOW_AGENTS"] = jsii.String(v)
	}
	if v := getConfigString("allowAgentRegistration"); v != "" {
		commonEnv["ALLOW_AGENT_REGISTRATION"] = jsii.String(v)
	}
	if v := getConfigString("agentAccessTokenDuration"); v != "" {
		commonEnv["AGENT_ACCESS_TOKEN_DURATION"] = jsii.String(v)
	}
	if v := getConfigString("apiCorsAllowedOrigins"); v != "" {
		commonEnv["API_CORS_ALLOWED_ORIGINS"] = jsii.String(v)
	}

	// Set JWT secret ARN from SharedStack (securely passed, never synthesized)
	if props.JwtSecret != nil {
		commonEnv["JWT_SECRET_ARN"] = props.JwtSecret.SecretArn()
	}

	// Select baseline defaults by environment (Lift-aligned)
	defaults := inventory.LambdaInventory.Defaults
	if naming.IsLiveEnvironment(props.Environment) {
		defaults = inventory.ProductionDefaults
	}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		memory := defaults.MemoryMB
		if spec.Overrides.MemoryMB != nil {
			memory = *spec.Overrides.MemoryMB
		}
		timeout := defaults.TimeoutSeconds
		if spec.Overrides.TimeoutSeconds != nil {
			timeout = *spec.Overrides.TimeoutSeconds
		}
		retentionDays := defaults.LogRetentionDays
		if spec.Overrides.LogRetentionDays != nil {
			retentionDays = *spec.Overrides.LogRetentionDays
		}

		retention := awslogs.RetentionDays_ONE_WEEK
		if retentionDays >= 30 {
			retention = awslogs.RetentionDays_ONE_MONTH
		}

		role := props.BasicRole
		if spec.Role == inventory.RoleClassEncryption {
			role = props.EncryptionRole
		}

		functionName := naming.ResourceNameWithApp(appName, spec.Name, props.Environment)
		logGroup := awslogs.NewLogGroup(stack, jsii.String(spec.Name+"LogGroup"), &awslogs.LogGroupProps{
			LogGroupName:  jsii.String(fmt.Sprintf("/aws/lambda/%s", functionName)),
			Retention:     retention,
			RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
		})

		// Clone env per-Lambda. Queue URLs are injected after queues are created to keep this
		// baseline small and avoid the Lambda 4KB environment variable limit.
		env := copyEnv(commonEnv)

		for _, key := range spec.RequiredEnvVars {
			switch key {
			case "VAPID_PUBLIC_KEY":
				if v := getConfigString("vapidPublicKey"); v != "" {
					env[key] = jsii.String(v)
				}
			case "VAPID_SUBJECT":
				if v := getConfigString("vapidSubject"); v != "" {
					env[key] = jsii.String(v)
				}
			case "VAPID_SECRET_ARN":
				if v := getConfigString("vapidSecretArn"); v != "" {
					env[key] = jsii.String(v)
				}
			}
		}

		fnProps := awslambda.FunctionProps{
			Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
			Architecture: awslambda.Architecture_ARM_64(),
			MemorySize:   jsii.Number(memory),
			Timeout:      awscdk.Duration_Seconds(jsii.Number(timeout)),
			Environment:  &env,
			Tracing:      awslambda.Tracing_ACTIVE,
			FunctionName: jsii.String(functionName),
			Code:         awslambda.Code_FromAsset(jsii.String(filepath.ToSlash(filepath.Join(lambdaAssetRoot, "bin", spec.Name+".zip"))), nil),
			Handler:      jsii.String("bootstrap"),
			LogGroup:     logGroup,
			Role:         role,
		}

		if len(spec.ScheduleTriggers) > 0 {
			validateScheduleCapable(spec)
		}

		fn := newInventoryLambdaFunction(stack, spec, &fnProps)
		functions.Functions[spec.Name] = fn

		if len(spec.ScheduleTriggers) == 0 {
			continue
		}

		for idx, trig := range spec.ScheduleTriggers {
			ruleID := fmt.Sprintf("%sScheduleRule%d", sanitizeScheduleId(spec.Name), idx)
			ruleName := naming.ResourceNameWithApp(appName, fmt.Sprintf("%s-schedule-%d", spec.Name, idx), props.Environment)

			// Use an AppTheoryQueue-backed DLQ for scheduled invocations.
			// This ensures failures are captured without relying on bespoke queue wiring.
			dlqName := naming.ResourceNameWithApp(appName, fmt.Sprintf("%s-schedule-%d-dlq", spec.Name, idx), props.Environment)
			dlq := apptheorycdk.NewAppTheoryQueue(stack, jsii.String(fmt.Sprintf("%sScheduleDLQ%d", sanitizeScheduleId(spec.Name), idx)), &apptheorycdk.AppTheoryQueueProps{
				QueueName:         jsii.String(dlqName),
				EnableDlq:         jsii.Bool(false), // DLQ itself should not have a DLQ.
				RetentionPeriod:   awscdk.Duration_Days(jsii.Number(14)),
				RemovalPolicy:     awscdk.RemovalPolicy_DESTROY,
				VisibilityTimeout: awscdk.Duration_Minutes(jsii.Number(2)),
			})

			targetProps := &awseventstargets.LambdaFunctionProps{
				DeadLetterQueue: dlq.Queue(),
			}
			if trig.Input != "" {
				targetProps.Event = awsevents.RuleTargetInput_FromText(jsii.String(trig.Input))
			}

			apptheorycdk.NewAppTheoryEventBridgeHandler(stack, jsii.String(ruleID), &apptheorycdk.AppTheoryEventBridgeHandlerProps{
				Handler:     fn,
				Schedule:    awsevents.Schedule_Expression(jsii.String(trig.Expression)),
				RuleName:    jsii.String(ruleName),
				Enabled:     jsii.Bool(true),
				Description: jsii.String(fmt.Sprintf("Inventory-driven schedule for %s (%s)", spec.Name, trig.Expression)),
				TargetProps: targetProps,
			})
		}
	}

	return functions
}

func newInventoryLambdaFunction(stack awscdk.Stack, spec inventory.LambdaSpec, fnProps *awslambda.FunctionProps) awslambda.Function {
	if !usesAppTheoryFunction(spec) {
		return awslambda.NewFunction(stack, jsii.String(spec.Name+"Function"), fnProps)
	}

	appTheoryFn := apptheorycdk.NewAppTheoryFunction(stack, jsii.String(spec.Name+"Function"), appTheoryFunctionProps(fnProps))
	fn := appTheoryFn.Fn()
	preservePreAppTheoryLambdaLogicalID(fn, spec.Name)
	return fn
}

func usesAppTheoryFunction(spec inventory.LambdaSpec) bool {
	// Phase 3b adopts AppTheoryFunction only for Lambdas whose downstream
	// resources do not derive logical IDs from the Lambda construct path.
	// Stream/SQS mappings and schedule permissions need a separate parity proof
	// before their functions migrate, because AppTheoryFunction's nested child
	// path changes those dependent resource logical IDs even when the Lambda
	// resource itself is preserved.
	return len(spec.SQSTriggers) == 0 && len(spec.StreamTriggers) == 0 && len(spec.ScheduleTriggers) == 0
}

func appTheoryFunctionProps(props *awslambda.FunctionProps) *apptheorycdk.AppTheoryFunctionProps {
	return &apptheorycdk.AppTheoryFunctionProps{
		Runtime:      props.Runtime,
		Architecture: props.Architecture,
		MemorySize:   props.MemorySize,
		Timeout:      props.Timeout,
		Environment:  props.Environment,
		Tracing:      props.Tracing,
		FunctionName: props.FunctionName,
		Code:         props.Code,
		Handler:      props.Handler,
		LogGroup:     props.LogGroup,
		Role:         props.Role,
	}
}

func preservePreAppTheoryLambdaLogicalID(fn awslambda.Function, lambdaName string) {
	logicalID, ok := preAppTheoryLambdaLogicalIDs[lambdaName]
	if !ok {
		return
	}

	child := fn.Node().DefaultChild()
	cfn, ok := child.(awslambda.CfnFunction)
	if !ok {
		panic(fmt.Sprintf("lambda %s default child is %T, want awslambda.CfnFunction", lambdaName, child))
	}
	cfn.OverrideLogicalId(jsii.String(logicalID))
}

func copyEnv(src map[string]*string) map[string]*string {
	dst := make(map[string]*string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func queueURL(queueMap map[string]QueuePair, logical string) *string {
	if qp, ok := queueMap[logical]; ok && qp.Primary != nil {
		return qp.Primary.QueueUrl()
	}
	return jsii.String("")
}

func dlqURL(queueMap map[string]QueuePair, logical string) *string {
	if qp, ok := queueMap[logical]; ok && qp.DLQ != nil {
		return qp.DLQ.QueueUrl()
	}
	return jsii.String("")
}

// ApplyQueueEnvironmentVariables updates all Lambda environment variables that depend on
// queue URLs (canonical + inventory-driven aliases). This is useful when queues are
// created after Lambda functions (e.g., when using Lift queue constructs).
func ApplyQueueEnvironmentVariables(functions *LambdaFunctions, queues map[string]QueuePair) {
	if functions == nil {
		return
	}

	for _, fn := range functions.Functions {
		if fn == nil {
			continue
		}

		// Canonical queue URLs used by runtime config (pkg/config). Keep the list small to stay under
		// Lambda's 4KB env-var limit.
		fn.AddEnvironment(jsii.String("IMPORT_QUEUE_URL"), queueURL(queues, "import-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("EXPORT_QUEUE_URL"), queueURL(queues, "export-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("MEDIA_QUEUE_URL"), queueURL(queues, "media-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("SCHEDULED_QUEUE_URL"), queueURL(queues, "scheduled-queue"), nil)
		fn.AddEnvironment(jsii.String("FEDERATION_DELIVERY_QUEUE_URL"), queueURL(queues, "federation-delivery-queue"), nil)
		fn.AddEnvironment(jsii.String("PUSH_NOTIFICATION_QUEUE_URL"), queueURL(queues, "push-delivery-queue"), nil)
	}

	functions.Must("federation-aggregator").AddEnvironment(
		jsii.String("FEDERATION_AGGREGATOR_QUEUE_URL"),
		queueURL(queues, "federation-aggregator-queue"),
		nil,
	)
}
