package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type hostedGenesisProjectionContract struct {
	Kind                   string                               `json:"kind"`
	SchemaVersion          int                                  `json:"schema_version"`
	ContractVersion        string                               `json:"contract_version"`
	GraphQLOperations      []string                             `json:"graphql_operations"`
	LockedProjectionFields []hostedGenesisLockedProjectionField `json:"locked_projection_fields"`
	CanonicalHostStatuses  []string                             `json:"canonical_host_statuses"`
	Rows                   []hostedGenesisProjectionRow         `json:"rows"`
}

type hostedGenesisLockedProjectionField struct {
	PersistedField             string `json:"persisted_field"`
	CurrentGraphQLField        string `json:"current_graphql_field"`
	M3CompactGraphQLField      string `json:"m3_compact_graphql_field"`
	ImplementedInCurrentSchema bool   `json:"implemented_in_current_schema"`
	Rule                       string `json:"rule"`
}

type hostedGenesisProjectionRow struct {
	HostStatus                  string                         `json:"host_status"`
	Label                       string                         `json:"label"`
	AcceptedHostStatuses        []string                       `json:"accepted_host_statuses"`
	Phase                       string                         `json:"phase"`
	Status                      string                         `json:"status"`
	TypedNextActionOptions      []string                       `json:"typed_next_action_options"`
	ExampleTypedNextAction      string                         `json:"example_typed_next_action"`
	Recovery                    *hostedGenesisRecovery         `json:"recovery"`
	HostIDs                     map[string]string              `json:"host_ids"`
	TerminalDeclarationEvidence string                         `json:"terminal_declaration_evidence"`
	PublicationEvidence         string                         `json:"publication_evidence"`
	PublishGate                 string                         `json:"publish_gate"`
	Example                     hostedGenesisProjectionExample `json:"example"`
}

type hostedGenesisRecovery struct {
	Kind     string   `json:"kind"`
	Category string   `json:"category"`
	Action   string   `json:"action"`
	Alts     []string `json:"alternatives"`
}

type hostedGenesisProjectionExample struct {
	Host          any `json:"host"`
	LesserGraphQL struct {
		State       hostedGenesisProjectionState `json:"state"`
		PublishGate hostedGenesisPublishGate     `json:"publishGate"`
	} `json:"lesserGraphql"`
}

type hostedGenesisProjectionState struct {
	HostRegistrationID          *string          `json:"hostRegistrationId"`
	HostSoulAgentID             *string          `json:"hostSoulAgentId"`
	HostConversationID          *string          `json:"hostConversationId"`
	Phase                       string           `json:"phase"`
	State                       string           `json:"state"`
	TypedNextAction             string           `json:"typedNextAction"`
	RecoveryCategory            *string          `json:"recoveryCategory"`
	RecoveryAction              *string          `json:"recoveryAction"`
	SigningCheckpoints          []map[string]any `json:"signingCheckpoints"`
	TerminalDeclarationEvidence map[string]any   `json:"terminalDeclarationEvidence"`
	Publication                 map[string]any   `json:"publication"`
	Error                       map[string]any   `json:"error"`
}

type hostedGenesisPublishGate struct {
	CanPublishHostedSoul                                  bool   `json:"canPublishHostedSoul"`
	Reason                                                string `json:"reason"`
	RequiresActiveConversationTerminalDeclarationEvidence bool   `json:"requiresActiveConversationTerminalDeclarationEvidence"`
}

func TestHostedSoulGenesisProjectionTableContract(t *testing.T) {
	repoRoot, err := findRepoRoot()
	require.NoError(t, err)

	fixturePath := filepath.Join(repoRoot, "docs", "contracts", "examples", "hosted-soul-genesis-projection-table.example.json")
	fixtureData, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	var contract hostedGenesisProjectionContract
	require.NoError(t, json.Unmarshal(fixtureData, &contract))
	require.Equal(t, "lesser.hosted_soul_genesis_projection_table", contract.Kind)
	require.Equal(t, 1, contract.SchemaVersion)
	require.Equal(t, "p49-m1.2", contract.ContractVersion)

	expectedStatuses := []string{
		"no_registration",
		"registration_active_no_conversation",
		"in_progress",
		"assistant_turn_ready",
		"declaration_extraction_pending",
		"declaration_ready",
		"failed",
		"published_bound",
	}
	require.Equal(t, expectedStatuses, contract.CanonicalHostStatuses)
	require.Len(t, contract.Rows, len(expectedStatuses))

	schemaData, err := os.ReadFile(filepath.Join(repoRoot, "docs", "contracts", "graphql-schema.graphql"))
	require.NoError(t, err)
	schema := string(schemaData)

	phaseValues := hostedGenesisGraphQLEnumValues(t, schema, "SoulBootstrapPhase")
	nextActionValues := hostedGenesisGraphQLEnumValues(t, schema, "SoulBootstrapNextAction")
	recoveryCategoryValues := hostedGenesisGraphQLEnumValues(t, schema, "SoulBootstrapRecoveryCategory")
	recoveryActionValues := hostedGenesisGraphQLEnumValues(t, schema, "SoulBootstrapRecoveryAction")
	stateFields := hostedGenesisGraphQLTypeFields(t, schema, "SoulBootstrapState")

	for _, op := range contract.GraphQLOperations {
		require.Contains(t, schema, op+"(input:", "hosted GraphQL operation must remain published")
	}
	for _, field := range contract.LockedProjectionFields {
		require.NotEmpty(t, field.PersistedField)
		require.NotEmpty(t, field.CurrentGraphQLField)
		require.NotEmpty(t, field.M3CompactGraphQLField)
		require.NotEmpty(t, field.Rule)
		if field.ImplementedInCurrentSchema {
			require.Contains(t, stateFields, field.CurrentGraphQLField, "current projection field must exist in SoulBootstrapState")
		}
	}

	seen := make(map[string]struct{}, len(contract.Rows))
	for idx, row := range contract.Rows {
		require.Equal(t, expectedStatuses[idx], row.HostStatus)
		require.NotEmpty(t, row.Label)
		require.NotEmpty(t, row.Status)
		require.Contains(t, phaseValues, row.Phase)
		require.NotEmpty(t, row.TypedNextActionOptions)
		require.Contains(t, nextActionValues, row.ExampleTypedNextAction)
		require.Contains(t, row.TypedNextActionOptions, row.ExampleTypedNextAction)
		require.Equal(t, row.Phase, row.Example.LesserGraphQL.State.Phase)
		require.Equal(t, row.Status, row.Example.LesserGraphQL.State.State)
		require.Equal(t, row.ExampleTypedNextAction, row.Example.LesserGraphQL.State.TypedNextAction)
		require.Equal(t, row.PublishGate, row.Example.LesserGraphQL.PublishGate.Reason)
		require.True(t, row.Example.LesserGraphQL.PublishGate.RequiresActiveConversationTerminalDeclarationEvidence)

		_, duplicate := seen[row.HostStatus]
		require.False(t, duplicate, "duplicate hosted genesis projection row %q", row.HostStatus)
		seen[row.HostStatus] = struct{}{}

		for _, action := range row.TypedNextActionOptions {
			require.Contains(t, nextActionValues, action)
		}
		if row.Recovery != nil {
			require.Contains(t, recoveryCategoryValues, row.Recovery.Category)
			require.Contains(t, recoveryActionValues, row.Recovery.Action)
			require.Equal(t, row.Recovery.Category, derefHostedGenesisString(row.Example.LesserGraphQL.State.RecoveryCategory))
			require.Equal(t, row.Recovery.Action, derefHostedGenesisString(row.Example.LesserGraphQL.State.RecoveryAction))
		}

		validateHostedGenesisProjectionInvariants(t, row)
	}

	docData, err := os.ReadFile(filepath.Join(repoRoot, "docs", "contracts", "hosted-soul-genesis-projection.md"))
	require.NoError(t, err)
	doc := string(docData)
	require.Contains(t, doc, "hosted-soul-genesis-projection-table.example.json")
	for _, row := range contract.Rows {
		require.Contains(t, doc, row.Label)
	}
}

func validateHostedGenesisProjectionInvariants(t *testing.T, row hostedGenesisProjectionRow) {
	t.Helper()

	state := row.Example.LesserGraphQL.State
	conversationRows := map[string]struct{}{
		"in_progress":                    {},
		"assistant_turn_ready":           {},
		"declaration_extraction_pending": {},
		"declaration_ready":              {},
		"failed":                         {},
		"published_bound":                {},
	}
	if _, ok := conversationRows[row.HostStatus]; ok {
		require.NotNil(t, state.HostConversationID, "%s must persist hostConversationId early", row.HostStatus)
		require.NotEmpty(t, derefHostedGenesisString(state.HostConversationID))
		require.Contains(t, row.HostIDs["host_conversation_id"], "required")
	}

	if row.HostStatus != "no_registration" {
		require.NotNil(t, state.HostRegistrationID)
		require.NotNil(t, state.HostSoulAgentID)
	}

	if row.HostStatus == "in_progress" {
		require.Equal(t, "CONVERSATION", row.Phase)
		require.NotContains(t, strings.ToLower(row.Status), "error")
		require.NotEqual(t, "ERROR", row.Phase)
		require.Equal(t, "REFRESH_STATE", row.ExampleTypedNextAction)
		require.False(t, row.Example.LesserGraphQL.PublishGate.CanPublishHostedSoul)
		require.Nil(t, state.Error)
	}

	if row.ExampleTypedNextAction == "PUBLISH_HOSTED_SOUL" {
		require.Equal(t, "declaration_ready", row.HostStatus)
		require.True(t, row.Example.LesserGraphQL.PublishGate.CanPublishHostedSoul)
		require.Equal(t, "present:active_conversation", row.TerminalDeclarationEvidence)
		require.NotNil(t, state.TerminalDeclarationEvidence)
		require.Equal(t, derefHostedGenesisString(state.HostConversationID), state.TerminalDeclarationEvidence["conversationId"])
	} else if row.HostStatus != "published_bound" && row.HostStatus != "declaration_ready" {
		require.NotContains(t, row.TypedNextActionOptions, "PUBLISH_HOSTED_SOUL")
		require.False(t, row.Example.LesserGraphQL.PublishGate.CanPublishHostedSoul)
		require.True(t, strings.HasPrefix(row.PublishGate, "blocked:"), "non-terminal rows must fail closed")
	}

	if row.HostStatus == "failed" {
		require.Equal(t, "ERROR", row.Phase)
		require.NotNil(t, row.Recovery)
		require.NotNil(t, state.Error)
		require.NotEqual(t, "HOST_RESPONSE_INVALID", state.Error["code"])
		require.NotContains(t, row.Status, "host_unavailable")
	}

	if row.HostStatus == "published_bound" || row.HostStatus == "declaration_ready" {
		require.Equal(t, "COMPLETE", row.Phase)
		require.Equal(t, "COMPLETE", row.ExampleTypedNextAction)
		require.NotNil(t, state.TerminalDeclarationEvidence)
		require.NotNil(t, state.Publication)
		require.False(t, row.Example.LesserGraphQL.PublishGate.CanPublishHostedSoul)
		require.True(t, strings.HasPrefix(row.PublishGate, "complete:"))
	}
}

func hostedGenesisGraphQLEnumValues(t *testing.T, schema string, enumName string) map[string]struct{} {
	t.Helper()

	pattern := regexp.MustCompile(`(?s)enum\s+` + regexp.QuoteMeta(enumName) + `\s*\{([^}]*)\}`)
	matches := pattern.FindStringSubmatch(schema)
	require.Len(t, matches, 2, "missing GraphQL enum %s", enumName)

	values := map[string]struct{}{}
	for _, line := range strings.Split(matches[1], "\n") {
		line = strings.TrimSpace(strings.Split(line, "#")[0])
		if line == "" || strings.HasPrefix(line, "\"") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		values[strings.TrimSuffix(parts[0], ",")] = struct{}{}
	}
	require.NotEmpty(t, values, "GraphQL enum %s must have values", enumName)
	return values
}

func hostedGenesisGraphQLTypeFields(t *testing.T, schema string, typeName string) map[string]struct{} {
	t.Helper()

	pattern := regexp.MustCompile(`(?s)type\s+` + regexp.QuoteMeta(typeName) + `\s*\{([^}]*)\}`)
	matches := pattern.FindStringSubmatch(schema)
	require.Len(t, matches, 2, "missing GraphQL type %s", typeName)

	fields := map[string]struct{}{}
	for _, line := range strings.Split(matches[1], "\n") {
		line = strings.TrimSpace(strings.Split(line, "#")[0])
		if line == "" || strings.HasPrefix(line, "\"") {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(name)] = struct{}{}
	}
	require.NotEmpty(t, fields, "GraphQL type %s must have fields", typeName)
	return fields
}

func derefHostedGenesisString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
