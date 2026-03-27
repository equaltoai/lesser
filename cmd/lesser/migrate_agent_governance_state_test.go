package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type fakeAgentGovernanceStateMigrationClient struct {
	queryOutputs []*dynamodb.QueryOutput
	getItems     map[string]map[string]types.AttributeValue
	putItems     []map[string]types.AttributeValue
	queryErr     error
	getErr       error
	putErr       error
	putErrs      []error
	queryCalls   int
}

func (f *fakeAgentGovernanceStateMigrationClient) Query(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	f.queryCalls++
	if len(f.queryOutputs) == 0 {
		return &dynamodb.QueryOutput{}, nil
	}
	out := f.queryOutputs[0]
	f.queryOutputs = f.queryOutputs[1:]
	return out, nil
}

func (f *fakeAgentGovernanceStateMigrationClient) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	pk := agentGovernanceTestStringAttr(input.Key["PK"])
	sk := agentGovernanceTestStringAttr(input.Key["SK"])
	item := f.getItems[pk+"#"+sk]
	if item == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: cloneAttributeValueMap(item)}, nil
}

func (f *fakeAgentGovernanceStateMigrationClient) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if f.putErr != nil {
		return nil, f.putErr
	}
	if len(f.putErrs) > 0 {
		err := f.putErrs[0]
		f.putErrs = f.putErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	cloned := cloneAttributeValueMap(input.Item)
	f.putItems = append(f.putItems, cloned)
	pk := agentGovernanceTestStringAttr(cloned["PK"])
	sk := agentGovernanceTestStringAttr(cloned["SK"])
	if f.getItems == nil {
		f.getItems = map[string]map[string]types.AttributeValue{}
	}
	f.getItems[pk+"#"+sk] = cloned
	return &dynamodb.PutItemOutput{}, nil
}

func TestExecuteAgentGovernanceStateMigration_BackfillsAndCleansLegacyMetadata(t *testing.T) {
	userCreatedAt := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	userUpdatedAt := userCreatedAt.Add(2 * time.Hour)
	userItem := agentGovernanceTestUserItem("agent", userCreatedAt, userUpdatedAt, map[string]types.AttributeValue{
		"agent_quarantine_status": &types.AttributeValueMemberS{Value: "quarantined"},
		"agent_quarantine_start":  &types.AttributeValueMemberS{Value: userCreatedAt.Format(time.RFC3339Nano)},
		"agent_quarantine_end":    &types.AttributeValueMemberS{Value: userUpdatedAt.Format(time.RFC3339Nano)},
		"agent_delegated_scopes": &types.AttributeValueMemberL{Value: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: "write"},
			&types.AttributeValueMemberS{Value: "read"},
		}},
		"agent_verified":    &types.AttributeValueMemberBOOL{Value: true},
		"agent_verified_at": &types.AttributeValueMemberS{Value: userUpdatedAt.Format(time.RFC3339Nano)},
		"other":             &types.AttributeValueMemberS{Value: "keep"},
	})

	t.Run("dry-run plans both writes", func(t *testing.T) {
		client := &fakeAgentGovernanceStateMigrationClient{
			queryOutputs: []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{userItem}}},
			getItems:     map[string]map[string]types.AttributeValue{},
		}

		summary, err := executeAgentGovernanceStateMigration(context.Background(), client, "test-table", false, 0)
		require.NoError(t, err)
		require.Equal(t, 1, summary.ScannedAgents)
		require.Equal(t, 1, summary.AgentsWithLegacyState)
		require.Equal(t, 1, summary.GovernanceRowsPlanned)
		require.Equal(t, 1, summary.UserRowsCleanupPlanned)
		require.Equal(t, 1, summary.ParityMismatches)
		require.Zero(t, summary.ValidationErrors)
		require.Empty(t, client.putItems)
	})

	t.Run("apply writes typed row and cleaned user row", func(t *testing.T) {
		client := &fakeAgentGovernanceStateMigrationClient{
			queryOutputs: []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{userItem}}},
			getItems:     map[string]map[string]types.AttributeValue{},
		}

		summary, err := executeAgentGovernanceStateMigration(context.Background(), client, "test-table", true, 0)
		require.NoError(t, err)
		require.Equal(t, 1, summary.GovernanceRowsUpserted)
		require.Equal(t, 1, summary.UserRowsUpdated)
		require.Len(t, client.putItems, 2)

		governanceItem := client.putItems[0]
		require.Equal(t, "USER#agent", agentGovernanceTestStringAttr(governanceItem["PK"]))
		require.Equal(t, models.SKAgentGovernance, agentGovernanceTestStringAttr(governanceItem["SK"]))
		require.Equal(t, "agent", agentGovernanceTestStringAttr(governanceItem["username"]))
		require.True(t, agentGovernanceTestBoolAttr(governanceItem["verified"]))
		require.Equal(t, []string{"read", "write"}, agentGovernanceTestListAttr(governanceItem["delegatedScopes"]))

		cleanedUser := client.putItems[1]
		metadata := cleanedUser["metadata"].(*types.AttributeValueMemberM).Value
		require.Contains(t, metadata, "other")
		require.NotContains(t, metadata, "agent_quarantine_status")
		require.NotContains(t, metadata, "agent_delegated_scopes")
		require.NotContains(t, metadata, "agent_verified")
	})
}

func TestExecuteAgentGovernanceStateMigration_IdempotentWhenTypedRowAlreadyMatches(t *testing.T) {
	createdAt := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	userItem := agentGovernanceTestUserItem("agent", createdAt, updatedAt, map[string]types.AttributeValue{
		"other": &types.AttributeValueMemberS{Value: "keep"},
	})
	existingGovernance := buildAgentGovernanceStateItem(&storage.AgentGovernanceState{
		Username:         "agent",
		QuarantineStatus: "approved",
		DelegatedScopes:  []string{"read"},
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	})

	client := &fakeAgentGovernanceStateMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{userItem}}},
		getItems: map[string]map[string]types.AttributeValue{
			"USER#agent#" + models.SKAgentGovernance: existingGovernance,
		},
	}

	summary, err := executeAgentGovernanceStateMigration(context.Background(), client, "test-table", true, 0)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedAgents)
	require.Equal(t, 1, summary.ExistingTypedRows)
	require.Zero(t, summary.GovernanceRowsPlanned)
	require.Zero(t, summary.UserRowsCleanupPlanned)
	require.Empty(t, client.putItems)
}

func TestRunMigrateAgentGovernanceState_PrintsSummary(t *testing.T) {
	prevLoadAWS := loadAWSConfigForCLIFn
	prevNewClient := newAgentGovernanceStateMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = prevLoadAWS
		newAgentGovernanceStateMigrationClientFn = prevNewClient
	})

	loadAWSConfigForCLIFn = func(_ context.Context, profile string) (aws.Config, string, error) {
		require.Equal(t, "Sim", profile)
		return aws.Config{Region: "us-east-1"}, "Sim", nil
	}

	client := &fakeAgentGovernanceStateMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{}},
	}
	newAgentGovernanceStateMigrationClientFn = func(cfg aws.Config) agentGovernanceStateMigrationClient {
		require.Equal(t, "us-east-1", cfg.Region)
		return client
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateAgentGovernanceState([]string{
			"--table", "lesser-main",
			"--aws-profile", "Sim",
			"--apply",
		}))
	})

	require.Contains(t, output, "migrate-agent-governance-state apply complete")
	require.Contains(t, output, "table: lesser-main")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "scanned_agents: 0")
	require.NotContains(t, output, "sample_usernames:")
}

func TestPrintAgentGovernanceStateMigrationSummary_DryRun(t *testing.T) {
	output := captureStdout(t, func() {
		printAgentGovernanceStateMigrationSummary(agentGovernanceStateMigrationSummary{
			ScannedAgents:          2,
			AgentsWithLegacyState:  1,
			ExistingTypedRows:      1,
			GovernanceRowsPlanned:  1,
			UserRowsCleanupPlanned: 1,
			ParityMatches:          1,
			ParityMismatches:       0,
			ValidationErrors:       0,
			SampleUsernames:        []string{"agent-a", "agent-b"},
		}, "lesser-main", "Sim", false)
	})

	require.Contains(t, output, "migrate-agent-governance-state dry-run complete")
	require.Contains(t, output, "sample_usernames:")
	require.Contains(t, output, "  agent-a")
	require.Contains(t, output, "no writes performed; re-run with --apply")
}

func TestExecuteAgentGovernanceStateMigration_ValidatesInputsAndErrors(t *testing.T) {
	_, err := executeAgentGovernanceStateMigration(context.Background(), nil, "table", false, 0)
	require.EqualError(t, err, "migration client is required")

	client := &fakeAgentGovernanceStateMigrationClient{}
	_, err = executeAgentGovernanceStateMigration(context.Background(), client, " ", false, 0)
	require.EqualError(t, err, "table name is required")

	client = &fakeAgentGovernanceStateMigrationClient{queryErr: errors.New("query failed")}
	_, err = executeAgentGovernanceStateMigration(context.Background(), client, "table", false, 0)
	require.EqualError(t, err, "query agent user rows: query failed")

	badUser := agentGovernanceTestUserItem("agent", time.Now().UTC(), time.Now().UTC(), map[string]types.AttributeValue{
		"agent_verified": &types.AttributeValueMemberS{Value: "definitely"},
	})
	client = &fakeAgentGovernanceStateMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{badUser}}},
	}

	summary, err := executeAgentGovernanceStateMigration(context.Background(), client, "table", false, 0)
	require.EqualError(t, err, "parse legacy governance row for agent: agent_verified is not a boolean")
	require.Equal(t, 1, summary.ScannedAgents)
	require.Equal(t, 1, summary.ValidationErrors)
	require.Equal(t, []string{"agent"}, summary.SampleUsernames)
}

func TestExecuteAgentGovernanceStateMigration_HonorsLimitAcrossPages(t *testing.T) {
	firstUser := agentGovernanceTestUserItem("agent-a", time.Now().UTC(), time.Now().UTC(), nil)
	secondUser := agentGovernanceTestUserItem("agent-b", time.Now().UTC(), time.Now().UTC(), nil)
	client := &fakeAgentGovernanceStateMigrationClient{
		queryOutputs: []*dynamodb.QueryOutput{
			{
				Items:            []map[string]types.AttributeValue{firstUser},
				LastEvaluatedKey: map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "USER#agent-a"}},
			},
			{
				Items: []map[string]types.AttributeValue{secondUser},
			},
		},
	}

	summary, err := executeAgentGovernanceStateMigration(context.Background(), client, "table", false, 1)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedAgents)
	require.Equal(t, 2, client.queryCalls)
}

func TestLoadExistingAgentGovernanceState_ErrorsAndMisses(t *testing.T) {
	client := &fakeAgentGovernanceStateMigrationClient{getErr: errors.New("read failed")}
	_, _, err := loadExistingAgentGovernanceState(context.Background(), client, "table", "agent")
	require.EqualError(t, err, "get typed governance row for agent: read failed")

	client = &fakeAgentGovernanceStateMigrationClient{
		getItems: map[string]map[string]types.AttributeValue{
			"USER#agent#" + models.SKAgentGovernance: {
				"PK":       &types.AttributeValueMemberS{Value: "USER#agent"},
				"SK":       &types.AttributeValueMemberS{Value: models.SKAgentGovernance},
				"username": &types.AttributeValueMemberS{Value: "agent"},
				"verified": &types.AttributeValueMemberBOOL{Value: true},
			},
		},
	}

	item, state, err := loadExistingAgentGovernanceState(context.Background(), client, "table", "agent")
	require.NoError(t, err)
	require.NotNil(t, item)
	require.NotNil(t, state)
	require.True(t, state.Verified)

	item, state, err = loadExistingAgentGovernanceState(context.Background(), &fakeAgentGovernanceStateMigrationClient{}, "table", "missing")
	require.NoError(t, err)
	require.Nil(t, item)
	require.Nil(t, state)
}

func TestProcessAgentGovernanceMigrationItem_TracksParityAndWriteErrors(t *testing.T) {
	now := time.Date(2026, 3, 27, 15, 0, 0, 0, time.UTC)
	userItem := agentGovernanceTestUserItem("agent", now, now, map[string]types.AttributeValue{
		"agent_verified":         &types.AttributeValueMemberBOOL{Value: true},
		"agent_delegated_scopes": &types.AttributeValueMemberS{Value: "write read"},
	})
	existingState := buildAgentGovernanceStateItem(&storage.AgentGovernanceState{
		Username:        "agent",
		Verified:        true,
		DelegatedScopes: []string{"read", "write"},
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	client := &fakeAgentGovernanceStateMigrationClient{
		getItems: map[string]map[string]types.AttributeValue{
			"USER#agent#" + models.SKAgentGovernance: existingState,
		},
	}
	summary := &agentGovernanceStateMigrationSummary{}
	require.NoError(t, processAgentGovernanceMigrationItem(context.Background(), client, "table", userItem, false, summary))
	require.Equal(t, 1, summary.ParityMatches)
	require.Zero(t, summary.GovernanceRowsPlanned)

	summary = &agentGovernanceStateMigrationSummary{}
	require.EqualError(t, processAgentGovernanceMigrationItem(context.Background(), client, "table", map[string]types.AttributeValue{}, false, summary), "validate agent governance row: missing username")
	require.Equal(t, 1, summary.ValidationErrors)

	client = &fakeAgentGovernanceStateMigrationClient{
		putErr: errors.New("write failed"),
	}
	err := processAgentGovernanceMigrationItem(context.Background(), client, "table", userItem, true, &agentGovernanceStateMigrationSummary{})
	require.EqualError(t, err, "put agent governance row for agent: write failed")

	client = &fakeAgentGovernanceStateMigrationClient{
		putErrs: []error{nil, errors.New("cleanup failed")},
	}
	err = processAgentGovernanceMigrationItem(context.Background(), client, "table", userItem, true, &agentGovernanceStateMigrationSummary{})
	require.EqualError(t, err, "update user metadata row for agent: cleanup failed")
}

func TestProcessAgentGovernanceMigrationItem_RerunAfterCleanupFailureKeepsTypedState(t *testing.T) {
	now := time.Date(2026, 3, 27, 16, 0, 0, 0, time.UTC)
	userItem := agentGovernanceTestUserItem("agent", now, now, map[string]types.AttributeValue{
		"agent_verified":         &types.AttributeValueMemberBOOL{Value: true},
		"agent_delegated_scopes": &types.AttributeValueMemberS{Value: "write read"},
	})
	client := &fakeAgentGovernanceStateMigrationClient{
		putErrs:  []error{nil, errors.New("cleanup failed")},
		getItems: map[string]map[string]types.AttributeValue{},
	}

	err := processAgentGovernanceMigrationItem(context.Background(), client, "table", userItem, true, &agentGovernanceStateMigrationSummary{})
	require.EqualError(t, err, "update user metadata row for agent: cleanup failed")
	require.Len(t, client.putItems, 1)

	newerTypedState := buildAgentGovernanceStateItem(&storage.AgentGovernanceState{
		Username:        "agent",
		Verified:        false,
		DelegatedScopes: []string{"read"},
		CreatedAt:       now,
		UpdatedAt:       now.Add(time.Hour),
	})
	client.getItems["USER#agent#"+models.SKAgentGovernance] = newerTypedState
	client.putItems = nil
	client.putErrs = nil

	summary := &agentGovernanceStateMigrationSummary{}
	require.NoError(t, processAgentGovernanceMigrationItem(context.Background(), client, "table", userItem, true, summary))
	require.Equal(t, 1, summary.ParityMismatches)
	require.Zero(t, summary.GovernanceRowsPlanned)
	require.Equal(t, 1, summary.UserRowsUpdated)
	require.Len(t, client.putItems, 1)
	require.Equal(t, models.SKMetadata, agentGovernanceTestStringAttr(client.putItems[0]["SK"]))
	require.Equal(t, []string{"read"}, agentGovernanceMigrationAttributeStringSliceValue(client.getItems["USER#agent#"+models.SKAgentGovernance]["delegatedScopes"]))
}

func TestDesiredAgentGovernanceState_MergesExistingLegacyAndUserTimestamps(t *testing.T) {
	userCreatedAt := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	userUpdatedAt := userCreatedAt.Add(45 * time.Minute)
	existingCreatedAt := userCreatedAt.Add(-2 * time.Hour)

	existing := &storage.AgentGovernanceState{
		Username:         "AGENT",
		DelegatedScopes:  []string{"existing"},
		CreatedAt:        existingCreatedAt,
		UpdatedAt:        time.Time{},
		QuarantineStatus: storage.AgentQuarantineStatusApproved,
	}
	legacy := &storage.AgentGovernanceState{
		QuarantineStatus: storage.AgentQuarantineStatusQuarantined,
		DelegatedScopes:  []string{"write", "read", "read"},
		SelfScopes:       []string{"publish", "read", "publish"},
		Verified:         true,
	}
	userItem := agentGovernanceTestUserItem("Agent", userCreatedAt, userUpdatedAt, nil)

	desired, ok := desiredAgentGovernanceState("Agent", userItem, existing, legacy, true)
	require.True(t, ok)
	require.Equal(t, "agent", desired.Username)
	require.Equal(t, storage.AgentQuarantineStatusApproved, desired.QuarantineStatus)
	require.Equal(t, []string{"existing"}, desired.DelegatedScopes)
	require.Nil(t, desired.SelfScopes)
	require.Equal(t, existingCreatedAt, desired.CreatedAt)
	require.Equal(t, userUpdatedAt, desired.UpdatedAt)

	seeded, seededOK := desiredAgentGovernanceState("Agent", userItem, nil, legacy, true)
	require.True(t, seededOK)
	require.Equal(t, storage.AgentQuarantineStatusQuarantined, seeded.QuarantineStatus)
	require.Equal(t, []string{"read", "write"}, seeded.DelegatedScopes)
	require.Equal(t, []string{"publish", "read"}, seeded.SelfScopes)
	require.Equal(t, userCreatedAt, seeded.CreatedAt)
	require.Equal(t, userUpdatedAt, seeded.UpdatedAt)

	none, hasState := desiredAgentGovernanceState("agent", userItem, nil, nil, false)
	require.False(t, hasState)
	require.Nil(t, none)
}

func TestAgentGovernanceStateItem_RoundTripsThroughTypedParser(t *testing.T) {
	now := time.Date(2026, 3, 27, 13, 30, 0, 0, time.UTC)
	state := &storage.AgentGovernanceState{
		Username:             "Agent",
		QuarantineStatus:     storage.AgentQuarantineStatusQuarantined,
		QuarantineStart:      ptrTime(now),
		QuarantineEnd:        ptrTime(now.Add(time.Hour)),
		QuarantineApprovedBy: "moderator",
		QuarantineApprovedAt: ptrTime(now.Add(2 * time.Hour)),
		DelegatedScopes:      []string{" write ", "read", "read"},
		SelfScopes:           []string{"publish", " publish "},
		SelfSovereign:        true,
		Verified:             true,
		VerifiedAt:           ptrTime(now.Add(3 * time.Hour)),
		VerifiedBy:           "staff",
		VerifiedReason:       "checked",
		UnverifiedAt:         ptrTime(now.Add(4 * time.Hour)),
		UnverifiedBy:         "staff-2",
		UnverifiedReason:     "rotate",
		KeyRotatedAt:         ptrTime(now.Add(5 * time.Hour)),
		CreatedAt:            now,
		UpdatedAt:            now.Add(6 * time.Hour),
	}

	item := buildAgentGovernanceStateItem(state)
	parsed, err := parseTypedAgentGovernanceState(item)
	require.NoError(t, err)
	require.Equal(t, "agent", parsed.Username)
	require.Equal(t, []string{"read", "write"}, parsed.DelegatedScopes)
	require.Equal(t, []string{"publish"}, parsed.SelfScopes)
	require.True(t, parsed.SelfSovereign)
	require.True(t, parsed.Verified)
	require.Equal(t, now.Add(6*time.Hour), parsed.UpdatedAt)
}

func TestExtractLegacyAgentGovernanceState_ParsesAndValidatesLegacyMetadata(t *testing.T) {
	now := time.Date(2026, 3, 27, 14, 0, 0, 0, time.UTC)
	item := map[string]types.AttributeValue{
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"agent_quarantine_status":      &types.AttributeValueMemberS{Value: " quarantined "},
			"agent_quarantine_start":       &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)},
			"agent_quarantine_end":         &types.AttributeValueMemberS{Value: now.Add(time.Hour).Format(time.RFC3339Nano)},
			"agent_quarantine_approved_by": &types.AttributeValueMemberS{Value: " mod "},
			"agent_quarantine_approved_at": &types.AttributeValueMemberS{Value: now.Add(2 * time.Hour).Format(time.RFC3339Nano)},
			"agent_delegated_scopes":       &types.AttributeValueMemberSS{Value: []string{"write", "read", "write"}},
			"agent_self_scopes":            &types.AttributeValueMemberS{Value: "publish read publish"},
			"agent_self_sovereign":         &types.AttributeValueMemberS{Value: "true"},
			"agent_verified":               &types.AttributeValueMemberBOOL{Value: true},
			"agent_verified_at":            &types.AttributeValueMemberS{Value: now.Add(3 * time.Hour).Format(time.RFC3339Nano)},
			"agent_verified_by":            &types.AttributeValueMemberS{Value: " ops "},
			"agent_verified_reason":        &types.AttributeValueMemberS{Value: " approved "},
			"agent_unverified_at":          &types.AttributeValueMemberS{Value: now.Add(4 * time.Hour).Format(time.RFC3339Nano)},
			"agent_unverified_by":          &types.AttributeValueMemberS{Value: "ops2"},
			"agent_unverified_reason":      &types.AttributeValueMemberS{Value: "rotate"},
			"agent_key_rotated_at":         &types.AttributeValueMemberS{Value: now.Add(5 * time.Hour).Format(time.RFC3339Nano)},
		}},
	}

	state, ok, err := extractLegacyAgentGovernanceState(item)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, storage.AgentQuarantineStatusQuarantined, state.QuarantineStatus)
	require.Equal(t, []string{"read", "write"}, state.DelegatedScopes)
	require.Equal(t, []string{"publish", "read"}, state.SelfScopes)
	require.True(t, state.SelfSovereign)
	require.True(t, state.Verified)
	require.Equal(t, "mod", state.QuarantineApprovedBy)
	require.Equal(t, "approved", state.VerifiedReason)

	_, ok, err = extractLegacyAgentGovernanceState(map[string]types.AttributeValue{
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"other": &types.AttributeValueMemberS{Value: "keep"},
		}},
	})
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = extractLegacyAgentGovernanceState(map[string]types.AttributeValue{
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"agent_verified":         &types.AttributeValueMemberS{Value: "maybe"},
			"agent_delegated_scopes": &types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberBOOL{Value: true}}},
		}},
	})
	require.True(t, ok)
	require.EqualError(t, err, "agent_delegated_scopes contains a non-string scope; agent_verified is not a boolean")
}

func TestStripLegacyAgentGovernanceMetadata_RemovesLegacyOnly(t *testing.T) {
	item := map[string]types.AttributeValue{
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"agent_verified":    &types.AttributeValueMemberBOOL{Value: true},
			"agent_self_scopes": &types.AttributeValueMemberS{Value: "read"},
			"other":             &types.AttributeValueMemberS{Value: "keep"},
		}},
	}
	cleaned, removed := stripLegacyAgentGovernanceMetadata(item)
	require.True(t, removed)
	require.NotSame(t, item["metadata"], cleaned["metadata"])
	metadata := cleaned["metadata"].(*types.AttributeValueMemberM).Value
	require.Equal(t, "keep", agentGovernanceTestStringAttr(metadata["other"]))
	require.NotContains(t, metadata, "agent_verified")

	removedAll, removed := stripLegacyAgentGovernanceMetadata(map[string]types.AttributeValue{
		"metadata": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"agent_verified": &types.AttributeValueMemberBOOL{Value: true},
		}},
	})
	require.True(t, removed)
	require.NotContains(t, removedAll, "metadata")
}

func TestAgentGovernanceMigrationHelpers_HandleVariantsAndErrors(t *testing.T) {
	summary := &agentGovernanceStateMigrationSummary{}
	for i := range 12 {
		recordAgentGovernanceMigrationSample(summary, fmt.Sprintf("agent-%d", i))
	}
	recordAgentGovernanceMigrationSample(summary, "agent-1")
	require.Len(t, summary.SampleUsernames, 10)
	require.Equal(t, "agent-0", summary.SampleUsernames[0])
	require.Equal(t, "agent-9", summary.SampleUsernames[9])

	require.Equal(t, "agent", agentGovernanceMigrationUsername(map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#Agent"},
	}))
	require.Equal(t, "", agentGovernanceMigrationUsername(map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "OTHER#agent"},
	}))

	item := map[string]types.AttributeValue{}
	setAgentGovernanceMigrationStringListAttribute(item, "delegatedScopes", []string{" read ", "", "write"})
	require.Equal(t, []string{"read", "write"}, agentGovernanceTestListAttr(item["delegatedScopes"]))

	value, err := agentGovernanceMigrationLegacyBool(&types.AttributeValueMemberS{Value: "false"}, "verified")
	require.NoError(t, err)
	require.False(t, value)
	_, err = agentGovernanceMigrationLegacyBool(&types.AttributeValueMemberN{Value: "1"}, "verified")
	require.EqualError(t, err, "verified is not a boolean")

	parsedTime, err := agentGovernanceMigrationLegacyTime(&types.AttributeValueMemberS{Value: "2026-03-27T12:00:00Z"}, "verified_at")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC), *parsedTime)
	_, err = agentGovernanceMigrationLegacyTime(&types.AttributeValueMemberBOOL{Value: true}, "verified_at")
	require.EqualError(t, err, "verified_at is not a timestamp")
	_, err = agentGovernanceMigrationLegacyTime(&types.AttributeValueMemberS{Value: "nope"}, "verified_at")
	require.EqualError(t, err, "verified_at is not a valid timestamp")

	scopes, err := agentGovernanceMigrationLegacyStringSlice(&types.AttributeValueMemberS{Value: "write read write"}, "delegated")
	require.NoError(t, err)
	require.Equal(t, []string{"read", "write"}, scopes)
	_, err = agentGovernanceMigrationLegacyStringSlice(&types.AttributeValueMemberBOOL{Value: true}, "delegated")
	require.EqualError(t, err, "delegated is not a string list")
}

func TestAgentGovernanceMigrationHelpers_ParseFallbacksAndNoops(t *testing.T) {
	recordAgentGovernanceMigrationSample(nil, "agent")
	summary := &agentGovernanceStateMigrationSummary{}
	recordAgentGovernanceMigrationSample(summary, " ")
	require.Empty(t, summary.SampleUsernames)

	parsed, err := parseTypedAgentGovernanceState(map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#Agent"},
	})
	require.NoError(t, err)
	require.Equal(t, "agent", parsed.Username)

	cloned := cloneAttributeValueMap(nil)
	require.Empty(t, cloned)

	item := map[string]types.AttributeValue{}
	setAgentGovernanceMigrationTimeAttribute(item, "verifiedAt", nil)
	require.Empty(t, item)

	state, ok, err := extractLegacyAgentGovernanceState(map[string]types.AttributeValue{})
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, state)

	state, ok, err = extractLegacyAgentGovernanceState(map[string]types.AttributeValue{
		"metadata": &types.AttributeValueMemberS{Value: "not-a-map"},
	})
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, state)
}

func agentGovernanceTestUserItem(username string, createdAt, updatedAt time.Time, metadata map[string]types.AttributeValue) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "USER#" + username},
		"SK":        &types.AttributeValueMemberS{Value: models.SKMetadata},
		"username":  &types.AttributeValueMemberS{Value: username},
		"createdAt": &types.AttributeValueMemberS{Value: createdAt.UTC().Format(time.RFC3339Nano)},
		"updatedAt": &types.AttributeValueMemberS{Value: updatedAt.UTC().Format(time.RFC3339Nano)},
	}
	if len(metadata) > 0 {
		item["metadata"] = &types.AttributeValueMemberM{Value: metadata}
	}
	return item
}

func agentGovernanceTestStringAttr(value types.AttributeValue) string {
	attr, ok := value.(*types.AttributeValueMemberS)
	if !ok || attr == nil {
		return ""
	}
	return attr.Value
}

func agentGovernanceTestBoolAttr(value types.AttributeValue) bool {
	attr, ok := value.(*types.AttributeValueMemberBOOL)
	if !ok || attr == nil {
		return false
	}
	return attr.Value
}

func agentGovernanceTestListAttr(value types.AttributeValue) []string {
	attr, ok := value.(*types.AttributeValueMemberL)
	if !ok || attr == nil {
		return nil
	}
	out := make([]string, 0, len(attr.Value))
	for _, entry := range attr.Value {
		out = append(out, agentGovernanceTestStringAttr(entry))
	}
	return out
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
