package inventory

// LambdaInventory is the canonical machine-readable source for all product Lambdas.
// It must stay in lockstep with Makefile LAMBDAS and Spec 01.
var LambdaInventory = Inventory{
	Defaults: BaselineDefaults,
	Lambdas: []LambdaSpec{
		{
			Name: "activity-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table", // replaced by stack DDB table at deploy time
					StartingPosition:         StreamStartTrimHorizon,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    5,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
			Overrides: LambdaOverrides{
				MemoryMB:       intPtr(1024),
				TimeoutSeconds: intPtr(300),
			},
		},
		{
			Name: "actor",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/users/{username}"},
			},
		},
		{
			Name: "ai-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartTrimHorizon,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    2,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
			Overrides: LambdaOverrides{
				MemoryMB:       intPtr(1024),
				TimeoutSeconds: intPtr(300),
			},
		},
		{
			Name: "api",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "ANY", Path: "/api/v1/{proxy+}"},
				{Method: "ANY", Path: "/api/v2/{proxy+}"},
				{Method: "GET", Path: "/.well-known/nodeinfo"},
			},
			RequiredEnvVars: []string{"S3_MEDIA_BUCKET", "DOMAIN_NAME"},
		},
		{
			Name: "collections",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/users/{username}/followers"},
				{Method: "GET", Path: "/users/{username}/following"},
				{Method: "GET", Path: "/users/{username}/liked"},
			},
		},
		{
			Name: "cost-aggregator",
			Type: LambdaTypeProcessorStream, // TODO(spec04): confirm if hybrid stream + schedule
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 2,
					ParallelizationFactor:    1,
					ReportBatchItemFailures:  true,
				},
			},
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "TODO-spec04-cost-aggregator-cadence"},
			},
		},
		{
			Name: "dlq-processor",
			Type: LambdaTypeHybrid, // SQS + scheduled sweeps
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{
					Queue:                    "dlq-queue",
					DeadLetterQueue:          "dlq-dlq",
					BatchSize:                10,
					MaxBatchingWindowSeconds: 1,
					EnablePartialFailure:     true,
				},
			},
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "TODO-spec04-dlq-sweep-cadence"},
			},
		},
		{
			Name: "enhanced-federation-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "enhanced-federation-queue", EnablePartialFailure: true},
			},
		},
		{
			Name: "export-generator",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "export-processor-queue", EnablePartialFailure: true},
			},
			RequiredEnvVars: []string{"EXPORT_PROCESSOR_QUEUE_URL"}, // TODO(spec04): confirm queue model
		},
		{
			Name: "federation-aggregator",
			Type: LambdaTypeProcessorScheduled,
			Role: RoleClassBasic,
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "TODO-spec04-federation-aggregator-cadence"},
			},
		},
		{
			Name: "federation-delivery",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "federation-delivery-queue", EnablePartialFailure: true},
			},
		},
		{
			Name: "federation-timeseries",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "federation-tracker",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "graphql",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/api/graphql"},
				{Method: "POST", Path: "/api/graphql"},
			},
			RequiredEnvVars: []string{"JWT_SECRET"}, // required when ARN is not injected
		},
		{
			Name:            "graphql-ws",
			Type:            LambdaTypeAPIWS,
			Role:            RoleClassEncryption,
			RequiredEnvVars: []string{"JWT_SECRET"},
		},
		{
			Name: "import-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "import-processor-queue", EnablePartialFailure: true},
			},
			RequiredEnvVars: []string{"IMPORT_PROCESSOR_QUEUE_URL"}, // TODO(spec04): confirm queue model
		},
		{
			Name: "inbox",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "ANY", Path: "/inbox/{username}"},
			},
		},
		{
			Name: "media-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "media-processor-queue", EnablePartialFailure: true},
			},
			RequiredEnvVars: []string{"MEDIA_PROCESSOR_QUEUE_URL"},
		},
		{
			Name: "metrics-aggregator",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "metrics-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "ml-training-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                5,
					MaxBatchingWindowSeconds: 1,
					ParallelizationFactor:    1,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
			Overrides: LambdaOverrides{
				MemoryMB:       intPtr(1024),
				TimeoutSeconds: intPtr(900),
			},
		},
		{
			Name: "moderation-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    2,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "note-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "notification-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "notification-processor-queue", EnablePartialFailure: true},
			},
		},
		{
			Name: "objects",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/objects/{id}"},
			},
		},
		{
			Name: "outbox",
			Type: LambdaTypeHybrid, // HTTP + stream
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "ANY", Path: "/users/{username}/outbox"},
			},
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 2,
					ParallelizationFactor:    3,
					MaxRetryAttempts:         5,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "push-delivery",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "push-delivery-queue", EnablePartialFailure: true},
			},
			RequiredEnvVars: []string{"VAPID_PUBLIC_KEY", "VAPID_SUBJECT", "VAPID_SECRET_ARN"},
		},
		{
			Name: "report-trust-updater",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "search-indexer",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                100,
					MaxBatchingWindowSeconds: 30,
					ParallelizationFactor:    5,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "severance-processor",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    2,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
			Overrides: LambdaOverrides{
				MemoryMB:       intPtr(1024),
				TimeoutSeconds: intPtr(30),
			},
		},
		{
			Name: "status-indexer",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ReportBatchItemFailures:  true,
				},
			},
		},
		{
			Name: "stream-router",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassEncryption,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					StartingPosition:         StreamStartLatest,
					BatchSize:                50,
					MaxBatchingWindowSeconds: 2,
					ParallelizationFactor:    5,
					MaxRetryAttempts:         3,
					EnableBisectOnError:      true,
					ReportBatchItemFailures:  true,
				},
			},
			RequiredEnvVars: []string{"WEBSOCKET_API_URL", "WEBSOCKET_ENDPOINT", "STREAMING_SUBSCRIPTIONS_TABLE", "WEBSOCKET_API_ID", "WEBSOCKET_STAGE", "DOMAIN_NAME"},
		},
		{
			Name: "streaming",
			Type: LambdaTypeAPIWS,
			Role: RoleClassEncryption,
		},
		{
			Name: "trend-aggregator",
			Type: LambdaTypeProcessorScheduled,
			Role: RoleClassBasic,
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "TODO-spec04-trend-aggregator-cadence"},
			},
		},
		{
			Name: "webfinger",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassBasic,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/.well-known/webfinger"},
			},
		},
		{
			Name: "websocket-cost-aggregator",
			Type: LambdaTypeProcessorScheduled,
			Role: RoleClassBasic,
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "TODO-spec04-websocket-cost-cadence"},
			},
		},
	},
}

// intPtr is a local helper for optional override fields.
func intPtr(v int) *int { return &v }
