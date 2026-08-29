package repositories

// Batch #1505 (issue #1500) — two-range sort-key KeyConditionExpression
// consolidation.
//
// Every fixed production site is now pinned at the DynamoDB-contract level by
// SEAM-driven tests in scanfree_wave1469_batch_1505_seam_test.go: each seam
// test drives the REAL production repository function through a capturing
// core.DB seam and asserts the compiled query the production chain actually
// builds — Operation == Query, EXACTLY ONE condition on the sort key in the
// KeyConditionExpression, no `>=` + `<=` pair. A mutation restoring the old
// two-range pair (`>=` + `<=`, or `begins_with` + `>`, etc.) dies on the
// duplicate name-value count and on the stray range operator.
//
// This file retains only the shared contract helpers the seam file reuses
// (compileRow / compileSchema / compileB1505 / countKCEAttr /
// compiledStringValues / assertSingleKeyCondition) plus mirror pins for the
// sites the seam cannot drive.
//
// Seam-unsupported sites:
//   - QueryUtils_TimeRangeQuery: production compiles against
//     Model(&[]map[string]interface{}) — a slice-of-maps model tabletheory
//     refuses to register, so the seam cannot capture or compile the query;
//     the mirror below pins the intended chain.
//   - QueryCache_PrefixQuery: the seam cannot drive invalidateCachePrefix's
//     cursor page (the BETWEEN window is built only after the first page
//     returns rows), so the mirror pins the production cursor-page chain —
//     SK BETWEEN [cursor, prefix~] with begins_with demoted to a post-read
//     filter — at the compile level.

import (
	"regexp"
	"strings"
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

var mainSchema = compileSchema{fields: []string{"PK", "SK"}}

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
// DynamoDB-rejected shape. A filter named "attr:<name>" asserts the compiled
// FilterExpression references exactly one ExpressionAttributeName equal to
// <name> (the DynamoDB attribute, e.g. the camelCase `operationType`).
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
	} else if strings.HasPrefix(filter, "attr:") {
		require.Equal(t, 1, countCompiledNameValues(compiled, strings.TrimPrefix(filter, "attr:")))
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
// Seam-unsupported mirror pins — sites the seam cannot drive (see header).
// ============================================================================

func TestBatch1505_QueryUtils_TimeRangeQuery_CompiledSingleBetweenKeyCondition(t *testing.T) {
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "PK#x").
			Where("SK", "BETWEEN", []any{"TIME#100", "TIME#200"}).
			Limit(50)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"TIME#100", "TIME#200"}, "")
}

func TestBatch1505_QueryCache_PrefixQuery_CompiledSingleKeyCondition(t *testing.T) {
	// Production invalidateCachePrefix cursor page (see the file header): the
	// `~` sentinel (0x7E) closes the key range at the top of the block so the
	// final page can never keep reading the partition tail; begins_with is
	// demoted to a post-read FilterExpression. The fetch limit is pageLimit+2:
	// the first +1 lets len(entries) > pageLimit detect a next page; the second
	// +1 (cursor pages only) covers the inclusive cursor row, which
	// dropSortKeyCursorDuplicate removes before the hasMore check.
	compiled := compileB1505(t, mainSchema, func(q core.Query) core.Query {
		return q.Where("PK", "=", "CACHE#ns").
			OrderBy("SK", "ASC").
			Where("SK", "BETWEEN", []any{"KEY#x", "KEY#pre~"}).
			Filter("SK", "begins_with", "KEY#pre").
			Limit(202)
	})
	assertSingleKeyCondition(t, compiled, "SK", "", "BETWEEN", []string{"KEY#x", "KEY#pre~"}, "begins_with")
}
