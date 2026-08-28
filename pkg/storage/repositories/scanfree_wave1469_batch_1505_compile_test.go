package repositories

// Batch #1505 (issue #1500) — two-range sort-key KeyConditionExpression
// consolidation. Every fixed site is pinned here at the DynamoDB-contract
// level: the chain mirrors the fixed production query and compiles against
// tabletheory v3.0.6, asserting the KeyConditionExpression carries EXACTLY ONE
// condition on the sort key. A mutation restoring the old two-range pair
// (`>=` + `<=`, or `begins_with` + `>`, etc.) dies on the duplicate name-value
// count and on the stray range operator.

import (
	"regexp"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	ttquery "github.com/theory-cloud/tabletheory/v3/pkg/query"
)

// compileRow is a shared dummy model whose key fields cover every schema used
// below; query compilation resolves attribute names via compileSchema, not the
// model's tags.
type compileRow struct {
	PK            string `theorydb:"pk"`
	SK            string `theorydb:"sk"`
	Gsi1PK        string `theorydb:"gsi1pk"`
	Gsi1SK        string `theorydb:"gsi1sk"`
	Gsi2PK        string `theorydb:"gsi2pk"`
	Gsi2SK        string `theorydb:"gsi2sk"`
	Gsi3PK        string `theorydb:"gsi3pk"`
	Gsi3SK        string `theorydb:"gsi3sk"`
	Gsi4PK        string `theorydb:"gsi4pk"`
	Gsi4SK        string `theorydb:"gsi4sk"`
	Gsi5PK        string `theorydb:"gsi5pk"`
	Gsi5SK        string `theorydb:"gsi5sk"`
	OperationType string `theorydb:"attr:operationType"`
}

type compileSchema struct {
	indexes []core.IndexSchema
	fields  []string
}

func (s compileSchema) TableName() string { return models.MainTableName }
func (s compileSchema) PrimaryKey() core.KeySchema {
	return core.KeySchema{PartitionKey: "PK", SortKey: "SK"}
}
func (s compileSchema) Indexes() []core.IndexSchema { return s.indexes }
func (s compileSchema) AttributeMetadata(field string) *core.AttributeMetadata {
	for _, f := range s.fields {
		if f == field {
			return &core.AttributeMetadata{Name: field, DynamoDBName: field, Type: "string"}
		}
	}
	return nil
}
func (s compileSchema) VersionFieldName() string { return "version" }

var (
	mainSchema = compileSchema{fields: []string{"PK", "SK"}}
	gsi1Schema = compileSchema{
		indexes: []core.IndexSchema{{Name: "gsi1", Type: "GSI", PartitionKey: "gsi1PK", SortKey: "gsi1SK"}},
		fields:  []string{"PK", "SK", "gsi1PK", "gsi1SK"},
	}
	gsi1OpSchema = compileSchema{
		indexes: []core.IndexSchema{{Name: "gsi1", Type: "GSI", PartitionKey: "gsi1PK", SortKey: "gsi1SK"}},
		fields:  []string{"PK", "SK", "gsi1PK", "gsi1SK", "OperationType"},
	}
	gsi2Schema = compileSchema{
		indexes: []core.IndexSchema{{Name: "gsi2", Type: "GSI", PartitionKey: "gsi2PK", SortKey: "gsi2SK"}},
		fields:  []string{"PK", "SK", "gsi2PK", "gsi2SK"},
	}
	gsi3Schema = compileSchema{
		indexes: []core.IndexSchema{{Name: "gsi3", Type: "GSI", PartitionKey: "gsi3PK", SortKey: "gsi3SK"}},
		fields:  []string{"PK", "SK", "gsi3PK", "gsi3SK"},
	}
	gsi4Schema = compileSchema{
		indexes: []core.IndexSchema{{Name: "gsi4", Type: "GSI", PartitionKey: "gsi4PK", SortKey: "gsi4SK"}},
		fields:  []string{"PK", "SK", "gsi4PK", "gsi4SK"},
	}
	gsi5Schema = compileSchema{
		indexes: []core.IndexSchema{{Name: "gsi5", Type: "GSI", PartitionKey: "gsi5PK", SortKey: "gsi5SK"}},
		fields:  []string{"PK", "SK", "gsi5PK", "gsi5SK"},
	}
)

var kceNameRe = regexp.MustCompile(`#n\d+`)

// countKCEAttr counts the DISTINCT name-placeholders in the
// KeyConditionExpression that resolve to attr. This is the DynamoDB
// "one condition per key" contract: a second condition on the same sort key
// would resolve a second placeholder to the same attr.
func countKCEAttr(compiled *core.CompiledQuery, attr string) int {
	seen := map[string]bool{}
	count := 0
	for _, name := range kceNameRe.FindAllString(compiled.KeyConditionExpression, -1) {
		if seen[name] {
			continue
		}
		seen[name] = true
		if compiled.ExpressionAttributeNames[name] == attr {
			count++
		}
	}
	return count
}

// assertSingleKeyCondition pins the compiled query contract: a Query on the
// expected index whose KeyConditionExpression carries exactly one condition on
// the sort key, contains the expected operator, and does not carry the
// two-range pair ` >= ` / ` <= `.
func assertSingleKeyCondition(t *testing.T, compiled *core.CompiledQuery, skAttr, index, op string, values []string, filter string) {
	t.Helper()
	assertSingleKeyConditionLE(t, compiled, skAttr, index, op, values, filter, false)
}

// assertSingleKeyConditionLE is the same pin with an explicit toggle for the
// ` <= ` absence check — a single `<=` upper bound (e.g. the draft due-cutoff
// first page) is a valid one-condition KCE; only the `>=`+`<=` PAIR is the
// DynamoDB-rejected shape.
func assertSingleKeyConditionLE(t *testing.T, compiled *core.CompiledQuery, skAttr, index, op string, values []string, filter string, allowLE bool) {
	t.Helper()
	require.Equal(t, "Query", compiled.Operation)
	require.Equal(t, index, compiled.IndexName)
	require.Equal(t, 1, countKCEAttr(compiled, skAttr), "KeyConditionExpression=%q must carry exactly one %s condition", compiled.KeyConditionExpression, skAttr)
	require.Contains(t, compiled.KeyConditionExpression, op)
	require.NotContains(t, compiled.KeyConditionExpression, " >= ")
	if !allowLE {
		require.NotContains(t, compiled.KeyConditionExpression, " <= ")
	}
	if filter == "" {
		require.Empty(t, compiled.FilterExpression)
	} else if filter == "attr:OperationType" {
		require.Equal(t, 1, countCompiledNameValues(compiled, "OperationType"))
	} else {
		require.Contains(t, compiled.FilterExpression, filter)
	}
	if len(values) > 0 {
		got := compiledStringValues(compiled)
		for _, v := range values {
			require.Contains(t, got, v)
		}
	}
}

func compileB1505(t *testing.T, schema compileSchema, build func(core.Query) core.Query) *core.CompiledQuery {
	t.Helper()
	q := ttquery.New(&compileRow{}, schema, nil)
	compiled, err := build(q).(*ttquery.Query).Compile()
	require.NoError(t, err)
	return compiled
}

// ============================================================================
// Shape A — `>=` + `<=` (or `>=` + `<`) collapsed to one BETWEEN key condition
// ============================================================================

func TestBatch1505_FederationActivity_ListByDomain_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "fed_activity#example.com").
			Where("SK", "BETWEEN", []any{"activity#20260827170919", "activity#20260828170919"}).
			OrderBy("SK", "DESC").
			Limit(100)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"activity#20260827170919", "activity#20260828170919"}, "")
}

func TestBatch1505_FederationCost_DomainWindow_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "FED_COSTS#DOMAIN#example.com#2026-08").
			Where("gsi1SK", "BETWEEN", []any{"TS#0000000000001", "TS#0000000000002~"}).
			OrderBy("gsi1SK", "ASC")
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"TS#0000000000001", "TS#0000000000002~"}, "")
}

func TestBatch1505_FederationCost_ActivityTypeWindow_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi2Schema, func(q core.Query) core.Query {
		return q.Where("gsi2PK", "=", "FED_TYPE#announce").
			Where("gsi2SK", "BETWEEN", []any{"DOMAIN#20260827170919", "DOMAIN#20260828170919"}).
			Limit(500)
	})
	assertSingleKeyCondition(t, compiled, "gsi2SK", "gsi2", "BETWEEN", []string{"DOMAIN#20260827170919", "DOMAIN#20260828170919"}, "")
}

func TestBatch1505_Federation_UserCosts_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "USER_FEDERATION_COSTS#u1").
			Where("SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}).
			Limit(100)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_Federation_Statistics_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "FEDERATION_ACTIVE").
			Where("gsi1SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_Federation_TimeSeriesByDomain_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "FEDERATION_TIMESERIES#example.com#2026-08").
			Where("SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_Federation_TimeSeriesByPeriod_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi2Schema, func(q core.Query) core.Query {
		return q.Where("gsi2PK", "=", "PERIOD#2026-08").
			Where("gsi2SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-27T00:00:00Z#zzzz"})
	})
	assertSingleKeyCondition(t, compiled, "gsi2SK", "gsi2", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-27T00:00:00Z#zzzz"}, "")
}

func TestBatch1505_ImportExport_UserCosts_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "USER#u1").
			Where("gsi1SK", "BETWEEN", []any{"COST#2026-08-27T00:00:00Z", "COST#2026-08-28T00:00:00Z"}).
			OrderBy("gsi1SK", "DESC").
			Limit(100)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"COST#2026-08-27T00:00:00Z", "COST#2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_Metrics_ListByType_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "metrics#request").
			Where("SK", "BETWEEN", []any{"ts#20260827170919", "ts#20260828170919"}).
			OrderBy("SK", "DESC").
			Limit(20)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"ts#20260827170919", "ts#20260828170919"}, "")
}

func TestBatch1505_Metrics_ListByService_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "METRICS_SVC#api").
			Where("gsi1SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}).
			OrderBy("gsi1SK", "DESC").
			Limit(20)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_NotificationCost_AggregationsByPeriod_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "NOTIF_AGG#day#email").
			Where("SK", "BETWEEN", []any{"WINDOW#2026-08-27T00:00:00Z", "WINDOW#2026-08-28T00:00:00Z"}).
			OrderBy("SK", "DESC").
			Limit(500)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"WINDOW#2026-08-27T00:00:00Z", "WINDOW#2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_NotificationCost_DailySpending_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "USER#alice").
			Where("gsi1SK", "BETWEEN", []any{"COST#2026-08-27T00:00:00Z", "COST#2026-08-28T00:00:00Z"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"COST#2026-08-27T00:00:00Z", "COST#2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_ScheduledJobCost_ListByJob_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "SCHEDULED_JOB_COST#cleanup#daily").
			Where("SK", "BETWEEN", []any{"RUN#20260827170919", "RUN#20260828170919"}).
			OrderBy("SK", "DESC").
			Limit(100)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"RUN#20260827170919", "RUN#20260828170919"}, "")
}

func TestBatch1505_ScheduledJobCost_ListByStatus_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "SCHEDULED_JOB_STATUS#success").
			Where("gsi1SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}).
			OrderBy("gsi1SK", "DESC").
			Limit(100)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_CostTracking_GetCostProjections_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "COST_PROJECTIONS#day").
			Where("SK", "BETWEEN", []any{"DAILY#", "DAILY#~"}).
			OrderBy("SK", "DESC").
			Limit(1)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"DAILY#", "DAILY#~"}, "")
}

func TestBatch1505_AICost_ByTimeRange_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "AI_COSTS#2026-08").
			Where("gsi1SK", "BETWEEN", []any{"TS#0000000000001", "TS#0000000000002~"}).
			OrderBy("gsi1SK", "ASC")
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"TS#0000000000001", "TS#0000000000002~"}, "")
}

func TestBatch1505_AICost_GetAggregatedCosts_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "AI_AGG_TIME#day").
			Where("gsi1SK", "BETWEEN", []any{"20260827", "20260828"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"20260827", "20260828"}, "")
}

func TestBatch1505_QueryUtils_TimeRangeQuery_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "PK#x").
			Where("SK", "BETWEEN", []any{"TIME#100", "TIME#200"}).
			Limit(50)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"TIME#100", "TIME#200"}, "")
}

func TestBatch1505_RouteOptimizer_RouteMetrics_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "ROUTE#r1").
			Where("SK", "BETWEEN", []any{"RESULT#100", "RESULT#200"}).
			OrderBy("SK", "DESC").
			Limit(500)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"RESULT#100", "RESULT#200"}, "")
}

func TestBatch1505_RouteOptimizer_AllRouteMetrics_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "RESULTS").
			Where("gsi1SK", "BETWEEN", []any{"1756000000", "1756000001"}).
			OrderBy("gsi1SK", "DESC").
			Limit(500)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"1756000000", "1756000001"}, "")
}

func TestBatch1505_Instance_GetInstanceHistory_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "METRIC#memory").
			Where("gsi1SK", "BETWEEN", []any{"DATE#2026-08-27", "DATE#2026-08-28"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"DATE#2026-08-27", "DATE#2026-08-28"}, "")
}

func TestBatch1505_Instance_SummarizeInstanceMetrics_CompiledSingleBetweenKeyCondition(t *testing.T) {
	// Same keyed gsi1 DATE# window as GetInstanceHistory (issue #1500); pinned
	// separately because it is a distinct production site.
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "METRIC#cpu").
			Where("gsi1SK", "BETWEEN", []any{"DATE#2026-08-27", "DATE#2026-08-28"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"DATE#2026-08-27", "DATE#2026-08-28"}, "")
}

func TestBatch1505_Filter_GetUserFilters_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "USER#alice").
			Where("SK", "BETWEEN", []any{"FILTER#", "FILTER~"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"FILTER#", "FILTER~"}, "")
}

func TestBatch1505_Filter_GetFilterKeywords_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "FILTER#f1").
			Where("SK", "BETWEEN", []any{"KEYWORD#", "KEYWORD~"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"KEYWORD#", "KEYWORD~"}, "")
}

func TestBatch1505_Filter_GetFilterStatuses_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "FILTER#f1").
			Where("SK", "BETWEEN", []any{"STATUS#", "STATUS~"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"STATUS#", "STATUS~"}, "")
}

func TestBatch1505_Moderation_GetFiltersForUser_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "USER#alice").
			Where("SK", "BETWEEN", []any{"FILTER#", "FILTER~"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"FILTER#", "FILTER~"}, "")
}

func TestBatch1505_Moderation_GetFilterKeywords_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "FILTER#f1").
			Where("SK", "BETWEEN", []any{"KEYWORD#", "KEYWORD~"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"KEYWORD#", "KEYWORD~"}, "")
}

func TestBatch1505_Moderation_GetFilterStatuses_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "FILTER#f1").
			Where("SK", "BETWEEN", []any{"STATUS#", "STATUS~"})
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"STATUS#", "STATUS~"}, "")
}

func TestBatch1505_ModerationMetrics_GetFalsePositives_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "FALSE_POSITIVES").
			Where("gsi1SK", "BETWEEN", []any{"DATE#2026-08-27", "DATE#2026-08-28#Z"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"DATE#2026-08-27", "DATE#2026-08-28#Z"}, "")
}

func TestBatch1505_ModerationMetrics_GetDecisionSamples_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "DECISION#approve").
			Where("gsi1SK", "BETWEEN", []any{"DATE#2026-08-27", "DATE#2026-08-28#Z"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"DATE#2026-08-27", "DATE#2026-08-28#Z"}, "")
}

func TestBatch1505_ModerationMetrics_GetMetricsEntries_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "METRIC_TYPE#spam").
			Where("gsi1SK", "BETWEEN", []any{"DATE#2026-08-27", "DATE#2026-08-28#Z"})
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"DATE#2026-08-27", "DATE#2026-08-28#Z"}, "")
}

// ============================================================================
// Shape B — window + cursor: the cursor clamps the BETWEEN bound (inclusive),
// the cursor row is dropped post-read; the KCE stays a single BETWEEN.
// ============================================================================

func TestBatch1505_Audit_GetSecurityEvents_CompiledSingleBetweenKeyCondition(t *testing.T) {
	t.Run("first page window", func(t *testing.T) {
		compiled := compileB1505(t, gsi4Schema, func(q core.Query) core.Query {
			return q.Where("gsi4PK", "=", "SEVERITY#HIGH").
				Where("gsi4SK", "BETWEEN", []any{"AUDIT#100", "AUDIT#200"}).
				OrderBy("gsi4SK", "ASC").
				Limit(101)
		})
		assertSingleKeyCondition(t, compiled, "gsi4SK", "gsi4", "BETWEEN", []string{"AUDIT#100", "AUDIT#200"}, "")
	})
	t.Run("cursor page clamps the lower bound to the cursor", func(t *testing.T) {
		compiled := compileB1505(t, gsi4Schema, func(q core.Query) core.Query {
			return q.Where("gsi4PK", "=", "SEVERITY#HIGH").
				Where("gsi4SK", "BETWEEN", []any{"AUDIT#150", "AUDIT#200"}).
				OrderBy("gsi4SK", "ASC").
				Limit(102)
		})
		assertSingleKeyCondition(t, compiled, "gsi4SK", "gsi4", "BETWEEN", []string{"AUDIT#150", "AUDIT#200"}, "")
	})
	t.Run("no-window cursor page keys the bare bound", func(t *testing.T) {
		compiled := compileB1505(t, gsi4Schema, func(q core.Query) core.Query {
			return q.Where("gsi4PK", "=", "SEVERITY#HIGH").
				Where("gsi4SK", ">", "AUDIT#150").
				OrderBy("gsi4SK", "ASC").
				Limit(101)
		})
		assertSingleKeyCondition(t, compiled, "gsi4SK", "gsi4", ">", nil, "")
	})
}

func TestBatch1505_Base_ListAggregatedByPeriod_CompiledSingleBetweenKeyCondition(t *testing.T) {
	t.Run("first page window", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "cost_agg#day#entity").
				Where("SK", "BETWEEN", []any{"window#2026-08-27T00:00:00Z", "window#2026-08-28T00:00:00Z"}).
				OrderBy("SK", "DESC").
				Limit(11)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"window#2026-08-27T00:00:00Z", "window#2026-08-28T00:00:00Z"}, "")
	})
	t.Run("cursor page clamps the upper bound to the cursor", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "cost_agg#day#entity").
				Where("SK", "BETWEEN", []any{"window#2026-08-27T00:00:00Z", "window#cursor"}).
				OrderBy("SK", "DESC").
				Limit(12)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"window#2026-08-27T00:00:00Z", "window#cursor"}, "")
	})
}

func TestBatch1505_Base_QueryBetweenPaginated_CompiledSingleBetweenKeyCondition(t *testing.T) {
	t.Run("desc cursor page", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "PK#1").
				Where("SK", "BETWEEN", []any{"SK#start", "SK#cursor"}).
				OrderBy("SK", "DESC").
				Limit(12)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"SK#start", "SK#cursor"}, "")
	})
	t.Run("asc cursor page", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "PK#1").
				Where("SK", "BETWEEN", []any{"SK#cursor", "SK#end"}).
				OrderBy("SK", "ASC").
				Limit(12)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"SK#cursor", "SK#end"}, "")
	})
}

func TestBatch1505_CostTracking_ListByTable_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "COST_TABLE#users").
			Where("gsi1SK", "BETWEEN", []any{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}).
			OrderBy("gsi1SK", "DESC").
			Limit(11)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"2026-08-27T00:00:00Z", "2026-08-28T00:00:00Z"}, "")
}

func TestBatch1505_CostTracking_GetRelayCostsByURL_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1OpSchema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "RELAY_COSTS#relay.example").
			Where("gsi1SK", "BETWEEN", []any{"TS#20260827", "TS#20260828"}).
			OrderBy("gsi1SK", "DESC").
			Limit(11).
			Filter("OperationType", "=", "deliver")
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"TS#20260827", "TS#20260828"}, "attr:OperationType")
}

func TestBatch1505_CostTracking_GetRelayMetricsHistory_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "RELAY_METRICS#relay.example").
			Where("gsi1SK", "BETWEEN", []any{"daily#20260827", "daily#20260828"}).
			OrderBy("gsi1SK", "DESC").
			Limit(11)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "BETWEEN", []string{"daily#20260827", "daily#20260828"}, "")
}

func TestBatch1505_Draft_ListScheduledDraftsDuePaginated_CompiledSingleBetweenKeyCondition(t *testing.T) {
	t.Run("first page keys the bare upper bound", func(t *testing.T) {
		compiled := compileB1505(t, gsi4Schema, func(q core.Query) core.Query {
			return q.Where("gsi4PK", "=", "DRAFT#STATUS#scheduled").
				Where("gsi4SK", "<=", "TIME#1756000000~").
				OrderBy("gsi4SK", "ASC").
				Limit(26)
		})
		assertSingleKeyConditionLE(t, compiled, "gsi4SK", "gsi4", "<=", []string{"TIME#1756000000~"}, "", true)
	})
	t.Run("cursor page between cursor and cutoff", func(t *testing.T) {
		compiled := compileB1505(t, gsi4Schema, func(q core.Query) core.Query {
			return q.Where("gsi4PK", "=", "DRAFT#STATUS#scheduled").
				Where("gsi4SK", "BETWEEN", []any{"TIME#1755999999~", "TIME#1756000000~"}).
				OrderBy("gsi4SK", "ASC").
				Limit(27)
		})
		assertSingleKeyCondition(t, compiled, "gsi4SK", "gsi4", "BETWEEN", []string{"TIME#1755999999~", "TIME#1756000000~"}, "")
	})
}

// ============================================================================
// Shape C — prefix + cursor: the cursor page keys the exclusive bound and
// demotes BEGINS_WITH to a post-read FilterExpression.
// ============================================================================

func TestBatch1505_PaginationHelpers_ListByPKSKPrefix_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("first page keys begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "ASC").
				Where("SK", "BEGINS_WITH", "FOLLOW#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "begins_with(", []string{"FOLLOW#"}, "")
	})
	t.Run("cursor page keys the bound and filters begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "ASC").
				Where("SK", ">", "FOLLOW#bob").
				Filter("SK", "BEGINS_WITH", "FOLLOW#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", ">", []string{"FOLLOW#bob"}, "begins_with")
	})
}

func TestBatch1505_Account_SearchHandles_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("first page keys begins_with", func(t *testing.T) {
		compiled := compileB1505(t, gsi5Schema, func(q core.Query) core.Query {
			return q.Where("gsi5PK", "=", "USER_HANDLE_PREFIX#al").
				OrderBy("gsi5SK", "ASC").
				Where("gsi5SK", "BEGINS_WITH", "alice").
				Limit(21)
		})
		assertSingleKeyCondition(t, compiled, "gsi5SK", "gsi5", "begins_with(", []string{"alice"}, "")
	})
	t.Run("cursor page keys the bound and filters begins_with", func(t *testing.T) {
		compiled := compileB1505(t, gsi5Schema, func(q core.Query) core.Query {
			return q.Where("gsi5PK", "=", "USER_HANDLE_PREFIX#al").
				OrderBy("gsi5SK", "ASC").
				Where("gsi5SK", ">", "alice#1").
				Filter("gsi5SK", "BEGINS_WITH", "alice").
				Limit(21)
		})
		assertSingleKeyCondition(t, compiled, "gsi5SK", "gsi5", ">", []string{"alice#1"}, "begins_with")
	})
}

func TestBatch1505_Account_ShortHandlePrefixPartition_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi5Schema, func(q core.Query) core.Query {
		return q.Where("gsi5PK", "=", "USER_HANDLE_PREFIX#al").
			OrderBy("gsi5SK", "ASC").
			Where("gsi5SK", ">", "alice#1").
			Filter("gsi5SK", "BEGINS_WITH", "alice").
			Limit(21)
	})
	assertSingleKeyCondition(t, compiled, "gsi5SK", "gsi5", ">", []string{"alice#1"}, "begins_with")
}

func TestBatch1505_Account_GetLoginHistory_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("first page keys begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "begins_with", "LOGIN#").
				Limit(51)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "begins_with(", []string{"LOGIN#"}, "")
	})
	t.Run("cursor page keys the bound and filters begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "<", "LOGIN#123").
				Filter("SK", "begins_with", "LOGIN#").
				Limit(51)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"LOGIN#123"}, "begins_with")
	})
}

func TestBatch1505_AccountSocial_GetFollowing_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "FOLLOW#alice").
			OrderBy("SK", "ASC").
			Where("SK", ">", "following#bob").
			Filter("SK", "BEGINS_WITH", "following#").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", ">", []string{"following#bob"}, "begins_with")
}

func TestBatch1505_Auth_WalletCredentials_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "WALLET#w1").
			OrderBy("SK", "ASC").
			Where("SK", ">", "CRED#c1").
			Filter("SK", "BEGINS_WITH", "CRED#").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", ">", []string{"CRED#c1"}, "begins_with")
}

func TestBatch1505_Base_QueryWithSKPrefix_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("desc cursor page", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "PK#1").
				OrderBy("SK", "DESC").
				Where("SK", "<", "SK#zzz").
				Filter("SK", "BEGINS_WITH", "SK#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"SK#zzz"}, "begins_with")
	})
	t.Run("asc cursor page", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "PK#1").
				OrderBy("SK", "ASC").
				Where("SK", ">", "SK#zzz").
				Filter("SK", "BEGINS_WITH", "SK#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", ">", []string{"SK#zzz"}, "begins_with")
	})
	t.Run("first page keys begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "PK#1").
				OrderBy("SK", "DESC").
				Where("SK", "BEGINS_WITH", "SK#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "begins_with(", []string{"SK#"}, "")
	})
}

func TestBatch1505_Base_QueryCollectionWithConversion_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "OBJ#1").
			Where("SK", ">", "LIKES#c1").
			Filter("SK", "BEGINS_WITH", "LIKES").
			Limit(11)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", ">", []string{"LIKES#c1"}, "begins_with")
}

func TestBatch1505_QueryCache_PrefixQuery_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "CACHE#ns").
			OrderBy("SK", "ASC").
			Where("SK", ">", "KEY#x").
			Filter("SK", "begins_with", "KEY#pre").
			Limit(201)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", ">", []string{"KEY#x"}, "begins_with")
}

func TestBatch1505_Revision_ListRevisionsPaginated_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "OBJECT#o1#REVISION").
			OrderBy("SK", "DESC").
			Where("SK", "<", "VERSION#v10").
			Filter("SK", "BEGINS_WITH", "VERSION#").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"VERSION#v10"}, "begins_with")
}

func TestBatch1505_Article_CMSIndexPaginated_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "CMS_CATEGORY#c1").
			OrderBy("SK", "DESC").
			Where("SK", "<", "ART#x").
			Filter("SK", "BEGINS_WITH", "ART#").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"ART#x"}, "begins_with")
}

func TestBatch1505_Search_HashtagSearch_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("first page keys begins_with", func(t *testing.T) {
		compiled := compileB1505(t, gsi3Schema, func(q core.Query) core.Query {
			return q.Where("gsi3PK", "=", "HASHTAG_SEARCH#ab").
				OrderBy("gsi3SK", "ASC").
				Where("gsi3SK", "BEGINS_WITH", "abc").
				Limit(21)
		})
		assertSingleKeyCondition(t, compiled, "gsi3SK", "gsi3", "begins_with(", []string{"abc"}, "")
	})
	t.Run("cursor page keys the bound and filters begins_with", func(t *testing.T) {
		compiled := compileB1505(t, gsi3Schema, func(q core.Query) core.Query {
			return q.Where("gsi3PK", "=", "HASHTAG_SEARCH#ab").
				OrderBy("gsi3SK", "ASC").
				Where("gsi3SK", ">", "abc#1").
				Filter("gsi3SK", "BEGINS_WITH", "abc").
				Limit(21)
		})
		assertSingleKeyCondition(t, compiled, "gsi3SK", "gsi3", ">", []string{"abc#1"}, "begins_with")
	})
}

// ============================================================================
// Shape D — timeline family: `< maxID` keyed (DESC paging bound); begins_with
// and `> sinceID` become post-read FilterExpressions.
// ============================================================================

func TestBatch1505_Timeline_GetHomeTimeline_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("first page keys begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "BEGINS_WITH", "HOME#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "begins_with(", []string{"HOME#"}, "")
	})
	t.Run("maxID page keys the bound and filters begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "<", "HOME#max").
				Filter("SK", "BEGINS_WITH", "HOME#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"HOME#max"}, "begins_with")
	})
	t.Run("maxID + sinceID page filters both", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "<", "HOME#max").
				Filter("SK", "BEGINS_WITH", "HOME#").
				Filter("SK", ">", "HOME#since").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"HOME#max"}, "begins_with")
		require.Contains(t, compiled.FilterExpression, ">")
	})
}

func TestBatch1505_Timeline_GetLocalTimeline_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi1Schema, func(q core.Query) core.Query {
		return q.Where("gsi1PK", "=", "LOCAL_TIMELINE").
			OrderBy("gsi1SK", "DESC").
			Where("gsi1SK", "<", "max").
			Filter("gsi1SK", ">", "since").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "gsi1SK", "gsi1", "<", []string{"max"}, ">")
}

func TestBatch1505_Timeline_GetPublicTimeline_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi2Schema, func(q core.Query) core.Query {
		return q.Where("gsi2PK", "=", "PUBLIC_TIMELINE").
			OrderBy("gsi2SK", "DESC").
			Where("gsi2SK", "<", "max").
			Filter("gsi2SK", ">", "since").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "gsi2SK", "gsi2", "<", []string{"max"}, ">")
}

func TestBatch1505_Timeline_GetHashtagTimeline_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi3Schema, func(q core.Query) core.Query {
		return q.Where("gsi3PK", "=", "HASHTAG#golang").
			OrderBy("gsi3SK", "DESC").
			Where("gsi3SK", "<", "max").
			Filter("gsi3SK", ">", "since").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "gsi3SK", "gsi3", "<", []string{"max"}, ">")
}

func TestBatch1505_Timeline_GetListTimeline_CompiledSingleKeyCondition(t *testing.T) {
	compiled := compileB1505(t, gsi4Schema, func(q core.Query) core.Query {
		return q.Where("gsi4PK", "=", "LIST#l1").
			OrderBy("gsi4SK", "DESC").
			Where("gsi4SK", "<", "max").
			Filter("gsi4SK", ">", "since").
			Limit(26)
	})
	assertSingleKeyCondition(t, compiled, "gsi4SK", "gsi4", "<", []string{"max"}, ">")
}

func TestBatch1505_Timeline_GetConversations_CompiledSingleKeyCondition(t *testing.T) {
	t.Run("maxID page keys the bound and filters begins_with", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "<", "CONVERSATION#max").
				Filter("SK", "BEGINS_WITH", "CONVERSATION#").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"CONVERSATION#max"}, "begins_with")
	})
	t.Run("maxID + sinceID page filters both", func(t *testing.T) {
		compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
			return q.Where("PK", "=", "USER#alice").
				OrderBy("SK", "DESC").
				Where("SK", "<", "CONVERSATION#max").
				Filter("SK", "BEGINS_WITH", "CONVERSATION#").
				Filter("SK", ">", "CONVERSATION#since").
				Limit(26)
		})
		assertSingleKeyCondition(t, compiled, "SK", "", "<", []string{"CONVERSATION#max"}, "begins_with")
		require.Contains(t, compiled.FilterExpression, ">")
	})
}
