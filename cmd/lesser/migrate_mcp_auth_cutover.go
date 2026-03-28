package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

const (
	mcpAuthCutoverOAuthClientPKPrefix  = "OAUTH_CLIENT#"
	mcpAuthCutoverOAuthClientSK        = "CLIENT"
	mcpAuthCutoverRefreshTokenPKPrefix = "REFRESHTOKEN#"
	mcpAuthCutoverRefreshTokenSK       = "TOKEN"
	mcpAuthCutoverAuthorizationCodePK  = "AUTHCODE#"
	mcpAuthCutoverAuthorizationCodeSK  = "CODE"
	mcpAuthCutoverOAuthStatePKPrefix   = "OAUTH_STATE#"
	mcpAuthCutoverOAuthStateSK         = "STATE"
	mcpAuthCutoverOAuthDevicePKPrefix  = "OAUTH_DEVICE#"
	mcpAuthCutoverOAuthDeviceSK        = "SESSION"
	mcpAuthCutoverUserConsentPKPrefix  = "USER#"
	mcpAuthCutoverUserConsentSKPrefix  = "CONSENT#"
)

type mcpAuthCutoverMigrationClient interface {
	Scan(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

type mcpAuthCutoverMigrationSummary struct {
	OAuthClientsScanned                int
	ConnectorClientsMatched            int
	ConnectorClientsDeleted            int
	RefreshTokensScanned               int
	ConnectorRefreshTokensMatched      int
	ConnectorRefreshTokensDeleted      int
	RuntimeRefreshTokensPreserved      int
	AuthorizationCodesScanned          int
	ConnectorAuthorizationCodesMatched int
	ConnectorAuthorizationCodesDeleted int
	OAuthStatesScanned                 int
	ConnectorOAuthStatesMatched        int
	ConnectorOAuthStatesDeleted        int
	DeviceSessionsScanned              int
	ConnectorDeviceSessionsMatched     int
	ConnectorDeviceSessionsDeleted     int
	UserAppConsentsScanned             int
	ConnectorUserAppConsentsMatched    int
	ConnectorUserAppConsentsDeleted    int
	SampleConnectorClientIDs           []string
	SampleConnectorRefreshSessions     []string
	SampleRuntimeRefreshSessions       []string
}

type mcpAuthCutoverDeleteCandidate struct {
	PK     string
	SK     string
	Sample string
}

type mcpAuthCutoverOAuthClientCandidate struct {
	mcpAuthCutoverDeleteCandidate
	ClientID string
}

var newMCPAuthCutoverMigrationClientFn = func(cfg aws.Config) mcpAuthCutoverMigrationClient {
	return dynamodb.NewFromConfig(cfg)
}

func runMigrateMCPAuthCutover(argv []string) error {
	return runCommonMigrationCLI(
		argv,
		"migrate-mcp-auth-cutover",
		"maximum number of connector-era OAuth clients to process (0 = all)",
		"revoke connector-bound MCP sessions and retire persisted connector client records",
		func(ctx context.Context, awsCfg aws.Config, tableName string, apply bool, limit int) (mcpAuthCutoverMigrationSummary, error) {
			return executeMCPAuthCutoverMigration(
				ctx,
				newMCPAuthCutoverMigrationClientFn(awsCfg),
				tableName,
				apply,
				limit,
			)
		},
		printMCPAuthCutoverMigrationSummary,
	)
}

func printMCPAuthCutoverMigrationSummary(
	summary mcpAuthCutoverMigrationSummary,
	tableName string,
	resolvedProfile string,
	apply bool,
) {
	fmt.Printf("migrate-mcp-auth-cutover %s complete\n", selectedMigrationMode(apply))
	fmt.Printf("table: %s\n", tableName)
	if resolvedProfile != "" {
		fmt.Printf("aws_profile: %s\n", resolvedProfile)
	}
	fmt.Printf("oauth_clients_scanned: %d\n", summary.OAuthClientsScanned)
	fmt.Printf("connector_clients_matched: %d\n", summary.ConnectorClientsMatched)
	fmt.Printf("connector_clients_deleted: %d\n", summary.ConnectorClientsDeleted)
	fmt.Printf("refresh_tokens_scanned: %d\n", summary.RefreshTokensScanned)
	fmt.Printf("connector_refresh_tokens_matched: %d\n", summary.ConnectorRefreshTokensMatched)
	fmt.Printf("connector_refresh_tokens_deleted: %d\n", summary.ConnectorRefreshTokensDeleted)
	fmt.Printf("runtime_refresh_tokens_preserved: %d\n", summary.RuntimeRefreshTokensPreserved)
	fmt.Printf("authorization_codes_scanned: %d\n", summary.AuthorizationCodesScanned)
	fmt.Printf("connector_authorization_codes_matched: %d\n", summary.ConnectorAuthorizationCodesMatched)
	fmt.Printf("connector_authorization_codes_deleted: %d\n", summary.ConnectorAuthorizationCodesDeleted)
	fmt.Printf("oauth_states_scanned: %d\n", summary.OAuthStatesScanned)
	fmt.Printf("connector_oauth_states_matched: %d\n", summary.ConnectorOAuthStatesMatched)
	fmt.Printf("connector_oauth_states_deleted: %d\n", summary.ConnectorOAuthStatesDeleted)
	fmt.Printf("device_sessions_scanned: %d\n", summary.DeviceSessionsScanned)
	fmt.Printf("connector_device_sessions_matched: %d\n", summary.ConnectorDeviceSessionsMatched)
	fmt.Printf("connector_device_sessions_deleted: %d\n", summary.ConnectorDeviceSessionsDeleted)
	fmt.Printf("user_app_consents_scanned: %d\n", summary.UserAppConsentsScanned)
	fmt.Printf("connector_user_app_consents_matched: %d\n", summary.ConnectorUserAppConsentsMatched)
	fmt.Printf("connector_user_app_consents_deleted: %d\n", summary.ConnectorUserAppConsentsDeleted)
	printMigrationSamples("sample_connector_client_ids", summary.SampleConnectorClientIDs)
	printMigrationSamples("sample_connector_refresh_sessions", summary.SampleConnectorRefreshSessions)
	printMigrationSamples("sample_runtime_refresh_sessions", summary.SampleRuntimeRefreshSessions)

	if !apply {
		fmt.Println("no writes performed; re-run with --apply to revoke connector-bound refresh sessions and retire persisted connector client artifacts")
	}
}

func executeMCPAuthCutoverMigration(
	ctx context.Context,
	client mcpAuthCutoverMigrationClient,
	tableName string,
	apply bool,
	limit int,
) (mcpAuthCutoverMigrationSummary, error) {
	summary := mcpAuthCutoverMigrationSummary{}

	if client == nil {
		return summary, fmt.Errorf("migration client is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return summary, fmt.Errorf("table name is required")
	}

	oauthClientItems, err := scanMCPAuthCutoverItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :pk) AND SK = :sk",
		map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: mcpAuthCutoverOAuthClientPKPrefix},
			":sk": &types.AttributeValueMemberS{Value: mcpAuthCutoverOAuthClientSK},
		},
		nil,
	)
	if err != nil {
		return summary, fmt.Errorf("scan oauth clients: %w", err)
	}

	connectorClients, connectorClientIDs, err := buildMCPAuthCutoverOAuthClientCandidates(oauthClientItems, limit, &summary)
	if err != nil {
		return summary, err
	}

	refreshTokenItems, err := scanMCPAuthCutoverItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :pk) AND SK = :sk",
		map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: mcpAuthCutoverRefreshTokenPKPrefix},
			":sk": &types.AttributeValueMemberS{Value: mcpAuthCutoverRefreshTokenSK},
		},
		nil,
	)
	if err != nil {
		return summary, fmt.Errorf("scan refresh tokens: %w", err)
	}
	connectorRefreshTokens, runtimeRefreshTokens, err := buildMCPAuthCutoverRefreshCandidates(refreshTokenItems, connectorClientIDs, &summary)
	if err != nil {
		return summary, err
	}

	authorizationCodeItems, err := scanMCPAuthCutoverItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :pk) AND SK = :sk",
		map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: mcpAuthCutoverAuthorizationCodePK},
			":sk": &types.AttributeValueMemberS{Value: mcpAuthCutoverAuthorizationCodeSK},
		},
		nil,
	)
	if err != nil {
		return summary, fmt.Errorf("scan authorization codes: %w", err)
	}
	connectorAuthorizationCodes, err := buildMCPAuthCutoverAuthorizationCodeCandidates(authorizationCodeItems, connectorClientIDs, &summary)
	if err != nil {
		return summary, err
	}

	oauthStateItems, err := scanMCPAuthCutoverItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :pk) AND SK = :sk",
		map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: mcpAuthCutoverOAuthStatePKPrefix},
			":sk": &types.AttributeValueMemberS{Value: mcpAuthCutoverOAuthStateSK},
		},
		nil,
	)
	if err != nil {
		return summary, fmt.Errorf("scan oauth state: %w", err)
	}
	connectorStates, err := buildMCPAuthCutoverOAuthStateCandidates(oauthStateItems, connectorClientIDs, &summary)
	if err != nil {
		return summary, err
	}

	deviceSessionItems, err := scanMCPAuthCutoverItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :pk) AND SK = :sk",
		map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: mcpAuthCutoverOAuthDevicePKPrefix},
			":sk": &types.AttributeValueMemberS{Value: mcpAuthCutoverOAuthDeviceSK},
		},
		nil,
	)
	if err != nil {
		return summary, fmt.Errorf("scan oauth device sessions: %w", err)
	}
	connectorDeviceSessions, err := buildMCPAuthCutoverDeviceSessionCandidates(deviceSessionItems, connectorClientIDs, &summary)
	if err != nil {
		return summary, err
	}

	consentItems, err := scanMCPAuthCutoverItems(
		ctx,
		client,
		tableName,
		"begins_with(PK, :pk) AND begins_with(SK, :sk)",
		map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: mcpAuthCutoverUserConsentPKPrefix},
			":sk": &types.AttributeValueMemberS{Value: mcpAuthCutoverUserConsentSKPrefix},
		},
		nil,
	)
	if err != nil {
		return summary, fmt.Errorf("scan user app consents: %w", err)
	}
	connectorConsents, err := buildMCPAuthCutoverUserConsentCandidates(consentItems, connectorClientIDs, &summary)
	if err != nil {
		return summary, err
	}

	if err := deleteMCPAuthCutoverCandidates(ctx, client, tableName, apply, connectorAuthorizationCodes, &summary.ConnectorAuthorizationCodesDeleted); err != nil {
		return summary, err
	}
	if err := deleteMCPAuthCutoverCandidates(ctx, client, tableName, apply, connectorStates, &summary.ConnectorOAuthStatesDeleted); err != nil {
		return summary, err
	}
	if err := deleteMCPAuthCutoverCandidates(ctx, client, tableName, apply, connectorDeviceSessions, &summary.ConnectorDeviceSessionsDeleted); err != nil {
		return summary, err
	}
	if err := deleteMCPAuthCutoverCandidates(ctx, client, tableName, apply, connectorConsents, &summary.ConnectorUserAppConsentsDeleted); err != nil {
		return summary, err
	}
	if err := deleteMCPAuthCutoverCandidates(ctx, client, tableName, apply, connectorRefreshTokens, &summary.ConnectorRefreshTokensDeleted); err != nil {
		return summary, err
	}
	if err := deleteMCPAuthCutoverOAuthClientCandidates(ctx, client, tableName, apply, connectorClients, &summary.ConnectorClientsDeleted); err != nil {
		return summary, err
	}

	summary.SampleConnectorClientIDs = sampleStrings(summary.SampleConnectorClientIDs, 5)
	summary.SampleConnectorRefreshSessions = sampleStrings(summary.SampleConnectorRefreshSessions, 5)
	summary.SampleRuntimeRefreshSessions = sampleStrings(summary.SampleRuntimeRefreshSessions, 5)
	_ = runtimeRefreshTokens

	return summary, nil
}

func scanMCPAuthCutoverItems(
	ctx context.Context,
	client mcpAuthCutoverMigrationClient,
	tableName string,
	filterExpression string,
	expressionAttributeValues map[string]types.AttributeValue,
	expressionAttributeNames map[string]string,
) ([]map[string]types.AttributeValue, error) {
	scanInput := &dynamodb.ScanInput{
		TableName:                 aws.String(tableName),
		FilterExpression:          aws.String(filterExpression),
		ExpressionAttributeValues: expressionAttributeValues,
	}
	if len(expressionAttributeNames) > 0 {
		scanInput.ExpressionAttributeNames = expressionAttributeNames
	}

	var items []map[string]types.AttributeValue
	for {
		out, err := client.Scan(ctx, scanInput)
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		scanInput.ExclusiveStartKey = out.LastEvaluatedKey
	}
}

func buildMCPAuthCutoverOAuthClientCandidates(
	items []map[string]types.AttributeValue,
	limit int,
	summary *mcpAuthCutoverMigrationSummary,
) ([]mcpAuthCutoverOAuthClientCandidate, map[string]struct{}, error) {
	summary.OAuthClientsScanned = len(items)

	candidates := make([]mcpAuthCutoverOAuthClientCandidate, 0, len(items))
	for _, item := range items {
		var client models.OAuthClient
		if err := attributevalue.UnmarshalMap(item, &client); err != nil {
			return nil, nil, fmt.Errorf("unmarshal oauth client: %w", err)
		}
		if !isConnectorOAuthClient(&client) {
			continue
		}

		clientID := strings.TrimSpace(client.ClientID)
		candidates = append(candidates, mcpAuthCutoverOAuthClientCandidate{
			mcpAuthCutoverDeleteCandidate: mcpAuthCutoverDeleteCandidate{
				PK:     strings.TrimSpace(client.PK),
				SK:     strings.TrimSpace(client.SK),
				Sample: clientID,
			},
			ClientID: clientID,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ClientID < candidates[j].ClientID
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	clientIDs := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		clientIDs[candidate.ClientID] = struct{}{}
		summary.SampleConnectorClientIDs = append(summary.SampleConnectorClientIDs, candidate.Sample)
	}
	summary.ConnectorClientsMatched = len(candidates)

	return candidates, clientIDs, nil
}

func buildMCPAuthCutoverRefreshCandidates(
	items []map[string]types.AttributeValue,
	connectorClientIDs map[string]struct{},
	summary *mcpAuthCutoverMigrationSummary,
) ([]mcpAuthCutoverDeleteCandidate, []mcpAuthCutoverDeleteCandidate, error) {
	summary.RefreshTokensScanned = len(items)

	connectorCandidates := make([]mcpAuthCutoverDeleteCandidate, 0, len(items))
	runtimeCandidates := make([]mcpAuthCutoverDeleteCandidate, 0, len(items))
	for _, item := range items {
		var token models.RefreshToken
		if err := attributevalue.UnmarshalMap(item, &token); err != nil {
			return nil, nil, fmt.Errorf("unmarshal refresh token: %w", err)
		}

		candidate := mcpAuthCutoverDeleteCandidate{
			PK:     strings.TrimSpace(token.PK),
			SK:     strings.TrimSpace(token.SK),
			Sample: summarizeMCPAuthCutoverRefreshToken(&token),
		}

		switch {
		case isPreservedRuntimeRefreshToken(&token):
			runtimeCandidates = append(runtimeCandidates, candidate)
			summary.SampleRuntimeRefreshSessions = append(summary.SampleRuntimeRefreshSessions, candidate.Sample)
		case isConnectorRefreshToken(&token, connectorClientIDs):
			connectorCandidates = append(connectorCandidates, candidate)
			summary.SampleConnectorRefreshSessions = append(summary.SampleConnectorRefreshSessions, candidate.Sample)
		}
	}

	summary.ConnectorRefreshTokensMatched = len(connectorCandidates)
	summary.RuntimeRefreshTokensPreserved = len(runtimeCandidates)
	return connectorCandidates, runtimeCandidates, nil
}

func buildMCPAuthCutoverAuthorizationCodeCandidates(
	items []map[string]types.AttributeValue,
	connectorClientIDs map[string]struct{},
	summary *mcpAuthCutoverMigrationSummary,
) ([]mcpAuthCutoverDeleteCandidate, error) {
	summary.AuthorizationCodesScanned = len(items)

	candidates := make([]mcpAuthCutoverDeleteCandidate, 0, len(items))
	for _, item := range items {
		var code models.AuthorizationCode
		if err := attributevalue.UnmarshalMap(item, &code); err != nil {
			return nil, fmt.Errorf("unmarshal authorization code: %w", err)
		}
		if !isConnectorAuthorizationCode(&code, connectorClientIDs) {
			continue
		}
		candidates = append(candidates, mcpAuthCutoverDeleteCandidate{
			PK:     strings.TrimSpace(code.PK),
			SK:     strings.TrimSpace(code.SK),
			Sample: summarizeMCPAuthCutoverClientArtifact(code.ClientID, code.Username, code.Resource),
		})
	}

	summary.ConnectorAuthorizationCodesMatched = len(candidates)
	return candidates, nil
}

func buildMCPAuthCutoverOAuthStateCandidates(
	items []map[string]types.AttributeValue,
	connectorClientIDs map[string]struct{},
	summary *mcpAuthCutoverMigrationSummary,
) ([]mcpAuthCutoverDeleteCandidate, error) {
	summary.OAuthStatesScanned = len(items)

	candidates := make([]mcpAuthCutoverDeleteCandidate, 0, len(items))
	for _, item := range items {
		var state models.OAuthState
		if err := attributevalue.UnmarshalMap(item, &state); err != nil {
			return nil, fmt.Errorf("unmarshal oauth state: %w", err)
		}
		if !isConnectorOAuthState(&state, connectorClientIDs) {
			continue
		}
		candidates = append(candidates, mcpAuthCutoverDeleteCandidate{
			PK:     strings.TrimSpace(state.PK),
			SK:     strings.TrimSpace(state.SK),
			Sample: summarizeMCPAuthCutoverClientArtifact(state.ClientID, state.Username, state.Resource),
		})
	}

	summary.ConnectorOAuthStatesMatched = len(candidates)
	return candidates, nil
}

func buildMCPAuthCutoverDeviceSessionCandidates(
	items []map[string]types.AttributeValue,
	connectorClientIDs map[string]struct{},
	summary *mcpAuthCutoverMigrationSummary,
) ([]mcpAuthCutoverDeleteCandidate, error) {
	summary.DeviceSessionsScanned = len(items)

	candidates := make([]mcpAuthCutoverDeleteCandidate, 0, len(items))
	for _, item := range items {
		var session models.OAuthDeviceSession
		if err := attributevalue.UnmarshalMap(item, &session); err != nil {
			return nil, fmt.Errorf("unmarshal oauth device session: %w", err)
		}
		if !isConnectorDeviceSession(&session, connectorClientIDs) {
			continue
		}
		candidates = append(candidates, mcpAuthCutoverDeleteCandidate{
			PK:     strings.TrimSpace(session.PK),
			SK:     strings.TrimSpace(session.SK),
			Sample: summarizeMCPAuthCutoverClientArtifact(session.ClientID, session.ApprovedUsername, ""),
		})
	}

	summary.ConnectorDeviceSessionsMatched = len(candidates)
	return candidates, nil
}

func buildMCPAuthCutoverUserConsentCandidates(
	items []map[string]types.AttributeValue,
	connectorClientIDs map[string]struct{},
	summary *mcpAuthCutoverMigrationSummary,
) ([]mcpAuthCutoverDeleteCandidate, error) {
	summary.UserAppConsentsScanned = len(items)

	candidates := make([]mcpAuthCutoverDeleteCandidate, 0, len(items))
	for _, item := range items {
		var consent models.UserAppConsent
		if err := attributevalue.UnmarshalMap(item, &consent); err != nil {
			return nil, fmt.Errorf("unmarshal user app consent: %w", err)
		}
		if !isConnectorUserConsent(&consent, connectorClientIDs) {
			continue
		}
		candidates = append(candidates, mcpAuthCutoverDeleteCandidate{
			PK:     strings.TrimSpace(consent.PK),
			SK:     strings.TrimSpace(consent.SK),
			Sample: summarizeMCPAuthCutoverClientArtifact(consent.AppID, consent.UserID, consent.Resource),
		})
	}

	summary.ConnectorUserAppConsentsMatched = len(candidates)
	return candidates, nil
}

func deleteMCPAuthCutoverOAuthClientCandidates(
	ctx context.Context,
	client mcpAuthCutoverMigrationClient,
	tableName string,
	apply bool,
	candidates []mcpAuthCutoverOAuthClientCandidate,
	deleted *int,
) error {
	keys := make([]mcpAuthCutoverDeleteCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		keys = append(keys, candidate.mcpAuthCutoverDeleteCandidate)
	}
	return deleteMCPAuthCutoverCandidates(ctx, client, tableName, apply, keys, deleted)
}

func deleteMCPAuthCutoverCandidates(
	ctx context.Context,
	client mcpAuthCutoverMigrationClient,
	tableName string,
	apply bool,
	candidates []mcpAuthCutoverDeleteCandidate,
	deleted *int,
) error {
	if !apply {
		return nil
	}

	for _, candidate := range candidates {
		if _, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: candidate.PK},
				"SK": &types.AttributeValueMemberS{Value: candidate.SK},
			},
		}); err != nil {
			return fmt.Errorf("delete %s %s: %w", candidate.PK, candidate.SK, err)
		}
		if deleted != nil {
			*deleted = *deleted + 1
		}
	}

	return nil
}

func isConnectorOAuthClient(client *models.OAuthClient) bool {
	if client == nil {
		return false
	}
	clientID := strings.TrimSpace(client.ClientID)
	if clientID == "" || auth.IsAgentRuntimeClientID(clientID) {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(client.ClientClass), auth.ClientClassAgent) ||
		strings.TrimSpace(client.AgentUsername) != ""
}

func isPreservedRuntimeRefreshToken(token *models.RefreshToken) bool {
	return token != nil && auth.IsAgentRuntimeClientID(strings.TrimSpace(token.ClientID))
}

func isConnectorRefreshToken(token *models.RefreshToken, connectorClientIDs map[string]struct{}) bool {
	if token == nil {
		return false
	}
	clientID := strings.TrimSpace(token.ClientID)
	if clientID == "" || auth.IsAgentRuntimeClientID(clientID) {
		return false
	}
	if _, ok := connectorClientIDs[clientID]; ok {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(token.ClientClass), auth.ClientClassAgent)
}

func isConnectorAuthorizationCode(code *models.AuthorizationCode, connectorClientIDs map[string]struct{}) bool {
	if code == nil {
		return false
	}
	_, ok := connectorClientIDs[strings.TrimSpace(code.ClientID)]
	return ok
}

func isConnectorOAuthState(state *models.OAuthState, connectorClientIDs map[string]struct{}) bool {
	if state == nil {
		return false
	}
	if strings.TrimSpace(state.AgentUsername) != "" {
		return true
	}
	_, ok := connectorClientIDs[strings.TrimSpace(state.ClientID)]
	return ok
}

func isConnectorDeviceSession(session *models.OAuthDeviceSession, connectorClientIDs map[string]struct{}) bool {
	if session == nil {
		return false
	}
	_, ok := connectorClientIDs[strings.TrimSpace(session.ClientID)]
	return ok
}

func isConnectorUserConsent(consent *models.UserAppConsent, connectorClientIDs map[string]struct{}) bool {
	if consent == nil {
		return false
	}
	_, ok := connectorClientIDs[strings.TrimSpace(consent.AppID)]
	return ok
}

func summarizeMCPAuthCutoverRefreshToken(token *models.RefreshToken) string {
	if token == nil {
		return ""
	}
	resource := strings.TrimSpace(token.Resource)
	if resource != "" {
		return summarizeMCPAuthCutoverClientArtifact(token.ClientID, token.Username, resource)
	}
	sessionID := strings.TrimSpace(token.SessionID)
	if sessionID != "" {
		return fmt.Sprintf("%s user=%s session=%s", strings.TrimSpace(token.ClientID), strings.TrimSpace(token.Username), sessionID)
	}
	return summarizeMCPAuthCutoverClientArtifact(token.ClientID, token.Username, "")
}

func summarizeMCPAuthCutoverClientArtifact(clientID, username, resource string) string {
	clientID = strings.TrimSpace(clientID)
	username = strings.TrimSpace(username)
	resource = strings.TrimSpace(resource)

	parts := make([]string, 0, 3)
	if clientID != "" {
		parts = append(parts, clientID)
	}
	if username != "" {
		parts = append(parts, "user="+username)
	}
	if resource != "" {
		parts = append(parts, "resource="+resource)
	}
	return strings.Join(parts, " ")
}

func sampleStrings(values []string, limit int) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, minInt(limit, len(values)))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
