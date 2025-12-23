package constructs

import (
	"fmt"
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
	liftcdk "github.com/pay-theory/lift/pkg/cdk/constructs"
)

type LambdaFunctionsProps struct {
	AppName             string
	Environment         string
	Domain              string
	Table               awsdynamodb.Table
	RateLimitTable      awsdynamodb.Table
	StreamEventsTable   awsdynamodb.Table
	MediaBucket         awss3.Bucket
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

	// Helper to get config values
	getConfigString := func(key string, defaultVal string) *string {
		if props.Config != nil {
			if val, ok := props.Config[key].(string); ok {
				return jsii.String(val)
			}
		}
		return jsii.String(defaultVal)
	}

	if appName == "" {
		if appPtr := getConfigString("appName", naming.DefaultAppName); appPtr != nil && *appPtr != "" {
			appName = *appPtr
		} else {
			appName = naming.DefaultAppName
		}
	}

	domainValue := strings.TrimSpace(props.Domain)
	if domainValue == "" {
		domainValue = "lesser.host"
		if domainPtr := getConfigString("domain", "lesser.host"); domainPtr != nil && *domainPtr != "" {
			domainValue = *domainPtr
		}
	}

	// Common environment variables shared across functions
	commonEnv := map[string]*string{
		// Environment selectors (D1)
		"ENVIRONMENT": jsii.String(props.Environment),
		"STAGE":       jsii.String(string(stage)),
		"APP_NAME":    jsii.String(appName),

		// Domain (D2)
		"DOMAIN_NAME": jsii.String(domainValue),
		"DOMAIN":      jsii.String(domainValue),

		// DynamoDB tables (D3)
		"DYNAMODB_TABLE":        props.Table.TableName(),
		"DYNAMO_TABLE_NAME":     props.Table.TableName(),
		"RATE_LIMIT_TABLE_NAME": props.RateLimitTable.TableName(),
		"LIMITED_TABLE_NAME":    props.RateLimitTable.TableName(), // For limited library
		"CONNECTIONS_TABLE":     props.Table.TableName(),
		"SUBSCRIPTIONS_TABLE":   props.Table.TableName(),
		"STREAM_EVENTS_TABLE_NAME": func() *string {
			if props.StreamEventsTable != nil {
				return props.StreamEventsTable.TableName()
			}
			return jsii.String("")
		}(),

		// Media bucket aliases (D4)
		"S3_BUCKET_NAME":    props.MediaBucket.BucketName(),
		"S3_BUCKET":         props.MediaBucket.BucketName(),
		"S3_MEDIA_BUCKET":   props.MediaBucket.BucketName(),
		"MEDIA_BUCKET_NAME": props.MediaBucket.BucketName(),

		// Canonical queues + aliases (D7)
		"IMPORT_QUEUE_URL":              queueURL(props.Queues, "import-processor-queue"),
		"IMPORT_PROCESSOR_QUEUE_URL":    queueURL(props.Queues, "import-processor-queue"),
		"EXPORT_QUEUE_URL":              queueURL(props.Queues, "export-processor-queue"),
		"EXPORT_PROCESSOR_QUEUE_URL":    queueURL(props.Queues, "export-processor-queue"),
		"MEDIA_QUEUE_URL":               queueURL(props.Queues, "media-processor-queue"),
		"MEDIA_PROCESSOR_QUEUE_URL":     queueURL(props.Queues, "media-processor-queue"),
		"SCHEDULED_QUEUE_URL":           queueURL(props.Queues, "scheduled-queue"),
		"FEDERATION_DELIVERY_QUEUE_URL": queueURL(props.Queues, "federation-delivery-queue"),
		"FEDERATION_QUEUE_URL":          queueURL(props.Queues, "federation-delivery-queue"),
		"FEDERATION_DLQ_URL":            dlqURL(props.Queues, "federation-delivery-queue"),
		"PUSH_NOTIFICATION_QUEUE_URL":   queueURL(props.Queues, "push-delivery-queue"),
		"PUSH_QUEUE_URL":                queueURL(props.Queues, "push-delivery-queue"),

		// Secrets (D5, D6)
		"PRIVATE_KEY_SECRET":     props.PrivateKey.SecretArn(),
		"PRIVATE_KEY_SECRET_ARN": props.PrivateKey.SecretArn(),                                                           // optional alias (Spec 05)
		"KMS_KEY_ID":             jsii.String(fmt.Sprintf("alias/%s", naming.SharedResourceName(appName, "encryption"))), // KMS key for encrypting actor private keys

		// Media CDN and instance metadata (unchanged)
		"CDN_DOMAIN":           jsii.String("REPLACE_WITH_MEDIA_DOMAIN"), // Set by CDK context
		"INSTANCE_TITLE":       jsii.String("Lesser Instance"),
		"INSTANCE_SHORT_DESC":  jsii.String("A personal ActivityPub server"),
		"INSTANCE_DESCRIPTION": jsii.String("A lightweight, serverless ActivityPub implementation"),
		"INSTANCE_ADMIN_EMAIL": jsii.String("REPLACE_WITH_ADMIN_EMAIL"), // Set by CDK context
		"REGISTRATIONS_OPEN":   jsii.String("false"),
		"APPROVAL_REQUIRED":    jsii.String("true"),
		"INVITES_ENABLED":      jsii.String("false"),
		"FEDERATION_ENABLED":   jsii.String("true"),

		// Media Streaming Configuration (Phase 2.2)
		"MEDIA_SOURCE_BUCKET_NAME":    props.MediaBucket.BucketName(),
		"MEDIA_STREAMING_BUCKET_NAME": props.StreamingBucket.BucketName(),
		"MEDIA_CONVERT_ENDPOINT":      getConfigString("mediaConvertEndpoint", ""),
		"MEDIA_CONVERT_ROLE_ARN":      props.MediaConvertRoleArn,
		"CLOUDFRONT_DOMAIN":           getConfigString("cloudfrontDomain", ""),
		"CLOUDFRONT_PRIVATE_KEY_PATH": getConfigString("cloudfrontPrivateKeySecret", ""),
		"CLOUDFRONT_KEY_PAIR_ID":      getConfigString("cloudfrontKeyPairId", ""), // Set after manual upload
		"MANIFEST_TTL_HOURS":          getConfigString("manifestTTLHours", "24"),
		"VAPID_PUBLIC_KEY":            getConfigString("vapidPublicKey", ""),
		"VAPID_SUBJECT":               getConfigString("vapidSubject", ""),
		"VAPID_SECRET_ARN":            getConfigString("vapidSecretArn", ""),

		// ML Moderation Configuration (Phase 2.3)
		"MODERATION_TRAINING_BUCKET_NAME": props.TrainingBucket.BucketName(),
		"MODERATION_MODEL_METADATA_TABLE": props.ModelMetadataTable,
		"BEDROCK_TRAINING_REGION":         getConfigString("bedrockRegion", "us-east-1"),
		"BEDROCK_INFERENCE_MODEL_ID":      getConfigString("bedrockInferenceModelId", ""),
		"BEDROCK_CUSTOMIZATION_ROLE_ARN":  getConfigString("bedrockCustomizationRoleArn", ""),
		"BEDROCK_GUARDRAIL_ID":            getConfigString("bedrockGuardrailId", ""),
		"BEDROCK_GUARDRAIL_VERSION":       getConfigString("bedrockGuardrailVersion", "DRAFT"),
		"MODERATION_ML_ENABLED":           getConfigString("moderationMLEnabled", "false"),
		"MODERATION_ML_TENANTS":           getConfigString("moderationMLTenants", ""),
	}

	// WebSocket endpoints
	// - GraphQL subscriptions: ws.<domain>
	// - Streaming: ws.<domain>/stream
	wsHost := fmt.Sprintf("ws.%s", domainValue)
	commonEnv["WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("https://%s", wsHost))
	commonEnv["WEBSOCKET_API_URL"] = jsii.String(fmt.Sprintf("https://%s", wsHost))
	commonEnv["GRAPHQL_WS_URL"] = jsii.String(fmt.Sprintf("wss://%s", wsHost))
	commonEnv["STREAM_WEBSOCKET_ENDPOINT"] = jsii.String(fmt.Sprintf("wss://%s/stream", wsHost))
	commonEnv["STREAM_WEBSOCKET_API_URL"] = jsii.String(fmt.Sprintf("https://%s/stream", wsHost))

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

		// Clone and extend env per-Lambda with queue URLs derived from inventory
		env := copyEnv(commonEnv)
		for _, trig := range spec.SQSTriggers {
			var queueVal *string
			if trig.ConsumeDeadLetterQueue {
				queueVal = dlqURL(props.Queues, trig.Queue)
			} else {
				queueVal = queueURL(props.Queues, trig.Queue)
			}

			canonicalKey := ""
			switch trig.Queue {
			case "import-processor-queue":
				canonicalKey = "IMPORT_QUEUE_URL"
			case "export-processor-queue":
				canonicalKey = "EXPORT_QUEUE_URL"
			case "media-processor-queue":
				canonicalKey = "MEDIA_QUEUE_URL"
			case "scheduled-queue":
				canonicalKey = "SCHEDULED_QUEUE_URL"
			case "federation-delivery-queue":
				canonicalKey = "FEDERATION_DELIVERY_QUEUE_URL"
			case "push-delivery-queue":
				canonicalKey = "PUSH_NOTIFICATION_QUEUE_URL"
			default:
				canonicalKey = fmt.Sprintf("%s_QUEUE_URL", strings.ToUpper(strings.ReplaceAll(trig.Queue, "-", "_")))
			}

			env[canonicalKey] = queueVal

			aliasKey := fmt.Sprintf("%s_QUEUE_URL", strings.ToUpper(strings.ReplaceAll(trig.Queue, "-", "_")))
			env[aliasKey] = queueVal

			if trig.DeadLetterQueue != "" {
				dlqKey := fmt.Sprintf("%s_DLQ_URL", strings.ToUpper(strings.ReplaceAll(trig.DeadLetterQueue, "-", "_")))
				env[dlqKey] = dlqURL(props.Queues, trig.Queue)
			}
		}
		for _, key := range spec.RequiredEnvVars {
			if _, exists := env[key]; exists {
				continue
			}
			switch key {
			case "EXPORT_QUEUE_URL", "EXPORT_PROCESSOR_QUEUE_URL":
				env[key] = queueURL(props.Queues, "export-processor-queue")
			case "IMPORT_QUEUE_URL", "IMPORT_PROCESSOR_QUEUE_URL":
				env[key] = queueURL(props.Queues, "import-processor-queue")
			case "MEDIA_QUEUE_URL", "MEDIA_PROCESSOR_QUEUE_URL":
				env[key] = queueURL(props.Queues, "media-processor-queue")
			default:
				// leave untouched for non-queue vars
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
			Code:         awslambda.Code_FromAsset(jsii.String(fmt.Sprintf("../../bin/%s.zip", spec.Name)), nil),
			Handler:      jsii.String("bootstrap"),
			LogGroup:     logGroup,
			Role:         role,
		}

		if len(spec.ScheduleTriggers) > 0 {
			validateScheduleCapable(spec)

			var scheduledFn awslambda.Function
			for idx, trig := range spec.ScheduleTriggers {
				ruleID := fmt.Sprintf("%sScheduleRule%d", sanitizeScheduleId(spec.Name), idx)
				ruleName := naming.ResourceNameWithApp(appName, fmt.Sprintf("%s-schedule-%d", spec.Name, idx), props.Environment)

				if idx == 0 {
					var inputTransformation *awsevents.RuleTargetInput
					if trig.Input != "" {
						input := awsevents.RuleTargetInput_FromText(jsii.String(trig.Input))
						inputTransformation = &input
					}

					handler, err := liftcdk.NewEventBridgeHandler(stack, jsii.String(ruleID), &liftcdk.EventBridgeHandlerProps{
						FunctionProps:         fnProps,
						ScheduleExpression:    jsii.String(trig.Expression),
						InputTransformation:   inputTransformation,
						EnableDeadLetterQueue: jsii.Bool(true),
						RuleProps: &awsevents.RuleProps{
							RuleName:    jsii.String(ruleName),
							Enabled:     jsii.Bool(true),
							Description: jsii.String(fmt.Sprintf("Inventory-driven schedule for %s (%s)", spec.Name, trig.Expression)),
						},
					})
					if err != nil {
						panic(err)
					}

					scheduledFn = handler.Function.Function
					functions.Functions[spec.Name] = scheduledFn
					continue
				}

				if scheduledFn == nil {
					panic(fmt.Sprintf("schedule wiring invariant violated: %s missing primary schedule function", spec.Name))
				}

				// Lift's EventBridgeHandler currently creates the Lambda function as part of the construct.
				// For additional schedules, create native rules that target the already-created function.
				rule := awsevents.NewRule(stack, jsii.String(ruleID), &awsevents.RuleProps{
					RuleName:    jsii.String(ruleName),
					Enabled:     jsii.Bool(true),
					Schedule:    awsevents.Schedule_Expression(jsii.String(trig.Expression)),
					Description: jsii.String(fmt.Sprintf("Inventory-driven schedule for %s (%s)", spec.Name, trig.Expression)),
				})

				targetProps := &awseventstargets.LambdaFunctionProps{}
				if trig.Input != "" {
					targetProps.Event = awsevents.RuleTargetInput_FromText(jsii.String(trig.Input))
				}
				rule.AddTarget(awseventstargets.NewLambdaFunction(scheduledFn, targetProps))
			}
			continue
		}

		liftFn := liftcdk.NewLiftFunction(stack, jsii.String(spec.Name+"Function"), &liftcdk.LiftFunctionProps{
			FunctionProps: fnProps,
			EnableTracing: jsii.Bool(true),
		})
		functions.Functions[spec.Name] = liftFn.Function
	}

	return functions
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

		fn.AddEnvironment(jsii.String("IMPORT_QUEUE_URL"), queueURL(queues, "import-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("IMPORT_PROCESSOR_QUEUE_URL"), queueURL(queues, "import-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("EXPORT_QUEUE_URL"), queueURL(queues, "export-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("EXPORT_PROCESSOR_QUEUE_URL"), queueURL(queues, "export-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("MEDIA_QUEUE_URL"), queueURL(queues, "media-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("MEDIA_PROCESSOR_QUEUE_URL"), queueURL(queues, "media-processor-queue"), nil)
		fn.AddEnvironment(jsii.String("SCHEDULED_QUEUE_URL"), queueURL(queues, "scheduled-queue"), nil)

		fn.AddEnvironment(jsii.String("FEDERATION_DELIVERY_QUEUE_URL"), queueURL(queues, "federation-delivery-queue"), nil)
		fn.AddEnvironment(jsii.String("FEDERATION_QUEUE_URL"), queueURL(queues, "federation-delivery-queue"), nil)
		fn.AddEnvironment(jsii.String("FEDERATION_DLQ_URL"), dlqURL(queues, "federation-delivery-queue"), nil)

		fn.AddEnvironment(jsii.String("PUSH_NOTIFICATION_QUEUE_URL"), queueURL(queues, "push-delivery-queue"), nil)
		fn.AddEnvironment(jsii.String("PUSH_QUEUE_URL"), queueURL(queues, "push-delivery-queue"), nil)
	}

	for _, spec := range inventory.LambdaInventory.Lambdas {
		fn := functions.Must(spec.Name)
		for _, trig := range spec.SQSTriggers {
			var queueVal *string
			if trig.ConsumeDeadLetterQueue {
				queueVal = dlqURL(queues, trig.Queue)
			} else {
				queueVal = queueURL(queues, trig.Queue)
			}

			canonicalKey := ""
			switch trig.Queue {
			case "import-processor-queue":
				canonicalKey = "IMPORT_QUEUE_URL"
			case "export-processor-queue":
				canonicalKey = "EXPORT_QUEUE_URL"
			case "media-processor-queue":
				canonicalKey = "MEDIA_QUEUE_URL"
			case "scheduled-queue":
				canonicalKey = "SCHEDULED_QUEUE_URL"
			case "federation-delivery-queue":
				canonicalKey = "FEDERATION_DELIVERY_QUEUE_URL"
			case "push-delivery-queue":
				canonicalKey = "PUSH_NOTIFICATION_QUEUE_URL"
			default:
				canonicalKey = fmt.Sprintf("%s_QUEUE_URL", strings.ToUpper(strings.ReplaceAll(trig.Queue, "-", "_")))
			}

			fn.AddEnvironment(jsii.String(canonicalKey), queueVal, nil)

			aliasKey := fmt.Sprintf("%s_QUEUE_URL", strings.ToUpper(strings.ReplaceAll(trig.Queue, "-", "_")))
			fn.AddEnvironment(jsii.String(aliasKey), queueVal, nil)

			if trig.DeadLetterQueue != "" {
				dlqKey := fmt.Sprintf("%s_DLQ_URL", strings.ToUpper(strings.ReplaceAll(trig.DeadLetterQueue, "-", "_")))
				fn.AddEnvironment(jsii.String(dlqKey), dlqURL(queues, trig.Queue), nil)
			}
		}

		for _, key := range spec.RequiredEnvVars {
			switch key {
			case "EXPORT_QUEUE_URL", "EXPORT_PROCESSOR_QUEUE_URL":
				fn.AddEnvironment(jsii.String(key), queueURL(queues, "export-processor-queue"), nil)
			case "IMPORT_QUEUE_URL", "IMPORT_PROCESSOR_QUEUE_URL":
				fn.AddEnvironment(jsii.String(key), queueURL(queues, "import-processor-queue"), nil)
			case "MEDIA_QUEUE_URL", "MEDIA_PROCESSOR_QUEUE_URL":
				fn.AddEnvironment(jsii.String(key), queueURL(queues, "media-processor-queue"), nil)
			default:
				// ignore non-queue vars
			}
		}
	}
}
