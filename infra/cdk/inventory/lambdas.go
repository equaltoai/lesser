package inventory

import "github.com/equaltoai/lesser/pkg/federation/surface"

func inboxHTTPRoutes() []HTTPRoute {
	sharedInbox := surface.SharedInbox()
	routes := make([]HTTPRoute, 0, len(sharedInbox.ServedMethods())+2)
	for _, method := range sharedInbox.ServedMethods() {
		routes = append(routes, HTTPRoute{Method: method, Path: sharedInbox.Path})
	}
	routes = append(routes,
		HTTPRoute{Method: "GET", Path: "/users/{username}/inbox"},
		HTTPRoute{Method: "POST", Path: "/users/{username}/inbox"},
	)
	return routes
}

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
					PoisonRecordQueue:        "activity-processor-stream-poison-queue",
					StartingPosition:         StreamStartTrimHorizon,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "ai-processor-stream-poison-queue",
					StartingPosition:         StreamStartTrimHorizon,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    2,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
				{Method: "GET", Path: "/robots.txt"},
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
					PoisonRecordQueue:        "cost-aggregator-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 2,
					ParallelizationFactor:    1,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
		},
		{
			Name: "federation-aggregator",
			Type: LambdaTypeHybrid,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "federation-aggregator-queue", EnablePartialFailure: true},
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
					PoisonRecordQueue:        "federation-timeseries-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "federation-tracker-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
				{Queue: "import-processor-queue", EnablePartialFailure: true},
			},
		},
		{
			Name:       "inbox",
			Type:       LambdaTypeAPIHTTP,
			Role:       RoleClassEncryption,
			HTTPRoutes: inboxHTTPRoutes(),
		},
		{
			Name: "media-processor",
			Type: LambdaTypeProcessorSQS,
			Role: RoleClassBasic,
			SQSTriggers: []SQSTrigger{
				{Queue: "media-processor-queue", EnablePartialFailure: true},
			},
		},
		{
			Name: "metrics-aggregator",
			Type: LambdaTypeProcessorStream,
			Role: RoleClassBasic,
			StreamTriggers: []StreamTrigger{
				{
					SourceTable:              "main-table",
					PoisonRecordQueue:        "metrics-aggregator-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "metrics-processor-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "ml-training-processor-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                5,
					MaxBatchingWindowSeconds: 1,
					ParallelizationFactor:    1,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "moderation-processor-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    2,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "note-processor-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
				{Method: "GET", Path: "/users/{username}/statuses/{id}"},
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
					PoisonRecordQueue:        "report-trust-updater-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "search-indexer-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                100,
					MaxBatchingWindowSeconds: 30,
					ParallelizationFactor:    5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "severance-processor-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                10,
					MaxBatchingWindowSeconds: 5,
					ParallelizationFactor:    2,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "status-indexer-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                25,
					MaxBatchingWindowSeconds: 5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
					PoisonRecordQueue:        "stream-router-stream-poison-queue",
					StartingPosition:         StreamStartLatest,
					BatchSize:                50,
					MaxBatchingWindowSeconds: 2,
					ParallelizationFactor:    5,
					MaxRetryAttempts:         3,
					MaxRecordAgeSeconds:      3600,
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
