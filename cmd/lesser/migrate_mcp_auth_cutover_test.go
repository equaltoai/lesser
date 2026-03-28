package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

type stagedMCPAuthCutoverMigrationClient struct {
	scanOutputs []*dynamodb.ScanOutput
	scanErr     error
	failScanAt  int
	scanCalls   int
}

func (s *stagedMCPAuthCutoverMigrationClient) Scan(_ context.Context, _ *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	if s.failScanAt > 0 && s.scanCalls == s.failScanAt {
		return nil, s.scanErr
	}
	if len(s.scanOutputs) == 0 {
		return &dynamodb.ScanOutput{}, nil
	}
	out := s.scanOutputs[0]
	s.scanOutputs = s.scanOutputs[1:]
	return out, nil
}

func (s *stagedMCPAuthCutoverMigrationClient) DeleteItem(_ context.Context, _ *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return &dynamodb.DeleteItemOutput{}, nil
}

func TestRunMigrateMCPAuthCutover_PrintsDryRunSummary(t *testing.T) {
	previousLoadAWSConfig := loadAWSConfigForCLIFn
	previousClientFactory := newMCPAuthCutoverMigrationClientFn
	t.Cleanup(func() {
		loadAWSConfigForCLIFn = previousLoadAWSConfig
		newMCPAuthCutoverMigrationClientFn = previousClientFactory
	})

	client := &fakeUserKeyMigrationClient{
		scanOutputs: mcpAuthCutoverScanOutputs(t),
	}

	var seenProfile string
	loadAWSConfigForCLIFn = func(_ context.Context, awsProfile string) (aws.Config, string, error) {
		seenProfile = awsProfile
		return aws.Config{}, "Sim", nil
	}
	newMCPAuthCutoverMigrationClientFn = func(aws.Config) mcpAuthCutoverMigrationClient {
		return client
	}

	output := captureStdout(t, func() {
		require.NoError(t, runMigrateMCPAuthCutover([]string{
			"--app", "simulacrum",
			"--env", "dev",
			"--aws-profile", "Sim",
		}))
	})

	require.Equal(t, "Sim", seenProfile)
	require.Contains(t, output, "migrate-mcp-auth-cutover dry-run complete")
	require.Contains(t, output, "table: simulacrum-dev-main-table")
	require.Contains(t, output, "aws_profile: Sim")
	require.Contains(t, output, "oauth_clients_scanned: 2")
	require.Contains(t, output, "connector_clients_matched: 1")
	require.Contains(t, output, "connector_clients_deleted: 0")
	require.Contains(t, output, "refresh_tokens_scanned: 2")
	require.Contains(t, output, "connector_refresh_tokens_matched: 1")
	require.Contains(t, output, "connector_refresh_tokens_deleted: 0")
	require.Contains(t, output, "runtime_refresh_tokens_preserved: 1")
	require.Contains(t, output, "authorization_codes_scanned: 1")
	require.Contains(t, output, "connector_authorization_codes_matched: 1")
	require.Contains(t, output, "oauth_states_scanned: 1")
	require.Contains(t, output, "connector_oauth_states_matched: 1")
	require.Contains(t, output, "device_sessions_scanned: 1")
	require.Contains(t, output, "connector_device_sessions_matched: 1")
	require.Contains(t, output, "user_app_consents_scanned: 1")
	require.Contains(t, output, "connector_user_app_consents_matched: 1")
	require.Contains(t, output, "sample_connector_client_ids:")
	require.Contains(t, output, "client-agent")
	require.Contains(t, output, "sample_connector_refresh_sessions:")
	require.Contains(t, output, "resource=https://example.com/mcp/agent1")
	require.Contains(t, output, "sample_runtime_refresh_sessions:")
	require.Contains(t, output, auth.DelegatedAgentRuntimeClientID)
	require.Contains(t, output, "no writes performed; re-run with --apply")
	require.Empty(t, client.deleteInputs)
}

func TestExecuteMCPAuthCutoverMigration_ApplyDeletesConnectorArtifacts(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: mcpAuthCutoverScanOutputs(t),
	}

	summary, err := executeMCPAuthCutoverMigration(context.Background(), client, "simulacrum-dev-main-table", true, 0)
	require.NoError(t, err)

	require.Equal(t, 1, summary.ConnectorClientsDeleted)
	require.Equal(t, 1, summary.ConnectorRefreshTokensDeleted)
	require.Equal(t, 1, summary.ConnectorAuthorizationCodesDeleted)
	require.Equal(t, 1, summary.ConnectorOAuthStatesDeleted)
	require.Equal(t, 1, summary.ConnectorDeviceSessionsDeleted)
	require.Equal(t, 1, summary.ConnectorUserAppConsentsDeleted)
	require.Len(t, client.deleteInputs, 6)

	deletedKeys := make([]string, 0, len(client.deleteInputs))
	for _, input := range client.deleteInputs {
		deletedKeys = append(deletedKeys, strAttr(t, input.Key["PK"])+"\x00"+strAttr(t, input.Key["SK"]))
	}

	require.Contains(t, deletedKeys, "AUTHCODE#code-agent\x00CODE")
	require.Contains(t, deletedKeys, "OAUTH_STATE#state-agent\x00STATE")
	require.Contains(t, deletedKeys, "OAUTH_DEVICE#hash-agent\x00SESSION")
	require.Contains(t, deletedKeys, "USER#owner\x00CONSENT#client-agent#RESOURCE#https://example.com/mcp/agent1")
	require.Contains(t, deletedKeys, "REFRESHTOKEN#rt-agent\x00TOKEN")
	require.Contains(t, deletedKeys, "OAUTH_CLIENT#client-agent\x00CLIENT")
}

func TestExecuteMCPAuthCutoverMigration_ValidatesInputs(t *testing.T) {
	_, err := executeMCPAuthCutoverMigration(context.Background(), nil, "simulacrum-dev-main-table", false, 0)
	require.ErrorContains(t, err, "migration client is required")

	client := &fakeUserKeyMigrationClient{}
	_, err = executeMCPAuthCutoverMigration(context.Background(), client, "", false, 0)
	require.ErrorContains(t, err, "table name is required")
}

func TestExecuteMCPAuthCutoverMigration_PropagatesStageErrors(t *testing.T) {
	t.Run("oauth client scan failure", func(t *testing.T) {
		client := &stagedMCPAuthCutoverMigrationClient{
			failScanAt: 1,
			scanErr:    context.DeadlineExceeded,
		}
		_, err := executeMCPAuthCutoverMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
		require.ErrorContains(t, err, "scan oauth clients")
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("refresh token scan failure", func(t *testing.T) {
		client := &stagedMCPAuthCutoverMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{{}},
			failScanAt:  2,
			scanErr:     context.DeadlineExceeded,
		}
		_, err := executeMCPAuthCutoverMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
		require.ErrorContains(t, err, "scan refresh tokens")
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("malformed oauth client row", func(t *testing.T) {
		client := &stagedMCPAuthCutoverMigrationClient{
			scanOutputs: []*dynamodb.ScanOutput{{
				Items: []map[string]types.AttributeValue{{
					"clientID": &types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberS{Value: "bad"}}},
				}},
			}},
		}
		_, err := executeMCPAuthCutoverMigration(context.Background(), client, "simulacrum-dev-main-table", false, 0)
		require.ErrorContains(t, err, "unmarshal oauth client")
	})
}

func TestMCPAuthCutoverHelpers(t *testing.T) {
	t.Run("connector client classification excludes runtime ids and blanks", func(t *testing.T) {
		require.False(t, isConnectorOAuthClient(nil))
		require.False(t, isConnectorOAuthClient(&models.OAuthClient{}))
		require.False(t, isConnectorOAuthClient(&models.OAuthClient{ClientID: auth.DelegatedAgentRuntimeClientID, ClientClass: auth.ClientClassAgent}))
		require.True(t, isConnectorOAuthClient(&models.OAuthClient{ClientID: "client-agent", ClientClass: auth.ClientClassAgent}))
		require.True(t, isConnectorOAuthClient(&models.OAuthClient{ClientID: "client-legacy", AgentUsername: "agent1"}))
	})

	t.Run("refresh token classification preserves runtime and revokes connector state", func(t *testing.T) {
		connectorClientIDs := map[string]struct{}{"client-agent": {}}

		require.False(t, isPreservedRuntimeRefreshToken(nil))
		require.True(t, isPreservedRuntimeRefreshToken(&models.RefreshToken{ClientID: auth.DelegatedAgentRuntimeClientID}))
		require.False(t, isPreservedRuntimeRefreshToken(&models.RefreshToken{ClientID: "client-cli"}))

		require.False(t, isConnectorRefreshToken(nil, connectorClientIDs))
		require.False(t, isConnectorRefreshToken(&models.RefreshToken{}, connectorClientIDs))
		require.False(t, isConnectorRefreshToken(&models.RefreshToken{ClientID: auth.DelegatedAgentRuntimeClientID, ClientClass: auth.ClientClassAgent}, connectorClientIDs))
		require.True(t, isConnectorRefreshToken(&models.RefreshToken{ClientID: "client-agent"}, connectorClientIDs))
		require.True(t, isConnectorRefreshToken(&models.RefreshToken{ClientID: "client-legacy", ClientClass: auth.ClientClassAgent}, connectorClientIDs))
		require.False(t, isConnectorRefreshToken(&models.RefreshToken{ClientID: "client-cli", ClientClass: auth.ClientClassCLI}, connectorClientIDs))
	})

	t.Run("artifact classifiers key off connector client ids or legacy agent usernames", func(t *testing.T) {
		connectorClientIDs := map[string]struct{}{"client-agent": {}}

		require.False(t, isConnectorAuthorizationCode(nil, connectorClientIDs))
		require.True(t, isConnectorAuthorizationCode(&models.AuthorizationCode{ClientID: "client-agent"}, connectorClientIDs))
		require.False(t, isConnectorAuthorizationCode(&models.AuthorizationCode{ClientID: "client-cli"}, connectorClientIDs))

		require.False(t, isConnectorOAuthState(nil, connectorClientIDs))
		require.True(t, isConnectorOAuthState(&models.OAuthState{AgentUsername: "agent1"}, connectorClientIDs))
		require.True(t, isConnectorOAuthState(&models.OAuthState{ClientID: "client-agent"}, connectorClientIDs))
		require.False(t, isConnectorOAuthState(&models.OAuthState{ClientID: "client-cli"}, connectorClientIDs))

		require.False(t, isConnectorDeviceSession(nil, connectorClientIDs))
		require.True(t, isConnectorDeviceSession(&models.OAuthDeviceSession{ClientID: "client-agent"}, connectorClientIDs))
		require.False(t, isConnectorDeviceSession(&models.OAuthDeviceSession{ClientID: "client-cli"}, connectorClientIDs))

		require.False(t, isConnectorUserConsent(nil, connectorClientIDs))
		require.True(t, isConnectorUserConsent(&models.UserAppConsent{AppID: "client-agent"}, connectorClientIDs))
		require.False(t, isConnectorUserConsent(&models.UserAppConsent{AppID: "client-cli"}, connectorClientIDs))
	})

	t.Run("summaries prefer resource then session fallback", func(t *testing.T) {
		require.Empty(t, summarizeMCPAuthCutoverRefreshToken(nil))
		require.Equal(t,
			"client-agent user=agent1 resource=https://example.com/mcp/agent1",
			summarizeMCPAuthCutoverRefreshToken(&models.RefreshToken{
				ClientID: "client-agent",
				Username: "agent1",
				Resource: "https://example.com/mcp/agent1",
			}),
		)
		require.Equal(t,
			"client-agent user=agent1 session=session-1",
			summarizeMCPAuthCutoverRefreshToken(&models.RefreshToken{
				ClientID:  "client-agent",
				Username:  "agent1",
				SessionID: "session-1",
			}),
		)
		require.Equal(t,
			"client-agent user=agent1",
			summarizeMCPAuthCutoverRefreshToken(&models.RefreshToken{
				ClientID: "client-agent",
				Username: "agent1",
			}),
		)
		require.Equal(t,
			"client-agent user=agent1 resource=https://example.com/mcp/agent1",
			summarizeMCPAuthCutoverClientArtifact(" client-agent ", " agent1 ", " https://example.com/mcp/agent1 "),
		)
	})

	t.Run("sample helpers deduplicate and respect limits", func(t *testing.T) {
		require.Nil(t, sampleStrings(nil, 5))
		require.Equal(t, []string{"one", "two"}, sampleStrings([]string{"one", " ", "one", "two", "two"}, 0))
		require.Equal(t, []string{"one"}, sampleStrings([]string{"one", "two"}, 1))
		require.Equal(t, 1, minInt(1, 2))
		require.Equal(t, 2, minInt(3, 2))
	})
}

func TestScanMCPAuthCutoverItems_PaginatesAndHandlesErrors(t *testing.T) {
	client := &fakeUserKeyMigrationClient{
		scanOutputs: []*dynamodb.ScanOutput{
			{
				Items: []map[string]types.AttributeValue{
					{"PK": sAttr("first"), "SK": sAttr("row")},
				},
				LastEvaluatedKey: map[string]types.AttributeValue{
					"PK": sAttr("cursor-pk"),
					"SK": sAttr("cursor-sk"),
				},
			},
			{
				Items: []map[string]types.AttributeValue{
					{"PK": sAttr("second"), "SK": sAttr("row")},
				},
			},
		},
	}

	items, err := scanMCPAuthCutoverItems(
		context.Background(),
		client,
		"simulacrum-dev-main-table",
		"begins_with(PK, :pk)",
		map[string]types.AttributeValue{":pk": sAttr("OAUTH_CLIENT#")},
		map[string]string{"#pk": "PK"},
	)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Len(t, client.scanInputs, 2)
	require.Equal(t, "PK", client.scanInputs[0].ExpressionAttributeNames["#pk"])
	require.Equal(t, "cursor-pk", strAttr(t, client.scanInputs[1].ExclusiveStartKey["PK"]))

	client.scanErr = context.DeadlineExceeded
	client.scanOutputs = nil
	_, err = scanMCPAuthCutoverItems(
		context.Background(),
		client,
		"simulacrum-dev-main-table",
		"begins_with(PK, :pk)",
		map[string]types.AttributeValue{":pk": sAttr("OAUTH_CLIENT#")},
		nil,
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMCPAuthCutoverBuilders_HandleLimitsAndMalformedItems(t *testing.T) {
	baseTime := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	connectorA := models.OAuthClient{
		ClientID:      "client-a",
		ClientClass:   auth.ClientClassAgent,
		AgentUsername: "agent-a",
		CreatedAt:     baseTime,
		UpdatedAt:     baseTime,
	}
	require.NoError(t, connectorA.BeforeCreate())

	connectorB := models.OAuthClient{
		ClientID:      "client-b",
		ClientClass:   auth.ClientClassAgent,
		AgentUsername: "agent-b",
		CreatedAt:     baseTime,
		UpdatedAt:     baseTime,
	}
	require.NoError(t, connectorB.BeforeCreate())

	publicClient := models.OAuthClient{
		ClientID:    "client-cli",
		ClientClass: auth.ClientClassCLI,
		CreatedAt:   baseTime,
		UpdatedAt:   baseTime,
	}
	require.NoError(t, publicClient.BeforeCreate())

	summary := &mcpAuthCutoverMigrationSummary{}
	candidates, clientIDs, err := buildMCPAuthCutoverOAuthClientCandidates([]map[string]types.AttributeValue{
		mustMarshalMCPAuthCutoverModel(t, connectorB),
		mustMarshalMCPAuthCutoverModel(t, publicClient),
		mustMarshalMCPAuthCutoverModel(t, connectorA),
	}, 1, summary)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "client-a", candidates[0].ClientID)
	require.Equal(t, 1, summary.ConnectorClientsMatched)
	_, ok := clientIDs["client-a"]
	require.True(t, ok)

	badItem := []map[string]types.AttributeValue{{
		"clientID": &types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberS{Value: "bad"}}},
	}}
	_, _, err = buildMCPAuthCutoverOAuthClientCandidates(badItem, 0, &mcpAuthCutoverMigrationSummary{})
	require.ErrorContains(t, err, "unmarshal oauth client")
	_, _, err = buildMCPAuthCutoverRefreshCandidates(badItem, map[string]struct{}{}, &mcpAuthCutoverMigrationSummary{})
	require.ErrorContains(t, err, "unmarshal refresh token")
	_, err = buildMCPAuthCutoverAuthorizationCodeCandidates(badItem, map[string]struct{}{}, &mcpAuthCutoverMigrationSummary{})
	require.ErrorContains(t, err, "unmarshal authorization code")
	_, err = buildMCPAuthCutoverOAuthStateCandidates(badItem, map[string]struct{}{}, &mcpAuthCutoverMigrationSummary{})
	require.ErrorContains(t, err, "unmarshal oauth state")
	_, err = buildMCPAuthCutoverDeviceSessionCandidates(badItem, map[string]struct{}{}, &mcpAuthCutoverMigrationSummary{})
	require.ErrorContains(t, err, "unmarshal oauth device session")

	badConsentItem := []map[string]types.AttributeValue{{
		"appID": &types.AttributeValueMemberL{Value: []types.AttributeValue{&types.AttributeValueMemberS{Value: "bad"}}},
	}}
	_, err = buildMCPAuthCutoverUserConsentCandidates(badConsentItem, map[string]struct{}{}, &mcpAuthCutoverMigrationSummary{})
	require.ErrorContains(t, err, "unmarshal user app consent")
}

func TestMCPAuthCutoverArtifactBuilders_SkipNonConnectorRows(t *testing.T) {
	baseTime := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)
	summary := &mcpAuthCutoverMigrationSummary{}
	connectorClientIDs := map[string]struct{}{"client-agent": {}}

	publicCode := models.AuthorizationCode{
		Code:        "code-cli",
		ClientID:    "client-cli",
		RedirectURI: "https://example.com/callback",
		Username:    "owner",
		ExpiresAt:   baseTime.Add(10 * time.Minute),
		CreatedAt:   baseTime,
	}
	require.NoError(t, publicCode.BeforeCreate())
	candidates, err := buildMCPAuthCutoverAuthorizationCodeCandidates(
		[]map[string]types.AttributeValue{mustMarshalMCPAuthCutoverModel(t, publicCode)},
		connectorClientIDs,
		summary,
	)
	require.NoError(t, err)
	require.Empty(t, candidates)
	require.Equal(t, 0, summary.ConnectorAuthorizationCodesMatched)
}

func TestDeleteMCPAuthCutoverCandidates_ReportsDeleteErrors(t *testing.T) {
	client := &fakeUserKeyMigrationClient{deleteErr: errors.New("boom")}
	deleted := 0

	err := deleteMCPAuthCutoverCandidates(context.Background(), client, "simulacrum-dev-main-table", true, []mcpAuthCutoverDeleteCandidate{{
		PK: "OAUTH_CLIENT#client-agent",
		SK: "CLIENT",
	}}, &deleted)
	require.ErrorContains(t, err, "delete OAUTH_CLIENT#client-agent CLIENT")
	require.ErrorContains(t, err, "boom")
	require.Zero(t, deleted)
}

func TestDeleteMCPAuthCutoverCandidates_ApplyIncrementsDeleted(t *testing.T) {
	client := &fakeUserKeyMigrationClient{}
	deleted := 0

	err := deleteMCPAuthCutoverCandidates(context.Background(), client, "simulacrum-dev-main-table", true, []mcpAuthCutoverDeleteCandidate{
		{PK: "OAUTH_CLIENT#client-a", SK: "CLIENT"},
		{PK: "REFRESHTOKEN#token-a", SK: "TOKEN"},
	}, &deleted)
	require.NoError(t, err)
	require.Equal(t, 2, deleted)
	require.Len(t, client.deleteInputs, 2)
	require.Equal(t, "OAUTH_CLIENT#client-a", strAttr(t, client.deleteInputs[0].Key["PK"]))
	require.Equal(t, "TOKEN", strAttr(t, client.deleteInputs[1].Key["SK"]))
}

func mcpAuthCutoverScanOutputs(t *testing.T) []*dynamodb.ScanOutput {
	t.Helper()

	baseTime := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	connectorClient := models.OAuthClient{
		ClientID:      "client-agent",
		ClientClass:   auth.ClientClassAgent,
		AgentUsername: "agent1",
		GrantTypes:    []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
		Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
		CreatedAt:     baseTime,
		UpdatedAt:     baseTime,
	}
	require.NoError(t, connectorClient.BeforeCreate())

	publicClient := models.OAuthClient{
		ClientID:    "client-cli",
		ClientClass: auth.ClientClassCLI,
		GrantTypes:  []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, "urn:ietf:params:oauth:grant-type:device_code"},
		Scopes:      []string{auth.ScopeRead},
		CreatedAt:   baseTime,
		UpdatedAt:   baseTime,
	}
	require.NoError(t, publicClient.BeforeCreate())

	connectorRefresh := models.RefreshToken{
		Token:       "rt-agent",
		ClientID:    connectorClient.ClientID,
		Username:    "agent1",
		Resource:    "https://example.com/mcp/agent1",
		ExpiresAt:   baseTime.Add(24 * time.Hour),
		Scopes:      []string{auth.ScopeRead},
		ClientClass: auth.ClientClassAgent,
		CreatedAt:   baseTime,
	}
	require.NoError(t, connectorRefresh.BeforeCreate())

	runtimeRefresh := models.RefreshToken{
		Token:             "rt-runtime",
		ClientID:          auth.DelegatedAgentRuntimeClientID,
		Username:          "agent1",
		ExpiresAt:         baseTime.Add(24 * time.Hour),
		Scopes:            []string{auth.ScopeRead},
		ClientClass:       auth.ClientClassAgent,
		SessionID:         "session-runtime",
		FamilyID:          "family-runtime",
		Generation:        1,
		Current:           true,
		DeviceLabel:       "local-agent",
		LastUsedAt:        baseTime,
		IdleExpiresAt:     baseTime.Add(24 * time.Hour),
		AbsoluteExpiresAt: baseTime.Add(30 * 24 * time.Hour),
		SessionCreatedAt:  baseTime.Add(-1 * time.Hour),
		AccessTTLSeconds:  3600,
		CreatedAt:         baseTime,
	}
	require.NoError(t, runtimeRefresh.BeforeCreate())

	authCode := models.AuthorizationCode{
		Code:        "code-agent",
		ClientID:    connectorClient.ClientID,
		RedirectURI: "https://example.com/callback",
		Resource:    "https://example.com/mcp/agent1",
		Username:    "owner",
		ExpiresAt:   baseTime.Add(10 * time.Minute),
		Scopes:      []string{auth.ScopeRead},
		CreatedAt:   baseTime,
	}
	require.NoError(t, authCode.BeforeCreate())

	oauthState := models.OAuthState{
		State:         "state-agent",
		RedirectURI:   "https://example.com/callback",
		Username:      "owner",
		AgentUsername: "agent1",
		ClientID:      connectorClient.ClientID,
		Resource:      "https://example.com/mcp/agent1",
		CreatedAt:     baseTime,
		ExpiresAt:     baseTime.Add(10 * time.Minute),
	}
	require.NoError(t, oauthState.UpdateKeys())

	deviceSession := models.OAuthDeviceSession{
		DeviceCodeHash:   "hash-agent",
		UserCode:         "ABCD-EFGH",
		ClientID:         connectorClient.ClientID,
		Status:           "approved",
		ApprovedUsername: "owner",
		CreatedAt:        baseTime,
		UpdatedAt:        baseTime,
		ExpiresAt:        baseTime.Add(10 * time.Minute),
	}
	require.NoError(t, deviceSession.UpdateKeys())

	consent := models.UserAppConsent{
		UserID:    "owner",
		AppID:     connectorClient.ClientID,
		Resource:  "https://example.com/mcp/agent1",
		Scopes:    []string{auth.ScopeRead},
		CreatedAt: baseTime,
		UpdatedAt: baseTime,
		Active:    true,
	}
	require.NoError(t, consent.UpdateKeys())

	return []*dynamodb.ScanOutput{
		{Items: []map[string]types.AttributeValue{
			mustMarshalMCPAuthCutoverModel(t, connectorClient),
			mustMarshalMCPAuthCutoverModel(t, publicClient),
		}},
		{Items: []map[string]types.AttributeValue{
			mustMarshalMCPAuthCutoverModel(t, connectorRefresh),
			mustMarshalMCPAuthCutoverModel(t, runtimeRefresh),
		}},
		{Items: []map[string]types.AttributeValue{
			mustMarshalMCPAuthCutoverModel(t, authCode),
		}},
		{Items: []map[string]types.AttributeValue{
			mustMarshalMCPAuthCutoverModel(t, oauthState),
		}},
		{Items: []map[string]types.AttributeValue{
			mustMarshalMCPAuthCutoverModel(t, deviceSession),
		}},
		{Items: []map[string]types.AttributeValue{
			mustMarshalMCPAuthCutoverModel(t, consent),
		}},
	}
}

func mustMarshalMCPAuthCutoverModel(t *testing.T, model any) map[string]types.AttributeValue {
	t.Helper()

	item, err := attributevalue.MarshalMap(model)
	require.NoError(t, err)
	return item
}
