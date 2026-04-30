package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestExecuteSecurityFindingsHashtagIndexCleanup_DryRunPlansNonPublicRepairs(t *testing.T) {
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{
				{Items: []map[string]types.AttributeValue{
					statusHashtagPrimaryItem("status#public", "status#public", "public"),
					statusHashtagPrimaryItem("status#private", "status#private", "private"),
				}},
				{Items: []map[string]types.AttributeValue{
					hashtagIndexItem("HASHTAG_INDEX#go", "2026#public", "public"),
					hashtagIndexItem("HASHTAG_INDEX#go", "2026#direct", "direct"),
				}},
				{Items: []map[string]types.AttributeValue{
					hashtagIndexItem("HASHTAG_TIMELINE#go", "STATUS#1#public", "public"),
					hashtagIndexItem("HASHTAG_TIMELINE#go", "STATUS#2#private", "private"),
				}},
			},
		},
	}

	summary, err := executeSecurityFindingsHashtagIndexCleanup(context.Background(), client, "theory-dev-main-table", false, 0)
	require.NoError(t, err)
	require.Equal(t, "hashtag-indexes", summary.Name)
	require.Equal(t, 6, summary.Scanned)
	require.Equal(t, 3, summary.Candidates)
	require.Equal(t, 3, summary.PlannedWrites)
	require.Equal(t, 0, summary.AppliedWrites)
	require.Empty(t, client.updateInputs)
	require.Empty(t, client.deleteInputs)
	require.Len(t, summary.Samples, 3)
	require.Contains(t, summary.Samples[0], "remove gsi5")
	require.Contains(t, summary.Samples[1], "delete stale hashtag index")
}

func TestExecuteSecurityFindingsHashtagIndexCleanup_ApplyUpdatesAndDeletes(t *testing.T) {
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{
				{Items: []map[string]types.AttributeValue{
					statusHashtagPrimaryItem("status#private", "status#private", "private"),
				}},
				{Items: []map[string]types.AttributeValue{
					hashtagIndexItem("HASHTAG_INDEX#go", "2026#private", "private"),
				}},
				{Items: []map[string]types.AttributeValue{
					hashtagIndexItem("HASHTAG_TIMELINE#go", "STATUS#2#direct", "direct"),
				}},
			},
		},
	}

	summary, err := executeSecurityFindingsHashtagIndexCleanup(context.Background(), client, "theory-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 3, summary.Candidates)
	require.Equal(t, 3, summary.AppliedWrites)
	require.Len(t, client.updateInputs, 1)
	require.Equal(t, "REMOVE gsi5PK, gsi5SK", *client.updateInputs[0].UpdateExpression)
	require.Equal(t, "status#private", strAttr(t, client.updateInputs[0].Key["PK"]))
	require.Len(t, client.deleteInputs, 2)
	require.Equal(t, "HASHTAG_INDEX#go", strAttr(t, client.deleteInputs[0].Key["PK"]))
	require.Equal(t, "HASHTAG_TIMELINE#go", strAttr(t, client.deleteInputs[1].Key["PK"]))
}

func TestExecuteSecurityFindingsHashtagIndexCleanup_RespectsLimitAcrossActions(t *testing.T) {
	client := &fakeSecurityFindingsMigrationClient{
		fakeUserKeyMigrationClient: fakeUserKeyMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{
				{Items: []map[string]types.AttributeValue{
					statusHashtagPrimaryItem("status#one", "status#one", "private"),
					statusHashtagPrimaryItem("status#two", "status#two", "private"),
				}},
				{Items: []map[string]types.AttributeValue{
					hashtagIndexItem("HASHTAG_INDEX#go", "2026#private", "private"),
				}},
				{Items: nil},
			},
		},
	}

	summary, err := executeSecurityFindingsHashtagIndexCleanup(context.Background(), client, "theory-dev-main-table", true, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.AppliedWrites)
	require.Len(t, client.updateInputs, 1)
	require.Empty(t, client.deleteInputs)
}

func statusHashtagPrimaryItem(pk, sk, visibility string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":         sAttr(pk),
		"SK":         sAttr(sk),
		"gsi5PK":     sAttr("HASHTAG#go"),
		"gsi5SK":     sAttr("2026#status"),
		"visibility": sAttr(visibility),
	}
}

func hashtagIndexItem(pk, sk, visibility string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":         sAttr(pk),
		"SK":         sAttr(sk),
		"visibility": sAttr(visibility),
	}
}
