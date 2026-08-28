package repositories

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

type createSemanticsAuditRecord struct {
	_     struct{} `theorydb:"naming:camelCase"`
	PK    string   `theorydb:"pk,attr:PK"`
	SK    string   `theorydb:"sk,attr:SK"`
	Value string   `theorydb:"attr:value"`
}

func (createSemanticsAuditRecord) TableName() string { return "create-semantics-v305" }

type upsertAuditSite struct {
	file     string
	function string
	method   string
	count    int
}

type replayToleranceAuditSite struct {
	file       string
	function   string
	guardCall  string
	guardCount int
}

var upsertAuditSites = []upsertAuditSite{
	{file: "cmd/activity-processor/handler.go", function: "processDeleteActivity", method: "CreateOrUpdate", count: 1},
	{file: "cmd/activity-processor/main.go", function: "updateActivityMetrics", method: "CreateOrUpdate", count: 1},
	{file: "cmd/activity-processor/main.go", function: "recordMetric", method: "CreateOrUpdate", count: 1},
	{file: "cmd/federation-aggregator/main.go", function: "storeAggregation", method: "CreateOrUpdate", count: 1},
	{file: "cmd/inbox/internal/routing/inbox.go", function: "storeMoveMigration", method: "CreateOrUpdate", count: 1},
	{file: "cmd/inbox/internal/routing/inbox.go", function: "createAccountTombstone", method: "CreateOrUpdate", count: 1},
	{file: "cmd/report-trust-updater/main.go", function: "UpdateReporterTrustOnDecision", method: "CreateOrUpdate", count: 1},
	{file: "cmd/search-indexer/main.go", function: "createSearchIndex", method: "CreateOrUpdate", count: 1},
	{file: "cmd/search-indexer/main.go", function: "createAdditionalIndexes", method: "CreateOrUpdate", count: 2},
	{file: "cmd/status-indexer/main.go", function: "indexWord", method: "CreateOrUpdate", count: 1},
	{file: "cmd/status-indexer/main.go", function: "indexHashtag", method: "CreateOrUpdate", count: 1},
	{file: "cmd/status-indexer/main.go", function: "indexByAuthor", method: "CreateOrUpdate", count: 1},
	{file: "cmd/status-indexer/main.go", function: "storeEmbedding", method: "CreateOrUpdate", count: 1},
	{file: "cmd/status-indexer/main.go", function: "updateTrendingStatus", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/cost/repository_adapter.go", function: "RecordCost", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/cost/repository_adapter.go", function: "UpdateInstanceHealth", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/cost/repository_adapter.go", function: "SaveInstanceConfig", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/dynamorm_storage.go", function: "CacheRemoteActor", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/dynamorm_storage.go", function: "RecordFederationActivity", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/relationship_tracker.go", function: "saveRelationship", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/relationship_tracker.go", function: "saveAggregate", method: "CreateOrUpdate", count: 1},
	{file: "pkg/federation/relationship_tracker.go", function: "archiveDormantRelationships", method: "CreateOrUpdate", count: 1},
	{file: "pkg/media/streaming/storage.go", function: "UpdateMediaMetadata", method: "CreateOrUpdate", count: 1},
	{file: "pkg/reputation/service.go", function: "setCachedMetric", method: "CreateOrUpdate", count: 1},
	{file: "pkg/reputation/service.go", function: "setCachedTimestamp", method: "CreateOrUpdate", count: 1},
	{file: "pkg/services/cms/article_service.go", function: "upsertCMSArticleIndexes", method: "CreateOrUpdate", count: 1},
	{file: "pkg/services/cms/revision_service.go", function: "Create", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/cache.go", function: "CacheUser", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/ai_cost_repository.go", function: "CreateOrUpdateAggregatedCost", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "storeTrendInternal", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "updateHashtagTrendScore", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "updateStatusTrendScore", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "updateLinkTrendScore", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "StoreEngagementMetrics", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "RecordEngagement", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "UpdateTrendingHashtag", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/analytics_repository.go", function: "RecordModerationAction", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/account_repository_search.go", function: "CacheRemoteActor", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/conversation_state_repository.go", function: "createOrUpdateUserConversationState", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/conversation_repository.go", function: "CreateConversationMute", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/cost_tracking_repository.go", function: "CreateAggregated", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/dns_cache_repository.go", function: "SetDNSCache", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/enhanced_pattern_repository.go", function: "SetPatternCache", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_activity_repository.go", function: "UpdateInstanceInfo", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_cost_repository.go", function: "CreateOrUpdateBudget", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "UpsertInstanceInfo", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "updateAggregatedCosts", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "AcknowledgeSeverance", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "AttemptReconnection", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "updateFederationInstanceStatus", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "UpdateFederationNode", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "UpdateFederationEdge", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "UpdateInstanceMetadata", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "StoreFederationTimeSeries", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "StoreInstanceCluster", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "RetryDelivery", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/federation_repository.go", function: "StoreDetailedFederationMetrics", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/graphql_stream_subscription_repository.go", function: "Put", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/hashtag_repository.go", function: "StoreHashtagTrend", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/instance_health_repository.go", function: "SaveHealthSummary", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/instance_repository.go", function: "SetInstanceRules", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/instance_repository.go", function: "SetExtendedDescription", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/instance_repository.go", function: "RecordDailyMetrics", method: "CreateOrUpdate", count: 4},
	{file: "pkg/storage/repositories/marker_repository.go", function: "SaveMarker", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/metrics_repository.go", function: "CreateAggregated", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/object_repository.go", function: "MarkThreadAsSynced", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/object_repository.go", function: "updateSearchIndexForWithdrawal", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/public_key_cache_repository.go", function: "Store", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/query_cache_repository.go", function: "SetCachedValue", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/relay_repository.go", function: "UpdateRelayStatus", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/relay_repository.go", function: "UpdateRelayState", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/severance_repository.go", function: "UpdateSeveranceStatus", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/severance_repository.go", function: "UpdateReconnectionAttempt", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/routing_metrics_repository.go", function: "StoreRouteMetricsWindow", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/routing_metrics_repository.go", function: "StoreGlobalMetricsWindow", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/routing_metrics_repository.go", function: "StoreInstanceMetricsWindow", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/routing_metrics_repository.go", function: "BatchStoreMetrics", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/routing_metrics_repository.go", function: "validateAndUpsertMetricsWindows", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/social_repository.go", function: "CreateAccountNote", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/social_repository.go", function: "UpdateAccountNote", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/streaming_cloudwatch_repository.go", function: "CacheQualityBreakdown", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/streaming_connection_repository.go", function: "WriteSubscription", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/streaming_repository.go", function: "UpdateDeviceStreamingPreferences", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/thread_repository.go", function: "SaveThreadSync", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/thread_repository.go", function: "SaveThreadNode", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/thread_repository.go", function: "MarkMissingReplies", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/thread_repository.go", function: "SaveMissingReply", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/thread_repository.go", function: "BulkSaveThreadNodes", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/trust_repository.go", function: "UpdateTrustScore", method: "ValidateAndCreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/user_repository.go", function: "UpdateAccountNote", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/user_repository.go", function: "StoreReputation", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/user_repository.go", function: "UpdateTrustScore", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/user_repository.go", function: "UpdateUserPreferences", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/repositories/user_repository.go", function: "CacheRemoteActor", method: "CreateOrUpdate", count: 1},
	{file: "pkg/storage/theorydb/migrations/migrator.go", function: "UpdateMigrationStatus", method: "CreateOrUpdate", count: 1},
}

// replayToleranceAuditSites records create-only rows whose uniqueness remains
// intentional but whose callers run on at-least-once or duplicate-tolerant
// paths. These callers accept only the existing-key condition; they continue
// to propagate every other storage error.
var replayToleranceAuditSites = []replayToleranceAuditSite{
	{file: "cmd/activity-processor/handler.go", function: "processCreateActivity", guardCall: "IsConditionFailed", guardCount: 1},
	{file: "cmd/activity-processor/main.go", function: "processInboxActivity", guardCall: "tolerateActivityReplayCreate", guardCount: 1},
	{file: "cmd/activity-processor/main.go", function: "processOutboxActivity", guardCall: "tolerateActivityReplayCreate", guardCount: 1},
	{file: "cmd/activity-processor/main.go", function: "cleanupActivityReferences", guardCall: "tolerateActivityReplayCreate", guardCount: 1},
	{file: "cmd/activity-processor/main.go", function: "createTombstone", guardCall: "tolerateActivityReplayCreate", guardCount: 1},
	{file: "cmd/import-processor/main.go", function: "followAccount", guardCall: "tolerateActivityPubImportReplay", guardCount: 1},
	{file: "cmd/import-processor/main.go", function: "importLikeActivity", guardCall: "tolerateActivityPubImportReplay", guardCount: 1},
	{file: "cmd/import-processor/main.go", function: "importAnnounceActivity", guardCall: "tolerateActivityPubImportReplay", guardCount: 1},
	{file: "pkg/storage/repositories/announcement_repository.go", function: "DismissAnnouncement", guardCall: "IsConditionFailed", guardCount: 1},
}

// TestIntentionalUpsertsUseV305StateBackedSemantics pins every production site
// reclassified after TableTheory v3.0.5 changed Create from Put/upsert to a
// primary-key-not-exists write. The AST assertion binds each audited site to
// the explicit upsert API; the state-backed fakedb assertion proves the chosen
// API overwrites a row in the same situation where Create now fails.
func TestIntentionalUpsertsUseV305StateBackedSemantics(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))

	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&createSemanticsAuditRecord{}))

	for index, site := range upsertAuditSites {
		site := site
		t.Run(fmt.Sprintf("%03d_%s_%s", index, filepath.Base(site.file), site.function), func(t *testing.T) {
			sourcePath := filepath.Join(repoRoot, filepath.FromSlash(site.file))
			parsed, parseErr := parser.ParseFile(token.NewFileSet(), sourcePath, nil, 0)
			require.NoError(t, parseErr)
			require.Equal(t, site.count, countMethodCallsInFunction(parsed, site.function, site.method),
				"%s must retain its audited explicit-upsert call(s)", site.file)

			pk := fmt.Sprintf("AUDIT#%03d", index)
			initial := &createSemanticsAuditRecord{PK: pk, SK: "ROW", Value: "before"}
			require.NoError(t, db.Model(initial).Create())

			conditionalOverwrite := &createSemanticsAuditRecord{PK: pk, SK: "ROW", Value: "conditional"}
			require.ErrorIs(t, db.Model(conditionalOverwrite).Create(), dynamormerrors.ErrConditionFailed,
				"v3.0.5 Create must reject overwriting the audited deterministic key")

			explicitUpsert := &createSemanticsAuditRecord{PK: pk, SK: "ROW", Value: site.file + ":" + site.function}
			require.NoError(t, db.Model(explicitUpsert).CreateOrUpdate())

			var persisted createSemanticsAuditRecord
			require.NoError(t, db.Model(&createSemanticsAuditRecord{}).
				Where("PK", "=", pk).
				Where("SK", "=", "ROW").
				First(&persisted))
			require.Equal(t, explicitUpsert.Value, persisted.Value)
		})
	}
}

func countMethodCallsInFunction(file *ast.File, function, method string) int {
	return countCallsInFunction(file, function, method)
}

func countCallsInFunction(file *ast.File, function, target string) int {
	count := 0
	for _, declaration := range file.Decls {
		functionDeclaration, ok := declaration.(*ast.FuncDecl)
		if !ok || functionDeclaration.Name.Name != function || functionDeclaration.Body == nil {
			continue
		}
		ast.Inspect(functionDeclaration.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch functionCall := call.Fun.(type) {
			case *ast.SelectorExpr:
				if functionCall.Sel.Name == target {
					count++
				}
			case *ast.Ident:
				if functionCall.Name == target {
					count++
				}
			}
			return true
		})
	}
	return count
}

func TestReplayTolerantCreatesRetainExplicitConditionGuards(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../.."))

	for _, site := range replayToleranceAuditSites {
		site := site
		t.Run(filepath.Base(site.file)+"_"+site.function, func(t *testing.T) {
			parsed, err := parser.ParseFile(
				token.NewFileSet(),
				filepath.Join(repoRoot, filepath.FromSlash(site.file)),
				nil,
				0,
			)
			require.NoError(t, err)
			require.Equal(t, site.guardCount, countCallsInFunction(parsed, site.function, site.guardCall),
				"%s must retain its replay-only condition guard", site.file)
		})
	}
}

func TestSocialRepositoryAccountNoteOverwritesOnDeterministicKey(t *testing.T) {
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.AccountNote{}))
	repo := NewSocialRepository(db, "test-table", zap.NewNop(), nil)

	note := &storage.AccountNote{
		Username:      "alice",
		TargetActorID: "https://remote.example/users/bob",
		Note:          "first",
	}
	require.NoError(t, repo.CreateAccountNote(ctx, note))

	note.Note = "second"
	require.NoError(t, repo.CreateAccountNote(ctx, note),
		"SetAccountNote calls CreateAccountNote for both insert and replacement")

	persisted, err := repo.GetAccountNote(ctx, note.Username, note.TargetActorID)
	require.NoError(t, err)
	require.Equal(t, "second", persisted.Note)
}

func TestFederationRepositoryDeterministicRowsOverwriteOnSecondWrite(t *testing.T) {
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.FederationInstance{}))
	repo := NewFederationRepository(db, "test-table", zap.NewNop(), nil, nil)

	instance := &storage.InstanceInfo{Domain: "remote.example", Software: "mastodon", Version: "1"}
	require.NoError(t, repo.UpsertInstanceInfo(ctx, instance))
	instance.Version = "2"
	require.NoError(t, repo.UpsertInstanceInfo(ctx, instance))

	node := &storage.FederationNode{Domain: "remote.example", DisplayName: "first"}
	require.NoError(t, repo.UpdateFederationNode(ctx, node))
	node.DisplayName = "second"
	require.NoError(t, repo.UpdateFederationNode(ctx, node))

	edge := &storage.FederationEdge{SourceDomain: "local.example", TargetDomain: "remote.example", Strength: 1}
	require.NoError(t, repo.UpdateFederationEdge(ctx, edge))
	edge.Strength = 2
	require.NoError(t, repo.UpdateFederationEdge(ctx, edge))

	metadata := &storage.InstanceMetadata{Domain: "remote.example", Description: "first"}
	require.NoError(t, repo.UpdateInstanceMetadata(ctx, metadata))
	metadata.Description = "second"
	require.NoError(t, repo.UpdateInstanceMetadata(ctx, metadata))

	cluster := &storage.InstanceCluster{ClusterID: "cluster-1", Name: "first", Instances: []string{"remote.example"}}
	require.NoError(t, repo.StoreInstanceCluster(ctx, cluster))
	cluster.Name = "second"
	require.NoError(t, repo.StoreInstanceCluster(ctx, cluster))
}
