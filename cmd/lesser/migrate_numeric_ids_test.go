package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestRunMigrateNumericIDs_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newUserKeyMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newUserKeyMigrationClientFn = previousClientFactory
	})

	legacyNumericID := common.GenerateNumericID("Medic")
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					legacyActorMigrationItem("medic", legacyNumericID, "Medic"),
				},
			},
			{
				Items: []map[string]types.AttributeValue{
					legacyNumericMappingItem(legacyNumericID, "Medic", "https://example.com/users/Medic"),
				},
			},
		},
	}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newUserKeyMigrationClientFn = func(aws.Config) userKeyMigrationClient { return client }

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateNumericIDs([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-numeric-ids dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_actors: 1")
	require.Contains(t, output, "candidates: 1")
	require.Contains(t, output, "actor_rows_updated: 0")
	require.Contains(t, output, "mappings_upserted: 0")
	require.Contains(t, output, "legacy_mappings_deleted: 0")
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Len(t, client.scanInputs, 2)
	require.Equal(t, actorPartitionPrefix, strAttr(t, client.scanInputs[0].ExpressionAttributeValues[":prefix"]))
	require.Equal(t, numericMappingPartition, strAttr(t, client.scanInputs[1].ExpressionAttributeValues[":prefix"]))
}

func TestExecuteNumericIDMigration_ApplyRewritesLegacyActorAndMapping(t *testing.T) {
	legacyNumericID := common.GenerateNumericID("Pilot")
	desiredNumericID := common.GenerateNumericID("pilot")
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					legacyActorMigrationItem("pilot", legacyNumericID, "Pilot"),
				},
			},
			{
				Items: []map[string]types.AttributeValue{
					legacyNumericMappingItem(legacyNumericID, "Pilot", "https://example.com/users/Pilot"),
				},
			},
		},
	}

	summary, err := executeNumericIDMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedActors)
	require.Equal(t, 1, summary.Candidates)
	require.Equal(t, 1, summary.ActorRowsUpdated)
	require.Equal(t, 1, summary.MappingsUpserted)
	require.Equal(t, 1, summary.LegacyMappingsDeleted)

	require.Len(t, client.putInputs, 2)
	actorPut := client.putInputs[0].Item
	require.Equal(t, "ACTOR#pilot", strAttr(t, actorPut["PK"]))
	require.Equal(t, desiredNumericID, strAttr(t, actorPut["numericID"]))
	require.Equal(t, "pilot", strAttr(t, actorPut["username"]))
	actorValue, ok := actorPut["actor"].(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Equal(t, "pilot", strAttr(t, actorValue.Value["preferredUsername"]))

	mappingPut := client.putInputs[1].Item
	require.Equal(t, "NUMERIC_ID#"+desiredNumericID, strAttr(t, mappingPut["PK"]))
	require.Equal(t, desiredNumericID, strAttr(t, mappingPut["numericID"]))
	require.Equal(t, "pilot", strAttr(t, mappingPut["username"]))
	require.Equal(t, "https://example.com/users/pilot", strAttr(t, mappingPut["actorID"]))
	require.Equal(t, numericMappingTypeName, strAttr(t, mappingPut["type"]))

	require.Len(t, client.deleteInputs, 1)
	require.Equal(t, "NUMERIC_ID#"+legacyNumericID, strAttr(t, client.deleteInputs[0].Key["PK"]))
	require.Equal(t, numericMetadataSortKey, strAttr(t, client.deleteInputs[0].Key["SK"]))
}

func TestExecuteNumericIDMigration_SkipsConformantActor(t *testing.T) {
	numericID := common.GenerateNumericID("scout")
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					legacyActorMigrationItem("scout", numericID, "scout"),
				},
			},
			{
				Items: []map[string]types.AttributeValue{
					legacyNumericMappingItem(numericID, "scout", "https://example.com/users/scout"),
				},
			},
		},
	}

	summary, err := executeNumericIDMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedActors)
	require.Equal(t, 0, summary.Candidates)
	require.Empty(t, client.putInputs)
	require.Empty(t, client.deleteInputs)
}

func TestExecuteNumericIDMigration_ValidatesInputsAndScanErrors(t *testing.T) {
	_, err := executeNumericIDMigration(context.Background(), nil, "simulacrum-dev-main-table", false, 0)
	require.Error(t, err)

	_, err = executeNumericIDMigration(context.Background(), &fakeUserKeyMigrationClient{}, "", false, 0)
	require.Error(t, err)

	client := &fakeUserKeyMigrationClient{scanErr: context.DeadlineExceeded}
	_, err = executeNumericIDMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
	require.Error(t, err)
	require.ErrorContains(t, err, "scan actor profiles")
}

func TestNumericIDMigrationHelpers(t *testing.T) {
	t.Run("canonical username falls back through item fields", func(t *testing.T) {
		require.Equal(t, "alice", canonicalMigrationUsername(map[string]types.AttributeValue{
			"username": sAttr("Alice"),
		}, "ACTOR#ignored"))

		require.Equal(t, "bob", canonicalMigrationUsername(map[string]types.AttributeValue{
			"actor": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"preferredUsername": sAttr("Bob"),
			}},
		}, "ACTOR#ignored"))

		require.Equal(t, "carol", canonicalMigrationUsername(map[string]types.AttributeValue{}, "ACTOR#Carol"))
	})

	t.Run("normalize actor payload identity rewrites canonical fields", func(t *testing.T) {
		actorItem := map[string]types.AttributeValue{
			"actor": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"preferredUsername": sAttr("Agent-0"),
				"id":                sAttr("https://example.com/users/Agent-0"),
				"url":               sAttr("https://example.com/@Agent-0"),
				"inbox":             sAttr("https://example.com/users/Agent-0/inbox"),
			}},
		}

		require.True(t, normalizeActorPayloadIdentity(actorItem, "agent-0"))
		actorValue := actorItem["actor"].(*types.AttributeValueMemberM)
		require.Equal(t, "agent-0", strAttr(t, actorValue.Value["preferredUsername"]))
		require.Equal(t, "https://example.com/users/agent-0", strAttr(t, actorValue.Value["id"]))
		require.Equal(t, "https://example.com/@agent-0", strAttr(t, actorValue.Value["url"]))
		require.Equal(t, "https://example.com/users/Agent-0/inbox", strAttr(t, actorValue.Value["inbox"]))
		require.False(t, normalizeActorPayloadIdentity(map[string]types.AttributeValue{}, "agent-0"))
	})

	t.Run("desired actor ID and mapping equivalence helpers", func(t *testing.T) {
		actorItem := map[string]types.AttributeValue{
			"actor": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"id": sAttr("https://example.com/users/Agent-0"),
			}},
		}
		existingDesired := legacyNumericMappingItem(common.GenerateNumericID("agent-0"), "agent-0", "https://example.com/users/agent-0")
		legacy := legacyNumericMappingItem(common.GenerateNumericID("Agent-0"), "Agent-0", "https://example.com/users/Agent-0")

		require.Equal(t, "https://example.com/users/agent-0", desiredActorIDForMapping(actorItem, existingDesired, legacy, "agent-0"))
		require.Equal(t, "https://example.com/users/agent-0", desiredActorIDForMapping(map[string]types.AttributeValue{}, existingDesired, nil, "agent-0"))
		require.Equal(t, "https://example.com/users/agent-0", desiredActorIDForMapping(map[string]types.AttributeValue{}, nil, legacy, "agent-0"))
		require.Empty(t, desiredActorIDForMapping(map[string]types.AttributeValue{}, nil, nil, "agent-0"))

		createdAt := desiredCreatedAtAttribute(existingDesired, nil)
		require.Equal(t, "2026-03-24T12:00:00Z", strAttr(t, createdAt))
		newCreatedAt := desiredCreatedAtAttribute(nil, nil)
		_, ok := newCreatedAt.(*types.AttributeValueMemberS)
		require.True(t, ok)

		desired := buildNumericIDMappingItem(common.GenerateNumericID("agent-0"), "agent-0", "https://example.com/users/agent-0", createdAt)
		require.True(t, mappingItemsEquivalent(existingDesired, desired))
		desired["username"] = sAttr("Agent-0")
		require.False(t, mappingItemsEquivalent(existingDesired, desired))
	})

	t.Run("clone attribute helpers deep copy nested values", func(t *testing.T) {
		original := map[string]types.AttributeValue{
			"list": &types.AttributeValueMemberL{Value: []types.AttributeValue{sAttr("one")}},
			"map": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"nested": sAttr("value"),
			}},
			"binary": &types.AttributeValueMemberB{Value: []byte("hi")},
		}

		cloned := cloneAttributeMap(original)
		cloned["map"].(*types.AttributeValueMemberM).Value["nested"] = sAttr("changed")
		cloned["list"].(*types.AttributeValueMemberL).Value[0] = sAttr("two")
		cloned["binary"].(*types.AttributeValueMemberB).Value[0] = 'H'

		require.Equal(t, "value", strAttr(t, original["map"].(*types.AttributeValueMemberM).Value["nested"]))
		require.Equal(t, "one", strAttr(t, original["list"].(*types.AttributeValueMemberL).Value[0]))
		require.Equal(t, byte('h'), original["binary"].(*types.AttributeValueMemberB).Value[0])

		require.Equal(t, "https://example.com/@agent-0", normalizeActorReference("https://example.com/@Agent-0", "agent-0"))
		require.Equal(t, "https://remote.example/actors/Agent-0", normalizeActorReference("https://remote.example/actors/Agent-0", "agent-0"))
	})
}

func legacyActorMigrationItem(username, numericID, preferredUsername string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":        sAttr("ACTOR#" + username),
		"SK":        sAttr(actorProfileSortKey),
		"username":  sAttr(username),
		"numericID": sAttr(numericID),
		"actor": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"preferredUsername": sAttr(preferredUsername),
		}},
	}
}

func legacyNumericMappingItem(numericID, username, actorID string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":        sAttr(numericMappingPartition + numericID),
		"SK":        sAttr(numericMetadataSortKey),
		"numericID": sAttr(numericID),
		"username":  sAttr(username),
		"actorID":   sAttr(actorID),
		"type":      sAttr(numericMappingTypeName),
		"createdAt": sAttr("2026-03-24T12:00:00Z"),
	}
}
