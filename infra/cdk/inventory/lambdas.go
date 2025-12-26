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
		},
		{
			Name: "sse",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			Overrides: LambdaOverrides{
				TimeoutSeconds: intPtr(900),
			},
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
			Name: "cms-scheduler",
			Type: LambdaTypeProcessorScheduled,
			Role: RoleClassBasic,
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "rate(1 minute)"},
			},
			Overrides: LambdaOverrides{
				TimeoutSeconds: intPtr(300),
			},
		},
		{
			Name: "cost-aggregator",
			Type: LambdaTypeProcessorStream, // stream-only per Spec04 (Q2) decision
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
		},
		{
			Name: "dlq-processor",
			Type: LambdaTypeHybrid, // consumes per-job DLQs + scheduled sweeps
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "enhanced-federation-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "export-processor-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "federation-aggregator-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "federation-delivery-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "import-processor-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "media-processor-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "notification-processor-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
				{Queue: "push-delivery-queue", ConsumeDeadLetterQueue: true, BatchSize: 10, MaxBatchingWindowSeconds: 1},
			},
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "rate(15 minutes)"},
			},
		},
		{
			Name: "enhanced-federation-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "enhanced-federation-queue", EnablePartialFailure: false},
			},
		},
		{
			Name: "export-generator",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "export-processor-queue", EnablePartialFailure: false},
			},
		},
		{
			Name: "federation-aggregator",
			Type: LambdaTypeHybrid,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "federation-aggregator-queue", EnablePartialFailure: false},
			},
			ScheduleTriggers: []ScheduleTrigger{
				{Expression: "rate(1 hour)"},
			},
		},
		{
			Name: "federation-delivery",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "federation-delivery-queue", EnablePartialFailure: false},
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
		},
		{
			Name: "graphql-ws",
			Type: LambdaTypeAPIWS,
			Role: RoleClassEncryption,
		},
		{
			Name: "import-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "import-processor-queue", EnablePartialFailure: false},
			},
		},
		{
			Name: "inbox",
			Type: LambdaTypeAPIHTTP,
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/users/{username}/inbox"},
				{Method: "POST", Path: "/users/{username}/inbox"},
			},
		},
		{
			Name: "media-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "media-processor-queue", EnablePartialFailure: false},
			},
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
				{Queue: "notification-processor-queue", EnablePartialFailure: false},
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
			Type: LambdaTypeAPIHTTP, // HTTP-only
			Role: RoleClassEncryption,
			HTTPRoutes: []HTTPRoute{
				{Method: "GET", Path: "/users/{username}/outbox"},
				{Method: "POST", Path: "/users/{username}/outbox"},
			},
		},
		{
			Name: "push-delivery",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "push-delivery-queue", EnablePartialFailure: false},
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
				{Expression: "cron(0 2 * * ? *)"},
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
				{Expression: "rate(1 hour)"},
			},
		},
	},
}

// intPtr is a local helper for optional override fields.
func intPtr(v int) *int { return &v }
