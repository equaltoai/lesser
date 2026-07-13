package repositories

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	ttquery "github.com/theory-cloud/tabletheory/v2/pkg/query"
)

type notificationCompileMetadata struct{}

func (notificationCompileMetadata) TableName() string {
	return models.MainTableName
}

func (notificationCompileMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{PartitionKey: "PK", SortKey: "SK"}
}

func (notificationCompileMetadata) Indexes() []core.IndexSchema {
	return nil
}

func (notificationCompileMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	switch field {
	case "PK", "SK":
		return &core.AttributeMetadata{Name: field, DynamoDBName: field, Type: "string"}
	default:
		return nil
	}
}

func (notificationCompileMetadata) VersionFieldName() string {
	return "version"
}

func compileNotificationSortKeyScope(t *testing.T, cursor string) *core.CompiledQuery {
	t.Helper()

	q := ttquery.New(&models.Notification{}, notificationCompileMetadata{}, nil)
	scoped := applyNotificationSortKeyScope(
		q.Where("PK", "=", "USER#alice").OrderBy("SK", "DESC"),
		cursor,
	)

	compiled, err := scoped.(*ttquery.Query).Compile()
	require.NoError(t, err)
	return compiled
}

func countCompiledNameValues(compiled *core.CompiledQuery, attr string) int {
	count := 0
	for _, value := range compiled.ExpressionAttributeNames {
		if value == attr {
			count++
		}
	}
	return count
}

func compiledStringValues(compiled *core.CompiledQuery) []string {
	out := make([]string, 0, len(compiled.ExpressionAttributeValues))
	for _, value := range compiled.ExpressionAttributeValues {
		if stringValue, ok := value.(*types.AttributeValueMemberS); ok {
			out = append(out, stringValue.Value)
		}
	}
	return out
}

func TestNotificationRepository_TableTheoryCompiledSortKeyScope(t *testing.T) {
	t.Run("no cursor compiles to one begins_with sort key condition", func(t *testing.T) {
		compiled := compileNotificationSortKeyScope(t, "")

		require.Equal(t, "Query", compiled.Operation)
		require.Contains(t, compiled.KeyConditionExpression, "begins_with")
		require.NotContains(t, compiled.KeyConditionExpression, "BETWEEN")
		require.Equal(t, 1, countCompiledNameValues(compiled, "SK"))
		require.Contains(t, compiledStringValues(compiled), notificationSortKeyPrefix)
	})

	t.Run("cursor compiles to one prefix-safe between sort key condition", func(t *testing.T) {
		cursor := "notif#20260428120000#cursor"
		compiled := compileNotificationSortKeyScope(t, cursor)

		require.Equal(t, "Query", compiled.Operation)
		require.Contains(t, compiled.KeyConditionExpression, " BETWEEN ")
		require.NotContains(t, compiled.KeyConditionExpression, "begins_with")
		require.NotContains(t, compiled.KeyConditionExpression, " < ")
		require.Equal(t, 1, countCompiledNameValues(compiled, "SK"))
		values := compiledStringValues(compiled)
		require.Contains(t, values, notificationSortKeyPrefix)
		require.Contains(t, values, cursor)
	})

	t.Run("malformed cursor falls back to prefix condition", func(t *testing.T) {
		compiled := compileNotificationSortKeyScope(t, "not-a-notification-sk")

		require.Contains(t, compiled.KeyConditionExpression, "begins_with")
		require.NotContains(t, compiled.KeyConditionExpression, "BETWEEN")
		require.NotContains(t, compiled.KeyConditionExpression, " < ")
		require.Equal(t, 1, countCompiledNameValues(compiled, "SK"))
	})
}
